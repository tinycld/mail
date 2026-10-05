package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The thread list is one live query on mail_thread_state ordered by
// latest_date and scoped by mailbox. Both are copies of the thread's columns;
// these tests pin that every writer gets them filled and that a thread save
// keeps them current.

func stateFor(t *testing.T, env *scopeEnv, threadID string) *core.Record {
	t.Helper()
	state := findThreadState(env.app, threadID, env.user.Id)
	if state == nil {
		t.Fatalf("no state row for thread %s", threadID)
	}
	return state
}

func TestThreadStateCreate_FillsMailboxAndLatestDate(t *testing.T) {
	env := setupScopeEnv(t)
	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	thread.Set("latest_date", "2026-10-05 10:00:00.000Z")
	if err := env.app.Save(thread); err != nil {
		t.Fatal(err)
	}

	// setupScopeEnv seeded the state row with neither column set; the create
	// hook must have filled both from the thread.
	state := stateFor(t, env, thread.Id)
	if got := state.GetString("mailbox"); got != env.mbA.Id {
		t.Errorf("mailbox = %q, want %q", got, env.mbA.Id)
	}
	if got := state.GetString("latest_date"); got != "2026-10-05 10:00:00.000Z" {
		t.Errorf("latest_date = %q, want the thread's", got)
	}
}

func TestThreadLatestDateChange_PropagatesToEveryStateRow(t *testing.T) {
	env := setupScopeEnv(t)
	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	// A second member's row on the same thread.
	other := newUser(t, env.app, "second@example.org")
	seedMember(t, env.app, env.mbA.Id, other.Id)
	if err := setThreadFolder(env.app, thread.Id, other.Id, "inbox"); err != nil {
		t.Fatal(err)
	}

	thread.Set("latest_date", "2026-10-06 09:30:00.000Z")
	if err := env.app.Save(thread); err != nil {
		t.Fatal(err)
	}

	for _, userID := range []string{env.user.Id, other.Id} {
		state := findThreadState(env.app, thread.Id, userID)
		if state == nil {
			t.Fatalf("no state for %s", userID)
		}
		if got := state.GetString("latest_date"); got != "2026-10-06 09:30:00.000Z" {
			t.Errorf("user %s latest_date = %q, want propagated", userID, got)
		}
	}
}

func TestThreadSaveWithoutLatestDateChange_DoesNotTouchStateRows(t *testing.T) {
	env := setupScopeEnv(t)
	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	before := stateFor(t, env, thread.Id).GetString("updated")

	thread.Set("snippet", "only the snippet changed")
	if err := env.app.Save(thread); err != nil {
		t.Fatal(err)
	}

	if after := stateFor(t, env, thread.Id).GetString("updated"); after != before {
		t.Errorf("state row was saved (updated %q -> %q) although latest_date did not change", before, after)
	}
}

// newUser creates a users row for a second mailbox member.
func newUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail(email)
	user.SetPassword("Password123!")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	return user
}
