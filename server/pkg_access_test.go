package mail

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// pkg_access_test.go pins that a readonly org_pkg_access grant for the mail
// package binds the protocol listeners. IMAP and SMTP bypass the REST layer
// (where core's request-hook guard lives): without their own checks a
// readonly user could still send as the deployment's domain over :465 and
// mutate mailbox state over :993. Readonly means read — IMAP login and
// fetches stay available; sending and mutation do not.

// giveMailReadonly builds the org_pkg_access collection (absent from the bare
// test fixture; shipped by core's migrations in a real deployment) and grants
// user a readonly override for mail.
func giveMailReadonly(t *testing.T, app core.App, user *core.Record) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	access := core.NewBaseCollection("org_pkg_access")
	access.Fields.Add(&core.RelationField{
		Name: "user", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	access.Fields.Add(&core.TextField{Name: "pkg", Required: true})
	access.Fields.Add(&core.SelectField{
		Name: "access", Required: true, Values: []string{"full", "readonly", "none"}, MaxSelect: 1,
	})
	if err := app.Save(access); err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(access)
	row.Set("user", user.Id)
	row.Set("pkg", "mail")
	row.Set("access", "readonly")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
}

// SMTP submission exists only to send; a readonly grant refuses at AUTH.
func TestSMTPAuth_ReadonlyGrantRefused(t *testing.T) {
	env := setupMailGuestApp(t)
	giveMailReadonly(t, env.app, env.member)

	s := &smtpSession{app: env.app}
	if err := s.authenticate(env.member.Email(), "Password123!"); err == nil {
		t.Fatal("readonly user authenticated for SMTP — they could send as the deployment's domain")
	}
}

// IMAP login (reading) stays available; mutating commands are refused.
func TestIMAP_ReadonlyGrantBlocksMutationsNotReads(t *testing.T) {
	env := setupMailGuestApp(t)
	giveMailReadonly(t, env.app, env.member)

	s := newIMAPSession(env.app)
	if err := s.Login(env.member.Email(), "Password123!"); err != nil {
		t.Fatalf("readonly user should still log in to READ mail, got %v", err)
	}

	// Every mutating command must refuse WITH THE READ-ONLY REASON — an
	// incidental error (e.g. "no mailbox selected", nil args) would mean the
	// guard never ran and a well-formed command would mutate.
	wantReadonly := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("readonly user's %s was accepted", label)
		}
		if !strings.Contains(err.Error(), "read-only") {
			t.Fatalf("%s refused with %v — want the read-only refusal, not an incidental error", label, err)
		}
	}

	wantReadonly("CREATE", s.Create("Labels/ReadonlyMade", nil))
	wantReadonly("DELETE", s.Delete("Labels/Whatever"))
	wantReadonly("RENAME", s.Rename("Labels/A", "Labels/B", nil))
	wantReadonly("EXPUNGE", s.Expunge(nil, nil))
	wantReadonly("STORE", s.Store(nil, nil, nil, nil))
	_, appendErr := s.Append("INBOX", nil, nil)
	wantReadonly("APPEND", appendErr)
	_, copyErr := s.Copy(nil, "Archive")
	wantReadonly("COPY", copyErr)
	wantReadonly("MOVE", s.Move(nil, nil, "Archive"))
}
