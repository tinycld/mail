package mail

import (
	"context"

	"tinycld.org/core/mailer"
)

// Re-export shared types so existing mail code doesn't need to change import paths.
type Recipient = mailer.Recipient
type Attachment = mailer.Attachment
type Header = mailer.Header
type SendRequest = mailer.SendRequest
type SendResult = mailer.SendResult
type RecipientFailure = mailer.RecipientFailure

// Provider defines the pluggable email provider interface.
// Send is delegated to the shared mailer package. The remaining methods are
// mail-package-specific (inbound parsing, bounces, inbound domain config).
//
// Enrolling a sending domain and reading back its verification state are NOT
// here: those are account-credential operations, and on a hosted deployment
// this process never holds the account token. They go through
// tinycld.org/core/maildomains instead — see checkOutbound in
// domain_verify.go.
type Provider interface {
	// Configured reports whether the provider has the credentials it needs to
	// reach the provider API (e.g. a Postmark server token). Inbound webhook
	// parsing works without credentials, so an unconfigured provider is still
	// useful for ParseInbound/ParseBounce — but Send and the domain/inbound
	// checks will fail. Callers on those paths should reject early when this
	// returns false rather than surfacing an opaque API error.
	Configured() bool

	// Send puts a message on the wire. Every caller MUST call
	// checkSendAllowed (send_gate.go) immediately beforehand — that gate is
	// what bounds an account's outbound fan-out and volume, and it is
	// enforced only by its call sites. Two exist today, in endpoints_send.go
	// and smtp_session.go; a third that skips the gate sends unmetered mail
	// under this deployment's domain reputation, and no test of the normal
	// path would notice.
	Send(ctx context.Context, req *SendRequest) (*SendResult, error)
	ParseInbound(body []byte) (*InboundMessage, error)
	ParseBounce(body []byte) (*BounceEvent, error)
	VerifyWebhookSignature(headers map[string]string, body []byte) error
	CheckInboundDomain(ctx context.Context) (*InboundVerification, error)
}

// InboundVerification describes the server-side InboundDomain setting reported
// by the mail provider (e.g. Postmark). Used to confirm the provider end of
// inbound mail forwarding is wired up for the org's domain.
type InboundVerification struct {
	ServerInboundDomain string `json:"server_inbound_domain"`
	InboundAddress      string `json:"inbound_address,omitempty"`
}

// BounceEvent represents a parsed bounce or spam complaint notification.
type BounceEvent struct {
	// ID is the provider's own identifier for this notification. Providers
	// retry, and an operator can replay from a dashboard, so anything that
	// COUNTS bounces must deduplicate on it — overwriting a message's status
	// twice is harmless, adding to a tally twice is not.
	ID string `json:"id"`

	RecordType string `json:"record_type"`

	// BounceType is the provider's display name for the kind of failure
	// ("HardBounce", "SoftBounce", "SpamNotification", …). TypeCode is the
	// stable numeric form of the same thing; prefer it and fall back to this,
	// because a display string is free to change in a way a code is not.
	BounceType string `json:"bounce_type"`
	TypeCode   int    `json:"type_code"`

	MessageID string `json:"message_id"`

	// From is the address the failed message was sent AS. The recipient is in
	// Email; this is the sender, and it is what lets something outside this
	// deployment attribute a bounce back to whoever sent it.
	From string `json:"from"`

	Email string `json:"email"`

	// Inactive reports that the provider has suppressed this address as a
	// result of the failure. A stronger signal than the bounce alone: the
	// provider has decided not to try again.
	Inactive bool `json:"inactive"`

	Description string `json:"description"`
	BouncedAt   string `json:"bounced_at"`
}

type InboundMessage struct {
	From          Recipient           `json:"from"`
	To            []Recipient         `json:"to"`
	Cc            []Recipient         `json:"cc,omitempty"`
	Subject       string              `json:"subject"`
	HTMLBody      string              `json:"html_body"`
	TextBody      string              `json:"text_body"`
	StrippedReply string              `json:"stripped_reply"`
	Date          string              `json:"date"`
	MessageID     string              `json:"message_id"`
	InReplyTo     string              `json:"in_reply_to"`
	References    string              `json:"references"`
	Headers       []Header            `json:"headers,omitempty"`
	Attachments   []InboundAttachment `json:"attachments,omitempty"`
}

type InboundAttachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
	ContentID   string `json:"content_id,omitempty"`
	Size        int64  `json:"size"`
}
