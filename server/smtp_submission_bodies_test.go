package mail

import (
	"strings"
	"testing"
)

// A message with only a subject, or a subject and files, is valid mail, but
// Postmark refuses a send with no body at all (ErrorCode 300). It accepts
// white space, so such a message is sent with a single space.
func TestSubmissionBodies(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name               string
		text, html         string
		wantText, wantHTML string
	}{
		{"no body", "", "", " ", ""},
		{"white space only", "\r\n", "", " ", ""},
		{"text", "hello", "", "hello", ""},
		{"html only", "", "<p>hi</p>", "", "<p>hi</p>"},
		{"both", "hi", "<p>hi</p>", "hi", "<p>hi</p>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, html := submissionBodies(&storedMessage{TextBody: tc.text, HTMLBody: tc.html})
			if text != tc.wantText || html != tc.wantHTML {
				t.Fatalf("bodies = %q, %q; want %q, %q", text, html, tc.wantText, tc.wantHTML)
			}
		})
	}
}

// Data sends a subject-only submission with the single-space body, so
// Postmark does not refuse it.
func TestSMTPSubmission_SubjectOnlyIsSent(t *testing.T) {
	env, s := smtpSendEnv(t)
	// A real (non-demo) send, so the message reaches the provider.
	s.user.Set("is_demo", false)
	if err := env.app.Save(s.user); err != nil {
		t.Fatal(err)
	}
	sent := fakePostmark(t, env.app)

	if err := s.Mail("nathan@tinycld.org", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Rcpt("friend@example.net", nil); err != nil {
		t.Fatal(err)
	}
	raw := "From: nathan@tinycld.org\r\nTo: friend@example.net\r\nSubject: only a subject\r\nMessage-ID: <subj1@tinycld.org>\r\n\r\n"
	if err := s.Data(strings.NewReader(raw)); err != nil {
		t.Fatalf("refused: %v", err)
	}
	if len(*sent) != 1 {
		t.Fatalf("provider got %d sends, want 1", len(*sent))
	}
	email := (*sent)[0]
	if text, _ := email["TextBody"].(string); text != " " {
		t.Errorf("TextBody = %q, want a single space", text)
	}
	if html, _ := email["HtmlBody"].(string); html != "" {
		t.Errorf("HtmlBody = %q, want none", html)
	}
}
