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
	t.Parallel()
	if mailNotificationURL != "/a/mail" {
		t.Errorf("mailNotificationURL = %q, want %q", mailNotificationURL, "/a/mail")
	}
}

func TestStillInInbox(t *testing.T) {
	env := setupActionEnv(t, "notify-folder.test", "mb_notify_fold1")
	if err := ensureThreadState(env.app, env.threadID, env.alice, "inbox", false); err != nil {
		t.Fatal(err)
	}
	archived := newTestThread(t, env.app, env.mailboxID, "archived")
	if err := ensureThreadState(env.app, archived.Id, env.alice, "archive", false); err != nil {
		t.Fatal(err)
	}
	got := stillInInbox(env.app, env.alice, []pendingMail{
		{subject: "keep", threadID: env.threadID},
		{subject: "drop", threadID: archived.Id},
		{subject: "no thread"},
	})
	if len(got) != 2 || got[0].subject != "keep" || got[1].subject != "no thread" {
		t.Fatalf("got %+v", got)
	}
}
