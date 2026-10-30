package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/acme/autocert"
	"tinycld.org/core/mailproto"
	"tinycld.org/core/readonly"
)

// listenInbound serves on the supervisor's "smtp" listener, or binds the
// address itself when there is none — the same seam StartIMAPServer and
// StartSMTPServer use for their own names. mail still terminates STARTTLS
// itself here, unlike the injected-listener path (startSMTPInboundOnListener)
// a different embedding uses. Unlike IMAP/submission, this server always
// binds exactly one address (there is no separate dev address), so the
// inherited listener is always for the address actually requested.
func listenInbound(addr string) (net.Listener, error) {
	return acceptingListeners.listen("smtp", addr)(addr)
}

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
//
// Unlike the IMAP/submission listeners, this one keeps its own bind loop rather
// than going through mailproto.StartSMTP: it is opt-in via env, always binds a
// single plain listener (STARTTLS only), and derives its domain from the public
// hostname. Forcing it through the shared options struct would distort both.
// It does share the TLS resolution.
func StartSMTPInboundServer(app core.App, certManager *autocert.Manager) (func(), error) {
	if os.Getenv("MAIL_INBOUND_SMTP_ENABLED") != "true" {
		return func() {}, nil
	}

	addr := os.Getenv("SMTP_INBOUND_ADDR")
	if addr == "" {
		addr = ":25"
	}

	tlsConfig, err := mailproto.ResolveTLSConfig(
		"SMTP_INBOUND_TLS_CERT", "SMTP_INBOUND_TLS_KEY", "SMTP_TLS_CERT", "SMTP_TLS_KEY", certManager,
	)
	if err != nil {
		return nil, err
	}

	hostname := os.Getenv("SMTP_PUBLIC_HOSTNAME")
	if hostname == "" {
		hostname = "localhost"
	}

	server := newInboundSMTPServer(app, hostname, tlsConfig)
	server.Addr = addr

	ln, err := listenInbound(addr)
	if err != nil {
		return nil, fmt.Errorf("inbound SMTP listen on %s: %w", addr, err)
	}
	app.Logger().Info("inbound SMTP server listening", "addr", addr, "hostname", hostname, "starttls", tlsConfig != nil)

	return mailproto.ServeSMTP(app, server, ln, "inbound SMTP"), nil
}

// maxInboundMessageBytes is the size limit of one inbound message. go-smtp
// enforces it while Data reads the message.
const maxInboundMessageBytes = 25 << 20

// maxInboundRecipients is the RCPT limit of one inbound transaction.
const maxInboundRecipients = 100

// newInboundSMTPServer builds the inbound-MX server around the shared inbound
// backend; both the own-ports TCP bind and the injected-listener path
// serve exactly this instance, through mailproto.ServeSMTP, which drains it at
// the app's terminate.
func newInboundSMTPServer(app core.App, hostname string, tlsConfig *tls.Config) *smtp.Server {
	backend := &smtpInboundBackend{app: app, hostname: hostname}
	server := smtp.NewServer(backend)
	server.Domain = hostname
	server.AllowInsecureAuth = false
	server.MaxMessageBytes = maxInboundMessageBytes
	// Each RCPT runs an address lookup and holds the inboxes it reaches (one
	// per member for a group address), so the count is capped. 100 is the
	// least RFC 5321 (4.5.3.1.8) lets a server accept.
	server.MaxRecipients = maxInboundRecipients
	server.EnableSMTPUTF8 = true
	server.ReadTimeout = 60 * time.Second
	server.WriteTimeout = 60 * time.Second
	if tlsConfig != nil {
		server.TLSConfig = tlsConfig
	}
	return server
}

// startSMTPInboundOnListener serves the inbound-MX session on an injected
// plaintext listener — the injected path. The supervisor owns :25: it
// offers and terminates STARTTLS there, routes each transaction by RCPT TO
// domain through the control-plane registry, and relays accepted messages
// here over the org's private unix socket. There is no
// MAIL_INBOUND_SMTP_ENABLED gate on this path: whether :25 is open is the
// supervisor operator's call, and a handed-down listener nothing dials costs
// nothing.
func startSMTPInboundOnListener(app core.App, listen mailproto.ListenFunc) (func(), error) {
	hostname := inboundHostname(app)
	server := newInboundSMTPServer(app, hostname, nil)

	ln, err := listen(":25")
	if err != nil {
		return nil, fmt.Errorf("inbound SMTP listen: %w", err)
	}
	app.Logger().Info("inbound SMTP server listening (external TLS termination)", "hostname", hostname)

	return mailproto.ServeSMTP(app, server, ln, "inbound SMTP"), nil
}

// inboundHostname resolves the identity the MX greeting announces:
// SMTP_PUBLIC_HOSTNAME when the operator set one, else the app's public host —
// under a supervisor the allowlist env is empty, but AppURL was adopted from
// the app config the supervisor wrote at boot, so this yields the
// deployment's own public host.
func inboundHostname(app core.App) string {
	if h := os.Getenv("SMTP_PUBLIC_HOSTNAME"); h != "" {
		return h
	}
	if u, err := url.Parse(app.Settings().Meta.AppURL); err == nil && u.Host != "" {
		return u.Host
	}
	return "localhost"
}

type smtpInboundBackend struct {
	app      core.App
	hostname string
}

func (b *smtpInboundBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &smtpInboundSession{app: b.app, hostname: b.hostname}, nil
}

// smtpInboundSession implements smtp.Session for one incoming connection.
// Recipients are accepted only when they resolve to a local mailbox (no open
// relay); DATA is parsed via the SMTP provider's ParseInbound and dispatched
// through processInboundForMailbox once per inbox the accepted recipients
// reach.
//
// The app field is typed as core.App (the minimal interface this code needs)
// so production core.App and the test *tests.TestApp both
// satisfy it without conversion.
type smtpInboundSession struct {
	app      core.App
	hostname string

	from      string
	mailboxes []recipientBinding // accepted recipients + the inboxes they reach
}

type recipientBinding struct {
	address string
	targets []deliveryTarget
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

// errReadOnly defers a message while this process is read-only. It is then
// only waiting to be replaced, and a message stored now is lost if the next
// build rolls back; a 451 makes the sending MTA queue it and retry.
var errReadOnly = &smtp.SMTPError{
	Code:         451,
	EnhancedCode: smtp.EnhancedCode{4, 3, 2},
	Message:      "Mail service is restarting; try again later",
}

func (s *smtpInboundSession) Mail(from string, _ *smtp.MailOptions) error {
	if readonly.Active() {
		return errReadOnly
	}
	s.from = from
	s.mailboxes = nil
	return nil
}

// errRecipientLookupFailed defers a recipient whose lookup failed on our
// side. A 550 would bounce mail a retry delivers; a 451 makes the sending
// MTA queue it.
var errRecipientLookupFailed = &smtp.SMTPError{
	Code:         451,
	EnhancedCode: smtp.EnhancedCode{4, 3, 0},
	Message:      "Temporary failure looking up the recipient; try again later",
}

// errMXRelayDenied refuses a recipient on a domain this server does not host.
// The MX is only for inbound mail; a mail app that reaches it by mistake
// (Apple Mail tries port 25 first) shows this text, so it says how to send.
var errMXRelayDenied = &smtp.SMTPError{
	Code:         550,
	EnhancedCode: smtp.EnhancedCode{5, 7, 1},
	Message:      "Relaying denied. To send mail, log in on port 465.",
}

// Rcpt resolves each recipient to the local inboxes it delivers to: one for
// an inbox address, one per member for a group address. Unknown recipients,
// and a group with no member who can receive mail, get 550 5.1.1 so the
// sending MTA generates a clean bounce instead of silently accepting and
// discarding. Plus-tags are stripped (support+tag@ → support@) to mirror the
// webhook path.
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
	if !mailDomainHosted(s.app, domain) {
		return errMXRelayDenied
	}
	targets, err := deliveryTargets(s.app, localPart, domain)
	if errors.Is(err, errAddressNotFound) {
		return &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 1},
			Message:      "No such mailbox",
		}
	}
	if err != nil {
		addressLog.Error("inbound SMTP: recipient lookup failed", "rcpt", to, "error", err)
		return errRecipientLookupFailed
	}
	s.mailboxes = append(s.mailboxes, recipientBinding{address: to, targets: targets})
	return nil
}

// Data reads the RFC 5322 message and dispatches it to every inbox the
// accepted recipients reach, once per inbox, in RCPT order.
//
// A storage failure in any inbox answers 451 when the message has a
// Message-ID: the sending MTA retries the whole message, and the retry skips
// the inboxes that already hold that Message-ID, so only the missing copies
// are stored. Without a Message-ID a retry would store a second copy in every
// inbox that succeeded, so a partial failure is accepted (250) and logged as
// an error instead. When nothing is stored the answer is always 451.
func (s *smtpInboundSession) Data(r io.Reader) error {
	// Read-only can begin after MAIL FROM was accepted, so DATA checks again
	// before it stores anything.
	if readonly.Active() {
		return errReadOnly
	}
	if len(s.mailboxes) == 0 {
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 5, 1},
			Message:      "No valid recipients",
		}
	}

	// Read to the end: go-smtp refuses a message over MaxMessageBytes while
	// it is read, and the drain treats the store as a write only once the
	// reader reaches EOF. A LimitReader of the cap stopped there without EOF:
	// a message at the cap was stored cut short, and could be cut again by a
	// shutdown in the middle of its store.
	body, err := io.ReadAll(r)
	if errors.Is(err, smtp.ErrDataTooLarge) {
		return smtp.ErrDataTooLarge
	}
	if err != nil {
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 7, 0},
			Message:      "Failed to read message",
		}
	}
	// Use the SMTP provider's ParseInbound — it just wraps parseRFC5322 in
	// the InboundMessage shape that processInboundForMailbox expects.
	parser := NewSMTPProvider(s.app, SMTPConfig{PublicHostname: s.hostname})
	msg, err := parser.ParseInbound(body)
	if err != nil {
		s.app.Logger().Error("inbound SMTP: parse failed", "error", err, "from", s.from)
		return &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 6, 0},
			Message:      "Message could not be parsed",
		}
	}

	stored, failed := 0, 0
	var delivered targetSet
	for _, binding := range s.mailboxes {
		for _, target := range binding.targets {
			if !delivered.add(target) {
				continue
			}
			if err := processInboundForMailbox(s.app, target.Mailbox, msg, target.AddressID); err != nil {
				addressLog.Error("inbound SMTP: storage failed",
					"mailboxID", target.Mailbox.Id, "address", binding.address, "messageID", msg.MessageID, "error", err)
				failed++
				continue
			}
			stored++
		}
	}

	if stored == 0 || (failed > 0 && retryIsSafe(msg)) {
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 3, 0},
			Message:      "Temporary storage failure",
		}
	}
	if failed > 0 {
		addressLog.Error("inbound SMTP: accepted a partial delivery; with no Message-ID a retry would store duplicates",
			"from", s.from, "stored", stored, "failed", failed)
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
