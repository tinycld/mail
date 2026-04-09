package mail

import (
	"crypto/subtle"
	"io"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func handleInbound(app *pocketbase.PocketBase, provider Provider, re *core.RequestEvent, secret string) error {
	// Validate secret token (constant-time comparison to prevent timing attacks)
	token := re.Request.PathValue("token")
	if secret == "" || subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		return re.UnauthorizedError("Invalid inbound token", nil)
	}

	// Read and parse body (capped at 25MB — Postmark's max message size)
	const maxInboundBodySize = 25 << 20
	body, err := io.ReadAll(io.LimitReader(re.Request.Body, maxInboundBodySize))
	if err != nil {
		return re.BadRequestError("Failed to read request body", err)
	}

	// Verify webhook signature (provider-specific)
	headers := make(map[string]string)
	for k, v := range re.Request.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	if err := provider.VerifyWebhookSignature(headers, body); err != nil {
		return re.UnauthorizedError("Webhook signature verification failed", err)
	}

	msg, err := provider.ParseInbound(body)
	if err != nil {
		return re.BadRequestError("Failed to parse inbound message", err)
	}

	// Route to mailboxes: check all To and Cc addresses
	allRecipients := make([]Recipient, 0, len(msg.To)+len(msg.Cc))
	allRecipients = append(allRecipients, msg.To...)
	allRecipients = append(allRecipients, msg.Cc...)
	matched := false

	for _, rcpt := range allRecipients {
		localPart, domain := splitAddress(rcpt.Email)
		if localPart == "" || domain == "" {
			continue
		}

		// Handle plus-addressing: strip +tag before lookup
		localPart = stripPlusTag(localPart)

		mailbox, err := resolveMailboxByAddress(app, localPart, domain)
		if err != nil {
			continue // not our mailbox
		}

		if err := processInboundForMailbox(app, mailbox, msg); err != nil {
			continue
		}
		matched = true
	}

	if !matched {
		// Return 200 anyway — Postmark expects it, and we don't want retries for unknown recipients
		return re.JSON(http.StatusOK, map[string]string{"status": "no matching mailbox"})
	}

	// Postmark requires empty 200 response
	return re.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func processInboundForMailbox(app *pocketbase.PocketBase, mailbox *core.Record, msg *InboundMessage) error {
	mailboxID := mailbox.Id

	// Idempotency: skip if message with this message_id already exists in a thread for this mailbox
	if msg.MessageID != "" {
		existing, err := app.FindRecordsByFilter(
			"mail_messages",
			"message_id = {:messageID}",
			"",
			1,
			0,
			map[string]any{"messageID": msg.MessageID},
		)
		if err == nil && len(existing) > 0 {
			threadID := existing[0].GetString("thread")
			thread, err := app.FindRecordById("mail_threads", threadID)
			if err == nil && thread.GetString("mailbox") == mailboxID {
				return nil // already processed
			}
		}
	}

	// Find or create thread
	thread, err := findOrCreateThread(app, mailboxID, msg.Subject, msg.InReplyTo, msg.References)
	if err != nil {
		return err
	}

	stored := &storedMessage{
		MessageID:     msg.MessageID,
		InReplyTo:     msg.InReplyTo,
		SenderName:    msg.From.Name,
		SenderEmail:   msg.From.Email,
		To:            msg.To,
		Cc:            msg.Cc,
		Date:          msg.Date,
		Subject:       msg.Subject,
		HTMLBody:      msg.HTMLBody,
		TextBody:      msg.TextBody,
		StrippedReply: msg.StrippedReply,
		Attachments:   msg.Attachments,
	}

	if _, err := storeMessage(app, thread.Id, stored); err != nil {
		return err
	}

	// Use stripped reply for snippet when available (just the new content, no quoted history)
	snippet := msg.StrippedReply
	if snippet == "" {
		snippet = msg.TextBody
	}
	if snippet == "" {
		snippet = msg.Subject
	}
	if err := updateThreadMetadata(app, thread, msg.From.Name, msg.From.Email, snippet, msg.Date); err != nil {
		return err
	}

	// Create thread_state for each mailbox member
	members, err := getMailboxMembers(app, mailboxID)
	if err != nil {
		return err
	}
	for _, member := range members {
		userOrgID := member.GetString("user_org")
		if err := ensureThreadState(app, thread.Id, userOrgID, "inbox", false); err != nil {
			app.Logger().Error("failed to create thread state",
				"threadID", thread.Id, "userOrgID", userOrgID, "error", err)
		}
	}

	return nil
}

func splitAddress(email string) (localPart, domain string) {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.LastIndex(email, "@")
	if at < 1 || at >= len(email)-1 {
		return "", ""
	}
	return email[:at], email[at+1:]
}

func stripPlusTag(localPart string) string {
	if plus := strings.Index(localPart, "+"); plus >= 0 {
		return localPart[:plus]
	}
	return localPart
}
