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
		// The goroutine touches no t, so abandoning it on a hang is safe.
		result := make(chan []string, 1)
		start := time.Now()
		go func() { result <- losesNothingProblems(raw) }()
		select {
		case problems := <-result:
			for _, problem := range problems {
				t.Error(problem)
			}
		case <-time.After(time.Second):
			t.Fatalf("parsing %d bytes did not finish within a second (waited %s)", len(raw), time.Since(start))
		}
	})
}
