package mail

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/acme/autocert"
)

// StartSMTPInboundServer starts the public-facing SMTP listener that accepts
// inbound mail from other MTAs (this is the MX target operators publish in
// DNS when using the self-hosted SMTP provider in "smtp" inbound mode). It is
// distinct from StartSMTPServer (mail/server/smtp_server.go), which handles
// authenticated client submission for users sending mail.
//
// The listener is started when MAIL_INBOUND_SMTP_ENABLED=true. We deliberately
// gate on env rather than scanning org settings: only the operator (with
// system access) decides whether port 25 is open, and most cloud VMs block
// outbound 25 too — so we don't want the app silently trying to grab the
// port based on a user-editable setting.
//
// Default port is 25 (the MX port); SMTP_INBOUND_ADDR can override (handy
// for dev where 25 needs root). AUTH is never offered — this is server-to-
// server traffic, not user submission. The session dispatches each accepted
// recipient through processInboundForMailbox, the same path the Postmark
// webhook uses, so all downstream behavior (threading, FTS, notifications)
// stays identical.
func StartSMTPInboundServer(app *pocketbase.PocketBase, certManager *autocert.Manager) (func(), error) {
	if os.Getenv("MAIL_INBOUND_SMTP_ENABLED") != "true" {
		return func() {}, nil
	}

	addr := os.Getenv("SMTP_INBOUND_ADDR")
	if addr == "" {
		addr = ":25"
	}

	tlsConfig, err := resolveTLSConfig("SMTP_INBOUND_TLS_CERT", "SMTP_INBOUND_TLS_KEY", "SMTP_TLS_CERT", "SMTP_TLS_KEY", certManager)
	if err != nil {
		return nil, err
	}

	hostname := os.Getenv("SMTP_PUBLIC_HOSTNAME")
	if hostname == "" {
		hostname = "localhost"
	}

	backend := &smtpInboundBackend{app: app, hostname: hostname}
	server := smtp.NewServer(backend)
	server.Domain = hostname
	server.AllowInsecureAuth = false
	server.MaxMessageBytes = 25 << 20
	server.EnableSMTPUTF8 = true
	server.ReadTimeout = 60 * time.Second
	server.WriteTimeout = 60 * time.Second
	server.Addr = addr
	if tlsConfig != nil {
		server.TLSConfig = tlsConfig
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("inbound SMTP listen on %s: %w", addr, err)
	}
	app.Logger().Info("inbound SMTP server listening", "addr", addr, "hostname", hostname, "starttls", tlsConfig != nil)

	go func() {
		if err := server.Serve(ln); err != nil {
			app.Logger().Error("inbound SMTP server error", "addr", addr, "error", err)
		}
	}()

	return func() {
		app.Logger().Info("shutting down inbound SMTP server")
		ln.Close()
		server.Close()
	}, nil
}

type smtpInboundBackend struct {
	app      core.App
	hostname string
}

func (b *smtpInboundBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &smtpInboundSession{app: b.app, hostname: b.hostname}, nil
}

// smtpInboundSession implements smtp.Session for one incoming connection.
// Recipients are accepted only when they resolve to a local mailbox (no open
// relay); DATA is parsed via the SMTP provider's ParseInbound and dispatched
// through processInboundForMailbox once per accepted recipient.
//
// The app field is typed as core.App (the minimal interface this code needs)
// so production *pocketbase.PocketBase and the test *tests.TestApp both
// satisfy it without conversion.
type smtpInboundSession struct {
	app      core.App
	hostname string

	from      string
	mailboxes []recipientBinding // accepted recipients + resolved mailbox
}

type recipientBinding struct {
	address string
	mailbox *core.Record
}

// AuthMechanisms returns no mechanisms — AUTH is disabled on the inbound
// server (it's an MX endpoint, not a submission endpoint).
func (s *smtpInboundSession) AuthMechanisms() []string { return nil }

func (s *smtpInboundSession) Auth(_ string) (sasl.Server, error) {
	return nil, &smtp.SMTPError{
		Code:         502,
		EnhancedCode: smtp.EnhancedCode{5, 5, 1},
		Message:      "Authentication not supported on inbound MX endpoint",
	}
}

func (s *smtpInboundSession) Mail(from string, _ *smtp.MailOptions) error {
	s.from = from
	s.mailboxes = nil
	return nil
}

// Rcpt resolves each recipient to a local mailbox. Unknown recipients get
// 550 5.1.1 so the sending MTA generates a clean bounce instead of
// silently accepting and discarding. Plus-tags are stripped (support+tag@
// → support@) to mirror the webhook path's resolveMailboxByAddress.
func (s *smtpInboundSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	localPart, domain := splitAddress(to)
	if localPart == "" || domain == "" {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 3},
			Message:      "Invalid recipient address",
		}
	}
	localPart = stripPlusTag(localPart)
	mailbox, _, err := resolveMailboxByAddress(s.app, localPart, domain)
	if err != nil {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 1},
			Message:      "No such mailbox",
		}
	}
	s.mailboxes = append(s.mailboxes, recipientBinding{address: to, mailbox: mailbox})
	return nil
}

// Data reads the RFC 5322 message and dispatches it to every accepted mailbox.
// Storage failures on individual mailboxes are logged but don't fail the SMTP
// transaction — the sender would retry the whole batch and we'd risk infinite
// loops; better to surface partial delivery in logs than to bounce the rest.
func (s *smtpInboundSession) Data(r io.Reader) error {
	if len(s.mailboxes) == 0 {
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 5, 1},
			Message:      "No valid recipients",
		}
	}

	body, err := io.ReadAll(io.LimitReader(r, 25<<20))
	if err != nil {
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 7, 0},
			Message:      "Failed to read message",
		}
	}

	// Use the SMTP provider's ParseInbound — it just wraps parseRFC5322 in
	// the InboundMessage shape that processInboundForMailbox expects.
	parser := NewSMTPProvider(SMTPConfig{PublicHostname: s.hostname})
	msg, err := parser.ParseInbound(body)
	if err != nil {
		s.app.Logger().Error("inbound SMTP: parse failed", "error", err, "from", s.from)
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 6, 0},
			Message:      "Message could not be parsed",
		}
	}

	storedAny := false
	for _, binding := range s.mailboxes {
		if err := processInboundForMailbox(s.app, binding.mailbox, msg); err != nil {
			s.app.Logger().Error("inbound SMTP: storage failed",
				"mailboxID", binding.mailbox.Id, "address", binding.address, "error", err)
			continue
		}
		storedAny = true
	}

	if !storedAny {
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 3, 0},
			Message:      "Temporary storage failure",
		}
	}
	return nil
}

func (s *smtpInboundSession) Reset() {
	s.from = ""
	s.mailboxes = nil
}

func (s *smtpInboundSession) Logout() error {
	s.Reset()
	return nil
}

var _ smtp.Session = (*smtpInboundSession)(nil)
