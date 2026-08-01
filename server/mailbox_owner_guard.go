package mail

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const lastMailboxOwnerMessage = "this is the mailbox's last owner — make another member an owner first"

// hasOtherMailboxOwner reports whether the mailbox has an owner row besides
// excludeRowID.
func hasOtherMailboxOwner(app core.App, mailboxID, excludeRowID string) (bool, error) {
	n, err := app.CountRecords("mail_mailbox_members",
		dbx.HashExp{"mailbox": mailboxID, "role": "owner"},
		dbx.Not(dbx.HashExp{"id": excludeRowID}),
	)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// registerMailboxLastOwnerGuard rejects API writes that would leave a shared
// mailbox with zero owners: demoting the last owner row or deleting it.
//
// The drawer's remove button already excludes the last owner, and the shipped
// help says removing the last owner is blocked — but the role toggle had no
// such check and no server backstop existed for either path. An ownerless
// mailbox is unmanageable: the members createRule's ownerCanAdd branch
// requires an owner row, and its bootstrap branch only fires on a MEMBERLESS
// mailbox, so nobody can ever become owner again.
func registerMailboxLastOwnerGuard(app core.App) {
	app.OnRecordUpdateRequest("mail_mailbox_members").BindFunc(func(e *core.RecordRequestEvent) error {
		original := e.Record.Original()
		if original.GetString("role") != "owner" || e.Record.GetString("role") == "owner" {
			return e.Next()
		}
		// Operators can repair any state from the superuser console —
		// consistent with core's RegisterLastOwnerGuard bypass.
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		other, err := hasOtherMailboxOwner(e.App, original.GetString("mailbox"), e.Record.Id)
		if err != nil {
			return e.InternalServerError("last-owner check failed", err)
		}
		if !other {
			return e.ForbiddenError(lastMailboxOwnerMessage, nil)
		}
		return e.Next()
	})

	app.OnRecordDeleteRequest("mail_mailbox_members").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Record.GetString("role") != "owner" {
			return e.Next()
		}
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		other, err := hasOtherMailboxOwner(e.App, e.Record.GetString("mailbox"), e.Record.Id)
		if err != nil {
			return e.InternalServerError("last-owner check failed", err)
		}
		if !other {
			return e.ForbiddenError(lastMailboxOwnerMessage, nil)
		}
		return e.Next()
	})
}
