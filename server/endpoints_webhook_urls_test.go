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
// domain. verifyOrgAdmin gates the endpoint: only an admin/owner of the
// domain's org may read it; everyone else must get 403 with no secret in the
// body. These tests drive the *real* route through the real router (auth token
// + requireAuth + the inline handler) so they exercise the guard exactly as
// production does.
const webhookTestSecret = "s3cr3twebhookdomaintoken0001"

// setupWebhookURLsTestApp builds a test app with the collections the endpoint
// touches: an auth collection to mint request tokens, mail_domains (carrying
// org + webhook_secret), and user_org (user↔org membership with a role, which
// verifyOrgAdmin reads).
func setupWebhookURLsTestApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	authCol := core.NewAuthCollection("wh_users")
	if err := app.Save(authCol); err != nil {
		t.Fatalf("failed to save auth collection: %v", err)
	}

	domains := core.NewBaseCollection("mail_domains")
	domains.Fields.Add(&core.TextField{Name: "domain", Required: true})
	domains.Fields.Add(&core.TextField{Name: "org"})
	domains.Fields.Add(&core.TextField{Name: "webhook_secret"})
	if err := app.Save(domains); err != nil {
		t.Fatalf("failed to save mail_domains: %v", err)
	}

	userOrg := core.NewBaseCollection("user_org")
	userOrg.Fields.Add(&core.TextField{Name: "user", Required: true})
	userOrg.Fields.Add(&core.TextField{Name: "org", Required: true})
	userOrg.Fields.Add(&core.TextField{Name: "role"})
	if err := app.Save(userOrg); err != nil {
		t.Fatalf("failed to save user_org: %v", err)
	}

	return app
}

// seedWebhookDomain creates a mail_domains row with a known webhook_secret and
// returns its record ID.
func seedWebhookDomain(t *testing.T, app core.App, domainStr, orgID, secret string) string {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatalf("mail_domains collection missing: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("domain", domainStr)
	rec.Set("org", orgID)
	rec.Set("webhook_secret", secret)
	if err := app.Save(rec); err != nil {
		t.Fatalf("failed to save mail_domains: %v", err)
	}
	return rec.Id
}

// seedAuthUser creates an auth record (usable for minting a request token) and
// returns it.
func seedAuthUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	authCol, err := app.FindCollectionByNameOrId("wh_users")
	if err != nil {
		t.Fatalf("auth collection missing: %v", err)
	}
	user := core.NewRecord(authCol)
	user.Set("email", email)
	user.Set("password", "password12345")
	if err := app.Save(user); err != nil {
		t.Fatalf("failed to save auth user %s: %v", email, err)
	}
	return user
}

// seedUserOrg links a user to an org with a role. verifyOrgAdmin matches on the
// user's auth record ID (== re.Auth.Id at request time).
func seedUserOrg(t *testing.T, app core.App, userID, orgID, role string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("user_org")
	if err != nil {
		t.Fatalf("user_org collection missing: %v", err)
	}
	uo := core.NewRecord(col)
	uo.Set("user", userID)
	uo.Set("org", orgID)
	uo.Set("role", role)
	if err := app.Save(uo); err != nil {
		t.Fatalf("failed to save user_org: %v", err)
	}
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
// verifyOrgAdmin actually use). BeforeTestFunc runs after the factory but
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

// Happy path: an org OWNER reading the webhook URLs gets 200 and the response
// carries the secret-bearing inbound/bounce URLs.
func TestWebhookURLs_OwnerGets200WithSecret(t *testing.T) {
	runWebhookURLsScenario(t, "owner reads webhook urls",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", "orgacme00000001", webhookTestSecret)
			user := seedAuthUser(t, app, "owner@acme.com")
			seedUserOrg(t, app, user.Id, "orgacme00000001", "owner")
			return domainID, user
		},
		http.StatusOK,
		[]string{webhookTestSecret, "/api/mail/inbound/", "/api/mail/bounces/"},
		nil,
	)
}

// Happy path: an org ADMIN also gets 200 with the secret (admin and owner are
// both allowed by verifyOrgAdmin).
func TestWebhookURLs_AdminGets200WithSecret(t *testing.T) {
	runWebhookURLsScenario(t, "admin reads webhook urls",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", "orgacme00000001", webhookTestSecret)
			user := seedAuthUser(t, app, "admin@acme.com")
			seedUserOrg(t, app, user.Id, "orgacme00000001", "admin")
			return domainID, user
		},
		http.StatusOK,
		[]string{webhookTestSecret},
		nil,
	)
}

// KEY REGRESSION: a plain MEMBER of the same org must be denied. Before the
// fix any authenticated user got 200 + the secret; now they get 403 and the
// secret must not appear anywhere in the response body.
func TestWebhookURLs_MemberGets403NoSecret(t *testing.T) {
	runWebhookURLsScenario(t, "member is denied",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", "orgacme00000001", webhookTestSecret)
			user := seedAuthUser(t, app, "member@acme.com")
			seedUserOrg(t, app, user.Id, "orgacme00000001", "member")
			return domainID, user
		},
		http.StatusForbidden,
		nil,
		[]string{webhookTestSecret},
	)
}

// KEY REGRESSION: a user who belongs to a DIFFERENT org (even as its owner)
// must not read this domain's secret. Cross-tenant disclosure was the core of
// the vulnerability. Expect 403, secret absent from the body.
func TestWebhookURLs_CrossOrgOwnerGets403NoSecret(t *testing.T) {
	runWebhookURLsScenario(t, "cross-org owner is denied",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", "orgacme00000001", webhookTestSecret)
			// Owner of a completely different org.
			user := seedAuthUser(t, app, "owner@other.com")
			seedUserOrg(t, app, user.Id, "orgother0000001", "owner")
			return domainID, user
		},
		http.StatusForbidden,
		nil,
		[]string{webhookTestSecret},
	)
}

// A user with NO membership in any org (authenticated, but not a member of the
// domain's org) is likewise denied — resolveUserOrg finds no row → 403.
func TestWebhookURLs_NonMemberGets403NoSecret(t *testing.T) {
	runWebhookURLsScenario(t, "non-member is denied",
		func(app core.App) (string, *core.Record) {
			domainID := seedWebhookDomain(t, app, "acme.com", "orgacme00000001", webhookTestSecret)
			user := seedAuthUser(t, app, "stranger@nowhere.com")
			// deliberately no user_org row
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
			domainID := seedWebhookDomain(t, app, "acme.com", "orgacme00000001", webhookTestSecret)
			return domainID, nil // no token user → no Authorization header
		},
		http.StatusUnauthorized,
		nil,
		[]string{webhookTestSecret},
	)
}
