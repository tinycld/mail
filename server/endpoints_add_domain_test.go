package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/maildomains"
)

// setupAddDomainTestApp builds a test app with the collections
// handleAddDomain touches: an auth collection to mint request tokens
// (carrying the `role` field verifyAdmin reads) and the full mail_domains
// field set the handler writes on enrollment.
func setupAddDomainTestApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	authCol := core.NewAuthCollection("ad_users")
	authCol.Fields.Add(&core.TextField{Name: "role"})
	if err := app.Save(authCol); err != nil {
		t.Fatalf("failed to save auth collection: %v", err)
	}

	domains := core.NewBaseCollection("mail_domains")
	domains.Fields.Add(&core.TextField{Name: "domain", Required: true})
	domains.Fields.Add(&core.BoolField{Name: "verified"})
	domains.Fields.Add(&core.BoolField{Name: "mx_verified"})
	domains.Fields.Add(&core.BoolField{Name: "inbound_domain_verified"})
	domains.Fields.Add(&core.BoolField{Name: "spf_verified"})
	domains.Fields.Add(&core.BoolField{Name: "dkim_verified"})
	domains.Fields.Add(&core.BoolField{Name: "return_path_verified"})
	domains.Fields.Add(&core.JSONField{Name: "provider_domain_metadata", MaxSize: 2000})
	if err := app.Save(domains); err != nil {
		t.Fatalf("failed to save mail_domains: %v", err)
	}

	return app
}

// seedAddDomainAuthUser creates an auth record on this file's own auth
// collection (ad_users, distinct from webhook tests' wh_users) so scenarios
// here don't depend on another test file's collection name.
func seedAddDomainAuthUser(t *testing.T, app core.App, email, role string) *core.Record {
	t.Helper()
	authCol, err := app.FindCollectionByNameOrId("ad_users")
	if err != nil {
		t.Fatalf("auth collection missing: %v", err)
	}
	user := core.NewRecord(authCol)
	user.Set("email", email)
	user.Set("password", "password12345")
	user.Set("role", role)
	if err := app.Save(user); err != nil {
		t.Fatalf("failed to save auth user %s: %v", email, err)
	}
	return user
}

// countMailDomains reports how many mail_domains rows exist. The most
// important assertion in this file is that a failed enrollment creates none.
func countMailDomains(t *testing.T, app core.App) int {
	t.Helper()
	records, err := app.FindAllRecords("mail_domains")
	if err != nil {
		t.Fatalf("failed to list mail_domains: %v", err)
	}
	return len(records)
}

// runAddDomainScenario drives POST /api/mail/domains against the *real*
// production route, mirroring runWebhookURLsScenario in
// endpoints_webhook_urls_test.go: TestAppFactory builds + seeds the app and
// installs the stub registrar, then calls the real Register(pb) so the
// endpoint's OnServe hook is bound before ApiScenario triggers OnServe.
//
// maildomains is process-wide state, so every scenario resets it before
// installing its own stub and via t.Cleanup — otherwise a claim from one
// test leaks into the next and makes its SetResolver inert.
//
// after runs while the test app is still live (ApiScenario's AfterTestFunc,
// which fires before the scenario's own deferred cleanup) — assertions like
// "no row was created" must run here, not after Test() returns, since the
// app's DB is torn down by then.
func runAddDomainScenario(
	t *testing.T,
	name string,
	registrar maildomains.Registrar,
	seed func(app core.App) (tokenUser *core.Record),
	body string,
	expectStatus int,
	expectContent []string,
	notExpectContent []string,
	after func(t *testing.T, app *tests.TestApp),
) {
	t.Helper()
	t.Setenv("IMAP_ENABLED", "false")
	t.Setenv("SMTP_ENABLED", "false")

	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(registrar)

	var tokenUser *core.Record

	scenario := tests.ApiScenario{
		Name:               name,
		Method:             http.MethodPost,
		URL:                "/api/mail/domains",
		Body:               strings.NewReader(body),
		ExpectedStatus:     expectStatus,
		ExpectedContent:    expectContent,
		NotExpectedContent: notExpectContent,
		Headers:            map[string]string{"Content-Type": "application/json"},
		TestAppFactory: func(_ testing.TB) *tests.TestApp {
			app := setupAddDomainTestApp(t)
			tokenUser = seed(app)
			Register(&pocketbase.PocketBase{App: app})
			return app
		},
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		if tokenUser != nil {
			scenario.Headers["Authorization"] = authTokenFor(t, tokenUser)
		}
	}
	if after != nil {
		scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
			after(t, app)
		}
	}
	scenario.Test(t)
}

// Adding a domain enrolls it with the provider AND creates the row, so the
// DNS records exist before the admin is asked to publish anything.
func TestAddDomainEnrollsAndCreatesRecord(t *testing.T) {
	registrar := &stubRegistrar{rec: &maildomains.DomainRecords{
		Domain: "acme.com", ID: 12345, SPFVerified: true, DKIMVerified: true,
		DKIMHost: "sel._domainkey.acme.com", DKIMTextValue: "k=rsa;p=X",
		ReturnPathDomain: "pm-bounces.acme.com", ReturnPathCNAMEValue: "pm.mtasv.net",
	}}

	runAddDomainScenario(t, "admin adds a domain", registrar,
		func(app core.App) *core.Record {
			return seedAddDomainAuthUser(t, app, "admin@acme.com", "admin")
		},
		`{"domain":"acme.com"}`,
		http.StatusOK,
		[]string{`"domain":"acme.com"`, "sel._domainkey.acme.com", "k=rsa;p=X"},
		nil,
		func(t *testing.T, app *tests.TestApp) {
			records, err := app.FindRecordsByFilter("mail_domains", "domain = {:domain}", "", 1, 0,
				map[string]any{"domain": "acme.com"})
			if err != nil {
				t.Fatalf("failed to query mail_domains: %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("expected 1 mail_domains row for acme.com, got %d", len(records))
			}
		},
	)
}

// Non-admins cannot add a domain — same gate as verify.
func TestAddDomainRequiresAdmin(t *testing.T) {
	registrar := &stubRegistrar{rec: &maildomains.DomainRecords{Domain: "acme.com", ID: 1}}

	runAddDomainScenario(t, "member cannot add a domain", registrar,
		func(app core.App) *core.Record {
			return seedAddDomainAuthUser(t, app, "member@acme.com", "member")
		},
		`{"domain":"acme.com"}`,
		http.StatusForbidden,
		[]string{`"message"`},
		nil,
		func(t *testing.T, app *tests.TestApp) {
			if got := countMailDomains(t, app); got != 0 {
				t.Fatalf("expected no mail_domains row to be created, got %d", got)
			}
		},
	)
}

// If enrollment fails, NO row is created: a row without provider enrollment
// is exactly the silent half-broken state this task exists to remove.
func TestAddDomainDoesNotCreateRecordWhenEnrollmentFails(t *testing.T) {
	registrar := &stubRegistrar{err: maildomains.ErrDomainAlreadyEnrolled}

	runAddDomainScenario(t, "enrollment failure creates nothing", registrar,
		func(app core.App) *core.Record {
			return seedAddDomainAuthUser(t, app, "admin@acme.com", "admin")
		},
		`{"domain":"acme.com"}`,
		http.StatusConflict,
		[]string{"already configured on this host"},
		nil,
		func(t *testing.T, app *tests.TestApp) {
			if got := countMailDomains(t, app); got != 0 {
				t.Fatalf("expected no mail_domains row to be created, got %d", got)
			}
		},
	)
}
