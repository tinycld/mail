package mail

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"tinycld.org/packages/mail/api"
)

func handleDraft(app core.App, re *core.RequestEvent) error {
	userID := re.Auth.Id

	var req api.SaveDraftRequest
	var uploadedFiles []*multipart.FileHeader

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
		if re.Request.MultipartForm != nil {
			uploadedFiles = re.Request.MultipartForm.File[api.MultipartFieldAttachments]
		}
	} else if strings.HasPrefix(contentType, "application/json") || contentType == "" {
		if err := json.NewDecoder(re.Request.Body).Decode(&req); err != nil {
			return re.BadRequestError("Invalid request body", err)
		}
	} else {
		return re.BadRequestError("Unsupported Content-Type: "+contentType, nil)
	}

	if req.MailboxID == "" {
		return re.BadRequestError("mailbox_id is required", nil)
	}

	// Load mailbox and domain
	mailbox, err := app.FindRecordById("mail_mailboxes", req.MailboxID)
	if err != nil {
		return re.NotFoundError("Mailbox not found", err)
	}

	if _, err := verifyMailboxMembership(app, req.MailboxID, userID); err != nil {
		return re.ForbiddenError("Not a member of this mailbox", err)
	}

	// An old client's draft update names no address; keep the one a newer
	// client chose rather than re-resolving the default over it.
	fromAddressID := req.FromAddressID
	if req.MessageID != "" && fromAddressID == "" && req.AliasID == "" {
		if existing, err := app.FindRecordById("mail_messages", req.MessageID); err == nil {
			if kept, err := loadResolvedAddress(app, existing.GetString("sent_from")); err == nil && kept.Mailbox.Id == mailbox.Id {
				fromAddressID = kept.Address.Id
			}
		}
	}
	from, err := resolveSendAddress(app, mailbox, fromAddressID, req.AliasID)
	if err != nil {
		return sendErrorForAddress(err).asAPIError(re)
	}

	displayName := mailbox.GetString("display_name")

	subject := req.Subject
	if subject == "" {
		subject = noSubject
	}

	senderEmail := from.Email()
	now := time.Now().UTC().Format(time.RFC3339)

	// Update existing draft in-place
	if req.MessageID != "" {
		record, err := app.FindRecordById("mail_messages", req.MessageID)
		if err != nil {
			return re.NotFoundError("Draft message not found", err)
		}

		thread, err := app.FindRecordById("mail_threads", record.GetString("thread"))
		if err != nil {
			return re.InternalServerError("Failed to load thread", err)
		}
		if thread.GetString("mailbox") != req.MailboxID {
			return re.ForbiddenError("Draft does not belong to this mailbox", nil)
		}

		if err := updateDraftRecord(app, record, req, subject, senderEmail, from.Address.Id, now, uploadedFiles); err != nil {
			return re.InternalServerError("Failed to update draft", err)
		}
		// The draft save above runs the message hooks, which can save the
		// thread (has_draft, has_attachments). Change a fresh copy so this save
		// neither writes nor broadcasts the values from before them.
		fresh, err := app.FindRecordById("mail_threads", thread.Id)
		if err != nil {
			return re.InternalServerError("Failed to reload thread", err)
		}
		fresh.Set("snippet", truncateSnippet(req.TextBody, 200))
		fresh.Set("subject", subject)
		fresh.Set("latest_date", now)
		if err := app.Save(fresh); err != nil {
			return re.InternalServerError("Failed to update thread", err)
		}
		if err := addAddressKeys(app, thread.Id, from.Address.Id); err != nil {
			return re.InternalServerError("Failed to record the thread's addresses", err)
		}

		return re.JSON(http.StatusOK, api.SendEmailResponse{
			MessageID: record.Id,
			ThreadID:  thread.Id,
		})
	}

	// Create new draft
	thread, err := findOrCreateThread(app, req.MailboxID, subject, "", "")
	if err != nil {
		return re.InternalServerError("Failed to create thread", err)
	}

	msg := &storedMessage{
		MessageID:      fmt.Sprintf("draft-%s-%d", userID, time.Now().UnixNano()),
		Alias:          req.AliasID,
		SentFrom:       from.Address.Id,
		SenderName:     displayName,
		SenderEmail:    senderEmail,
		To:             toMailerRecipients(req.To),
		Cc:             toMailerRecipients(req.Cc),
		Bcc:            toMailerRecipients(req.Bcc),
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

	if err := addFileAttachmentsToRecord(app, record, uploadedFiles); err != nil {
		return re.InternalServerError("Failed to store attachments", err)
	}

	if err := updateThreadMetadata(app, thread.Id, displayName, senderEmail, msg.TextBody, now); err != nil {
		return re.InternalServerError("Failed to update thread", err)
	}

	if err := ensureThreadState(app, thread.Id, userID, "drafts", true); err != nil {
		return re.InternalServerError("Failed to create thread state", err)
	}

	return re.JSON(http.StatusOK, api.SendEmailResponse{
		MessageID: record.Id,
		ThreadID:  thread.Id,
	})
}

func updateDraftRecord(app core.App, record *core.Record, req api.SaveDraftRequest, subject, senderEmail, sentFromID, date string, uploadedFiles []*multipart.FileHeader) error {
	record.Set("subject", subject)
	record.Set("date", date)
	record.Set("sender_email", senderEmail)
	record.Set("alias", req.AliasID)
	record.Set("sent_from", sentFromID)

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

	bccJSON, err := json.Marshal(req.Bcc)
	if err != nil {
		return fmt.Errorf("failed to marshal recipients_bcc: %w", err)
	}
	record.Set("recipients_bcc", string(bccJSON))

	if req.HTMLBody != "" {
		sanitized := sanitizeEmailHTML(req.HTMLBody)
		htmlFile, err := filesystem.NewFileFromBytes([]byte(sanitized), "body.html")
		if err != nil {
			return fmt.Errorf("failed to create body_html file: %w", err)
		}
		record.Set("body_html", htmlFile)
	}

	if err := appendFileAttachments(record, uploadedFiles); err != nil {
		return err
	}

	record.Set("has_attachments", len(record.GetStringSlice("attachments")) > 0 || len(uploadedFiles) > 0)

	return app.Save(record)
}

func appendFileAttachments(record *core.Record, fileHeaders []*multipart.FileHeader) error {
	for _, fh := range fileHeaders {
		f, err := fh.Open()
		if err != nil {
			return fmt.Errorf("failed to open attachment %s: %w", fh.Filename, err)
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("failed to read attachment %s: %w", fh.Filename, err)
		}
		file, err := filesystem.NewFileFromBytes(data, fh.Filename)
		if err != nil {
			return fmt.Errorf("failed to create attachment file %s: %w", fh.Filename, err)
		}
		record.Set("attachments+", file)
	}
	return nil
}

// addFileAttachmentsToRecord adds uploaded files directly to a PocketBase record's
// attachments field and saves it.
func addFileAttachmentsToRecord(app core.App, record *core.Record, fileHeaders []*multipart.FileHeader) error {
	if len(fileHeaders) == 0 {
		return nil
	}
	if err := appendFileAttachments(record, fileHeaders); err != nil {
		return err
	}
	record.Set("has_attachments", true)
	return app.Save(record)
}
