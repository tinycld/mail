package mail

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/background"
	"tinycld.org/core/readonly/readonlytest"
)

// newTestJobLimiter is a limiter on a test app's mail runner. Cleaning up
// the app terminates it, which waits for the limiter's jobs.
func newTestJobLimiter(t *testing.T, n int) (*jobLimiter, *tests.TestApp) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)
	return newJobLimiter(background.For(app, "mail"), n), app
}

func TestJobLimiter_TryAcquireStopsAtLimit(t *testing.T) {
	t.Parallel()
	l, _ := newTestJobLimiter(t, 2)
	if !l.tryAcquire() || !l.tryAcquire() {
		t.Fatal("tryAcquire failed below the limit")
	}
	if l.tryAcquire() {
		t.Fatal("tryAcquire took a third slot of two")
	}
	l.release()
	if !l.tryAcquire() {
		t.Fatal("tryAcquire failed after a release")
	}
}

// Every job runs, and no more than the limit run at once. The first two jobs
// hold their slots until the gate opens, so the test sees the limiter full.
func TestJobLimiter_RunsEveryJobWithinLimit(t *testing.T) {
	t.Parallel()
	l, _ := newTestJobLimiter(t, 2)
	const jobs = 6
	var active, peak, ran atomic.Int32
	started := make(chan struct{}, jobs)
	gate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(jobs)
	for i := range jobs {
		l.runKeyed(fmt.Sprintf("key-%d", i), func(context.Context) {
			defer wg.Done()
			n := active.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			started <- struct{}{}
			<-gate
			ran.Add(1)
			active.Add(-1)
		})
	}
	<-started
	<-started
	if l.tryAcquire() {
		t.Fatal("a slot is free while two jobs run")
	}
	select {
	case <-started:
		t.Fatal("a third job started while two held the slots")
	default:
	}
	close(gate)
	wg.Wait()
	if got := peak.Load(); got != 2 {
		t.Errorf("peak concurrency = %d, want 2", got)
	}
	if got := ran.Load(); got != jobs {
		t.Errorf("%d jobs ran, want %d", got, jobs)
	}
}

// A job that panics gives its slot back, so the queue does not stall.
func TestJobLimiter_PanicReleasesSlot(t *testing.T) {
	l, _ := newTestJobLimiter(t, 1)
	done := make(chan struct{})
	runMemberJob(l, "test", "mb", "user", func(context.Context) error { panic("boom") })
	runMemberJob(l, "test", "mb", "user", func(context.Context) error { close(done); return nil })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the job after a panic did not run in 2s; the panic kept its slot")
	}
}

// Jobs for one (mailbox, user) key run one at a time, in the order they were
// queued. A job for another key does not wait for them.
func TestMemberJobs_SameKeyRunsInOrder(t *testing.T) {
	l, _ := newTestJobLimiter(t, 2)
	firstStarted := make(chan struct{})
	gate := make(chan struct{})
	secondStarted := make(chan struct{})
	otherRan := make(chan struct{})
	var order []string
	var mu sync.Mutex
	record := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, s)
	}

	runMemberJob(l, "cleanup", "mb", "ann", func(context.Context) error {
		close(firstStarted)
		<-gate
		record("cleanup")
		return nil
	})
	<-firstStarted
	runMemberJob(l, "backfill", "mb", "ann", func(context.Context) error {
		record("backfill")
		close(secondStarted)
		return nil
	})
	runMemberJob(l, "backfill", "mb", "bob", func(context.Context) error {
		close(otherRan)
		return nil
	})

	select {
	case <-otherRan:
	case <-time.After(2 * time.Second):
		t.Fatal("a job for another person did not run in 2s while the first job held its key")
	}
	select {
	case <-secondStarted:
		t.Fatal("a second job for the same person started while the first one ran")
	case <-time.After(100 * time.Millisecond):
	}
	close(gate)
	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("the second job for the same person did not run in 2s after the first one ended")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "cleanup" || order[1] != "backfill" {
		t.Errorf("jobs ran in order %v, want [cleanup backfill]", order)
	}
}

// The app's terminate waits for the member job that runs, and drops the ones
// still queued behind it: the database they need is about to close.
func TestMemberJobs_TerminateWaitsAndDropsQueued(t *testing.T) {
	logs := readonlytest.CaptureLogs(t)
	l, app := newTestJobLimiter(t, 1)

	started := make(chan struct{})
	release := make(chan struct{})
	var queuedRan atomic.Bool
	runMemberJob(l, "cleanup", "mb", "ann", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		<-release
		return nil
	})
	<-started
	runMemberJob(l, "backfill", "mb", "ann", func(context.Context) error {
		queuedRan.Store(true)
		return nil
	})

	cleaned := make(chan struct{})
	go func() {
		app.Cleanup()
		close(cleaned)
	}()
	select {
	case <-cleaned:
		t.Fatal("the terminate finished while a member job was still running")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-cleaned:
	case <-time.After(10 * time.Second):
		t.Fatal("the terminate did not finish after the member job returned")
	}
	if queuedRan.Load() {
		t.Fatal("a queued member job ran after the terminate began")
	}
	r, ok := readonlytest.Find(logs(), "member jobs dropped: the app is terminating")
	if !ok || r.Attrs["key"] != "mb/ann" || r.Attrs["jobs"] != "1" {
		t.Fatalf("want a log naming the dropped job's key, got %v (found %v)", r.Attrs, ok)
	}

	// After the terminate, a new job for the key is dropped too, and does not
	// leave the key looking busy.
	runMemberJob(l, "backfill", "mb", "ann", func(context.Context) error {
		queuedRan.Store(true)
		return nil
	})
	l.mu.Lock()
	_, busy := l.queued["mb/ann"]
	l.mu.Unlock()
	if busy || queuedRan.Load() {
		t.Fatal("a member job started after the terminate, or left its key busy")
	}
}
