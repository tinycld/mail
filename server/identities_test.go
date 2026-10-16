package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func newIdentityTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newRealSchemaApp(t)
	registerAddressHooks(app)
	return app
}

func identityOf(t *testing.T, app core.App, userID string) *core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_identities", "user = {:user}", "", 0, 0, map[string]any{"user": userID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func TestEnsureIdentity_CreatesInboxOnMainDomain(t *testing.T) {
	app := newIdentityTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "second.example", "verified": true, "rank": 2})
	main := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true, "rank": 1})
	user := saveRealRecord(t, app, "users", map[string]any{"email": "alice@ext.example", "username": "alice", "name": "Alice", "role": "member"})

	identity, err := ensureIdentity(app, user)
	if err != nil || identity == nil {
		t.Fatalf("ensureIdentity = %v, %v", identity, err)
	}
	if identity.GetString("email_name") != "alice" {
		t.Fatalf("email_name = %q, want alice", identity.GetString("email_name"))
	}
	inbox, err := app.FindRecordById("mail_mailboxes", identity.GetString("inbox"))
	if err != nil {
		t.Fatal(err)
	}
	if inbox.GetString("type") != "personal" || inbox.GetString("domain") != main.Id {
		t.Fatalf("inbox type %q domain %s; want personal on the main domain", inbox.GetString("type"), inbox.GetString("domain"))
	}
	if row, _ := findAddressRow(app, "alice", main.Id); row == nil || row.GetString("mailbox") != inbox.Id {
		t.Fatal("the new inbox has no primary address on the main domain")
	}
	members, _ := app.FindRecordsByFilter("mail_mailbox_members", "mailbox = {:mb} && user = {:u} && role = 'owner'", "", 0, 0,
		map[string]any{"mb": inbox.Id, "u": user.Id})
	if len(members) != 1 {
		t.Fatal("the user does not own their new inbox")
	}

	again, err := ensureIdentity(app, user)
	if err != nil || again.Id != identity.Id {
		t.Fatal("ensureIdentity is not idempotent")
	}
}

func TestEnsureIdentity_AdoptsExistingPersonalInbox(t *testing.T) {
	app := newIdentityTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})
	user := saveRealRecord(t, app, "users", map[string]any{"email": "nathan@argosity.com", "username": "nathan", "name": "Nathan", "role": "owner"})
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "nathan", "domain": domain.Id, "type": "personal", "display_name": "Nathan", "name": "Nathan"})
	saveRealRecord(t, app, "mail_mailbox_members", map[string]any{"mailbox": inbox.Id, "user": user.Id, "role": "owner"})

	identity, err := ensureIdentity(app, user)
	if err != nil || identity == nil {
		t.Fatalf("ensureIdentity = %v, %v", identity, err)
	}
	if identity.GetString("inbox") != inbox.Id || identity.GetString("email_name") != "nathan" {
		t.Fatalf("identity inbox %s name %q; want the existing inbox and its address", identity.GetString("inbox"), identity.GetString("email_name"))
	}
}

func TestEnsureIdentity_GuestGetsNothing(t *testing.T) {
	app := newIdentityTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org", "verified": true})
	guest := saveRealRecord(t, app, "users", map[string]any{"email": "eve@ext.example", "username": "eve", "name": "Eve", "role": "guest"})

	identity, err := ensureIdentity(app, guest)
	if err != nil || identity != nil {
		t.Fatalf("a guest got an identity: %v, %v", identity, err)
	}
	if rows, _ := app.FindRecordsByFilter("mail_mailboxes", "address = 'eve'", "", 0, 0, nil); len(rows) > 0 {
		t.Fatal("a guest got a mailbox")
	}
}

func TestEnsureIdentity_WaitsForAVerifiedDomain(t *testing.T) {
	app := newIdentityTestApp(t)
	user := saveRealRecord(t, app, "users", map[string]any{"email": "bob@ext.example", "username": "bob", "name": "Bob", "role": "member"})
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "pending.example", "verified": false})

	if identity, err := ensureIdentity(app, user); err != nil || identity != nil {
		t.Fatalf("no verified domain must give no identity yet: %v, %v", identity, err)
	}
}

func TestDeriveEmailName_AvoidsReservedAndTakenNames(t *testing.T) {
	app := newIdentityTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org", "verified": true, "coverage": coverageEveryone})
	saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "support", "domain": domain.Id, "type": "shared", "display_name": "S", "name": "S"})

	if got := deriveEmailName(app, "Support", domain.Id); got != "support2" {
		t.Fatalf("deriveEmailName(Support) = %q, want support2 (support is a shared inbox)", got)
	}
	if got := deriveEmailName(app, "日本語", domain.Id); got != "" {
		t.Fatalf("deriveEmailName(unicode) = %q, want empty", got)
	}
}

func TestIdentityHooks_UserCreateAndGuestPromotion(t *testing.T) {
	app := newIdentityTestApp(t)
	registerIdentityHooks(app)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org", "verified": true})

	member := saveRealRecord(t, app, "users", map[string]any{"email": "m@ext.example", "username": "mia", "name": "Mia", "role": "member"})
	if identityOf(t, app, member.Id) == nil {
		t.Fatal("a new member got no identity")
	}

	guest := saveRealRecord(t, app, "users", map[string]any{"email": "g@ext.example", "username": "gus", "name": "Gus", "role": "guest"})
	if identityOf(t, app, guest.Id) != nil {
		t.Fatal("a new guest got an identity")
	}
	// Reload: a request or admin edit works on a record loaded from the database,
	// whose original values hold the stored role.
	guest, err := app.FindRecordById("users", guest.Id)
	if err != nil {
		t.Fatal(err)
	}
	guest.Set("role", "member")
	if err := app.Save(guest); err != nil {
		t.Fatal(err)
	}
	if identityOf(t, app, guest.Id) == nil {
		t.Fatal("a guest promoted to member got no identity")
	}
}
