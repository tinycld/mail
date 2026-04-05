package mail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

type draftRequest struct {
	MailboxID string      `json:"mailbox_id"`
	MessageID string      `json:"message_id"` // existing draft record ID to update
	To        []Recipient `json:"to"`
	Cc        []Recipient `json:"cc"`
	Bcc       []Recipient `json:"bcc"`
	Subject   string      `json:"subject"`
	HTMLBody  string      `json:"html_body"`
	TextBody  string      `json:"text_body"`
}

func handleDraft(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	userID := re.Auth.Id

	var req draftRequest
	if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}

	if req.MailboxID == "" {
		return re.BadRequestError("mailbox_id is required", nil)
	}

	// Load mailbox and domain
	mailbox, err := app.FindRecordById("mail_mailboxes", req.MailboxID)
	if err != nil {
		return re.NotFoundError("Mailbox not found", err)
	}

	domainRecord, err := app.FindRecordById("mail_domains", mailbox.GetString("domain"))
	if err != nil {
		return re.NotFoundError("Domain not found", err)
	}

	orgID := domainRecord.GetString("org")

	// Verify user is a member of this mailbox's org
	userOrg, err := resolveUserOrg(app, userID, orgID)
	if err != nil {
		return re.ForbiddenError("Not a member of this organization", err)
	}

	if _, err := verifyMailboxMembership(app, req.MailboxID, userOrg.Id); err != nil {
		return re.ForbiddenError("Not a member of this mailbox", err)
	}

	displayName := mailbox.GetString("display_name")
	address := mailbox.GetString("address")
	domain := domainRecord.GetString("domain")

	subject := req.Subject
	if subject == "" {
		subject = "(no subject)"
	}

	senderEmail := fmt.Sprintf("%s@%s", address, domain)
	now := time.Now().UTC().Format(time.RFC3339)

	// Update existing draft in-place
	if req.MessageID != "" {
		record, err := app.FindRecordById("mail_messages", req.MessageID)
		if err != nil {
			return re.NotFoundError("Draft message not found", err)
		}
		if err := updateDraftRecord(app, record, req, subject, senderEmail, now); err != nil {
			return re.InternalServerError("Failed to update draft", err)
		}

		thread, err := app.FindRecordById("mail_threads", record.GetString("thread"))
		if err != nil {
			return re.InternalServerError("Failed to load thread", err)
		}
		thread.Set("snippet", truncateSnippet(req.TextBody, 200))
		thread.Set("subject", subject)
		thread.Set("latest_date", now)
		if err := app.Save(thread); err != nil {
			return re.InternalServerError("Failed to update thread", err)
		}

		return re.JSON(http.StatusOK, map[string]string{
			"message_id": record.Id,
			"thread_id":  thread.Id,
		})
	}

	// Create new draft
	thread, err := findOrCreateThread(app, req.MailboxID, subject, "", "")
	if err != nil {
		return re.InternalServerError("Failed to create thread", err)
	}

	msg := &storedMessage{
		MessageID:      fmt.Sprintf("draft-%s-%d", userID, time.Now().UnixNano()),
		SenderName:     displayName,
		SenderEmail:    senderEmail,
		To:             req.To,
		Cc:             req.Cc,
		Date:           now,
		Subject:        subject,
		HTMLBody:       req.HTMLBody,
		TextBody:       req.TextBody,
		DeliveryStatus: "draft",
	}

	record, err := storeMessage(app, thread.Id, msg)
	if err != nil {
		return re.InternalServerError("Failed to store message", err)
	}

	if err := updateThreadMetadata(app, thread, displayName, senderEmail, msg.TextBody, now); err != nil {
		return re.InternalServerError("Failed to update thread", err)
	}

	if err := ensureThreadState(app, thread.Id, userOrg.Id, "drafts", true); err != nil {
		return re.InternalServerError("Failed to create thread state", err)
	}

	return re.JSON(http.StatusOK, map[string]string{
		"message_id": record.Id,
		"thread_id":  thread.Id,
	})
}

func updateDraftRecord(app *pocketbase.PocketBase, record *core.Record, req draftRequest, subject, senderEmail, date string) error {
	record.Set("subject", subject)
	record.Set("date", date)
	record.Set("sender_email", senderEmail)

	snippetSource := req.TextBody
	record.Set("snippet", truncateSnippet(snippetSource, 200))

	toJSON, err := json.Marshal(req.To)
	if err != nil {
		return fmt.Errorf("failed to marshal recipients_to: %w", err)
	}
	record.Set("recipients_to", string(toJSON))

	ccJSON, err := json.Marshal(req.Cc)
	if err != nil {
		return fmt.Errorf("failed to marshal recipients_cc: %w", err)
	}
	record.Set("recipients_cc", string(ccJSON))

	if req.HTMLBody != "" {
		sanitized := sanitizeEmailHTML(req.HTMLBody)
		htmlFile, err := filesystem.NewFileFromBytes([]byte(sanitized), "body.html")
		if err != nil {
			return fmt.Errorf("failed to create body_html file: %w", err)
		}
		record.Set("body_html", htmlFile)
	}

	return app.Save(record)
}
