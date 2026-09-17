package mail

import (
	"context"
	"net"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/maildomains"
)

// FIX 6: the hourly reverify ticker must honour the same provider-configured
// guard the verify ENDPOINT already applies.
//
// handleVerifyDomain refuses to run when the provider is unconfigured, and
// documents exactly why: "return a targeted 400 instead of persisting
// misleading per-check failures". reverifyUnconfirmedDomains called
// verifyDomainRecord directly with no such check, so on a deployment with no
// provider configured every domain row got stamped — 30s after boot and hourly
// after — with failures attributing the problem to the admin's DNS rather than
// the missing config. That is precisely what the endpoint's guard exists to
// prevent, reintroduced on a timer nobody triggered.
func TestReverifyUnconfirmedDomainsSkipsWhenProviderUnconfigured(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(&stubRegistrar{err: maildomains.ErrNotConfigured})

	// Any MX answer at all; the point is that the check must never run.
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{{Host: "mx.somewhere-else.example.", Pref: 10}}, nil
	})

	app := newReverifyTestApp(t)
	// No mail.provider and no credentials: newProviderFromSystem defaults to
	// Postmark with an empty token, so Configured() is false — the exact state
	// the endpoint guard refuses to run in.
	record := newReverifyTestRecord(t, app)

	reverifyUnconfirmedDomains(context.Background(), app)

	saved, err := app.FindRecordById("mail_domains", record.Id)
	if err != nil {
		t.Fatalf("reload record: %v", err)
	}
	if saved.GetString("last_checked_at") != "" {
		t.Fatalf("last_checked_at = %q — the ticker stamped a check although no "+
			"provider is configured; every per-check failure it persists blames "+
			"the admin's DNS for a missing-credentials problem",
			saved.GetString("last_checked_at"))
	}
	// An untouched JSON field reads back as the literal "null"; anything else
	// means a verdict was persisted for checks that should never have run.
	if details := saved.GetString("verification_details"); details != "" && details != "null" {
		t.Errorf("verification_details = %s, want the row left untouched", details)
	}
}

// The guard must not disable the ticker outright: with a provider configured,
// the run proceeds and stamps the row as before.
func TestReverifyUnconfirmedDomainsRunsWhenProviderConfigured(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(&stubRegistrar{rec: &maildomains.DomainRecords{
		Domain: "acme.com", ID: 1,
	}})

	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{{Host: "inbound.postmarkapp.com.", Pref: 10}}, nil
	})

	app := newReverifyTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "postmark")
	saveSystemSetting(t, app, "mail.postmark_server_token", "tok")
	record := newReverifyTestRecord(t, app)

	reverifyUnconfirmedDomains(context.Background(), app)

	saved, err := app.FindRecordById("mail_domains", record.Id)
	if err != nil {
		t.Fatalf("reload record: %v", err)
	}
	if saved.GetString("last_checked_at") == "" {
		t.Fatal("last_checked_at is empty — the ticker did not run although the " +
			"provider is configured; the guard must skip only the unconfigured case")
	}
}

func newReverifyTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := setupSettingsTestApp(t)

	col := core.NewBaseCollection("mail_domains")
	col.Fields.Add(&core.TextField{Name: "domain", Required: true})
	col.Fields.Add(&core.BoolField{Name: "verified"})
	col.Fields.Add(&core.BoolField{Name: "mx_verified"})
	col.Fields.Add(&core.BoolField{Name: "inbound_domain_verified"})
	col.Fields.Add(&core.BoolField{Name: "spf_verified"})
	col.Fields.Add(&core.BoolField{Name: "dkim_verified"})
	col.Fields.Add(&core.BoolField{Name: "return_path_verified"})
	col.Fields.Add(&core.TextField{Name: "last_checked_at"})
	col.Fields.Add(&core.JSONField{Name: "provider_domain_metadata", MaxSize: 2000})
	col.Fields.Add(&core.JSONField{Name: "verification_details", MaxSize: 4000})
	if err := app.Save(col); err != nil {
		t.Fatalf("save mail_domains collection: %v", err)
	}
	return app
}

func newReverifyTestRecord(t *testing.T, app core.App) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatalf("find mail_domains: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("domain", "acme.com")
	rec.Set("verified", false)
	if err := app.Save(rec); err != nil {
		t.Fatalf("save record: %v", err)
	}
	return rec
}
