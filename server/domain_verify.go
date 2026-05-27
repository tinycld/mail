package mail

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// postmarkInboundMXHost is the target MX host Postmark requires for inbound
// domain forwarding. See:
// https://postmarkapp.com/developer/user-guide/inbound/inbound-domain-forwarding
const postmarkInboundMXHost = "inbound.postmarkapp.com"

// expectedInboundMXHost returns the MX host the operator should publish in
// DNS so this provider can deliver inbound mail. For Postmark this is the
// fixed "inbound.postmarkapp.com"; for the self-hosted SMTP provider it is
// the operator's PublicHostname (the host running the inbound SMTP listener);
// for everything else (NoopProvider, unconfigured) it returns "" — which
// suppresses the MX check (always failing OK=false with an informative hint
// in the UI).
func expectedInboundMXHost(provider Provider) string {
	switch p := provider.(type) {
	case *PostmarkProvider:
		return postmarkInboundMXHost
	case *SMTPProvider:
		// For SMTP IMAP-fetch mode there is no MX target on this side; the
		// operator's existing MTA receives mail and we just poll it.
		if p.cfg.InboundMode == "imap" {
			return ""
		}
		return p.cfg.PublicHostname
	default:
		return ""
	}
}

// mxLookup is swappable for tests.
var mxLookup = net.DefaultResolver.LookupMX

// verifyLocks serializes verify runs per mail_domains record so the hourly
// ticker and a user-triggered Verify don't write to the same row concurrently.
var verifyLocks sync.Map // recordID -> *sync.Mutex

// errProviderNotConfigured is a sentinel the endpoint can detect to return
// a targeted 400 instead of persisting misleading per-check failures when
// the org has no mail provider credentials.
var errProviderNotConfigured = errors.New("mail provider not configured")

type mxCheckResult struct {
	OK       bool     `json:"ok"`
	Expected string   `json:"expected"`
	Actual   []string `json:"actual,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// providerCheckResult records the provider-side inbound configuration check.
// For Postmark this verifies the server's InboundDomain matches the domain;
// for SMTP it verifies the operator's PublicHostname matches the domain (so
// the MX record will resolve correctly back to us).
type providerCheckResult struct {
	OK             bool   `json:"ok"`
	ExpectedDomain string `json:"expected_domain,omitempty"`
	ServerDomain   string `json:"server_domain,omitempty"`
	InboundAddress string `json:"inbound_address,omitempty"`
	Error          string `json:"error,omitempty"`
}

type outboundCheckResult struct {
	SPF        bool   `json:"spf"`
	DKIM       bool   `json:"dkim"`
	ReturnPath bool   `json:"return_path"`
	Error      string `json:"error,omitempty"`
}

type verificationDetails struct {
	MX                 mxCheckResult       `json:"mx"`
	Provider           providerCheckResult `json:"provider"`
	Outbound           outboundCheckResult `json:"outbound"`
	ProviderConfigured bool                `json:"provider_configured"`
	ProviderName       string              `json:"provider_name,omitempty"`
}

// checkMX resolves MX records for the domain and matches them against the
// provider-specific expected inbound host. Returns OK=true if any MX record
// points to the expected host. An empty expectedHost means the provider has
// no MX requirement on our side (e.g. SMTP provider in IMAP-fetch mode) —
// in that case we report OK=true with an explanatory hint and do not fetch
// MX records (avoids a confusing "missing MX" error when none is required).
func checkMX(ctx context.Context, domain, expectedHost string) mxCheckResult {
	result := mxCheckResult{Expected: expectedHost}
	if expectedHost == "" {
		result.OK = true
		return result
	}
	records, err := mxLookup(ctx, domain)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for _, r := range records {
		host := strings.TrimSuffix(strings.ToLower(r.Host), ".")
		result.Actual = append(result.Actual, fmt.Sprintf("%s (pref %d)", host, r.Pref))
		if host == expectedHost {
			result.OK = true
		}
	}
	if !result.OK && len(result.Actual) == 0 {
		result.Error = "no MX records found"
	}
	return result
}

// providerRequiresExactInboundMatch reports whether the provider's
// InboundDomain must textually equal the verifying domain to count as
// configured. Postmark requires this (one server per inbound domain); SMTP
// does not (one operator host serves any number of tenant domains, so as
// long as the operator's PublicHostname is set we accept it and lean on the
// MX check to prove inbound mail actually arrives).
func providerRequiresExactInboundMatch(provider Provider) bool {
	_, isPostmark := provider.(*PostmarkProvider)
	return isPostmark
}

// checkProviderInbound asks the provider for its inbound-domain configuration
// and checks whether it satisfies the verifying domain. The strictness is
// provider-dependent: see providerRequiresExactInboundMatch. For NoopProvider /
// unconfigured providers we return an explicit "not configured" hint.
func checkProviderInbound(ctx context.Context, provider Provider, domain string) providerCheckResult {
	return checkProviderInboundStrict(ctx, provider, domain, providerRequiresExactInboundMatch(provider))
}

// checkProviderInboundStrict is the testable form — strict=true requires
// exact match (Postmark semantics), strict=false accepts any non-empty
// ServerInboundDomain (SMTP semantics).
func checkProviderInboundStrict(ctx context.Context, provider Provider, domain string, strict bool) providerCheckResult {
	result := providerCheckResult{ExpectedDomain: domain}
	info, err := provider.CheckInboundDomain(ctx)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.ServerDomain = info.ServerInboundDomain
	result.InboundAddress = info.InboundAddress
	if info.ServerInboundDomain == "" {
		result.Error = "provider has no inbound domain configured"
		return result
	}
	if strings.EqualFold(info.ServerInboundDomain, domain) {
		result.OK = true
		return result
	}
	if !strict {
		result.OK = true
	}
	return result
}

// checkOutbound queries the provider for SPF/DKIM/Return-Path verification.
// Failure is best-effort — missing outbound doesn't block the inbound verdict.
func checkOutbound(ctx context.Context, provider Provider, domain string) outboundCheckResult {
	result := outboundCheckResult{}
	v, err := provider.CheckDomainVerification(ctx, domain)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.SPF = v.SPFVerified
	result.DKIM = v.DKIMVerified
	result.ReturnPath = v.ReturnPathVerified
	return result
}

// describeProvider returns a human-readable provider name and whether the
// provider has the credentials/config it needs to do meaningful work. Used to
// surface provider-aware status in the domain-verification details payload.
func describeProvider(provider Provider) (string, bool) {
	switch provider.(type) {
	case *PostmarkProvider:
		return "postmark", provider.Configured()
	case *SMTPProvider:
		return "smtp", true
	default:
		return "none", false
	}
}

// recordLock returns a mutex that is unique per record ID. Callers must call
// the returned unlock func when done.
func recordLock(recordID string) func() {
	m, _ := verifyLocks.LoadOrStore(recordID, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// verifyDomainRecord runs all checks for a single mail_domains record, persists
// per-check status and the overall `verified` flag, and returns the details.
// `verified` is derived from inbound-readiness only (MX + Postmark InboundDomain
// match). Outbound checks are advisory. Concurrent calls on the same record
// serialize via recordLock so writes don't interleave.
func verifyDomainRecord(ctx context.Context, app *pocketbase.PocketBase, record *core.Record) (*verificationDetails, error) {
	unlock := recordLock(record.Id)
	defer unlock()

	orgID := record.GetString("org")
	domain := record.GetString("domain")

	provider := providerForOrg(app, orgID)
	providerName, providerConfigured := describeProvider(provider)

	details := &verificationDetails{
		ProviderConfigured: providerConfigured,
		ProviderName:       providerName,
	}
	details.MX = checkMX(ctx, domain, expectedInboundMXHost(provider))
	details.Provider = checkProviderInbound(ctx, provider, domain)
	details.Outbound = checkOutbound(ctx, provider, domain)

	record.Set("mx_verified", details.MX.OK)
	record.Set("inbound_domain_verified", details.Provider.OK)
	record.Set("spf_verified", details.Outbound.SPF)
	record.Set("dkim_verified", details.Outbound.DKIM)
	record.Set("return_path_verified", details.Outbound.ReturnPath)
	record.Set("verification_details", details)
	record.Set("last_checked_at", time.Now().UTC().Format(time.RFC3339Nano))
	record.Set("verified", details.MX.OK && details.Provider.OK)

	if err := app.Save(record); err != nil {
		return details, err
	}
	return details, nil
}
