package mail

import (
	"context"
	"net"
	"strings"

	"tinycld.org/core/maildomains"
)

// SMTPRegistrar is the self-hosted counterpart of the Postmark registrar.
//
// A self-hosted deployment enrolls nothing: the operator publishes their own
// DNS and there is no provider account to register with. So AddDomain is a
// no-op returning the current state, and "enrolled" is always true — which is
// what keeps the `verified` verdict unchanged for SMTP deployments.
type SMTPRegistrar struct {
	cfg SMTPConfig
}

func NewSMTPRegistrar(cfg SMTPConfig) *SMTPRegistrar {
	return &SMTPRegistrar{cfg: cfg}
}

func (s *SMTPRegistrar) AddDomain(ctx context.Context, domain string) (*maildomains.DomainRecords, error) {
	return s.GetDomain(ctx, domain, 0)
}

// GetDomain runs the DNS checks directly — the logic moved verbatim from the
// former SMTPProvider.CheckDomainVerification, which needed no credentials.
// providerDomainID is ignored: SMTP has no provider account, so there is
// nothing an id could identify.
func (s *SMTPRegistrar) GetDomain(ctx context.Context, domain string, _ int64) (*maildomains.DomainRecords, error) {
	spf, dkimHost, dkimText, returnPathDomain, returnPathValue := checkSMTPDNS(ctx, domain, s.cfg.DKIMSelector)
	return &maildomains.DomainRecords{
		Domain:               domain,
		SPFVerified:          spf,
		DKIMVerified:         dkimText != "",
		ReturnPathVerified:   returnPathValue != "",
		DKIMHost:             dkimHost,
		DKIMTextValue:        dkimText,
		ReturnPathDomain:     returnPathDomain,
		ReturnPathCNAMEValue: returnPathValue,
	}, nil
}

// checkSMTPDNS runs pure-DNS checks against the domain — SPF (TXT at the
// apex), DKIM (TXT at <selector>._domainkey.<domain>), and DMARC (TXT at
// _dmarc.<domain>) as a proxy for Return-Path alignment. It threads ctx into
// all three lookups: this runs on an hourly ticker, and dropping the context
// would silently remove cancellation/timeout from three network calls.
//
// Returns the DKIM host name unconditionally (it is derived from the
// selector, not looked up) alongside the raw TXT values found, if any — the
// only DNS record values a self-hosted deployment has to show the admin.
func checkSMTPDNS(ctx context.Context, domain, selector string) (spfVerified bool, dkimHost, dkimTextValue, returnPathDomain, returnPathValue string) {
	dkimHost = selector + "._domainkey." + domain
	resolver := net.DefaultResolver

	if txts, err := resolver.LookupTXT(ctx, domain); err == nil {
		for _, txt := range txts {
			if strings.HasPrefix(strings.ToLower(txt), "v=spf1") {
				spfVerified = true
				break
			}
		}
	}

	if txts, err := resolver.LookupTXT(ctx, dkimHost); err == nil {
		for _, txt := range txts {
			if strings.Contains(strings.ToLower(txt), "v=dkim1") {
				dkimTextValue = txt
				break
			}
		}
	}

	if txts, err := resolver.LookupTXT(ctx, "_dmarc."+domain); err == nil {
		for _, txt := range txts {
			if strings.HasPrefix(strings.ToLower(txt), "v=dmarc1") {
				returnPathDomain = "_dmarc." + domain
				returnPathValue = txt
				break
			}
		}
	}

	return spfVerified, dkimHost, dkimTextValue, returnPathDomain, returnPathValue
}

// Compile-time assertion that SMTPRegistrar satisfies maildomains.Registrar.
var _ maildomains.Registrar = (*SMTPRegistrar)(nil)
