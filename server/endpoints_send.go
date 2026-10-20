package mail

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"tinycld.org/core/coreserver"
	"tinycld.org/core/mailer"
	"tinycld.org/packages/mail/api"
)

// demoMessageID returns a synthesized RFC-822-ish Message-ID for messages
// "sent" by demo accounts. The "demo-" prefix makes simulated sends easy to
// distinguish from real ones in the database for forensic purposes.
func demoMessageID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "demo-" + hex.EncodeToString(buf) + "@tinycld.local"
}

// toMailerRecipients converts the wire-contract recipient type to the
// provider/store type. The shapes are identical, but api is deliberately
// dependency-free so the generator and the CLI can consume it standalone.
func toMailerRecipients(rs []api.Recipient) []Recipient {
	if rs == nil {
		return nil
	}
	out := make([]Recipient, len(rs))
	for i, r := range rs {
		out[i] = Recipient{Name: r.Name, Email: r.Email}
	}
	return out
}

// sendErrKind classifies a send failure so an HTTP caller can map it back to
// the status code that path returned before sendMessage was extracted, while
// a non-HTTP caller (an automation action) can just treat it as an error.
type sendErrKind int

const (
	sendErrBadRequest sendErrKind = iota
	sendErrNotFound
	sendErrForbidden
	sendErrUpstream
	sendErrInternal
)

// sendError is a transport-free send failure: a message, a classification,
// and the underlying cause.
type sendError struct {
	kind sendErrKind
	msg  string
	err  error
}

func (e *sendError) Error() string {
	if e.err != nil {
		return e.msg + ": " + e.err.Error()
	}
	return e.msg
}

func (e *sendError) Unwrap() error { return e.err }

// asAPIError maps a send failure onto the router error the /send endpoint
// used to construct inline, preserving its original status codes.
func (e *sendError) asAPIError(re *core.RequestEvent) error {
	switch e.kind {
	case sendErrBadRequest:
		return re.BadRequestError(e.msg, e.err)
	case sendErrNotFound:
		return re.NotFoundError(e.msg, e.err)
	case sendErrForbidden:
		return re.ForbiddenError(e.msg, e.err)
	case sendErrUpstream:
		return router.NewApiError(http.StatusBadGateway, e.msg, e.err)
	default:
		return re.InternalServerError(e.msg, e.err)
	}
}

// sendParams is the transport-free input to sendMessage — everything the
// send path needs once a caller has produced it, whether that caller is the
// HTTP endpoint or an automation action handler.
//
// Context carries the send's cancellation scope: the request context for
// handleSend, the action's context for rule-driven sends.
type sendParams struct {
	Ctx                context.Context
	UserID             string
	MailboxID          string
	AliasID            string
	FromAddressID      string
	Subject            string
	HTMLBody           string
	TextBody           string
	To                 []api.Recipient
	Cc                 []api.Recipient
	Bcc                []api.Recipient
	InReplyToMessageID string
	Attachments        []Attachment
	// DraftID is the draft this send resumes, and DraftAttachments the saved
	// files of it to send; see api.SendEmailRequest.
	DraftID          string
	DraftAttachments []string
	// Headers are extra headers the message carries, such as the
	// Auto-Submitted and loop headers of a rule send.
	Headers []Header
}

// sendResultRecord is what a completed send produces: the stored message and
// the thread it landed in.
type sendResultRecord struct {
	MessageRecordID string
	ThreadID        string
}

// sendErrorForAddress maps a sender-address failure onto the endpoint's
// status codes.
func sendErrorForAddress(err error) *sendError {
	switch {
	case errors.Is(err, errAddressNotInMailbox):
		return &sendError{kind: sendErrForbidden, msg: "The sender address does not belong to this mailbox", err: err}
	case errors.Is(err, errNoSendAddress):
		return &sendError{kind: sendErrBadRequest, msg: "This mailbox has no address to send from", err: err}
	case errors.Is(err, errAddressNotFound):
		return &sendError{kind: sendErrNotFound, msg: "Sender address not found", err: err}
	default:
		return &sendError{kind: sendErrInternal, msg: "Could not resolve the sender address", err: err}
	}
}

// sendMessage performs a send end-to-end with no HTTP dependency: verifies
// mailbox membership, builds the From address, hands the message to the
// configured provider, then persists it (message row, thread metadata, and
// the sender's thread state, marked sent).
//
// It is the shared core behind both the /send endpoint (handleSend) and
// mail's native automation actions (send-message, forward-message), so a
// rule-sent message goes through exactly the same provider and persistence
// path as a user-composed one.
//
// Errors are plain values, never router API errors — callers map them onto
// their own transport. sendError carries the classification (which HTTP
// status the endpoint should report) so handleSend keeps its existing
// response codes unchanged.
func sendMessage(app core.App, p sendParams) (*sendResultRecord, error) {
	if p.MailboxID == "" {
		return nil, &sendError{kind: sendErrBadRequest, msg: "mailbox_id is required"}
	}
	if len(p.To) == 0 {
		return nil, &sendError{kind: sendErrBadRequest, msg: "at least one recipient is required"}
	}
	// An ended context means nobody waits for this send any more: the request
	// was dropped, or a rule action timed out or is cancelled at terminate.
	// Checked here as well as by provider.Send, because a demo send never
	// reaches the provider and would still store a message.
	if err := p.Ctx.Err(); err != nil {
		return nil, &sendError{kind: sendErrUpstream, msg: "The send was cancelled", err: err}
	}

	mailbox, err := app.FindRecordById("mail_mailboxes", p.MailboxID)
	if err != nil {
		return nil, &sendError{kind: sendErrNotFound, msg: "Mailbox not found", err: err}
	}

	// Resolve the deployment-wide provider from system settings
	provider := newProviderFromSystem(app)

	// Verify the user has access to this mailbox
	if _, err := verifyMailboxMembership(app, p.MailboxID, p.UserID); err != nil {
		return nil, &sendError{kind: sendErrForbidden, msg: "Not a member of this mailbox", err: err}
	}

	// Reject before doing any send work if the provider has no
	// credentials — otherwise provider.Send fails deep in the API call with an
	// opaque error after we've already built the message.
	if !provider.Configured() {
		return nil, &sendError{
			kind: sendErrBadRequest,
			msg:  "configure the mail provider in settings before sending",
			err:  errProviderNotConfigured,
		}
	}

	from, err := resolveSendAddress(app, mailbox, p.FromAddressID, p.AliasID)
	if err != nil {
		return nil, sendErrorForAddress(err)
	}

	// A resumed draft's saved files are read before anything is sent, so a
	// file that cannot be read stops the send and the draft is kept.
	attachments := p.Attachments
	var draft *core.Record
	if p.DraftID != "" {
		var saved []Attachment
		draft, saved, err = resolveSendDraft(app, p.DraftID, p.MailboxID, p.DraftAttachments)
		if err != nil {
			return nil, err
		}
		attachments = append(saved, p.Attachments...)
	}
	if err := checkAttachmentLimits(app, attachments); err != nil {
		return nil, err
	}

	domainRecord := from.Domain
	displayName := mailbox.GetString("display_name")
	domain := domainRecord.GetString("domain")
	senderEmail := from.Email()
	fromAddr := formatFromAddress(displayName, senderEmail)

	// Build threading headers if replying
	var inReplyToHeader, referencesHeader string
	if p.InReplyToMessageID != "" {
		originalMsg, err := app.FindRecordById("mail_messages", p.InReplyToMessageID)
		if err == nil {
			inReplyToHeader = originalMsg.GetString("message_id")
			referencesHeader = inReplyToHeader
		}
	}

	// Send via provider. The Message-ID is chosen here, not by the provider,
	// so the stored copy carries the header recipients see: their replies
	// thread onto it, and an IMAP client's copy of it matches.
	sendReq := &SendRequest{
		MessageID:   mailer.GenerateMessageID(domain),
		From:        fromAddr,
		To:          toMailerRecipients(p.To),
		Cc:          toMailerRecipients(p.Cc),
		Bcc:         toMailerRecipients(p.Bcc),
		Subject:     p.Subject,
		HTMLBody:    p.HTMLBody,
		TextBody:    p.TextBody,
		InReplyTo:   inReplyToHeader,
		References:  referencesHeader,
		Headers:     p.Headers,
		Attachments: attachments,

		// Echoed back on the provider's delivery and bounce notifications,
		// which carry no From address for a receiver to attribute them by.
		// fromAddr is "Name <addr@domain>" or "<addr@domain>" (formatFromAddress),
		// not a bare address — the domain record itself is the reliable source.
		Metadata: map[string]string{senderDomainMetadataKey: strings.ToLower(domain)},
	}

	// The abuse gate runs above the demo branch on purpose, so a demo account
	// exercises the same refusals without real provider credentials.
	if refusal := checkSendAllowed(app, p.UserID, p.MailboxID, domainRecord, len(p.To)+len(p.Cc)+len(p.Bcc)); refusal != nil {
		return nil, refusal
	}

	var result *SendResult
	if coreserver.IsDemoUser(app, p.UserID) {
		// Demo accounts: skip the provider call but synthesize a result so
		// the rest of the persistence path runs unchanged. Message lands in
		// the user's Sent folder; nothing leaves the box.
		result = &SendResult{MessageID: demoMessageID()}
	} else {
		var sendErr error
		result, sendErr = provider.Send(p.Ctx, sendReq)
		if sendErr != nil {
			return nil, &sendError{kind: sendErrUpstream, msg: "Failed to send email", err: sendErr}
		}
	}

	// Store in database
	now := time.Now().UTC().Format(time.RFC3339)

	thread, err := findOrCreateThread(app, p.MailboxID, p.Subject, inReplyToHeader, referencesHeader)
	if err != nil {
		return nil, &sendError{kind: sendErrInternal, msg: "Failed to create thread", err: err}
	}

	// Convert Attachment (base64) → InboundAttachment for storage
	var storedAttachments []InboundAttachment
	for _, att := range attachments {
		storedAttachments = append(storedAttachments, InboundAttachment{
			Name:        att.Name,
			ContentType: att.ContentType,
			Content:     att.Content,
		})
	}

	deliveryStatus, bounceReason := deliveryStatusForResult(result, len(p.To)+len(p.Cc)+len(p.Bcc))

	msg := &storedMessage{
		MessageID:         result.MessageID,
		ProviderMessageID: result.ProviderMessageID,
		InReplyTo:         inReplyToHeader,
		Alias:             p.AliasID,
		SentFrom:          from.Address.Id,
		SentBy:            p.UserID,
		SenderName:        displayName,
		SenderEmail:       senderEmail,
		To:                toMailerRecipients(p.To),
		Cc:                toMailerRecipients(p.Cc),
		Bcc:               toMailerRecipients(p.Bcc),
		Date:              now,
		Subject:           p.Subject,
		HTMLBody:          p.HTMLBody,
		TextBody:          p.TextBody,
		DeliveryStatus:    deliveryStatus,
		BounceReason:      bounceReason,
		Attachments:       storedAttachments,
		Headers:           p.Headers,
	}

	record, err := storeMessage(app, thread.Id, msg)
	if err != nil {
		return nil, &sendError{kind: sendErrInternal, msg: "Failed to store message", err: err}
	}

	if err := updateThreadMetadata(app, thread.Id, displayName, msg.SenderEmail, msg.TextBody, now); err != nil {
		return nil, &sendError{kind: sendErrInternal, msg: "Failed to update thread", err: err}
	}

	if err := markThreadSent(app, thread.Id, p.UserID); err != nil {
		return nil, &sendError{kind: sendErrInternal, msg: "Failed to create thread state", err: err}
	}

	if draft != nil {
		deleteSentDraft(app, draft)
	}

	return &sendResultRecord{MessageRecordID: record.Id, ThreadID: thread.Id}, nil
}

func handleSend(app core.App, re *core.RequestEvent) error {
	userID := re.Auth.Id

	var req api.SendEmailRequest
	var fileAttachments []Attachment

	contentType := re.Request.Header.Get("Content-Type")
	isMultipart := strings.HasPrefix(contentType, "multipart/form-data") ||
		strings.HasPrefix(contentType, "multipart/mixed")

	if isMultipart {
		if err := re.Request.ParseMultipartForm(25 << 20); err != nil {
			return re.BadRequestError("Failed to parse multipart form", err)
		}
		jsonStr := re.Request.FormValue(api.MultipartFieldJSON)
		if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
			return re.BadRequestError("Invalid JSON in form field", err)
		}
		var err error
		fileAttachments, err = parseFileAttachments(re)
		if err != nil {
			return re.BadRequestError("Failed to read attachments", err)
		}
	} else if strings.HasPrefix(contentType, "application/json") || contentType == "" {
		if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
			return re.BadRequestError("Invalid request body", err)
		}
	} else {
		return re.BadRequestError("Unsupported Content-Type: "+contentType, nil)
	}

	result, err := sendMessage(app, sendParams{
		Ctx:                re.Request.Context(),
		UserID:             userID,
		MailboxID:          req.MailboxID,
		AliasID:            req.AliasID,
		FromAddressID:      req.FromAddressID,
		Subject:            req.Subject,
		HTMLBody:           req.HTMLBody,
		TextBody:           req.TextBody,
		To:                 req.To,
		Cc:                 req.Cc,
		Bcc:                req.Bcc,
		InReplyToMessageID: req.InReplyToMessageID,
		Attachments:        fileAttachments,
		DraftID:            req.DraftID,
		DraftAttachments:   req.DraftAttachments,
	})
	if err != nil {
		var sendErr *sendError
		if errors.As(err, &sendErr) {
			return sendErr.asAPIError(re)
		}
		return re.InternalServerError("Failed to send email", err)
	}

	return re.JSON(http.StatusOK, api.SendEmailResponse{
		MessageID: result.MessageRecordID,
		ThreadID:  result.ThreadID,
	})
}

// deliveryStatusForResult inspects the provider's send outcome and decides
// what to persist on the stored message. Providers that don't surface
// synchronous bounces (Postmark) always return an empty FailedRecipients
// slice → status "sent" and empty reason. The SMTP provider may return
// per-recipient permanent failures inline; we surface them as bounce_reason
// regardless, but only mark the whole message "bounced" when every recipient
// failed (a partial failure still means the message reached someone).
func deliveryStatusForResult(result *SendResult, totalRecipients int) (status string, reason string) {
	if result == nil || len(result.FailedRecipients) == 0 {
		return "sent", ""
	}
	var parts []string
	for _, f := range result.FailedRecipients {
		parts = append(parts, f.Email+": "+f.Reason)
	}
	reason = strings.Join(parts, "; ")
	if totalRecipients > 0 && len(result.FailedRecipients) >= totalRecipients {
		return "bounced", reason
	}
	return "sent", reason
}

// parseFileAttachments reads uploaded files from a multipart form and returns
// them as base64-encoded Attachment structs ready for the email provider.
func parseFileAttachments(re *core.RequestEvent) ([]Attachment, error) {
	if re.Request.MultipartForm == nil {
		return nil, nil
	}
	fileHeaders := re.Request.MultipartForm.File[api.MultipartFieldAttachments]
	if len(fileHeaders) == 0 {
		return nil, nil
	}

	attachments := make([]Attachment, 0, len(fileHeaders))
	for _, fh := range fileHeaders {
		f, err := fh.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open attachment %s: %w", fh.Filename, err)
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read attachment %s: %w", fh.Filename, err)
		}

		attachments = append(attachments, Attachment{
			Name:        fh.Filename,
			ContentType: fh.Header.Get("Content-Type"),
			Content:     base64.StdEncoding.EncodeToString(data),
		})
	}
	return attachments, nil
}
