package mail

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestSMTPProviderParseInbound_PlainText asserts the SMTP provider's
// ParseInbound returns an InboundMessage shape compatible with
// processInboundForMailbox for a minimal text/plain RFC 5322 message.
func TestSMTPProviderParseInbound_PlainText(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: Alice <alice@example.com>",
		"To: Bob <bob@example.org>",
		"Subject: Hello",
		"Date: Mon, 02 Jan 2006 15:04:05 +0000",
		"Message-ID: <plain-1@example.com>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"hello world",
	}, "\r\n"))

	p := NewSMTPProvider(SMTPConfig{})
	msg, err := p.ParseInbound(raw)
	if err != nil {
		t.Fatalf("ParseInbound: %v", err)
	}
	if msg.From.Email != "alice@example.com" || msg.From.Name != "Alice" {
		t.Errorf("from: got %+v, want Alice <alice@example.com>", msg.From)
	}
	if len(msg.To) != 1 || msg.To[0].Email != "bob@example.org" {
		t.Errorf("to: got %+v, want bob@example.org", msg.To)
	}
	if msg.Subject != "Hello" {
		t.Errorf("subject: got %q, want %q", msg.Subject, "Hello")
	}
	if !strings.Contains(msg.TextBody, "hello world") {
		t.Errorf("text body: got %q, want to contain %q", msg.TextBody, "hello world")
	}
	if msg.MessageID != "<plain-1@example.com>" {
		t.Errorf("message-id: got %q, want %q", msg.MessageID, "<plain-1@example.com>")
	}
}

// Threading headers (In-Reply-To, References) must round-trip so
// findOrCreateThread can stitch the message onto an existing thread.
func TestSMTPProviderParseInbound_ThreadingHeaders(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: a@example.com",
		"To: b@example.org",
		"Subject: Re: Hello",
		"Date: Mon, 02 Jan 2006 15:04:05 +0000",
		"Message-ID: <reply-1@example.com>",
		"In-Reply-To: <original-1@example.com>",
		"References: <original-1@example.com>",
		"Content-Type: text/plain",
		"",
		"reply body",
	}, "\r\n"))

	p := NewSMTPProvider(SMTPConfig{})
	msg, err := p.ParseInbound(raw)
	if err != nil {
		t.Fatalf("ParseInbound: %v", err)
	}
	if msg.InReplyTo != "<original-1@example.com>" {
		t.Errorf("in-reply-to: got %q", msg.InReplyTo)
	}
	if !strings.Contains(msg.References, "<original-1@example.com>") {
		t.Errorf("references: got %q", msg.References)
	}
}

// Bounce parsing must surface the no-webhook sentinel so the bounce endpoint
// can return a clear error instead of a generic parse failure.
func TestSMTPProviderParseBounce_ReturnsSentinel(t *testing.T) {
	p := NewSMTPProvider(SMTPConfig{})
	_, err := p.ParseBounce([]byte("{}"))
	if !errors.Is(err, errSMTPNoBounceWebhook) {
		t.Errorf("expected errSMTPNoBounceWebhook, got %v", err)
	}
}

// CheckInboundDomain reflects the org's inbound mode so domain_verify can
// reason about MX expectations correctly.
func TestSMTPProviderCheckInboundDomain_ReflectsMode(t *testing.T) {
	tests := []struct {
		name        string
		cfg         SMTPConfig
		wantDomain  string
		wantAddress string
	}{
		{
			name:        "smtp listener",
			cfg:         SMTPConfig{PublicHostname: "mx.example.com", InboundMode: "smtp"},
			wantDomain:  "mx.example.com",
			wantAddress: "mx@mx.example.com",
		},
		{
			name:        "imap fetcher",
			cfg:         SMTPConfig{InboundMode: "imap", IMAPHost: "imap.example.com", IMAPUsername: "user@example.com"},
			wantDomain:  "imap.example.com",
			wantAddress: "user@example.com",
		},
		{
			name:       "no inbound",
			cfg:        SMTPConfig{},
			wantDomain: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewSMTPProvider(tc.cfg)
			info, err := p.CheckInboundDomain(context.Background())
			if err != nil {
				t.Fatalf("CheckInboundDomain: %v", err)
			}
			if info.ServerInboundDomain != tc.wantDomain {
				t.Errorf("ServerInboundDomain: got %q, want %q", info.ServerInboundDomain, tc.wantDomain)
			}
			if info.InboundAddress != tc.wantAddress {
				t.Errorf("InboundAddress: got %q, want %q", info.InboundAddress, tc.wantAddress)
			}
		})
	}
}
