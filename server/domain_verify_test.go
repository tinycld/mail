package mail

import (
	"context"
	"errors"
	"net"
	"testing"
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
	inboundDomain    string
	inboundAddress   string
	inboundErr       error
	outboundSPF      bool
	outboundDKIM     bool
	outboundRP       bool
	outboundErr      error
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
func (f *fakeProvider) AddDomain(context.Context, string) (*DomainVerification, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeProvider) CheckDomainVerification(_ context.Context, _ string) (*DomainVerification, error) {
	if f.outboundErr != nil {
		return nil, f.outboundErr
	}
	return &DomainVerification{
		SPFVerified:        f.outboundSPF,
		DKIMVerified:       f.outboundDKIM,
		ReturnPathVerified: f.outboundRP,
	}, nil
}
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

func TestCheckOutbound_AllTrue(t *testing.T) {
	p := &fakeProvider{outboundSPF: true, outboundDKIM: true, outboundRP: true}
	got := checkOutbound(context.Background(), p, "example.com")
	if !(got.SPF && got.DKIM && got.ReturnPath) {
		t.Fatalf("expected all outbound true; got %+v", got)
	}
}

func TestCheckOutbound_ProviderError(t *testing.T) {
	p := &fakeProvider{outboundErr: errors.New("domain not found")}
	got := checkOutbound(context.Background(), p, "example.com")
	if got.SPF || got.DKIM || got.ReturnPath {
		t.Fatalf("expected all outbound false on error; got %+v", got)
	}
	if got.Error != "domain not found" {
		t.Fatalf("expected error passthrough; got %q", got.Error)
	}
}
