package mail

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var threadStateSyncLog = logging.ForPackage("mail")

// registerThreadStateSyncHooks keeps mail_thread_state.mailbox and
// .latest_date equal to the thread's. The thread list is one live query on
// mail_thread_state, and a live query cannot sort or filter through a
// relation, so the thread's values live on the state row too. Copied on
// create for every writer (Go, the seed, tests), and propagated on every
// thread save that changes latest_date. Record saves, not SQL, so PocketBase
// emits the realtime events the list depends on.
//
// Both columns are server-owned. The state updateRule lets a user write their
// own rows, so a client could otherwise point mailbox at a mailbox they are
// not in (creating a counts row for it) or pin latest_date to float a thread.
// Create always takes the thread's values; an API update keeps the stored
// ones. propagateLatestDate is an app.Save, which the ...Request hook does not
// see, so propagation still writes through.
func registerThreadStateSyncHooks(app core.App) {
	app.OnRecordCreate("mail_thread_state").BindFunc(func(e *core.RecordEvent) error {
		if err := fillThreadStateFromThread(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordUpdateRequest("mail_thread_state").BindFunc(func(e *core.RecordRequestEvent) error {
		// Restore rather than refuse: a client that saves an edited state row
		// can send the whole record back, both columns included.
		original := e.Record.Original()
		e.Record.Set("mailbox", original.GetString("mailbox"))
		e.Record.Set("latest_date", original.GetString("latest_date"))
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess("mail_threads").BindFunc(func(e *core.RecordEvent) error {
		before := e.Record.Original().GetString("latest_date")
		after := e.Record.GetString("latest_date")
		if before == after {
			return e.Next()
		}
		if err := propagateLatestDate(e.App, e.Record.Id, after); err != nil {
			threadStateSyncLog.Warn("propagateLatestDate failed", "thread", e.Record.Id, "error", err)
		}
		return e.Next()
	})
}

// fillThreadStateFromThread copies mailbox and latest_date from the thread
// onto a new state row, replacing any value the writer supplied.
func fillThreadStateFromThread(app core.App, state *core.Record) error {
	thread, err := app.FindRecordById("mail_threads", state.GetString("thread"))
	if err != nil {
		return fmt.Errorf("thread for state row: %w", err)
	}
	state.Set("mailbox", thread.GetString("mailbox"))
	state.Set("latest_date", thread.GetString("latest_date"))
	return nil
}

// propagateLatestDate writes the thread's new latest_date to each of its
// state rows, skipping rows that already carry it.
func propagateLatestDate(app core.App, threadID, latestDate string) error {
	states, err := app.FindRecordsByFilter(
		"mail_thread_state",
		"thread = {:thread}",
		"",
		0,
		0,
		map[string]any{"thread": threadID},
	)
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.GetString("latest_date") == latestDate {
			continue
		}
		state.Set("latest_date", latestDate)
		if err := app.Save(state); err != nil {
			return err
		}
	}
	return nil
}
