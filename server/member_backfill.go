package mail

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var memberBackfillLog = logging.ForPackage("mail")

const memberBackfillBatch = 500

// registerMemberBackfillHooks gives a new mailbox member a state row for every
// thread already in the mailbox. The thread list and search both read
// mail_thread_state, so without these rows a new member of a shared mailbox
// sees nothing until new mail arrives.
//
// The backfill runs in a goroutine: a mailbox can hold many thousands of
// threads, and the member insert (an admin action in the UI) must not wait for
// one save per thread. The root app is captured at registration because the
// goroutine outlives the request, and the event's app can be a finished
// transaction.
func registerMemberBackfillHooks(app core.App) {
	app.OnRecordAfterCreateSuccess("mail_mailbox_members").BindFunc(func(e *core.RecordEvent) error {
		mailboxID := e.Record.GetString("mailbox")
		userID := e.Record.GetString("user")
		go func() {
			defer func() {
				if r := recover(); r != nil {
					memberBackfillLog.Error("member backfill panicked",
						"mailbox", mailboxID, "user", userID, "panic", r)
				}
			}()
			if err := backfillMemberThreadState(app, mailboxID, userID); err != nil {
				memberBackfillLog.Error("member backfill failed",
					"mailbox", mailboxID, "user", userID, "error", err)
			}
		}()
		return e.Next()
	})
}

// backfillMemberThreadState creates the user's missing state rows for every
// thread in the mailbox. Rows that exist are left alone, so it is safe to run
// again. New rows are read: joining a mailbox must not raise thousands of
// unread threads. Each row is a record save, so realtime and the folder-count
// hook fire, and the create hook fills mailbox and latest_date.
//
// A failed row is logged and skipped so one bad thread does not stop the
// rest; the first error is returned for the caller to log.
func backfillMemberThreadState(app core.App, mailboxID, userID string) error {
	var firstErr error
	for offset := 0; ; offset += memberBackfillBatch {
		// The app can be torn down while a backfill is still running (test
		// teardown, dev DB reset); a query on a closed DB panics.
		if !appIsLive(app) {
			return firstErr
		}
		threads, err := app.FindRecordsByFilter(
			"mail_threads",
			"mailbox = {:mailbox}",
			"id",
			memberBackfillBatch,
			offset,
			map[string]any{"mailbox": mailboxID},
		)
		if err != nil {
			return fmt.Errorf("list threads of mailbox %s: %w", mailboxID, err)
		}
		for _, thread := range threads {
			if err := backfillOneThreadState(app, thread.Id, userID); err != nil {
				memberBackfillLog.Warn("member backfill skipped a thread",
					"thread", thread.Id, "user", userID, "error", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
		if len(threads) < memberBackfillBatch {
			return firstErr
		}
	}
}

func backfillOneThreadState(app core.App, threadID, userID string) error {
	if findThreadState(app, threadID, userID) != nil {
		return nil
	}
	return createBackfillState(app, threadID, userID)
}

// createBackfillState saves a read inbox row for a thread the caller found
// without one. An inbound delivery for the same thread can write the member's
// row between that find and this save; the unique (thread, user) index then
// rejects the save. That race is expected and the delivered row is correct,
// so a failed save with a row now present is success, not an error to report.
func createBackfillState(app core.App, threadID, userID string) error {
	state, err := newThreadState(app, threadID, userID)
	if err != nil {
		return err
	}
	state.Set("folder", "inbox")
	state.Set("is_read", true)
	if err := app.Save(state); err != nil {
		if findThreadState(app, threadID, userID) != nil {
			return nil
		}
		return fmt.Errorf("save state for thread %s: %w", threadID, err)
	}
	return nil
}
