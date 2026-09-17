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

// setupDomainGuardApp builds the add-domain test app and then opens
// mail_domains' generic-API rules to admin/owner, exactly as the shipped
// migration (1713000011_mail_domains_allow_owner.js) does. Without this the
// generic record routes 404 and the guard would never be exercised — the
// scenario has to reach the real attack surface to prove anything.
func setupDomainGuardApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := setupAddDomainTestApp(t)

	domains, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatalf("mail_domains missing: %v", err)
	}
	member := `@request.auth.id != ""`
	adminOrOwner := `@request.auth.id != "" && (@request.auth.role = "admin" || @request.auth.role = "owner")`
	domains.ListRule = &member
	domains.ViewRule = &member
	domains.CreateRule = &adminOrOwner
	domains.UpdateRule = &adminOrOwner
	domains.DeleteRule = &adminOrOwner
	if err := app.Save(domains); err != nil {
		t.Fatalf("failed to open mail_domains rules: %v", err)
	}
	return app
}

// seedGuardDomain writes a mail_domains row the way the SERVER does — a plain
// app.Save, which is the internal path handleAddDomain and verifyDomainRecord
// use. That this succeeds at all is half the proof: the guard must catch
// request-driven writes without touching the server's own.
func seedGuardDomain(t *testing.T, app core.App, domain string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatalf("mail_domains missing: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("domain", domain)
	rec.Set("provider_domain_metadata", providerDomainMetadata{
		Postmark: &postmarkDomainMetadata{DomainID: 7, Enrolled: boolPtr(true)},
	})
	if err := app.Save(rec); err != nil {
		t.Fatalf("server-side save of %s failed: %v", domain, err)
	}
	return rec
}

// runDomainGuardScenario drives a generic record-API request (the attack
// surface: PocketBase's own /api/collections/mail_domains/records routes)
// against the real Register() wiring.
// A PATCH scenario's URL needs the seeded row's id, which does not exist
// until TestAppFactory runs, so the URL is built there. url is used verbatim
// when non-empty (the create case); otherwise it is derived from the seeded
// record.
func runDomainGuardScenario(
	t *testing.T,
	name, method, url, body string,
	expectStatus int,
	seed func(app core.App) *core.Record,
	after func(t *testing.T, app *tests.TestApp),
) {
	t.Helper()
	t.Setenv("IMAP_ENABLED", "false")
	t.Setenv("SMTP_ENABLED", "false")

	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)

	var tokenUser *core.Record

	scenario := tests.ApiScenario{
		Name:            name,
		Method:          method,
		Body:            strings.NewReader(body),
		ExpectedStatus:  expectStatus,
		ExpectedContent: []string{`"message"`},
		Headers:         map[string]string{"Content-Type": "application/json"},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		app := setupDomainGuardApp(t)
		tokenUser = seedAddDomainAuthUser(t, app, "admin@attacker.example", "admin")
		scenario.URL = url
		if seed != nil {
			rec := seed(app)
			if scenario.URL == "" {
				scenario.URL = "/api/collections/mail_domains/records/" + rec.Id
			}
		}
		Register(&pocketbase.PocketBase{App: app})
		return app
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

// C1 layer 2: provider_domain_metadata carries the provider's account-scoped
// domain id. On a hosted deployment that account is shared by every org, so a
// tenant that can choose the id can make the router dereference ANOTHER org's
// enrollment with the operator's account token. An org admin is authenticated
// and passes the collection's update rule, so the rule is no defence — the
// field simply is not client state.
func TestMailDomainsGuardRejectsProviderMetadataPatch(t *testing.T) {
	var target *core.Record
	runDomainGuardScenario(t,
		"admin cannot PATCH provider_domain_metadata",
		http.MethodPatch, "", // URL filled in below
		`{"provider_domain_metadata":{"postmark":{"domain_id":42}}}`,
		http.StatusForbidden,
		func(app core.App) *core.Record {
			target = seedGuardDomain(t, app, "attacker.example")
			return target
		},
		func(t *testing.T, app *tests.TestApp) {
			rec, err := app.FindRecordById("mail_domains", target.Id)
			if err != nil {
				t.Fatalf("row vanished: %v", err)
			}
			meta := readProviderDomainMetadata(rec)
			if meta.Postmark == nil || meta.Postmark.DomainID != 7 {
				t.Fatalf("provider domain id = %+v, want the server's 7 — the tenant overwrote it", meta.Postmark)
			}
		},
	)
}

// C2: a direct create bypasses handleAddDomain, so the provider never enrolls
// the domain. The row then renders as configured while every send from it
// fails — the exact regression this branch fixed at the endpoint.
func TestMailDomainsGuardRejectsDirectCreate(t *testing.T) {
	runDomainGuardScenario(t,
		"admin cannot create a mail_domains row directly",
		http.MethodPost, "/api/collections/mail_domains/records",
		`{"domain":"acme.com","verified":true}`,
		http.StatusForbidden,
		nil,
		func(t *testing.T, app *tests.TestApp) {
			if got := countMailDomains(t, app); got != 0 {
				t.Fatalf("expected no mail_domains row, got %d — a direct insert succeeded", got)
			}
		},
	)
}

// C2, second half: `verified` is derived by verifyDomainRecord from live DNS
// and provider checks. A client-set flag is a green badge on a domain nothing
// verified.
func TestMailDomainsGuardRejectsVerifiedPatch(t *testing.T) {
	var target *core.Record
	runDomainGuardScenario(t,
		"admin cannot PATCH verified",
		http.MethodPatch, "",
		`{"verified":true}`,
		http.StatusForbidden,
		func(app core.App) *core.Record {
			target = seedGuardDomain(t, app, "attacker.example")
			return target
		},
		func(t *testing.T, app *tests.TestApp) {
			rec, err := app.FindRecordById("mail_domains", target.Id)
			if err != nil {
				t.Fatalf("row vanished: %v", err)
			}
			if rec.GetBool("verified") {
				t.Fatal("verified = true — the tenant self-certified an unverified domain")
			}
		},
	)
}

// The guard must not block the SERVER. handleAddDomain's app.Save runs the
// internal OnRecordCreate family, not OnRecordCreateRequest, so the create
// hook above never sees it; verifyDomainRecord's save is the same for the
// update hook. Both are asserted here against the real Register() wiring.
func TestMailDomainsGuardAllowsServerWrites(t *testing.T) {
	t.Setenv("IMAP_ENABLED", "false")
	t.Setenv("SMTP_ENABLED", "false")

	app := setupDomainGuardApp(t)
	Register(&pocketbase.PocketBase{App: app})

	// The create path handleAddDomain uses.
	rec := seedGuardDomain(t, app, "acme.com")

	// The update path verifyDomainRecord/checkOutbound uses: both a
	// server-owned metadata write and a derived flag, in one save.
	rec.Set("provider_domain_metadata", providerDomainMetadata{
		Postmark: &postmarkDomainMetadata{DomainID: 99, Enrolled: boolPtr(true)},
	})
	rec.Set("verified", true)
	if err := app.Save(rec); err != nil {
		t.Fatalf("server-side update was blocked by the guard: %v", err)
	}

	fresh, err := app.FindRecordById("mail_domains", rec.Id)
	if err != nil {
		t.Fatalf("row vanished: %v", err)
	}
	if !fresh.GetBool("verified") {
		t.Error("the server's own verified write did not persist")
	}
	meta := readProviderDomainMetadata(fresh)
	if meta.Postmark == nil || meta.Postmark.DomainID != 99 {
		t.Errorf("the server's own provider id write did not persist: %+v", meta.Postmark)
	}
}
