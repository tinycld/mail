package mail

import "testing"

// A message with no subject, or one whose subject is only a reply prefix,
// must still get a thread. mail_threads.subject is required, and Apple Mail
// saves an untitled draft through IMAP APPEND before the user types a subject.
func TestFindOrCreateThread_BlankSubjectGetsPlaceholder(t *testing.T) {
	t.Parallel()
	app := newRealSchemaApp(t)
	mailbox := seedRealMailbox(t, app)

	for _, subject := range []string{"", "   ", "Re:", "Fwd: re:"} {
		thread, err := findOrCreateThread(app, mailbox.Id, subject, "", "")
		if err != nil {
			t.Fatalf("subject %q: %v", subject, err)
		}
		if got := thread.GetString("subject"); got != noSubject {
			t.Fatalf("subject %q stored as %q, want %q", subject, got, noSubject)
		}
	}
}

func TestFindOrCreateThread_KeepsNormalizedSubject(t *testing.T) {
	t.Parallel()
	app := newRealSchemaApp(t)
	mailbox := seedRealMailbox(t, app)

	thread, err := findOrCreateThread(app, mailbox.Id, "Re: Quarterly plan", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := thread.GetString("subject"); got != "Quarterly plan" {
		t.Fatalf("subject stored as %q, want %q", got, "Quarterly plan")
	}
}
