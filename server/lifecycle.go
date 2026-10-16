package mail

import (
	"fmt"
	"regexp"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

var addressSanitizer = regexp.MustCompile(`[^a-z0-9._-]`)

// findPersonalMailboxesOf returns the ids of the personal mailboxes that
// belong to the user. A personal mailbox has no user column: it belongs to its
// first owner, the one createPersonalInbox added. An owner can share the mailbox
// and even make someone else an owner too, and that later owner must not take
// the mailbox down with them when they leave, so "the user is an owner" is not
// enough. Ties on created (same millisecond) fall back to insertion order.
func findPersonalMailboxesOf(app core.App, userID string) ([]string, error) {
	var rows []struct {
		Mailbox string `db:"mailbox"`
	}
	err := app.DB().NewQuery(`
		SELECT m.mailbox AS mailbox
		FROM mail_mailbox_members m
		JOIN mail_mailboxes mb ON mb.id = m.mailbox
		WHERE m.user = {:user} AND m.role = 'owner' AND mb.type = 'personal'
			AND NOT EXISTS (SELECT 1 FROM mail_mailbox_members o
				WHERE o.mailbox = m.mailbox AND o.role = 'owner' AND o.user != {:user}
					AND (o.created < m.created OR (o.created = m.created AND o.rowid < m.rowid)))
		ORDER BY m.mailbox`,
	).Bind(dbx.Params{"user": userID}).All(&rows)
	if err != nil {
		return nil, fmt.Errorf("find personal mailboxes: %w", err)
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.Mailbox
	}
	return ids, nil
}

// deletePersonalMailboxesOf deletes the user's personal mailboxes. A personal
// mailbox is the user's own address, so it must not outlive the account, even
// when the user shared it. Deleting the mailbox record cascades to its
// memberships (so the other users lose the share), threads, messages and
// aliases.
func deletePersonalMailboxesOf(app core.App, userID string) error {
	ids, err := findPersonalMailboxesOf(app, userID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		mailbox, err := app.FindRecordById("mail_mailboxes", id)
		if err != nil {
			return fmt.Errorf("load personal mailbox %s: %w", id, err)
		}
		if err := app.Delete(mailbox); err != nil {
			return fmt.Errorf("delete personal mailbox %s: %w", id, err)
		}
	}
	return nil
}

// registerPersonalMailboxCleanup deletes a user's personal mailboxes together
// with the users row. It must run BEFORE the delete: the users cascade removes
// the user's membership rows first, and after that nothing records whose
// mailbox it was. handleUserDeleted cannot do this, because a mailbox the user
// shared still has members and so is not swept. This is a model hook, so it
// covers a REST delete and a superuser or server-side delete alike. The
// transaction makes the mailbox delete and the users delete one unit.
func registerPersonalMailboxCleanup(app core.App) {
	app.OnRecordDelete("users").BindFunc(func(e *core.RecordEvent) error {
		original := e.App
		defer func() { e.App = original }()
		return e.App.RunInTransaction(func(txApp core.App) error {
			if err := deletePersonalMailboxesOf(txApp, e.Record.Id); err != nil {
				return err
			}
			e.App = txApp
			return e.Next()
		})
	})
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
