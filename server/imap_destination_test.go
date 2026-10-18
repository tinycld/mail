package mail

import (
	"errors"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/pocketbase/pocketbase/core"
)

// COPY, MOVE and APPEND name a destination folder. A destination that does
// not exist is refused with NO [TRYCREATE] (RFC 3501 6.4.7), a folder whose
// content the server decides with NO [CANNOT], and the rest file the thread
// without changing its read state.

type destinationEnv struct {
	*addressFolderEnv
	thread *core.Record
	label  *core.Record
}

// seedDestination: one read inbox thread with one message (UID 1), and the
// personal label "Work".
func seedDestination(t *testing.T) *destinationEnv {
	t.Helper()
	app := newRealSchemaApp(t)
	mailbox := seedRealMailbox(t, app)
	user := saveRealRecord(t, app, "users", map[string]any{
		"email": "owner@example.org", "username": "owner", "name": "Owner", "role": "member",
	})
	label := saveRealRecord(t, app, "labels", map[string]any{"name": "Work", "color": "#000000", "user": user.Id})
	thread := saveRealRecord(t, app, "mail_threads", map[string]any{
		"mailbox": mailbox.Id, "subject": "plan", "latest_date": "2026-10-01 09:00:00.000Z",
	})
	saveRealRecord(t, app, "mail_thread_state", map[string]any{
		"thread": thread.Id, "user": user.Id, "folder": "inbox", "mailbox": mailbox.Id, "is_read": true,
	})
	saveRealRecord(t, app, "mail_messages", map[string]any{
		"thread": thread.Id, "subject": "plan", "imap_uid": 1,
		"date": "2026-10-01 09:00:00.000Z", "sender_email": "a@example.org",
	})
	return &destinationEnv{
		addressFolderEnv: &addressFolderEnv{app: app, user: user, mailbox: mailbox},
		thread:           thread,
		label:            label,
	}
}

func (env *destinationEnv) state(t *testing.T, threadID string) *core.Record {
	t.Helper()
	state := findThreadState(env.app, threadID, env.user.Id)
	if state == nil {
		t.Fatalf("no state row for thread %s", threadID)
	}
	return state
}

func (env *destinationEnv) labelled(t *testing.T, threadID string) bool {
	t.Helper()
	rows, err := env.app.FindRecordsByFilter("label_assignments",
		`label = {:label} && record_id = {:state} && collection = "mail_thread_state"`, "", 0, 0,
		map[string]any{"label": env.label.Id, "state": env.state(t, threadID).Id})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows) > 0
}

type destinationOutcome int

const (
	filed destinationOutcome = iota
	tryCreate
	cannot
)

type destinationCase struct {
	dest string
	want destinationOutcome
	// check runs on the state of the thread the command filed.
	check func(t *testing.T, env *destinationEnv, threadID string)
}

func wantFolder(folder string, read bool) func(*testing.T, *destinationEnv, string) {
	return func(t *testing.T, env *destinationEnv, threadID string) {
		state := env.state(t, threadID)
		if got := state.GetString("folder"); got != folder {
			t.Errorf("folder = %q, want %q", got, folder)
		}
		if got := state.GetBool("is_read"); got != read {
			t.Errorf("is_read = %v, want %v", got, read)
		}
	}
}

func wantStarredIn(folder string) func(*testing.T, *destinationEnv, string) {
	return func(t *testing.T, env *destinationEnv, threadID string) {
		state := env.state(t, threadID)
		if !state.GetBool("is_starred") {
			t.Error("is_starred = false, want true")
		}
		if got := state.GetString("folder"); got != folder {
			t.Errorf("folder = %q, want %q (a star does not move the thread)", got, folder)
		}
	}
}

func wantLabelledIn(folder string) func(*testing.T, *destinationEnv, string) {
	return func(t *testing.T, env *destinationEnv, threadID string) {
		if !env.labelled(t, threadID) {
			t.Error("the thread has no Work label")
		}
		if got := env.state(t, threadID).GetString("folder"); got != folder {
			t.Errorf("folder = %q, want %q", got, folder)
		}
	}
}

func wantDestinationOutcome(t *testing.T, err error, want destinationOutcome) {
	t.Helper()
	var imapErr *imap.Error
	switch want {
	case filed:
		if err != nil {
			t.Fatalf("got %v, want OK", err)
		}
	case tryCreate:
		if !errors.As(err, &imapErr) || imapErr.Code != imap.ResponseCodeTryCreate {
			t.Fatalf("got %v, want NO [TRYCREATE]", err)
		}
	case cannot:
		if !errors.As(err, &imapErr) || imapErr.Code != imap.ResponseCodeCannot {
			t.Fatalf("got %v, want NO [CANNOT]", err)
		}
	}
}

// copyMoveCases are the destinations of COPY and MOVE of the read inbox thread.
var copyMoveCases = []destinationCase{
	{dest: "Nowhere", want: tryCreate},
	{dest: "Labels/Missing", want: tryCreate},
	{dest: "Archive", want: filed, check: wantFolder("archive", true)},
	{dest: "aRCHIVE", want: filed, check: wantFolder("archive", true)},
	{dest: "Trash", want: filed, check: wantFolder("trash", true)},
	{dest: "Labels/Work", want: filed, check: wantLabelledIn("inbox")},
	{dest: "labels/Work", want: filed, check: wantLabelledIn("inbox")},
	{dest: "Starred", want: filed, check: wantStarredIn("inbox")},
	{dest: "All Mail", want: cannot},
	{dest: "Labels", want: cannot},
	{dest: "Addresses", want: cannot},
	{dest: "addresses/owner@example.org", want: cannot},
}

func selectInbox(t *testing.T, client *imapclient.Client) {
	t.Helper()
	if _, err := client.Select("INBOX", nil).Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestIMAPCopy_Destinations(t *testing.T) {
	for _, tc := range copyMoveCases {
		t.Run(tc.dest, func(t *testing.T) {
			env := seedDestination(t)
			client := dialAddressFolder(t, env.addressFolderEnv)
			selectInbox(t, client)

			data, err := client.Copy(imap.SeqSetNum(1), tc.dest).Wait()
			wantDestinationOutcome(t, err, tc.want)
			if tc.want != filed {
				wantFolder("inbox", true)(t, env, env.thread.Id)
				if env.labelled(t, env.thread.Id) {
					t.Error("a refused COPY labelled the thread")
				}
				return
			}
			if uids, ok := data.DestUIDs.Nums(); !ok || len(uids) != 1 || uids[0] != 1 {
				t.Errorf("COPYUID destination = %v, want UID 1", data.DestUIDs)
			}
			tc.check(t, env, env.thread.Id)
		})
	}
}

func TestIMAPMove_Destinations(t *testing.T) {
	for _, tc := range copyMoveCases {
		t.Run(tc.dest, func(t *testing.T) {
			env := seedDestination(t)
			client := dialAddressFolder(t, env.addressFolderEnv)
			selectInbox(t, client)

			_, err := client.Move(imap.SeqSetNum(1), tc.dest).Wait()
			wantDestinationOutcome(t, err, tc.want)
			if tc.want != filed {
				wantFolder("inbox", true)(t, env, env.thread.Id)
				if env.labelled(t, env.thread.Id) {
					t.Error("a refused MOVE labelled the thread")
				}
				return
			}
			tc.check(t, env, env.thread.Id)
		})
	}
}

// appendCases are the destinations of an APPEND of a new message. A new
// message is unread; a label or the star files it in the Inbox.
var appendCases = []destinationCase{
	{dest: "", want: tryCreate},
	{dest: "Nowhere", want: tryCreate},
	{dest: "Labels/Missing", want: tryCreate},
	{dest: "aRCHIVE", want: filed, check: wantFolder("archive", false)},
	{dest: "Labels/Work", want: filed, check: wantLabelledIn("inbox")},
	{dest: "labels/Work", want: filed, check: wantLabelledIn("inbox")},
	{dest: "Starred", want: filed, check: wantStarredIn("inbox")},
	{dest: "All Mail", want: cannot},
	{dest: "Addresses", want: cannot},
}

func TestIMAPAppend_Destinations(t *testing.T) {
	const messageID = "<append-dest@example.org>"
	raw := []byte("From: a@example.org\r\nTo: owner@example.org\r\nSubject: appended\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: " + messageID + "\r\n\r\nbody\r\n")
	for _, tc := range appendCases {
		t.Run(tc.dest, func(t *testing.T) {
			env := seedDestination(t)
			client := dialAddressFolder(t, env.addressFolderEnv)

			cmd := client.Append(tc.dest, int64(len(raw)), nil)
			if _, err := cmd.Write(raw); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Close(); err != nil {
				t.Fatal(err)
			}
			_, err := cmd.Wait()
			wantDestinationOutcome(t, err, tc.want)

			stored, err := env.app.FindRecordsByFilter("mail_messages", "message_id = {:id}", "", 0, 0,
				map[string]any{"id": messageID})
			if err != nil {
				t.Fatal(err)
			}
			if tc.want != filed {
				if len(stored) != 0 {
					t.Fatalf("a refused APPEND stored %d messages", len(stored))
				}
				return
			}
			if len(stored) != 1 {
				t.Fatalf("APPEND stored %d messages, want 1", len(stored))
			}
			tc.check(t, env, stored[0].GetString("thread"))
		})
	}
}

// The thread metadata save is part of the APPEND's transaction: when it
// fails, the message is not stored and the client gets an error.
func TestIMAPAppend_ThreadMetadataFailureRollsBack(t *testing.T) {
	const messageID = "<append-rollback@example.org>"
	raw := []byte("From: a@example.org\r\nTo: owner@example.org\r\nSubject: rollback\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: " + messageID + "\r\n\r\nbody\r\n")
	env := seedDestination(t)
	env.app.OnRecordUpdate("mail_threads").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetInt("message_count") != e.Record.Original().GetInt("message_count") {
			return errors.New("thread save refused by the test")
		}
		return e.Next()
	})
	client := dialAddressFolder(t, env.addressFolderEnv)

	cmd := client.Append("INBOX", int64(len(raw)), nil)
	if _, err := cmd.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := cmd.Wait(); err == nil {
		t.Fatal("APPEND returned OK after the thread save failed")
	}
	stored, err := env.app.FindRecordsByFilter("mail_messages", "message_id = {:id}", "", 0, 0,
		map[string]any{"id": messageID})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("the failed APPEND stored %d messages, want 0", len(stored))
	}
}
