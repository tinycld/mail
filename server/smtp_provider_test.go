package mail

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestParseInbound_PlainText asserts the SMTP provider's ParseInbound returns
// an InboundMessage shape compatible with processInboundForMailbox for a
// minimal text/plain RFC 5322 message.
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

func TestEnvelopeAddress(t *testing.T) {
	cases := map[string]string{
		"alice@example.com":          "alice@example.com",
		"Alice <alice@example.com>":  "alice@example.com",
		"  bob@example.org  ":        "bob@example.org",
		"\"Carol Q\" <carol@x.com>":  "carol@x.com",
	}
	for in, want := range cases {
		got, err := envelopeAddress(in)
		if err != nil {
			t.Errorf("envelopeAddress(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("envelopeAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsPermanentSMTPError(t *testing.T) {
	cases := map[string]bool{
		"":                              false,
		"foo":                           false,
		"550 5.1.1 user unknown":        true,
		"5":                             false,
		"552 mailbox full":              true,
		"450 try again later":           false,
		"421 service not available":     false,
		"5XX bogus":                     false,
		"4xx not numeric":               false,
	}
	for in, want := range cases {
		if got := isPermanentSMTPError(in); got != want {
			t.Errorf("isPermanentSMTPError(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestGroupRecipientsByDomain(t *testing.T) {
	req := &SendRequest{
		To: []Recipient{{Email: "a@example.com"}, {Email: "b@example.com"}},
		Cc: []Recipient{{Email: "c@example.org"}},
		Bcc: []Recipient{{Email: "d@example.org"}, {Email: "malformed"}},
	}
	got := groupRecipientsByDomain(req)
	if len(got["example.com"]) != 2 {
		t.Errorf("example.com: got %v", got["example.com"])
	}
	if len(got["example.org"]) != 2 {
		t.Errorf("example.org: got %v", got["example.org"])
	}
	if _, ok := got[""]; ok {
		t.Errorf("malformed address should not produce empty-key group")
	}
}

// TestSMTPProviderSend_DirectMX_Success drives a fake in-process SMTP
// receiver and asserts the full conversation (MAIL FROM / RCPT TO / DATA) is
// issued and the right body is delivered. This is the integration-level test
// for the outbound code path.
func TestSMTPProviderSend_DirectMX_Success(t *testing.T) {
	receiver := newFakeSMTPReceiver(t, smtpReceiverConfig{})
	defer receiver.close()

	swapMXLookup(t, map[string][]*net.MX{
		"example.org": {{Host: "127.0.0.1", Pref: 10}},
	})
	swapSMTPDial(t, receiver.dialOverride)

	p := NewSMTPProvider(SMTPConfig{PublicHostname: "mx.tinycld.test"})
	result, err := p.Send(context.Background(), &SendRequest{
		From:     "alice@tinycld.test",
		To:       []Recipient{{Email: "bob@example.org"}},
		Subject:  "Hello",
		TextBody: "hi from tinycld",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(result.FailedRecipients) != 0 {
		t.Errorf("expected 0 failed recipients, got %+v", result.FailedRecipients)
	}
	if result.MessageID == "" {
		t.Errorf("expected non-empty MessageID")
	}

	received := receiver.firstMessage(t)
	if received.from != "alice@tinycld.test" {
		t.Errorf("MAIL FROM: got %q, want alice@tinycld.test", received.from)
	}
	if len(received.rcpt) != 1 || received.rcpt[0] != "bob@example.org" {
		t.Errorf("RCPT TO: got %v", received.rcpt)
	}
	if !strings.Contains(received.body, "hi from tinycld") {
		t.Errorf("body missing payload; got: %s", received.body)
	}
	if !strings.Contains(received.body, "Subject: Hello") {
		t.Errorf("body missing Subject header; got: %s", received.body)
	}
}

// A 5xx response at RCPT TO must surface as a RecipientFailure rather than a
// transport error. This is the contract that lets endpoints_send mark the
// stored message as bounced with the right reason.
func TestSMTPProviderSend_PermanentFailureSurfacesAsRecipientFailure(t *testing.T) {
	receiver := newFakeSMTPReceiver(t, smtpReceiverConfig{
		rejectRcpt: map[string]string{
			"unknown@example.org": "550 5.1.1 No such user",
		},
	})
	defer receiver.close()

	swapMXLookup(t, map[string][]*net.MX{
		"example.org": {{Host: "127.0.0.1", Pref: 10}},
	})
	swapSMTPDial(t, receiver.dialOverride)

	p := NewSMTPProvider(SMTPConfig{PublicHostname: "mx.tinycld.test"})
	result, err := p.Send(context.Background(), &SendRequest{
		From:     "alice@tinycld.test",
		To:       []Recipient{{Email: "unknown@example.org"}},
		Subject:  "hi",
		TextBody: "test",
	})
	if err != nil {
		t.Fatalf("Send returned error (should be nil with FailedRecipients): %v", err)
	}
	if len(result.FailedRecipients) != 1 {
		t.Fatalf("expected 1 failed recipient, got %+v", result.FailedRecipients)
	}
	if result.FailedRecipients[0].Email != "unknown@example.org" {
		t.Errorf("failed recipient email: got %q", result.FailedRecipients[0].Email)
	}
	if !strings.HasPrefix(result.FailedRecipients[0].Reason, "550") {
		t.Errorf("failed reason: got %q, want to start with 550", result.FailedRecipients[0].Reason)
	}
}

// When MX lookup yields nothing AND the bare-domain fallback also fails to
// dial, Send must return a transport error so the calling endpoint surfaces
// a 502 to the user instead of silently dropping the message.
func TestSMTPProviderSend_AllMXDown_TransportError(t *testing.T) {
	swapMXLookup(t, map[string][]*net.MX{})
	swapSMTPDial(t, func(_ context.Context, _, _ string) (net.Conn, error) {
		return nil, errors.New("dial error: connection refused")
	})

	p := NewSMTPProvider(SMTPConfig{PublicHostname: "mx.tinycld.test", OutboundTimeout: 2 * time.Second})
	_, err := p.Send(context.Background(), &SendRequest{
		From:     "alice@tinycld.test",
		To:       []Recipient{{Email: "bob@example.org"}},
		Subject:  "hi",
		TextBody: "test",
	})
	if err == nil {
		t.Fatalf("expected transport error, got nil")
	}
}

// --- Test helpers ---

func swapMXLookup(t *testing.T, m map[string][]*net.MX) {
	t.Helper()
	orig := smtpMXLookup
	smtpMXLookup = func(_ context.Context, name string) ([]*net.MX, error) {
		if mxs, ok := m[name]; ok {
			return mxs, nil
		}
		return nil, nil
	}
	t.Cleanup(func() { smtpMXLookup = orig })
}

func swapSMTPDial(t *testing.T, fn func(ctx context.Context, network, addr string) (net.Conn, error)) {
	t.Helper()
	orig := smtpDial
	smtpDial = fn
	t.Cleanup(func() { smtpDial = orig })
}

// fakeSMTPReceiver is an in-process SMTP server good enough to test the
// outbound conversation. It uses net/textproto so the test stays independent
// of the production go-smtp library's behavior.
type fakeSMTPReceiver struct {
	listener net.Listener
	cfg      smtpReceiverConfig
	mu       sync.Mutex
	received []fakeSMTPMessage
	done     chan struct{}
}

type smtpReceiverConfig struct {
	rejectRcpt map[string]string // RCPT addr → SMTP response (e.g. "550 5.1.1 No such user")
}

type fakeSMTPMessage struct {
	from string
	rcpt []string
	body string
}

func newFakeSMTPReceiver(t *testing.T, cfg smtpReceiverConfig) *fakeSMTPReceiver {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	r := &fakeSMTPReceiver{listener: ln, cfg: cfg, done: make(chan struct{})}
	go r.serve()
	return r
}

func (r *fakeSMTPReceiver) close() {
	r.listener.Close()
}

// dialOverride lets the SMTPProvider connect to this in-process server even
// though the MX records claim a different host. The provider always dials
// "<host>:25"; we ignore the address and dial our actual listener.
func (r *fakeSMTPReceiver) dialOverride(_ context.Context, _, _ string) (net.Conn, error) {
	return net.Dial("tcp", r.listener.Addr().String())
}

func (r *fakeSMTPReceiver) firstMessage(t *testing.T) fakeSMTPMessage {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		r.mu.Lock()
		if len(r.received) > 0 {
			msg := r.received[0]
			r.mu.Unlock()
			return msg
		}
		r.mu.Unlock()
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for SMTP message")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (r *fakeSMTPReceiver) serve() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			return
		}
		go r.handle(conn)
	}
}

func (r *fakeSMTPReceiver) handle(conn net.Conn) {
	defer conn.Close()
	tc := textproto.NewConn(conn)
	defer tc.Close()

	_ = tc.PrintfLine("220 fake.smtp ready")

	msg := fakeSMTPMessage{}
	var inData bool
	var bodyBuf bytes.Buffer

	for {
		line, err := tc.ReadLine()
		if err != nil {
			return
		}
		if inData {
			if line == "." {
				inData = false
				msg.body = bodyBuf.String()
				bodyBuf.Reset()
				_ = tc.PrintfLine("250 OK queued")
				r.mu.Lock()
				r.received = append(r.received, msg)
				msg = fakeSMTPMessage{}
				r.mu.Unlock()
				continue
			}
			bodyBuf.WriteString(line)
			bodyBuf.WriteString("\r\n")
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			_ = tc.PrintfLine("250-fake.smtp")
			_ = tc.PrintfLine("250 8BITMIME")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			msg.from = extractAddress(line[len("MAIL FROM:"):])
			_ = tc.PrintfLine("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			addr := extractAddress(line[len("RCPT TO:"):])
			if resp, blocked := r.cfg.rejectRcpt[addr]; blocked {
				_ = tc.PrintfLine("%s", resp)
				continue
			}
			msg.rcpt = append(msg.rcpt, addr)
			_ = tc.PrintfLine("250 OK")
		case upper == "DATA":
			inData = true
			_ = tc.PrintfLine("354 end with <CR><LF>.<CR><LF>")
		case upper == "QUIT":
			_ = tc.PrintfLine("221 bye")
			return
		case upper == "RSET":
			msg = fakeSMTPMessage{}
			_ = tc.PrintfLine("250 OK")
		case upper == "NOOP":
			_ = tc.PrintfLine("250 OK")
		default:
			_ = tc.PrintfLine("502 5.5.2 command not implemented")
		}
	}
}

// extractAddress strips angle brackets, surrounding whitespace, and any
// trailing ESMTP parameters (e.g. " BODY=8BITMIME") from an SMTP address
// argument like " <alice@example.com> BODY=8BITMIME". Used by the fake server.
func extractAddress(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "<"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j > 0 {
			return s[i+1 : i+j]
		}
	}
	if sp := strings.IndexAny(s, " \t"); sp >= 0 {
		s = s[:sp]
	}
	return s
}

