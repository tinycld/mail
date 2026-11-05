package mail

import (
	"os"
	"testing"
	"time"
)

// FuzzParseRFC5322 starts from every corpus message. Whatever the input, the
// parser must not panic, must keep the salvage guarantees, and must finish
// quickly (a slow input is a quadratic path an attacker can use). In CI it
// runs over the seeds; run it longer locally with
// go test -fuzz FuzzParseRFC5322 -fuzztime 5m .
func FuzzParseRFC5322(f *testing.F) {
	for _, p := range corpusPaths(f) {
		raw, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		done := make(chan struct{})
		start := time.Now()
		go func() {
			defer close(done)
			checkLosesNothing(t, raw)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			// Wait for the goroutine so its late t.Errorf cannot hit a finished test.
			<-done
			t.Fatalf("parsing %d bytes took %s, over a second", len(raw), time.Since(start))
		}
	})
}
