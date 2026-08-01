package mail

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

var addressSanitizer = regexp.MustCompile(`[^a-z0-9._-]`)

// handleUserCreated auto-creates a personal mailbox for a new user. Single-org:
// the deployment IS the org, so this fires on the users record itself (the former
// user_org junction is gone) and provisions against the first verified domain.
func handleUserCreated(app core.App, user *core.Record) {
	// A share-link guest is a real users row, but must not be provisioned mail
	// infrastructure: a mailbox plus owner membership would mint a working
	// <username>@<verified-domain> identity that passes verifyMailboxMembership,
	// grants IMAP login, and receives inbound mail — contradicting the
	// guest-exclusion rules (1830000002/1830000003).
	if user.GetString("role") == "guest" {
		return
	}

	// Find the deployment's verified mail domains
	domains, err := app.FindRecordsByFilter(
		"mail_domains",
		"verified = true",
		"created",
		1,
		0,
		nil,
	)
	if err != nil || len(domains) == 0 {
		return
	}
	domain := domains[0]

	// Deliberate: a user we cannot derive an address for (unsanitizable legacy
	// username, or 99 suffix collisions) gets NO mailbox rather than failing
	// account creation — a mailboxless user is recoverable by an admin, a
	// blocked signup is not. Error, not Warn: the user is silently mail-less
	// until someone notices.
	address := deriveMailboxAddress(app, user.GetString("username"), domain.Id)
	if address == "" {
		app.Logger().Error("mail lifecycle: could not derive mailbox address — user has no mailbox",
			"username", user.GetString("username"), "user", user.Id)
		return
	}

	// Create the personal mailbox
	mbCollection, err := app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		app.Logger().Warn("mail lifecycle: mail_mailboxes collection not found", "error", err)
		return
	}

	mailbox := core.NewRecord(mbCollection)
	mailbox.Set("address", address)
	mailbox.Set("domain", domain.Id)
	mailbox.Set("display_name", user.GetString("name"))
	mailbox.Set("type", "personal")
	mailbox.Set("name", user.GetString("name"))
	if err := app.Save(mailbox); err != nil {
		app.Logger().Warn("mail lifecycle: failed to create personal mailbox",
			"address", address, "error", err)
		return
	}

	// Add the user as owner
	memberCollection, err := app.FindCollectionByNameOrId("mail_mailbox_members")
	if err != nil {
		app.Logger().Warn("mail lifecycle: mail_mailbox_members collection not found", "error", err)
		return
	}

	member := core.NewRecord(memberCollection)
	member.Set("mailbox", mailbox.Id)
	member.Set("user", user.Id)
	member.Set("role", "owner")
	if err := app.Save(member); err != nil {
		app.Logger().Warn("mail lifecycle: failed to create mailbox member",
			"mailbox", mailbox.Id, "error", err)
	}
}

// handleUserDeleted cleans up personal mailboxes orphaned by a user deletion.
// The membership rows cascade away with the users record, so a personal mailbox
// left with no members has no owner and is swept here.
func handleUserDeleted(app core.App, _ *core.Record) {
	mailboxes, err := app.FindRecordsByFilter(
		"mail_mailboxes",
		"type = 'personal'",
		"",
		1000,
		0,
		nil,
	)
	if err != nil {
		return
	}

	if len(mailboxes) == 0 {
		return
	}

	// One query for which of them still have members — not one per mailbox.
	// On a query error, sweep nothing: deleting on unknown membership state
	// would destroy a mailbox someone still owns.
	ids := make([]any, len(mailboxes))
	for i, mailbox := range mailboxes {
		ids[i] = mailbox.Id
	}
	var memberRows []struct {
		Mailbox string `db:"mailbox"`
	}
	err = app.DB().
		Select("mailbox").
		Distinct(true).
		From("mail_mailbox_members").
		Where(dbx.In("mailbox", ids...)).
		All(&memberRows)
	if err != nil {
		app.Logger().Warn("mail lifecycle: failed to load mailbox memberships for orphan sweep",
			"error", err)
		return
	}
	hasMembers := make(map[string]bool, len(memberRows))
	for _, row := range memberRows {
		hasMembers[row.Mailbox] = true
	}

	for _, mailbox := range mailboxes {
		if hasMembers[mailbox.Id] {
			continue
		}
		if err := app.Delete(mailbox); err != nil {
			app.Logger().Warn("mail lifecycle: failed to delete orphaned mailbox",
				"mailboxID", mailbox.Id, "error", err)
		}
	}
}

// deriveMailboxAddress generates a unique mailbox address from a user's
// USERNAME — deliberately not their email.
//
// Provisioning from the email took its local-part and dropped the domain, so
// inviting bob@google.com minted bob@<our-verified-domain>: an address the
// invitee never asked for, silently colliding with any real `bob` and implying
// we can host mail for a domain we don't control. The email is a contact
// address for an account that may live anywhere; only the username is ours to
// map into our own domain.
//
// It's also the better key mechanically: usernames are already validated as
// ^[a-z0-9][a-z0-9_-]{0,31}$ (coreserver.IsValidUsername) — lowercase, unique,
// and never empty — whereas email is optional at invite time, which left
// emailless invitees with no mailbox at all. The sanitizer below is kept as
// defence in depth for legacy rows predating that validation.
func deriveMailboxAddress(app core.App, username, domainID string) string {
	base := strings.ToLower(strings.TrimSpace(username))
	base = addressSanitizer.ReplaceAllString(base, "")
	if base == "" {
		return ""
	}

	// Try the base address first, then append numeric suffixes
	candidate := base
	for i := 2; i <= 99; i++ {
		existing, err := app.FindRecordsByFilter(
			"mail_mailboxes",
			"address = {:address} && domain = {:domain}",
			"",
			1,
			0,
			map[string]any{"address": candidate, "domain": domainID},
		)
		if err != nil || len(existing) == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s%d", base, i)
	}

	return ""
}
