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
			msg, err := parseRFC5322([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
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
	msg, err := parseRFC5322([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
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
