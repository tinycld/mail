package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
)

// makeInboundRequest constructs a *core.RequestEvent suitable for calling
// handleInbound directly. The request body is the JSON-encoded payload, the
// {token} path value is set explicitly, and the response writer is a
// httptest.ResponseRecorder so the test can inspect status and body.
func makeInboundRequest(t *testing.T, app core.App, token string, payload []byte) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/mail/inbound/"+token, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("token", token)

	rec := httptest.NewRecorder()

	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	return re, rec
}

// postmarkPayload builds a minimal Postmark inbound JSON payload for the given
// recipient(s).
func postmarkPayload(t *testing.T, recipients []string, subject, textBody, messageID string) []byte {
	t.Helper()

	type recipient struct {
		Name  string `json:"Name"`
		Email string `json:"Email"`
	}
	type header struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
	}
	type payload struct {
		FromFull  recipient   `json:"FromFull"`
		ToFull    []recipient `json:"ToFull"`
		Subject   string      `json:"Subject"`
		TextBody  string      `json:"TextBody"`
		HTMLBody  string      `json:"HtmlBody"`
		Date      string      `json:"Date"`
		MessageID string      `json:"MessageID"`
		Headers   []header    `json:"Headers"`
	}

	to := make([]recipient, 0, len(recipients))
	for _, addr := range recipients {
		to = append(to, recipient{Email: addr})
	}

	p := payload{
		FromFull:  recipient{Name: "Sender", Email: "sender@example.org"},
		ToFull:    to,
		Subject:   subject,
		TextBody:  textBody,
		Date:      "Fri, 02 May 2026 10:00:00 -0500",
		MessageID: messageID,
	}

	body, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	return body
}

// stubProvider is a Provider that returns a parsed InboundMessage from a
// pre-supplied parser func. Lets tests force ParseInbound success or failure
// without depending on Postmark's exact JSON shape.
type stubProvider struct {
	parse func(body []byte) (*InboundMessage, error)
}

func (s *stubProvider) Send(_ context.Context, _ *SendRequest) (*SendResult, error) {
	return nil, nil
}
func (s *stubProvider) ParseInbound(body []byte) (*InboundMessage, error) {
	return s.parse(body)
}
func (s *stubProvider) ParseBounce(_ []byte) (*BounceEvent, error) { return nil, nil }
func (s *stubProvider) VerifyWebhookSignature(_ map[string]string, _ []byte) error {
	return nil
}
func (s *stubProvider) AddDomain(_ context.Context, _ string) (*DomainVerification, error) {
	return nil, nil
}
func (s *stubProvider) CheckDomainVerification(_ context.Context, _ string) (*DomainVerification, error) {
	return nil, nil
}
func (s *stubProvider) CheckInboundDomain(_ context.Context) (*InboundVerification, error) {
	return nil, nil
}

// setupInboundTestApp creates a test app with all collections needed by the
// inbound webhook flow.
func setupInboundTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := setupAliasTestApp(t)

	mailboxesCol, err := app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatalf("mail_mailboxes collection missing: %v", err)
	}

	members := core.NewBaseCollection("mail_mailbox_members")
	members.Fields.Add(&core.RelationField{
		Name:          "mailbox",
		Required:      true,
		CollectionId:  mailboxesCol.Id,
		CascadeDelete: true,
		MaxSelect:     1,
	})
	members.Fields.Add(&core.TextField{Name: "user_org", Required: true})
	members.Fields.Add(&core.TextField{Name: "role"})
	if err := app.Save(members); err != nil {
		t.Fatalf("failed to save mail_mailbox_members: %v", err)
	}

	threads := core.NewBaseCollection("mail_threads")
	threads.Fields.Add(&core.RelationField{
		Name:          "mailbox",
		Required:      true,
		CollectionId:  mailboxesCol.Id,
		CascadeDelete: true,
		MaxSelect:     1,
	})
	threads.Fields.Add(&core.TextField{Name: "subject"})
	threads.Fields.Add(&core.TextField{Name: "snippet"})
	threads.Fields.Add(&core.NumberField{Name: "message_count"})
	threads.Fields.Add(&core.TextField{Name: "latest_date"})
	threads.Fields.Add(&core.TextField{Name: "participants"})
	threads.Fields.Add(&core.TextField{Name: "last_sender_name"})
	threads.Fields.Add(&core.TextField{Name: "last_sender_email"})
	if err := app.Save(threads); err != nil {
		t.Fatalf("failed to save mail_threads: %v", err)
	}

	messages := core.NewBaseCollection("mail_messages")
	messages.Fields.Add(&core.RelationField{
		Name:          "thread",
		Required:      true,
		CollectionId:  threads.Id,
		CascadeDelete: true,
		MaxSelect:     1,
	})
	messages.Fields.Add(&core.TextField{Name: "message_id"})
	messages.Fields.Add(&core.TextField{Name: "in_reply_to"})
	messages.Fields.Add(&core.TextField{Name: "alias"})
	messages.Fields.Add(&core.TextField{Name: "sender_name"})
	messages.Fields.Add(&core.TextField{Name: "sender_email"})
	messages.Fields.Add(&core.TextField{Name: "date"})
	messages.Fields.Add(&core.TextField{Name: "subject"})
	messages.Fields.Add(&core.TextField{Name: "snippet"})
	messages.Fields.Add(&core.BoolField{Name: "has_attachments"})
	messages.Fields.Add(&core.NumberField{Name: "total_size"})
	messages.Fields.Add(&core.TextField{Name: "delivery_status"})
	messages.Fields.Add(&core.FileField{Name: "body_html", MaxSelect: 1})
	messages.Fields.Add(&core.FileField{Name: "attachments", MaxSelect: 99})
	messages.Fields.Add(&core.TextField{Name: "recipients_to"})
	messages.Fields.Add(&core.TextField{Name: "recipients_cc"})
	if err := app.Save(messages); err != nil {
		t.Fatalf("failed to save mail_messages: %v", err)
	}

	threadState := core.NewBaseCollection("mail_thread_state")
	threadState.Fields.Add(&core.RelationField{
		Name:          "thread",
		Required:      true,
		CollectionId:  threads.Id,
		CascadeDelete: true,
		MaxSelect:     1,
	})
	threadState.Fields.Add(&core.TextField{Name: "user_org", Required: true})
	threadState.Fields.Add(&core.TextField{Name: "folder"})
	threadState.Fields.Add(&core.BoolField{Name: "is_read"})
	threadState.Fields.Add(&core.BoolField{Name: "is_starred"})
	if err := app.Save(threadState); err != nil {
		t.Fatalf("failed to save mail_thread_state: %v", err)
	}

	return app
}

// seedMember creates a mail_mailbox_members row linking a mailbox to a user_org.
func seedMember(t *testing.T, app core.App, mailboxID, userOrgID string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_mailbox_members")
	if err != nil {
		t.Fatalf("mail_mailbox_members collection missing: %v", err)
	}
	member := core.NewRecord(col)
	member.Set("mailbox", padID(mailboxID))
	member.Set("user_org", userOrgID)
	member.Set("role", "owner")
	if err := app.Save(member); err != nil {
		t.Fatalf("failed to save mailbox member: %v", err)
	}
}

// TestHandleInbound_UnknownRecipientReturns403 — when the only To address
// doesn't resolve to any mailbox, return 403 so Postmark generates a bounce.
func TestHandleInbound_UnknownRecipientReturns403(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "real", "mb_unknown_001")

	body := postmarkPayload(t, []string{"nobody@acme.com"}, "test", "hello", "<msg-unknown-1@example.org>")
	re, rec := makeInboundRequest(t, app, "tok-unknown", body)

	provider := &stubProvider{parse: (&PostmarkProvider{}).ParseInbound}

	err := handleInbound(app, provider, re, "tok-unknown")
	if err == nil {
		t.Fatalf("expected error response (403), got nil (recorder body: %s)", rec.Body.String())
	}

	apiErr, ok := err.(*router.ApiError)
	if !ok {
		t.Fatalf("expected *router.ApiError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", apiErr.Status)
	}
}
