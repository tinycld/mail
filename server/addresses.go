package mail

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var addressLog = logging.ForPackage("mail")

// Domain coverage: who gets an automatic address on a domain. Phase 2 adds
// "groups".
const (
	coverageEveryone = "everyone"
	coverageManual   = "manual"
)

// An address row is either added by hand (manual) or written by the
// reconciler from a domain rule (rule). Rule rows belong to the server.
const (
	addressSourceManual = "manual"
	addressSourceRule   = "rule"
)

var errAddressNotFound = errors.New("address not found")

// resolvedAddress is one mail_addresses row with the inbox it delivers to and
// the domain it is on.
type resolvedAddress struct {
	Address *core.Record
	Mailbox *core.Record
	Domain  *core.Record
}

// Email returns the full address, local@domain.
func (r *resolvedAddress) Email() string {
	return r.Address.GetString("local_part") + "@" + r.Domain.GetString("domain")
}

func normalizeLocalPart(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// domainHasRule reports whether a domain gives addresses automatically, so a
// person's email name is reserved on it.
func domainHasRule(domain *core.Record) bool {
	return domain.GetString("coverage") == coverageEveryone
}

// resolveAddress is the one lookup for "which inbox receives mail for
// local@domain" and "which inbox may send as it". It reads mail_addresses
// only; plus-tag stripping is the caller's job.
func resolveAddress(app core.App, localPart, domainName string) (*resolvedAddress, error) {
	local := normalizeLocalPart(localPart)
	name := strings.ToLower(strings.TrimSpace(domainName))
	if local == "" || name == "" {
		return nil, errAddressNotFound
	}
	domain, err := app.FindFirstRecordByFilter("mail_domains", "domain = {:domain}", dbx.Params{"domain": name})
	if err != nil {
		return nil, fmt.Errorf("%w: no domain %s", errAddressNotFound, name)
	}
	address, err := findAddressRow(app, local, domain.Id)
	if err != nil {
		return nil, err
	}
	if address == nil {
		return nil, fmt.Errorf("%w: %s@%s", errAddressNotFound, local, name)
	}
	mailbox, err := app.FindRecordById("mail_mailboxes", address.GetString("mailbox"))
	if err != nil {
		return nil, fmt.Errorf("%s@%s points at a missing mailbox: %w", local, name, err)
	}
	return &resolvedAddress{Address: address, Mailbox: mailbox, Domain: domain}, nil
}

// loadResolvedAddress loads an address row by id with its inbox and domain.
func loadResolvedAddress(app core.App, addressID string) (*resolvedAddress, error) {
	address, err := app.FindRecordById("mail_addresses", addressID)
	if err != nil {
		return nil, fmt.Errorf("%w: id %s", errAddressNotFound, addressID)
	}
	domain, err := app.FindRecordById("mail_domains", address.GetString("domain"))
	if err != nil {
		return nil, fmt.Errorf("address %s points at a missing domain: %w", addressID, err)
	}
	mailbox, err := app.FindRecordById("mail_mailboxes", address.GetString("mailbox"))
	if err != nil {
		return nil, fmt.Errorf("address %s points at a missing mailbox: %w", addressID, err)
	}
	return &resolvedAddress{Address: address, Mailbox: mailbox, Domain: domain}, nil
}

// findAddressRow returns the row for (localPart, domainID), or nil when there
// is none. localPart must already be normalized.
func findAddressRow(app core.App, localPart, domainID string) (*core.Record, error) {
	row, err := app.FindFirstRecordByFilter("mail_addresses",
		"local_part = {:local} && domain = {:domain}",
		dbx.Params{"local": localPart, "domain": domainID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("address lookup failed: %w", err)
	}
	return row, nil
}

// checkAddressAvailable refuses an address that another row already holds, or
// that is a person's email name on a domain with a rule. mailboxID is the inbox
// the address would deliver to ("" for an inbox not yet saved); selfID is the
// row being updated ("" on create).
func checkAddressAvailable(app core.App, localPart, domainID, mailboxID, selfID string) error {
	existing, err := findAddressRow(app, localPart, domainID)
	if err != nil {
		return err
	}
	if existing != nil && existing.Id != selfID {
		return fmt.Errorf("%s is already an address on this domain", localPart)
	}
	domain, err := app.FindRecordById("mail_domains", domainID)
	if err != nil {
		return fmt.Errorf("unknown domain: %w", err)
	}
	if !domainHasRule(domain) {
		return nil
	}
	owner, err := app.FindFirstRecordByFilter("mail_identities", "email_name = {:name}", dbx.Params{"name": localPart})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("identity lookup failed: %w", err)
	}
	if owner.GetString("inbox") != mailboxID {
		return fmt.Errorf("%s is reserved on %s: it is a person's email name, and everyone gets that name there",
			localPart, domain.GetString("domain"))
	}
	return nil
}

// savePrimaryAddress writes the inbox's own address (mail_mailboxes.address on
// mail_mailboxes.domain) as its primary row, creating or moving that row.
func savePrimaryAddress(app core.App, mailbox *core.Record) error {
	row, err := app.FindFirstRecordByFilter("mail_addresses",
		"mailbox = {:mailbox} && is_primary = true", dbx.Params{"mailbox": mailbox.Id})
	if errors.Is(err, sql.ErrNoRows) {
		col, err := app.FindCollectionByNameOrId("mail_addresses")
		if err != nil {
			return err
		}
		row = core.NewRecord(col)
		row.Set("mailbox", mailbox.Id)
		row.Set("source", addressSourceManual)
		row.Set("is_primary", true)
	} else if err != nil {
		return err
	}
	row.Set("local_part", normalizeLocalPart(mailbox.GetString("address")))
	row.Set("domain", mailbox.GetString("domain"))
	return app.Save(row)
}

// applyDomainDefaults gives a new domain a rank after every existing domain.
// The first domain gives everyone an address (so the first users get one with
// no extra step); every later domain starts as "only people I add", so adding
// a domain never hands out addresses until an admin chooses to.
func applyDomainDefaults(app core.App, domain *core.Record) error {
	var stats struct {
		Count   int `db:"count"`
		MaxRank int `db:"max_rank"`
	}
	err := app.DB().NewQuery("SELECT COUNT(*) AS count, COALESCE(MAX(`rank`), 0) AS max_rank FROM mail_domains").One(&stats)
	if err != nil {
		return fmt.Errorf("read domain ranks: %w", err)
	}
	if domain.GetString("coverage") == "" {
		if stats.Count == 0 {
			domain.Set("coverage", coverageEveryone)
		} else {
			domain.Set("coverage", coverageManual)
		}
	}
	if domain.GetInt("rank") == 0 {
		domain.Set("rank", stats.MaxRank+1)
	}
	return nil
}

// registerAddressHooks keeps mail_addresses consistent with the mailboxes and
// legacy aliases that feed it, and guards the rows the server owns.
func registerAddressHooks(app core.App) {
	app.OnRecordCreate("mail_domains").BindFunc(func(e *core.RecordEvent) error {
		if err := applyDomainDefaults(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})

	app.OnRecordCreate("mail_addresses").BindFunc(func(e *core.RecordEvent) error {
		local := normalizeLocalPart(e.Record.GetString("local_part"))
		e.Record.Set("local_part", local)
		if err := checkAddressAvailable(e.App, local, e.Record.GetString("domain"), e.Record.GetString("mailbox"), ""); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordUpdate("mail_addresses").BindFunc(func(e *core.RecordEvent) error {
		local := normalizeLocalPart(e.Record.GetString("local_part"))
		e.Record.Set("local_part", local)
		if err := checkAddressAvailable(e.App, local, e.Record.GetString("domain"), e.Record.GetString("mailbox"), e.Record.Id); err != nil {
			return err
		}
		return e.Next()
	})

	// Through the API an admin may only add or remove hand-made, non-primary
	// addresses: rule rows follow the domain's rule, and a primary row follows
	// its mailbox's address.
	app.OnRecordCreateRequest("mail_addresses").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth == nil || !e.Auth.IsSuperuser() {
			e.Record.Set("source", addressSourceManual)
			e.Record.Set("is_primary", false)
		}
		return e.Next()
	})
	app.OnRecordDeleteRequest("mail_addresses").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		if e.Record.GetString("source") == addressSourceRule {
			return e.ForbiddenError("This address comes from its domain's rule. To remove it, change who gets an address on that domain.", nil)
		}
		if e.Record.GetBool("is_primary") {
			return e.ForbiddenError("This is the inbox's main address. Change the inbox's address instead.", nil)
		}
		return e.Next()
	})

	app.OnRecordCreate("mail_mailboxes").BindFunc(func(e *core.RecordEvent) error {
		local := normalizeLocalPart(e.Record.GetString("address"))
		e.Record.Set("address", local)
		if err := checkAddressAvailable(e.App, local, e.Record.GetString("domain"), "", ""); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordAfterCreateSuccess("mail_mailboxes").BindFunc(func(e *core.RecordEvent) error {
		if err := savePrimaryAddress(e.App, e.Record); err != nil {
			addressLog.Error("a new mailbox has no address row", "mailbox", e.Record.Id, "error", err)
		}
		return e.Next()
	})
	app.OnRecordUpdate("mail_mailboxes").BindFunc(func(e *core.RecordEvent) error {
		local := normalizeLocalPart(e.Record.GetString("address"))
		e.Record.Set("address", local)
		original := e.Record.Original()
		if original.GetString("address") == local && original.GetString("domain") == e.Record.GetString("domain") {
			return e.Next()
		}
		primaryID := ""
		if row, err := e.App.FindFirstRecordByFilter("mail_addresses",
			"mailbox = {:mailbox} && is_primary = true", dbx.Params{"mailbox": e.Record.Id}); err == nil {
			primaryID = row.Id
		}
		if err := checkAddressAvailable(e.App, local, e.Record.GetString("domain"), e.Record.Id, primaryID); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess("mail_mailboxes").BindFunc(func(e *core.RecordEvent) error {
		original := e.Record.Original()
		if original.GetString("address") != e.Record.GetString("address") ||
			original.GetString("domain") != e.Record.GetString("domain") {
			if err := savePrimaryAddress(e.App, e.Record); err != nil {
				addressLog.Error("could not move a mailbox's address row", "mailbox", e.Record.Id, "error", err)
			}
		}
		return e.Next()
	})

	// Released clients still write mail_mailbox_aliases. Mirror those writes
	// so their aliases route; an alias uses its mailbox's domain.
	app.OnRecordCreate("mail_mailbox_aliases").BindFunc(func(e *core.RecordEvent) error {
		local := normalizeLocalPart(e.Record.GetString("address"))
		e.Record.Set("address", local)
		mailbox, err := e.App.FindRecordById("mail_mailboxes", e.Record.GetString("mailbox"))
		if err != nil {
			return fmt.Errorf("alias references unknown mailbox: %w", err)
		}
		if err := checkAddressAvailable(e.App, local, mailbox.GetString("domain"), mailbox.Id, ""); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordAfterCreateSuccess("mail_mailbox_aliases").BindFunc(func(e *core.RecordEvent) error {
		if err := mirrorLegacyAlias(e.App, e.Record); err != nil {
			addressLog.Error("a legacy alias has no address row", "alias", e.Record.Id, "error", err)
		}
		return e.Next()
	})
	app.OnRecordAfterDeleteSuccess("mail_mailbox_aliases").BindFunc(func(e *core.RecordEvent) error {
		if err := removeLegacyAlias(e.App, e.Record); err != nil {
			addressLog.Error("a deleted legacy alias still has an address row", "alias", e.Record.Id, "error", err)
		}
		return e.Next()
	})
}

func mirrorLegacyAlias(app core.App, alias *core.Record) error {
	mailbox, err := app.FindRecordById("mail_mailboxes", alias.GetString("mailbox"))
	if err != nil {
		return err
	}
	col, err := app.FindCollectionByNameOrId("mail_addresses")
	if err != nil {
		return err
	}
	row := core.NewRecord(col)
	row.Set("local_part", alias.GetString("address"))
	row.Set("domain", mailbox.GetString("domain"))
	row.Set("mailbox", mailbox.Id)
	row.Set("source", addressSourceManual)
	return app.Save(row)
}

func removeLegacyAlias(app core.App, alias *core.Record) error {
	mailbox, err := app.FindRecordById("mail_mailboxes", alias.GetString("mailbox"))
	if err != nil {
		// The mailbox delete cascaded to both rows already.
		return nil
	}
	row, err := findAddressRow(app, alias.GetString("address"), mailbox.GetString("domain"))
	if err != nil || row == nil {
		return err
	}
	if row.GetString("mailbox") != mailbox.Id || row.GetBool("is_primary") || row.GetString("source") != addressSourceManual {
		return nil
	}
	return app.Delete(row)
}
