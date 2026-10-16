package mail

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"tinycld.org/core/logging"
)

var sendDraftLog = logging.ForPackage("mail")

// storedNameSuffix matches the random part PocketBase adds to a stored file
// name ("hippo_ab12cd34ef.jpg"). The client strips it the same way for
// display (cleanFilename in attachment-preview-source.ts).
var storedNameSuffix = regexp.MustCompile(`^(.+)_[a-zA-Z0-9]{10}(\.\w+)$`)

// originalAttachmentName is a stored file name without PocketBase's random
// suffix: the name the sender attached it under. A copied file must carry
// this name, or each copy adds one more suffix and the recipient sees it.
func originalAttachmentName(stored string) string {
	if m := storedNameSuffix.FindStringSubmatch(stored); m != nil {
		return m[1] + m[2]
	}
	return stored
}

// loadSendDraft finds the draft a send resumes. It must be a draft in the
// mailbox the send uses: the caller's right to send from that mailbox is what
// lets them read and delete the draft, so a draft from any other mailbox is
// refused.
func loadSendDraft(app core.App, draftID, mailboxID string) (*core.Record, error) {
	draft, err := app.FindRecordById("mail_messages", draftID)
	if err != nil {
		return nil, &sendError{kind: sendErrNotFound, msg: "Draft not found", err: err}
	}
	if draft.GetString("delivery_status") != "draft" {
		return nil, &sendError{kind: sendErrBadRequest, msg: "This message is not a draft"}
	}
	thread, err := app.FindRecordById("mail_threads", draft.GetString("thread"))
	if err != nil {
		return nil, &sendError{kind: sendErrInternal, msg: "Failed to load the draft's thread", err: err}
	}
	if thread.GetString("mailbox") != mailboxID {
		return nil, &sendError{kind: sendErrForbidden, msg: "Draft does not belong to this mailbox"}
	}
	return draft, nil
}

// draftAttachments reads the draft's saved files, less the ones the sender
// removed, as provider attachments. A file that cannot be read fails the
// send and names the file: sending without it would drop it silently, and
// the draft is deleted after a send, so it would be gone.
func draftAttachments(app core.App, draft *core.Record, removed []string) ([]Attachment, error) {
	names := draft.GetStringSlice("attachments")
	if len(names) == 0 {
		return nil, nil
	}
	fsys, err := app.NewFilesystem()
	if err != nil {
		return nil, &sendError{kind: sendErrInternal, msg: "Could not open file storage", err: err}
	}
	defer fsys.Close()

	out := make([]Attachment, 0, len(names))
	for _, stored := range names {
		if slices.Contains(removed, stored) {
			continue
		}
		name := originalAttachmentName(stored)
		data, err := readStoredFile(fsys, draft.BaseFilesPath()+"/"+stored)
		if err != nil {
			return nil, &sendError{
				kind: sendErrInternal,
				msg:  fmt.Sprintf("Could not read the attachment %q from the draft", name),
				err:  err,
			}
		}
		contentType := mime.TypeByExtension(filepath.Ext(name))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		out = append(out, Attachment{
			Name:        name,
			ContentType: contentType,
			Content:     base64.StdEncoding.EncodeToString(data),
		})
	}
	return out, nil
}

func readStoredFile(fsys *filesystem.System, key string) ([]byte, error) {
	blob, err := fsys.GetReader(key)
	if err != nil {
		return nil, err
	}
	defer blob.Close()
	return io.ReadAll(blob)
}

// deleteSentDraft removes the draft once its send succeeded. The message has
// already gone out, so a failure here must not report the send as failed —
// the sender would send it again. It is logged instead.
func deleteSentDraft(app core.App, draft *core.Record) {
	if err := app.Delete(draft); err != nil {
		sendDraftLog.Warn("deleting a sent draft failed", "draft", draft.Id, "error", err)
	}
}
