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

// NewSMTPRegistrar builds the registrar with defaults applied, exactly as
// NewSMTPProvider does. Skipping applyDefaults here left DKIMSelector empty on
// a deployment that never set one, so the DKIM lookup targeted the malformed
// "._domainkey.<domain>" — a name that can never resolve, leaving DKIM
// permanently red and showing that malformed host as something to publish,
// while the settings field's own placeholder promises "tinycld".
func NewSMTPRegistrar(cfg SMTPConfig) *SMTPRegistrar {
	cfg.applyDefaults()
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
	spf, dkimHost, dkimText, dmarcPublished := checkSMTPDNS(ctx, domain, s.cfg.DKIMSelector)
	return &maildomains.DomainRecords{
		Domain:       domain,
		SPFVerified:  spf,
		DKIMVerified: dkimText != "",
		// ReturnPathVerified does NOT mean a return-path CNAME resolves — a
		// self-hosted deployment bounces to its own host and has no such
		// record to publish. It reports whether a DMARC policy is published,
		// which is the closest signal this path has that the domain's
		// envelope-sender alignment was deliberately configured. The name is
		// the seam's (it is Postmark's meaning there); the meaning here is
		// narrower, so read it as advisory only.
		ReturnPathVerified: dmarcPublished,
		DKIMHost:           dkimHost,
		DKIMTextValue:      dkimText,
		// ReturnPathDomain/ReturnPathCNAMEValue mean "publish this CNAME, with
		// this target". There is no such record on the SMTP path, so they stay
		// empty. Filling them from the DMARC TXT lookup told an admin to
		// create a CNAME at _dmarc.<domain> whose target was a DMARC policy
		// string — invalid DNS that, if published over the existing TXT,
		// destroys the domain's DMARC policy.
	}, nil
}

// txtLookup is swappable for tests, matching mxLookup in domain_verify.go.
// These checks are pure DNS, so a test that cannot stub them can only assert
// against whatever the network answers that minute.
var txtLookup = net.DefaultResolver.LookupTXT

// checkSMTPDNS runs pure-DNS checks against the domain — SPF (TXT at the
// apex), DKIM (TXT at <selector>._domainkey.<domain>), and DMARC (TXT at
// _dmarc.<domain>). It threads ctx into all three lookups: this runs on an
// hourly ticker, and dropping the context would silently remove
// cancellation/timeout from three network calls.
//
// Returns the DKIM host name unconditionally (it is derived from the
// selector, not looked up) alongside the DKIM TXT value found, if any — the
// only publishable record value a self-hosted deployment has to show the
// admin. The DMARC result is a BOOLEAN, not a value: its TXT content is a
// policy the operator already published, never something to publish, and
// returning it as a value is what let it be rendered as a CNAME target.
func checkSMTPDNS(ctx context.Context, domain, selector string) (spfVerified bool, dkimHost, dkimTextValue string, dmarcPublished bool) {
	dkimHost = selector + "._domainkey." + domain

	if txts, err := txtLookup(ctx, domain); err == nil {
		for _, txt := range txts {
			if strings.HasPrefix(strings.ToLower(txt), "v=spf1") {
				spfVerified = true
				break
			}
		}
	}

	if txts, err := txtLookup(ctx, dkimHost); err == nil {
		for _, txt := range txts {
			if strings.Contains(strings.ToLower(txt), "v=dkim1") {
				dkimTextValue = txt
				break
			}
		}
	}

	if txts, err := txtLookup(ctx, "_dmarc."+domain); err == nil {
		for _, txt := range txts {
			if strings.HasPrefix(strings.ToLower(txt), "v=dmarc1") {
				dmarcPublished = true
				break
			}
		}
	}

	return spfVerified, dkimHost, dkimTextValue, dmarcPublished
}

// Compile-time assertion that SMTPRegistrar satisfies maildomains.Registrar.
var _ maildomains.Registrar = (*SMTPRegistrar)(nil)
