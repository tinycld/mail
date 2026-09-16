package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/maildomains"
	"tinycld.org/packages/mail/api"
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

// checkMX resolves MX records for the domain and matches them against the
// provider-specific expected inbound host. Returns OK=true if any MX record
// points to the expected host. An empty expectedHost means the provider has
// no MX requirement on our side (e.g. SMTP provider in IMAP-fetch mode) —
// in that case we report OK=true with an explanatory hint and do not fetch
// MX records (avoids a confusing "missing MX" error when none is required).
func checkMX(ctx context.Context, domain, expectedHost string) api.MXCheckResult {
	result := api.MXCheckResult{Expected: expectedHost}
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
func checkProviderInbound(ctx context.Context, provider Provider, domain string) api.ProviderCheckResult {
	return checkProviderInboundStrict(ctx, provider, domain, providerRequiresExactInboundMatch(provider))
}

// checkProviderInboundStrict is the testable form — strict=true requires
// exact match (Postmark semantics), strict=false accepts any non-empty
// ServerInboundDomain (SMTP semantics).
func checkProviderInboundStrict(ctx context.Context, provider Provider, domain string, strict bool) api.ProviderCheckResult {
	result := api.ProviderCheckResult{ExpectedDomain: domain}
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

// postmarkDomainMetadata is the per-provider slice of provider_domain_metadata
// this package writes and reads. Field names and shape match the TypeScript
// ProviderDomainMetadata type in tinycld/mail/types.ts — keep them in sync.
type postmarkDomainMetadata struct {
	DomainID           int64  `json:"domain_id,omitempty"`
	CheckedAt          string `json:"checked_at,omitempty"`
	Enrolled           *bool  `json:"enrolled,omitempty"`
	SPFVerified        *bool  `json:"spf_verified,omitempty"`
	DKIMVerified       *bool  `json:"dkim_verified,omitempty"`
	ReturnPathVerified *bool  `json:"return_path_verified,omitempty"`
}

// providerDomainMetadata is the provider_domain_metadata JSON field. Only
// Postmark stamps it today (SMTP has no provider account and thus no id to
// remember); the outer struct leaves room for a second provider without a
// further migration.
type providerDomainMetadata struct {
	Postmark *postmarkDomainMetadata `json:"postmark,omitempty"`
}

// readProviderDomainMetadata decodes the record's provider_domain_metadata
// field. A json field arrives as types.JSONRaw or an already-decoded any
// depending on how the record was loaded, so this round-trips through
// encoding/json rather than asserting a concrete type. A missing or
// unparsable value yields the zero struct — never an error — because this
// field is reporting-only and must not block a verification run.
func readProviderDomainMetadata(record *core.Record) providerDomainMetadata {
	var meta providerDomainMetadata
	raw := record.Get("provider_domain_metadata")
	if raw == nil {
		return meta
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return meta
	}
	_ = json.Unmarshal(data, &meta)
	return meta
}

// boolPtr is a small helper so call sites can take the address of a literal.
func boolPtr(b bool) *bool { return &b }

// checkOutbound reads the domain's provider-side state through the
// maildomains seam. It does NOT take a Provider: these are account-credential
// operations, and on a hosted deployment they are performed by the router,
// not here.
//
// Failure stays best-effort — outbound never blocks the inbound verdict — but
// is now three-valued. A provider that has never heard of the domain is the
// admin's problem ("no"); a socket or API failure is not ("unknown"), and
// conflating them would tell an admin to fix a domain that is fine.
//
// It also maintains provider_domain_metadata on the record (in memory only —
// the caller's app.Save persists it alongside the row's own flags, so the two
// cannot drift): the stored provider id is read and forwarded so a by-name
// scan happens at most once per domain, a newly learned id is persisted, and
// the provider's answer is stamped with checked_at on every call, success or
// failure, so "asked and told no" stays distinguishable from "never asked".
func checkOutbound(ctx context.Context, record *core.Record) api.OutboundCheckResult {
	domain := record.GetString("domain")
	meta := readProviderDomainMetadata(record)
	var priorID int64
	if meta.Postmark != nil {
		priorID = meta.Postmark.DomainID
	}

	result := api.OutboundCheckResult{}
	rec, err := maildomains.Current().GetDomain(ctx, domain, priorID)

	stamp := &postmarkDomainMetadata{DomainID: priorID, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	switch {
	case errors.Is(err, maildomains.ErrDomainNotEnrolled):
		result.Enrolled = "no"
		result.Error = err.Error()
		stamp.Enrolled = boolPtr(false)
		record.Set("provider_domain_metadata", providerDomainMetadata{Postmark: stamp})
		return result
	case err != nil:
		result.Enrolled = "unknown"
		result.Error = err.Error()
		stamp.Enrolled = boolPtr(false)
		record.Set("provider_domain_metadata", providerDomainMetadata{Postmark: stamp})
		return result
	}

	result.Enrolled = "yes"
	result.SPF = rec.SPFVerified
	result.DKIM = rec.DKIMVerified
	result.ReturnPath = rec.ReturnPathVerified
	result.DKIMHost = rec.DKIMHost
	result.DKIMTextValue = rec.DKIMTextValue
	result.ReturnPathDomain = rec.ReturnPathDomain
	result.ReturnPathCNAMEValue = rec.ReturnPathCNAMEValue

	// A zero prior id that just resolved via the by-name fallback: persist the
	// id the seam learned so that fallback doesn't run again next time.
	if priorID == 0 && rec.ID != 0 {
		stamp.DomainID = rec.ID
	}
	stamp.Enrolled = boolPtr(true)
	stamp.SPFVerified = boolPtr(rec.SPFVerified)
	stamp.DKIMVerified = boolPtr(rec.DKIMVerified)
	stamp.ReturnPathVerified = boolPtr(rec.ReturnPathVerified)
	record.Set("provider_domain_metadata", providerDomainMetadata{Postmark: stamp})

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
func verifyDomainRecord(ctx context.Context, app core.App, record *core.Record) (*api.VerificationDetails, error) {
	unlock := recordLock(record.Id)
	defer unlock()

	domain := record.GetString("domain")

	provider := newProviderFromSystem(app)
	providerName, providerConfigured := describeProvider(provider)

	details := &api.VerificationDetails{
		ProviderConfigured: providerConfigured,
		ProviderName:       providerName,
	}
	details.MX = checkMX(ctx, domain, expectedInboundMXHost(provider))
	details.Provider = checkProviderInbound(ctx, provider, domain)
	details.Outbound = checkOutbound(ctx, record)

	record.Set("mx_verified", details.MX.OK)
	record.Set("inbound_domain_verified", details.Provider.OK)
	record.Set("spf_verified", details.Outbound.SPF)
	record.Set("dkim_verified", details.Outbound.DKIM)
	record.Set("return_path_verified", details.Outbound.ReturnPath)
	record.Set("verification_details", details)
	record.Set("last_checked_at", time.Now().UTC().Format(time.RFC3339Nano))
	// A domain the provider has never enrolled cannot send at all, so it must
	// not show as verified. Previously `verified` was inbound-only, and an
	// unenrolled domain displayed a green badge while every send failed.
	// "unknown" is treated as not-disqualifying: a transport failure must not
	// flip a working domain to unverified.
	record.Set("verified", details.MX.OK && details.Provider.OK && details.Outbound.Enrolled != "no")

	if err := app.Save(record); err != nil {
		return details, err
	}
	return details, nil
}
