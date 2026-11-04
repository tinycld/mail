package mail

import "testing"

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
