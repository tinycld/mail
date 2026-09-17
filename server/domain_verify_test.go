package mail

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/maildomains"
)

func withMXLookup(t *testing.T, fn func(ctx context.Context, name string) ([]*net.MX, error)) {
	t.Helper()
	orig := mxLookup
	mxLookup = fn
	t.Cleanup(func() { mxLookup = orig })
}

func TestCheckMX_MatchesPostmarkHost(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{{Host: "inbound.postmarkapp.com.", Pref: 10}}, nil
	})
	got := checkMX(context.Background(), "example.com", postmarkInboundMXHost)
	if !got.OK {
		t.Fatalf("expected MX check OK, got %+v", got)
	}
	if len(got.Actual) != 1 || got.Actual[0] != "inbound.postmarkapp.com (pref 10)" {
		t.Fatalf("expected normalized actual host with pref, got %+v", got.Actual)
	}
}

func TestCheckMX_RejectsWrongHost(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{{Host: "mail.google.com.", Pref: 10}}, nil
	})
	got := checkMX(context.Background(), "example.com", postmarkInboundMXHost)
	if got.OK {
		t.Fatalf("expected MX check to fail, got OK")
	}
}

func TestCheckMX_EmptyRecords(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return nil, nil
	})
	got := checkMX(context.Background(), "example.com", postmarkInboundMXHost)
	if got.OK {
		t.Fatalf("expected failure on empty MX set")
	}
	if got.Error == "" {
		t.Fatalf("expected error message for empty MX set")
	}
}

func TestCheckMX_LookupError(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return nil, errors.New("boom")
	})
	got := checkMX(context.Background(), "example.com", postmarkInboundMXHost)
	if got.OK || got.Error != "boom" {
		t.Fatalf("expected error passthrough, got %+v", got)
	}
}

func TestCheckMX_MultipleRecordsOneMatches(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{
			{Host: "alt.example.com.", Pref: 20},
			{Host: "INBOUND.POSTMARKAPP.COM.", Pref: 10},
		}, nil
	})
	got := checkMX(context.Background(), "example.com", postmarkInboundMXHost)
	if !got.OK {
		t.Fatalf("expected match on case-insensitive host; got %+v", got)
	}
}

// SMTP IMAP-fetch mode publishes no MX target on our side — checkMX must
// report OK without even consulting DNS so the UI doesn't render a spurious
// "no MX records" error for an org that's intentionally pulling mail.
func TestCheckMX_EmptyExpectedSkipsLookup(t *testing.T) {
	called := false
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		called = true
		return nil, nil
	})
	got := checkMX(context.Background(), "example.com", "")
	if !got.OK {
		t.Fatalf("expected OK when expected MX host is empty; got %+v", got)
	}
	if called {
		t.Fatalf("expected MX lookup to be skipped when expected host is empty")
	}
}

func TestExpectedInboundMXHost_PerProvider(t *testing.T) {
	if got := expectedInboundMXHost(NewPostmarkProvider("tok", "")); got != postmarkInboundMXHost {
		t.Errorf("postmark: got %q, want %q", got, postmarkInboundMXHost)
	}
	if got := expectedInboundMXHost(NewSMTPProvider(SMTPConfig{PublicHostname: "mx.example.com", InboundMode: "smtp"})); got != "mx.example.com" {
		t.Errorf("smtp/listener: got %q, want %q", got, "mx.example.com")
	}
	if got := expectedInboundMXHost(NewSMTPProvider(SMTPConfig{PublicHostname: "mx.example.com", InboundMode: "imap"})); got != "" {
		t.Errorf("smtp/imap-fetch: got %q, want empty (no MX on our side)", got)
	}
	if got := expectedInboundMXHost(&NoopProvider{}); got != "" {
		t.Errorf("noop: got %q, want empty", got)
	}
}

// --- Provider check tests (pure — no DB involved) ---

type fakeProvider struct {
	inboundDomain  string
	inboundAddress string
	inboundErr     error
}

func (f *fakeProvider) Configured() bool { return true }
func (f *fakeProvider) Send(context.Context, *SendRequest) (*SendResult, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeProvider) ParseInbound([]byte) (*InboundMessage, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeProvider) ParseBounce([]byte) (*BounceEvent, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeProvider) VerifyWebhookSignature(map[string]string, []byte) error { return nil }
func (f *fakeProvider) CheckInboundDomain(context.Context) (*InboundVerification, error) {
	if f.inboundErr != nil {
		return nil, f.inboundErr
	}
	return &InboundVerification{
		ServerInboundDomain: f.inboundDomain,
		InboundAddress:      f.inboundAddress,
	}, nil
}

func TestCheckProviderInboundStrict_CaseInsensitiveMatch(t *testing.T) {
	p := &fakeProvider{inboundDomain: "INBOUND.Example.COM", inboundAddress: "hi@inbound.example.com"}
	got := checkProviderInboundStrict(context.Background(), p, "inbound.example.com", true)
	if !got.OK {
		t.Fatalf("expected case-insensitive match; got %+v", got)
	}
	if got.InboundAddress != "hi@inbound.example.com" {
		t.Fatalf("expected inbound address surfaced; got %q", got.InboundAddress)
	}
}

func TestCheckProviderInboundStrict_EmptyServerDomain(t *testing.T) {
	p := &fakeProvider{inboundDomain: ""}
	got := checkProviderInboundStrict(context.Background(), p, "example.com", true)
	if got.OK {
		t.Fatalf("expected fail when server InboundDomain is empty")
	}
}

func TestCheckProviderInboundStrict_DomainMismatch(t *testing.T) {
	p := &fakeProvider{inboundDomain: "other.example.com"}
	got := checkProviderInboundStrict(context.Background(), p, "example.com", true)
	if got.OK {
		t.Fatalf("expected fail on domain mismatch under strict mode")
	}
	if got.ServerDomain != "other.example.com" {
		t.Fatalf("expected server domain surfaced; got %q", got.ServerDomain)
	}
}

// Non-strict (SMTP-style) verification accepts any non-empty server domain —
// the operator's PublicHostname rarely equals each tenant's domain, and the
// MX check is the actual proof that mail will arrive.
func TestCheckProviderInboundStrict_NonStrictAcceptsAnyHostname(t *testing.T) {
	p := &fakeProvider{inboundDomain: "mx.operator.example", inboundAddress: "mx@operator.example"}
	got := checkProviderInboundStrict(context.Background(), p, "tenant.example", false)
	if !got.OK {
		t.Fatalf("expected non-strict to accept any non-empty hostname; got %+v", got)
	}
}

func TestCheckProviderInboundStrict_NonStrictStillRejectsEmpty(t *testing.T) {
	p := &fakeProvider{inboundDomain: ""}
	got := checkProviderInboundStrict(context.Background(), p, "tenant.example", false)
	if got.OK {
		t.Fatalf("expected non-strict to still reject empty server domain")
	}
}

func TestCheckProviderInboundStrict_ProviderError(t *testing.T) {
	p := &fakeProvider{inboundErr: errors.New("403")}
	got := checkProviderInboundStrict(context.Background(), p, "example.com", true)
	if got.OK {
		t.Fatalf("expected fail on provider error")
	}
	if got.Error != "403" {
		t.Fatalf("expected error passthrough; got %q", got.Error)
	}
}

// --- checkOutbound tests (the maildomains seam, not a Provider) ---

// stubRegistrar is a maildomains.Registrar test double. GetDomain ignores the
// providerDomainID it's passed except to record it, so tests can assert
// checkOutbound forwarded the id it read off the record.
type stubRegistrar struct {
	rec         *maildomains.DomainRecords
	err         error
	gotDomainID int64
}

func (s *stubRegistrar) AddDomain(context.Context, string) (*maildomains.DomainRecords, error) {
	return s.rec, s.err
}
func (s *stubRegistrar) GetDomain(_ context.Context, _ string, providerDomainID int64) (*maildomains.DomainRecords, error) {
	s.gotDomainID = providerDomainID
	return s.rec, s.err
}

// newOutboundTestApp builds a minimal in-memory app with a mail_domains
// collection carrying just the two fields checkOutbound touches: domain and
// provider_domain_metadata. Pure unit test — no migrations, no HTTP.
func newOutboundTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	col := core.NewBaseCollection("mail_domains")
	col.Fields.Add(&core.TextField{Name: "domain", Required: true})
	col.Fields.Add(&core.JSONField{Name: "provider_domain_metadata", MaxSize: 2000})
	if err := app.Save(col); err != nil {
		t.Fatalf("save mail_domains collection: %v", err)
	}
	return app
}

func newOutboundTestRecord(t *testing.T, app core.App, domain string, providerDomainID int64) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatalf("find mail_domains: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("domain", domain)
	if providerDomainID != 0 {
		rec.Set("provider_domain_metadata", map[string]any{
			"postmark": map[string]any{"domain_id": providerDomainID},
		})
	}
	if err := app.Save(rec); err != nil {
		t.Fatalf("save record: %v", err)
	}
	return rec
}

// The record values must survive into the wire type — they are what the admin
// publishes in DNS, and the old code fetched and discarded them.
func TestCheckOutboundCarriesRecords(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(&stubRegistrar{rec: &maildomains.DomainRecords{
		Domain: "acme.com", ID: 12345, SPFVerified: true, DKIMVerified: true,
		DKIMHost: "sel._domainkey.acme.com", DKIMTextValue: "k=rsa;p=X",
		ReturnPathDomain: "pm-bounces.acme.com", ReturnPathCNAMEValue: "pm.mtasv.net",
	}})

	app := newOutboundTestApp(t)
	record := newOutboundTestRecord(t, app, "acme.com", 0)

	got := checkOutbound(context.Background(), record)

	if !got.SPF || !got.DKIM {
		t.Errorf("got = %+v, want SPF and DKIM true", got)
	}
	if got.DKIMHost != "sel._domainkey.acme.com" || got.DKIMTextValue != "k=rsa;p=X" {
		t.Errorf("DKIM record values missing: %+v", got)
	}
	if got.ReturnPathDomain != "pm-bounces.acme.com" || got.ReturnPathCNAMEValue != "pm.mtasv.net" {
		t.Errorf("return-path record values missing: %+v", got)
	}
	if got.Enrolled != "yes" {
		t.Errorf("Enrolled = %q, want %q", got.Enrolled, "yes")
	}
}

// A domain the provider has never heard of is reported as not enrolled, which
// is actionable, rather than as a generic error.
func TestCheckOutboundNotEnrolled(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(&stubRegistrar{err: maildomains.ErrDomainNotEnrolled})

	app := newOutboundTestApp(t)
	record := newOutboundTestRecord(t, app, "acme.com", 0)

	got := checkOutbound(context.Background(), record)

	if got.Enrolled != "no" {
		t.Fatalf("Enrolled = %q, want %q", got.Enrolled, "no")
	}
	if got.SPF || got.DKIM || got.ReturnPath {
		t.Error("an unenrolled domain must not report verified checks")
	}
}

// A transport or provider failure is UNKNOWN, not "no": the router being
// restarted must not make a working domain look unenrolled.
func TestCheckOutboundTransportFailureIsUnknown(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(&stubRegistrar{err: errors.New("socket closed")})

	app := newOutboundTestApp(t)
	record := newOutboundTestRecord(t, app, "acme.com", 0)

	got := checkOutbound(context.Background(), record)

	if got.Enrolled != "unknown" {
		t.Fatalf("Enrolled = %q, want %q", got.Enrolled, "unknown")
	}
	if got.Error == "" {
		t.Error("the failure reason must be carried")
	}
}

// AMENDMENT 1: checkOutbound must read the stored provider id off the record
// and pass it to the seam, so a by-name scan happens at most once per domain.
func TestCheckOutboundPassesStoredProviderID(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	registrar := &stubRegistrar{rec: &maildomains.DomainRecords{Domain: "acme.com", ID: 999}}
	maildomains.SetResolver(registrar)

	app := newOutboundTestApp(t)
	record := newOutboundTestRecord(t, app, "acme.com", 999)

	checkOutbound(context.Background(), record)

	if registrar.gotDomainID != 999 {
		t.Fatalf("gotDomainID = %d, want 999 (the id stored on the record)", registrar.gotDomainID)
	}
}

// AMENDMENT 1: when the stored id was 0 and the lookup succeeded (a by-name
// scan fallback), checkOutbound persists the id it learned so the scan runs
// at most once per domain thereafter.
func TestCheckOutboundPersistsLearnedProviderID(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(&stubRegistrar{rec: &maildomains.DomainRecords{Domain: "acme.com", ID: 4242}})

	app := newOutboundTestApp(t)
	record := newOutboundTestRecord(t, app, "acme.com", 0)

	checkOutbound(context.Background(), record)

	meta := readProviderDomainMetadata(record)
	if meta.Postmark == nil || meta.Postmark.DomainID != 4242 {
		t.Fatalf("provider_domain_metadata.postmark.domain_id not persisted: %+v", meta)
	}
}

// AMENDMENT 1: the provider's reported verification state and a checked_at
// stamp are written into provider_domain_metadata on every call — success or
// failure — so "asked and told no" is distinguishable from "never asked".
func TestCheckOutboundStampsMetadataOnFailure(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(&stubRegistrar{err: maildomains.ErrDomainNotEnrolled})

	app := newOutboundTestApp(t)
	record := newOutboundTestRecord(t, app, "acme.com", 0)

	checkOutbound(context.Background(), record)

	meta := readProviderDomainMetadata(record)
	if meta.Postmark == nil {
		t.Fatal("expected postmark metadata to be stamped even on failure")
	}
	if meta.Postmark.Enrolled == nil || *meta.Postmark.Enrolled != false {
		t.Errorf("Enrolled = %+v, want a pointer to false", meta.Postmark.Enrolled)
	}
	if meta.Postmark.CheckedAt == "" {
		t.Error("expected checked_at to be stamped even on failure")
	}
}

// --- verifyDomainRecord tests: the `verified` verdict itself ---

// newVerifyVerdictApp builds an app carrying the full field set
// verifyDomainRecord writes plus the system_settings rows that choose the
// provider. The SMTP provider is used deliberately: it verifies non-strictly
// (any non-empty server hostname satisfies the inbound check) and publishes a
// concrete MX host, so both inbound legs can be driven to a known value and
// the test can isolate the Outbound term of the verdict.
func newVerifyVerdictApp(t *testing.T) *tests.TestApp {
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

	saveSystemSetting(t, app, "mail.provider", "smtp")
	saveSystemSetting(t, app, "mail.smtp_inbound_mode", "smtp")
	saveSystemSetting(t, app, "mail.smtp_public_hostname", "mx.operator.example")

	return app
}

// runVerifyVerdict drives verifyDomainRecord end to end with both inbound legs
// forced to inboundOK and the outbound seam answering with registrar, and
// returns the persisted row.
func runVerifyVerdict(t *testing.T, inboundOK bool, registrar maildomains.Registrar) *core.Record {
	t.Helper()

	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(registrar)

	// The SMTP provider's expected MX host is its PublicHostname; answering
	// with a different host is how the inbound leg is driven to false.
	mxHost := "mx.operator.example."
	if !inboundOK {
		mxHost = "mx.somewhere-else.example."
	}
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{{Host: mxHost, Pref: 10}}, nil
	})

	app := newVerifyVerdictApp(t)
	if !inboundOK {
		// An inbound mode the provider does not serve makes
		// CheckInboundDomain report no inbound domain at all, so the Provider
		// leg fails alongside the mismatched MX above.
		saveSystemSetting(t, app, "mail.smtp_inbound_mode", "none")
	}
	record := newVerifyTestRecord(t, app)

	details, err := verifyDomainRecord(context.Background(), app, record)
	if err != nil {
		t.Fatalf("verifyDomainRecord: %v", err)
	}
	if details.MX.OK != inboundOK {
		t.Fatalf("precondition: MX.OK = %v, want %v (%+v)", details.MX.OK, inboundOK, details.MX)
	}
	if details.Provider.OK != inboundOK {
		t.Fatalf("precondition: Provider.OK = %v, want %v (%+v)", details.Provider.OK, inboundOK, details.Provider)
	}

	saved, err := app.FindRecordById("mail_domains", record.Id)
	if err != nil {
		t.Fatalf("reload record: %v", err)
	}
	return saved
}

func newVerifyTestRecord(t *testing.T, app core.App) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatalf("find mail_domains: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("domain", "acme.com")
	if err := app.Save(rec); err != nil {
		t.Fatalf("save record: %v", err)
	}
	return rec
}

// The happy path: inbound ready AND the provider has the domain enrolled.
func TestVerifyDomainRecord_EnrolledAndInboundReadyIsVerified(t *testing.T) {
	saved := runVerifyVerdict(t, true, &stubRegistrar{rec: &maildomains.DomainRecords{
		Domain: "acme.com", ID: 1, DKIMVerified: true, ReturnPathVerified: true,
	}})

	if !saved.GetBool("verified") {
		t.Fatalf("verified = false, want true for an enrolled domain with inbound ready")
	}
}

// THE regression this branch exists to fix: a domain the provider has never
// enrolled cannot send at all, so it must NOT show a green badge even when
// both inbound checks pass. Deleting the `Enrolled != "no"` term from the
// verdict must fail here.
func TestVerifyDomainRecord_NotEnrolledIsNotVerified(t *testing.T) {
	saved := runVerifyVerdict(t, true, &stubRegistrar{err: maildomains.ErrDomainNotEnrolled})

	if saved.GetBool("verified") {
		t.Fatalf("verified = true for a domain the provider has never enrolled; " +
			"inbound readiness alone must not produce a green badge")
	}
	// The inbound legs genuinely passed — the verdict must be driven by the
	// outbound term, not by an inbound check having been broken instead.
	if !saved.GetBool("mx_verified") || !saved.GetBool("inbound_domain_verified") {
		t.Fatalf("expected both inbound flags true; mx=%v inbound=%v",
			saved.GetBool("mx_verified"), saved.GetBool("inbound_domain_verified"))
	}
}

// "unknown" must NOT disqualify. A single provider blip during the hourly
// reverify would otherwise flip every working customer domain to unverified.
// Changing `!= "no"` to `== "yes"` must fail here.
func TestVerifyDomainRecord_UnknownEnrollmentStaysVerified(t *testing.T) {
	saved := runVerifyVerdict(t, true, &stubRegistrar{err: errors.New("socket closed")})

	if !saved.GetBool("verified") {
		t.Fatalf("verified = false after a transient provider failure; " +
			`an "unknown" enrollment state must not disqualify a working domain`)
	}
}

// Outbound enrollment alone is not sufficient: inbound readiness is still
// required, so the verdict is a conjunction rather than the outbound term
// having replaced it.
func TestVerifyDomainRecord_EnrolledButInboundNotReadyIsNotVerified(t *testing.T) {
	saved := runVerifyVerdict(t, false, &stubRegistrar{rec: &maildomains.DomainRecords{
		Domain: "acme.com", ID: 1, DKIMVerified: true, ReturnPathVerified: true,
	}})

	if saved.GetBool("verified") {
		t.Fatalf("verified = true with inbound not ready; enrollment alone is not enough")
	}
}
