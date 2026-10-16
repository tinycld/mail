package mail

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/offboard"
)

// newRegisteredMailApp is the real schema with every mail hook and the mail
// offboard handler registered, as in production.
func newRegisteredMailApp(t *testing.T) *tests.TestApp {
	t.Helper()
	t.Setenv("IMAP_ENABLED", "false")
	t.Setenv("SMTP_ENABLED", "false")
	app := newRealSchemaApp(t)
	offboard.ResetHandlersForTesting()
	t.Cleanup(offboard.ResetHandlersForTesting)
	Register(&pocketbase.PocketBase{App: app})
	return app
}

// triggerBoot runs the OnServe hooks, the boot repair among them.
func triggerBoot(t *testing.T, app *tests.TestApp) {
	t.Helper()
	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	if err := app.OnServe().Trigger(&core.ServeEvent{App: app, Router: router}, func(*core.ServeEvent) error { return nil }); err != nil {
		t.Fatalf("OnServe trigger: %v", err)
	}
}

// requireNoMailFor fails when the user owns a mailbox or any address with
// the local part still exists.
func requireNoMailFor(t *testing.T, app core.App, user *core.Record, local, when string) {
	t.Helper()
	owned, err := findPersonalMailboxesOf(app, user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 0 {
		t.Fatalf("%s: the leaver owns personal mailboxes %v", when, owned)
	}
	n, err := app.CountRecords("mail_addresses", dbx.HashExp{"local_part": local})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%s: %d address rows for %s remain", when, n, local)
	}
}

// Offboarding deletes the leaver's inbox. Core then saves the anonymized
// users row, which keeps its role: nothing may give that account a new inbox
// with the leaver's old address, not the users hook, not boot, not a domain
// verification. The email name is free for the next person.
func TestOffboard_LeaverGetsNoNewInbox(t *testing.T) {
	app := newRegisteredMailApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})
	com := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.com", "verified": true, "coverage": coverageEveryone})
	admin := saveRealRecord(t, app, "users", map[string]any{"email": "ada@ext.example", "username": "ada", "name": "Ada", "role": "admin"})
	leaver := saveRealRecord(t, app, "users", map[string]any{"email": "nathan@argosity.com", "username": "nathan", "name": "Nathan", "role": "member"})
	before := identityOf(t, app, leaver.Id)
	if before == nil {
		t.Fatal("precondition: the leaver has no identity")
	}
	if n, _ := app.CountRecords("mail_addresses", dbx.HashExp{"local_part": "nathan"}); n != 2 {
		t.Fatalf("precondition: nathan has %d addresses, want primary + rule", n)
	}

	if _, err := offboard.OffboardUser(app, leaver.Id, offboard.Plan{Mode: offboard.ModeDeleteMyData}, admin.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	requireNoMailFor(t, app, leaver, "nathan", "after offboard")
	identity := identityOf(t, app, leaver.Id)
	if identity == nil || !identity.GetBool("retired") || identity.GetString("inbox") != "" ||
		identity.GetString("email_name") != "retired-"+identity.Id {
		t.Fatalf("identity after offboard = %v, want retired with no inbox and a tombstone name", identity)
	}
	anonymized, err := app.FindRecordById("users", leaver.Id)
	if err != nil || anonymized.GetString("role") != "member" {
		t.Fatalf("precondition: the anonymized user keeps the member role (err %v)", err)
	}

	// A second leaver: two retired identities, both with no inbox, must not
	// collide on the inbox index.
	second := saveRealRecord(t, app, "users", map[string]any{"email": "mia@ext.example", "username": "mia", "name": "Mia", "role": "member"})
	if _, err := offboard.OffboardUser(app, second.Id, offboard.Plan{Mode: offboard.ModeDeleteMyData}, admin.Id); err != nil {
		t.Fatalf("OffboardUser second leaver: %v", err)
	}
	if got := identityOf(t, app, second.Id); got == nil || !got.GetBool("retired") {
		t.Fatalf("second leaver identity = %v, want retired", got)
	}

	triggerBoot(t, app)
	requireNoMailFor(t, app, leaver, "nathan", "after boot")
	requireNoMailFor(t, app, second, "mia", "after boot")
	if err := ensureIdentities(app, adoptOrCreateInbox); err != nil {
		t.Fatal(err)
	}
	requireNoMailFor(t, app, leaver, "nathan", "after a domain verification")

	if taken, err := emailNameTaken(app, "nathan", ""); err != nil || taken {
		t.Fatalf("emailNameTaken(nathan) = %v, %v; the leaver's name must be free", taken, err)
	}
	// On an everyone domain a live email name is reserved; a retired one is not.
	saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "nathan", "domain": com.Id, "type": "shared", "display_name": "Desk", "name": "Desk",
	})
}

// Boot repair only adopts: a user who owns a personal inbox but has no
// identity gets one; a user with no inbox (possibly offboarded before
// identities existed) gets nothing.
func TestBoot_AdoptsInboxesAndCreatesNone(t *testing.T) {
	app := newRealSchemaApp(t)
	registerAddressHooks(app)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "boot.example", "verified": true})
	owner := saveRealRecord(t, app, "users", map[string]any{"email": "o@ext.example", "username": "olive", "name": "Olive", "role": "member"})
	inbox := seedOwnedPersonalInbox(t, app, owner, domain.Id, "olive")
	leaver := saveRealRecord(t, app, "users", map[string]any{"email": "deleted-x@deleted.invalid", "username": "lee", "name": "Deleted user", "role": "member"})

	t.Setenv("IMAP_ENABLED", "false")
	t.Setenv("SMTP_ENABLED", "false")
	offboard.ResetHandlersForTesting()
	t.Cleanup(offboard.ResetHandlersForTesting)
	Register(&pocketbase.PocketBase{App: app})
	triggerBoot(t, app)

	if got := identityOf(t, app, owner.Id); got == nil || got.GetString("inbox") != inbox.Id {
		t.Fatalf("boot did not adopt the existing inbox: %v", got)
	}
	if got := identityOf(t, app, leaver.Id); got != nil {
		t.Fatalf("boot gave a user with no inbox an identity: %v", got)
	}
	requireNoMailFor(t, app, leaver, "lee", "after boot")
}
