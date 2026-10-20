package mail

import (
	"bytes"
	"errors"
	"github.com/emersion/go-smtp"
	"log/slog"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/rlstest"
)

// captureLogs routes the process default logger, which logging.ForPackage
// resolves on each call, into a buffer for the rest of the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestSMTPRefusal_UnknownSenderIsLogged(t *testing.T) {
	app := setupAliasTestApp(t)
	logs := captureLogs(t)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail("sender@example.org")
	user.Set("username", testUsername("sender@example.org"))
	rlstest.SetPassword(user, "Password123!")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	s := &smtpSession{app: app, user: user}
	if err := s.Mail("nobody@unhosted.example", nil); err == nil {
		t.Fatal("MAIL FROM an address with no mailbox must be refused")
	}

	out := logs.String()
	for _, want := range []string{
		`"msg":"smtp submission refused"`,
		`"stage":"mail"`,
		`"code":550`,
		`"from":"nobody@unhosted.example"`,
		`"user":"` + user.Id + `"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log is missing %s:\n%s", want, out)
		}
	}
}

func TestSMTPRefusal_AuthFailureLogsUsernameNotPassword(t *testing.T) {
	app := setupAliasTestApp(t)
	logs := captureLogs(t)

	s := &smtpSession{app: app}
	const secret = "not-the-password-xyz"
	if err := s.authenticate("ghost@example.org", secret); err == nil {
		t.Fatal("AUTH for an unknown account must be refused")
	}

	out := logs.String()
	if !strings.Contains(out, `"stage":"auth"`) || !strings.Contains(out, `"username":"ghost@example.org"`) {
		t.Fatalf("auth refusal not logged with its username:\n%s", out)
	}
	if strings.Contains(out, secret) {
		t.Fatalf("the password reached the log:\n%s", out)
	}
}

func TestSMTPRefusal_RcptWithoutMailIsLogged(t *testing.T) {
	app := setupAliasTestApp(t)
	logs := captureLogs(t)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail("rcpt@example.org")
	user.Set("username", testUsername("rcpt@example.org"))
	rlstest.SetPassword(user, "Password123!")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	s := &smtpSession{app: app, user: user}
	if err := s.Rcpt("someone@example.net", nil); err == nil {
		t.Fatal("RCPT before MAIL must be refused")
	}
	if !strings.Contains(logs.String(), `"stage":"rcpt"`) {
		t.Fatalf("rcpt refusal not logged:\n%s", logs.String())
	}
}

func newRefusalUser(t *testing.T, app core.App) *core.Record {
	t.Helper()
	return saveRealRecord(t, app, "users", map[string]any{
		"email": "lvl@example.org", "username": "lvl", "name": "Lvl", "role": "member",
	})
}

func TestSMTPRefusal_ClientCausedLogsInfo(t *testing.T) {
	app := setupAliasTestApp(t)
	logs := captureLogs(t)

	s := &smtpSession{app: app}
	if err := s.authenticate("ghost@example.org", "wrong"); err == nil {
		t.Fatal("AUTH for an unknown account must be refused")
	}
	if out := logs.String(); !strings.Contains(out, `"level":"INFO"`) || strings.Contains(out, `"level":"WARN"`) {
		t.Fatalf("a bad login is the client's mistake and must log at INFO:\n%s", out)
	}
}

func TestSMTPRefusal_SendGateLogsWarn(t *testing.T) {
	app := newAddressTestApp(t)
	mailbox := seedRealMailbox(t, app)
	domain, err := app.FindRecordById("mail_domains", mailbox.GetString("domain"))
	if err != nil {
		t.Fatal(err)
	}
	address, err := findAddressRow(app, mailbox.GetString("address"), domain.Id)
	if err != nil {
		t.Fatal(err)
	}
	user := newRefusalUser(t, app)
	logs := captureLogs(t)

	// The domain has no verified DNS records, so the send gate refuses.
	s := &smtpSession{
		app: app, user: user, from: "owner@example.org",
		mailbox: mailbox, address: address, domain: domain, recipients: []string{"to@example.net"},
	}
	raw := "From: owner@example.org\r\nTo: to@example.net\r\nSubject: hi\r\nMessage-ID: <gate@example.org>\r\n\r\nbody\r\n"
	if err := s.Data(strings.NewReader(raw)); err == nil {
		t.Fatal("an unverified sending domain must be refused")
	}
	out := logs.String()
	if !strings.Contains(out, `"gate"`) {
		t.Fatalf("send-gate refusal not logged:\n%s", out)
	}
	if !strings.Contains(out, `"level":"WARN","msg":"smtp submission refused"`) {
		t.Fatalf("a send-gate refusal is server policy and must log at WARN:\n%s", out)
	}
}

func TestSMTPRefusal_ServerSideHelperLogsWarn(t *testing.T) {
	app := setupAliasTestApp(t)
	logs := captureLogs(t)

	s := &smtpSession{app: app}
	_ = s.refuseServerSide("data", &smtp.SMTPError{Code: 451, Message: "Temporary delivery failure"}, errors.New("boom"))
	if !strings.Contains(logs.String(), `"level":"WARN"`) {
		t.Fatalf("server-side refusal must log at WARN:\n%s", logs.String())
	}
}

func TestSMTPRefusal_SecondMailResetsTransaction(t *testing.T) {
	app := newAddressTestApp(t)
	mailbox := seedRealMailbox(t, app)
	user := newRefusalUser(t, app)
	saveRealRecord(t, app, "mail_mailbox_members", map[string]any{
		"mailbox": mailbox.Id, "user": user.Id, "role": "owner",
	})
	logs := captureLogs(t)

	s := &smtpSession{app: app, user: user}
	if err := s.Mail("owner@example.org", nil); err != nil {
		t.Fatalf("first MAIL: %v", err)
	}
	if err := s.Mail("nobody@unhosted.example", nil); err == nil {
		t.Fatal("second MAIL from an unknown sender must be refused")
	}
	err := s.Rcpt("to@example.net", nil)
	if err == nil || !strings.Contains(err.Error(), "Bad sequence of commands") {
		t.Fatalf("RCPT after a refused MAIL must be a bad sequence, got %v", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if !strings.Contains(line, `"from":"nobody@unhosted.example"`) {
			continue
		}
		if n := strings.Count(line, `"from"`); n != 1 {
			t.Fatalf("want exactly one from key, got %d:\n%s", n, line)
		}
		return
	}
	t.Fatalf("second MAIL refusal not logged:\n%s", logs.String())
}
