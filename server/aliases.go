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

// checkAliasAddressAvailable returns an error if `address` collides with an
// existing primary mailbox or another alias on the same domain. Comparisons
// are case-insensitive.
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
	if err != nil {
		return fmt.Errorf("mailbox lookup failed: %w", err)
	}
	if len(mailboxes) > 0 {
		return fmt.Errorf("address %s is already a primary mailbox on this domain", normalized)
	}

	aliases, err := app.FindRecordsByFilter(
		"mail_mailbox_aliases",
		"address = {:address} && mailbox.domain = {:domain}",
		"",
		1,
		0,
		map[string]any{"address": normalized, "domain": domainID},
	)
	if err != nil {
		return fmt.Errorf("alias lookup failed: %w", err)
	}
	if len(aliases) > 0 {
		return fmt.Errorf("address %s is already an alias on this domain", normalized)
	}

	return nil
}

// checkPrimaryAddressAvailable returns an error if `address` is already taken
// by an alias on the same domain, excluding aliases owned by excludeMailboxID
// (so a mailbox can update without self-colliding). Case-insensitive.
func checkPrimaryAddressAvailable(app core.App, domainID, address, excludeMailboxID string) error {
	normalized := strings.ToLower(strings.TrimSpace(address))

	// limit 2: at most one "self" alias + any colliding alias from another mailbox
	aliases, err := app.FindRecordsByFilter(
		"mail_mailbox_aliases",
		"address = {:address} && mailbox.domain = {:domain}",
		"",
		2,
		0,
		map[string]any{"address": normalized, "domain": domainID},
	)
	if err != nil {
		return fmt.Errorf("alias lookup failed: %w", err)
	}
	for _, a := range aliases {
		if excludeMailboxID != "" && a.GetString("mailbox") == excludeMailboxID {
			continue
		}
		return fmt.Errorf("address %s is already an alias on this domain", normalized)
	}
	return nil
}
