package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// automation_test.go proves:
//   - messageOwnerResolver maps an arriving mail_messages record to the
//     mailbox members who own it, reusing the same thread -> mailbox ->
//     mail_mailbox_members resolution bufferMailNotification already relies
//     on for notification delivery (mailboxMembersForMessage, register.go).
//   - messageIsInbound (the "mail:message-received" TriggerFilter) admits
//     genuinely received messages and rejects drafts/outbound sends, so the
//     mail_messages create hook firing for every kind of message row doesn't
//     turn every draft keystroke or outbound send into a "message-received"
//     automation run.

func newTestThread(t *testing.T, app core.App, mailboxID, subject string) *core.Record {
	t.Helper()
	threads, err := app.FindCollectionByNameOrId("mail_threads")
	if err != nil {
		t.Fatalf("mail_threads collection missing: %v", err)
	}
	thread := core.NewRecord(threads)
	thread.Set("mailbox", mailboxID)
	thread.Set("subject", subject)
	if err := app.Save(thread); err != nil {
		t.Fatalf("failed to save thread: %v", err)
	}
	return thread
}

func newTestMessage(t *testing.T, app core.App, threadID, subject string) *core.Record {
	t.Helper()
	return newTestMessageWithStatus(t, app, threadID, subject, "")
}

func newTestMessageWithStatus(t *testing.T, app core.App, threadID, subject, deliveryStatus string) *core.Record {
	t.Helper()
	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	msg := core.NewRecord(messages)
	msg.Set("thread", threadID)
	msg.Set("subject", subject)
	if deliveryStatus != "" {
		msg.Set("delivery_status", deliveryStatus)
	}
	if err := app.Save(msg); err != nil {
		t.Fatalf("failed to save message: %v", err)
	}
	return msg
}

func TestMessageOwnerResolution(t *testing.T) {
	app := setupInboundTestApp(t)

	seedDomainAndMailbox(t, app, "owner-resolver.test", "alice", "mb_owner_resolve1")
	mailboxID := padID("mb_owner_resolve1")
	seedMember(t, app, mailboxID, "user_a_0000000")

	thread := newTestThread(t, app, mailboxID, "hello")
	msg := newTestMessage(t, app, thread.Id, "hello")

	owners := messageOwnerResolver(app, msg)
	if len(owners) != 1 || owners[0] != "user_a_0000000" {
		t.Fatalf("owners = %v, want [user_a_0000000]", owners)
	}
}

func TestMessageOwnerResolution_DanglingThreadResolvesNil(t *testing.T) {
	app := setupInboundTestApp(t)

	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	msg := core.NewRecord(messages)
	msg.Id = padID("msg_dangling0001")
	msg.Set("thread", padID("no_such_thread1"))
	msg.Set("subject", "orphan")
	// Skip normal validation: the point of this fixture is an inconsistent
	// thread reference, which app.Save's relation check would reject.
	if err := app.SaveNoValidate(msg); err != nil {
		t.Fatalf("failed to save dangling message: %v", err)
	}

	if owners := messageOwnerResolver(app, msg); owners != nil {
		t.Fatalf("owners = %v, want nil for a dangling thread reference", owners)
	}
}

func TestMessageIsInbound_AdmitsReceivedMail(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "inbound-filter.test", "alice", "mb_inbound_filtr1")
	thread := newTestThread(t, app, padID("mb_inbound_filtr1"), "hello")

	// The inbound webhook never sets delivery_status; storeMessage defaults
	// it to "sending". IMAP APPEND-to-inbox sets "delivered". Both are
	// genuinely received mail and must pass the filter.
	unset := newTestMessage(t, app, thread.Id, "webhook delivery")
	if !messageIsInbound(app, unset) {
		t.Fatal("a message with no delivery_status must be admitted (inbound webhook default)")
	}

	delivered := newTestMessageWithStatus(t, app, thread.Id, "imap append", "delivered")
	if !messageIsInbound(app, delivered) {
		t.Fatal("a delivered message must be admitted")
	}
}

func TestMessageIsInbound_RejectsOutboundAndDrafts(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "outbound-filter.test", "alice", "mb_outbound_fltr1")
	thread := newTestThread(t, app, padID("mb_outbound_fltr1"), "hello")

	for _, status := range []string{"draft", "sent", "bounced", "spam_complaint"} {
		msg := newTestMessageWithStatus(t, app, thread.Id, "subject-"+status, status)
		if messageIsInbound(app, msg) {
			t.Errorf("delivery_status %q must be rejected by the message-received filter", status)
		}
	}
}

// messageDidBounce gates "mail:message-bounced". The declaration watches
// delivery_status, so the hook fires on every transition of that column —
// including the sending → sent path every successful send takes.
func TestMessageDidBounce_AdmitsOnlyDeliveryFailures(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "bounce-filter.test", "alice", "mb_bounce_filter")
	thread := newTestThread(t, app, padID("mb_bounce_filter"), "hello")

	for _, status := range []string{"bounced", "spam_complaint"} {
		msg := newTestMessageWithStatus(t, app, thread.Id, "subject-"+status, status)
		if !messageDidBounce(app, msg) {
			t.Errorf("delivery_status %q must be admitted as a bounce", status)
		}
	}

	// "sent" is the one that matters: every successful send passes through it,
	// and admitting it would run a bounce rule on all outbound mail.
	for _, status := range []string{"sending", "sent", "delivered", "draft", ""} {
		msg := newTestMessageWithStatus(t, app, thread.Id, "ok-"+status, status)
		if messageDidBounce(app, msg) {
			t.Errorf("delivery_status %q must NOT be treated as a bounce", status)
		}
	}

	if messageDidBounce(app, nil) {
		t.Error("a nil record must not be treated as a bounce")
	}
}
