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
	wantLatestDate := thread.GetString("latest_date")
	if wantLatestDate == "" {
		t.Fatal("seeded thread has no latest_date to copy")
	}

	// setupScopeEnv seeded the state row with neither column set, before this
	// test touches the thread at all — so a value on the state row can only
	// have come from the create hook's fill, not from update propagation.
	state := stateFor(t, env, thread.Id)
	if got := state.GetString("mailbox"); got != env.mbA.Id {
		t.Errorf("mailbox = %q, want %q", got, env.mbA.Id)
	}
	if got := state.GetString("latest_date"); got != wantLatestDate {
		t.Errorf("latest_date = %q, want the thread's %q", got, wantLatestDate)
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

// A client-supplied mailbox or latest_date on create is ignored: a foreign
// mailbox would create a counts row for a mailbox the user is not in, and a
// pinned latest_date would float the thread.
func TestThreadStateCreate_IgnoresSuppliedMailboxAndLatestDate(t *testing.T) {
	env := setupScopeEnv(t)
	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	other := newUser(t, env.app, "second@example.org")
	seedMember(t, env.app, env.mbA.Id, other.Id)

	states, err := env.app.FindCollectionByNameOrId("mail_thread_state")
	if err != nil {
		t.Fatal(err)
	}
	state := core.NewRecord(states)
	state.Set("thread", thread.Id)
	state.Set("user", other.Id)
	state.Set("folder", "inbox")
	state.Set("mailbox", env.mbB.Id)
	state.Set("latest_date", "2099-01-01 00:00:00.000Z")
	if err := env.app.Save(state); err != nil {
		t.Fatal(err)
	}

	if got := state.GetString("mailbox"); got != env.mbA.Id {
		t.Errorf("mailbox = %q, want the thread's %q", got, env.mbA.Id)
	}
	if got, want := state.GetString("latest_date"), thread.GetString("latest_date"); got != want {
		t.Errorf("latest_date = %q, want the thread's %q", got, want)
	}
}

// An API update cannot change either column; the stored values stay.
func TestThreadStateUpdateRequest_KeepsMailboxAndLatestDate(t *testing.T) {
	env := setupScopeEnv(t)
	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	// A real PATCH loads the stored row, so Original() holds the stored
	// values; mutating an already-saved in-memory record would not.
	fresh := stateFor(t, env, thread.Id)
	wantLatestDate := fresh.GetString("latest_date")
	fresh.Set("mailbox", env.mbB.Id)
	fresh.Set("latest_date", "2099-01-01 00:00:00.000Z")
	fresh.Set("is_starred", true)

	states, err := env.app.FindCollectionByNameOrId("mail_thread_state")
	if err != nil {
		t.Fatal(err)
	}
	e := &core.RecordRequestEvent{
		RequestEvent: &core.RequestEvent{Auth: env.user, App: env.app},
		Record:       fresh,
	}
	// Tags() reads the embedded collection; without it the tagged hook skips
	// the handler and the test passes for the wrong reason.
	e.Collection = states
	if err := env.app.OnRecordUpdateRequest("mail_thread_state").Trigger(
		e,
		func(e *core.RecordRequestEvent) error { return nil },
	); err != nil {
		t.Fatalf("trigger update hook: %v", err)
	}

	if got := fresh.GetString("mailbox"); got != env.mbA.Id {
		t.Errorf("mailbox = %q, want it kept at %q", got, env.mbA.Id)
	}
	if got := fresh.GetString("latest_date"); got != wantLatestDate {
		t.Errorf("latest_date = %q, want it kept at %q", got, wantLatestDate)
	}
	if !fresh.GetBool("is_starred") {
		t.Error("the guard dropped an unrelated field the user may change")
	}
}
