package mail

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/offboard"
)

func newUsernameFollowApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newIdentityEditApp(t)
	registerUsernameFollowHook(app)
	return app
}

func renameUser(t *testing.T, app core.App, id, username string) error {
	t.Helper()
	user := mustRecord(t, app, "users", id)
	user.Set("username", username)
	return app.Save(user)
}

func TestUsernameFollow_RenameMovesAddressAndKeepsTheOld(t *testing.T) {
	t.Parallel()
	app := newUsernameFollowApp(t)
	s := seedIdentityEdit(t, app)

	if err := renameUser(t, app, s.ann.Id, "anna"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	assertIdentityField(t, app, s, "email_name", "anna")
	if got := primaryRowOf(t, app, s.annInbox.Id).GetString("local_part"); got != "anna" {
		t.Fatalf("primary = %q, want anna", got)
	}
	old, err := findAddressRow(app, "ann", s.domain.Id)
	if err != nil || old == nil || old.GetString("mailbox") != s.annInbox.Id || old.GetBool("is_primary") {
		t.Fatalf("ann@ must stay as an added address of ann's inbox: %v %v", old, err)
	}
}

func TestUsernameFollow_TakenNameRefusesTheRename(t *testing.T) {
	t.Parallel()
	app := newUsernameFollowApp(t)
	s := seedIdentityEdit(t, app)

	err := renameUser(t, app, s.ann.Id, "team") // the shared inbox's address
	if err == nil || !strings.Contains(err.Error(), "username") {
		t.Fatalf("err = %v, want a username field error", err)
	}
	if got := mustRecord(t, app, "users", s.ann.Id).GetString("username"); got != "ann" {
		t.Fatalf("username = %q, want ann (rolled back)", got)
	}
	assertIdentityField(t, app, s, "email_name", "ann")
}

func TestUsernameFollow_CustomEmailNameIsLeftAlone(t *testing.T) {
	t.Parallel()
	app := newUsernameFollowApp(t)
	s := seedIdentityEdit(t, app)
	identity := mustRecord(t, app, "mail_identities", s.annID.Id)
	identity.Set("email_name", "a.smith")
	if err := app.Save(identity); err != nil {
		t.Fatalf("custom name: %v", err)
	}
	if err := renameUser(t, app, s.ann.Id, "anna"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	assertIdentityField(t, app, s, "email_name", "a.smith")
}

func TestUsernameFollow_OffboardingDoesNotFollow(t *testing.T) {
	t.Parallel()
	app := newUsernameFollowApp(t)
	s := seedIdentityEdit(t, app)
	if _, err := offboard.OffboardUser(app, s.ann.Id, offboard.Plan{Mode: offboard.ModeKeep}, ""); err != nil {
		t.Fatalf("offboard: %v", err)
	}
	identity := mustRecord(t, app, "mail_identities", s.annID.Id)
	if strings.HasPrefix(identity.GetString("email_name"), "deleted-") {
		t.Fatalf("a tombstone username became a mail name: %q", identity.GetString("email_name"))
	}
}
