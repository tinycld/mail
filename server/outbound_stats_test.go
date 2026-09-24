package mail

import (
	"testing"
	"time"

	"tinycld.org/core/outboundstats"
)

// The counter mail registers must be the same one the send gate enforces
// against. If these ever diverge, a deployment would be throttled on one
// number and judged on another.
func TestOutboundStatsCounter_MatchesTheGatesOwnCount(t *testing.T) {
	outboundstats.ResetForTesting()
	t.Cleanup(outboundstats.ResetForTesting)
	outboundstats.Register("mail", sendsSince)

	app := setupSendCapApp(t, "")
	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		seedMessage(t, app, "sent", now)
	}
	// Inbound mail carries "sending"; counting it would inflate the
	// denominator and make a real bounce rate look harmless.
	seedMessage(t, app, "sending", now)

	since := startOfUTCDay(now)

	direct, err := sendsSince(app, since)
	if err != nil {
		t.Fatalf("sendsSince: %v", err)
	}
	total, partial, errs := outboundstats.Total(app, since)
	if partial {
		t.Errorf("partial = true: %v", errs)
	}
	if total != direct {
		t.Errorf("registry total = %d, gate's own count = %d — these must not diverge", total, direct)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4 — received mail must not count as sent", total)
	}
}
