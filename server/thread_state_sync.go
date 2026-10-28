package mail

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var threadStateSyncLog = logging.ForPackage("mail")

// registerThreadStateSyncHooks keeps mail_thread_state.mailbox, .latest_date
// and .address_keys equal to the thread's. The thread list is one live query on
// mail_thread_state, and a live query cannot sort or filter through a
// relation, so the thread's values live on the state row too. Copied on
// create for every writer (Go, the seed, tests), and propagated on every
// thread save that changes latest_date or address_keys. Record saves, not
// SQL, so PocketBase emits the realtime events the list depends on.
//
// All three columns are server-owned. The state updateRule lets a user write their
// own rows, so a client could otherwise point mailbox at a mailbox they are
// not in (creating a counts row for it), pin latest_date to float a thread,
// or add a thread to an address view. Create always takes the thread's
// values; an API update keeps the stored ones. propagateThreadFields is an
// app.Save, which the ...Request hook does not see, so propagation still
// writes through.
func registerThreadStateSyncHooks(app core.App) {
	app.OnRecordCreate("mail_thread_state").BindFunc(func(e *core.RecordEvent) error {
		if err := fillThreadStateFromThread(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordUpdateRequest("mail_thread_state").BindFunc(func(e *core.RecordRequestEvent) error {
		// The request loaded the row before it waited to write, and it saves
		// every column. A server write in between (an IMAP APPEND marking the
		// thread sent) would be overwritten with the loaded values, so the
		// stored row is read again inside the transaction that saves it: all
		// writes share one connection, so nothing commits between this read
		// and the save.
		return e.App.RunInTransaction(func(txApp core.App) error {
			stored, err := txApp.FindRecordById("mail_thread_state", e.Record.Id)
			if err != nil {
				return err
			}
			// Restore rather than refuse: a client that saves an edited state
			// row can send the whole record back, these columns included.
			for _, field := range threadOwnedStateFields {
				e.Record.Set(field, stored.GetString(field))
			}
			// is_sent only ever becomes true (a send, or filing the thread
			// under Sent), so a stored true is kept.
			if stored.GetBool("is_sent") {
				e.Record.Set("is_sent", true)
			}
			originalApp := e.App
			e.App = txApp
			defer func() { e.App = originalApp }()
			return e.Next()
		})
	})
	app.OnRecordAfterUpdateSuccess("mail_threads").BindFunc(func(e *core.RecordEvent) error {
		original := e.Record.Original()
		changed := map[string]string{}
		for _, field := range propagatedThreadFields {
			if after := e.Record.GetString(field); after != original.GetString(field) {
				changed[field] = after
			}
		}
		if len(changed) == 0 {
			return e.Next()
		}
		if err := propagateThreadFields(e.App, e.Record.Id, changed); err != nil {
			threadStateSyncLog.Warn("propagateThreadFields failed", "thread", e.Record.Id, "error", err)
		}
		return e.Next()
	})
}

// threadOwnedStateFields are the state columns copied from the thread. The
// mailbox never changes after create, so only the others are propagated.
var (
	threadOwnedStateFields = []string{"mailbox", "latest_date", "address_keys"}
	propagatedThreadFields = []string{"latest_date", "address_keys"}
)

// fillThreadStateFromThread copies the thread-owned columns from the thread
// onto a new state row, replacing any value the writer supplied.
func fillThreadStateFromThread(app core.App, state *core.Record) error {
	thread, err := app.FindRecordById("mail_threads", state.GetString("thread"))
	if err != nil {
		return fmt.Errorf("thread for state row: %w", err)
	}
	for _, field := range threadOwnedStateFields {
		state.Set(field, thread.GetString(field))
	}
	return nil
}

// propagateThreadFields writes the thread's changed values to each of its
// state rows, saving only rows that do not already carry them.
func propagateThreadFields(app core.App, threadID string, values map[string]string) error {
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
		stale := false
		for field, value := range values {
			if state.GetString(field) != value {
				state.Set(field, value)
				stale = true
			}
		}
		if !stale {
			continue
		}
		if err := app.Save(state); err != nil {
			return err
		}
	}
	return nil
}
