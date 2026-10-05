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
// relation, so the thread's values live on the state row too. Filled on
// create for every writer (Go, the seed, tests), and propagated on every
// thread save that changes latest_date. Record saves, not SQL, so PocketBase
// emits the realtime events the list depends on.
func registerThreadStateSyncHooks(app core.App) {
	app.OnRecordCreate("mail_thread_state").BindFunc(func(e *core.RecordEvent) error {
		if err := fillThreadStateFromThread(e.App, e.Record); err != nil {
			return err
		}
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
// onto a state row that lacks them.
func fillThreadStateFromThread(app core.App, state *core.Record) error {
	if state.GetString("mailbox") != "" && state.GetString("latest_date") != "" {
		return nil
	}
	thread, err := app.FindRecordById("mail_threads", state.GetString("thread"))
	if err != nil {
		return fmt.Errorf("thread for state row: %w", err)
	}
	if state.GetString("mailbox") == "" {
		state.Set("mailbox", thread.GetString("mailbox"))
	}
	if state.GetString("latest_date") == "" {
		state.Set("latest_date", thread.GetString("latest_date"))
	}
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
