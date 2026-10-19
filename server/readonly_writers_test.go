package mail

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/maildomains"
	"tinycld.org/core/readonly"
	"tinycld.org/core/readonly/readonlytest"
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
	flushMailBuffer(context.Background(), app)
	if n := countNotifications(t, app, userID); n != 0 {
		t.Fatalf("%d notifications were written while read-only", n)
	}
	if _, held := mailBuffer.Load(userID); !held {
		t.Fatal("the flush dropped the buffered mail while read-only")
	}

	readonly.Leave()
	flushMailBuffer(context.Background(), app)
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

// waitForSignal fails the test when ch does not close in time, or when done
// closes first: the writer finished without reaching its read-only wait.
func waitForSignal(t *testing.T, ch, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-done:
		t.Fatalf("%s finished without waiting for read-only mode to end", what)
	case <-time.After(5 * time.Second):
		t.Fatalf("%s never reached the read-only wait", what)
	}
}

func waitDone(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not run after read-only mode ended", what)
	}
}

// A flush that comes due while the server is read-only waits for the mode to
// end, and then sends the buffered notifications.
func TestMailBatcherFlushWaitsWhileReadOnly(t *testing.T) {
	const userID = "user_ro_flush_wait"
	t.Cleanup(func() { mailBuffer.Delete(userID) })
	app := newNotificationsTestApp(t)
	bufferMailNotificationForUser(userID, "sender@external.example", "waits for read-only")
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	probe, waiting := readonlytest.WaitProbe(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		flushMailBufferWhenWritable(probe, app)
	}()
	waitForSignal(t, waiting, done, "the mail notification flush")
	if n := countNotifications(t, app, userID); n != 0 {
		t.Fatalf("%d notifications were written while read-only", n)
	}

	readonly.Leave()
	waitDone(t, done, "the mail notification flush")
	if n := countNotifications(t, app, userID); n != 1 {
		t.Fatalf("%d notifications after read-only ended, want 1", n)
	}
}

// When the wait ends first (TailWait, or the app's terminate), the buffered
// notifications are dropped, and the drop is logged at Warn.
func TestMailBatcherFlushDroppedAndLoggedWhenReadOnlyOutlastsTheWait(t *testing.T) {
	const userID = "user_ro_flush_drop"
	t.Cleanup(func() { mailBuffer.Delete(userID) })
	app := newNotificationsTestApp(t)
	bufferMailNotificationForUser(userID, "sender@external.example", "dropped one")
	bufferMailNotificationForUser(userID, "sender@external.example", "dropped two")
	logs := readonlytest.CaptureLogs(t)
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	flushMailBufferWhenWritable(ctx, app)

	if n := countNotifications(t, app, userID); n != 0 {
		t.Fatalf("%d notifications were written while read-only", n)
	}
	if _, held := mailBuffer.Load(userID); held {
		t.Fatal("the buffer still holds the dropped mail")
	}
	r, ok := readonlytest.Find(logs(), mailNotifyDroppedMsg)
	if !ok {
		t.Fatalf("no %q log for the dropped notifications", mailNotifyDroppedMsg)
	}
	if r.Level != slog.LevelWarn {
		t.Fatalf("want Warn for dropped mail notifications, got %v", r.Level)
	}
	if r.Attrs["users"] != "1" || r.Attrs["messages"] != "2" {
		t.Fatalf("the log must count what was dropped, got %v", r.Attrs)
	}
}

// The buffer lives only in memory: at the app's terminate the batcher sends
// what it holds instead of losing it.
func TestMailBatcherFlushesAtTerminate(t *testing.T) {
	const userID = "user_flush_at_terminate"
	t.Cleanup(func() { mailBuffer.Delete(userID) })
	app := newNotificationsTestApp(t)
	bufferMailNotificationForUser(userID, "sender@external.example", "sent at terminate")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	startMailBatcher(ctx, app)

	if n := countNotifications(t, app, userID); n != 1 {
		t.Fatalf("%d notifications after the terminate, want 1", n)
	}
}

// A member backfill that starts while the server is read-only writes no row
// until the mode ends.
func TestMemberBackfillWaitsWhileReadOnly(t *testing.T) {
	env := setupScopeEnv(t)
	joiner := newUser(t, env.app, "ro-joiner@example.org")
	seedMember(t, env.app, env.mbA.Id, joiner.Id)
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	probe, waiting := readonlytest.WaitProbe(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := backfillMemberThreadState(probe, env.app, env.mbA.Id, joiner.Id); err != nil {
			t.Errorf("backfill: %v", err)
		}
	}()
	waitForSignal(t, waiting, done, "the member backfill")
	if n := stateRowCount(t, env.app, joiner.Id); n != 0 {
		t.Fatalf("the backfill wrote %d state rows while read-only", n)
	}

	readonly.Leave()
	waitDone(t, done, "the member backfill")
	if n := stateRowCount(t, env.app, joiner.Id); n == 0 {
		t.Fatal("the backfill wrote no state rows after read-only ended")
	}
}

// A member access cleanup that starts while the server is read-only deletes
// no row until the mode ends.
func TestMemberAccessCleanupWaitsWhileReadOnly(t *testing.T) {
	env := setupScopeEnv(t)
	leaver := newUser(t, env.app, "ro-leaver@example.org")
	seedMember(t, env.app, env.mbA.Id, leaver.Id)
	if err := backfillMemberThreadState(context.Background(), env.app, env.mbA.Id, leaver.Id); err != nil {
		t.Fatal(err)
	}
	rows := stateRowCount(t, env.app, leaver.Id)
	if rows == 0 {
		t.Fatal("precondition: the leaver has no state rows")
	}
	member, err := env.app.FindFirstRecordByFilter("mail_mailbox_members", "user = {:u}", map[string]any{"u": leaver.Id})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.app.Delete(member); err != nil {
		t.Fatal(err)
	}
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	probe, waiting := readonlytest.WaitProbe(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := cleanupLostMailboxAccess(probe, env.app, env.mbA.Id, leaver.Id); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	waitForSignal(t, waiting, done, "the member access cleanup")
	if n := stateRowCount(t, env.app, leaver.Id); n != rows {
		t.Fatalf("the cleanup deleted rows while read-only: %d left of %d", n, rows)
	}

	readonly.Leave()
	waitDone(t, done, "the member access cleanup")
	if n := stateRowCount(t, env.app, leaver.Id); n != 0 {
		t.Fatalf("%d state rows left after read-only ended, want 0", n)
	}
}

// When a member job's wait ends first, the job is dropped and logged at Warn.
func TestMemberJobDroppedAndLoggedWhenReadOnlyOutlastsTheWait(t *testing.T) {
	logs := readonlytest.CaptureLogs(t)
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitMemberJobWritable(ctx, "backfill", "mbx_ro", "user_ro") {
		t.Fatal("the wait reported writable while read-only")
	}
	r, ok := readonlytest.Find(logs(), memberJobDroppedMsg)
	if !ok {
		t.Fatalf("no %q log for the dropped job", memberJobDroppedMsg)
	}
	if r.Level != slog.LevelWarn {
		t.Fatalf("want Warn for a dropped member job, got %v", r.Level)
	}
	if r.Attrs["job"] != "backfill" || r.Attrs["mailbox"] != "mbx_ro" || r.Attrs["user"] != "user_ro" {
		t.Fatalf("the log must name the job, got %v", r.Attrs)
	}
}
