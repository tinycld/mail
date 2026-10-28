package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// PocketBase checks a file field's mimeTypes by sniffing the file's first
// bytes. The real schema must store content that sniffs as another type.

func storeSniffTestMessage(t *testing.T, app core.App, htmlBody string) (*core.Record, error) {
	t.Helper()
	mailbox := seedRealMailbox(t, app)
	thread, err := findOrCreateThread(app, mailbox.Id, "Hi", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return storeMessage(app, thread.Id, &storedMessage{
		MessageID:      "<sniff@example.org>",
		SenderEmail:    "a@example.org",
		Subject:        "Hi",
		Date:           "2026-10-28T19:45:00Z",
		HTMLBody:       htmlBody,
		DeliveryStatus: "delivered",
	})
}

// Valid HTML that starts with text sniffs as text/plain.
func TestMessageFiles_StoresBodyThatStartsWithText(t *testing.T) {
	app := newRealSchemaApp(t)
	record, err := storeSniffTestMessage(t, app, `Thanks!<div><br></div><blockquote>earlier</blockquote>`)
	if err != nil {
		t.Fatalf("store a body that starts with text: %v", err)
	}
	if record.GetString("body_html") == "" {
		t.Fatal("body_html is empty")
	}
}

// Headers that start with Received: sniff as message/rfc822.
func TestMessageFiles_StoresHeadersThatStartWithReceived(t *testing.T) {
	app := newRealSchemaApp(t)
	record, err := storeSniffTestMessage(t, app, "")
	if err != nil {
		t.Fatal(err)
	}
	storeRawHeaders(app, record, []byte("Received: from mx.example.org\r\nFrom: a@example.org"))
	stored, err := app.FindRecordById("mail_messages", record.Id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetString("raw_headers") == "" {
		t.Fatal("raw_headers is empty")
	}
}
