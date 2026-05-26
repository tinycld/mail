package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// guest_rls_test.go proves the mail-infra access rules tightened by the
// 1713000020 migration against PocketBase's REAL rule engine. A role='guest'
// member of an org must be DENIED while a real member is still ALLOWED.
//
// Background: a guest share-link visitor gets a real users record + a user_org
// row with role='guest' in the owner's org. Several mail-infra rules granted
// access to ANY org member regardless of role:
//   - mail_domains   list/view  (read leak: guest enumerates mail domains)
//   - mail_labels    all CRUD   (read + write leak)
//   - mail_mailboxes create     (write leak: guest creates a mailbox)
//   - mail_mailbox_aliases list/view (read leak: guest enumerates aliases)
//
// Each tightened rule pins the role check to the SAME relation-path prefix as
// the user check, so PB applies both to the same joined user_org row (verified
// here and in core/server/coreserver/guest_rls_test.go).
//
// NOTE the rules left UNCHANGED (already safe): mail_domains create/update/
// delete + mail_mailbox_aliases create/update/delete are admin/owner-gated;
// mail_mailboxes list/view is mailbox-membership-gated (a guest is not a
// mailbox member). mail_mailbox_members create's bootstrap branch is the one
// deferred item — see the migration's header note.
//
// Each scenario builds a FRESH TestApp (ApiScenario.Test re-triggers OnServe;
// reusing one app panics on duplicate route registration under PB v0.38.1).

// Rule strings mirror the 1713000020 migration verbatim.
const (
	mailDomainsGuestReadRule = `org.user_org_via_org.user ?= @request.auth.id && ` +
		`org.user_org_via_org.role ?!= "guest"`
	mailLabelsGuestRule = `org.user_org_via_org.user ?= @request.auth.id && ` +
		`org.user_org_via_org.role ?!= "guest"`
	mailMailboxesGuestCreateRule = `domain.org.user_org_via_org.user ?= @request.auth.id && ` +
		`domain.org.user_org_via_org.role ?!= "guest"`
	mailAliasesGuestReadRule = `@request.auth.id != "" && ` +
		`mailbox.domain.org.user_org_via_org.user ?= @request.auth.id && ` +
		`mailbox.domain.org.user_org_via_org.role ?!= "guest"`
)

type mailGuestEnv struct {
	app         *tests.TestApp
	org         *core.Record
	domain      *core.Record
	mailbox     *core.Record
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

	orgs := core.NewBaseCollection("orgs")
	orgs.Id = "pbc_orgs_00001"
	orgs.Fields.Add(&core.TextField{Name: "name", Required: true})
	orgs.Fields.Add(&core.TextField{Name: "slug", Required: true})
	if err := app.Save(orgs); err != nil {
		t.Fatal(err)
	}

	userOrg := core.NewBaseCollection("user_org")
	userOrg.Id = "pbc_user_org_01"
	userOrg.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	userOrg.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	userOrg.Fields.Add(&core.SelectField{
		Name: "role", Required: true, MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	if err := app.Save(userOrg); err != nil {
		t.Fatal(err)
	}

	domains := core.NewBaseCollection("mail_domains")
	domains.Id = "pbc_mail_domains_01"
	domains.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
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

	labels := core.NewBaseCollection("mail_labels")
	labels.Id = "pbc_mail_labels_01"
	labels.Fields.Add(&core.RelationField{
		Name: "org", Required: true, CollectionId: orgs.Id,
		CascadeDelete: true, MaxSelect: 1,
	})
	labels.Fields.Add(&core.TextField{Name: "name", Required: true})
	labels.Fields.Add(&core.TextField{Name: "color"})
	if err := app.Save(labels); err != nil {
		t.Fatal(err)
	}

	org := core.NewRecord(orgs)
	org.Set("name", "Acme")
	org.Set("slug", "acme")
	if err := app.Save(org); err != nil {
		t.Fatal(err)
	}

	member := mailGuestUser(t, app, "member@test.local")
	guest := mailGuestUser(t, app, "guest@test.local")
	mailGuestMembership(t, app, member, org, "member")
	mailGuestMembership(t, app, guest, org, "guest")

	domain := core.NewRecord(domains)
	domain.Set("org", org.Id)
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
		org:         org,
		domain:      domain,
		mailbox:     mailbox,
		memberToken: memberToken,
		guestToken:  guestToken,
	}
}

func mailGuestUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("users")
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.Set("name", "Test")
	r.SetVerified(true)
	r.SetPassword("Password123!")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func mailGuestMembership(t *testing.T, app core.App, user, org *core.Record, role string) {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("user_org")
	r := core.NewRecord(col)
	r.Set("user", user.Id)
	r.Set("org", org.Id)
	r.Set("role", role)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
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

// ----- mail_labels read + create -----

func TestMailGuestRLS_Labels_GuestCannotRead(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetAllCRUD(t, env.app, "mail_labels", mailLabelsGuestRule)
	labelsCol, _ := env.app.FindCollectionByNameOrId("mail_labels")
	lbl := core.NewRecord(labelsCol)
	lbl.Set("org", env.org.Id)
	lbl.Set("name", "Receipts")
	if err := env.app.Save(lbl); err != nil {
		t.Fatal(err)
	}
	mailRunList(t, env.app, "mail_labels", env.guestToken,
		[]string{`"totalItems":0`}, []string{"Receipts"})
}

func TestMailGuestRLS_Labels_MemberCanRead(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetAllCRUD(t, env.app, "mail_labels", mailLabelsGuestRule)
	labelsCol, _ := env.app.FindCollectionByNameOrId("mail_labels")
	lbl := core.NewRecord(labelsCol)
	lbl.Set("org", env.org.Id)
	lbl.Set("name", "Receipts")
	if err := env.app.Save(lbl); err != nil {
		t.Fatal(err)
	}
	mailRunList(t, env.app, "mail_labels", env.memberToken,
		[]string{`"totalItems":1`, "Receipts"}, nil)
}

func TestMailGuestRLS_Labels_GuestCannotCreate(t *testing.T) {
	env := setupMailGuestApp(t)
	mailSetAllCRUD(t, env.app, "mail_labels", mailLabelsGuestRule)

	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/collections/mail_labels/records",
		Body:                  strings.NewReader(`{"org":"` + env.org.Id + `","name":"X"}`),
		Headers:               map[string]string{"Authorization": env.guestToken, "Content-Type": "application/json"},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
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

func mailSetAllCRUD(t *testing.T, app core.App, name, rule string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatal(err)
	}
	col.ListRule = &rule
	col.ViewRule = &rule
	col.CreateRule = &rule
	col.UpdateRule = &rule
	col.DeleteRule = &rule
	if err := app.Save(col); err != nil {
		t.Fatalf("set all-CRUD on %s: %v", name, err)
	}
}
