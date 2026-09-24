package mail

import (
	"github.com/pocketbase/pocketbase/core"
)

// registerMailboxCreatorGuard makes mail_mailboxes.created_by server-owned.
//
// The mail_mailbox_members bootstrap branch lets a caller add themselves as
// the first owner of a memberless mailbox only when they are its created_by
// (see 1830000007). That check is worth nothing if a client can choose the
// value: an owner could name any user as creator and hand them the right to
// claim the mailbox once it empties. So an API create always records the
// caller, and an API update always keeps the stored value.
//
// This is a hook, not a create rule that requires created_by in the body,
// because released clients do not send the field; a rule would break
// shared-mailbox creation for them. The ...Request hooks do not fire for a
// server-side app.Save, so handleUserCreated is unaffected.
//
// Superusers pass, matching registerSentByGuard: an operator with console
// access can already rewrite the row directly.
func registerMailboxCreatorGuard(app core.App) {
	app.OnRecordCreateRequest("mail_mailboxes").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		creator := ""
		if e.Auth != nil {
			creator = e.Auth.Id
		}
		e.Record.Set("created_by", creator)
		return e.Next()
	})

	app.OnRecordUpdateRequest("mail_mailboxes").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		// Restore rather than refuse: a client that saves an edited mailbox
		// can send the whole record back, created_by included.
		e.Record.Set("created_by", e.Record.Original().GetString("created_by"))
		return e.Next()
	})
}
