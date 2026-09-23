package mail

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// domainWith builds a mail_domains record with the three outbound DNS flags
// set as given.
func domainWith(t *testing.T, app *tests.TestApp, spf, dkim, returnPath bool) *core.Record {
	t.Helper()

	col, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		col = core.NewBaseCollection("mail_domains")
		col.Fields.Add(&core.TextField{Name: "domain"})
		col.Fields.Add(&core.BoolField{Name: "spf_verified"})
		col.Fields.Add(&core.BoolField{Name: "dkim_verified"})
		col.Fields.Add(&core.BoolField{Name: "return_path_verified"})
		if err := app.Save(col); err != nil {
			t.Fatalf("save mail_domains: %v", err)
		}
	}

	rec := core.NewRecord(col)
	rec.Set("domain", "example.com")
	rec.Set("spf_verified", spf)
	rec.Set("dkim_verified", dkim)
	rec.Set("return_path_verified", returnPath)
	if err := app.Save(rec); err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	return rec
}

func TestCheckDomainAuthenticated_AllowsAFullyVerifiedDomain(t *testing.T) {
	app := setupSendCapApp(t, "")

	if refusal := checkDomainAuthenticated(domainWith(t, app, true, true, true)); refusal != nil {
		t.Errorf("a fully verified domain must be allowed, got %v", refusal)
	}
}

// Each record is independently required: any one missing refuses, and the
// message names which. A sender who cannot tell what to fix will not fix it.
func TestCheckDomainAuthenticated_RefusesEachMissingRecord(t *testing.T) {
	cases := []struct {
		name               string
		spf, dkim, retPath bool
		wants              string
	}{
		{"no SPF", false, true, true, "SPF"},
		{"no DKIM", true, false, true, "DKIM"},
		{"no return-path", true, true, false, "Return-Path"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := setupSendCapApp(t, "")
			refusal := checkDomainAuthenticated(domainWith(t, app, c.spf, c.dkim, c.retPath))
			if refusal == nil {
				t.Fatalf("%s must refuse the send", c.name)
			}
			if refusal.kind != sendErrForbidden {
				t.Errorf("kind = %v, want sendErrForbidden", refusal.kind)
			}
			if !strings.Contains(refusal.msg, c.wants) {
				t.Errorf("refusal should name %q, got %q", c.wants, refusal.msg)
			}
		})
	}
}

// A brand-new domain has none of them, and the message should list all three
// rather than making the sender discover them one at a time.
func TestCheckDomainAuthenticated_NamesEveryMissingRecord(t *testing.T) {
	app := setupSendCapApp(t, "")

	refusal := checkDomainAuthenticated(domainWith(t, app, false, false, false))
	if refusal == nil {
		t.Fatal("an unverified domain must refuse the send")
	}
	for _, want := range []string{"SPF", "DKIM", "Return-Path"} {
		if !strings.Contains(refusal.msg, want) {
			t.Errorf("refusal should name %q, got %q", want, refusal.msg)
		}
	}
}

// A caller that could not resolve a domain is exactly the state this gate
// exists to refuse — it must not be read as "nothing to check, carry on".
func TestCheckDomainAuthenticated_RefusesANilDomain(t *testing.T) {
	refusal := checkDomainAuthenticated(nil)
	if refusal == nil {
		t.Fatal("a nil domain must refuse the send, not pass it")
	}
	if refusal.kind != sendErrForbidden {
		t.Errorf("kind = %v, want sendErrForbidden", refusal.kind)
	}
}

// The gate runs the domain check before the daily cap, so an unverified
// domain is refused for the reason its owner can act on — and the log line
// must not panic on the nil case.
func TestCheckSendAllowed_RefusesAnUnverifiedDomain(t *testing.T) {
	app := setupSendCapApp(t, "")

	refusal := checkSendAllowed(app, "u1", "mb1", domainWith(t, app, true, false, true), 1)
	if refusal == nil {
		t.Fatal("a send from an unverified domain must be refused")
	}
	if !strings.Contains(refusal.msg, "DKIM") {
		t.Errorf("refusal should name the missing record, got %q", refusal.msg)
	}

	if refusal := checkSendAllowed(app, "u1", "mb1", nil, 1); refusal == nil {
		t.Error("a send with no resolved domain must be refused")
	}
}
