package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrz1836/postmark"
)

type PostmarkProvider struct {
	client *postmark.Client
}

func NewPostmarkProvider(serverToken, accountToken string) *PostmarkProvider {
	return &PostmarkProvider{
		client: postmark.NewClient(serverToken, accountToken),
	}
}

func (p *PostmarkProvider) Send(ctx context.Context, req *SendRequest) (*SendResult, error) {
	email := postmark.Email{
		From:     req.From,
		To:       formatRecipients(req.To),
		Cc:       formatRecipients(req.Cc),
		Bcc:      formatRecipients(req.Bcc),
		Subject:  req.Subject,
		HTMLBody: req.HTMLBody,
		TextBody: req.TextBody,
		ReplyTo:  req.ReplyTo,
	}

	// Set threading headers
	var headers []postmark.Header
	if req.InReplyTo != "" {
		headers = append(headers, postmark.Header{Name: "In-Reply-To", Value: req.InReplyTo})
	}
	if req.References != "" {
		headers = append(headers, postmark.Header{Name: "References", Value: req.References})
	}
	for _, h := range req.Headers {
		headers = append(headers, postmark.Header{Name: h.Name, Value: h.Value})
	}
	if len(headers) > 0 {
		email.Headers = headers
	}

	for _, att := range req.Attachments {
		email.Attachments = append(email.Attachments, postmark.Attachment{
			Name:        att.Name,
			ContentType: att.ContentType,
			Content:     att.Content,
			ContentID:   att.ContentID,
		})
	}

	resp, err := p.client.SendEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("postmark send failed: %w", err)
	}
	if resp.ErrorCode != 0 {
		return nil, fmt.Errorf("postmark error %d: %s", resp.ErrorCode, resp.Message)
	}

	return &SendResult{
		ProviderMessageID: resp.MessageID,
		MessageID:         resp.MessageID,
	}, nil
}

// postmarkInboundPayload matches Postmark's inbound webhook JSON structure.
type postmarkInboundPayload struct {
	From          string              `json:"From"`
	FromName      string              `json:"FromName"`
	FromFull      postmarkRecipient   `json:"FromFull"`
	To            string              `json:"To"`
	ToFull        []postmarkRecipient `json:"ToFull"`
	CcFull        []postmarkRecipient `json:"CcFull"`
	Subject       string              `json:"Subject"`
	Date          string              `json:"Date"`
	TextBody          string              `json:"TextBody"`
	HTMLBody          string              `json:"HtmlBody"`
	StrippedTextReply string              `json:"StrippedTextReply"`
	MessageID         string              `json:"MessageID"`
	MailboxHash   string              `json:"MailboxHash"`
	Headers       []postmarkHeader    `json:"Headers"`
	Attachments   []postmarkAttachment `json:"Attachments"`
}

type postmarkRecipient struct {
	Name  string `json:"Name"`
	Email string `json:"Email"`
}

type postmarkHeader struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

type postmarkAttachment struct {
	Name          string `json:"Name"`
	Content       string `json:"Content"`
	ContentType   string `json:"ContentType"`
	ContentID     string `json:"ContentID"`
	ContentLength int64  `json:"ContentLength"`
}

func (p *PostmarkProvider) ParseInbound(body []byte) (*InboundMessage, error) {
	var payload postmarkInboundPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse inbound payload: %w", err)
	}

	msg := &InboundMessage{
		From: Recipient{
			Name:  payload.FromFull.Name,
			Email: payload.FromFull.Email,
		},
		To:       convertRecipients(payload.ToFull),
		Cc:       convertRecipients(payload.CcFull),
		Subject:       payload.Subject,
		HTMLBody:       payload.HTMLBody,
		TextBody:       payload.TextBody,
		StrippedReply:  payload.StrippedTextReply,
		Date:           payload.Date,
	}

	// Extract threading headers from the raw headers
	for _, h := range payload.Headers {
		switch h.Name {
		case "Message-ID", "Message-Id":
			msg.MessageID = h.Value
		case "In-Reply-To":
			msg.InReplyTo = h.Value
		case "References":
			msg.References = h.Value
		}
		msg.Headers = append(msg.Headers, Header{Name: h.Name, Value: h.Value})
	}

	// Fall back to top-level MessageID if not found in headers
	if msg.MessageID == "" && payload.MessageID != "" {
		msg.MessageID = "<" + payload.MessageID + ">"
	}

	for _, att := range payload.Attachments {
		msg.Attachments = append(msg.Attachments, InboundAttachment{
			Name:        att.Name,
			ContentType: att.ContentType,
			Content:     att.Content, // already base64-encoded by Postmark
			ContentID:   att.ContentID,
			Size:        att.ContentLength,
		})
	}

	return msg, nil
}

// VerifyWebhookSignature is a no-op for Postmark — security is via the secret URL token.
func (p *PostmarkProvider) VerifyWebhookSignature(_ map[string]string, _ []byte) error {
	return nil
}

func (p *PostmarkProvider) AddDomain(ctx context.Context, domain string) (*DomainVerification, error) {
	details, err := p.client.CreateDomain(ctx, postmark.DomainCreateRequest{
		Name: domain,
	})
	if err != nil {
		return nil, fmt.Errorf("postmark create domain failed: %w", err)
	}
	return domainDetailsToVerification(details), nil
}

func (p *PostmarkProvider) CheckDomainVerification(ctx context.Context, domain string) (*DomainVerification, error) {
	// Look up the domain by listing and matching by name
	domains, err := p.client.GetDomains(ctx, 100, 0)
	if err != nil {
		return nil, fmt.Errorf("postmark list domains failed: %w", err)
	}

	for _, d := range domains.Domains {
		if d.Name == domain {
			details, err := p.client.GetDomain(ctx, d.ID)
			if err != nil {
				return nil, fmt.Errorf("postmark get domain failed: %w", err)
			}
			return domainDetailsToVerification(details), nil
		}
	}

	return nil, fmt.Errorf("domain %q not found in Postmark", domain)
}

func domainDetailsToVerification(d postmark.DomainDetails) *DomainVerification {
	return &DomainVerification{
		Domain:               d.Name,
		ID:                   d.ID,
		SPFVerified:          d.SPFVerified,
		DKIMVerified:         d.DKIMVerified,
		ReturnPathVerified:   d.ReturnPathDomainVerified,
		DKIMHost:             d.DKIMHost,
		DKIMTextValue:        d.DKIMTextValue,
		ReturnPathDomain:     d.ReturnPathDomain,
		ReturnPathCNAMEValue: d.ReturnPathDomainCNAMEValue,
	}
}

func formatRecipients(recipients []Recipient) string {
	parts := make([]string, 0, len(recipients))
	for _, r := range recipients {
		if r.Name != "" {
			parts = append(parts, fmt.Sprintf("%q <%s>", r.Name, r.Email))
		} else {
			parts = append(parts, r.Email)
		}
	}
	return strings.Join(parts, ", ")
}

func convertRecipients(prs []postmarkRecipient) []Recipient {
	out := make([]Recipient, len(prs))
	for i, pr := range prs {
		out[i] = Recipient{Name: pr.Name, Email: pr.Email}
	}
	return out
}
