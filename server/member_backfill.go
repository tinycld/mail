package mail

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

const memberBackfillBatch = 500

// registerMemberBackfillHooks gives a new mailbox member a state row for every
// thread already in the mailbox. The thread list and search both read
// mail_thread_state, so without these rows a new member of a shared mailbox
// sees nothing until new mail arrives.
//
// The backfill runs as a member job (member_jobs.go): a mailbox can hold many
// thousands of threads, and the member insert (an admin action in the UI) must
// not wait for one save per thread. The root app is captured at registration
// because the job outlives the request, and the event's app can be a finished
// transaction.
func registerMemberBackfillHooks(app core.App) {
	memberJobs := memberJobsFor(app)
	app.OnRecordAfterCreateSuccess("mail_mailbox_members").BindFunc(func(e *core.RecordEvent) error {
		mailboxID := e.Record.GetString("mailbox")
		userID := e.Record.GetString("user")
		runMemberJob(memberJobs, "backfill", mailboxID, userID, func(ctx context.Context) error {
			return backfillMemberThreadState(ctx, app, mailboxID, userID)
		})
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
// rest; the first error is returned for the caller to log. It stops between
// batches once ctx ends (the app's terminate), and waits for read-only mode
// to end before each batch (waitMemberJobWritable).
func backfillMemberThreadState(ctx context.Context, app core.App, mailboxID, userID string) error {
	// A group grant row has no user: its people get their own derived rows,
	// and each of those starts its own backfill.
	if userID == "" {
		return nil
	}
	var firstErr error
	for offset := 0; ; offset += memberBackfillBatch {
		if ctx.Err() != nil {
			return firstErr
		}
		if !waitMemberJobWritable(ctx, app, "backfill", mailboxID, userID) {
			return firstErr
		}
		// The job can run after the person lost access again, and the
		// cleanup for that loss can have run already. Rows written now would
		// stay with nobody to remove them.
		has, err := userHasMailboxRow(app, mailboxID, userID)
		if err != nil {
			return fmt.Errorf("member rows of user %s on mailbox %s: %w", userID, mailboxID, err)
		}
		if !has {
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
				memberJobLog.Warn("member backfill skipped a thread",
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
