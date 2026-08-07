// Package api declares the HTTP payload contract of the mail package: one
// exported struct per request/response, plus the multipart form keys.
//
// This is the single source of truth for these shapes. The app generator
// parses this package with go/ast (it is never imported by the generator) and
// emits lib/generated/mail-api.ts for the web client; the CLI imports the
// structs directly. It is deliberately self-contained — no imports — so the
// generator needs no cross-package type resolution and a CLI import does not
// drag in the mail server's dependency chain. Constructs the generator cannot
// map faithfully (untagged exported fields, `any`, embedded fields, non-stdlib
// imports) fail generation loudly, so keep this package boring.
package api

// Multipart form keys accepted by POST /api/mail/send and /api/mail/draft
// alongside plain application/json bodies.
const (
	// MultipartFieldJSON carries the JSON-encoded request struct.
	MultipartFieldJSON = "json"
	// MultipartFieldAttachments carries the attachment files, one part each.
	MultipartFieldAttachments = "attachments"
)

// Recipient is one name/address pair. Field-compatible with the provider-side
// mailer.Recipient; the server converts at the provider boundary.
type Recipient struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// SendEmailRequest is the body of POST /api/mail/send.
type SendEmailRequest struct {
	MailboxID string      `json:"mailbox_id"`
	AliasID   string      `json:"alias_id,omitempty"`
	To        []Recipient `json:"to"`
	Cc        []Recipient `json:"cc,omitempty"`
	Bcc       []Recipient `json:"bcc,omitempty"`
	Subject   string      `json:"subject"`
	HTMLBody  string      `json:"html_body"`
	TextBody  string      `json:"text_body"`
	// InReplyToMessageID is the PocketBase record ID of the message being
	// replied to — not an RFC 822 Message-ID header value.
	InReplyToMessageID string `json:"in_reply_to_message_id,omitempty"`
}

// SaveDraftRequest is the body of POST /api/mail/draft.
type SaveDraftRequest struct {
	MailboxID string `json:"mailbox_id"`
	AliasID   string `json:"alias_id,omitempty"`
	// MessageID is the existing draft's PocketBase record ID; empty creates
	// a new draft.
	MessageID string      `json:"message_id,omitempty"`
	To        []Recipient `json:"to,omitempty"`
	Cc        []Recipient `json:"cc,omitempty"`
	Bcc       []Recipient `json:"bcc,omitempty"`
	Subject   string      `json:"subject,omitempty"`
	HTMLBody  string      `json:"html_body"`
	TextBody  string      `json:"text_body"`
}

// SendEmailResponse is returned by both POST /api/mail/send and
// POST /api/mail/draft. MessageID and ThreadID are PocketBase record IDs of
// the stored message and its thread — MessageID is not the RFC 822
// Message-ID header.
type SendEmailResponse struct {
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id"`
}

// SearchRequest names the query parameters of GET /api/mail/search. It is a
// query-string contract, not a JSON body: every value travels as a string and
// the tags name the parameter keys. Limit defaults to 25 (max 100); Query
// terms shorter than 2 characters are ignored.
type SearchRequest struct {
	Query         string `json:"q,omitempty"`
	MailboxID     string `json:"mailbox_id,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	Offset        int    `json:"offset,omitempty"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	Subject       string `json:"subject,omitempty"`
	HasWords      string `json:"has_words,omitempty"`
	Exclude       string `json:"not,omitempty"`
	DateAfter     string `json:"date_after,omitempty"`
	DateBefore    string `json:"date_before,omitempty"`
	Folder        string `json:"folder,omitempty"`
	HasAttachment bool   `json:"has_attachment,omitempty"`
}

// SearchResultItem is one thread-level hit in a SearchResponse. Highlight
// fields carry <mark> markup from FTS; Participants is the thread's stored
// JSON participant list, passed through as a string.
type SearchResultItem struct {
	ThreadID         string `json:"thread_id"`
	Subject          string `json:"subject"`
	SubjectHighlight string `json:"subject_highlight"`
	SnippetHighlight string `json:"snippet_highlight"`
	LatestDate       string `json:"latest_date"`
	Participants     string `json:"participants"`
	MessageCount     int    `json:"message_count"`
	MailboxID        string `json:"mailbox_id"`
	HasAttachments   bool   `json:"has_attachments"`
}

// SearchResponse is returned by GET /api/mail/search. Items is never null.
type SearchResponse struct {
	Items []SearchResultItem `json:"items"`
	Total int                `json:"total"`
}

// MXCheckResult reports whether the domain's MX records point at the
// provider's expected inbound host.
type MXCheckResult struct {
	OK       bool     `json:"ok"`
	Expected string   `json:"expected"`
	Actual   []string `json:"actual,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// ProviderCheckResult records the provider-side inbound configuration check.
type ProviderCheckResult struct {
	OK             bool   `json:"ok"`
	ExpectedDomain string `json:"expected_domain,omitempty"`
	ServerDomain   string `json:"server_domain,omitempty"`
	InboundAddress string `json:"inbound_address,omitempty"`
	Error          string `json:"error,omitempty"`
}

// OutboundCheckResult reports the DNS checks for outbound deliverability.
type OutboundCheckResult struct {
	SPF        bool   `json:"spf"`
	DKIM       bool   `json:"dkim"`
	ReturnPath bool   `json:"return_path"`
	Error      string `json:"error,omitempty"`
}

// VerificationDetails is the full per-check breakdown of a domain
// verification run, persisted on mail_domains.verification_details and
// returned by the verify endpoint.
type VerificationDetails struct {
	MX                 MXCheckResult       `json:"mx"`
	Provider           ProviderCheckResult `json:"provider"`
	Outbound           OutboundCheckResult `json:"outbound"`
	ProviderConfigured bool                `json:"provider_configured"`
	ProviderName       string              `json:"provider_name,omitempty"`
}

// VerifyDomainResponse is returned by POST /api/mail/domains/{id}/verify.
// The verification checks always ran when this arrives; Saved reports whether
// their outcome could be persisted. A client must treat Saved == false as a
// failure (the record still holds the previous state) — SaveError carries the
// reason.
type VerifyDomainResponse struct {
	ID                    string               `json:"id"`
	Verified              bool                 `json:"verified"`
	MXVerified            bool                 `json:"mx_verified"`
	InboundDomainVerified bool                 `json:"inbound_domain_verified"`
	SPFVerified           bool                 `json:"spf_verified"`
	DKIMVerified          bool                 `json:"dkim_verified"`
	ReturnPathVerified    bool                 `json:"return_path_verified"`
	LastCheckedAt         string               `json:"last_checked_at"`
	VerificationDetails   *VerificationDetails `json:"verification_details"`
	Saved                 bool                 `json:"saved"`
	SaveError             string               `json:"save_error,omitempty"`
}

// WebhookURLsResponse is returned by GET /api/mail/domains/{id}/webhook-urls:
// the provider-facing inbound and bounce webhook URLs for the domain.
type WebhookURLsResponse struct {
	Inbound string `json:"inbound"`
	Bounces string `json:"bounces"`
}

// WebhookAckResponse acknowledges a provider webhook delivery. Status is
// "ok" (inbound accepted), or for bounces "processed" / "ignored" (unknown
// message).
type WebhookAckResponse struct {
	Status string `json:"status"`
}
