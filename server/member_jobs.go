package mail

import (
	"sync"

	"tinycld.org/core/logging"
)

var memberJobLog = logging.ForPackage("mail")

// memberJobConcurrency is how many member jobs (backfill and access cleanup)
// run at once, server-wide. A group grant gives one derived row per group
// member, so adding or removing a grant, or deleting a group, starts one job
// per person at the same moment. Each job loads batches of up to 500 records,
// and the database has one writer, so running more at once only adds memory.
const memberJobConcurrency = 2

var memberJobs = newJobLimiter(memberJobConcurrency)

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
// A job that waits holds no records, and it is never dropped.
type jobLimiter struct {
	slots chan struct{}

	mu     sync.Mutex
	queued map[string][]func()
}

func newJobLimiter(n int) *jobLimiter {
	return &jobLimiter{slots: make(chan struct{}, n), queued: map[string][]func(){}}
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
// starts one goroutine that runs that key's jobs in turn, each in its own
// slot, and stops when the key has no more jobs.
func (l *jobLimiter) runKeyed(key string, job func()) {
	l.mu.Lock()
	pending, busy := l.queued[key]
	l.queued[key] = append(pending, job)
	l.mu.Unlock()
	if busy {
		return
	}
	go l.drain(key)
}

func (l *jobLimiter) drain(key string) {
	for {
		l.mu.Lock()
		pending := l.queued[key]
		if len(pending) == 0 {
			delete(l.queued, key)
			l.mu.Unlock()
			return
		}
		job := pending[0]
		l.queued[key] = pending[1:]
		l.mu.Unlock()

		l.acquire()
		func() {
			defer l.release()
			job()
		}()
	}
}

// runMemberJob queues one member job for the (mailbox, user) key on the
// limiter. A panic or an error is logged, and the slot is given back either
// way, so one bad job does not stop the queue.
func runMemberJob(l *jobLimiter, kind, mailboxID, userID string, job func() error) {
	l.runKeyed(mailboxID+"/"+userID, func() {
		defer func() {
			if r := recover(); r != nil {
				memberJobLog.Error("member "+kind+" panicked",
					"mailbox", mailboxID, "user", userID, "panic", r)
			}
		}()
		if err := job(); err != nil {
			memberJobLog.Error("member "+kind+" failed",
				"mailbox", mailboxID, "user", userID, "error", err)
		}
	})
}
