package mail

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

var (
	errAddressNotInMailbox = errors.New("the sender address does not belong to this mailbox")
	errNoSendAddress       = errors.New("this mailbox has no address to send from")
)

// resolveSendAddress picks the address a message from mailbox goes out as:
// the explicit address row, else the legacy alias a released client named,
// else the mailbox's default. The From header, DKIM domain and send gate all
// use the returned address's domain, so sending as name@second.example signs
// as second.example.
func resolveSendAddress(app core.App, mailbox *core.Record, fromAddressID, legacyAliasID string) (*resolvedAddress, error) {
	switch {
	case fromAddressID != "":
		from, err := loadResolvedAddress(app, fromAddressID)
		if err != nil {
			return nil, err
		}
		if from.Mailbox.Id != mailbox.Id {
			return nil, errAddressNotInMailbox
		}
		return from, nil
	case legacyAliasID != "":
		alias, err := app.FindRecordById("mail_mailbox_aliases", legacyAliasID)
		if err != nil {
			return nil, fmt.Errorf("%w: alias %s", errAddressNotFound, legacyAliasID)
		}
		if err := verifyAliasBelongsToMailbox(alias, mailbox.Id); err != nil {
			return nil, errAddressNotInMailbox
		}
		row, err := findLegacyAliasRow(app, alias)
		if err != nil {
			return nil, fmt.Errorf("%w: alias %s has no address row", errAddressNotFound, legacyAliasID)
		}
		return loadResolvedAddress(app, row.Id)
	default:
		return defaultSendAddress(app, mailbox)
	}
}

// defaultSendAddress is the address a mailbox sends from when none is chosen:
// its owner's chosen default (mail_identities.default_from), else the address
// on the highest-ranked verified domain, else any address.
func defaultSendAddress(app core.App, mailbox *core.Record) (*resolvedAddress, error) {
	identity, err := app.FindFirstRecordByFilter("mail_identities", "inbox = {:inbox} && retired != true", dbx.Params{"inbox": mailbox.Id})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		addressLog.Error("could not read the inbox identity for its default sender", "mailbox", mailbox.Id, "error", err)
	}
	if err == nil {
		if id := identity.GetString("default_from"); id != "" {
			if from, err := loadResolvedAddress(app, id); err == nil && from.Mailbox.Id == mailbox.Id {
				return from, nil
			}
		}
	}

	rows, err := app.FindRecordsByFilter("mail_addresses", "mailbox = {:mailbox}", "", 0, 0, dbx.Params{"mailbox": mailbox.Id})
	if err != nil {
		return nil, fmt.Errorf("list mailbox addresses: %w", err)
	}
	if len(rows) == 0 {
		return nil, errNoSendAddress
	}
	candidates := make([]*resolvedAddress, 0, len(rows))
	for _, row := range rows {
		domain, err := app.FindRecordById("mail_domains", row.GetString("domain"))
		if err != nil {
			continue
		}
		candidates = append(candidates, &resolvedAddress{Address: row, Mailbox: mailbox, Domain: domain})
	}
	if len(candidates) == 0 {
		return nil, errNoSendAddress
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Domain.GetBool("verified") != b.Domain.GetBool("verified") {
			return a.Domain.GetBool("verified")
		}
		if a.Domain.GetInt("rank") != b.Domain.GetInt("rank") {
			return a.Domain.GetInt("rank") < b.Domain.GetInt("rank")
		}
		if a.Address.GetBool("is_primary") != b.Address.GetBool("is_primary") {
			return a.Address.GetBool("is_primary")
		}
		return a.Address.GetString("created") < b.Address.GetString("created")
	})
	return candidates[0], nil
}

// formatFromAddress builds the From header value. The display name comes from
// the mailbox, never from a client, so every channel sends the same identity.
func formatFromAddress(displayName, email string) string {
	if displayName == "" {
		return "<" + email + ">"
	}
	return displayName + " <" + email + ">"
}
