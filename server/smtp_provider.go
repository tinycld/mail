package mail

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"tinycld.org/core/mailer"
)

// errSMTPNoBounceWebhook is returned by SMTPProvider.ParseBounce because the
// SMTP provider surfaces synchronous SMTP failures inline in Send (returned as
// SendResult.FailedRecipients) rather than via a webhook. The bounce endpoint
// detects this sentinel and returns a clear 404 instead of a parse error.
var errSMTPNoBounceWebhook = errors.New("smtp provider does not use a bounce webhook — synchronous failures are reported by Send")

// SMTPConfig is the per-org configuration for the self-hosted SMTP provider.
// Defaults are applied by NewSMTPProvider for any zero-valued field.
type SMTPConfig struct {
	// PublicHostname is what TinyCld announces in EHLO and what the org must
	// publish as their MX target when inbound mode is "smtp". Defaults to the
	// SMTP_PUBLIC_HOSTNAME env var, then to "localhost".
	PublicHostname string

	// OutboundTimeout caps the total time per recipient-domain delivery
	// attempt (MX lookup + SMTP conversation). Defaults to 30s.
	OutboundTimeout time.Duration

	// InboundMode chooses how mail arrives: "smtp" runs the built-in inbound
	// MX listener; "imap" runs the polling fetcher; "" disables inbound.
	InboundMode string

	// IMAP fetcher configuration (only consulted when InboundMode == "imap").
	IMAPHost         string
	IMAPPort         int
	IMAPUsername     string
	IMAPPassword     string
	IMAPUseTLS       bool
	IMAPMailbox      string
	IMAPPollInterval time.Duration

	// DKIMSelector is the DNS selector to expect at <selector>._domainkey.
	// Defaults to "tinycld". Verification reports whether a TXT record exists
	// there; signing is out of scope for v1.
	DKIMSelector string
}

// applyDefaults populates zero-valued fields from package defaults.
func (c *SMTPConfig) applyDefaults() {
	if c.PublicHostname == "" {
		c.PublicHostname = "localhost"
	}
	if c.OutboundTimeout == 0 {
		c.OutboundTimeout = 30 * time.Second
	}
	if c.IMAPMailbox == "" {
		c.IMAPMailbox = "INBOX"
	}
	if c.IMAPPollInterval == 0 {
		c.IMAPPollInterval = 60 * time.Second
	}
	if c.DKIMSelector == "" {
		c.DKIMSelector = "tinycld"
	}
}

// SMTPProvider implements Provider by talking SMTP directly: outbound is
// delegated to the shared core mailer's SMTPSender (recipient-domain MX
// lookup); inbound arrives via the built-in inbound listener or IMAP fetcher.
// There is no provider account — credentials are not used for outbound;
// per-recipient delivery either works or it doesn't.
type SMTPProvider struct {
	cfg    SMTPConfig
	sender *mailer.SMTPSender
}

// NewSMTPProvider builds an SMTPProvider with defaults applied. Callers
// (providerForOrg) populate cfg from system settings. Outbound sending is
// handled by a core mailer.SMTPSender built from the outbound config subset.
func NewSMTPProvider(cfg SMTPConfig) *SMTPProvider {
	cfg.applyDefaults()
	return &SMTPProvider{
		cfg: cfg,
		sender: mailer.NewSMTPSender(mailer.SMTPConfig{
			PublicHostname:  cfg.PublicHostname,
			OutboundTimeout: cfg.OutboundTimeout,
		}),
	}
}

// Configured reports true: the SMTP provider needs no credentials to attempt
// a send (direct MX). Misconfiguration (e.g. operator's host can't make
// outbound TCP 25) surfaces as a Send error, not as "not configured" — so
// endpoints don't reject up-front.
func (p *SMTPProvider) Configured() bool {
	return true
}

// Config returns the active configuration. Used by the inbound SMTP listener
// and IMAP fetcher to read settings without re-resolving the provider.
func (p *SMTPProvider) Config() SMTPConfig {
	return p.cfg
}

// Send delegates to the core mailer's direct-MX sender. See
// mailer.SMTPSender.SendFull for the delivery + bounce-reporting contract.
func (p *SMTPProvider) Send(ctx context.Context, req *SendRequest) (*SendResult, error) {
	return p.sender.SendFull(ctx, req)
}

// ParseInbound parses a raw RFC 5322 message into the InboundMessage shape
// expected by processInboundForMailbox. We reuse parseRFC5322 (also used by
// IMAP APPEND) and adapt its storedMessage output to InboundMessage so the
// downstream pipeline stays unchanged.
//
// One important normalization: parseRFC5322 uses go-message's MsgID() helper
// which strips the angle brackets. Postmark (and the rest of our threading
// code) stores message identifiers with brackets — re-wrap so the SMTP path
// matches the Postmark path bit-for-bit.
func (p *SMTPProvider) ParseInbound(body []byte) (*InboundMessage, error) {
	stored, err := parseRFC5322(body)
	if err != nil {
		return nil, fmt.Errorf("smtp inbound: %w", err)
	}

	msg := &InboundMessage{
		From:        Recipient{Name: stored.SenderName, Email: stored.SenderEmail},
		To:          stored.To,
		Cc:          stored.Cc,
		Subject:     stored.Subject,
		HTMLBody:    stored.HTMLBody,
		TextBody:    stored.TextBody,
		Date:        stored.Date,
		MessageID:   wrapMsgID(stored.MessageID),
		InReplyTo:   wrapMsgID(stored.InReplyTo),
		References:  wrapMsgIDList(stored.References),
		Attachments: stored.Attachments,
	}
	return msg, nil
}

// wrapMsgID adds angle brackets to a Message-ID if missing. Empty strings
// pass through unchanged so callers can keep using "" as the absent marker.
func wrapMsgID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, "<") && strings.HasSuffix(id, ">") {
		return id
	}
	return "<" + id + ">"
}

// wrapMsgIDList wraps each space-separated Message-ID in a References header.
func wrapMsgIDList(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Fields(s)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, wrapMsgID(p))
	}
	return strings.Join(out, " ")
}

// ParseBounce is not used for SMTP — bounces are reported inline by Send via
// SendResult.FailedRecipients. The bounce endpoint detects the sentinel and
// returns a clear 404.
func (p *SMTPProvider) ParseBounce(_ []byte) (*BounceEvent, error) {
	return nil, errSMTPNoBounceWebhook
}

// VerifyWebhookSignature is a no-op: SMTP doesn't use webhooks. The bounce
// endpoint never reaches signature verification because ParseBounce errors
// out first; the inbound endpoint (still useful for testing) also has no
// signed payload to verify.
func (p *SMTPProvider) VerifyWebhookSignature(_ map[string]string, _ []byte) error {
	return nil
}

// AddDomain is a no-op for SMTP. The operator publishes their own DNS records;
// there's no provider-side enrollment. We return a zero-valued DomainVerification
// so callers persist consistent state and surface the operator-action via the
// per-check booleans set by CheckDomainVerification.
func (p *SMTPProvider) AddDomain(_ context.Context, domain string) (*DomainVerification, error) {
	return &DomainVerification{Domain: domain}, nil
}

// CheckDomainVerification runs pure-DNS checks against the domain — SPF (TXT
// at the apex), DKIM (TXT at <selector>._domainkey.<domain>), and DMARC (TXT
// at _dmarc.<domain>) as a proxy for Return-Path alignment. Failure of any
// individual check sets its respective flag to false; an error from DNS
// resolution surfaces in the result fields' textual hints (left empty here —
// the calling layer logs failures separately).
func (p *SMTPProvider) CheckDomainVerification(ctx context.Context, domain string) (*DomainVerification, error) {
	v := &DomainVerification{
		Domain:   domain,
		DKIMHost: p.cfg.DKIMSelector + "._domainkey." + domain,
	}

	resolver := net.DefaultResolver

	if txts, err := resolver.LookupTXT(ctx, domain); err == nil {
		for _, txt := range txts {
			if strings.HasPrefix(strings.ToLower(txt), "v=spf1") {
				v.SPFVerified = true
				break
			}
		}
	}

	if txts, err := resolver.LookupTXT(ctx, v.DKIMHost); err == nil {
		for _, txt := range txts {
			if strings.Contains(strings.ToLower(txt), "v=dkim1") {
				v.DKIMVerified = true
				v.DKIMTextValue = txt
				break
			}
		}
	}

	if txts, err := resolver.LookupTXT(ctx, "_dmarc."+domain); err == nil {
		for _, txt := range txts {
			if strings.HasPrefix(strings.ToLower(txt), "v=dmarc1") {
				v.ReturnPathVerified = true
				v.ReturnPathDomain = "_dmarc." + domain
				v.ReturnPathCNAMEValue = txt
				break
			}
		}
	}

	return v, nil
}

// CheckInboundDomain returns a synthetic InboundVerification reflecting the
// org's configured inbound mode. The domain-verification layer compares
// ServerInboundDomain against the verifying domain to set inbound_domain_verified.
// For SMTP-mode inbound, we return the operator's PublicHostname — DNS-level
// verification (the MX target matches) is handled separately by checkMX.
func (p *SMTPProvider) CheckInboundDomain(_ context.Context) (*InboundVerification, error) {
	switch p.cfg.InboundMode {
	case "smtp":
		return &InboundVerification{
			ServerInboundDomain: p.cfg.PublicHostname,
			InboundAddress:      "mx@" + p.cfg.PublicHostname,
		}, nil
	case "imap":
		return &InboundVerification{
			ServerInboundDomain: p.cfg.IMAPHost,
			InboundAddress:      p.cfg.IMAPUsername,
		}, nil
	default:
		return &InboundVerification{}, nil
	}
}

// Compile-time assertion that SMTPProvider satisfies Provider.
var _ Provider = (*SMTPProvider)(nil)
