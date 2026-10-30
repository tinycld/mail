package mail

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/readonly"
	"tinycld.org/core/readonly/readonlytest"
)

// While the server is read-only it is waiting to be replaced, and a write it
// makes now is lost if the next build rolls back. IMAP and SMTP submission do
// not pass through the HTTP middleware that refuses writes, so their sessions
// check the mode themselves: a write is refused with a temporary error the
// client retries, and reading keeps working.

func wantIMAPUnavailable(t *testing.T, what string, err error) {
	t.Helper()
	var imapErr *imap.Error
	if !errors.As(err, &imapErr) || imapErr.Type != imap.StatusResponseTypeNo ||
		imapErr.Code != imap.ResponseCodeUnavailable {
		t.Fatalf("%s while read-only: got %v, want NO [UNAVAILABLE]", what, err)
	}
}

func appendRaw(client *imapclient.Client, mailbox string, raw []byte) error {
	cmd := client.Append(mailbox, int64(len(raw)), nil)
	if _, err := cmd.Write(raw); err != nil {
		return err
	}
	if err := cmd.Close(); err != nil {
		return err
	}
	_, err := cmd.Wait()
	return err
}

type imapWriteCase struct {
	name string
	// prepare runs while the server is writable, before the mode is entered.
	prepare func(t *testing.T, env *destinationEnv, client *imapclient.Client)
	run     func(env *destinationEnv, client *imapclient.Client) error
	// written reports whether the command's write reached the database.
	written func(t *testing.T, env *destinationEnv) bool
}

const readOnlyAppendID = "<readonly-append@example.org>"

var readOnlyAppendRaw = []byte("From: a@example.org\r\nTo: owner@example.org\r\nSubject: appended\r\n" +
	"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: " + readOnlyAppendID + "\r\n\r\nbody\r\n")

// seedSharedLabel adds the shared label "Team": IMAP's DELETE and RENAME act
// only on shared labels.
func seedSharedLabel(t *testing.T, env *destinationEnv, _ *imapclient.Client) {
	t.Helper()
	saveRealRecord(t, env.app, "labels", map[string]any{"name": "Team", "color": "#000000"})
}

func labelNamed(t *testing.T, env *destinationEnv, name string) bool {
	t.Helper()
	rows, err := env.app.FindRecordsByFilter("labels", "name = {:name}", "", 0, 0,
		map[string]any{"name": name})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows) > 0
}

var imapWriteCases = []imapWriteCase{
	{
		name: "STORE",
		run: func(_ *destinationEnv, client *imapclient.Client) error {
			return client.Store(imap.SeqSetNum(1), &imap.StoreFlags{
				Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagFlagged}, Silent: true,
			}, nil).Close()
		},
		written: func(t *testing.T, env *destinationEnv) bool {
			return env.state(t, env.thread.Id).GetBool("is_starred")
		},
	},
	{
		name: "COPY",
		run: func(_ *destinationEnv, client *imapclient.Client) error {
			_, err := client.Copy(imap.SeqSetNum(1), "Labels/Work").Wait()
			return err
		},
		written: func(t *testing.T, env *destinationEnv) bool { return env.labelled(t, env.thread.Id) },
	},
	{
		name: "MOVE",
		run: func(_ *destinationEnv, client *imapclient.Client) error {
			_, err := client.Move(imap.SeqSetNum(1), "Archive").Wait()
			return err
		},
		written: func(t *testing.T, env *destinationEnv) bool {
			return env.state(t, env.thread.Id).GetString("folder") == "archive"
		},
	},
	{
		name: "APPEND",
		run: func(_ *destinationEnv, client *imapclient.Client) error {
			return appendRaw(client, "INBOX", readOnlyAppendRaw)
		},
		written: func(t *testing.T, env *destinationEnv) bool {
			rows, err := env.app.FindRecordsByFilter("mail_messages", "message_id = {:id}", "", 0, 0,
				map[string]any{"id": readOnlyAppendID})
			if err != nil {
				t.Fatal(err)
			}
			return len(rows) > 0
		},
	},
	{
		name: "EXPUNGE",
		prepare: func(t *testing.T, _ *destinationEnv, client *imapclient.Client) {
			if err := client.Store(imap.SeqSetNum(1), &imap.StoreFlags{
				Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagDeleted}, Silent: true,
			}, nil).Close(); err != nil {
				t.Fatal(err)
			}
		},
		run: func(_ *destinationEnv, client *imapclient.Client) error {
			return client.Expunge().Close()
		},
		written: func(t *testing.T, env *destinationEnv) bool {
			return env.state(t, env.thread.Id).GetString("folder") == "trash"
		},
	},
	{
		name: "CREATE",
		run: func(_ *destinationEnv, client *imapclient.Client) error {
			return client.Create("Labels/Fresh", nil).Wait()
		},
		written: func(t *testing.T, env *destinationEnv) bool { return labelNamed(t, env, "Fresh") },
	},
	{
		name:    "DELETE",
		prepare: seedSharedLabel,
		run: func(_ *destinationEnv, client *imapclient.Client) error {
			return client.Delete("Labels/Team").Wait()
		},
		written: func(t *testing.T, env *destinationEnv) bool { return !labelNamed(t, env, "Team") },
	},
	{
		name:    "RENAME",
		prepare: seedSharedLabel,
		run: func(_ *destinationEnv, client *imapclient.Client) error {
			return client.Rename("Labels/Team", "Labels/Play", nil).Wait()
		},
		written: func(t *testing.T, env *destinationEnv) bool { return labelNamed(t, env, "Play") },
	},
}

// Every IMAP write is refused with NO [UNAVAILABLE] while read-only, writes
// nothing, and works once the mode is left.
func TestIMAPWritesRefusedWhileReadOnly(t *testing.T) {
	for _, tc := range imapWriteCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(readonly.Leave)
			env := seedDestination(t)
			client := dialAddressFolder(t, env.addressFolderEnv)
			selectInbox(t, client)
			if tc.prepare != nil {
				tc.prepare(t, env, client)
			}
			logs := readonlytest.CaptureLogs(t)

			readonly.Enter()
			wantIMAPUnavailable(t, tc.name, tc.run(env, client))
			if tc.written(t, env) {
				t.Fatalf("%s wrote while the server was read-only", tc.name)
			}
			r, ok := readonlytest.Find(logs(), "imap write refused: the server is read-only")
			if !ok || r.Level != slog.LevelInfo || r.Attrs["command"] != tc.name || r.Attrs["user"] != env.user.Id {
				t.Fatalf("refusal not logged at Info with its command and user: %+v (found %v)", r, ok)
			}

			readonly.Leave()
			if err := tc.run(env, client); err != nil {
				t.Fatalf("%s after the mode was left: %v", tc.name, err)
			}
			if !tc.written(t, env) {
				t.Fatalf("%s did not write after the mode was left", tc.name)
			}
		})
	}
}

// fetchBody runs a non-peek FETCH BODY[] of message 1 and returns the body.
func fetchBody(t *testing.T, client *imapclient.Client) []byte {
	t.Helper()
	section := &imap.FetchItemBodySection{}
	msgs, err := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{section},
	}).Collect()
	if err != nil {
		t.Fatalf("FETCH: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("FETCH returned %d messages, want 1", len(msgs))
	}
	return msgs[0].FindBodySection(section)
}

// Reading keeps working while read-only: a FETCH serves the body but does
// not set \Seen, which is a write. After the mode is left the same FETCH
// marks the thread read.
func TestIMAPFetchWhileReadOnlyServesBodyWithoutSeen(t *testing.T) {
	t.Cleanup(readonly.Leave)
	env := seedDestination(t)
	state := env.state(t, env.thread.Id)
	state.Set("is_read", false)
	if err := env.app.Save(state); err != nil {
		t.Fatal(err)
	}
	client := dialAddressFolder(t, env.addressFolderEnv)
	selectInbox(t, client)
	logs := readonlytest.CaptureLogs(t)

	readonly.Enter()
	if body := fetchBody(t, client); !strings.Contains(string(body), "Subject: plan") {
		t.Fatalf("FETCH while read-only did not serve the message:\n%s", body)
	}
	if env.state(t, env.thread.Id).GetBool("is_read") {
		t.Fatal("FETCH set \\Seen while the server was read-only")
	}
	for _, r := range logs() {
		if strings.Contains(r.Msg, "Seen") {
			t.Fatalf("a skipped \\Seen logged at %v; it must log at Debug only: %+v", r.Level, r)
		}
	}

	readonly.Leave()
	fetchBody(t, client)
	if !env.state(t, env.thread.Id).GetBool("is_read") {
		t.Fatal("FETCH did not set \\Seen after the mode was left")
	}
}

// SELECT and STATUS only read: while read-only they must not create the
// mailbox's UID state row.
func TestIMAPSelectWhileReadOnlyDoesNotWrite(t *testing.T) {
	t.Cleanup(readonly.Leave)
	env := seedDestination(t)
	client := dialAddressFolder(t, env.addressFolderEnv)

	readonly.Enter()
	data, err := client.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatalf("SELECT while read-only: %v", err)
	}
	if data.UIDValidity != 1 || data.UIDNext != 1 {
		t.Fatalf("SELECT of a mailbox with no UID state: UIDVALIDITY %d UIDNEXT %d, want 1 and 1",
			data.UIDValidity, data.UIDNext)
	}
	if _, err := client.Status("Archive", &imap.StatusOptions{UIDValidity: true, UIDNext: true}).Wait(); err != nil {
		t.Fatalf("STATUS while read-only: %v", err)
	}
	if state, err := findMailboxState(env.app, env.mailbox.Id); err != nil || state != nil {
		t.Fatalf("SELECT or STATUS wrote the mailbox's UID state: %v %v", state, err)
	}
}

func wantSMTPReadOnly(t *testing.T, what string, err error) {
	t.Helper()
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) || smtpErr.Code != 451 || smtpErr.EnhancedCode != (smtp.EnhancedCode{4, 3, 2}) {
		t.Fatalf("%s while read-only: got %v, want 451 4.3.2", what, err)
	}
}

const readOnlySubmitRaw = "From: nathan@tinycld.com\r\nTo: friend@example.net\r\nSubject: readonly-submit\r\n" +
	"Message-ID: <readonly-submit@tinycld.com>\r\n\r\nbody\r\n"

// submissionEnv is smtpSendEnv for a real (non-demo) send, so the message
// reaches the fake provider, which runs onSend inside the send.
func submissionEnv(t *testing.T, onSend func()) (*sendAddressEnv, *smtpSession, *[]map[string]any) {
	t.Helper()
	env, s := smtpSendEnv(t)
	s.user.Set("is_demo", false)
	if err := env.app.Save(s.user); err != nil {
		t.Fatal(err)
	}
	return env, s, fakePostmarkWith(t, env.app, onSend)
}

func submit(s *smtpSession) error {
	if err := s.Mail("nathan@tinycld.com", nil); err != nil {
		return err
	}
	if err := s.Rcpt("friend@example.net", nil); err != nil {
		return err
	}
	return s.Data(strings.NewReader(readOnlySubmitRaw))
}

func sentCopies(t *testing.T, app core.App) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_messages", "subject = 'readonly-submit'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

// Submission is refused at MAIL FROM while read-only: nothing is sent and no
// Sent copy is stored, so the send and its copy cannot split. It works once
// the mode is left.
func TestSMTPSubmissionRefusedWhileReadOnly(t *testing.T) {
	t.Cleanup(readonly.Leave)
	env, s, sent := submissionEnv(t, nil)
	logs := readonlytest.CaptureLogs(t)

	readonly.Enter()
	wantSMTPReadOnly(t, "MAIL FROM", submit(s))
	if len(*sent) != 0 || sentCopies(t, env.app) != 0 {
		t.Fatalf("while read-only: %d sends and %d Sent copies, want none", len(*sent), sentCopies(t, env.app))
	}
	r, ok := readonlytest.Find(logs(), "smtp submission refused")
	if !ok || r.Level != slog.LevelInfo || r.Attrs["stage"] != "mail" || r.Attrs["code"] != "451" {
		t.Fatalf("refusal not logged at Info with stage mail: %+v (found %v)", r, ok)
	}

	readonly.Leave()
	if err := submit(s); err != nil {
		t.Fatalf("submission after the mode was left: %v", err)
	}
	if len(*sent) != 1 || sentCopies(t, env.app) != 1 {
		t.Fatalf("after the mode was left: %d sends and %d Sent copies, want 1 and 1", len(*sent), sentCopies(t, env.app))
	}
}

// Read-only can begin after MAIL FROM was accepted: DATA checks again before
// the send.
func TestSMTPSubmissionDataRefusedWhenReadOnlyBeginsAfterMail(t *testing.T) {
	t.Cleanup(readonly.Leave)
	env, s, sent := submissionEnv(t, nil)
	if err := s.Mail("nathan@tinycld.com", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Rcpt("friend@example.net", nil); err != nil {
		t.Fatal(err)
	}

	readonly.Enter()
	wantSMTPReadOnly(t, "DATA", s.Data(strings.NewReader(readOnlySubmitRaw)))
	if len(*sent) != 0 || sentCopies(t, env.app) != 0 {
		t.Fatalf("while read-only: %d sends and %d Sent copies, want none", len(*sent), sentCopies(t, env.app))
	}
}

// Read-only can begin while the provider sends. The message is then
// delivered, so DATA answers 250, but its Sent copy waits for the mode to be
// left rather than being written while read-only.
func TestSMTPSubmissionSentCopyWaitsOutReadOnly(t *testing.T) {
	t.Cleanup(readonly.Leave)
	sending := make(chan struct{})
	env, s, sent := submissionEnv(t, func() {
		readonly.Enter()
		close(sending)
	})
	env.app.OnRecordCreate("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		if readonly.Active() {
			t.Error("the Sent copy was stored while the server was read-only")
		}
		return e.Next()
	})
	created := make(chan struct{}, 1)
	env.app.OnRecordAfterCreateSuccess("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		select {
		case created <- struct{}{}:
		default:
		}
		return e.Next()
	})

	done := make(chan error, 1)
	go func() { done <- submit(s) }()
	<-sending
	// Nothing may be stored while the mode is on; give a wrong save the
	// chance to happen before the mode is left.
	select {
	case <-created:
		t.Fatal("the Sent copy was stored while the server was read-only")
	case err := <-done:
		t.Fatalf("DATA returned (%v) before the mode was left; the Sent copy was dropped", err)
	case <-time.After(200 * time.Millisecond):
	}
	readonly.Leave()

	if err := <-done; err != nil {
		t.Fatalf("DATA of a delivered message: %v", err)
	}
	if len(*sent) != 1 || sentCopies(t, env.app) != 1 {
		t.Fatalf("%d sends and %d Sent copies, want 1 and 1", len(*sent), sentCopies(t, env.app))
	}
}
