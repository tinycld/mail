package mail

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/packages/mail/api"
)

// makeBounceRequest constructs a *core.RequestEvent suitable for calling
// handleBounce directly, mirroring makeInboundRequest in
// endpoints_inbound_test.go.
func makeBounceRequest(t *testing.T, app core.App, token string, payload []byte) (*core.RequestEvent, *httptest.ResponseRecorder) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/mail/bounce/"+token, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("token", token)

	rec := httptest.NewRecorder()

	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	return re, rec
}

// postmarkBouncePayloadJSON builds a minimal Postmark bounce/delivery webhook
// body with the given RecordType and TypeCode.
func postmarkBouncePayloadJSON(t *testing.T, recordType string, typeCode int, messageID string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"RecordType": recordType,
		"TypeCode":   typeCode,
		"MessageID":  messageID,
	})
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	return body
}

func decodeAck(t *testing.T, rec *httptest.ResponseRecorder) api.WebhookAckResponse {
	t.Helper()
	var ack api.WebhookAckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &ack); err != nil {
		t.Fatalf("failed to decode response body %q: %v", rec.Body.String(), err)
	}
	return ack
}

// A RECOGNISED non-failure (auto-responder, TypeCode 64) must reply
// "ignored" and must not touch the message row — matching the hosted
// router, which does not forward these either.
func TestHandleBounce_RecognisedNonFailureRepliesIgnored(t *testing.T) {
	app := setupDeliveryEventsTestApp(t)
	seedDomainAndMailbox(t, app, "delivery-events.test", "alice", "mb_bounce00001")
	thread := newTestThread(t, app, padID("mb_bounce00001"), "hello")
	newDeliveryTestMessage(t, app, thread.Id, "pm-bounce-1", "sent")

	payload := postmarkBouncePayloadJSON(t, "Bounce", pmCodeAutoResponder, "pm-bounce-1")
	re, rec := makeBounceRequest(t, app, "secret", payload)

	if err := handleBounce(app, &PostmarkProvider{}, re, "secret"); err != nil {
		t.Fatalf("handleBounce returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ack := decodeAck(t, rec); ack.Status != "ignored" {
		t.Errorf("status = %q, want ignored", ack.Status)
	}

	record, err := app.FindFirstRecordByFilter("mail_messages", "message_id = {:id}", map[string]any{"id": "pm-bounce-1"})
	if err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if record.GetString("delivery_status") != "sent" {
		t.Errorf("delivery_status = %q, want unchanged %q", record.GetString("delivery_status"), "sent")
	}
}

// An UNRECOGNISED type keeps today's behaviour: applied as Bounced with an
// empty class, and the endpoint still answers "processed".
func TestHandleBounce_UnrecognisedTypeStillAppliesAndProcesses(t *testing.T) {
	app := setupDeliveryEventsTestApp(t)
	seedDomainAndMailbox(t, app, "delivery-events.test", "alice", "mb_bounce00002")
	thread := newTestThread(t, app, padID("mb_bounce00002"), "hello")
	newDeliveryTestMessage(t, app, thread.Id, "pm-bounce-2", "sent")

	payload := postmarkBouncePayloadJSON(t, "Bounce", 999999, "pm-bounce-2")
	re, rec := makeBounceRequest(t, app, "secret", payload)

	if err := handleBounce(app, &PostmarkProvider{}, re, "secret"); err != nil {
		t.Fatalf("handleBounce returned error: %v", err)
	}
	if ack := decodeAck(t, rec); ack.Status != "processed" {
		t.Errorf("status = %q, want processed", ack.Status)
	}

	record, err := app.FindFirstRecordByFilter("mail_messages", "message_id = {:id}", map[string]any{"id": "pm-bounce-2"})
	if err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if record.GetString("delivery_status") != "bounced" {
		t.Errorf("delivery_status = %q, want bounced", record.GetString("delivery_status"))
	}
	if record.GetString("bounce_class") != "" {
		t.Errorf("bounce_class = %q, want empty for an unrecognised type", record.GetString("bounce_class"))
	}
}
