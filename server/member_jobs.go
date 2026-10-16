package mail

import "tinycld.org/core/logging"

var memberJobLog = logging.ForPackage("mail")

// memberJobConcurrency is how many member jobs (backfill and access cleanup)
// run at once, server-wide. A group grant gives one derived row per group
// member, so adding or removing a grant, or deleting a group, starts one job
// per person at the same moment. Each job loads batches of up to 500 records,
// and the database has one writer, so running more at once only adds memory.
const memberJobConcurrency = 2

var memberJobs = newJobLimiter(memberJobConcurrency)

// jobLimiter is a counting semaphore: a job takes a slot before its first
// query and gives it back when it ends. A job that finds no free slot waits
// in its goroutine; it holds no records while it waits, and it is never
// dropped.
type jobLimiter struct {
	slots chan struct{}
}

func newJobLimiter(n int) *jobLimiter {
	return &jobLimiter{slots: make(chan struct{}, n)}
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

// run starts job in a goroutine that waits for a slot first.
func (l *jobLimiter) run(job func()) {
	go func() {
		l.acquire()
		defer l.release()
		job()
	}()
}

// runMemberJob queues one member job on the limiter. A panic or an error is
// logged, and the slot is given back either way, so one bad job does not stop
// the queue.
func runMemberJob(l *jobLimiter, kind, mailboxID, userID string, job func() error) {
	l.run(func() {
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
