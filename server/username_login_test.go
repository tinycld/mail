package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/authidentity"
)

// username_login_test.go pins that the mail listeners accept the same
// identifiers as every other protocol server: a username, or a hosted address
// (core/davauth's contract). Members are invited by username with the email
// optional, and the account email is not a sign-in name: it is not an address
// mail hosts.

// loginName gives user a username and returns it, for a test that logs in.
func loginName(t *testing.T, app core.App, user *core.Record) string {
	t.Helper()
	name := "u-" + user.Id
	giveUsername(t, app, user, name)
	return name
}

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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	env := setupMailGuestApp(t)
	giveUsername(t, env.app, env.member, "member-real")

	s := newIMAPSession(env.app)
	if err := s.Login("member-ghost", "Password123!"); err == nil {
		t.Fatal("unknown username must not authenticate")
	}
}

func TestIMAPLogin_RefusesTheAccountEmail(t *testing.T) {
	env := setupMailGuestApp(t)
	loginName(t, env.app, env.member)

	s := newIMAPSession(env.app)
	if err := s.Login(env.member.Email(), "Password123!"); err == nil {
		t.Fatal("the account email logged in to IMAP; it is not a sign-in name")
	}
}

func TestMailLogin_AcceptsAHostedAddress(t *testing.T) {
	t.Cleanup(authidentity.ResetForTest)
	authidentity.RegisterResolver("mail", addressOwner)
	app := newAddressTestApp(t)
	org := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})
	user := saveRealRecord(t, app, "users", map[string]any{
		"email": "nathan@argosity.com", "username": "nathan", "name": "Nathan", "role": "owner",
		"password": "Password123!",
	})
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "nathan", "domain": org.Id, "type": "personal", "display_name": "Nathan", "name": "Nathan",
	})
	saveRealRecord(t, app, "mail_identities", map[string]any{"user": user.Id, "inbox": inbox.Id, "email_name": "nathan"})

	imap := newIMAPSession(app)
	if err := imap.Login("nathan@tinycld.org", "Password123!"); err != nil {
		t.Fatalf("IMAP login with the hosted address = %v, want success", err)
	}
	if imap.user == nil || imap.user.Id != user.Id {
		t.Fatal("IMAP session should hold the address's owner")
	}
	smtp := &smtpSession{app: app}
	if err := smtp.authenticate("nathan@tinycld.org", "Password123!"); err != nil {
		t.Fatalf("SMTP auth with the hosted address = %v, want success", err)
	}
	if err := newIMAPSession(app).Login("nathan@argosity.com", "Password123!"); err == nil {
		t.Fatal("the account email logged in to IMAP; it is not a sign-in name")
	}
}
