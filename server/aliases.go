package mail

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// resolveSenderAddressRecords returns the local part of the From address,
// preferring the alias address when present.
func resolveSenderAddressRecords(mailbox, alias *core.Record) string {
	if alias != nil {
		return alias.GetString("address")
	}
	return mailbox.GetString("address")
}

// buildFromAddress formats an outgoing From header using the alias address
// when supplied, else the mailbox's primary address. Display name always
// comes from the mailbox record.
func buildFromAddress(mailbox, domain, alias *core.Record) string {
	address := resolveSenderAddressRecords(mailbox, alias)
	displayName := mailbox.GetString("display_name")
	domainName := domain.GetString("domain")
	if displayName == "" {
		return fmt.Sprintf("<%s@%s>", address, domainName)
	}
	return fmt.Sprintf("%s <%s@%s>", displayName, address, domainName)
}

// verifyAliasBelongsToMailbox returns an error if alias is non-nil and does
// not belong to mailboxID. Nil alias is OK.
func verifyAliasBelongsToMailbox(alias *core.Record, mailboxID string) error {
	if alias == nil {
		return nil
	}
	if alias.GetString("mailbox") != mailboxID {
		return fmt.Errorf("alias %s does not belong to mailbox %s", alias.Id, mailboxID)
	}
	return nil
}

// findMailboxViaAlias looks up a mailbox by searching mail_mailbox_aliases
// for an address belonging to the given domain. Returns the mailbox record
// and the matching alias record. Comparison is case-insensitive.
func findMailboxViaAlias(app core.App, domainID, localPart string) (*core.Record, *core.Record, error) {
	normalized := strings.ToLower(strings.TrimSpace(localPart))

	aliases, err := app.FindRecordsByFilter(
		"mail_mailbox_aliases",
		"address = {:address} && mailbox.domain = {:domain}",
		"",
		1,
		0,
		map[string]any{"address": normalized, "domain": domainID},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("alias lookup failed: %w", err)
	}
	if len(aliases) == 0 {
		return nil, nil, fmt.Errorf("alias %s not found on domain %s", localPart, domainID)
	}

	alias := aliases[0]
	mailbox, err := app.FindRecordById("mail_mailboxes", alias.GetString("mailbox"))
	if err != nil {
		return nil, nil, fmt.Errorf("alias %s references unknown mailbox: %w", localPart, err)
	}

	return mailbox, alias, nil
}
