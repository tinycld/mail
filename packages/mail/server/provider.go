package mail

import "context"

// Provider defines the pluggable email provider interface.
// Implement this interface to add support for a new provider (SES, SendGrid, etc.).
type Provider interface {
	Send(ctx context.Context, req *SendRequest) (*SendResult, error)
	ParseInbound(body []byte) (*InboundMessage, error)
	VerifyWebhookSignature(headers map[string]string, body []byte) error
	AddDomain(ctx context.Context, domain string) (*DomainVerification, error)
	CheckDomainVerification(ctx context.Context, domain string) (*DomainVerification, error)
}

type Recipient struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Attachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"` // base64 encoded
	ContentID   string `json:"content_id,omitempty"`
}

type SendRequest struct {
	From        string       `json:"from"`
	To          []Recipient  `json:"to"`
	Cc          []Recipient  `json:"cc,omitempty"`
	Bcc         []Recipient  `json:"bcc,omitempty"`
	Subject     string       `json:"subject"`
	HTMLBody    string       `json:"html_body"`
	TextBody    string       `json:"text_body"`
	ReplyTo     string       `json:"reply_to,omitempty"`
	InReplyTo   string       `json:"in_reply_to,omitempty"`   // RFC 5322 Message-ID
	References  string       `json:"references,omitempty"`    // RFC 5322 References
	Headers     []Header     `json:"headers,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SendResult struct {
	ProviderMessageID string `json:"provider_message_id"`
	MessageID         string `json:"message_id"` // RFC 5322 Message-ID assigned by the provider
}

type InboundMessage struct {
	From          Recipient    `json:"from"`
	To            []Recipient  `json:"to"`
	Cc            []Recipient  `json:"cc,omitempty"`
	Subject       string       `json:"subject"`
	HTMLBody      string       `json:"html_body"`
	TextBody      string       `json:"text_body"`
	StrippedReply string       `json:"stripped_reply"` // reply text with quoted content removed
	Date          string       `json:"date"`
	MessageID   string       `json:"message_id"`   // RFC 5322 Message-ID
	InReplyTo   string       `json:"in_reply_to"`   // RFC 5322 In-Reply-To
	References  string       `json:"references"`    // RFC 5322 References
	Headers     []Header     `json:"headers,omitempty"`
	Attachments []InboundAttachment `json:"attachments,omitempty"`
}

type InboundAttachment struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"` // base64 encoded
	ContentID   string `json:"content_id,omitempty"`
	Size        int64  `json:"size"`
}

type DomainVerification struct {
	Domain               string `json:"domain"`
	ID                   int64  `json:"id"`
	SPFVerified          bool   `json:"spf_verified"`
	DKIMVerified         bool   `json:"dkim_verified"`
	ReturnPathVerified   bool   `json:"return_path_verified"`
	DKIMHost             string `json:"dkim_host,omitempty"`
	DKIMTextValue        string `json:"dkim_text_value,omitempty"`
	ReturnPathDomain     string `json:"return_path_domain,omitempty"`
	ReturnPathCNAMEValue string `json:"return_path_cname_value,omitempty"`
}
