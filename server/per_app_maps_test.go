package mail

import (
	"testing"
	"time"
)

// The verify lock serializes runs per mail_domains row within one app. Two
// apps with the same row id must not wait on each other.
func TestRecordLockIsPerApp(t *testing.T) {
	a, b := newNotificationsTestApp(t), newNotificationsTestApp(t)
	unlockA := recordLock(a, "domain-1")
	defer unlockA()

	got := make(chan struct{})
	go func() {
		unlock := recordLock(b, "domain-1")
		unlock()
		close(got)
	}()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("app B waited on app A's verify lock")
	}
}
