package mail

import "github.com/pocketbase/pocketbase/core"

// threadOwnedFields are the mail_threads columns the server derives from the
// thread's messages. The thread updateRule lets any member write the thread,
// and the after-update hook (thread_state_sync.go) copies latest_date and
// address_keys to every member's state row, so a client write would float a
// thread or change the address views for the whole inbox. The client never
// writes these columns; only subject stays writable.
var threadOwnedFields = []string{
	"address_keys", "latest_date", "has_draft", "has_attachments",
	"message_count", "snippet", "participants", "mailbox",
}

// threadMessageMarkers are the owned columns a new thread starts without: the
// message hooks set them as messages arrive. The other owned columns are
// taken on create, because the seed creates its threads through the API with
// them, and they reach no other member's rows.
var threadMessageMarkers = map[string]any{
	"address_keys": "", "has_draft": false, "has_attachments": false,
}

// registerThreadOwnedFieldsGuard keeps the thread-owned columns server-owned
// on API writes. Restore rather than refuse, as the state row guard does: a
// client may send the whole record back.
func registerThreadOwnedFieldsGuard(app core.App) {
	app.OnRecordCreateRequest("mail_threads").BindFunc(func(e *core.RecordRequestEvent) error {
		for field, empty := range threadMessageMarkers {
			e.Record.Set(field, empty)
		}
		return e.Next()
	})
	app.OnRecordUpdateRequest("mail_threads").BindFunc(func(e *core.RecordRequestEvent) error {
		original := e.Record.Original()
		for _, field := range threadOwnedFields {
			e.Record.Set(field, original.Get(field))
		}
		return e.Next()
	})
}
