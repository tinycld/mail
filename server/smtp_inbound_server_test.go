package mail

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/emersion/go-smtp"
)

// TestSMTPInboundSession_AcceptsKnownRecipient drives the inbound SMTP
// session against a seeded test app and asserts the message is stored via the
// shared processInboundForMailbox pipeline (same path the Postmark webhook
// uses). We bypass net.Listen — the session's logic is the load-bearing bit.
func TestSMTPInboundSession_AcceptsKnownRecipient(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_smtp_in_001")
	seedMember(t, app, "mb_smtp_in_001", "userorg_alice")

	sess := &smtpInboundSession{app: app, hostname: "mx.tinycld.test"}

	if err := sess.Mail("sender@external.example", &smtp.MailOptions{}); err != nil {
		t.Fatalf("Mail: %v", err)
	}
	if err := sess.Rcpt("alice@acme.com", &smtp.RcptOptions{}); err != nil {
		t.Fatalf("Rcpt: %v", err)
	}

	raw := buildPlainRFC5322(t, "sender@external.example", "alice@acme.com", "Hello SMTP in", "first-smtp-inbound", "body content")
	if err := sess.Data(bytes.NewReader(raw)); err != nil {
		t.Fatalf("Data: %v", err)
	}

	msgs, _ := app.FindRecordsByFilter("mail_messages", "subject = {:s}", "", 10, 0, map[string]any{"s": "Hello SMTP in"})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 stored message, got %d", len(msgs))
	}
	stored := msgs[0]
	if !strings.Contains(stored.GetString("sender_email"), "sender@external.example") {
		t.Errorf("sender_email: got %q", stored.GetString("sender_email"))
	}
}

// Unknown recipients must be rejected with 550 5.1.1 so the sending MTA
// generates a clean bounce instead of silent discard. This is the bit that
// keeps us from acting as an open relay or a black hole.
func TestSMTPInboundSession_RejectsUnknownRecipient(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_smtp_in_002")
	seedMember(t, app, "mb_smtp_in_002", "userorg_alice")

	sess := &smtpInboundSession{app: app, hostname: "mx.tinycld.test"}
	if err := sess.Mail("sender@external.example", &smtp.MailOptions{}); err != nil {
		t.Fatalf("Mail: %v", err)
	}
	err := sess.Rcpt("ghost@acme.com", &smtp.RcptOptions{})
	if err == nil {
		t.Fatalf("expected unknown recipient to be rejected")
	}
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) {
		t.Fatalf("expected *smtp.SMTPError, got %T: %v", err, err)
	}
	if smtpErr.Code != 550 {
		t.Errorf("expected 550, got %d", smtpErr.Code)
	}
}

// AUTH must not be advertised — this listener is the MX endpoint for other
// MTAs, not a user submission endpoint. Confirms the Auth handler refuses
// any mechanism a confused client tries to negotiate.
func TestSMTPInboundSession_AuthRefused(t *testing.T) {
	sess := &smtpInboundSession{app: nil, hostname: "mx.tinycld.test"}
	if mechs := sess.AuthMechanisms(); len(mechs) != 0 {
		t.Errorf("expected zero AUTH mechanisms, got %v", mechs)
	}
	_, err := sess.Auth("PLAIN")
	if err == nil {
		t.Fatalf("expected Auth to be refused")
	}
}

// Data without any accepted recipient returns 554 — protects against clients
// that issue DATA after RCPT TO failures.
func TestSMTPInboundSession_DataWithoutRecipientsFails(t *testing.T) {
	app := setupInboundTestApp(t)
	sess := &smtpInboundSession{app: app, hostname: "mx.tinycld.test"}
	_ = sess.Mail("sender@external.example", &smtp.MailOptions{})

	err := sess.Data(strings.NewReader("From: a@b\r\nTo: c@d\r\nSubject: x\r\n\r\nbody"))
	if err == nil {
		t.Fatalf("expected error on DATA with no recipients")
	}
}

// buildPlainRFC5322 returns a minimal text/plain RFC 5322 message that
// processInboundForMailbox can parse and thread.
func buildPlainRFC5322(t *testing.T, from, to, subject, messageIDLocal, body string) []byte {
	t.Helper()
	return []byte(strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Date: Mon, 02 Jan 2006 15:04:05 +0000",
		"Message-ID: <" + messageIDLocal + "@example.org>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n"))
}
