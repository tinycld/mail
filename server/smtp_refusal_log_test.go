package mail

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
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
	user.SetPassword("Password123!")
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

	users, _ := app.FindCollectionByNameOrId("users")
	user := core.NewRecord(users)
	user.SetEmail("rcpt@example.org")
	user.SetPassword("Password123!")
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
