package mail

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func firstThreadOf(t *testing.T, env *scopeEnv, mailboxID string) *core.Record {
	t.Helper()
	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": mailboxID})
	if err != nil {
		t.Fatal(err)
	}
	return thread
}

func stateRowCount(t *testing.T, app core.App, userID string) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_thread_state", "user = {:u}", "", 0, 0, map[string]any{"u": userID})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestBackfillMemberThreadState_CreatesReadInboxRows(t *testing.T) {
	env := setupScopeEnv(t)
	thread := firstThreadOf(t, env, env.mbA.Id)
	joiner := newUser(t, env.app, "joiner@example.org")

	if err := backfillMemberThreadState(env.app, env.mbA.Id, joiner.Id); err != nil {
		t.Fatal(err)
	}

	state := findThreadState(env.app, thread.Id, joiner.Id)
	if state == nil {
		t.Fatal("no state row for the existing thread")
	}
	if got := state.GetString("folder"); got != "inbox" {
		t.Errorf("folder = %q, want inbox", got)
	}
	if !state.GetBool("is_read") {
		t.Error("is_read = false; a joiner must not get the mailbox's history as unread")
	}
	if got := state.GetString("mailbox"); got != env.mbA.Id {
		t.Errorf("mailbox = %q, want %q", got, env.mbA.Id)
	}
	if got, want := state.GetString("latest_date"), thread.GetString("latest_date"); got != want {
		t.Errorf("latest_date = %q, want the thread's %q", got, want)
	}

	counts, err := env.app.FindRecordById("mail_folder_counts", folderCountsID(joiner.Id, env.mbA.Id))
	if err != nil {
		t.Fatalf("no counts row for the joiner: %v", err)
	}
	if got := counts.GetInt("inbox"); got != 0 {
		t.Errorf("inbox unread = %d, want 0", got)
	}
	if got := counts.GetInt("inbox_total"); got != 1 {
		t.Errorf("inbox_total = %d, want 1", got)
	}
}

func TestBackfillMemberThreadState_IsIdempotent(t *testing.T) {
	env := setupScopeEnv(t)
	joiner := newUser(t, env.app, "twice@example.org")

	for range 2 {
		if err := backfillMemberThreadState(env.app, env.mbA.Id, joiner.Id); err != nil {
			t.Fatal(err)
		}
	}

	if got := stateRowCount(t, env.app, joiner.Id); got != 1 {
		t.Errorf("joiner has %d state rows after two backfills, want 1", got)
	}
}

func TestMemberCreate_BackfillsThreadState(t *testing.T) {
	env := setupScopeEnv(t)
	registerMemberBackfillHooks(env.app)
	thread := firstThreadOf(t, env, env.mbA.Id)
	other := newUser(t, env.app, "hooked@example.org")

	seedMember(t, env.app, env.mbA.Id, other.Id)

	// The backfill runs in a goroutine, so poll for its row.
	deadline := time.Now().Add(2 * time.Second)
	for findThreadState(env.app, thread.Id, other.Id) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if findThreadState(env.app, thread.Id, other.Id) == nil {
		t.Fatal("no state row 2s after the member was added; the backfill hook did not run")
	}
}
