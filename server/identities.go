package mail

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// inboxPolicy says whether ensureIdentity may create a personal inbox for a
// user who owns none.
type inboxPolicy int

const (
	// adoptInboxOnly gives an identity only to a user who already owns a
	// personal inbox. Boot uses it: a user without an inbox there may be a
	// leaver offboarded before identities existed, and an inbox would bring
	// their address back.
	adoptInboxOnly inboxPolicy = iota
	adoptOrCreateInbox
)

// ensureIdentity gives a non-guest user one personal inbox and one identity.
// It adopts the inbox the user already owns (every user created before this
// release has one, unless it was created before any domain was verified or
// has an unusable username) and otherwise creates an inbox on the main domain. It
// returns nil, nil for a guest, for a retired identity (an offboarded user),
// and when no domain is verified yet: the domain-verified hook calls
// ensureIdentities, so the user is picked up then.
func ensureIdentity(app core.App, user *core.Record) (*core.Record, error) {
	return ensureIdentityWith(app, user, adoptOrCreateInbox)
}

// ensureIdentityWith runs in one transaction. Two callers can run it at the
// same time: a new user's create hook and a domain-verified hook
// (ensureIdentities over every user) run on two requests. Each reads what
// exists and then creates what is missing; PocketBase has a single database
// writer, so in a transaction the read and the writes of one caller run
// before the next caller reads. The second caller for the same user finds the
// identity, and a second user finds the email name taken. A caller that holds
// a transaction already joins it. After-success hooks of the saves (the
// mailbox's primary address, the identity's rule addresses) run after commit.
func ensureIdentityWith(app core.App, user *core.Record, policy inboxPolicy) (*core.Record, error) {
	if user.GetString("role") == "guest" {
		return nil, nil
	}
	var identity *core.Record
	err := app.RunInTransaction(func(txApp core.App) error {
		var err error
		identity, err = ensureIdentityIn(txApp, user, policy)
		return err
	})
	if err != nil {
		return nil, err
	}
	return identity, nil
}

// ensureIdentityIn is ensureIdentityWith inside its transaction: app is the
// transaction, and every read and write goes through it.
func ensureIdentityIn(app core.App, user *core.Record, policy inboxPolicy) (*core.Record, error) {
	existing, err := app.FindFirstRecordByFilter("mail_identities", "user = {:user}", dbx.Params{"user": user.Id})
	if err == nil {
		if existing.GetBool("retired") {
			return nil, nil
		}
		if existing.GetString("inbox") == "" {
			return repairIdentityInbox(app, user, existing)
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("identity lookup: %w", err)
	}

	inbox, err := adoptPersonalInbox(app, user.Id)
	if err != nil {
		return nil, err
	}
	adopted := inbox != nil
	var emailName string
	switch {
	case inbox != nil:
		// Legacy inbox addresses were never pattern-checked.
		emailName = normalizeLocalPart(inbox.GetString("address"))
		taken := true
		if localPartPattern.MatchString(emailName) {
			taken, err = emailNameTaken(app, emailName, inbox.Id)
			if err != nil {
				return nil, err
			}
		}
		if taken {
			emailName = deriveEmailName(app, user.GetString("username"), inbox.GetString("domain"))
		}
	case policy == adoptInboxOnly:
		return nil, nil
	default:
		domain, err := mainDomain(app)
		if err != nil || domain == nil {
			return nil, err
		}
		emailName = deriveEmailName(app, user.GetString("username"), domain.Id)
		if emailName != "" {
			inbox, err = createPersonalInbox(app, user, emailName, domain.Id)
			if err != nil {
				return nil, err
			}
		}
	}
	// Deliberate: a user we cannot name gets no identity rather than a failed
	// account creation; an admin can fix the username. Error, not Warn: the
	// user is silently without mail until someone notices.
	if emailName == "" {
		addressLog.Error("could not choose an email name; the user has no mail identity",
			"user", user.Id, "username", user.GetString("username"))
		return nil, nil
	}

	col, err := app.FindCollectionByNameOrId("mail_identities")
	if err != nil {
		return nil, err
	}
	identity := core.NewRecord(col)
	identity.Set("user", user.Id)
	identity.Set("inbox", inbox.Id)
	identity.Set("email_name", emailName)
	if adopted {
		// A legacy inbox sent from its own address. Keep that as the default
		// From: the domain rules may now rank another domain first.
		primary, err := app.FindFirstRecordByFilter("mail_addresses",
			"mailbox = {:inbox} && is_primary = true", dbx.Params{"inbox": inbox.Id})
		if err == nil {
			identity.Set("default_from", primary.Id)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("primary address lookup: %w", err)
		}
	}
	if err := app.Save(identity); err != nil {
		return nil, fmt.Errorf("save identity: %w", err)
	}
	return identity, nil
}

// repairIdentityInbox gives a live identity whose inbox was deleted outside
// offboarding (a removed domain, the owner deleting it, a superuser) a new
// one: an inbox the user owns that no identity claims, else a new one on the
// main domain. It runs under both policies: a live identity is not a leaver.
// With no verified domain the identity is returned unchanged; the
// domain-verified hook retries.
func repairIdentityInbox(app core.App, user, identity *core.Record) (*core.Record, error) {
	inbox, err := adoptPersonalInbox(app, user.Id)
	if err != nil {
		return nil, err
	}
	if inbox == nil {
		domain, err := mainDomain(app)
		if err != nil || domain == nil {
			return identity, err
		}
		emailName, err := repairEmailName(app, user, identity, domain.Id)
		if err != nil {
			return nil, err
		}
		if emailName == "" {
			addressLog.Error("could not choose an email name; the user has no inbox",
				"user", user.Id, "username", user.GetString("username"))
			return identity, nil
		}
		inbox, err = createPersonalInbox(app, user, emailName, domain.Id)
		if err != nil {
			return nil, err
		}
		identity.Set("email_name", emailName)
	}
	identity.Set("inbox", inbox.Id)
	if err := app.Save(identity); err != nil {
		return nil, fmt.Errorf("save repaired identity: %w", err)
	}
	return identity, nil
}

// repairEmailName keeps the identity's email name when it is still free on the
// main domain, and otherwise derives a new one.
func repairEmailName(app core.App, user, identity *core.Record, domainID string) (string, error) {
	name := identity.GetString("email_name")
	if localPartPattern.MatchString(name) {
		others, err := app.FindRecordsByFilter("mail_identities",
			"email_name = {:name} && retired != true && id != {:id}", "", 1, 0,
			dbx.Params{"name": name, "id": identity.Id})
		if err != nil {
			return "", err
		}
		row, err := findAddressRow(app, name, domainID)
		if err != nil {
			return "", err
		}
		if len(others) == 0 && row == nil {
			return name, nil
		}
	}
	return deriveEmailName(app, user.GetString("username"), domainID), nil
}

// repairEmptyInboxIdentities runs the repair for every live identity without
// an inbox. Unlike ensureIdentities it never gives a user without an
// identity one.
func repairEmptyInboxIdentities(app core.App) error {
	identities, err := app.FindRecordsByFilter("mail_identities",
		"retired != true && inbox = '' && user.role != 'guest'", "created", 0, 0)
	if err != nil {
		return fmt.Errorf("list identities without an inbox: %w", err)
	}
	var errs []error
	for _, identity := range identities {
		user, err := app.FindRecordById("users", identity.GetString("user"))
		if err == nil {
			_, err = ensureIdentityWith(app, user, adoptOrCreateInbox)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("identity %s: %w", identity.Id, err))
		}
	}
	return errors.Join(errs...)
}

// ensureIdentities runs ensureIdentity for every non-guest user under the
// given inbox policy. One user's failure does not stop the others; the errors
// are joined.
func ensureIdentities(app core.App, policy inboxPolicy) error {
	users, err := app.FindRecordsByFilter("users", "role != 'guest'", "created", 0, 0)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	var errs []error
	for _, user := range users {
		if _, err := ensureIdentityWith(app, user, policy); err != nil {
			errs = append(errs, fmt.Errorf("user %s: %w", user.Id, err))
		}
	}
	return errors.Join(errs...)
}

// adoptPersonalInbox returns the personal inbox the user already owns and that
// no identity claims yet, or nil.
func adoptPersonalInbox(app core.App, userID string) (*core.Record, error) {
	ids, err := findPersonalMailboxesOf(app, userID)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		_, err := app.FindFirstRecordByFilter("mail_identities", "inbox = {:inbox}", dbx.Params{"inbox": id})
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("identity lookup for inbox %s: %w", id, err)
		}
		return app.FindRecordById("mail_mailboxes", id)
	}
	return nil, nil
}

// mainDomain is the verified domain new inboxes are created on: the first by
// the admin's order (rank), then the oldest.
func mainDomain(app core.App) (*core.Record, error) {
	rows, err := app.FindRecordsByFilter("mail_domains", "verified = true", "rank,created", 1, 0)
	if err != nil {
		return nil, fmt.Errorf("find main domain: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// emailNameTaken reports whether name cannot be a new identity's email name:
// another identity has it, or it is already an address of a different inbox
// on a domain with a rule (where the name would be handed out).
func emailNameTaken(app core.App, name, inboxID string) (bool, error) {
	if _, err := app.FindFirstRecordByFilter("mail_identities", "email_name = {:name} && retired != true", dbx.Params{"name": name}); err == nil {
		return true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	rows, err := app.FindRecordsByFilter("mail_addresses",
		"local_part = {:name} && "+ruleDomainRelFilter+" && mailbox != {:inbox}", "", 1, 0,
		dbx.Params{"name": name, "inbox": inboxID})
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// deriveEmailName turns a username into a free email name: the sanitized
// username, then name2 … name99. Free means emailNameTaken is false and the
// name is not already an address on the inbox's own domain.
func deriveEmailName(app core.App, username, inboxDomainID string) string {
	base := addressSanitizer.ReplaceAllString(strings.ToLower(strings.TrimSpace(username)), "")
	if base == "" {
		return ""
	}
	for i := 1; i <= 99; i++ {
		suffix := ""
		if i > 1 {
			suffix = fmt.Sprint(i)
		}
		trimmed := base
		if max := 64 - len(suffix); len(trimmed) > max {
			trimmed = trimmed[:max]
		}
		candidate := trimmed + suffix
		if !localPartPattern.MatchString(candidate) {
			continue
		}
		taken, err := emailNameTaken(app, candidate, "")
		if err != nil {
			addressLog.Error("could not check an email name", "name", candidate, "error", err)
			return ""
		}
		if taken {
			continue
		}
		row, err := findAddressRow(app, candidate, inboxDomainID)
		if err != nil {
			addressLog.Error("could not check an address", "name", candidate, "error", err)
			return ""
		}
		if row == nil {
			return candidate
		}
	}
	return ""
}

// createPersonalInbox creates the user's inbox and their owner membership in
// one transaction. The mailbox hooks give it its primary address row.
func createPersonalInbox(app core.App, user *core.Record, emailName, domainID string) (*core.Record, error) {
	var inbox *core.Record
	err := app.RunInTransaction(func(txApp core.App) error {
		mailboxes, err := txApp.FindCollectionByNameOrId("mail_mailboxes")
		if err != nil {
			return err
		}
		inbox = core.NewRecord(mailboxes)
		inbox.Set("address", emailName)
		inbox.Set("domain", domainID)
		inbox.Set("display_name", user.GetString("name"))
		inbox.Set("name", user.GetString("name"))
		inbox.Set("type", "personal")
		if err := txApp.Save(inbox); err != nil {
			return fmt.Errorf("create personal inbox: %w", err)
		}
		members, err := txApp.FindCollectionByNameOrId("mail_mailbox_members")
		if err != nil {
			return err
		}
		member := core.NewRecord(members)
		member.Set("mailbox", inbox.Id)
		member.Set("user", user.Id)
		member.Set("role", "owner")
		return txApp.Save(member)
	})
	if err != nil {
		return nil, err
	}
	return inbox, nil
}

// retireIdentity keeps an offboarded user's identity as a marker that they
// had mail, so no hook or boot repair gives the anonymized account a new
// inbox. It drops the inbox (about to be deleted) and frees the email name
// for a new person: the name has a unique index, so it is replaced with a
// tombstone that cannot be anyone's username-derived name.
func retireIdentity(app core.App, userID string) error {
	identity, err := app.FindFirstRecordByFilter("mail_identities", "user = {:user}", dbx.Params{"user": userID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("identity lookup: %w", err)
	}
	identity.Set("retired", true)
	identity.Set("inbox", "")
	identity.Set("default_from", "")
	identity.Set("email_name", "retired-"+identity.Id)
	if err := app.Save(identity); err != nil {
		return fmt.Errorf("retire identity: %w", err)
	}
	return nil
}

// registerIdentityHooks gives every new non-guest user, and every guest
// promoted to member, an identity.
func registerIdentityHooks(app core.App) {
	app.OnRecordAfterCreateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		if _, err := ensureIdentity(e.App, e.Record); err != nil {
			addressLog.Error("could not give a new user a mail identity", "user", e.Record.Id, "error", err)
		}
		return e.Next()
	})
	// Only a promotion: any other update (offboarding's anonymize save among
	// them) must not create an inbox for a user who has none.
	app.OnRecordAfterUpdateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		wasGuest := e.Record.Original().GetString("role") == "guest"
		if wasGuest && e.Record.GetString("role") != "guest" {
			if _, err := ensureIdentity(e.App, e.Record); err != nil {
				addressLog.Error("could not give a promoted guest a mail identity", "user", e.Record.Id, "error", err)
			}
		}
		return e.Next()
	})
}
