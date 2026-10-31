package mail

import (
	"context"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/background"
)

// Stopping one app's fetcher must not take the settings-change hook away
// from another app, and the two apps must not share one manager.
func TestIMAPFetcherManagerIsPerApp(t *testing.T) {
	a, b := newNotificationsTestApp(t), newNotificationsTestApp(t)

	stopA := startIMAPFetchers(a, background.For(a, "mail"))
	stopB := startIMAPFetchers(b, background.For(b, "mail"))
	t.Cleanup(stopB)

	if imapManagerFor(a) == nil || imapManagerFor(a) == imapManagerFor(b) {
		t.Fatal("the two apps do not each have their own IMAP fetcher manager")
	}
	stopA()
	if imapManagerFor(a) != nil {
		t.Fatal("app A's manager survived its shutdown")
	}
	if m := imapManagerFor(b); m == nil || m.app != core.App(b) {
		t.Fatal("stopping app A removed app B's manager")
	}
}

// Mailbox ids are unique only in their app: a change in one app must not
// wake an IDLE session of another.
func TestIdleNotifierIsPerApp(t *testing.T) {
	a, b := newNotificationsTestApp(t), newNotificationsTestApp(t)
	ch := make(chan struct{}, 1)
	idleNotifier(a).Subscribe("mbx-1", ch)
	t.Cleanup(func() { idleNotifier(a).Unsubscribe("mbx-1", ch) })

	idleNotifier(b).Notify("mbx-1")
	select {
	case <-ch:
		t.Fatal("a change on app B woke an IDLE session on app A")
	default:
	}

	idleNotifier(a).Notify("mbx-1")
	select {
	case <-ch:
	default:
		t.Fatal("app A's own change did not wake its session")
	}
}

// Each app's batcher flushes only its own buffer.
func TestMailBufferIsPerApp(t *testing.T) {
	a, b := newNotificationsTestApp(t), newNotificationsTestApp(t)
	const userID = "user_two_apps"
	bufferMailNotificationForUser(a, userID, "sender@external.example", "for app a")

	flushMailBuffer(context.Background(), b)

	if n := countNotifications(t, b, userID); n != 0 {
		t.Fatalf("app B wrote %d notifications for app A's mail", n)
	}
	if _, held := mailBufferFor(a).Load(userID); !held {
		t.Fatal("app B's flush emptied app A's buffer")
	}
	flushMailBuffer(context.Background(), a)
	if n := countNotifications(t, a, userID); n != 1 {
		t.Fatalf("app A wrote %d notifications, want 1", n)
	}
}
