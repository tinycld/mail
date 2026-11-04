package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

var sample = "From: \"Real Person\" <real.person@private.example.net>\r\n" +
	"To: Other <other@private.example.net>\r\n" +
	"Subject: my private subject\r\n" +
	"Message-Id: <ABC-123@private.example.net>\r\n" +
	"Received: from mail.private.example.net\r\n" +
	"Mime-Version: 1.0 (Mac OS X Mail 16.0)\r\n" +
	"Content-Type: multipart/mixed; boundary=\"B\"\r\n\r\n" +
	"--B\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
	"Hello Real, caf=C3=A9 tomorrow at 10?\r\n" +
	"--B\r\nContent-Type: application/pdf; name=\"secret plans.pdf\"\r\n" +
	"Content-Disposition: attachment; filename=\"secret plans.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
	strings.Repeat("JVBERi0xLjQKJcfsj6IKNSAwIG9iago8PC9MZW5ndGggNiAwIFIvRmlsdGVyIC9GbGF0ZURlY29kZT4+\r\n", 200) +
	"--B--\r\n"

func TestAnonymize(t *testing.T) {
	out := anonymize([]byte(sample), 256)
	for _, private := range []string{"Real Person", "Hello Real", "private.example.net", "my private subject", "secret plans", "ABC-123", "Received", "tomorrow"} {
		if bytes.Contains(out, []byte(private)) {
			t.Errorf("output still holds %q:\n%s", private, out)
		}
	}
	for _, kept := range []string{"boundary=\"B\"", "--B\r\n", "--B--", "Mac OS X Mail 16.0", "charset=utf-8", "quoted-printable", "=C3=A9", "Content-Disposition: attachment", ".pdf\""} {
		if !bytes.Contains(out, []byte(kept)) {
			t.Errorf("output lost %q:\n%s", kept, out)
		}
	}
	if len(out) > 2000 {
		t.Errorf("attachment not shortened: %d bytes", len(out))
	}
	if !bytes.Contains(out, []byte(`name="file1.pdf"`)) || !bytes.Contains(out, []byte(`filename="file1.pdf"`)) {
		t.Errorf("one file got two names:\n%s", out)
	}
	if n := bytes.Count(out, []byte(base64.StdEncoding.EncodeToString(placeholder(256))[:76])); n != 1 {
		t.Errorf("placeholder written %d times, want once", n)
	}
}

const headerLeaks = "From: =?utf-8?q?J=C3=B6rg_Private?= <joerg@private.example.net>\r\n" +
	"To: A One <a@private.example.net>,\r\n Bob Smith <b@private.example.net>,\r\n \"Trailing, Name\"\r\n" +
	"Reply-To: Reply Name <reply@private.example.net>\r\n" +
	"Resent-From: Resent Name <resent@private.example.net>\r\n" +
	"Mail-Followup-To: Followup Name <follow@private.example.net>\r\n" +
	"Content-Description: private description\r\n" +
	"Comments: private comment\r\n" +
	"Keywords: private keyword\r\n" +
	"X-Unknown-Thing: secret-value\r\n" +
	"Message-Id: <ABC123>\r\n" +
	"In-Reply-To: <parent-id>\r\n" +
	"Mime-Version: 1.0\r\n" +
	"Content-Type: text/plain;\r\n charset=utf-8\r\n\r\nbody\r\n"

func TestAnonymizeHeaders(t *testing.T) {
	out := anonymize([]byte(headerLeaks), 256)
	for _, private := range []string{"Private", "J=C3", "Bob Smith", "A One", "Trailing", "Reply Name", "Resent Name", "Followup Name", "private", "secret-value", "Comments", "Keywords", "ABC123", "parent-id", "Content-Description", "X-Unknown"} {
		if bytes.Contains(out, []byte(private)) {
			t.Errorf("output still holds %q:\n%s", private, out)
		}
	}
	for _, kept := range []string{"\r\nReply-To: Person ", "\r\nResent-From: Person ", "\r\nMail-Followup-To: Person ", "Message-Id: <id", "In-Reply-To: <id", "Mime-Version: 1.0", "charset=utf-8"} {
		if !bytes.Contains(out, []byte(kept)) {
			t.Errorf("output lost %q:\n%s", kept, out)
		}
	}
	if n := strings.Count(string(out), "@example.com"); n < 6 {
		t.Errorf("want every address replaced, got %d replaced addresses:\n%s", n, out)
	}
}
