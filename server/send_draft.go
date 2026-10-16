package mail

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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

// errDraftGone marks a send whose draft no longer exists: another member
// deleted it, or sent it, while it was open.
var errDraftGone = errors.New("mail: the draft no longer exists")

// draftGoneMessage is what a sender sees when the draft is gone but compose
// still showed its saved files: those files went with the draft.
const draftGoneMessage = "This draft was deleted, so its saved attachments are gone. Attach the files again to send."

// loadSendDraft finds the draft a send resumes. It must be a draft in the
// mailbox the send uses: the caller's right to send from that mailbox is what
// lets them read and delete the draft, so a draft from any other mailbox is
// refused. A draft that no longer exists returns errDraftGone.
func loadSendDraft(app core.App, draftID, mailboxID string) (*core.Record, error) {
	draft, err := app.FindRecordById("mail_messages", draftID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errDraftGone
	}
	if err != nil {
		return nil, &sendError{kind: sendErrInternal, msg: "Failed to load the draft", err: err}
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

// resolveSendDraft loads the draft a send resumes and reads the saved files
// the sender kept. A draft that is gone while compose showed no saved files
// is not an error: the send goes out as a new message, so the person can send
// what they wrote. With saved files shown it is refused, because those files
// are gone. A nil draft means there is none to delete after the send.
func resolveSendDraft(app core.App, draftID, mailboxID string, kept []string) (*core.Record, []Attachment, error) {
	draft, err := loadSendDraft(app, draftID, mailboxID)
	if errors.Is(err, errDraftGone) {
		if len(kept) > 0 {
			return nil, nil, &sendError{kind: sendErrNotFound, msg: draftGoneMessage, err: err}
		}
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	saved, err := draftAttachments(app, draft, kept)
	if err != nil {
		return nil, nil, err
	}
	return draft, saved, nil
}

// draftAttachments reads the saved files of the draft that the sender kept
// (the ones compose showed), as provider attachments. A kept file that is no
// longer on the draft, or cannot be read, fails the send and names the file:
// sending without it would drop it silently, and the draft is deleted after a
// send, so it would be gone.
func draftAttachments(app core.App, draft *core.Record, kept []string) ([]Attachment, error) {
	if len(kept) == 0 {
		return nil, nil
	}
	onDraft := draft.GetStringSlice("attachments")
	fsys, err := app.NewFilesystem()
	if err != nil {
		return nil, &sendError{kind: sendErrInternal, msg: "Could not open file storage", err: err}
	}
	defer fsys.Close()

	out := make([]Attachment, 0, len(kept))
	for _, stored := range kept {
		name := originalAttachmentName(stored)
		if !slices.Contains(onDraft, stored) {
			return nil, &sendError{
				kind: sendErrBadRequest,
				msg:  fmt.Sprintf("The attachment %q is no longer on the draft. Attach it again to send.", name),
			}
		}
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

// maxMessageAttachmentBytes is the most attachment data one message may carry: the
// provider's message size limit, and the client's MAX_TOTAL_SIZE
// (hooks/useAttachments.ts).
const maxMessageAttachmentBytes = 10 * 1024 * 1024

// checkAttachmentLimits refuses a message whose attachments the Sent copy
// cannot hold or the provider will not take. It runs before the provider
// call: a send that passed it and then failed to store would leave the mail
// sent with no Sent copy, and a resend would deliver it twice.
//
// The count limit is the attachments field's own maxSelect, read from the
// schema so the two cannot disagree.
func checkAttachmentLimits(app core.App, attachments []Attachment) error {
	collection, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		return &sendError{kind: sendErrInternal, msg: "mail_messages collection not found", err: err}
	}
	field, ok := collection.Fields.GetByName("attachments").(*core.FileField)
	if !ok {
		return &sendError{kind: sendErrInternal, msg: "mail_messages has no attachments file field"}
	}
	if len(attachments) > field.MaxSelect {
		return &sendError{
			kind: sendErrBadRequest,
			msg:  fmt.Sprintf("A message can have at most %d attachments. This one has %d.", field.MaxSelect, len(attachments)),
		}
	}
	total := 0
	for _, att := range attachments {
		total += decodedSize(att.Content)
	}
	if total > maxMessageAttachmentBytes {
		return &sendError{
			kind: sendErrBadRequest,
			msg:  fmt.Sprintf("The attachments total more than %d MB, the limit for one message.", maxMessageAttachmentBytes/(1024*1024)),
		}
	}
	return nil
}

// decodedSize is the byte length of standard base64 content, without
// decoding it.
func decodedSize(content string) int {
	n := len(content) / 4 * 3
	return n - (len(content) - len(strings.TrimRight(content, "=")))
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
