package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// username_login_test.go pins that the mail listeners accept the same
// identifiers as every other protocol server: a bare username OR a full email
// (core/davauth's contract, mirroring PocketBase's identityFields). Members
// are invited by username with the email optional, so an email-only login
// locks those accounts out of IMAP/SMTP entirely — and the help topics cannot
// give one true instruction across mail and DAV.

// giveUsername adds the username field to the bare test users collection (the
// real field ships in a core migration) and sets it on the record.
func giveUsername(t *testing.T, app core.App, user *core.Record, username string) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if users.Fields.GetByName("username") == nil {
		users.Fields.Add(&core.TextField{Name: "username"})
		if err := app.Save(users); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Set("username", username)
	if err := app.Save(fresh); err != nil {
		t.Fatal(err)
	}
}

func TestIMAPLogin_AcceptsBareUsername(t *testing.T) {
	env := setupMailGuestApp(t)
	giveUsername(t, env.app, env.member, "member-imap")

	s := newIMAPSession(env.app)
	if err := s.Login("member-imap", "Password123!"); err != nil {
		t.Fatalf("bare-username Login = %v, want success (DAV accepts it; mail must too)", err)
	}
	if s.user == nil || s.user.Id != env.member.Id {
		t.Fatal("session should hold the authenticated user")
	}
}

func TestSMTPAuth_AcceptsBareUsername(t *testing.T) {
	env := setupMailGuestApp(t)
	giveUsername(t, env.app, env.member, "member-smtp")

	s := &smtpSession{app: env.app}
	if err := s.authenticate("member-smtp", "Password123!"); err != nil {
		t.Fatalf("bare-username authenticate = %v, want success", err)
	}
	if s.user == nil || s.user.Id != env.member.Id {
		t.Fatal("session should hold the authenticated user")
	}
}

func TestIMAPLogin_WrongUsernameStillFails(t *testing.T) {
	env := setupMailGuestApp(t)
	giveUsername(t, env.app, env.member, "member-real")

	s := newIMAPSession(env.app)
	if err := s.Login("member-ghost", "Password123!"); err == nil {
		t.Fatal("unknown username must not authenticate")
	}
}
