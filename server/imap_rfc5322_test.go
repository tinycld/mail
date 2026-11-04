package mail

import (
	"strings"
	"testing"
)

const appleMailHeaders = "From: Nathan <nathan@example.org>\r\n" +
	"To: a@example.org\r\n" +
	"Subject: Attach pdf here\r\n" +
	"Mime-Version: 1.0 (Mac OS X Mail 16.0)\r\n"

const applePDFPart = "Content-Disposition: inline; filename=doc.pdf\r\n" +
	"Content-Type: application/pdf; x-unix-mode=0644; name=\"doc.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\n" +
	"JVBERi0xLjQK\r\n"

// Apple Mail's rich-text layout for a PDF placed inside the body: the html is
// split around the PDF, and the part after it holds no content.
const appleMailHTMLWithInlinePDF = appleMailHeaders +
	"Content-Type: multipart/alternative; boundary=\"A\"\r\n\r\n" +
	"--A\r\nContent-Type: text/plain; charset=us-ascii\r\n\r\n" +
	"Attach pdf here\r\n\r\n" +
	"--A\r\nContent-Type: multipart/mixed; boundary=\"B\"\r\n\r\n" +
	"--B\r\nContent-Type: text/html; charset=us-ascii\r\n\r\n" +
	"<html><head></head><body><div>Attach pdf here</div></body></html>\r\n" +
	"--B\r\n" + applePDFPart +
	"--B\r\nContent-Type: text/html; charset=us-ascii\r\n\r\n" +
	"<html><head></head><body></body></html>\r\n" +
	"--B--\r\n\r\n--A--\r\n"

// Apple Mail's plain-text layout for the same message.
const appleMailTextWithInlinePDF = appleMailHeaders +
	"Content-Type: multipart/mixed; boundary=\"B\"\r\n\r\n" +
	"--B\r\nContent-Type: text/plain; charset=us-ascii\r\n\r\n" +
	"Attach pdf here\r\n\r\n" +
	"--B\r\n" + applePDFPart +
	"--B\r\nContent-Type: text/plain; charset=us-ascii\r\n\r\n" +
	"\r\n\r\n" +
	"--B--\r\n"

func TestParseRFC5322_SplitBodyKeepsContentAndInlineFile(t *testing.T) {
	cases := map[string]string{
		"html": appleMailHTMLWithInlinePDF,
		"text": appleMailTextWithInlinePDF,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			msg, _ := parseRFC5322([]byte(raw))
			if !strings.Contains(msg.TextBody, "Attach pdf here") {
				t.Errorf("TextBody = %q, want the body text", msg.TextBody)
			}
			if name == "html" && !strings.Contains(sanitizeEmailHTML(msg.HTMLBody), "Attach pdf here") {
				t.Errorf("HTMLBody = %q, want the body text", msg.HTMLBody)
			}
			if len(msg.Attachments) != 1 {
				t.Fatalf("got %d attachments, want the inline PDF", len(msg.Attachments))
			}
			att := msg.Attachments[0]
			if att.Name != "doc.pdf" || att.ContentType != "application/pdf" {
				t.Errorf("attachment = %q (%s), want doc.pdf (application/pdf)", att.Name, att.ContentType)
			}
		})
	}
}

func TestParseRFC5322_InlineImageKeepsContentID(t *testing.T) {
	raw := appleMailHeaders +
		"Content-Type: multipart/related; boundary=\"R\"\r\n\r\n" +
		"--R\r\nContent-Type: text/html; charset=us-ascii\r\n\r\n" +
		"<p>Logo</p><img src=\"cid:logo@example.org\">\r\n" +
		"--R\r\nContent-Type: image/png\r\n" +
		"Content-Disposition: inline\r\n" +
		"Content-Id: <logo@example.org>\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		"iVBORw0KGgo=\r\n" +
		"--R--\r\n"
	msg, _ := parseRFC5322([]byte(raw))
	if len(msg.Attachments) != 1 {
		t.Fatalf("got %d attachments, want the inline image", len(msg.Attachments))
	}
	att := msg.Attachments[0]
	if normalizeCID(att.ContentID) != "logo@example.org" {
		t.Errorf("ContentID = %q, want logo@example.org", att.ContentID)
	}
	if att.Name != "attachment.png" {
		t.Errorf("Name = %q, want a name from the content type", att.Name)
	}
}

// A message with only a subject is valid: it parses to empty bodies and no
// files.
func TestParseRFC5322_SubjectOnly(t *testing.T) {
	t.Parallel()
	msg, report := parseRFC5322([]byte("From: a@example.org\r\nSubject: lunch at noon?\r\n\r\n"))
	if msg.Subject != "lunch at noon?" || msg.TextBody != "" || msg.HTMLBody != "" || len(msg.Attachments) != 0 {
		t.Fatalf("msg = %+v", msg)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("warnings %+v", report.Warnings)
	}
}

// The fault that ended the old part loop: a part in a charset go-message does
// not know dropped every part after it.
func TestParseRFC5322_UnknownCharsetKeepsEveryPart(t *testing.T) {
	t.Parallel()
	raw := "Content-Type: multipart/alternative; boundary=a\r\n\r\n" +
		"--a\r\nContent-Type: text/plain; charset=x-nobody-knows\r\n\r\nCaf\xe9 au lait\r\n" +
		"--a\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Café au lait</p>\r\n--a--\r\n"
	msg, report := parseRFC5322([]byte(raw))
	if msg.TextBody != "Café au lait" || !strings.Contains(msg.HTMLBody, "Café au lait") {
		t.Fatalf("text %q html %q", msg.TextBody, msg.HTMLBody)
	}
	if !hasWarning(report.Warnings, "unknown-charset") {
		t.Fatalf("warnings %+v", report.Warnings)
	}
}

// Review focus 5: a broken inner message does not break the outer one; it is
// kept as an .eml file, not parsed.
func TestParseRFC5322_BrokenInnerMessageIsAFile(t *testing.T) {
	t.Parallel()
	raw := "Content-Type: multipart/mixed; boundary=m\r\n\r\n" +
		"--m\r\nContent-Type: text/plain\r\n\r\nsee attached\r\n" +
		"--m\r\nContent-Type: message/rfc822\r\n\r\n\x00\x01not a message\r\nContent-Type: multipart/mixed; boundary=\r\n" +
		"--m--\r\n"
	msg, _ := parseRFC5322([]byte(raw))
	if msg.TextBody != "see attached" || len(msg.Attachments) != 1 || msg.Attachments[0].Name != "message.eml" {
		t.Fatalf("text %q files %+v", msg.TextBody, msg.Attachments)
	}
}

// Headers are read as before: addresses, ids and the subject.
func TestParseRFC5322_Headers(t *testing.T) {
	t.Parallel()
	raw := "From: Ann <ann@example.org>\r\nTo: bo@example.org\r\nCc: Cy <cy@example.org>\r\n" +
		"Subject: =?utf-8?q?caf=C3=A9?=\r\nMessage-ID: <m1@example.org>\r\nIn-Reply-To: <m0@example.org>\r\n" +
		"References: <a@example.org> <m0@example.org>\r\n\r\nhi\r\n"
	msg, _ := parseRFC5322([]byte(raw))
	if msg.SenderName != "Ann" || msg.SenderEmail != "ann@example.org" || msg.Subject != "café" ||
		msg.MessageID != "<m1@example.org>" || msg.InReplyTo != "<m0@example.org>" ||
		msg.References != "<a@example.org> <m0@example.org>" || len(msg.To) != 1 || len(msg.Cc) != 1 {
		t.Fatalf("msg = %+v", msg)
	}
}
