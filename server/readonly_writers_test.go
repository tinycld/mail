package mail

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/maildomains"
	"tinycld.org/core/readonly"
)

// While the server is read-only it is waiting to be replaced, and a write it
// makes now is lost if the next build rolls back. The HTTP middleware cannot
// refuse a write that no request made, so each background writer checks the
// mode itself.

// The hourly domain re-check skips its run: the next run picks the rows up.
func TestReverifyUnconfirmedDomainsSkipsWhileReadOnly(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(&stubRegistrar{rec: &maildomains.DomainRecords{Domain: "acme.com", ID: 1}})
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{{Host: "inbound.postmarkapp.com.", Pref: 10}}, nil
	})
	t.Cleanup(readonly.Leave)

	app := newReverifyTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "postmark")
	saveSystemSetting(t, app, "mail.postmark_server_token", "tok")
	record := newReverifyTestRecord(t, app)

	readonly.Enter()
	reverifyUnconfirmedDomains(context.Background(), app)
	if got := lastCheckedAt(t, app, record.Id); got != "" {
		t.Fatalf("last_checked_at = %q: the re-check wrote while read-only", got)
	}

	readonly.Leave()
	reverifyUnconfirmedDomains(context.Background(), app)
	if lastCheckedAt(t, app, record.Id) == "" {
		t.Fatal("last_checked_at is empty: the re-check did not run after read-only ended")
	}
}

func lastCheckedAt(t *testing.T, app core.App, id string) string {
	t.Helper()
	saved, err := app.FindRecordById("mail_domains", id)
	if err != nil {
		t.Fatalf("reload record: %v", err)
	}
	return saved.GetString("last_checked_at")
}

// The notification batcher keeps its buffer while read-only, so the
// notifications go out at the first flush after the mode ends.
func TestFlushMailBufferHoldsWhileReadOnly(t *testing.T) {
	const userID = "user_ro_batcher"
	t.Cleanup(func() { mailBuffer.Delete(userID) })
	t.Cleanup(readonly.Leave)
	app := newNotificationsTestApp(t)

	bufferMailNotificationForUser(userID, "sender@external.example", "held while read-only")

	readonly.Enter()
	flushMailBuffer(app)
	if n := countNotifications(t, app, userID); n != 0 {
		t.Fatalf("%d notifications were written while read-only", n)
	}
	if _, held := mailBuffer.Load(userID); !held {
		t.Fatal("the flush dropped the buffered mail while read-only")
	}

	readonly.Leave()
	flushMailBuffer(app)
	if n := countNotifications(t, app, userID); n != 1 {
		t.Fatalf("%d notifications after read-only ended, want 1", n)
	}
}

func newNotificationsTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("test app: %v", err)
	}
	t.Cleanup(app.Cleanup)
	col := core.NewBaseCollection("notifications")
	for _, name := range []string{"user", "type", "package", "title", "body", "url"} {
		col.Fields.Add(&core.TextField{Name: name})
	}
	col.Fields.Add(&core.JSONField{Name: "metadata"})
	col.Fields.Add(&core.BoolField{Name: "read"})
	col.Fields.Add(&core.BoolField{Name: "dismissed"})
	if err := app.Save(col); err != nil {
		t.Fatalf("save notifications: %v", err)
	}
	return app
}

func countNotifications(t *testing.T, app core.App, userID string) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter("notifications", "user = {:u}", "", 0, 0, map[string]any{"u": userID})
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	return len(rows)
}

// The IMAP fetcher waits before it stores anything, and leaves the message
// unseen on the remote server, so it is pulled once the mode ends.
func TestIMAPFetcherWaitsWhileReadOnly(t *testing.T) {
	const subject = "fetched after read-only"
	app, mgr, cfg := setupIMAPFetchTest(t, subject)
	t.Cleanup(readonly.Leave)
	stored := func() int {
		msgs, _ := app.FindRecordsByFilter("mail_messages", "subject = {:s}", "", 10, 0, map[string]any{"s": subject})
		return len(msgs)
	}

	readonly.Enter()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := mgr.tick(ctx, cfg); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("tick while read-only = %v, want it to wait until its context ends", err)
	}
	if n := stored(); n != 0 {
		t.Fatalf("%d messages were stored while read-only", n)
	}

	done := make(chan error, 1)
	go func() { done <- mgr.tick(context.Background(), cfg) }()
	readonly.Leave()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tick after read-only: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the fetcher still waits after read-only ended")
	}
	if n := stored(); n != 1 {
		t.Fatalf("%d messages stored after read-only ended, want 1", n)
	}
}

// Thumbnails generated for a message that arrived just before the mode began
// wait to be saved until it ends.
func TestSaveWhenWritableWaitsWhileReadOnly(t *testing.T) {
	app := newNotificationsTestApp(t)
	t.Cleanup(readonly.Leave)
	col, err := app.FindCollectionByNameOrId("notifications")
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(col)
	record.Set("title", "thumbnail write")

	readonly.Enter()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := saveWhenWritable(ctx, app, record); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("saveWhenWritable while read-only = %v, want it to wait until its context ends", err)
	}
	if n, _ := app.CountRecords("notifications"); n != 0 {
		t.Fatalf("%d rows were saved while read-only", n)
	}

	done := make(chan error, 1)
	go func() { done <- saveWhenWritable(context.Background(), app, record) }()
	readonly.Leave()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("saveWhenWritable after read-only: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("saveWhenWritable still waits after read-only ended")
	}
	if n, _ := app.CountRecords("notifications"); n != 1 {
		t.Fatalf("%d rows saved after read-only ended, want 1", n)
	}
}
