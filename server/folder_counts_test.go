package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func countsRow(t *testing.T, env *scopeEnv, mailboxID string) *core.Record {
	t.Helper()
	row, err := env.app.FindRecordById("mail_folder_counts", folderCountsID(env.user.Id, mailboxID))
	if err != nil {
		t.Fatalf("no counts row for %s: %v", mailboxID, err)
	}
	return row
}

func TestFolderCounts_RowFollowsStateChanges(t *testing.T) {
	env := setupScopeEnv(t)
	// setupScopeEnv seeds one unread inbox thread per mailbox.
	row := countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox") != 1 || row.GetInt("inbox_total") != 1 || row.GetInt("total") != 1 {
		t.Fatalf("after seed: inbox=%d inbox_total=%d total=%d, want 1 1 1",
			row.GetInt("inbox"), row.GetInt("inbox_total"), row.GetInt("total"))
	}

	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	if err := setThreadRead(env.app, thread.Id, env.user.Id, true); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox") != 0 || row.GetInt("inbox_total") != 1 {
		t.Errorf("after read: inbox=%d inbox_total=%d, want 0 1", row.GetInt("inbox"), row.GetInt("inbox_total"))
	}

	if err := setThreadFolder(env.app, thread.Id, env.user.Id, "archive"); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox_total") != 0 || row.GetInt("archive") != 1 || row.GetInt("total") != 1 {
		t.Errorf("after archive: inbox_total=%d archive=%d total=%d, want 0 1 1",
			row.GetInt("inbox_total"), row.GetInt("archive"), row.GetInt("total"))
	}

	if err := markThreadSent(env.app, thread.Id, env.user.Id); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("sent") != 1 {
		t.Errorf("after send: sent=%d, want 1", row.GetInt("sent"))
	}

	if err := setThreadFolder(env.app, thread.Id, env.user.Id, "trash"); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("sent") != 0 || row.GetInt("trash") != 1 {
		t.Errorf("after trash: sent=%d trash=%d, want 0 1 (sent excludes trash)", row.GetInt("sent"), row.GetInt("trash"))
	}

	state := findThreadState(env.app, thread.Id, env.user.Id)
	if err := env.app.Delete(state); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("total") != 0 || row.GetInt("trash") != 0 {
		t.Errorf("after delete: total=%d trash=%d, want 0 0", row.GetInt("total"), row.GetInt("trash"))
	}
}

func TestFolderCounts_BulkArchiveLeavesCorrectRow(t *testing.T) {
	env := setupScopeEnv(t)
	threads, _ := env.app.FindCollectionByNameOrId("mail_threads")
	for i := 0; i < 25; i++ {
		thread := core.NewRecord(threads)
		thread.Set("mailbox", env.mbA.Id)
		thread.Set("subject", "bulk")
		thread.Set("latest_date", "2026-10-05 00:00:00.000Z")
		if err := env.app.Save(thread); err != nil {
			t.Fatal(err)
		}
		if err := ensureThreadState(env.app, thread.Id, env.user.Id, "inbox", false); err != nil {
			t.Fatal(err)
		}
	}
	row := countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox") != 26 {
		t.Fatalf("inbox=%d, want 26", row.GetInt("inbox"))
	}

	states, err := env.app.FindRecordsByFilter("mail_thread_state", "user = {:u} && mailbox = {:mb}", "", 0, 0,
		map[string]any{"u": env.user.Id, "mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		state.Set("folder", "archive")
		if err := env.app.Save(state); err != nil {
			t.Fatal(err)
		}
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox") != 0 || row.GetInt("archive") != 26 || row.GetInt("total") != 26 {
		t.Errorf("inbox=%d archive=%d total=%d, want 0 26 26", row.GetInt("inbox"), row.GetInt("archive"), row.GetInt("total"))
	}
}

func TestFolderCounts_OtherMailboxUntouched(t *testing.T) {
	env := setupScopeEnv(t)
	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	if err := setThreadFolder(env.app, thread.Id, env.user.Id, "trash"); err != nil {
		t.Fatal(err)
	}
	other := countsRow(t, env, env.mbB.Id)
	if other.GetInt("inbox") != 1 || other.GetInt("trash") != 0 {
		t.Errorf("mailbox B inbox=%d trash=%d, want 1 0", other.GetInt("inbox"), other.GetInt("trash"))
	}
}

// Deleting a mailbox cascades to its threads, their state rows, and its counts
// row. The state delete hooks then run with the counts row gone; they must not
// recreate it, since the new row would point at the deleted mailbox and fail
// relation validation.
func TestFolderCounts_MailboxDeleteDoesNotRecreateRow(t *testing.T) {
	env := setupScopeEnv(t)
	countsRow(t, env, env.mbA.Id)

	if err := env.app.Delete(env.mbA); err != nil {
		t.Fatal(err)
	}
	id := folderCountsID(env.user.Id, env.mbA.Id)
	if _, err := env.app.FindRecordById("mail_folder_counts", id); err == nil {
		t.Fatal("counts row for the deleted mailbox was recreated")
	}
	// The hook only logs a failure, so call the recompute directly to see the
	// error the hook would have swallowed.
	if err := recomputeFolderCounts(env.app, env.user.Id, env.mbA.Id); err != nil {
		t.Errorf("recompute after mailbox delete: %v", err)
	}
	if _, err := env.app.FindRecordById("mail_folder_counts", id); err == nil {
		t.Error("recompute created a counts row for a mailbox with no state rows")
	}
}
