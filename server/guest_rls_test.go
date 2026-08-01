package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// guest_rls_test.go proves the mail-infra access rules tightened by the
// 1830000002 / 1830000003 migrations against PocketBase's REAL rule engine. A
// role='guest' user must be DENIED while a real member is still ALLOWED.
//
// Background: a guest share-link visitor gets a real users record with
// role='guest'. Several mail-infra rules granted access to ANY authenticated
// user regardless of role:
//   - mail_domains         list/view  (read leak: guest enumerates mail domains)
//   - mail_mailboxes       create     (write leak: guest creates a mailbox)
//   - mail_mailbox_aliases list/view  (read leak: guest enumerates aliases)
//
// Single-org: the caller's role lives on the `users` auth record, so the guest
// check is a direct `@request.auth.role != "guest"` — no relation-path join
// through a membership junction (the former user_org table is gone).
//
// The rule constants below MIRROR THE MIGRATIONS VERBATIM. Keep them in sync:
// if a migration's rule changes, change it here too, or this file silently
// stops testing what ships.
//
// NOTE the rules left UNCHANGED (already safe): mail_domains create/update/
// delete + mail_mailbox_aliases create/update/delete are admin/owner-gated;
// mail_mailboxes list/view is mailbox-membership-gated (a guest is not a
// mailbox member).
//
// Each scenario builds a FRESH TestApp (ApiScenario.Test re-triggers OnServe;
// reusing one app panics on duplicate route registration).

// Rule strings mirror pb-migrations/1830000002_exclude_guests_from_mail_infra.js
// and 1830000003_pin_guest_exclusion_on_mailbox_members_bootstrap.js.
const (
	nonGuestRule = `@request.auth.id != "" && @request.auth.role != "guest"`

	mailDomainsGuestReadRule     = nonGuestRule
	mailMailboxesGuestCreateRule = nonGuestRule
	mailAliasesGuestReadRule     = nonGuestRule

	// Mirrors 1830000003: the bootstrap branch pins `@request.auth.role !=
	// "guest"` alongside the self-insert check, so a guest can never
	// bootstrap-own a memberless mailbox.
	mailMembersOwnerCanAdd = `mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id && ` +
		`mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"`
	mailMembersBootstrapGuestRule = `user = @request.auth.id && ` +
		`role = "owner" && ` +
		`mailbox.mail_mailbox_members_via_mailbox.id = "" && ` +
		`@request.auth.role != "guest"`
	mailMembersGuestCreateRule = `(` + mailMembersOwnerCanAdd + `) || (` + mailMembersBootstrapGuestRule + `)`
)

type mailGuestEnv struct {
	app         *tests.TestApp
	domain      *core.Record
	mailbox     *core.Record
	member      *core.Record
	guest       *core.Record
	memberToken string
	guestToken  string
}

func setupMailGuestApp(t *testing.T) *mailGuestEnv {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	// Single-org: role is a field on the auth record itself.
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	domains := core.NewBaseCollection("mail_domains")
	domains.Id = "pbc_mail_domains_01"
	domains.Fields.Add(&core.TextField{Name: "domain", Required: true})
	domains.Fields.Add(&core.BoolField{Name: "verified"})
	if err := app.Save(domains); err != nil {
		t.Fatal(err)
	}

	mailboxes := core.NewBaseCollection("mail_mailboxes")
	mailboxes.Id = "pbc_mail_mailboxes_01"
	mailboxes.Fields.Add(&core.TextField{Name: "address", Required: true})
	mailboxes.Fields.Add(&core.RelationField{
		Name: "domain", Required: true, CollectionId: domains.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	mailboxes.Fields.Add(&core.SelectField{
		Name: "type", Required: true, MaxSelect: 1,
		Values: []string{"personal", "shared"},
	})
	if err := app.Save(mailboxes); err != nil {
		t.Fatal(err)
	}

	aliases := core.NewBaseCollection("mail_mailbox_aliases")
	aliases.Id = "pbc_mail_aliases_01"
	aliases.Fields.Add(&core.RelationField{
		Name: "mailbox", Required: true, CollectionId: mailboxes.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	aliases.Fields.Add(&core.TextField{Name: "address", Required: true})
	if err := app.Save(aliases); err != nil {
		t.Fatal(err)
	}

	members := core.NewBaseCollection("mail_mailbox_members")
	members.Id = "pbc_mail_mb_members_01"
	members.Fields.Add(&core.RelationField{
		Name: "mailbox", Required: true, CollectionId: mailboxes.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	// Single-org: membership points straight at the users collection.
	members.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	members.Fields.Add(&core.SelectField{
		Name: "role", Required: true, MaxSelect: 1,
		Values: []string{"owner", "member"},
	})
	if err := app.Save(members); err != nil {
		t.Fatal(err)
	}

	member := mailGuestUser(t, app, "member@test.local", "member")
	guest := mailGuestUser(t, app, "guest@test.local", "guest")

	domain := core.NewRecord(domains)
	domain.Set("domain", "acme.test")
	domain.Set("verified", true)
	if err := app.Save(domain); err != nil {
		t.Fatal(err)
	}

	mailbox := core.NewRecord(mailboxes)
	mailbox.Set("address", "team")
	mailbox.Set("domain", domain.Id)
	mailbox.Set("type", "shared")
	if err := app.Save(mailbox); err != nil {
		t.Fatal(err)
	}

	memberToken, err := member.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	guestToken, err := guest.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	return &mailGuestEnv{
		app:         app,
		domain:      domain,
		mailbox:     mailbox,
		member:      member,
		guest:       guest,
		memberToken: memberToken,
		guestToken:  guestToken,
	}
}

func mailGuestUser(t *testing.T, app core.App, email, role string) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("users")
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.Set("name", "Test")
	r.Set("role", role)
	r.SetVerified(true)
	r.SetPassword("Password123!")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func mailSetListView(t *testing.T, app core.App, name, rule string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatal(err)
	}
	col.ListRule = &rule
	col.ViewRule = &rule
	if err := app.Save(col); err != nil {
		t.Fatalf("set list/view on %s: %v", name, err)
	}
}

func mailSetCreate(t *testing.T, app core.App, name, rule string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatal(err)
	}
	col.CreateRule = &rule
	if err := app.Save(col); err != nil {
		t.Fatalf("set create on %s: %v", name, err)
	}
}

func mailRunList(t *testing.T, app *tests.TestApp, name, token string, want, notWant []string) {
	t.Helper()
	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/collections/" + name + "/records",
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       want,
		NotExpectedContent:    notWant,
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// ----- mail_domains read -----

func TestMailGuestRLS_Domains_GuestCannotRead(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetListView(t, env.app, "mail_domains", mailDomainsGuestReadRule)
	mailRunList(t, env.app, "mail_domains", env.guestToken,
		[]string{`"totalItems":0`}, []string{"acme.test"})
}

func TestMailGuestRLS_Domains_MemberCanRead(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetListView(t, env.app, "mail_domains", mailDomainsGuestReadRule)
	mailRunList(t, env.app, "mail_domains", env.memberToken,
		[]string{`"totalItems":1`, "acme.test"}, nil)
}

// ----- mail_mailboxes create -----

func TestMailGuestRLS_Mailboxes_GuestCannotCreate(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetCreate(t, env.app, "mail_mailboxes", mailMailboxesGuestCreateRule)

	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/collections/mail_mailboxes/records",
		Body:                  strings.NewReader(`{"address":"guest","domain":"` + env.domain.Id + `","type":"shared"}`),
		Headers:               map[string]string{"Authorization": env.guestToken, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestMailGuestRLS_Mailboxes_MemberCanCreate(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetCreate(t, env.app, "mail_mailboxes", mailMailboxesGuestCreateRule)

	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/collections/mail_mailboxes/records",
		Body:                  strings.NewReader(`{"address":"sales","domain":"` + env.domain.Id + `","type":"shared"}`),
		Headers:               map[string]string{"Authorization": env.memberToken, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"address":"sales"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// ----- mail_mailbox_aliases read -----

func TestMailGuestRLS_Aliases_GuestCannotRead(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetListView(t, env.app, "mail_mailbox_aliases", mailAliasesGuestReadRule)
	aliasesCol, _ := env.app.FindCollectionByNameOrId("mail_mailbox_aliases")
	a := core.NewRecord(aliasesCol)
	a.Set("mailbox", env.mailbox.Id)
	a.Set("address", "secret-alias")
	if err := env.app.Save(a); err != nil {
		t.Fatal(err)
	}
	mailRunList(t, env.app, "mail_mailbox_aliases", env.guestToken,
		[]string{`"totalItems":0`}, []string{"secret-alias"})
}

func TestMailGuestRLS_Aliases_MemberCanRead(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetListView(t, env.app, "mail_mailbox_aliases", mailAliasesGuestReadRule)
	aliasesCol, _ := env.app.FindCollectionByNameOrId("mail_mailbox_aliases")
	a := core.NewRecord(aliasesCol)
	a.Set("mailbox", env.mailbox.Id)
	a.Set("address", "team-alias")
	if err := env.app.Save(a); err != nil {
		t.Fatal(err)
	}
	mailRunList(t, env.app, "mail_mailbox_aliases", env.memberToken,
		[]string{`"totalItems":1`, "team-alias"}, nil)
}

// ----- mail_mailbox_members bootstrap-first-owner (1830000003) -----
//
// The bootstrap branch of the createRule lets a user self-insert as the first
// owner of a *memberless* mailbox. Migration 1830000003 pins
// `@request.auth.role != "guest"` on that branch so a guest can never take
// ownership. Guests are also blocked transitively (they cannot create a mailbox
// after 1830000002, and pre-existing mailboxes already have owners), but the
// defensive pin closes the gap if a mailbox ever becomes memberless.

func TestMailGuestRLS_MailboxMembers_GuestCannotBootstrapOwner(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetCreate(t, env.app, "mail_mailbox_members", mailMembersGuestCreateRule)

	// The env.mailbox has no members yet — the bootstrap branch would match on
	// `mail_mailbox_members_via_mailbox.id = ""`. Without the role pin the
	// guest's self-insert would satisfy the user check; with it, PB reads
	// role='guest' off the auth record and denies.
	body := `{"mailbox":"` + env.mailbox.Id +
		`","user":"` + env.guest.Id +
		`","role":"owner"}`

	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/collections/mail_mailbox_members/records",
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": env.guestToken, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestMailGuestRLS_MailboxMembers_MemberCanBootstrapOwner(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetCreate(t, env.app, "mail_mailbox_members", mailMembersGuestCreateRule)

	// Positive control: a non-guest must still be allowed to bootstrap-own a
	// memberless mailbox. This is the existing-good path the pin must not regress.
	body := `{"mailbox":"` + env.mailbox.Id +
		`","user":"` + env.member.Id +
		`","role":"owner"}`

	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/collections/mail_mailbox_members/records",
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": env.memberToken, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"role":"owner"`, `"mailbox":"` + env.mailbox.Id + `"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}
