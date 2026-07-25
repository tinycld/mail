package mail

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// The webhook-urls endpoint embeds the domain's webhook_secret in the URLs it
// returns. That secret is the bearer credential for the (unauthenticated)
// inbound/bounce webhooks, so disclosing it lets anyone inject mail for the
// domain. verifyAdmin gates the endpoint: only an admin/owner of the
// domain's org may read it; everyone else must get 403 with no secret in the
// body. These tests drive the *real* route through the real router (auth token
// + requireAuth + the inline handler) so they exercise the guard exactly as
// production does.
const webhookTestSecret = "s3cr3twebhookdomaintoken0001"

// setupWebhookURLsTestApp builds a test app with the collections the endpoint
// touches: an auth collection to mint request tokens (carrying the `role` field
// verifyAdmin reads) and mail_domains (carrying webhook_secret).
func setupWebhookURLsTestApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	// Single-org: the role lives on the auth record itself, so verifyAdmin reads
	// it straight off re.Auth with no membership lookup.
	authCol := core.NewAuthCollection("wh_users")
	authCol.Fields.Add(&core.TextField{Name: "role"})
	if err := app.Save(authCol); err != nil {
		t.Fatalf("failed to save auth collection: %v", err)
	}

	domains := core.NewBaseCollection("mail_domains")
	domains.Fields.Add(&core.TextField{Name: "domain", Required: true})
	domains.Fields.Add(&core.TextField{Name: "webhook_secret"})
	if err := app.Save(domains); err != nil {
		t.Fatalf("failed to save mail_domains: %v", err)
	}

	return app
}

// seedWebhookDomain creates a mail_domains row with a known webhook_secret and
// returns its record ID.
func seedWebhookDomain(t *testing.T, app core.App, domainStr, secret string) string {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatalf("mail_domains collection missing: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("domain", domainStr)
	rec.Set("webhook_secret", secret)
	if err := app.Save(rec); err != nil {
		t.Fatalf("failed to save mail_domains: %v", err)
	}
	return rec.Id
}

// seedAuthUser creates an auth record (usable for minting a request token) and
// returns it.
func seedAuthUser(t *testing.T, app core.App, email, role string) *core.Record {
	t.Helper()
	authCol, err := app.FindCollectionByNameOrId("wh_users")
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

// authTokenFor mints a request auth token for the given user record.
func authTokenFor(t *testing.T, user *core.Record) string {
	t.Helper()
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatalf("failed to mint auth token: %v", err)
	}
	return token
}

// runWebhookURLsScenario drives GET /api/mail/domains/{id}/webhook-urls against
// the *real* production route.
//
// Wiring: TestAppFactory builds + seeds the app, then calls the real
// Register(pb) so the endpoint's OnServe hook is bound before ApiScenario
// triggers OnServe (which registers the route on the router the mux is built
// from). *tests.TestApp only satisfies core.App, so it's wrapped in a
// pocketbase.PocketBase (whose embedded App is the interface Register/
// verifyAdmin actually use). BeforeTestFunc runs after the factory but
// before the request is built, so it's where the resolved URL and Authorization
// header are set. The IMAP/SMTP listeners Register starts at serve time are
// short-circuited with IMAP_ENABLED/SMTP_ENABLED=false so no ports are bound.
func runWebhookURLsScenario(
	t *testing.T,
	name string,
	seed func(app core.App) (domainID string, tokenUser *core.Record),
	expectStatus int,
	expectContent []string,
	notExpectContent []string,
) {
	t.Helper()
	t.Setenv("IMAP_ENABLED", "false")
	t.Setenv("SMTP_ENABLED", "false")

	var domainID string
	var tokenUser *core.Record

	scenario := tests.ApiScenario{
		Name:               name,
		Method:             http.MethodGet,
		ExpectedStatus:     expectStatus,
		ExpectedContent:    expectContent,
		NotExpectedContent: notExpectContent,
		Headers:            map[string]string{},
		TestAppFactory: func(_ testing.TB) *tests.TestApp {
			app := setupWebhookURLsTestApp(t)
			domainID, tokenUser = seed(app)
			// Register the real mail endpoints (incl. webhook-urls) before the
			// scenario triggers OnServe.
			Register(&pocketbase.PocketBase{App: app})
			return app
		},
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		scenario.URL = "/api/mail/domains/" + domainID + "/webhook-urls"
		if tokenUser != nil {
			scenario.Headers["Authorization"] = authTokenFor(t, tokenUser)
		}
	}
	scenario.Test(t)
}

// Happy path: an OWNER reading the webhook URLs gets 200 and the response
// carries the secret-bearing inbound/bounce URLs.
func TestWebhookURLs_OwnerGets200WithSecret(t *testing.T) {
	runWebhookURLsScenario(t, "owner reads webhook urls",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", webhookTestSecret)
			user := seedAuthUser(t, app, "owner@acme.com", "owner")
			return domainID, user
		},
		http.StatusOK,
		[]string{webhookTestSecret, "/api/mail/inbound/", "/api/mail/bounces/"},
		nil,
	)
}

// Happy path: an ADMIN also gets 200 with the secret (admin and owner are
// both allowed by verifyAdmin).
func TestWebhookURLs_AdminGets200WithSecret(t *testing.T) {
	runWebhookURLsScenario(t, "admin reads webhook urls",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", webhookTestSecret)
			user := seedAuthUser(t, app, "admin@acme.com", "admin")
			return domainID, user
		},
		http.StatusOK,
		[]string{webhookTestSecret},
		nil,
	)
}

// KEY REGRESSION: a plain MEMBER must be denied. Before the fix any
// authenticated user got 200 + the secret; now they get 403 and the secret must
// not appear anywhere in the response body.
func TestWebhookURLs_MemberGets403NoSecret(t *testing.T) {
	runWebhookURLsScenario(t, "member is denied",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", webhookTestSecret)
			user := seedAuthUser(t, app, "member@acme.com", "member")
			return domainID, user
		},
		http.StatusForbidden,
		nil,
		[]string{webhookTestSecret},
	)
}

// A GUEST — the least-privileged role — must be denied. (Cross-tenant
// disclosure is no longer a mail concern: single-org means one DB per org and
// the multi-org router owns isolation.)
func TestWebhookURLs_GuestGets403NoSecret(t *testing.T) {
	runWebhookURLsScenario(t, "guest is denied",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", webhookTestSecret)
			user := seedAuthUser(t, app, "guest@acme.com", "guest")
			return domainID, user
		},
		http.StatusForbidden,
		nil,
		[]string{webhookTestSecret},
	)
}

// An authenticated user with NO role at all is likewise denied — verifyAdmin
// reads an empty role and rejects.
func TestWebhookURLs_NoRoleGets403NoSecret(t *testing.T) {
	runWebhookURLsScenario(t, "user with no role is denied",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", webhookTestSecret)
			// deliberately no role set
			user := seedAuthUser(t, app, "stranger@nowhere.com", "")
			return domainID, user
		},
		http.StatusForbidden,
		nil,
		[]string{webhookTestSecret},
	)
}

// Route still binds requireAuth: an unauthenticated request is rejected with
// 401 before the handler runs, and the secret is never disclosed.
func TestWebhookURLs_UnauthenticatedGets401NoSecret(t *testing.T) {
	runWebhookURLsScenario(t, "unauthenticated is rejected",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", webhookTestSecret)
			return domainID, nil // no token user → no Authorization header
		},
		http.StatusUnauthorized,
		nil,
		[]string{webhookTestSecret},
	)
}
