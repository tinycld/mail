package mail

import (
	"slices"
	"strings"
	"testing"
)

// indexes renders a list as part indexes, for compact assertions.
func indexes(ps []*mimePart) []int {
	out := make([]int, len(ps))
	for i, p := range ps {
		out[i] = p.Index
	}
	return out
}

func structureOf(t *testing.T, raw string) bodyLists {
	t.Helper()
	root, _ := readTree(crlf(raw))
	return parseStructure(root)
}

func TestParseStructure_RFC8621(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name              string
		raw               string
		text, html, files []int
	}{
		{
			name: "single text part",
			raw:  "Content-Type: text/plain\n\nhi\n",
			text: []int{0}, html: []int{0}, files: []int{},
		},
		{
			name: "alternative picks plain for text and html for html",
			raw: "Content-Type: multipart/alternative; boundary=a\n\n" +
				"--a\nContent-Type: text/plain\n\nhi\n--a\nContent-Type: text/html\n\n<p>hi</p>\n--a--\n",
			text: []int{1}, html: []int{2}, files: []int{},
		},
		{
			name: "html only in an alternative fills the text list too",
			raw: "Content-Type: multipart/alternative; boundary=a\n\n" +
				"--a\nContent-Type: text/html\n\n<p>hi</p>\n--a--\n",
			text: []int{1}, html: []int{1}, files: []int{},
		},
		{
			name: "mixed: body then an attachment",
			raw: "Content-Type: multipart/mixed; boundary=m\n\n" +
				"--m\nContent-Type: text/plain\n\nhi\n--m\nContent-Type: application/pdf\nContent-Disposition: attachment; filename=a.pdf\n\nPDF\n--m--\n",
			text: []int{1}, html: []int{1}, files: []int{2},
		},
		{
			name: "html split around an inline PDF inside an alternative",
			raw: "Content-Type: multipart/alternative; boundary=a\n\n" +
				"--a\nContent-Type: text/plain\n\nAttach pdf here\n" +
				"--a\nContent-Type: multipart/mixed; boundary=m\n\n" +
				"--m\nContent-Type: text/html\n\n<p>Attach pdf here</p>\n" +
				"--m\nContent-Type: application/pdf; name=doc.pdf\nContent-Disposition: inline\n\nPDF\n" +
				"--m\nContent-Type: text/html\n\n<p></p>\n--m--\n--a--\n",
			text: []int{1}, html: []int{3, 5}, files: []int{4},
		},
		{
			name: "inline image in mixed is in both bodies and not a file",
			raw: "Content-Type: multipart/mixed; boundary=m\n\n" +
				"--m\nContent-Type: text/plain\n\nlook\n--m\nContent-Type: image/png\n\nPNG\n--m--\n",
			text: []int{1, 2}, html: []int{1, 2}, files: []int{},
		},
		{
			name: "related: only the first part is a body",
			raw: "Content-Type: multipart/related; boundary=r\n\n" +
				"--r\nContent-Type: text/html\n\n<img src=\"cid:x\">\n--r\nContent-Type: image/png\nContent-Id: <x>\n\nPNG\n--r--\n",
			text: []int{1}, html: []int{1}, files: []int{2},
		},
		{
			name: "files only, no text",
			raw: "Content-Type: multipart/mixed; boundary=m\n\n" +
				"--m\nContent-Type: application/octet-stream\nContent-Disposition: attachment; filename=a.bin\n\nBIN\n" +
				"--m\nContent-Type: application/pdf\nContent-Disposition: inline; filename=b.pdf\n\nPDF\n--m--\n",
			text: []int{}, html: []int{}, files: []int{1, 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := structureOf(t, tc.raw)
			if got := indexes(l.text); !slices.Equal(got, tc.text) {
				t.Errorf("text = %v, want %v", got, tc.text)
			}
			if got := indexes(l.html); !slices.Equal(got, tc.html) {
				t.Errorf("html = %v, want %v", got, tc.html)
			}
			if got := indexes(l.attachments); !slices.Equal(got, tc.files) {
				t.Errorf("attachments = %v, want %v", got, tc.files)
			}
		})
	}
}

// RFC 8621 defines attachments as every non-multipart part in neither body
// list, but the algorithm can drop one: inside an alternative, a mixed
// branch whose HTML part stops its text list and whose later plain part then
// stops its HTML list puts that plain part nowhere. It is added back.
func TestParseStructure_NoLeafIsDropped(t *testing.T) {
	t.Parallel()
	l := structureOf(t, "Content-Type: multipart/alternative; boundary=a\n\n"+
		"--a\nContent-Type: text/plain\n\nhi\n"+
		"--a\nContent-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: text/html\n\n<p>hi</p>\n--m\nContent-Type: text/plain\n\nfooter\n--m--\n--a--\n")
	in := map[int]bool{}
	for _, list := range [][]*mimePart{l.text, l.html, l.attachments} {
		for _, p := range list {
			in[p.Index] = true
		}
	}
	for _, i := range []int{1, 3, 4} {
		if !in[i] {
			t.Fatalf("part %d is in no list: text %v html %v files %v", i, indexes(l.text), indexes(l.html), indexes(l.attachments))
		}
	}
}

func flattened(t *testing.T, raw string) *storedMessage {
	t.Helper()
	msg := &storedMessage{}
	flatten(structureOf(t, raw), msg)
	return msg
}

func TestFlatten(t *testing.T) {
	t.Parallel()

	msg := flattened(t, "Content-Type: multipart/alternative; boundary=a\n\n"+
		"--a\nContent-Type: text/plain\n\nAttach pdf here\n"+
		"--a\nContent-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: text/html\n\n<p>Attach pdf here</p>\n"+
		"--m\nContent-Type: application/pdf; name=doc.pdf\nContent-Disposition: inline\n\nPDF\n"+
		"--m\nContent-Type: text/html\n\n<p>after</p>\n--m--\n--a--\n")
	if msg.TextBody != "Attach pdf here" {
		t.Errorf("text = %q", msg.TextBody)
	}
	if !strings.Contains(msg.HTMLBody, "Attach pdf here") || !strings.Contains(msg.HTMLBody, "after") {
		t.Errorf("html = %q", msg.HTMLBody)
	}
	if len(msg.Attachments) != 1 || msg.Attachments[0].Name != "doc.pdf" {
		t.Errorf("attachments = %+v", msg.Attachments)
	}

	// A plain-text message stays plain text.
	msg = flattened(t, "Content-Type: text/plain\n\nhello\n")
	if msg.TextBody != "hello" || msg.HTMLBody != "" {
		t.Errorf("plain: text %q html %q", msg.TextBody, msg.HTMLBody)
	}

	// An image between two HTML parts shows where it was put, and is a file.
	msg = flattened(t, "Content-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: text/html\n\n<p>one</p>\n--m\nContent-Type: image/png\n\nPNG\n"+
		"--m\nContent-Type: text/html\n\n<p>two</p>\n--m--\n")
	one, img, two := strings.Index(msg.HTMLBody, "one"), strings.Index(msg.HTMLBody, `<img src="cid:`), strings.Index(msg.HTMLBody, "two")
	if !(one >= 0 && one < img && img < two) {
		t.Errorf("html order: %q", msg.HTMLBody)
	}
	if len(msg.Attachments) != 1 || msg.Attachments[0].ContentID == "" {
		t.Errorf("image file: %+v", msg.Attachments)
	}

	// Files and no text: both bodies empty, every file kept.
	msg = flattened(t, "Content-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: application/octet-stream\nContent-Disposition: attachment; filename=a.bin\n\nBIN\n"+
		"--m\nContent-Type: application/pdf\nContent-Disposition: inline; filename=b.pdf\n\nPDF\n--m--\n")
	if msg.TextBody != "" || msg.HTMLBody != "" || len(msg.Attachments) != 2 {
		t.Errorf("files only: text %q html %q files %d", msg.TextBody, msg.HTMLBody, len(msg.Attachments))
	}

	// An attached message without a name is named message.eml.
	msg = flattened(t, "Content-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: text/plain\n\nfwd\n--m\nContent-Type: message/rfc822\n\nSubject: inner\n\nhi\n--m--\n")
	if len(msg.Attachments) != 1 || msg.Attachments[0].Name != "message.eml" {
		t.Errorf("attached message: %+v", msg.Attachments)
	}
}
