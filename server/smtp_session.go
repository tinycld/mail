package mail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/coreserver"
	"tinycld.org/core/davauth"
	"tinycld.org/core/logging"
	"tinycld.org/core/mailer"
	"tinycld.org/core/pkgaccess"
	"tinycld.org/core/readonly"
)

type smtpBackend struct {
	app core.App
}

func (b *smtpBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &smtpSession{app: b.app}, nil
}

type smtpSession struct {
	app core.App

	// Auth state (persists across transactions)
	user *core.Record

	// Transaction state (reset between MAIL FROM commands)
	from       string
	mailbox    *core.Record
	address    *core.Record // the mail_addresses row the session sends as
	domain     *core.Record
	recipients []string
}

var smtpLog = logging.ForPackage("mail")

// refuse logs a submission refusal and returns it. The client sees only the
// SMTP reply, which a mail app usually hides behind "cannot send"; without
// this record an operator has nothing to read. Never pass a password in
// attrs.
func (s *smtpSession) refuse(stage string, reply *smtp.SMTPError, cause error, attrs ...any) error {
	return s.logRefusal(slog.LevelInfo, stage, reply, cause, attrs...)
}

// refuseServerSide logs a refusal the client did not cause: a read or
// provider failure, or the send gate. An operator needs to see these.
func (s *smtpSession) refuseServerSide(stage string, reply *smtp.SMTPError, cause error, attrs ...any) error {
	return s.logRefusal(slog.LevelWarn, stage, reply, cause, attrs...)
}

func (s *smtpSession) logRefusal(level slog.Level, stage string, reply *smtp.SMTPError, cause error, attrs ...any) error {
	args := []any{"stage", stage, "code", reply.Code, "reply", reply.Message}
	if s.user != nil {
		args = append(args, "user", s.user.Id)
	}
	if s.from != "" {
		args = append(args, "from", s.from)
	}
	if cause != nil {
		args = append(args, "error", cause.Error())
	}
	args = append(args, attrs...)
	smtpLog.Log(context.Background(), level, "smtp submission refused", args...)
	return reply
}

// AuthMechanisms returns supported SASL mechanisms.
func (s *smtpSession) AuthMechanisms() []string {
	return []string{"PLAIN", "LOGIN"}
}

// Auth handles SASL authentication.
func (s *smtpSession) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case "PLAIN":
		return sasl.NewPlainServer(func(identity, username, password string) error {
			return s.authenticate(username, password)
		}), nil
	case "LOGIN":
		return &loginServer{auth: s.authenticate}, nil
	default:
		return nil, fmt.Errorf("unsupported mechanism")
	}
}

// loginServer implements sasl.Server for the LOGIN mechanism.
// LOGIN is a simple two-step exchange: server challenges with "Username:" then
// "Password:", client responds with each value.
type loginServer struct {
	auth     func(username, password string) error
	username string
	step     int
}

func (s *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch s.step {
	case 0:
		s.step++
		return []byte("Username:"), false, nil
	case 1:
		s.username = string(response)
		s.step++
		return []byte("Password:"), false, nil
	case 2:
		s.step++
		err := s.auth(s.username, string(response))
		return nil, true, err
	default:
		return nil, false, fmt.Errorf("unexpected LOGIN step")
	}
}

// authenticate validates AUTH credentials. davauth.VerifyCredentials is the
// shared protocol-server check: a username or a hosted address (mail's own,
// through addressOwner), and the disabled cutoff (this listener bypasses
// PocketBase's auth hooks, and unlike REST tokens a protocol login never
// expires). One opaque failure keeps the account's state invisible to a
// prober.
func (s *smtpSession) authenticate(username, password string) error {
	record, err := davauth.VerifyCredentials(s.app, username, password)
	if err != nil {
		return s.refuse("auth", &smtp.SMTPError{
			Code:         535,
			EnhancedCode: smtp.EnhancedCode{5, 7, 8},
			Message:      "Authentication credentials invalid",
		}, err, "username", username)
	}

	// SMTP submission exists only to send, and sending is a write: an
	// org_pkg_access level below full refuses at AUTH. (IMAP still logs the
	// same user in — readonly means read.)
	if !pkgaccess.CanWrite(s.app, record, "mail") {
		return s.refuse("auth", &smtp.SMTPError{
			Code:         535,
			EnhancedCode: smtp.EnhancedCode{5, 7, 8},
			Message:      "Your mail access is read-only; sending is not permitted",
		}, nil, "username", username)
	}

	s.user = record
	return nil
}

// Mail handles MAIL FROM command — validates the sender is authorized.
func (s *smtpSession) Mail(from string, opts *smtp.MailOptions) error {
	// A new MAIL FROM starts a new transaction even when it is refused;
	// otherwise the old sender stays live and appears in the refusal log.
	s.Reset()
	if s.user == nil {
		return s.refuse("mail", &smtp.SMTPError{
			Code:         530,
			EnhancedCode: smtp.EnhancedCode{5, 7, 0},
			Message:      "Authentication required",
		}, nil, "from", from)
	}
	// A message is sent only when its Sent copy can be stored, so nothing
	// is accepted while the server is read-only; the 451 makes the client
	// keep the message in its outbox and retry.
	if readonly.Active(s.app) {
		return s.refuse("mail", errReadOnly, nil, "from", from)
	}

	local, domainStr := splitAddress(from)
	if local == "" || domainStr == "" {
		return s.refuse("mail", &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 0},
			Message:      "Sender address not recognized",
		}, nil, "from", from)
	}

	target, err := resolveAddress(s.app, local, domainStr)
	if err != nil && !errors.Is(err, errAddressNotFound) {
		return s.refuseServerSide("mail", &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 3, 0},
			Message:      "Temporary failure looking up the sender address; try again later",
		}, err, "from", from)
	}
	if err != nil {
		return s.refuse("mail", &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 1, 0},
			Message:      "Sender address not recognized",
		}, err, "from", from)
	}

	if _, err := verifyMailboxMembership(s.app, target.Mailbox.Id, s.user.Id); err != nil {
		return s.refuse("mail", &smtp.SMTPError{
			Code:         550,
			EnhancedCode: smtp.EnhancedCode{5, 7, 1},
			Message:      "Not authorized to send from this address",
		}, err, "from", from)
	}

	s.from = from
	s.mailbox = target.Mailbox
	s.address = target.Address
	s.domain = target.Domain
	s.recipients = nil

	return nil
}

// Rcpt handles RCPT TO command — collects recipients.
func (s *smtpSession) Rcpt(to string, opts *smtp.RcptOptions) error {
	if s.user == nil {
		return s.refuse("rcpt", &smtp.SMTPError{
			Code:         530,
			EnhancedCode: smtp.EnhancedCode{5, 7, 0},
			Message:      "Authentication required",
		}, nil, "to", to)
	}
	if s.from == "" {
		return s.refuse("rcpt", &smtp.SMTPError{
			Code:         503,
			EnhancedCode: smtp.EnhancedCode{5, 5, 1},
			Message:      "Bad sequence of commands",
		}, nil, "to", to)
	}

	if !strings.Contains(to, "@") {
		return s.refuse("rcpt", &smtp.SMTPError{
			Code:         553,
			EnhancedCode: smtp.EnhancedCode{5, 1, 3},
			Message:      "Invalid recipient address",
		}, nil, "to", to)
	}

	if len(s.recipients) >= 100 {
		return s.refuse("rcpt", &smtp.SMTPError{
			Code:         452,
			EnhancedCode: smtp.EnhancedCode{4, 5, 3},
			Message:      "Too many recipients",
		}, nil, "to", to)
	}

	s.recipients = append(s.recipients, to)
	return nil
}

// Data handles the DATA command — parses the RFC 5322 message and sends it
// through the existing provider pipeline.
func (s *smtpSession) Data(r io.Reader) error {
	if s.user == nil {
		return s.refuse("data", &smtp.SMTPError{
			Code:         530,
			EnhancedCode: smtp.EnhancedCode{5, 7, 0},
			Message:      "Authentication required",
		}, nil)
	}
	if s.from == "" || len(s.recipients) == 0 {
		return s.refuse("data", &smtp.SMTPError{
			Code:         503,
			EnhancedCode: smtp.EnhancedCode{5, 5, 1},
			Message:      "Bad sequence of commands",
		}, nil)
	}

	raw, err := io.ReadAll(r)
	if err != nil {
		return s.refuseServerSide("data", &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 7, 0},
			Message:      "Temporary delivery failure",
		}, err)
	}

	msg, err := parseRFC5322(raw)
	if err != nil {
		return s.refuse("data", &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 6, 0},
			Message:      "Message could not be parsed",
		}, err)
	}

	// Build From address from the mailbox record — we intentionally use the
	// mailbox's configured display name, not the one from the message headers,
	// to enforce consistent sender identity across all channels.
	displayName := s.mailbox.GetString("display_name")
	domainName := s.domain.GetString("domain")
	senderEmail := s.address.GetString("local_part") + "@" + domainName
	fromAddr := formatFromAddress(displayName, senderEmail)

	// Build To/Cc from parsed headers, derive Bcc from RCPT TO envelope
	toMap := make(map[string]bool)
	for _, r := range msg.To {
		toMap[strings.ToLower(r.Email)] = true
	}
	for _, r := range msg.Cc {
		toMap[strings.ToLower(r.Email)] = true
	}

	var bcc []Recipient
	for _, rcpt := range s.recipients {
		if !toMap[strings.ToLower(rcpt)] {
			bcc = append(bcc, Recipient{Email: rcpt})
		}
	}

	// Convert InboundAttachment → Attachment for the provider
	var attachments []Attachment
	for _, att := range msg.Attachments {
		attachments = append(attachments, Attachment{
			Name:        att.Name,
			ContentType: att.ContentType,
			Content:     att.Content,
		})
	}

	// Resolve provider
	provider := newProviderFromSystem(s.app)

	// Preserve threading headers from the original message
	inReplyToHeader := msg.InReplyTo
	referencesHeader := msg.References
	if referencesHeader == "" && inReplyToHeader != "" {
		referencesHeader = inReplyToHeader
	}

	// Send under the client's own Message-ID: most clients APPEND their copy
	// of the message to Sent afterwards, and that copy can only be matched to
	// the one stored below if both carry the same ID.
	messageID := normalizeMessageID(msg.MessageID)
	if messageID == "" {
		messageID = mailer.GenerateMessageID(domainName)
	}

	sendReq := &SendRequest{
		MessageID:   messageID,
		From:        fromAddr,
		To:          msg.To,
		Cc:          msg.Cc,
		Bcc:         bcc,
		Subject:     msg.Subject,
		HTMLBody:    msg.HTMLBody,
		TextBody:    msg.TextBody,
		InReplyTo:   inReplyToHeader,
		References:  referencesHeader,
		Attachments: attachments,

		// Same attribution the /send endpoint gives: delivery and bounce
		// notifications carry no From address, only this metadata.
		Metadata: map[string]string{senderDomainMetadataKey: strings.ToLower(domainName)},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Read-only can begin after MAIL FROM was accepted, so the send checks
	// again before anything goes out.
	if readonly.Active(s.app) {
		return s.refuse("data", errReadOnly, nil)
	}

	// Count the true fan-out: header To/Cc plus the envelope-only Bcc derived
	// above. Not len(s.recipients), which includes addresses already in To/Cc
	// and would double-count them.
	//
	// Unlike the persistence failures further down, a refusal here MUST be
	// returned. Those return nil because provider.Send already succeeded — the
	// mail was delivered and only the local copy failed. This gate sits above
	// the send, so refusing and returning nil would accept the message from
	// the client and silently drop it.
	if refusal := checkSendAllowed(s.app, s.user.Id, s.mailbox.Id, s.domain, len(msg.To)+len(msg.Cc)+len(bcc)); refusal != nil {
		return s.refuseServerSide("data", smtpErrorForRefusal(refusal), nil, "gate", refusal.msg)
	}

	var result *SendResult
	if s.user != nil && coreserver.IsDemoUser(s.app, s.user.Id) {
		// Demo user: skip relay, synthesize a result so the local Sent-folder
		// persistence below runs unchanged.
		result = &SendResult{MessageID: messageID}
	} else {
		var sendErr error
		result, sendErr = provider.Send(ctx, sendReq)
		if sendErr != nil {
			return s.refuseServerSide("data", &smtp.SMTPError{
				Code:         451,
				EnhancedCode: smtp.EnhancedCode{4, 7, 0},
				Message:      "Temporary delivery failure",
			}, sendErr, "recipients", len(s.recipients))
		}
	}

	// The provider send succeeded, so the message was delivered: the reply
	// is 250 whatever happens to the Sent copy. Read-only can begin during
	// the send, and the copy must not be written then. The client waits for
	// this reply, so the save waits out the mode, bounded by
	// readonly.TailWait — well inside the 10 minutes RFC 5321 4.5.3.2.6
	// lets a client wait for the reply to DATA.
	tailCtx, release := readonly.TailContext(s.app)
	defer release()
	sent := sentCopy{
		msg: msg, result: result, displayName: displayName, senderEmail: senderEmail, fromAddr: fromAddr,
		inReplyTo: inReplyToHeader, references: referencesHeader, bcc: bcc,
	}
	if err := readonly.WhenWritable(tailCtx, s.app, func() error {
		s.storeSentCopy(sent)
		return nil
	}); err != nil {
		smtpLog.Error("SMTP: message sent but the server stayed read-only — it will not appear in the Sent folder",
			"error", err, "subject", msg.Subject, "from", fromAddr)
	}
	return nil
}

// sentCopy is what storeSentCopy needs of a message the provider has sent.
type sentCopy struct {
	msg         *storedMessage
	result      *SendResult
	displayName string
	senderEmail string
	fromAddr    string
	inReplyTo   string
	references  string
	bcc         []Recipient
}

// storeSentCopy stores the local copy of a sent message. The provider Send
// already succeeded, so storage failures are logged but don't fail the SMTP
// transaction — the message was delivered, it just won't appear in the
// user's Sent folder.
func (s *smtpSession) storeSentCopy(c sentCopy) {
	msg, result := c.msg, c.result
	now := time.Now().UTC().Format(time.RFC3339)
	mailboxID := s.mailbox.Id

	// Dedup: if the client already APPENDed this Message-ID to Sent (or this
	// is an SMTP retry), keep that copy and record the send on it. Only an
	// outgoing copy counts: a redirected received message or a saved draft
	// can carry the same Message-ID, and this send must not be recorded on it.
	existingMsg, existingThread, _ := findMessageInMailbox(s.app, mailboxID, result.MessageID)
	if existingMsg != nil && isOutgoingMessage(existingMsg) {
		existingMsg.Set("sent_by", s.user.Id)
		existingMsg.Set("sent_from", s.address.Id)
		if result.ProviderMessageID != "" {
			existingMsg.Set("provider_message_id", result.ProviderMessageID)
		}
		if err := s.app.Save(existingMsg); err != nil {
			smtpLog.Error("SMTP: failed to record the send on the deduped message", "error", err)
		}
		if err := addAddressKeys(s.app, existingThread.Id, s.address.Id); err != nil {
			storeLog.Warn("SMTP: recording the deduped thread's addresses failed", "thread", existingThread.Id, "error", err)
		}
		if err := markThreadSent(s.app, existingThread.Id, s.user.Id); err != nil {
			smtpLog.Error("SMTP: failed to mark deduped thread sent", "error", err)
		}
		idleNotifier(s.app).Notify(mailboxID)
		return
	}

	thread, err := findOrCreateThread(s.app, mailboxID, msg.Subject, c.inReplyTo, c.references)
	if err != nil {
		smtpLog.Error("SMTP: message sent but failed to create thread — will not appear in Sent folder",
			"error", err, "subject", msg.Subject, "from", c.fromAddr)
		return
	}

	// Same deduped count the gate uses: s.recipients is the full envelope and
	// repeats every address that also appears in To/Cc, so counting it here
	// inflated the total. deliveryStatusForResult marks a message "bounced"
	// only when len(FailedRecipients) >= totalRecipients, so an inflated
	// total meant a send whose every recipient failed was still recorded
	// "sent" — one address in both the header and the envelope counted twice,
	// and 1 >= 2 is false.
	deliveryStatus, bounceReason := deliveryStatusForResult(result, len(msg.To)+len(msg.Cc)+len(c.bcc))

	storedMsg := &storedMessage{
		MessageID:         result.MessageID,
		ProviderMessageID: result.ProviderMessageID,
		InReplyTo:         c.inReplyTo,
		SentBy:            s.user.Id,
		SenderName:        c.displayName,
		SenderEmail:       c.senderEmail,
		To:                msg.To,
		Cc:                msg.Cc,
		Date:              now,
		Subject:           msg.Subject,
		HTMLBody:          msg.HTMLBody,
		TextBody:          msg.TextBody,
		DeliveryStatus:    deliveryStatus,
		BounceReason:      bounceReason,
		Attachments:       msg.Attachments,
		SentFrom:          s.address.Id,
	}

	if _, err := storeMessage(s.app, thread.Id, storedMsg); err != nil {
		smtpLog.Error("SMTP: message sent but failed to store — will not appear in Sent folder",
			"error", err, "subject", msg.Subject, "from", c.fromAddr)
		return
	}

	if err := updateThreadMetadata(s.app, thread.Id, c.displayName, storedMsg.SenderEmail, storedMsg.TextBody, now); err != nil {
		smtpLog.Error("SMTP: failed to update thread metadata", "error", err)
	}

	if err := markThreadSent(s.app, thread.Id, s.user.Id); err != nil {
		smtpLog.Error("SMTP: failed to mark thread sent", "error", err)
	}

	idleNotifier(s.app).Notify(mailboxID)
}

// Reset clears transaction state but preserves authentication.
func (s *smtpSession) Reset() {
	s.from = ""
	s.mailbox = nil
	s.address = nil
	s.domain = nil
	s.recipients = nil
}

// Logout clears all session state.
func (s *smtpSession) Logout() error {
	s.user = nil
	s.Reset()
	return nil
}

// Ensure smtpSession implements the AuthSession interface.
var _ smtp.AuthSession = (*smtpSession)(nil)
