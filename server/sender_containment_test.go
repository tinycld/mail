package mail

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupContainmentApp builds the columns ConcentratedSender reads plus the
// users and notifications ContainSender writes.
func setupContainmentApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	messages := core.NewBaseCollection("mail_messages")
	messages.Fields.Add(&core.TextField{Name: "delivery_status"})
	messages.Fields.Add(&core.DateField{Name: "date"})
	messages.Fields.Add(&core.TextField{Name: "sent_by"})
	if err := app.Save(messages); err != nil {
		t.Fatal(err)
	}

	notifications := core.NewBaseCollection("notifications")
	notifications.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1})
	for _, f := range []string{"type", "package", "title", "body", "url"} {
		notifications.Fields.Add(&core.TextField{Name: f})
	}
	notifications.Fields.Add(&core.JSONField{Name: "metadata"})
	notifications.Fields.Add(&core.BoolField{Name: "read"})
	if err := app.Save(notifications); err != nil {
		t.Fatal(err)
	}

	return app
}

func seedSender(t *testing.T, app *tests.TestApp, email, role string) *core.Record {
	t.Helper()
	users, _ := app.FindCollectionByNameOrId("users")
	u := core.NewRecord(users)
	u.Set("email", email)
	u.SetPassword("Password123!")
	u.Set("role", role)
	if err := app.Save(u); err != nil {
		t.Fatalf("seed %s: %v", email, err)
	}
	return u
}

func seedFailure(t *testing.T, app *tests.TestApp, sentBy, status string, at time.Time) {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("mail_messages")
	rec := core.NewRecord(col)
	rec.Set("delivery_status", status)
	rec.Set("sent_by", sentBy)
	rec.Set("date", at.UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := app.Save(rec); err != nil {
		t.Fatalf("seed failure: %v", err)
	}
}

func TestConcentratedSender_FindsTheDominantAccount(t *testing.T) {
	app := setupContainmentApp(t)
	now := time.Now().UTC()
	bad := seedSender(t, app, "compromised@example.com", "member")
	ok := seedSender(t, app, "normal@example.com", "member")

	for i := 0; i < 9; i++ {
		seedFailure(t, app, bad.Id, "spam_complaint", now)
	}
	seedFailure(t, app, ok.Id, "bounced", now)

	got, n, err := ConcentratedSender(app, now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatalf("ConcentratedSender: %v", err)
	}
	if got != bad.Id {
		t.Errorf("sender = %q, want the dominant account", got)
	}
	if n != 9 {
		t.Errorf("failures = %d, want 9", n)
	}
}

// Failures spread across the organisation are a list problem, not one
// person's doing — and disabling somebody over a coincidence is worse than
// not acting, because they cannot undo it themselves.
func TestConcentratedSender_SpreadFailuresBlameNobody(t *testing.T) {
	app := setupContainmentApp(t)
	now := time.Now().UTC()

	for i := 0; i < 4; i++ {
		u := seedSender(t, app, string(rune('a'+i))+"@example.com", "member")
		for j := 0; j < 3; j++ {
			seedFailure(t, app, u.Id, "bounced", now)
		}
	}

	got, _, err := ConcentratedSender(app, now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatalf("ConcentratedSender: %v", err)
	}
	if got != "" {
		t.Errorf("sender = %q, want none — no account dominates", got)
	}
}

// Two failures from one person is a Tuesday. Concentration only means
// something once there are enough failures that it cannot be chance.
func TestConcentratedSender_TooFewFailuresToAttribute(t *testing.T) {
	app := setupContainmentApp(t)
	now := time.Now().UTC()
	u := seedSender(t, app, "unlucky@example.com", "member")

	for i := 0; i < 3; i++ {
		seedFailure(t, app, u.Id, "bounced", now)
	}

	got, _, err := ConcentratedSender(app, now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatalf("ConcentratedSender: %v", err)
	}
	if got != "" {
		t.Errorf("sender = %q, want none — three failures is not a pattern", got)
	}
}

// Failures outside the window must not implicate somebody for last month.
func TestConcentratedSender_RespectsTheWindow(t *testing.T) {
	app := setupContainmentApp(t)
	now := time.Now().UTC()
	u := seedSender(t, app, "historic@example.com", "member")

	for i := 0; i < 20; i++ {
		seedFailure(t, app, u.Id, "spam_complaint", now.AddDate(0, 0, -30))
	}

	got, _, err := ConcentratedSender(app, now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatalf("ConcentratedSender: %v", err)
	}
	if got != "" {
		t.Errorf("sender = %q, want none — those failures are outside the window", got)
	}
}

// Received mail carries no sender, and inbound failures must never implicate
// anybody.
func TestConcentratedSender_IgnoresUnattributedFailures(t *testing.T) {
	app := setupContainmentApp(t)
	now := time.Now().UTC()

	for i := 0; i < 20; i++ {
		seedFailure(t, app, "", "bounced", now)
	}

	got, _, err := ConcentratedSender(app, now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatalf("ConcentratedSender: %v", err)
	}
	if got != "" {
		t.Errorf("sender = %q, want none — unattributed failures blame nobody", got)
	}
}

func TestContainSender_DisablesAndTellsTheAdministrators(t *testing.T) {
	app := setupContainmentApp(t)
	owner := seedSender(t, app, "owner@example.com", "owner")
	bad := seedSender(t, app, "compromised@example.com", "member")

	if err := ContainSender(app, bad.Id, 9, time.Now().UTC().AddDate(0, 0, -7)); err != nil {
		t.Fatalf("ContainSender: %v", err)
	}

	fresh, _ := app.FindRecordById("users", bad.Id)
	if !fresh.GetBool("disabled") {
		t.Error("the account was not disabled")
	}

	notices, err := app.FindRecordsByFilter("notifications", "user = {:u}", "", 0, 0,
		map[string]any{"u": owner.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 {
		t.Fatalf("the owner got %d notices, want 1 — somebody has to restore the account", len(notices))
	}
	if body := notices[0].GetString("body"); body == "" {
		t.Error("the notice must say what happened and how to undo it")
	}
}

// Re-notifying every sweep would train the administrators to ignore the
// notice, which is the opposite of what it is for.
func TestContainSender_AlreadyContainedIsQuiet(t *testing.T) {
	app := setupContainmentApp(t)
	owner := seedSender(t, app, "owner@example.com", "owner")
	bad := seedSender(t, app, "compromised@example.com", "member")

	for i := 0; i < 3; i++ {
		if err := ContainSender(app, bad.Id, 9, time.Now().UTC()); err != nil {
			t.Fatalf("ContainSender %d: %v", i, err)
		}
	}

	notices, err := app.FindRecordsByFilter("notifications", "user = {:u}", "", 0, 0,
		map[string]any{"u": owner.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 {
		t.Errorf("%d notices for one containment, want 1", len(notices))
	}
}
