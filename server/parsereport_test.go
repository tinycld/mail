package mail

import (
	"strings"
	"testing"
)

func TestParseReport_Structure(t *testing.T) {
	t.Parallel()
	_, r := parseRFC5322(crlf("Content-Type: multipart/mixed; boundary=m\n\n" +
		"--m\nContent-Type: text/html; charset=x-nobody-knows\n\n<p>caf\xe9</p>\n" +
		"--m\nContent-Type: application/pdf; name=\"private name.pdf\"\nContent-Disposition: inline\n\nPDF\n--m--\n"))
	got := r.structure()
	want := `0 multipart/mixed; 1 text/html charset=x-nobody-knows 12B →text+html; 2 application/pdf inline 3B →file`
	if got != want {
		t.Fatalf("structure =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "private") || strings.Contains(got, "caf") {
		t.Fatalf("structure leaks content: %s", got)
	}
	if strings.Contains(r.warningText(), "private") {
		t.Fatalf("warnings leak content: %s", r.warningText())
	}
	if !r.needsLog() {
		t.Fatal("a salvaged charset does not log")
	}
}

func TestParseReport_NeedsLog(t *testing.T) {
	t.Parallel()
	if _, r := parseRFC5322(crlf("Content-Type: text/plain\n\nhi\n")); r.needsLog() {
		t.Fatal("a clean message logs")
	}
	if _, r := parseRFC5322(crlf("Subject: only a subject\n\n")); r.needsLog() {
		t.Fatal("a subject-only message logs; it is valid mail")
	}
}

func TestParseReport_XTextAttachmentDoesNotLog(t *testing.T) {
	t.Parallel()
	_, r := parseRFC5322(crlf("Content-Type: multipart/mixed; boundary=m\n\n" +
		"--m\nContent-Type: text/x-foo\n\nfoo data\n--m--\n"))
	if len(r.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", r.Warnings)
	}
	if r.needsLog() {
		t.Fatal("a text/x-foo attachment with empty bodies logs; it is an attachment under RFC 8621")
	}
}
