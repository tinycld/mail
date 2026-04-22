package mail

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

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

// checkAliasAddressAvailable returns an error if the given address would
// collide with an existing mailbox primary address or an existing alias on
// the same domain. Comparison is case-insensitive.
func checkAliasAddressAvailable(app core.App, domainID, address string) error {
	normalized := strings.ToLower(strings.TrimSpace(address))

	mailboxes, err := app.FindRecordsByFilter(
		"mail_mailboxes",
		"address = {:address} && domain = {:domain}",
		"",
		1,
		0,
		map[string]any{"address": normalized, "domain": domainID},
	)
	if err == nil && len(mailboxes) > 0 {
		return fmt.Errorf("address %s is already used as a mailbox primary address", address)
	}

	aliases, err := app.FindRecordsByFilter(
		"mail_mailbox_aliases",
		"address = {:address} && mailbox.domain = {:domain}",
		"",
		1,
		0,
		map[string]any{"address": normalized, "domain": domainID},
	)
	if err == nil && len(aliases) > 0 {
		return fmt.Errorf("address %s is already used as an alias", address)
	}

	return nil
}

// checkPrimaryAddressAvailable returns an error if the given address would
// collide with an existing alias on the same domain. Aliases that belong to
// excludeMailboxID are skipped so a mailbox can be updated without self-
// collision against its own aliases. Comparison is case-insensitive.
func checkPrimaryAddressAvailable(app core.App, domainID, address, excludeMailboxID string) error {
	normalized := strings.ToLower(strings.TrimSpace(address))

	aliases, err := app.FindRecordsByFilter(
		"mail_mailbox_aliases",
		"address = {:address} && mailbox.domain = {:domain}",
		"",
		100,
		0,
		map[string]any{"address": normalized, "domain": domainID},
	)
	if err != nil {
		return nil
	}

	for _, alias := range aliases {
		if alias.GetString("mailbox") == excludeMailboxID {
			continue
		}
		return fmt.Errorf("address %s is already used as an alias", address)
	}

	return nil
}
