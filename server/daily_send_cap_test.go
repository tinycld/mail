package mail

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/syscfg"
)

// setupSendCapApp builds the smallest schema the cap reads: mail_messages
// with the two columns the count filters on, plus a syscfg stub so the
// ceiling can be set without a settings collection.
func setupSendCapApp(t *testing.T, cap string) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	messages := core.NewBaseCollection("mail_messages")
	messages.Fields.Add(&core.TextField{Name: "delivery_status"})
	messages.Fields.Add(&core.DateField{Name: "date"})
	if err := app.Save(messages); err != nil {
		t.Fatalf("save mail_messages: %v", err)
	}

	syscfg.SetResolver(func(key string) string {
		if key == "mail.max_sends_per_day" {
			return cap
		}
		return ""
	})
	t.Cleanup(func() { syscfg.SetResolver(func(string) string { return "" }) })

	return app
}

// verifiedDomain returns a mail_domains record with all three outbound DNS
// checks passing, so a test of some OTHER gate is not refused by the domain
// one first.
func verifiedDomain(t *testing.T, app *tests.TestApp) *core.Record {
	t.Helper()

	col, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		col = core.NewBaseCollection("mail_domains")
		col.Fields.Add(&core.TextField{Name: "domain"})
		col.Fields.Add(&core.BoolField{Name: "spf_verified"})
		col.Fields.Add(&core.BoolField{Name: "dkim_verified"})
		col.Fields.Add(&core.BoolField{Name: "return_path_verified"})
		if err := app.Save(col); err != nil {
			t.Fatalf("save mail_domains: %v", err)
		}
	}

	rec := core.NewRecord(col)
	rec.Set("domain", "example.com")
	rec.Set("spf_verified", true)
	rec.Set("dkim_verified", true)
	rec.Set("return_path_verified", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	return rec
}

// seedMessage writes one mail_messages row with the given status and age.
func seedMessage(t *testing.T, app *tests.TestApp, status string, at time.Time) {
	t.Helper()

	col, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	when, err := types.ParseDateTime(at.UTC().Format("2006-01-02 15:04:05.000Z"))
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("delivery_status", status)
	rec.Set("date", when)
	if err := app.Save(rec); err != nil {
		t.Fatalf("seed message: %v", err)
	}
}

func TestDailySendCap_UnsetIsUnlimited(t *testing.T) {
	app := setupSendCapApp(t, "")
	if got := dailySendCap(app); got != 0 {
		t.Errorf("dailySendCap = %d, want 0 (unlimited)", got)
	}
}

// A typo must not refuse every send. The operator gets a loud log instead.
func TestDailySendCap_UnparseableIsUnlimited(t *testing.T) {
	for _, bad := range []string{"lots", "-5", "3.5", " "} {
		app := setupSendCapApp(t, bad)
		if got := dailySendCap(app); got != 0 {
			t.Errorf("dailySendCap(%q) = %d, want 0 (unlimited)", bad, got)
		}
	}
}

func TestDailySendCap_ReadsTheConfiguredValue(t *testing.T) {
	app := setupSendCapApp(t, "50")
	if got := dailySendCap(app); got != 50 {
		t.Errorf("dailySendCap = %d, want 50", got)
	}
}

func TestSendsToday_CountsSentAndBounced(t *testing.T) {
	app := setupSendCapApp(t, "")
	now := time.Now().UTC()

	seedMessage(t, app, "sent", now)
	seedMessage(t, app, "bounced", now)

	got, err := sendsToday(app)
	if err != nil {
		t.Fatalf("sendsToday: %v", err)
	}
	if got != 2 {
		t.Errorf("sendsToday = %d, want 2 — a bounce still consumed a send", got)
	}
}

// The trap that makes a busy mailbox look like an outage: "sending" is
// storeMessage's default for an unset status, so every inbound webhook and
// IMAP-synced message carries it. Counting those would charge RECEIVED mail
// against the send budget.
func TestSendsToday_IgnoresReceivedMail(t *testing.T) {
	app := setupSendCapApp(t, "")
	now := time.Now().UTC()

	for i := 0; i < 5; i++ {
		seedMessage(t, app, "sending", now)
	}
	seedMessage(t, app, "draft", now)
	seedMessage(t, app, "sent", now)

	got, err := sendsToday(app)
	if err != nil {
		t.Fatalf("sendsToday: %v", err)
	}
	if got != 1 {
		t.Errorf("sendsToday = %d, want 1 — only the outbound message counts", got)
	}
}

// The window is today, not all time: yesterday's sends must not hold a
// deployment at its ceiling forever.
func TestSendsToday_ExcludesEarlierDays(t *testing.T) {
	app := setupSendCapApp(t, "")
	now := time.Now().UTC()

	seedMessage(t, app, "sent", now.AddDate(0, 0, -1))
	seedMessage(t, app, "sent", now.AddDate(0, 0, -7))
	seedMessage(t, app, "sent", now)

	got, err := sendsToday(app)
	if err != nil {
		t.Fatalf("sendsToday: %v", err)
	}
	if got != 1 {
		t.Errorf("sendsToday = %d, want 1 — only today counts", got)
	}
}

// The RFC3339 trap: PocketBase compares dates as text, so a bound formatted
// as "...T00:00:00Z" sorts above every stored value and matches nothing. If
// this regresses, the count is always zero and the cap silently never fires.
func TestSendsToday_CountsAnythingAtAll(t *testing.T) {
	app := setupSendCapApp(t, "")
	seedMessage(t, app, "sent", time.Now().UTC())

	got, err := sendsToday(app)
	if err != nil {
		t.Fatalf("sendsToday: %v", err)
	}
	if got == 0 {
		t.Fatal("sendsToday = 0 with a message stored today — the date bound is not matching")
	}
}

func TestCheckSendAllowed_AllowsUnderTheDailyCap(t *testing.T) {
	app := setupSendCapApp(t, "3")
	now := time.Now().UTC()
	seedMessage(t, app, "sent", now)
	seedMessage(t, app, "sent", now)

	if refusal := checkSendAllowed(app, "u1", "mb1", verifiedDomain(t, app), 1); refusal != nil {
		t.Errorf("under the cap must be allowed, got %v", refusal)
	}
}

func TestCheckSendAllowed_RefusesAtTheDailyCap(t *testing.T) {
	app := setupSendCapApp(t, "3")
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		seedMessage(t, app, "sent", now)
	}

	refusal := checkSendAllowed(app, "u1", "mb1", verifiedDomain(t, app), 1)
	if refusal == nil {
		t.Fatal("the fourth send must be refused at a cap of 3")
	}
	if refusal.kind != sendErrForbidden {
		t.Errorf("kind = %v, want sendErrForbidden", refusal.kind)
	}
	// The message is the whole UX: a sender who cannot tell why will retry.
	if refusal.msg == "" {
		t.Error("a refusal must say something actionable")
	}
}

func TestCheckSendAllowed_NoCapNeverRefuses(t *testing.T) {
	app := setupSendCapApp(t, "")
	now := time.Now().UTC()
	for i := 0; i < 50; i++ {
		seedMessage(t, app, "sent", now)
	}

	if refusal := checkSendAllowed(app, "u1", "mb1", verifiedDomain(t, app), 1); refusal != nil {
		t.Errorf("with no cap configured nothing may be refused, got %v", refusal)
	}
}

// The load-bearing one. Unlike the download ceiling, this fails CLOSED: mail
// that has left cannot be recalled, so an uncountable state must refuse. A
// version of this test that passes by returning nil is the bug.
func TestCheckSendAllowed_FailsClosedWhenTheCountFails(t *testing.T) {
	app := setupSendCapApp(t, "10")

	// Drop the table the count reads, standing in for any state where the
	// query cannot answer.
	if _, err := app.DB().NewQuery(`DROP TABLE mail_messages`).Execute(); err != nil {
		t.Fatalf("drop mail_messages: %v", err)
	}

	refusal := checkSendAllowed(app, "u1", "mb1", verifiedDomain(t, app), 1)
	if refusal == nil {
		t.Fatal("an uncountable send budget must REFUSE, not allow — see the gate's header")
	}
	if refusal.kind != sendErrForbidden {
		t.Errorf("kind = %v, want sendErrForbidden", refusal.kind)
	}
}

// The recipient cap runs before the daily one, so an oversized message is
// refused for the reason its sender can act on.
func TestCheckSendAllowed_RecipientCapRunsFirst(t *testing.T) {
	app := setupSendCapApp(t, "1")
	seedMessage(t, app, "sent", time.Now().UTC())

	refusal := checkSendAllowed(app, "u1", "mb1", verifiedDomain(t, app), maxRecipientsPerMessage+1)
	if refusal == nil {
		t.Fatal("an oversized recipient list must be refused")
	}
	if refusal.msg == "" || refusal.kind != sendErrForbidden {
		t.Fatalf("unexpected refusal: %+v", refusal)
	}
}
