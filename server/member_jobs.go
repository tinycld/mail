package mail

import (
	"context"
	"sync"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/background"
	"tinycld.org/core/logging"
	"tinycld.org/core/readonly"
)

var memberJobLog = logging.ForPackage("mail")

// memberJobConcurrency is how many member jobs (backfill and access cleanup)
// run at once, per app. A group grant gives one derived row per group
// member, so adding or removing a grant, or deleting a group, starts one job
// per person at the same moment. Each job loads batches of up to 500 records,
// and the database has one writer, so running more at once only adds memory.
const memberJobConcurrency = 2

// memberJobsStoreKey holds an app's member-job limiter in its store. The
// backfill and the cleanup hooks must share one limiter: it orders their jobs
// for one person.
const memberJobsStoreKey = "mail.memberJobs"

// memberJobsFor returns app's member-job limiter, made on the first call.
// The runner is looked up first: GetOrSet holds the store's lock while it
// makes the value, so a store read inside it would deadlock.
func memberJobsFor(app core.App) *jobLimiter {
	jobs := background.For(app, "mail")
	v := app.Store().GetOrSet(memberJobsStoreKey, func() any {
		return newJobLimiter(jobs, memberJobConcurrency)
	})
	return v.(*jobLimiter)
}

// jobLimiter runs member jobs. Two rules apply:
//   - A counting semaphore bounds the jobs that run at once. A job takes a slot
//     before its first query and gives it back when it ends.
//   - Jobs for one (mailbox, user) key run one at a time, in the order they
//     were queued. A cleanup lists a person's state rows and then deletes
//     them; if the person gets access again in that time, a backfill beside
//     it would skip each thread whose old row still exists, and the cleanup
//     would then delete those rows. Run after the cleanup, the backfill finds
//     every row gone and writes them all again.
//
// A job that waits holds no records. It is dropped only when the app
// terminates: each key's queue runs as a background job, which the terminate
// cancels and waits for.
type jobLimiter struct {
	jobs  *background.Jobs
	slots chan struct{}

	mu     sync.Mutex
	queued map[string][]func(ctx context.Context)
}

func newJobLimiter(jobs *background.Jobs, n int) *jobLimiter {
	return &jobLimiter{
		jobs:   jobs,
		slots:  make(chan struct{}, n),
		queued: map[string][]func(ctx context.Context){},
	}
}

func (l *jobLimiter) acquire() { l.slots <- struct{}{} }

func (l *jobLimiter) tryAcquire() bool {
	select {
	case l.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *jobLimiter) release() { <-l.slots }

// runKeyed queues job behind the other jobs for key. The first job for a key
// starts one background job that runs that key's jobs in turn, each in its
// own slot, and stops when the key has no more jobs.
func (l *jobLimiter) runKeyed(key string, job func(ctx context.Context)) {
	l.mu.Lock()
	pending, busy := l.queued[key]
	l.queued[key] = append(pending, job)
	l.mu.Unlock()
	if busy {
		return
	}
	started := l.jobs.Run("mail member jobs", func(ctx context.Context) {
		l.drain(ctx, key)
	})
	if !started {
		// The app is terminating; without this the key would stay busy
		// with nothing to drain it.
		l.mu.Lock()
		delete(l.queued, key)
		l.mu.Unlock()
	}
}

// drain runs key's jobs until none are left, or until ctx ends: then the
// jobs still queued are dropped, because the database they need is about to
// close.
func (l *jobLimiter) drain(ctx context.Context, key string) {
	for {
		l.mu.Lock()
		pending := l.queued[key]
		if len(pending) == 0 || ctx.Err() != nil {
			delete(l.queued, key)
			l.mu.Unlock()
			if len(pending) > 0 {
				memberJobLog.Warn("member jobs dropped: the app is terminating",
					"key", key, "jobs", len(pending))
			}
			return
		}
		job := pending[0]
		l.queued[key] = pending[1:]
		l.mu.Unlock()

		l.acquire()
		func() {
			defer l.release()
			job(ctx)
		}()
	}
}

// runMemberJob queues one member job for the (mailbox, user) key on the
// limiter. A panic or an error is logged, and the slot is given back either
// way, so one bad job does not stop the queue. The background runner's own
// recover would end the whole queue for the key.
func runMemberJob(l *jobLimiter, kind, mailboxID, userID string, job func(ctx context.Context) error) {
	l.runKeyed(mailboxID+"/"+userID, func(ctx context.Context) {
		defer func() {
			if r := recover(); r != nil {
				memberJobLog.Error("member "+kind+" panicked",
					"mailbox", mailboxID, "user", userID, "panic", r)
			}
		}()
		if err := job(ctx); err != nil {
			memberJobLog.Error("member "+kind+" failed",
				"mailbox", mailboxID, "user", userID, "error", err)
		}
	})
}

const memberJobDroppedMsg = "member job dropped: the server stayed read-only past the wait"

// waitMemberJobWritable waits for the server to leave read-only mode before a
// member job writes its next batch. A job can start just before the mode
// begins, or run into it, and a row it writes while the mode is on is lost if
// the next build rolls back. The wait is bounded by readonly.TailWait and by
// ctx, which ends when the app terminates. When the wait ends first, the job
// is dropped and logged, and this returns false.
func waitMemberJobWritable(ctx context.Context, kind, mailboxID, userID string) bool {
	waitCtx, cancel := context.WithTimeout(ctx, readonly.TailWait)
	defer cancel()
	if err := readonly.WaitInactive(waitCtx); err != nil {
		memberJobLog.Warn(memberJobDroppedMsg, "job", kind, "mailbox", mailboxID, "user", userID, "err", err)
		return false
	}
	return true
}
