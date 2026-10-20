package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
		MailboxID:        env.inbox.Id,
		FromAddressID:    env.from.Id,
		To:               []api.Recipient{{Email: "friend@example.net"}},
		Subject:          "hello",
		TextBody:         "hi",
		DraftID:          draft.Id,
		DraftAttachments: draft.GetStringSlice("attachments"),
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

func TestSendMessage_OnlyTheSavedFilesTheSenderKeptAreSent(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{
		"keep.txt": "keep", "drop.txt": "drop",
	})
	keepName := storedNameOf(t, draft, "keep.txt")

	result, err := sendMessage(env.app, sendParams{
		Ctx:              context.Background(),
		UserID:           env.user.Id,
		MailboxID:        env.inbox.Id,
		FromAddressID:    env.from.Id,
		To:               []api.Recipient{{Email: "friend@example.net"}},
		Subject:          "hello",
		TextBody:         "hi",
		DraftID:          draft.Id,
		DraftAttachments: []string{keepName},
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
		draft     *core.Record
	}{
		{"a draft in another mailbox", env.inbox.Id, env.from.Id, otherDraft},
		{"another person's draft, sent from their mailbox", otherInbox.Id, otherFrom.Id, otherDraft},
		{"a message that is not a draft", env.inbox.Id, env.from.Id, notADraft},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := sentMessageCount(t, env.app)
			_, err := sendMessage(env.app, sendParams{
				Ctx:              context.Background(),
				UserID:           env.user.Id,
				MailboxID:        tc.mailboxID,
				FromAddressID:    tc.fromID,
				To:               []api.Recipient{{Email: "friend@example.net"}},
				Subject:          "hello",
				TextBody:         "hi",
				DraftID:          tc.draft.Id,
				DraftAttachments: tc.draft.GetStringSlice("attachments"),
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
		Ctx:              context.Background(),
		UserID:           env.user.Id,
		MailboxID:        env.inbox.Id,
		FromAddressID:    env.from.Id,
		To:               []api.Recipient{{Email: "friend@example.net"}},
		Subject:          "hello",
		TextBody:         "hi",
		DraftID:          draft.Id,
		DraftAttachments: []string{stored},
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
	dropName := storedNameOf(t, draft, "drop.txt")

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
	t.Parallel()
	cases := map[string]string{
		"hippo_ab12cd34ef.jpg":     "hippo.jpg",
		"my_report_0123456789.pdf": "my_report.pdf",
		"plain.txt":                "plain.txt",
		"short_abc.txt":            "short_abc.txt",
		"README_ab12cd34ef":        "README",
		"my_notes_0123456789":      "my_notes",
		"README":                   "README",
	}
	for in, want := range cases {
		if got := originalAttachmentName(in); got != want {
			t.Errorf("originalAttachmentName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A kept name sent twice attaches the file once.
func TestDraftAttachments_KeptNameTwiceAttachesOnce(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{"alpha.txt": "a", "beta.txt": "b"})
	a := storedNameOf(t, draft, "alpha.txt")
	b := storedNameOf(t, draft, "beta.txt")

	got, err := draftAttachments(env.app, draft, []string{a, b, a})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(got))
	for i, att := range got {
		names[i] = att.Name
	}
	if strings.Join(names, ",") != "alpha.txt,beta.txt" {
		t.Fatalf("attachments = %v, want alpha.txt,beta.txt once each in the kept order", names)
	}
}

// storedNameOf is the stored filename of the draft's file attached as name.
func storedNameOf(t *testing.T, draft *core.Record, name string) string {
	t.Helper()
	for _, stored := range draft.GetStringSlice("attachments") {
		if originalAttachmentName(stored) == name {
			return stored
		}
	}
	t.Fatalf("the draft has no %s", name)
	return ""
}

// sendDraft sends a resumed draft from env's inbox with the given kept saved
// files and new uploads.
func sendDraft(env *draftSendEnv, draftID string, kept []string, uploads []Attachment) (*sendResultRecord, error) {
	return sendMessage(env.app, sendParams{
		Ctx:              context.Background(),
		UserID:           env.user.Id,
		MailboxID:        env.inbox.Id,
		FromAddressID:    env.from.Id,
		To:               []api.Recipient{{Email: "friend@example.net"}},
		Subject:          "hello",
		TextBody:         "hi",
		DraftID:          draftID,
		DraftAttachments: kept,
		Attachments:      uploads,
	})
}

// expectRefusedAndKept checks that a refused send stored no message and left
// the draft in place.
func expectRefusedAndKept(t *testing.T, env *draftSendEnv, draft *core.Record, before int, err error, wantInError string) {
	t.Helper()
	if err == nil {
		t.Fatal("the send was accepted")
	}
	if !strings.Contains(err.Error(), wantInError) {
		t.Fatalf("error %q does not say %q", err, wantInError)
	}
	var sendErr *sendError
	if !errors.As(err, &sendErr) || sendErr.kind != sendErrBadRequest {
		t.Fatalf("error %v is not a bad request", err)
	}
	if after := sentMessageCount(t, env.app); after != before {
		t.Fatalf("a refused send stored a message (%d → %d)", before, after)
	}
	if _, err := env.app.FindRecordById("mail_messages", draft.Id); err != nil {
		t.Fatalf("the draft was deleted after a refused send: %v", err)
	}
}

func TestSendMessage_RefusesMoreFilesThanTheSentCopyHolds(t *testing.T) {
	env := setupDraftSendEnv(t)
	files := map[string]string{}
	for i := range 15 {
		files[fmt.Sprintf("saved%02d.txt", i)] = "saved"
	}
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", files)
	var uploads []Attachment
	for i := range 6 {
		uploads = append(uploads, Attachment{Name: fmt.Sprintf("new%d.txt", i), ContentType: "text/plain", Content: "bmV3"})
	}

	before := sentMessageCount(t, env.app)
	_, err := sendDraft(env, draft.Id, draft.GetStringSlice("attachments"), uploads)
	expectRefusedAndKept(t, env, draft, before, err, "at most 20 attachments")
}

func TestSendMessage_RefusesAttachmentsOverTheSizeLimit(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{
		"big.bin": strings.Repeat("a", 6*1024*1024),
	})
	upload := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("b"), 5*1024*1024))

	before := sentMessageCount(t, env.app)
	_, err := sendDraft(env, draft.Id, draft.GetStringSlice("attachments"),
		[]Attachment{{Name: "more.bin", ContentType: "application/octet-stream", Content: upload}})
	expectRefusedAndKept(t, env, draft, before, err, "more than 10 MB")
}

func TestSendMessage_AKeptFileNoLongerOnTheDraftIsRefusedByName(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{"notes.txt": "n"})

	before := sentMessageCount(t, env.app)
	_, err := sendDraft(env, draft.Id, []string{"gone_ab12cd34ef.pdf"}, nil)
	expectRefusedAndKept(t, env, draft, before, err, `"gone.pdf"`)
}

// Another member deleted the draft while it was open. With no saved files
// shown, the person can still send what they wrote.
func TestSendMessage_AGoneDraftWithNoSavedFilesSendsAsANewMessage(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", nil)
	if err := env.app.Delete(draft); err != nil {
		t.Fatal(err)
	}

	result, err := sendDraft(env, draft.Id, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.app.FindRecordById("mail_messages", result.MessageRecordID); err != nil {
		t.Fatalf("no sent copy stored: %v", err)
	}
}

// With saved files shown, a gone draft took them along; sending without
// them would drop them silently.
func TestSendMessage_AGoneDraftWithSavedFilesShownIsRefused(t *testing.T) {
	env := setupDraftSendEnv(t)
	draft := seedDraft(t, env.app, env.inbox.Id, "draft", map[string]string{"notes.txt": "n"})
	kept := draft.GetStringSlice("attachments")
	if err := env.app.Delete(draft); err != nil {
		t.Fatal(err)
	}

	before := sentMessageCount(t, env.app)
	_, err := sendDraft(env, draft.Id, kept, nil)
	if err == nil || !strings.Contains(err.Error(), draftGoneMessage) {
		t.Fatalf("error = %v, want %q", err, draftGoneMessage)
	}
	if after := sentMessageCount(t, env.app); after != before {
		t.Fatalf("a refused send stored a message (%d → %d)", before, after)
	}
}

// A member whose access to a shared inbox comes from a group (a derived
// member row) has send rights, so resuming a draft there must work and keep
// its files.
func TestSendMessage_AGroupDerivedMemberSendsADraftWithItsFiles(t *testing.T) {
	app := newInboxGroupTestApp(t)
	registerAddressHooks(app)
	syscfg.SetResolver(func(key string) string {
		if key == "mail.postmark_server_token" {
			return "test-token"
		}
		return ""
	})
	t.Cleanup(func() { syscfg.SetResolver(func(string) string { return "" }) })

	s := seedInboxGroup(t, app)
	domain, err := app.FindRecordById("mail_domains", s.shared.GetString("domain"))
	if err != nil {
		t.Fatal(err)
	}
	domain.Set("spf_verified", true)
	domain.Set("dkim_verified", true)
	domain.Set("return_path_verified", true)
	if err := app.Save(domain); err != nil {
		t.Fatal(err)
	}
	s.ann.Set("is_demo", true)
	if err := app.Save(s.ann); err != nil {
		t.Fatal(err)
	}
	saveInboxGrant(t, app, s.shared, s.sales, "member")
	row := derivedRow(t, app, s.shared, s.sales, s.ann)
	if row == nil {
		t.Fatal("ann has no derived row on the shared inbox")
	}
	from, err := findAddressRow(app, "sales", domain.Id)
	if err != nil {
		t.Fatal(err)
	}
	draft := seedDraft(t, app, s.shared.Id, "draft", map[string]string{"quote.pdf": "%PDF-1.4 quote"})

	result, err := sendMessage(app, sendParams{
		Ctx:              context.Background(),
		UserID:           s.ann.Id,
		MailboxID:        s.shared.Id,
		FromAddressID:    from.Id,
		To:               []api.Recipient{{Email: "client@example.net"}},
		Subject:          "quote",
		TextBody:         "attached",
		DraftID:          draft.Id,
		DraftAttachments: draft.GetStringSlice("attachments"),
	})
	if err != nil {
		t.Fatal(err)
	}
	sent, err := app.FindRecordById("mail_messages", result.MessageRecordID)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedFiles(t, app, sent); len(got) != 1 || got["quote.pdf"] != "%PDF-1.4 quote" {
		t.Fatalf("sent attachments = %v, want quote.pdf", got)
	}
	if _, err := app.FindRecordById("mail_messages", draft.Id); err == nil {
		t.Fatal("the draft still exists after a successful send")
	}
}
