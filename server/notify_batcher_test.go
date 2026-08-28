package mail

import "testing"

// The deep link every mail notification carries. NotificationDrawer pushes a
// stored url at the router verbatim, so an unprefixed value lands the reader on
// +not-found rather than the inbox — and nothing else in this package would
// catch that, since the constant is only ever read into a record field.
//
// Pinned as a literal on purpose: rebuilding it from approutes.Href here would
// assert the helper against itself and pass even if the prefix silently moved.
func TestMailNotificationURL(t *testing.T) {
	if mailNotificationURL != "/a/mail" {
		t.Errorf("mailNotificationURL = %q, want %q", mailNotificationURL, "/a/mail")
	}
}
