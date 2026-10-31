package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"tinycld.org/core/appstate"
	"tinycld.org/core/background"
	"tinycld.org/core/readonly"
	"tinycld.org/core/thumbnails"
)

// thumbnailTimeout bounds the omnidoc render of a single attachment. It
// only covers the document render: the storage-blob read and core's HEIF
// decode are not context-aware, so a hang there is not cut off by this.
const thumbnailTimeout = 60 * time.Second

// thumbnailReadOnlyWait bounds how long finished thumbnails wait for the
// server to leave read-only mode. A server that stays read-only longer is
// being replaced and exits, so the wait would end with the process anyway.
const thumbnailReadOnlyWait = 5 * time.Minute

// attachmentsChanged reports whether the `attachments` field differs from its
// pre-update snapshot. The thumbnail hook uses it to skip work when an unrelated
// field changed (including the thumbnail fields the hook itself writes).
func attachmentsChanged(record *core.Record) bool {
	current := record.GetStringSlice("attachments")
	previous := record.Original().GetStringSlice("attachments")
	if len(current) != len(previous) {
		return true
	}
	for i := range current {
		if current[i] != previous[i] {
			return true
		}
	}
	return false
}

// thumbInFlight prevents two jobs from racing on the same record. The hook
// fires on every create and update, including the update we ourselves trigger to
// store the generated thumbnails — without this guard, two parallel runs against
// the same record could see the same "no thumbs yet" snapshot and double-write.
// Keys are appstate.Scoped(app, recordID): a record id is unique only in its app.
var thumbInFlight sync.Map

// generateAttachmentThumbnails walks every attachment on a mail_messages record
// and produces a JPEG thumbnail for each one whose MIME type the core thumbnail
// pipeline supports. Generated thumbnails are stored on the message's
// attachment_thumbnails file field; the attachment_thumbnail_map JSON field
// records the original→thumbnail filename mapping so clients can look up the
// thumb by attachment name. It runs as a background job; ctx ends when the
// app terminates, and the job then stops before its next storage write.
func generateAttachmentThumbnails(ctx context.Context, app core.App, record *core.Record) {
	if ctx.Err() != nil {
		return
	}

	attachments := record.GetStringSlice("attachments")
	if len(attachments) == 0 {
		return
	}

	// Acquire the per-record in-flight lock; bail if another job is already
	// processing this record.
	key := appstate.Scoped(app, record.Id)
	if _, busy := thumbInFlight.LoadOrStore(key, struct{}{}); busy {
		return
	}
	defer thumbInFlight.Delete(key)

	// Don't regenerate for attachments that already have a thumbnail mapping.
	existingMap := readThumbnailMap(record)
	hasNewWork := false
	for _, fname := range attachments {
		if _, ok := existingMap[fname]; !ok {
			if mime := mimeForAttachment(fname); thumbnails.CanGenerate(mime) {
				hasNewWork = true
				break
			}
		}
	}
	if !hasNewWork {
		return
	}

	fsys, err := app.NewFilesystem()
	if err != nil {
		app.Logger().Warn("Mail thumbnail: filesystem open failed", "id", record.Id, "error", err)
		return
	}
	defer fsys.Close()

	// Re-fetch the record fresh; we'll mutate it.
	fresh, err := app.FindRecordById("mail_messages", record.Id)
	if err != nil {
		app.Logger().Warn("Mail thumbnail: record refetch failed", "id", record.Id, "error", err)
		return
	}

	thumbMap := readThumbnailMap(fresh)
	existingThumbs := fresh.GetStringSlice("attachment_thumbnails")
	produced := []*filesystem.File{}

	for _, fname := range attachments {
		if _, ok := thumbMap[fname]; ok {
			continue
		}
		mime := mimeForAttachment(fname)
		if !thumbnails.CanGenerate(mime) {
			continue
		}
		thumbFile, thumbName, err := renderThumbnail(ctx, fsys, record.BaseFilesPath(), fname, mime)
		if err != nil {
			app.Logger().Warn("Mail thumbnail: generation failed",
				"id", record.Id, "attachment", fname, "error", err)
			continue
		}
		thumbMap[fname] = thumbName
		produced = append(produced, thumbFile)
	}

	if len(produced) == 0 {
		return
	}

	// Append produced thumbnail files to existing slice.
	mergedThumbnails := make([]any, 0, len(existingThumbs)+len(produced))
	for _, name := range existingThumbs {
		mergedThumbnails = append(mergedThumbnails, name)
	}
	for _, file := range produced {
		mergedThumbnails = append(mergedThumbnails, file)
	}

	thumbMapBytes, err := json.Marshal(thumbMap)
	if err != nil {
		app.Logger().Warn("Mail thumbnail: thumb map marshal failed", "id", record.Id, "error", err)
		return
	}

	fresh.Set("attachment_thumbnails", mergedThumbnails)
	fresh.Set("attachment_thumbnail_map", string(thumbMapBytes))

	// The renders are slow, so ctx is checked again: a terminating app should
	// not start a new write.
	if ctx.Err() != nil {
		return
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, thumbnailReadOnlyWait)
	defer cancelWait()
	err = saveWhenWritable(waitCtx, app, fresh)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		app.Logger().Warn("Mail thumbnail: save failed", "id", record.Id, "error", err)
		return
	}
	app.Logger().Info("Mail thumbnail: saved",
		"id", record.Id, "added", len(produced), "total", len(thumbMap))
}

// saveWhenWritable saves record once the server is not read-only. A message
// can arrive just before the mode begins, and its background work must not
// write while the mode is on: the write is lost if the next build rolls back.
func saveWhenWritable(ctx context.Context, app core.App, record *core.Record) error {
	if err := readonly.WaitInactive(ctx, app); err != nil {
		return fmt.Errorf("mail: wait for read-only to end: %w", err)
	}
	return app.Save(record)
}

// startAttachmentThumbnails hands a message's thumbnail generation to a
// background job, which the app's terminate waits for.
func startAttachmentThumbnails(jobs *background.Jobs, app core.App, record *core.Record) {
	jobs.Run("mail attachment thumbnails", func(ctx context.Context) {
		generateAttachmentThumbnails(ctx, app, record)
	})
}

func renderThumbnail(
	ctx context.Context,
	fsys *filesystem.System,
	basePath string,
	originalName string,
	mimeType string,
) (*filesystem.File, string, error) {
	key := basePath + "/" + originalName
	blob, err := fsys.GetReader(key)
	if err != nil {
		return nil, "", err
	}
	defer blob.Close()

	renderCtx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()

	var thumb bytes.Buffer
	if err := thumbnails.Generate(renderCtx, &thumb, blob, mimeType, thumbnails.DefaultWidth, thumbnails.DefaultHeight); err != nil {
		return nil, "", err
	}

	thumbName := strings.TrimSuffix(originalName, filepath.Ext(originalName)) + "_thumb.jpg"
	f, err := filesystem.NewFileFromBytes(thumb.Bytes(), thumbName)
	if err != nil {
		return nil, "", err
	}
	return f, f.Name, nil
}

func readThumbnailMap(record *core.Record) map[string]string {
	raw := record.GetString("attachment_thumbnail_map")
	if raw == "" {
		return map[string]string{}
	}
	out := map[string]string{}
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		return map[string]string{}
	}
	return out
}

// mimeForAttachment infers a MIME type from an attachment's filename. PocketBase
// adds a 10-char random suffix to file names; we match against the trailing
// extension so a name like "doc_abc1234567.pdf" still maps to "application/pdf".
func mimeForAttachment(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".epub":
		return "application/epub+zip"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	// Legacy binary Office formats (.doc/.xls/.ppt) are intentionally unsupported.
	case ".heic", ".heif":
		return "image/heic"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	}
	return ""
}
