package mail

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestJobLimiter_TryAcquireStopsAtLimit(t *testing.T) {
	l := newJobLimiter(2)
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
	l := newJobLimiter(2)
	const jobs = 6
	var active, peak, ran atomic.Int32
	started := make(chan struct{}, jobs)
	gate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(jobs)
	for i := range jobs {
		l.runKeyed(fmt.Sprintf("key-%d", i), func() {
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
	l := newJobLimiter(1)
	done := make(chan struct{})
	runMemberJob(l, "test", "mb", "user", func() error { panic("boom") })
	runMemberJob(l, "test", "mb", "user", func() error { close(done); return nil })
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the job after a panic did not run in 2s; the panic kept its slot")
	}
}

// Jobs for one (mailbox, user) key run one at a time, in the order they were
// queued. A job for another key does not wait for them.
func TestMemberJobs_SameKeyRunsInOrder(t *testing.T) {
	l := newJobLimiter(2)
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

	runMemberJob(l, "cleanup", "mb", "ann", func() error {
		close(firstStarted)
		<-gate
		record("cleanup")
		return nil
	})
	<-firstStarted
	runMemberJob(l, "backfill", "mb", "ann", func() error {
		record("backfill")
		close(secondStarted)
		return nil
	})
	runMemberJob(l, "backfill", "mb", "bob", func() error {
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
