package mail

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// draftEvent builds a RequestEvent for calling handleDraft directly with a
// JSON body and an authenticated user.
func draftEvent(t *testing.T, app core.App, user *core.Record, body any) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal draft body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/mail/draft", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = user
	return re, rec
}

// recipientsBcc decodes the recipients_bcc JSON column of a stored message.
func recipientsBcc(t *testing.T, app core.App, messageID string) []Recipient {
	t.Helper()
	record, err := app.FindRecordById("mail_messages", messageID)
	if err != nil {
		t.Fatalf("draft record not found: %v", err)
	}
	raw := record.GetString("recipients_bcc")
	if raw == "" {
		t.Fatalf("recipients_bcc is empty — the column write was dropped")
	}
	var got []Recipient
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("recipients_bcc is not valid JSON (%q): %v", raw, err)
	}
	return got
}

// Draft BCC used to be silently lost: the create path never wrote it and no
// migration defined the column, so PocketBase dropped the update path's write
// too. Both paths must round-trip it now.
func TestHandleDraft_BccRoundTripsOnCreateAndUpdate(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_draftbcc_01")
	user := searchUser(t, app, "alice@acme.com")
	seedMember(t, app, "mb_draftbcc_01", user.Id)

	re, rec := draftEvent(t, app, user, map[string]any{
		"mailbox_id": padID("mb_draftbcc_01"),
		"to":         []Recipient{{Name: "Bob", Email: "bob@example.org"}},
		"bcc":        []Recipient{{Name: "Carol", Email: "carol@example.org"}},
		"subject":    "bcc round trip",
		"text_body":  "hello",
	})
	if err := handleDraft(app, re); err != nil {
		t.Fatalf("create draft failed: %v (body=%s)", err, rec.Body.String())
	}

	var created struct {
		MessageID string `json:"message_id"`
		ThreadID  string `json:"thread_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	got := recipientsBcc(t, app, created.MessageID)
	if len(got) != 1 || got[0].Email != "carol@example.org" {
		t.Fatalf("created draft recipients_bcc = %+v, want carol@example.org", got)
	}

	// Update the same draft with a different BCC list.
	re2, rec2 := draftEvent(t, app, user, map[string]any{
		"mailbox_id": padID("mb_draftbcc_01"),
		"message_id": created.MessageID,
		"to":         []Recipient{{Name: "Bob", Email: "bob@example.org"}},
		"bcc": []Recipient{
			{Name: "Carol", Email: "carol@example.org"},
			{Name: "Dave", Email: "dave@example.org"},
		},
		"subject":   "bcc round trip",
		"text_body": "hello again",
	})
	if err := handleDraft(app, re2); err != nil {
		t.Fatalf("update draft failed: %v (body=%s)", err, rec2.Body.String())
	}

	got = recipientsBcc(t, app, created.MessageID)
	if len(got) != 2 || got[1].Email != "dave@example.org" {
		t.Fatalf("updated draft recipients_bcc = %+v, want carol + dave", got)
	}
}

// The send path shares storeMessage with the draft create path; a sent
// message's own stored copy must record who was BCC'd (delivered headers are
// the provider's concern and never include it).
func TestStoreMessage_PersistsBcc(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_storebcc_01")

	thread, err := findOrCreateThread(app, padID("mb_storebcc_01"), "stored bcc", "", "")
	if err != nil {
		t.Fatalf("findOrCreateThread: %v", err)
	}

	record, err := storeMessage(app, thread.Id, &storedMessage{
		MessageID:      "msg-store-bcc",
		SenderName:     "Alice",
		SenderEmail:    "alice@acme.com",
		To:             []Recipient{{Email: "bob@example.org"}},
		Bcc:            []Recipient{{Email: "carol@example.org"}},
		Date:           "2026-08-06T00:00:00Z",
		Subject:        "stored bcc",
		TextBody:       "hi",
		DeliveryStatus: "sent",
	})
	if err != nil {
		t.Fatalf("storeMessage: %v", err)
	}

	got := recipientsBcc(t, app, record.Id)
	if len(got) != 1 || got[0].Email != "carol@example.org" {
		t.Fatalf("recipients_bcc = %+v, want carol@example.org", got)
	}
	if !strings.Contains(record.GetString("recipients_to"), "bob@example.org") {
		t.Fatalf("recipients_to lost its value: %q", record.GetString("recipients_to"))
	}
}
