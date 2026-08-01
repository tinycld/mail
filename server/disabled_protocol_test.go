package mail

import (
	"errors"
	"testing"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase/core"
)

// disabled_protocol_test.go proves the IMAP/SMTP half of the suspension gate.
//
// coreserver's disabled guard binds PocketBase's auth hooks, which cover token
// issuance — but the mail listeners authenticate with FindAuthRecordByEmail +
// ValidatePassword directly, so those hooks never run. Without an explicit
// check in the protocol Login paths, a disabled account keeps reading every
// mailbox and sending as the deployment's domain over :993/:465 indefinitely:
// unlike REST tokens, protocol logins never expire.

// disableUser flips the suspension flag, adding the `disabled` field when the
// bare test users collection predates it (the real field ships in a core
// migration).
func disableUser(t *testing.T, app core.App, user *core.Record) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if users.Fields.GetByName("disabled") == nil {
		users.Fields.Add(&core.BoolField{Name: "disabled"})
		if err := app.Save(users); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Set("disabled", true)
	if err := app.Save(fresh); err != nil {
		t.Fatal(err)
	}
}

func TestIMAPLogin_DisabledUserRejected(t *testing.T) {
	env := setupMailGuestApp(t)

	// Positive control first: the same credentials succeed while enabled, so a
	// failure below is the disabled check and not a broken fixture.
	s := newIMAPSession(env.app)
	if err := s.Login(env.member.Email(), "Password123!"); err != nil {
		t.Fatalf("enabled user should log in, got %v", err)
	}
	_ = s.Close()

	disableUser(t, env.app, env.member)

	s = newIMAPSession(env.app)
	err := s.Login(env.member.Email(), "Password123!")
	if !errors.Is(err, imapserver.ErrAuthFailed) {
		t.Fatalf("disabled user Login = %v, want ErrAuthFailed", err)
	}
}

func TestSMTPAuth_DisabledUserRejected(t *testing.T) {
	env := setupMailGuestApp(t)

	s := &smtpSession{app: env.app}
	if err := s.authenticate(env.member.Email(), "Password123!"); err != nil {
		t.Fatalf("enabled user should authenticate, got %v", err)
	}

	disableUser(t, env.app, env.member)

	s = &smtpSession{app: env.app}
	if err := s.authenticate(env.member.Email(), "Password123!"); err == nil {
		t.Fatal("disabled user authenticate = nil, want 535 rejection")
	} else if s.user != nil {
		t.Fatal("disabled user must not be attached to the session")
	}
}
