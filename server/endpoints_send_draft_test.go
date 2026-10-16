package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"tinycld.org/core/syscfg"
	"tinycld.org/packages/mail/api"
)

// draftSendEnv is a mailbox with a sending member, a provider that counts as
// configured, and a demo user so no send leaves the test.
type draftSendEnv struct {
	*sendAddressEnv
	user *core.Record
	from *core.Record
}

func setupDraftSendEnv(t *testing.T) *draftSendEnv {
	t.Helper()
	env := setupSendAddressEnv(t)
	syscfg.SetResolver(func(key string) string {
		if key == "mail.postmark_server_token" {
			return "test-token"
		}
		return ""
	})
	t.Cleanup(func() { syscfg.SetResolver(func(string) string { return "" }) })
	env.com.Set("spf_verified", true)
	env.com.Set("dkim_verified", true)
	env.com.Set("return_path_verified", true)
	if err := env.app.Save(env.com); err != nil {
		t.Fatal(err)
	}
	user := saveRealRecord(t, env.app, "users", map[string]any{
		"email": "n@ext.example", "username": "nathan", "name": "Nathan", "role": "owner", "is_demo": true,
	})
	saveRealRecord(t, env.app, "mail_mailbox_members", map[string]any{"mailbox": env.inbox.Id, "user": user.Id, "role": "owner"})
	from, err := findAddressRow(env.app, "nathan", env.com.Id)
	if err != nil {
		t.Fatal(err)
	}
	return &draftSendEnv{sendAddressEnv: env, user: user, from: from}
}

// seedDraft stores a message in mailbox with the given delivery status and
// saved files (name → content), the way the draft endpoint stores them.
func seedDraft(t *testing.T, app core.App, mailboxID, status string, files map[string]string) *core.Record {
	t.Helper()
	thread, err := findOrCreateThread(app, mailboxID, "draft subject", "", "")
	if err != nil {
		t.Fatal(err)
	}
	record, err := storeMessage(app, thread.Id, &storedMessage{
		MessageID:      "draft-seed-" + status + "-" + thread.Id,
		SenderEmail:    "nathan@tinycld.com",
		Subject:        "draft subject",
		TextBody:       "draft body",
		Date:           time.Now().UTC().Format(time.RFC3339),
		DeliveryStatus: status,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		f, err := filesystem.NewFileFromBytes([]byte(content), name)
		if err != nil {
			t.Fatal(err)
		}
		record.Set("attachments+", f)
	}
	if len(files) > 0 {
		record.Set("has_attachments", true)
		if err := app.Save(record); err != nil {
			t.Fatal(err)
		}
	}
	return record
}

// sendEvent builds a RequestEvent for calling handleSend directly with a JSON
// body and an authenticated user.
func sendEvent(t *testing.T, app core.App, user *core.Record, body api.SendEmailRequest) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/mail/send", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = user
	return re, rec
}

// storedFiles reads every attachment of a message, keyed by its name with
// the storage suffix removed.
func storedFiles(t *testing.T, app core.App, record *core.Record) map[string]string {
	t.Helper()
	fsys, err := app.NewFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	out := map[string]string{}
	for _, name := range record.GetStringSlice("attachments") {
		blob, err := fsys.GetReader(record.BaseFilesPath() + "/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		data, err := io.ReadAll(blob)
		blob.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[originalAttachmentName(name)] = string(data)
	}
	return out
}

// sentMessageCount counts the messages that are not drafts: a refused send
// must not store one.
func sentMessageCount(t *testing.T, app core.App) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_messages", "delivery_status != 'draft'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestHandleSend_DraftAttachmentsAreSentAndTheDraftIsDeleted(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{
		"notes.txt": "draft notes", "photo.jpg": "not really a jpeg",
	})

	re, rec := sendEvent(t, env.app, env.user, api.SendEmailRequest{
		MailboxID:     env.inbox.Id,
		FromAddressID: env.from.Id,
		To:            []api.Recipient{{Email: "friend@example.net"}},
		Subject:       "hello",
		TextBody:      "hi",
		DraftID:       draft.Id,
	})
	if err := handleSend(env.app, re); err != nil {
		t.Fatal(err)
	}
	var resp api.SendEmailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	sent, err := env.app.FindRecordById("mail_messages", resp.MessageID)
	if err != nil {
		t.Fatal(err)
	}
	got := storedFiles(t, env.app, sent)
	if got["notes.txt"] != "draft notes" || got["photo.jpg"] != "not really a jpeg" || len(got) != 2 {
		t.Fatalf("sent attachments = %v, want the draft's notes.txt and photo.jpg", got)
	}
	if !sent.GetBool("has_attachments") {
		t.Fatal("has_attachments = false on the sent message")
	}
	if _, err := env.app.FindRecordById("mail_messages", draft.Id); err == nil {
		t.Fatal("the draft still exists after a successful send")
	}
}

func TestSendMessage_DraftAttachmentTheSenderRemovedIsLeftOut(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{
		"keep.txt": "keep", "drop.txt": "drop",
	})
	var dropName string
	for _, name := range draft.GetStringSlice("attachments") {
		if originalAttachmentName(name) == "drop.txt" {
			dropName = name
		}
	}

	result, err := sendMessage(env.app, sendParams{
		Ctx:                    context.Background(),
		UserID:                 env.user.Id,
		MailboxID:              env.inbox.Id,
		FromAddressID:          env.from.Id,
		To:                     []api.Recipient{{Email: "friend@example.net"}},
		Subject:                "hello",
		TextBody:               "hi",
		DraftID:                draft.Id,
		RemoveDraftAttachments: []string{dropName},
		Attachments: []Attachment{{
			Name: "new.txt", ContentType: "text/plain", Content: "bmV3", // "new"
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sent, err := env.app.FindRecordById("mail_messages", result.MessageRecordID)
	if err != nil {
		t.Fatal(err)
	}
	got := storedFiles(t, env.app, sent)
	if len(got) != 2 || got["keep.txt"] != "keep" || got["new.txt"] != "new" {
		t.Fatalf("sent attachments = %v, want keep.txt and new.txt only", got)
	}
}

func TestSendMessage_RefusesADraftItCannotResume(t *testing.T) {
	env := setupDraftSendEnv(t)

	// Another person's personal mailbox, with a draft in it.
	other := saveRealRecord(t, env.app, "users", map[string]any{
		"email": "o@ext.example", "username": "other", "name": "Other", "role": "member",
	})
	otherInbox := saveRealRecord(t, env.app, "mail_mailboxes", map[string]any{
		"address": "other", "domain": env.org.Id, "type": "personal", "display_name": "Other", "name": "Other",
	})
	saveRealRecord(t, env.app, "mail_mailbox_members", map[string]any{"mailbox": otherInbox.Id, "user": other.Id, "role": "owner"})
	otherDraft := seedDraft(t, env.app, otherInbox.Id, "draft", map[string]string{"secret.txt": "secret"})
	otherFrom, err := findAddressRow(env.app, "other", env.org.Id)
	if err != nil {
		t.Fatal(err)
	}

	notADraft := seedDraft(t, env.app, env.inbox.Id, "sent", map[string]string{"old.txt": "old"})

	cases := []struct {
		name      string
		mailboxID string
		fromID    string
		draftID   string
	}{
		{"a draft in another mailbox", env.inbox.Id, env.from.Id, otherDraft.Id},
		{"another person's draft, sent from their mailbox", otherInbox.Id, otherFrom.Id, otherDraft.Id},
		{"a message that is not a draft", env.inbox.Id, env.from.Id, notADraft.Id},
		{"a draft that does not exist", env.inbox.Id, env.from.Id, "nosuchdraft0000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := sentMessageCount(t, env.app)
			_, err := sendMessage(env.app, sendParams{
				Ctx:           context.Background(),
				UserID:        env.user.Id,
				MailboxID:     tc.mailboxID,
				FromAddressID: tc.fromID,
				To:            []api.Recipient{{Email: "friend@example.net"}},
				Subject:       "hello",
				TextBody:      "hi",
				DraftID:       tc.draftID,
			})
			if err == nil {
				t.Fatal("the send was accepted")
			}
			if after := sentMessageCount(t, env.app); after != before {
				t.Fatalf("a refused send stored a message (%d → %d)", before, after)
			}
		})
	}
	if _, err := env.app.FindRecordById("mail_messages", otherDraft.Id); err != nil {
		t.Fatalf("the other person's draft was deleted: %v", err)
	}
}

func TestSendMessage_AnUnreadableDraftFileFailsTheSendAndKeepsTheDraft(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{"report.pdf": "%PDF-1.4 fake"})

	fsys, err := env.app.NewFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	stored := draft.GetStringSlice("attachments")[0]
	if err := fsys.Delete(draft.BaseFilesPath() + "/" + stored); err != nil {
		t.Fatal(err)
	}
	fsys.Close()

	before := sentMessageCount(t, env.app)
	_, err = sendMessage(env.app, sendParams{
		Ctx:           context.Background(),
		UserID:        env.user.Id,
		MailboxID:     env.inbox.Id,
		FromAddressID: env.from.Id,
		To:            []api.Recipient{{Email: "friend@example.net"}},
		Subject:       "hello",
		TextBody:      "hi",
		DraftID:       draft.Id,
	})
	if err == nil {
		t.Fatal("the send succeeded without the draft's file")
	}
	if !strings.Contains(err.Error(), "report.pdf") {
		t.Fatalf("error %q does not name the file", err)
	}
	if after := sentMessageCount(t, env.app); after != before {
		t.Fatalf("a failed send stored a message (%d → %d)", before, after)
	}
	if _, err := env.app.FindRecordById("mail_messages", draft.Id); err != nil {
		t.Fatalf("the draft was deleted after a failed send: %v", err)
	}
}

func TestSendMessage_WithoutADraftIDLeavesDraftsAlone(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{"notes.txt": "draft notes"})

	result, err := sendMessage(env.app, sendParams{
		Ctx:           context.Background(),
		UserID:        env.user.Id,
		MailboxID:     env.inbox.Id,
		FromAddressID: env.from.Id,
		To:            []api.Recipient{{Email: "friend@example.net"}},
		Subject:       "hello",
		TextBody:      "hi",
		Attachments:   []Attachment{{Name: "new.txt", ContentType: "text/plain", Content: "bmV3"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sent, err := env.app.FindRecordById("mail_messages", result.MessageRecordID)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedFiles(t, env.app, sent); len(got) != 1 || got["new.txt"] != "new" {
		t.Fatalf("sent attachments = %v, want only new.txt", got)
	}
	if _, err := env.app.FindRecordById("mail_messages", draft.Id); err != nil {
		t.Fatalf("an unrelated draft was deleted: %v", err)
	}
}

func TestHandleDraft_RemoveAttachmentsDropsThemFromTheDraft(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{
		"keep.txt": "keep", "drop.txt": "drop",
	})
	var dropName string
	for _, name := range draft.GetStringSlice("attachments") {
		if originalAttachmentName(name) == "drop.txt" {
			dropName = name
		}
	}

	re, _ := draftEvent(t, env.app, env.user, map[string]any{
		"mailbox_id": env.inbox.Id, "from_address_id": env.from.Id, "message_id": draft.Id,
		"subject": "s", "text_body": "body", "remove_attachments": []string{dropName},
	})
	if err := handleDraft(env.app, re); err != nil {
		t.Fatal(err)
	}
	saved, err := env.app.FindRecordById("mail_messages", draft.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedFiles(t, env.app, saved); len(got) != 1 || got["keep.txt"] != "keep" {
		t.Fatalf("draft attachments = %v, want keep.txt only", got)
	}
}

func TestOriginalAttachmentName(t *testing.T) {
	cases := map[string]string{
		"hippo_ab12cd34ef.jpg":     "hippo.jpg",
		"my_report_0123456789.pdf": "my_report.pdf",
		"plain.txt":                "plain.txt",
		"short_abc.txt":            "short_abc.txt",
	}
	for in, want := range cases {
		if got := originalAttachmentName(in); got != want {
			t.Errorf("originalAttachmentName(%q) = %q, want %q", in, got, want)
		}
	}
}
