package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// A message written straight through PocketBase (the Takeout importer posts
// to the records API) skipped storeMessage, so it never got an IMAP UID, and
// IMAP lists only messages that have one: 5,587 imported messages were
// invisible to mail clients.
func TestMessageUID_AssignedToAMessageWrittenOutsideStore(t *testing.T) {
	app := newRealSchemaApp(t)
	registerMessageUIDHook(app)
	mailbox := seedRealMailbox(t, app)
	thread, err := findOrCreateThread(app, mailbox.Id, "Imported", "", "")
	if err != nil {
		t.Fatal(err)
	}
	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(messages)
	record.Set("thread", thread.Id)
	record.Set("subject", "Imported")
	record.Set("sender_email", "old@example.org")
	record.Set("date", "2009-01-01T00:00:00Z")
	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}

	stored, err := app.FindRecordById("mail_messages", record.Id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetInt("imap_uid") <= 0 {
		t.Fatalf("imap_uid = %d, want a UID so IMAP lists the message", stored.GetInt("imap_uid"))
	}
}

// storeMessage assigns its own UID; the hook must not assign a second one.
func TestMessageUID_StoreMessageGetsOneUID(t *testing.T) {
	app := newRealSchemaApp(t)
	registerMessageUIDHook(app)
	mailbox := seedRealMailbox(t, app)
	thread, err := findOrCreateThread(app, mailbox.Id, "Live", "", "")
	if err != nil {
		t.Fatal(err)
	}
	record, err := storeMessage(app, thread.Id, &storedMessage{
		MessageID: "<live@example.org>", SenderEmail: "a@example.org",
		Subject: "Live", Date: "2026-10-28T19:45:00Z", DeliveryStatus: "delivered",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := record.GetInt("imap_uid"); got != 1 {
		t.Fatalf("imap_uid = %d, want 1 (one allocation)", got)
	}
	state, err := findMailboxState(app, mailbox.Id)
	if err != nil || state == nil {
		t.Fatalf("mailbox state: %v", err)
	}
	if next := state.GetInt("uid_next"); next != 2 {
		t.Fatalf("uid_next = %d, want 2 (one allocation)", next)
	}
}
