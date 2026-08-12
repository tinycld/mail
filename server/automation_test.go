package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// automation_test.go proves messageOwnerResolver maps an arriving
// mail_messages record to the mailbox members who own it, reusing the same
// thread -> mailbox -> mail_mailbox_members resolution bufferMailNotification
// already relies on for notification delivery.

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
	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	msg := core.NewRecord(messages)
	msg.Set("thread", threadID)
	msg.Set("subject", subject)
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
