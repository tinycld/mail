package mail

import (
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func newIdentityTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newRealSchemaApp(t)
	registerAddressHooks(app)
	return app
}

func identityOf(t testing.TB, app core.App, userID string) *core.Record {
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

func setMailboxAddressRaw(t *testing.T, app core.App, id, address string) {
	t.Helper()
	if _, err := app.DB().NewQuery("UPDATE mail_mailboxes SET address = {:a} WHERE id = {:id}").
		Bind(dbx.Params{"a": address, "id": id}).Execute(); err != nil {
		t.Fatal(err)
	}
}

func seedOwnedPersonalInbox(t *testing.T, app core.App, user *core.Record, domainID, address string) *core.Record {
	t.Helper()
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": address, "domain": domainID, "type": "personal", "display_name": "X", "name": "X"})
	saveRealRecord(t, app, "mail_mailbox_members", map[string]any{"mailbox": inbox.Id, "user": user.Id, "role": "owner"})
	return inbox
}

func TestEnsureIdentity_NormalizesLegacyInboxAddress(t *testing.T) {
	app := newIdentityTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})

	upper := saveRealRecord(t, app, "users", map[string]any{"email": "a@ext.example", "username": "nathan", "name": "N", "role": "member"})
	inbox := seedOwnedPersonalInbox(t, app, upper, domain.Id, "tmp1")
	setMailboxAddressRaw(t, app, inbox.Id, "Nathan")
	identity, err := ensureIdentity(app, upper)
	if err != nil || identity == nil || identity.GetString("email_name") != "nathan" {
		t.Fatalf("uppercase legacy address: %v, %v", identity, err)
	}

	plus := saveRealRecord(t, app, "users", map[string]any{"email": "b@ext.example", "username": "carol", "name": "C", "role": "member"})
	inbox2 := seedOwnedPersonalInbox(t, app, plus, domain.Id, "tmp2")
	setMailboxAddressRaw(t, app, inbox2.Id, "first+last")
	identity, err = ensureIdentity(app, plus)
	if err != nil || identity == nil || identity.GetString("email_name") != "carol" {
		t.Fatalf("unusable legacy address: %v, %v", identity, err)
	}
	if identity.GetString("inbox") != inbox2.Id {
		t.Fatal("the legacy inbox was not adopted")
	}
}

func TestEnsureIdentity_AdoptedAddressHeldByAnotherIdentity(t *testing.T) {
	app := newIdentityTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true, "rank": 1})
	first := saveRealRecord(t, app, "users", map[string]any{"email": "a@ext.example", "username": "dana", "name": "D", "role": "member"})
	if id, err := ensureIdentity(app, first); err != nil || id == nil || id.GetString("email_name") != "dana" {
		t.Fatalf("first identity: %v, %v", id, err)
	}
	second := saveRealRecord(t, app, "users", map[string]any{"email": "b@ext.example", "username": "erin", "name": "E", "role": "member"})
	// Another domain: the same address on the same domain is unique-constrained.
	otherDomain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "other.example", "verified": true})
	seedOwnedPersonalInbox(t, app, second, otherDomain.Id, "dana")

	identity, err := ensureIdentity(app, second)
	if err != nil || identity == nil || identity.GetString("email_name") != "erin" {
		t.Fatalf("want derived name erin: %v, %v", identity, err)
	}
}

func TestEnsureIdentity_AdoptsOldestOfTwoPersonalInboxes(t *testing.T) {
	app := newIdentityTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})
	user := saveRealRecord(t, app, "users", map[string]any{"email": "a@ext.example", "username": "finn", "name": "F", "role": "member"})
	oldest := seedOwnedPersonalInbox(t, app, user, domain.Id, "finn")
	seedOwnedPersonalInbox(t, app, user, domain.Id, "finn-two")

	identity, err := ensureIdentity(app, user)
	if err != nil || identity == nil || identity.GetString("inbox") != oldest.Id {
		t.Fatalf("want the oldest inbox adopted: %v, %v", identity, err)
	}
	all, _ := app.FindRecordsByFilter("mail_mailboxes", "type = 'personal'", "", 0, 0)
	if len(all) != 2 {
		t.Fatalf("personal inboxes = %d, want 2 (no third inbox)", len(all))
	}
}

func TestDeriveEmailName_AvoidsEveryoneAddressOnOtherDomain(t *testing.T) {
	app := newIdentityTestApp(t)
	own := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "own.example", "verified": true})
	other := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "other.example", "verified": true, "coverage": coverageEveryone})
	saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "sales", "domain": other.Id, "type": "shared", "display_name": "S", "name": "S"})

	if got := deriveEmailName(app, "sales", own.Id); got != "sales2" {
		t.Fatalf("deriveEmailName = %q, want sales2", got)
	}
}

func TestDeriveEmailName_CapsLength(t *testing.T) {
	app := newIdentityTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "own.example", "verified": true})
	long := strings.Repeat("a", 80)
	got := deriveEmailName(app, long, domain.Id)
	if len(got) != 64 {
		t.Fatalf("len = %d, want 64", len(got))
	}
}
