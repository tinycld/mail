package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"sort"
	"strings"
	"time"
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

// SMTPProvider implements Provider by talking SMTP directly: outbound via
// recipient-domain MX lookup, inbound via the built-in inbound listener or
// IMAP fetcher. There is no provider account — credentials are not used for
// outbound; per-recipient delivery either works or it doesn't.
type SMTPProvider struct {
	cfg SMTPConfig
}

// NewSMTPProvider builds an SMTPProvider with defaults applied. Callers
// (providerForOrg) populate cfg from org settings + env vars.
func NewSMTPProvider(cfg SMTPConfig) *SMTPProvider {
	cfg.applyDefaults()
	return &SMTPProvider{cfg: cfg}
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

// smtpMXLookup is swappable for tests. Mirrors the pattern in domain_verify.go.
var smtpMXLookup = net.DefaultResolver.LookupMX

// smtpDial opens a connection to host:port. Swappable for tests.
var smtpDial = func(ctx context.Context, network, addr string) (net.Conn, error) {
	d := net.Dialer{}
	return d.DialContext(ctx, network, addr)
}

// Send performs a single outbound delivery attempt per recipient-domain group.
// Recipients are grouped by their domain; for each group we resolve MX, walk
// MX hosts in priority order, and try opportunistic STARTTLS. A permanent
// (5xx) response for a recipient bubbles up as a RecipientFailure on the
// returned SendResult. A successful send to at least one recipient returns nil
// error; if every recipient permanently failed we still return nil error (the
// FailedRecipients slice carries the bad news so the caller can persist the
// message as 'bounced'). A transport-level failure (e.g. all MX hosts
// unreachable) returns an error and no SendResult.
func (p *SMTPProvider) Send(ctx context.Context, req *SendRequest) (*SendResult, error) {
	if req == nil {
		return nil, fmt.Errorf("nil send request")
	}

	messageID := generateMessageID(p.cfg.PublicHostname)
	body, err := buildOutgoingRFC5322(req, messageID, p.cfg.PublicHostname)
	if err != nil {
		return nil, fmt.Errorf("failed to build message: %w", err)
	}

	envelopeFrom, err := envelopeAddress(req.From)
	if err != nil {
		return nil, fmt.Errorf("invalid From address: %w", err)
	}

	groups := groupRecipientsByDomain(req)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no recipients")
	}

	ctx, cancel := context.WithTimeout(ctx, p.cfg.OutboundTimeout)
	defer cancel()

	result := &SendResult{MessageID: messageID, ProviderMessageID: messageID}
	deliveredAny := false
	var lastTransportErr error

	for domain, recipients := range groups {
		delivered, failures, transportErr := p.deliverToDomain(ctx, domain, envelopeFrom, recipients, body)
		if transportErr != nil {
			lastTransportErr = transportErr
			// Transport error against this domain → mark every recipient in
			// the group as failed so the caller can persist bounce_reason.
			for _, rcpt := range recipients {
				result.FailedRecipients = append(result.FailedRecipients, RecipientFailure{
					Email:  rcpt,
					Reason: transportErr.Error(),
				})
			}
			continue
		}
		result.FailedRecipients = append(result.FailedRecipients, failures...)
		if delivered {
			deliveredAny = true
		}
	}

	if !deliveredAny && lastTransportErr != nil && len(result.FailedRecipients) == len(allRecipients(req)) {
		// Every recipient failed transport — return error so the caller
		// surfaces 502 to the user instead of silently storing the message.
		return nil, fmt.Errorf("smtp delivery failed for all recipients: %w", lastTransportErr)
	}

	return result, nil
}

// deliverToDomain opens an SMTP connection to one MX host for `domain` and
// issues MAIL FROM / RCPT TO (one per recipient) / DATA. Returns whether at
// least one recipient was accepted, the list of recipients that got a
// permanent (5xx) failure, and any transport error (failed to reach any MX).
func (p *SMTPProvider) deliverToDomain(ctx context.Context, domain, from string, recipients []string, body []byte) (bool, []RecipientFailure, error) {
	hosts, err := resolveMXHosts(ctx, domain)
	if err != nil {
		return false, nil, fmt.Errorf("mx lookup for %s: %w", domain, err)
	}

	var lastErr error
	for _, host := range hosts {
		conn, err := smtpDial(ctx, "tcp", host+":25")
		if err != nil {
			lastErr = fmt.Errorf("dial %s: %w", host, err)
			continue
		}

		delivered, failures, convErr := runSMTPConversation(conn, host, p.cfg.PublicHostname, from, recipients, body)
		// runSMTPConversation closes its own connection on the happy path;
		// on transport error we still close defensively.
		_ = conn.Close()
		if convErr != nil {
			lastErr = fmt.Errorf("smtp to %s: %w", host, convErr)
			continue
		}
		return delivered, failures, nil
	}

	return false, nil, fmt.Errorf("all MX hosts unreachable for %s: %w", domain, lastErr)
}

// runSMTPConversation drives the SMTP conversation against an already-dialed
// connection. STARTTLS is attempted when advertised; AUTH is never offered
// (this is server-to-server traffic, not client submission).
func runSMTPConversation(conn net.Conn, host, helo, from string, recipients []string, body []byte) (bool, []RecipientFailure, error) {
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return false, nil, fmt.Errorf("new client: %w", err)
	}
	defer client.Close()

	if err := client.Hello(helo); err != nil {
		return false, nil, fmt.Errorf("EHLO: %w", err)
	}

	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			// Fall through without TLS — opportunistic. Most receivers accept.
			// We deliberately do not fail here: a failing STARTTLS is far
			// rarer than a misconfigured TLS cert on the receiver, and most
			// receivers accept plaintext fallback for incoming mail.
		}
	}

	if err := client.Mail(from); err != nil {
		return false, nil, fmt.Errorf("MAIL FROM: %w", err)
	}

	var failures []RecipientFailure
	acceptedCount := 0
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			// Distinguish permanent (5xx) from temporary (4xx) failures by
			// the leading digit of the SMTP code carried in the error. The
			// net/smtp client wraps the response as "<code> <text>" in
			// err.Error(), so a simple prefix check is reliable enough.
			reason := err.Error()
			if isPermanentSMTPError(reason) {
				failures = append(failures, RecipientFailure{Email: rcpt, Reason: reason})
				continue
			}
			// Temporary failure → treat as transport error so the caller
			// retries the whole batch on the next MX (or surfaces 502).
			return false, nil, fmt.Errorf("RCPT TO %s: %w", rcpt, err)
		}
		acceptedCount++
	}

	if acceptedCount == 0 {
		// All recipients permanently failed at RCPT — close cleanly without
		// sending DATA. The failures slice carries the per-recipient bounces.
		_ = client.Reset()
		_ = client.Quit()
		return false, failures, nil
	}

	w, err := client.Data()
	if err != nil {
		return false, failures, fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return false, failures, fmt.Errorf("write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return false, failures, fmt.Errorf("close DATA: %w", err)
	}

	_ = client.Quit()
	return true, failures, nil
}

// isPermanentSMTPError checks whether an error string from net/smtp begins
// with a 5xx response code. Net/smtp formats responses as "NNN <text>" or
// "NNN-<text>" for multiline; both forms are covered by checking the first
// digit. Returns false for empty strings and non-numeric prefixes so transient
// network errors don't get mistaken for permanent bounces.
func isPermanentSMTPError(s string) bool {
	if len(s) < 3 {
		return false
	}
	return s[0] == '5' && isDigit(s[1]) && isDigit(s[2])
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// resolveMXHosts returns hosts sorted by preference. If MX lookup yields no
// records (NXDOMAIN or empty), RFC 5321 §5.1 requires falling back to the
// domain's A/AAAA record — we honor that by returning the domain itself.
func resolveMXHosts(ctx context.Context, domain string) ([]string, error) {
	mxs, err := smtpMXLookup(ctx, domain)
	if err == nil && len(mxs) > 0 {
		sort.SliceStable(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
		hosts := make([]string, len(mxs))
		for i, mx := range mxs {
			hosts[i] = strings.TrimSuffix(mx.Host, ".")
		}
		return hosts, nil
	}
	// Fallback to the bare domain — receivers without explicit MX still get mail.
	return []string{domain}, nil
}

// groupRecipientsByDomain partitions To+Cc+Bcc by recipient domain.
func groupRecipientsByDomain(req *SendRequest) map[string][]string {
	groups := make(map[string][]string)
	add := func(addr string) {
		_, domain := splitAddress(addr)
		if domain == "" {
			return
		}
		groups[domain] = append(groups[domain], addr)
	}
	for _, r := range req.To {
		add(r.Email)
	}
	for _, r := range req.Cc {
		add(r.Email)
	}
	for _, r := range req.Bcc {
		add(r.Email)
	}
	return groups
}

// allRecipients flattens To+Cc+Bcc into a single slice of email addresses.
func allRecipients(req *SendRequest) []string {
	out := make([]string, 0, len(req.To)+len(req.Cc)+len(req.Bcc))
	for _, r := range req.To {
		out = append(out, r.Email)
	}
	for _, r := range req.Cc {
		out = append(out, r.Email)
	}
	for _, r := range req.Bcc {
		out = append(out, r.Email)
	}
	return out
}

// envelopeAddress extracts the bare email from a possibly-display-name-wrapped
// From string ("Alice <a@example.com>" → "a@example.com"). The wire-level
// MAIL FROM must be the bare address.
func envelopeAddress(from string) (string, error) {
	if from == "" {
		return "", fmt.Errorf("empty from")
	}
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.Index(from[i:], ">"); j > 0 {
			return strings.TrimSpace(from[i+1 : i+j]), nil
		}
	}
	return strings.TrimSpace(from), nil
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

// generateMessageID builds an RFC-compliant Message-ID rooted at the operator's
// public hostname. Time + 8 random hex chars keep collision probability negligible.
func generateMessageID(hostname string) string {
	if hostname == "" {
		hostname = "localhost"
	}
	suffix, _ := randomHex(8)
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UTC().UnixNano(), suffix, hostname)
}

// Compile-time assertion that SMTPProvider satisfies Provider.
var _ Provider = (*SMTPProvider)(nil)
