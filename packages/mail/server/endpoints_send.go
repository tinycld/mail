package mail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

type sendRequest struct {
	MailboxID          string      `json:"mailbox_id"`
	To                 []Recipient `json:"to"`
	Cc                 []Recipient `json:"cc"`
	Bcc                []Recipient `json:"bcc"`
	Subject            string      `json:"subject"`
	HTMLBody           string      `json:"html_body"`
	TextBody           string      `json:"text_body"`
	InReplyToMessageID string      `json:"in_reply_to_message_id"` // PB record ID of the message being replied to
}

func handleSend(app *pocketbase.PocketBase, provider Provider, re *core.RequestEvent) error {
	userID := re.Auth.Id

	var req sendRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}

	if req.MailboxID == "" {
		return re.BadRequestError("mailbox_id is required", nil)
	}
	if len(req.To) == 0 {
		return re.BadRequestError("at least one recipient is required", nil)
	}

	// Load mailbox and build From address
	mailbox, err := app.FindRecordById("mail_mailboxes", req.MailboxID)
	if err != nil {
		return re.NotFoundError("Mailbox not found", err)
	}

	domainRecord, err := app.FindRecordById("mail_domains", mailbox.GetString("domain"))
	if err != nil {
		return re.NotFoundError("Domain not found", err)
	}

	orgID := domainRecord.GetString("org")

	// Verify user is a member of this mailbox's org and has access
	userOrg, err := resolveUserOrg(app, userID, orgID)
	if err != nil {
		return re.ForbiddenError("Not a member of this organization", err)
	}

	if _, err := verifyMailboxMembership(app, req.MailboxID, userOrg.Id); err != nil {
		return re.ForbiddenError("Not a member of this mailbox", err)
	}

	// Build From address
	displayName := mailbox.GetString("display_name")
	address := mailbox.GetString("address")
	domain := domainRecord.GetString("domain")
	fromAddr := fmt.Sprintf("%s <%s@%s>", displayName, address, domain)

	// Build threading headers if replying
	var inReplyToHeader, referencesHeader string
	if req.InReplyToMessageID != "" {
		originalMsg, err := app.FindRecordById("mail_messages", req.InReplyToMessageID)
		if err == nil {
			inReplyToHeader = originalMsg.GetString("message_id")
			referencesHeader = inReplyToHeader
		}
	}

	// Send via provider
	sendReq := &SendRequest{
		From:       fromAddr,
		To:         req.To,
		Cc:         req.Cc,
		Bcc:        req.Bcc,
		Subject:    req.Subject,
		HTMLBody:   req.HTMLBody,
		TextBody:   req.TextBody,
		InReplyTo:  inReplyToHeader,
		References: referencesHeader,
	}

	result, err := provider.Send(re.Request.Context(), sendReq)
	if err != nil {
		return router.NewApiError(http.StatusBadGateway, "Failed to send email", err)
	}

	// Store in database
	now := time.Now().UTC().Format(time.RFC3339)

	thread, err := findOrCreateThread(app, req.MailboxID, req.Subject, inReplyToHeader, referencesHeader)
	if err != nil {
		return re.InternalServerError("Failed to create thread", err)
	}

	msg := &storedMessage{
		MessageID:   result.MessageID,
		InReplyTo:   inReplyToHeader,
		SenderName:  displayName,
		SenderEmail: fmt.Sprintf("%s@%s", address, domain),
		To:          req.To,
		Cc:          req.Cc,
		Date:        now,
		Subject:     req.Subject,
		HTMLBody:    req.HTMLBody,
		TextBody:    req.TextBody,
	}

	record, err := storeMessage(app, thread.Id, msg)
	if err != nil {
		return re.InternalServerError("Failed to store message", err)
	}

	if err := updateThreadMetadata(app, thread, displayName, msg.SenderEmail, msg.TextBody, now); err != nil {
		return re.InternalServerError("Failed to update thread", err)
	}

	// Create thread state for the sender
	if err := ensureThreadState(app, thread.Id, userOrg.Id, "sent", true); err != nil {
		return re.InternalServerError("Failed to create thread state", err)
	}

	return re.JSON(http.StatusOK, map[string]string{
		"message_id": record.Id,
		"thread_id":  thread.Id,
	})
}
