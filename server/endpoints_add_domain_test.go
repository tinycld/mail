package mail

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/maildomains"
	"tinycld.org/packages/mail/api"
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
	domains.Fields.Add(&core.JSONField{Name: "verification_details", MaxSize: 4000})
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
		// Register only BINDS its OnServe handlers inside TestAppFactory; they
		// don't run until ApiScenario triggers OnServe below, which is what
		// calls Register's own boot-time reconcileMailDomainsRegistrar (see
		// register.go) — it installs a real Postmark registrar by default
		// (system_settings has no mail.provider row here, so it defaults to
		// "postmark"), and that now happens for every provider, not just
		// smtp, since I1 was fixed to reconcile both branches. BeforeTestFunc
		// runs as the terminal handler of that same OnServe chain (see
		// ApiScenario.test: OnServe().Trigger(event, beforeTestFunc)), i.e.
		// AFTER reconcileMailDomainsRegistrar's handler has already run via
		// e.Next() — so setting the stub here, not in TestAppFactory, is what
		// makes it the one handleAddDomain actually calls through to.
		maildomains.SetResolver(registrar)
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

// readVerificationDetails unmarshals a mail_domains row's verification_details
// JSON field, mirroring readProviderDomainMetadata's marshal-roundtrip since
// core.Record.Get returns the field as a generic any, not the typed struct.
func readVerificationDetails(t *testing.T, record *core.Record) *api.VerificationDetails {
	t.Helper()
	raw := record.Get("verification_details")
	if raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("failed to marshal verification_details: %v", err)
	}
	var details api.VerificationDetails
	if err := json.Unmarshal(data, &details); err != nil {
		t.Fatalf("failed to unmarshal verification_details: %v", err)
	}
	return &details
}

// Adding a domain enrolls it with the provider AND creates the row, so the
// DNS records exist before the admin is asked to publish anything. An admin
// who never presses Verify must still be able to read verification_details
// off the row — this is the field the settings UI's DNS panel reads, and it
// used to stay empty until the first Verify.
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

			details := readVerificationDetails(t, records[0])
			if details == nil {
				t.Fatalf("expected verification_details to be populated on add, got nil")
			}
			outbound := details.Outbound
			if outbound.Enrolled != "yes" {
				t.Fatalf("expected outbound.enrolled = yes, got %q", outbound.Enrolled)
			}
			if outbound.DKIMHost != "sel._domainkey.acme.com" || outbound.DKIMTextValue != "k=rsa;p=X" {
				t.Fatalf("expected DKIM host/value on outbound, got %+v", outbound)
			}
			if outbound.ReturnPathDomain != "pm-bounces.acme.com" || outbound.ReturnPathCNAMEValue != "pm.mtasv.net" {
				t.Fatalf("expected return-path domain/value on outbound, got %+v", outbound)
			}
		},
	)
}

// The admin must see the MX target to publish before ever pressing Verify —
// enrollment succeeds without the MX check having run, so mx_host must be
// filled from expectedInboundMXHost directly, not read back off a check.
func TestAddDomainIncludesExpectedMXHost(t *testing.T) {
	registrar := &stubRegistrar{rec: &maildomains.DomainRecords{Domain: "acme.com", ID: 1}}

	runAddDomainScenario(t, "add response includes mx_host", registrar,
		func(app core.App) *core.Record {
			return seedAddDomainAuthUser(t, app, "admin@acme.com", "admin")
		},
		`{"domain":"acme.com"}`,
		http.StatusOK,
		[]string{`"mx_host":"inbound.postmarkapp.com"`},
		nil,
		nil,
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
		[]string{"domain_exists"},
		func(t *testing.T, app *tests.TestApp) {
			if got := countMailDomains(t, app); got != 0 {
				t.Fatalf("expected no mail_domains row to be created, got %d", got)
			}
		},
	)
}

// A GENERIC provider failure (a 5xx from the provider, a socket error — the
// fall-through case) must create nothing either. Only the
// ErrDomainAlreadyEnrolled path was pinned before, so the switch could fall
// through to the save and the suite stayed green: a row the provider has
// never heard of, which is the exact half-broken state this handler exists
// to prevent.
func TestAddDomainDoesNotCreateRecordOnGenericProviderError(t *testing.T) {
	registrar := &stubRegistrar{err: errors.New("postmark create domain: 500 Internal Server Error")}

	runAddDomainScenario(t, "generic provider failure creates nothing", registrar,
		func(app core.App) *core.Record {
			return seedAddDomainAuthUser(t, app, "admin@acme.com", "admin")
		},
		`{"domain":"acme.com"}`,
		http.StatusBadGateway,
		[]string{"Failed to enroll the domain with the mail provider"},
		// The raw provider text must not leak to an org admin.
		[]string{"Internal Server Error"},
		func(t *testing.T, app *tests.TestApp) {
			if got := countMailDomains(t, app); got != 0 {
				t.Fatalf("expected no mail_domains row after a provider error, got %d", got)
			}
		},
	)
}

// No credentials is the DEFAULT state of every fresh standalone install, so
// this is the path an operator is most likely to hit first. It must 503 and
// create nothing — a row saved here would display as a configured domain on
// a deployment that cannot provision domains at all.
func TestAddDomainDoesNotCreateRecordWhenProvisioningNotConfigured(t *testing.T) {
	registrar := &stubRegistrar{err: maildomains.ErrNotConfigured}

	runAddDomainScenario(t, "unconfigured provisioning creates nothing", registrar,
		func(app core.App) *core.Record {
			return seedAddDomainAuthUser(t, app, "admin@acme.com", "admin")
		},
		`{"domain":"acme.com"}`,
		http.StatusServiceUnavailable,
		[]string{"not configured for this deployment"},
		nil,
		func(t *testing.T, app *tests.TestApp) {
			if got := countMailDomains(t, app); got != 0 {
				t.Fatalf("expected no mail_domains row when provisioning is unconfigured, got %d", got)
			}
		},
	)
}

// AMENDMENT 1 / the new migration's entire justification: the created row must
// carry the provider's own domain id. Without it every later status check
// falls back to the paged by-name scan, which on a shared provider account
// reports a domain past the first page as unenrolled. The enrollment state is
// asserted alongside it so a write to the wrong field name — or to the right
// field with the id dropped — fails here.
func TestAddDomainStoresProviderDomainMetadata(t *testing.T) {
	registrar := &stubRegistrar{rec: &maildomains.DomainRecords{
		Domain: "acme.com", ID: 12345, DKIMVerified: true, ReturnPathVerified: true,
	}}

	runAddDomainScenario(t, "add stamps provider_domain_metadata", registrar,
		func(app core.App) *core.Record {
			return seedAddDomainAuthUser(t, app, "admin@acme.com", "admin")
		},
		`{"domain":"acme.com"}`,
		http.StatusOK,
		[]string{`"domain":"acme.com"`},
		nil,
		func(t *testing.T, app *tests.TestApp) {
			records, err := app.FindRecordsByFilter("mail_domains", "domain = {:domain}", "", 1, 0,
				map[string]any{"domain": "acme.com"})
			if err != nil {
				t.Fatalf("failed to query mail_domains: %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("expected 1 mail_domains row, got %d", len(records))
			}

			meta := readProviderDomainMetadata(records[0])
			if meta.Postmark == nil {
				t.Fatalf("provider_domain_metadata.postmark missing on the created row; "+
					"raw field = %#v", records[0].Get("provider_domain_metadata"))
			}
			if meta.Postmark.DomainID != 12345 {
				t.Fatalf("provider_domain_metadata.postmark.domain_id = %d, want 12345 — "+
					"without the stored id every later check falls back to the paged by-name scan",
					meta.Postmark.DomainID)
			}
			if meta.Postmark.Enrolled == nil || !*meta.Postmark.Enrolled {
				t.Fatalf("provider_domain_metadata.postmark.enrolled = %+v, want a pointer to true",
					meta.Postmark.Enrolled)
			}
			if meta.Postmark.CheckedAt == "" {
				t.Error("provider_domain_metadata.postmark.checked_at must be stamped on the add path")
			}
			if meta.Postmark.DKIMVerified == nil || !*meta.Postmark.DKIMVerified {
				t.Errorf("dkim_verified = %+v, want a pointer to true", meta.Postmark.DKIMVerified)
			}
			if meta.Postmark.ReturnPathVerified == nil || !*meta.Postmark.ReturnPathVerified {
				t.Errorf("return_path_verified = %+v, want a pointer to true", meta.Postmark.ReturnPathVerified)
			}
		},
	)
}

// A domain this deployment already holds is refused before the provider is
// asked, with a machine code a client can tell apart from the provider-side
// duplicate (which is also a 409).
func TestAddDomainExistingRowIsDomainExists(t *testing.T) {
	registrar := &stubRegistrar{err: errors.New("provider must not be called")}

	runAddDomainScenario(t, "re-adding an existing domain", registrar,
		func(app core.App) *core.Record {
			col, err := app.FindCollectionByNameOrId("mail_domains")
			if err != nil {
				t.Fatal(err)
			}
			row := core.NewRecord(col)
			row.Set("domain", "acme.com")
			if err := app.Save(row); err != nil {
				t.Fatal(err)
			}
			return seedAddDomainAuthUser(t, app, "admin@acme.com", "admin")
		},
		`{"domain":"ACME.com"}`,
		http.StatusConflict,
		[]string{`"code":"domain_exists"`},
		nil,
		func(t *testing.T, app *tests.TestApp) {
			if got := countMailDomains(t, app); got != 1 {
				t.Fatalf("expected the existing row only, got %d", got)
			}
		},
	)
}
