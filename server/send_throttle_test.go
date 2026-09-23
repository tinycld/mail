package mail

import (
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/sendquota"
)

// claimSendLimits installs a resolver for the duration of one test, standing
// in for the supervising composition that sets these in production.
func claimSendLimits(t *testing.T, l sendquota.SendLimits) {
	t.Helper()
	sendquota.ResetForTesting()
	t.Cleanup(sendquota.ResetForTesting)
	sendquota.SetLimits(func(core.App) sendquota.SendLimits { return l })
}

// With nobody claiming the seam a deployment keeps its own daily cap and has
// no hourly throttle — the standalone case must be unchanged by all of this.
func TestSendLimits_StandaloneFallsBackToItsOwnSetting(t *testing.T) {
	sendquota.ResetForTesting()
	t.Cleanup(sendquota.ResetForTesting)

	app := setupSendCapApp(t, "250")

	got := sendLimits(app)
	if got.PerDay != 250 {
		t.Errorf("PerDay = %d, want the deployment's own 250", got.PerDay)
	}
	if got.PerHour != 0 {
		t.Errorf("PerHour = %d, want 0 — a standalone deployment throttles nobody", got.PerHour)
	}
}

// Once claimed, the supervisor's numbers win and the deployment's own setting
// is ignored. A throttle it could override by editing its own settings would
// not be a throttle.
func TestSendLimits_AClaimedSeamOverridesTheLocalSetting(t *testing.T) {
	app := setupSendCapApp(t, "250")
	claimSendLimits(t, sendquota.SendLimits{PerDay: 10, PerHour: 5})

	got := sendLimits(app)
	if got.PerDay != 10 || got.PerHour != 5 {
		t.Errorf("limits = %+v, want {10 5} from the claimed seam", got)
	}
}

func TestCheckSendAllowed_AllowsUnderTheHourlyThrottle(t *testing.T) {
	app := setupSendCapApp(t, "")
	domain := verifiedDomain(t, app)
	claimSendLimits(t, sendquota.SendLimits{PerHour: 5})

	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		seedMessage(t, app, "sent", now)
	}

	if refusal := checkSendAllowed(app, "u1", "mb1", domain, 1); refusal != nil {
		t.Errorf("under the hourly throttle must be allowed, got %v", refusal)
	}
}

func TestCheckSendAllowed_RefusesAtTheHourlyThrottle(t *testing.T) {
	app := setupSendCapApp(t, "")
	domain := verifiedDomain(t, app)
	claimSendLimits(t, sendquota.SendLimits{PerHour: 5})

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		seedMessage(t, app, "sent", now)
	}

	refusal := checkSendAllowed(app, "u1", "mb1", domain, 1)
	if refusal == nil {
		t.Fatal("the sixth send in an hour must be refused at a throttle of 5")
	}
	if refusal.kind != sendErrForbidden {
		t.Errorf("kind = %v, want sendErrForbidden", refusal.kind)
	}
	// The sender cannot fix this themselves, so the message must say who can.
	if !strings.Contains(refusal.msg, "administrator") {
		t.Errorf("a throttle refusal should point somewhere actionable, got %q", refusal.msg)
	}
}

// The window is a rolling hour: sends from earlier today must not hold a
// throttled deployment at its ceiling all day.
func TestCheckSendAllowed_HourlyWindowRolls(t *testing.T) {
	app := setupSendCapApp(t, "")
	domain := verifiedDomain(t, app)
	claimSendLimits(t, sendquota.SendLimits{PerHour: 2})

	now := time.Now().UTC()
	// Two sends, but both more than an hour ago.
	seedMessage(t, app, "sent", now.Add(-90*time.Minute))
	seedMessage(t, app, "sent", now.Add(-2*time.Hour))

	if refusal := checkSendAllowed(app, "u1", "mb1", domain, 1); refusal != nil {
		t.Errorf("sends outside the hour must not count against it, got %v", refusal)
	}
}

// The hourly ceiling is checked first on purpose: a throttled deployment's
// sender needs to hear that it is under review, not a daily number they are
// nowhere near.
func TestCheckSendAllowed_ThrottleReasonWinsOverTheDailyCap(t *testing.T) {
	app := setupSendCapApp(t, "")
	domain := verifiedDomain(t, app)
	claimSendLimits(t, sendquota.SendLimits{PerDay: 1000, PerHour: 2})

	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		seedMessage(t, app, "sent", now)
	}

	refusal := checkSendAllowed(app, "u1", "mb1", domain, 1)
	if refusal == nil {
		t.Fatal("the throttle must refuse")
	}
	if strings.Contains(refusal.msg, "daily") {
		t.Errorf("the throttle's reason should surface, not the daily cap's: %q", refusal.msg)
	}
}

// A throttle of zero is not a throttle of none — unlimited is the convention
// everywhere else, and reading it as "refuse everything" would take a
// deployment offline the moment the seam resolved to a zero value.
func TestCheckSendAllowed_ZeroThrottleIsUnlimited(t *testing.T) {
	app := setupSendCapApp(t, "")
	domain := verifiedDomain(t, app)
	claimSendLimits(t, sendquota.SendLimits{})

	now := time.Now().UTC()
	for i := 0; i < 50; i++ {
		seedMessage(t, app, "sent", now)
	}

	if refusal := checkSendAllowed(app, "u1", "mb1", domain, 1); refusal != nil {
		t.Errorf("zero means unlimited, got %v", refusal)
	}
}

// Fails CLOSED on an uncountable window, same as the daily cap.
func TestCheckSendAllowed_ThrottleFailsClosed(t *testing.T) {
	app := setupSendCapApp(t, "")
	domain := verifiedDomain(t, app)
	claimSendLimits(t, sendquota.SendLimits{PerHour: 5})

	if _, err := app.DB().NewQuery(`DROP TABLE mail_messages`).Execute(); err != nil {
		t.Fatalf("drop mail_messages: %v", err)
	}

	if refusal := checkSendAllowed(app, "u1", "mb1", domain, 1); refusal == nil {
		t.Fatal("an uncountable throttle window must REFUSE, not allow")
	}
}
