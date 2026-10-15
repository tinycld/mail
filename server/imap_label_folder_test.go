package mail

import (
	"fmt"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/pocketbase/pocketbase/core"
)

type labelFolderEnv struct {
	app     core.App
	user    *core.Record
	mailbox *core.Record
	want    int
}

// seedLabelFolder builds one label with more threads than PocketBase's
// 200-expression filter limit, plus threads that must NOT appear in its
// folder: another label of the same user, and the same label assigned by a
// different user.
func seedLabelFolder(t *testing.T) *labelFolderEnv {
	t.Helper()
	app := newRealSchemaApp(t)
	mailbox := seedRealMailbox(t, app)
	user := saveRealRecord(t, app, "users", map[string]any{
		"email": "owner@example.org", "username": "owner", "name": "Owner", "role": "member",
	})
	other := saveRealRecord(t, app, "users", map[string]any{
		"email": "other@example.org", "username": "other", "name": "Other", "role": "member",
	})
	archived := saveRealRecord(t, app, "labels", map[string]any{"name": "Archived", "color": "#000000", "user": user.Id})
	unrelated := saveRealRecord(t, app, "labels", map[string]any{"name": "Unrelated", "color": "#000000", "user": user.Id})

	const labelled = 210
	for i := 0; i < 250; i++ {
		thread := saveRealRecord(t, app, "mail_threads", map[string]any{
			"mailbox": mailbox.Id, "subject": fmt.Sprintf("thread %d", i),
			"latest_date": "2026-10-01 09:00:00.000Z",
		})
		state := saveRealRecord(t, app, "mail_thread_state", map[string]any{
			"thread": thread.Id, "user": user.Id, "folder": "archive", "mailbox": mailbox.Id,
		})
		saveRealRecord(t, app, "mail_messages", map[string]any{
			"thread": thread.Id, "subject": fmt.Sprintf("thread %d", i), "imap_uid": i + 1,
			"date": "2026-10-01 09:00:00.000Z", "sender_email": "a@example.org",
		})
		label, assignedBy := archived.Id, user.Id
		switch {
		case i >= 230:
			assignedBy = other.Id
		case i >= labelled:
			label = unrelated.Id
		}
		saveRealRecord(t, app, "label_assignments", map[string]any{
			"label": label, "record_id": state.Id, "collection": "mail_thread_state", "user": assignedBy,
		})
	}
	return &labelFolderEnv{app: app, user: user, mailbox: mailbox, want: labelled}
}

func TestIMAPLabelFolder_ListsMoreThanFilterLimit(t *testing.T) {
	env := seedLabelFolder(t)
	s := &imapSession{app: env.app, user: env.user}

	msgs, err := s.messagesForFolder("Labels/Archived", env.mailbox.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != env.want {
		t.Fatalf("Labels/Archived holds %d messages, want %d", len(msgs), env.want)
	}
}

func TestIMAPLabelFolder_StatusCountsMoreThanFilterLimit(t *testing.T) {
	env := seedLabelFolder(t)
	s := &imapSession{app: env.app, user: env.user}

	data, err := s.buildStatusData("Labels/Archived", env.mailbox.Id,
		&imap.StatusOptions{NumMessages: true, NumUnseen: true})
	if err != nil {
		t.Fatal(err)
	}
	if *data.NumMessages != uint32(env.want) {
		t.Fatalf("MESSAGES = %d, want %d", *data.NumMessages, env.want)
	}
	if *data.NumUnseen != uint32(env.want) {
		t.Fatalf("UNSEEN = %d, want %d", *data.NumUnseen, env.want)
	}
}

func TestIMAPLabelFolder_MissingLabelIsEmpty(t *testing.T) {
	env := seedLabelFolder(t)
	s := &imapSession{app: env.app, user: env.user}

	msgs, err := s.messagesForFolder("Labels/NoSuchLabel", env.mailbox.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("a missing label's folder holds %d messages, want 0", len(msgs))
	}
}
