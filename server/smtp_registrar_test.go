package mail

import (
	"context"
	"strings"
	"testing"
)

// withTXTLookup stubs DNS for one test. The values below are the ones the
// review captured against live DNS, so these tests reproduce the reported
// defect exactly without depending on the network.
func withTXTLookup(t *testing.T, fn func(ctx context.Context, name string) ([]string, error)) {
	t.Helper()
	orig := txtLookup
	txtLookup = fn
	t.Cleanup(func() { txtLookup = orig })
}

// exampleComDNS answers as example.com does in reality: an SPF record at the
// apex, a published DMARC policy, and no DKIM key.
func exampleComDNS(_ context.Context, name string) ([]string, error) {
	switch name {
	case "example.com":
		return []string{"v=spf1 -all"}, nil
	case "_dmarc.example.com":
		return []string{"v=DMARC1;p=reject;rua=mailto:dmarc@example.com"}, nil
	default:
		return nil, nil
	}
}

// FIX 5: NewSMTPRegistrar must apply the same defaults NewSMTPProvider does.
//
// register.go calls NewSMTPRegistrar(smtpConfigFromSystem(app)) with the RAW
// config, and applyDefaults — which sets DKIMSelector to "tinycld" — used to
// be called only by NewSMTPProvider. The settings field's placeholder is
// literally "tinycld", so an admin who leaves it blank reasonably expects that
// default; instead the DKIM lookup targeted "._domainkey.<domain>", a
// malformed name that can never resolve, leaving DKIM permanently red AND
// printing that malformed host as a record to publish.
func TestNewSMTPRegistrarAppliesDKIMSelectorDefault(t *testing.T) {
	withTXTLookup(t, exampleComDNS)

	rec, err := NewSMTPRegistrar(SMTPConfig{}).GetDomain(context.Background(), "example.com", 0)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}

	if strings.HasPrefix(rec.DKIMHost, ".") {
		t.Fatalf("DKIMHost = %q — a malformed name that can never resolve; "+
			"the zero config must pick up the documented \"tinycld\" default", rec.DKIMHost)
	}
	if rec.DKIMHost != "tinycld._domainkey.example.com" {
		t.Fatalf("DKIMHost = %q, want %q", rec.DKIMHost, "tinycld._domainkey.example.com")
	}
}

// An explicitly configured selector must still win over the default.
func TestNewSMTPRegistrarKeepsConfiguredDKIMSelector(t *testing.T) {
	withTXTLookup(t, exampleComDNS)

	rec, err := NewSMTPRegistrar(SMTPConfig{DKIMSelector: "s1"}).
		GetDomain(context.Background(), "example.com", 0)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if rec.DKIMHost != "s1._domainkey.example.com" {
		t.Fatalf("DKIMHost = %q, want the configured selector to win", rec.DKIMHost)
	}
}

// FIX 4: the SMTP path must not emit a DMARC TXT value labelled as a
// Return-Path CNAME.
//
// ReturnPathDomain/ReturnPathCNAMEValue mean "publish a CNAME at this host,
// pointing at this target" — DnsRecordsPanel renders them under "DNS records
// to publish" as `Return-Path (CNAME)` with a copy button. Filling them from
// the _dmarc TXT lookup produced host="_dmarc.example.com",
// value="v=DMARC1;p=reject;..." — so an admin following the panel creates a
// CNAME whose target is a DMARC policy string (invalid DNS) and, if they
// replace the existing TXT, DESTROYS their DMARC policy.
func TestSMTPRegistrarEmitsNoReturnPathRecordToPublish(t *testing.T) {
	withTXTLookup(t, exampleComDNS)

	rec, err := NewSMTPRegistrar(SMTPConfig{}).GetDomain(context.Background(), "example.com", 0)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}

	if rec.ReturnPathDomain != "" {
		t.Errorf("ReturnPathDomain = %q, want empty: a self-hosted deployment has no "+
			"return-path CNAME to publish, and this field is rendered as one",
			rec.ReturnPathDomain)
	}
	if rec.ReturnPathCNAMEValue != "" {
		t.Errorf("ReturnPathCNAMEValue = %q, want empty: this is rendered as a CNAME "+
			"TARGET, and a DMARC policy string is not one", rec.ReturnPathCNAMEValue)
	}

	// The DMARC lookup must still RUN and still inform ReturnPathVerified —
	// that proxy is reasonable and was kept deliberately. Deleting the lookup
	// along with the bad fields would pass the two assertions above, so this
	// pins the half that had to survive.
	if !rec.ReturnPathVerified {
		t.Error("ReturnPathVerified = false although a DMARC policy is published; " +
			"the DMARC lookup must still inform this flag")
	}
}

// The mirror image: with no DMARC published, the advisory flag goes false.
// Without this, ReturnPathVerified could be hardcoded true and pass above.
func TestSMTPRegistrarReturnPathUnverifiedWithoutDMARC(t *testing.T) {
	withTXTLookup(t, func(_ context.Context, name string) ([]string, error) {
		if name == "example.com" {
			return []string{"v=spf1 -all"}, nil
		}
		return nil, nil
	})

	rec, err := NewSMTPRegistrar(SMTPConfig{}).GetDomain(context.Background(), "example.com", 0)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if rec.ReturnPathVerified {
		t.Error("ReturnPathVerified = true with no DMARC record published")
	}
	if !rec.SPFVerified {
		t.Error("SPFVerified = false although an SPF record is published")
	}
}

// A real DKIM key IS publishable and must survive — the fix removed the
// return-path fields, not the DKIM ones.
func TestSMTPRegistrarStillEmitsDKIMRecordToPublish(t *testing.T) {
	withTXTLookup(t, func(_ context.Context, name string) ([]string, error) {
		if name == "tinycld._domainkey.example.com" {
			return []string{"v=DKIM1; k=rsa; p=MIIBIj"}, nil
		}
		return nil, nil
	})

	rec, err := NewSMTPRegistrar(SMTPConfig{}).GetDomain(context.Background(), "example.com", 0)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if !rec.DKIMVerified || rec.DKIMTextValue != "v=DKIM1; k=rsa; p=MIIBIj" {
		t.Fatalf("DKIM record lost: verified=%v value=%q", rec.DKIMVerified, rec.DKIMTextValue)
	}
}
