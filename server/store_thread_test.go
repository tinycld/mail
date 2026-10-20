package mail

import "testing"

func TestFindOrCreateThread_PlaceholderSubjectNeverMergesBySubject(t *testing.T) {
	t.Parallel()
	app := newRealSchemaApp(t)
	mailbox := seedRealMailbox(t, app)

	first, err := findOrCreateThread(app, mailbox.Id, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := findOrCreateThread(app, mailbox.Id, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Id == second.Id {
		t.Fatal("two blank-subject messages are unrelated and must not share a thread")
	}
	fwd, err := findOrCreateThread(app, mailbox.Id, "Fwd: (no subject)", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if fwd.Id == first.Id || fwd.Id == second.Id {
		t.Fatal("a forwarded placeholder subject must not join an unrelated thread")
	}
}
