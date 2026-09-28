package mail

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/deliveryevents"
)

// setupDeliveryEventsTestApp builds on setupInboundTestApp's mail_messages
// fixture (thread/message_id/delivery_status already present) with the three
// columns applyDeliveryEvent writes, matching the shipped migration
// 1830000009 (delivered_at: DateField, bounce_class: SelectField
// soft/hard/complaint) plus the pre-existing bounce_reason TextField.
func setupDeliveryEventsTestApp(t *testing.T) core.App {
	t.Helper()
	app := setupInboundTestApp(t)

	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	messages.Fields.Add(&core.TextField{Name: "bounce_reason"})
	messages.Fields.Add(&core.DateField{Name: "delivered_at"})
	messages.Fields.Add(&core.SelectField{
		Name: "bounce_class", MaxSelect: 1,
		Values: []string{"soft", "hard", "complaint"},
	})
	if err := app.Save(messages); err != nil {
		t.Fatalf("failed to add delivery tracking fields: %v", err)
	}
	return app
}

// newDeliveryTestMessage creates a mail_messages row with the given provider
// message_id and delivery_status, standing in for a message this deployment
// already sent — the row applyDeliveryEvent must find and update.
func newDeliveryTestMessage(t *testing.T, app core.App, threadID, messageID, deliveryStatus string) *core.Record {
	t.Helper()
	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	msg := core.NewRecord(messages)
	msg.Set("thread", threadID)
	msg.Set("message_id", messageID)
	msg.Set("delivery_status", deliveryStatus)
	if err := app.Save(msg); err != nil {
		t.Fatalf("failed to save message: %v", err)
	}
	return msg
}

func TestApplyDeliveryEvent_DeliveredSetsTimestampOnly(t *testing.T) {
	app := setupDeliveryEventsTestApp(t)
	seedDomainAndMailbox(t, app, "delivery-events.test", "alice", "mb_delivered0001")
	thread := newTestThread(t, app, padID("mb_delivered0001"), "hello")
	newDeliveryTestMessage(t, app, thread.Id, "pm-1", "sent")

	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	handled, err := applyDeliveryEvent(app, deliveryevents.Event{
		Kind:              deliveryevents.Delivered,
		ProviderMessageID: "pm-1",
		At:                at,
	})
	if err != nil {
		t.Fatalf("applyDeliveryEvent returned error: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true for a known message_id")
	}

	record, err := app.FindFirstRecordByFilter("mail_messages", "message_id = {:id}", map[string]any{"id": "pm-1"})
	if err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if record.GetString("delivery_status") != "sent" {
		t.Errorf("delivery_status = %q, want unchanged %q", record.GetString("delivery_status"), "sent")
	}
	if record.GetDateTime("delivered_at").Time().IsZero() {
		t.Error("delivered_at was not set")
	}
}

func TestApplyDeliveryEvent_DeliveredAfterBouncedIsIgnored(t *testing.T) {
	app := setupDeliveryEventsTestApp(t)
	seedDomainAndMailbox(t, app, "delivery-events.test", "alice", "mb_delivered0002")
	thread := newTestThread(t, app, padID("mb_delivered0002"), "hello")
	newDeliveryTestMessage(t, app, thread.Id, "pm-1", "bounced")

	handled, err := applyDeliveryEvent(app, deliveryevents.Event{
		Kind:              deliveryevents.Delivered,
		ProviderMessageID: "pm-1",
		At:                time.Now(),
	})
	if err != nil {
		t.Fatalf("applyDeliveryEvent returned error: %v", err)
	}
	// A late per-recipient delivery for an already-failed send is not new
	// information, and must not be reported unhandled either — the event was
	// recognised and deliberately dropped, not a message from someone else.
	if !handled {
		t.Fatal("handled = false, want true (recognised but ignored)")
	}

	record, err := app.FindFirstRecordByFilter("mail_messages", "message_id = {:id}", map[string]any{"id": "pm-1"})
	if err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if !record.GetDateTime("delivered_at").Time().IsZero() {
		t.Error("delivered_at must stay empty when a delivery arrives after a bounce")
	}
	if record.GetString("delivery_status") != "bounced" {
		t.Errorf("delivery_status = %q, want unchanged %q", record.GetString("delivery_status"), "bounced")
	}
}

func TestApplyDeliveryEvent_BouncedSetsStatusClassAndTruncatedReason(t *testing.T) {
	app := setupDeliveryEventsTestApp(t)
	seedDomainAndMailbox(t, app, "delivery-events.test", "alice", "mb_delivered0003")
	thread := newTestThread(t, app, padID("mb_delivered0003"), "hello")
	newDeliveryTestMessage(t, app, thread.Id, "pm-1", "sent")

	// "é" is multi-byte in UTF-8; a byte-based cut at 500 would split a
	// character and produce invalid UTF-8, so this locks in the rune-safe cut.
	longReason := ""
	for i := 0; i < 600; i++ {
		longReason += "é"
	}

	handled, err := applyDeliveryEvent(app, deliveryevents.Event{
		Kind:              deliveryevents.Bounced,
		ProviderMessageID: "pm-1",
		Class:             "hard",
		Reason:            longReason,
		At:                time.Now(),
	})
	if err != nil {
		t.Fatalf("applyDeliveryEvent returned error: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}

	record, err := app.FindFirstRecordByFilter("mail_messages", "message_id = {:id}", map[string]any{"id": "pm-1"})
	if err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if record.GetString("delivery_status") != "bounced" {
		t.Errorf("delivery_status = %q, want bounced", record.GetString("delivery_status"))
	}
	if record.GetString("bounce_class") != "hard" {
		t.Errorf("bounce_class = %q, want hard", record.GetString("bounce_class"))
	}
	stored := record.GetString("bounce_reason")
	if got := len([]rune(stored)); got != 500 {
		t.Errorf("bounce_reason length = %d, want truncated to 500", got)
	}
	if !utf8.ValidString(stored) {
		t.Errorf("bounce_reason is not valid UTF-8: %q", stored)
	}
}

func TestApplyDeliveryEvent_ComplaintSetsSpamComplaintAndClass(t *testing.T) {
	app := setupDeliveryEventsTestApp(t)
	seedDomainAndMailbox(t, app, "delivery-events.test", "alice", "mb_delivered0004")
	thread := newTestThread(t, app, padID("mb_delivered0004"), "hello")
	newDeliveryTestMessage(t, app, thread.Id, "pm-1", "sent")

	handled, err := applyDeliveryEvent(app, deliveryevents.Event{
		Kind:              deliveryevents.Complaint,
		ProviderMessageID: "pm-1",
		Reason:            "recipient flagged as spam",
		At:                time.Now(),
	})
	if err != nil {
		t.Fatalf("applyDeliveryEvent returned error: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}

	record, err := app.FindFirstRecordByFilter("mail_messages", "message_id = {:id}", map[string]any{"id": "pm-1"})
	if err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if record.GetString("delivery_status") != "spam_complaint" {
		t.Errorf("delivery_status = %q, want spam_complaint", record.GetString("delivery_status"))
	}
	if record.GetString("bounce_class") != "complaint" {
		t.Errorf("bounce_class = %q, want complaint", record.GetString("bounce_class"))
	}
	if record.GetString("bounce_reason") != "recipient flagged as spam" {
		t.Errorf("bounce_reason = %q", record.GetString("bounce_reason"))
	}
}

func TestApplyDeliveryEvent_UnknownProviderIDReturnsFalseNil(t *testing.T) {
	app := setupDeliveryEventsTestApp(t)

	handled, err := applyDeliveryEvent(app, deliveryevents.Event{
		Kind:              deliveryevents.Delivered,
		ProviderMessageID: "pm-does-not-exist",
		At:                time.Now(),
	})
	if err != nil {
		t.Fatalf("applyDeliveryEvent returned error: %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false for an unknown message_id")
	}
}

// messageIsInbound (the "mail:message-received" TriggerFilter, automation.go)
// reads only delivery_status, so a delivered outbound message — status
// "sent" plus a populated delivered_at — must still be rejected. This proves
// wiring delivered_at into the schema didn't loosen that filter.
func TestApplyDeliveryEvent_DeliveredOutboundMessageStillNotInbound(t *testing.T) {
	app := setupDeliveryEventsTestApp(t)
	seedDomainAndMailbox(t, app, "delivery-events.test", "alice", "mb_delivered0005")
	thread := newTestThread(t, app, padID("mb_delivered0005"), "hello")
	msg := newDeliveryTestMessage(t, app, thread.Id, "pm-1", "sent")

	if _, err := applyDeliveryEvent(app, deliveryevents.Event{
		Kind:              deliveryevents.Delivered,
		ProviderMessageID: "pm-1",
		At:                time.Now(),
	}); err != nil {
		t.Fatalf("applyDeliveryEvent returned error: %v", err)
	}

	record, err := app.FindRecordById("mail_messages", msg.Id)
	if err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if messageIsInbound(app, record) {
		t.Fatal("a delivered outbound message must not be treated as inbound")
	}
}

func TestEventFromBounce_DeliveryMapsToDelivered(t *testing.T) {
	b := &BounceEvent{
		RecordType:  "Delivery",
		MessageID:   "pm-1",
		DeliveredAt: "2026-01-02T03:04:05Z",
	}

	event, recognised, ok := eventFromBounce(b)
	if !ok {
		t.Fatal("eventFromBounce reported not-ok for a Delivery record")
	}
	if !recognised {
		t.Error("recognised = false, want true for a Delivery record")
	}
	if event.Kind != deliveryevents.Delivered {
		t.Errorf("Kind = %v, want Delivered", event.Kind)
	}
	if event.ProviderMessageID != "pm-1" {
		t.Errorf("ProviderMessageID = %q, want pm-1", event.ProviderMessageID)
	}
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if !event.At.Equal(want) {
		t.Errorf("At = %v, want %v", event.At, want)
	}
}

func TestEventFromBounce_SpamComplaintMapsToComplaint(t *testing.T) {
	b := &BounceEvent{RecordType: "SpamComplaint", MessageID: "pm-1", Description: "spam"}

	event, recognised, ok := eventFromBounce(b)
	if !ok {
		t.Fatal("eventFromBounce reported not-ok for a SpamComplaint record")
	}
	if !recognised {
		t.Error("recognised = false, want true for a SpamComplaint record")
	}
	if event.Kind != deliveryevents.Complaint {
		t.Errorf("Kind = %v, want Complaint", event.Kind)
	}
	if event.Class != "complaint" {
		t.Errorf("Class = %q, want complaint", event.Class)
	}
	if event.Reason != "spam" {
		t.Errorf("Reason = %q, want spam", event.Reason)
	}
}

func TestEventFromBounce_HardBounceMapsToBouncedHard(t *testing.T) {
	b := &BounceEvent{RecordType: "Bounce", MessageID: "pm-1", TypeCode: pmCodeHardBounce, Description: "no such user"}

	event, recognised, ok := eventFromBounce(b)
	if !ok {
		t.Fatal("eventFromBounce reported not-ok for a recognised hard bounce")
	}
	if !recognised {
		t.Error("recognised = false, want true for a recognised hard bounce")
	}
	if event.Kind != deliveryevents.Bounced {
		t.Errorf("Kind = %v, want Bounced", event.Kind)
	}
	if event.Class != "hard" {
		t.Errorf("Class = %q, want hard", event.Class)
	}
}

// The unrecognised case is the one the review caught: ok (there IS a message
// to apply the event to) must stay true so the bounce still gets recorded,
// but recognised must go false so handleBounce's "unrecognised bounce type"
// warning still fires — folding the two into one bool silenced that warning
// for every unmapped provider failure type.
func TestEventFromBounce_UnrecognisedBounceIsStillAppliedButNotRecognised(t *testing.T) {
	b := &BounceEvent{RecordType: "Bounce", MessageID: "pm-1", TypeCode: 999999}

	event, recognised, ok := eventFromBounce(b)
	if !ok {
		t.Fatal("eventFromBounce reported not-ok for an unrecognised bounce")
	}
	if recognised {
		t.Error("recognised = true, want false for an unrecognised type")
	}
	if event.Kind != deliveryevents.Bounced {
		t.Errorf("Kind = %v, want Bounced", event.Kind)
	}
	if event.Class != "" {
		t.Errorf("Class = %q, want empty for an unrecognised type", event.Class)
	}
}

func TestEventFromBounce_EmptyMessageIDIsNotOk(t *testing.T) {
	_, _, ok := eventFromBounce(&BounceEvent{RecordType: "Bounce", TypeCode: pmCodeHardBounce})
	if ok {
		t.Fatal("ok = true, want false when the payload carries no message id")
	}
}

// A recognised non-failure (auto-responder, unsubscribe, …) must not be
// applied as a Bounced event with an empty class — that showed as "Bounced"
// on the address step and blocked a later real Delivered event. It matches
// the hosted router, which attributes and logs these but never forwards
// them because there is no outcome to record.
func TestEventFromBounce_RecognisedNonFailureIsNotOk(t *testing.T) {
	b := &BounceEvent{RecordType: "Bounce", MessageID: "pm-1", TypeCode: pmCodeAutoResponder}

	event, recognised, ok := eventFromBounce(b)
	if ok {
		t.Fatal("ok = true, want false for a recognised non-failure (auto-responder)")
	}
	if !recognised {
		t.Error("recognised = false, want true — classifyBounce does recognise this type")
	}
	if event != (deliveryevents.Event{}) {
		t.Errorf("event = %+v, want zero value when ok is false", event)
	}
}

// An UNRECOGNISED type must still be applied as Bounced with an empty class
// — this is the case that keeps today's behaviour and the warning log firing
// (see TestEventFromBounce_UnrecognisedBounceIsStillAppliedButNotRecognised).
// This test locks in the recognised-vs-unrecognised split at the ok boundary:
// only the recognised non-failure short-circuits to ok=false.
func TestEventFromBounce_UnrecognisedTypeIsStillOk(t *testing.T) {
	b := &BounceEvent{RecordType: "Bounce", MessageID: "pm-1", TypeCode: 999999}

	event, recognised, ok := eventFromBounce(b)
	if !ok {
		t.Fatal("ok = false, want true — an unrecognised type is still applied")
	}
	if recognised {
		t.Error("recognised = true, want false for an unrecognised type")
	}
	if event.Kind != deliveryevents.Bounced {
		t.Errorf("Kind = %v, want Bounced", event.Kind)
	}
	if event.Class != "" {
		t.Errorf("Class = %q, want empty for an unrecognised type", event.Class)
	}
}

// An unparsable or empty DeliveredAt must fall back to now, matching the
// bounce path's own bouncedAtOrNow — otherwise delivered_at stays empty while
// the caller still reports handled=true, silently losing the timestamp.
func TestEventFromBounce_DeliveryWithEmptyDeliveredAtFallsBackToNow(t *testing.T) {
	before := time.Now().UTC()
	b := &BounceEvent{RecordType: "Delivery", MessageID: "pm-1", DeliveredAt: ""}

	event, _, ok := eventFromBounce(b)
	if !ok {
		t.Fatal("ok = false, want true for a Delivery record")
	}
	if event.At.IsZero() {
		t.Fatal("At is zero, want a fallback to the current time")
	}
	if event.At.Before(before) || event.At.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("At = %v, want close to now (%v)", event.At, before)
	}
}

func TestEventFromBounce_DeliveryWithUnparsableDeliveredAtFallsBackToNow(t *testing.T) {
	before := time.Now().UTC()
	b := &BounceEvent{RecordType: "Delivery", MessageID: "pm-1", DeliveredAt: "not-a-timestamp"}

	event, _, ok := eventFromBounce(b)
	if !ok {
		t.Fatal("ok = false, want true for a Delivery record")
	}
	if event.At.Before(before) || event.At.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("At = %v, want close to now (%v)", event.At, before)
	}
}

// applyDeliveryEvent must not trust e.Class from the wire — Save would fail
// (500) against the bounce_class SelectField's fixed values for anything
// outside soft/hard/complaint. An unknown value is stored as "" instead.
func TestApplyDeliveryEvent_BouncedWithUnknownClassStoresEmpty(t *testing.T) {
	app := setupDeliveryEventsTestApp(t)
	seedDomainAndMailbox(t, app, "delivery-events.test", "alice", "mb_delivered0006")
	thread := newTestThread(t, app, padID("mb_delivered0006"), "hello")
	newDeliveryTestMessage(t, app, thread.Id, "pm-1", "sent")

	handled, err := applyDeliveryEvent(app, deliveryevents.Event{
		Kind:              deliveryevents.Bounced,
		ProviderMessageID: "pm-1",
		Class:             "not-a-real-class",
		Reason:            "unexpected wire value",
		At:                time.Now(),
	})
	if err != nil {
		t.Fatalf("applyDeliveryEvent returned error: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}

	record, err := app.FindFirstRecordByFilter("mail_messages", "message_id = {:id}", map[string]any{"id": "pm-1"})
	if err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if record.GetString("delivery_status") != "bounced" {
		t.Errorf("delivery_status = %q, want bounced", record.GetString("delivery_status"))
	}
	if record.GetString("bounce_class") != "" {
		t.Errorf("bounce_class = %q, want empty for an unrecognised wire value", record.GetString("bounce_class"))
	}
}
