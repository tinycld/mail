package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/thumbnails"
)

func TestMimeForAttachment(t *testing.T) {
	cases := []struct {
		filename string
		want     string
	}{
		{"report.pdf", "application/pdf"},
		// PocketBase appends a 10-char random suffix before the extension.
		{"report_abc123XYZ0.pdf", "application/pdf"},
		{"book_x1y2z3a4b5.epub", "application/epub+zip"},
		{"letter.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{"sheet.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"deck.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{"photo.heic", "image/heic"},
		{"photo.heif", "image/heic"},
		// Extension matching is case-insensitive.
		{"SCAN.PDF", "application/pdf"},
		{"IMG_0001.HEIC", "image/heic"},
		// Unknown or missing extensions map to no MIME type.
		{"notes.txt", ""},
		{"archive.zip", ""},
		{"README", ""},
		{"", ""},
		// Dotfiles have no extension per filepath.Ext-on-basename semantics.
		{"noext.", ""},
	}
	for _, tc := range cases {
		if got := mimeForAttachment(tc.filename); got != tc.want {
			t.Errorf("mimeForAttachment(%q) = %q, want %q", tc.filename, got, tc.want)
		}
	}
}

// Legacy binary Office formats are intentionally unsupported after the
// doctaculous migration: mimeForAttachment must not map them to anything
// the core pipeline would accept.
func TestMimeForAttachmentLegacyOfficeRejected(t *testing.T) {
	for _, filename := range []string{"old.doc", "old.xls", "old.ppt", "old_abc123XYZ0.doc"} {
		mime := mimeForAttachment(filename)
		if mime != "" {
			t.Errorf("mimeForAttachment(%q) = %q, want \"\"", filename, mime)
		}
		if thumbnails.CanGenerate(mime) {
			t.Errorf("CanGenerate(mimeForAttachment(%q)) = true, want false", filename)
		}
	}
}

// newMessageRecord builds a mail_messages-shaped record whose original
// snapshot holds `previous` — mimicking a record loaded from the DB right
// before an update — without spinning up a PocketBase app.
func newMessageRecord(t *testing.T, previous []string) *core.Record {
	t.Helper()

	collection := core.NewBaseCollection("mail_messages")
	collection.Fields.Add(&core.FileField{Name: "attachments", MaxSelect: 99})

	record := core.NewRecord(collection)
	record.Id = "msg_under_test_"
	record.Set("attachments", previous)
	if err := record.PostScan(); err != nil {
		t.Fatalf("PostScan: %v", err)
	}
	return record
}

func TestAttachmentsChanged(t *testing.T) {
	cases := []struct {
		name     string
		previous []string
		current  []string
		want     bool
	}{
		{"both empty", nil, nil, false},
		{"unchanged single", []string{"a.pdf"}, []string{"a.pdf"}, false},
		{"unchanged multiple", []string{"a.pdf", "b.docx"}, []string{"a.pdf", "b.docx"}, false},
		{"attachment added", []string{"a.pdf"}, []string{"a.pdf", "b.docx"}, true},
		{"attachment removed", []string{"a.pdf", "b.docx"}, []string{"a.pdf"}, true},
		{"first attachment", nil, []string{"a.pdf"}, true},
		{"all removed", []string{"a.pdf"}, nil, true},
		{"replaced same length", []string{"a.pdf"}, []string{"c.pdf"}, true},
		{"reordered", []string{"a.pdf", "b.docx"}, []string{"b.docx", "a.pdf"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record := newMessageRecord(t, tc.previous)
			record.Set("attachments", tc.current)
			if got := attachmentsChanged(record); got != tc.want {
				t.Errorf("attachmentsChanged() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The hook writes attachment_thumbnails/attachment_thumbnail_map back to the
// record; that update must not read as an attachments change or the hook
// would retrigger itself.
func TestAttachmentsChangedIgnoresOtherFields(t *testing.T) {
	record := newMessageRecord(t, []string{"a.pdf"})
	record.Set("attachment_thumbnail_map", `{"a.pdf":"a_thumb.jpg"}`)
	if attachmentsChanged(record) {
		t.Error("attachmentsChanged() = true after unrelated field update, want false")
	}
}
