package mail

import (
	"github.com/pocketbase/pocketbase/core"
)

// registerSentByGuard makes mail_messages.sent_by server-owned.
//
// The collection's create and update rules are both "is a member of this
// message's mailbox" (see the mail collections migration), which is right for
// a shared mailbox — members file, flag and delete each other's mail. It is
// wrong for sent_by: it would let any member rewrite who a message came from,
// and the one thing that field exists to survive is a member who wants to
// blame their sending on somebody else. Attribution the attributed party can
// edit is worse than none, because it reads as evidence.
//
// So sent_by is written only by storeMessage, on the two paths that actually
// hand a message to the provider. These hooks are the ...Request variants,
// which fire for API traffic and not for a server-side app.Save, so that
// write is unaffected while a client's is pinned.
//
// Superusers pass, matching registerMailboxLastOwnerGuard: an operator with
// console access can already rewrite the row directly, so refusing here would
// only block repair work without closing anything.
func registerSentByGuard(app core.App) {
	app.OnRecordCreateRequest("mail_messages").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		// A client-created row did not go through the send path, so there is
		// no sender to name. Empty is the honest value, and the field's
		// contract already reads "" as unknown.
		e.Record.Set("sent_by", "")
		return e.Next()
	})

	app.OnRecordUpdateRequest("mail_messages").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		// Silently restore rather than refuse: a client PATCHing an unrelated
		// field (folder, flags) sends the whole record back, so rejecting any
		// request carrying sent_by would break ordinary mailbox operations.
		e.Record.Set("sent_by", e.Record.Original().GetString("sent_by"))
		return e.Next()
	})
}
