package mail

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/offboard"
)

// offboard_test.go covers what happens to shared mailboxes when their owner
// leaves: the offboard handler that hands a sole-owned shared mailbox over
// (offboard.go), and the guard that stops a direct users delete from
// stripping a shared mailbox of its only owner. Both run against the SHIPPED
// schema (real migrations, so the unique (mailbox, user) index is present).
//
// The env from setupMailShareApp: env.owner is the only owner of the shared
// mailbox env.mailbox ("Support"), env.teammate is a member of it here, and
// env.outsider has no mailbox.

type mailOffboardEnv struct {
	*mailShareEnv
	message *core.Record
	admin   *core.Record
}

func setupMailOffboardApp(t *testing.T) *mailOffboardEnv {
	t.Helper()
	env := setupMailShareApp(t)
	offboard.ResetHandlersForTesting()
	t.Cleanup(offboard.ResetHandlersForTesting)
	offboard.RegisterHandler("mail", offboardMail)
	registerSoleOwnerDeleteGuard(env.app)
	registerPersonalMailboxCleanup(env.app)

	addMailboxMember(t, env.app, env.mailbox, env.teammate, "member")
	_, message := saveThreadWithMessage(t, env.app, env.mailbox, "Quarterly numbers")

	return &mailOffboardEnv{
		mailShareEnv: env,
		message:      message,
		admin:        mailGuestUser(t, env.app, "roster-admin@test.local", "admin"),
	}
}

func newMailbox(t *testing.T, env *mailOffboardEnv, address, kind string) *core.Record {
	t.Helper()
	col, err := env.app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	mb := core.NewRecord(col)
	mb.Set("address", address)
	mb.Set("name", address)
	mb.Set("domain", env.mailbox.GetString("domain"))
	mb.Set("type", kind)
	if err := env.app.Save(mb); err != nil {
		t.Fatalf("save mailbox %s: %v", address, err)
	}
	return mb
}

func mailboxMembership(t *testing.T, app core.App, mailbox, user *core.Record) *core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_mailbox_members",
		"mailbox = {:m} && user = {:u}", "", 0, 0,
		map[string]any{"m": mailbox.Id, "u": user.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) > 1 {
		t.Fatalf("%d memberships for one (mailbox, user)", len(rows))
	}
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func requireMailboxRole(t *testing.T, app core.App, mailbox, user *core.Record, want string) {
	t.Helper()
	m := mailboxMembership(t, app, mailbox, user)
	got := ""
	if m != nil {
		got = m.GetString("role")
	}
	if got != want {
		t.Errorf("role of %s on %q = %q, want %q", user.Email(), mailbox.GetString("address"), got, want)
	}
}

func requireMailKept(t *testing.T, env *mailOffboardEnv) {
	t.Helper()
	if _, err := env.app.FindRecordById("mail_mailboxes", env.mailbox.Id); err != nil {
		t.Errorf("handed-over mailbox is gone: %v", err)
	}
	if _, err := env.app.FindRecordById("mail_messages", env.message.Id); err != nil {
		t.Errorf("message in the handed-over mailbox is gone: %v", err)
	}
}

func requireUserAnonymized(t *testing.T, app core.App, user *core.Record, want bool) {
	t.Helper()
	fresh, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatalf("users row of %s is gone: %v", user.Email(), err)
	}
	if got := fresh.GetString("name") == "Deleted user"; got != want {
		t.Errorf("anonymized = %v, want %v", got, want)
	}
}

func TestMailOffboard_ReassignMovesSoleOwnedMailboxToSuccessor(t *testing.T) {
	env := setupMailOffboardApp(t)

	if _, err := offboard.OffboardUser(env.app, env.owner.Id, offboard.Plan{
		Mode: offboard.ModeReassign, SuccessorUserID: env.outsider.Id,
	}, env.admin.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}

	requireMailboxRole(t, env.app, env.mailbox, env.outsider, "owner")
	requireMailboxRole(t, env.app, env.mailbox, env.teammate, "member")
	requireMailKept(t, env)
	requireUserAnonymized(t, env.app, env.owner, true)
}

// The successor is already a member: the row is upgraded, not duplicated
// (the unique index would fail an insert).
func TestMailOffboard_ReassignUpgradesExistingMembership(t *testing.T) {
	env := setupMailOffboardApp(t)
	before := mailboxMembership(t, env.app, env.mailbox, env.teammate)

	if _, err := offboard.OffboardUser(env.app, env.owner.Id, offboard.Plan{
		Mode: offboard.ModeReassign, SuccessorUserID: env.teammate.Id,
	}, env.owner.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}

	after := mailboxMembership(t, env.app, env.mailbox, env.teammate)
	if after == nil || after.Id != before.Id || after.GetString("role") != "owner" {
		t.Errorf("membership not upgraded in place: before=%v after=%v", before, after)
	}
	requireMailKept(t, env)
}

// A mailbox with another owner does not need the leaver, so the successor
// gets nothing on it and the leaver's own rows stay. The leaver's personal
// mailbox is not handed over: it is deleted.
func TestMailOffboard_LeavesCoOwnedAndNonOwnerMembershipsAlone(t *testing.T) {
	env := setupMailOffboardApp(t)
	coOwned := newMailbox(t, env, "co-owned", "shared")
	addMailboxMember(t, env.app, coOwned, env.owner, "owner")
	addMailboxMember(t, env.app, coOwned, env.admin, "owner")
	joined := newMailbox(t, env, "someone-elses", "shared")
	addMailboxMember(t, env.app, joined, env.admin, "owner")
	addMailboxMember(t, env.app, joined, env.owner, "member")
	personal := newMailbox(t, env, "owner-inbox", "personal")
	addMailboxMember(t, env.app, personal, env.owner, "owner")

	if _, err := offboard.OffboardUser(env.app, env.owner.Id, offboard.Plan{
		Mode: offboard.ModeReassign, SuccessorUserID: env.outsider.Id,
	}, env.owner.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}

	requireMailboxRole(t, env.app, coOwned, env.outsider, "")
	requireMailboxRole(t, env.app, joined, env.outsider, "")
	requireMailboxRole(t, env.app, coOwned, env.owner, "owner")
	requireMailboxRole(t, env.app, joined, env.owner, "member")
	requireMailboxRole(t, env.app, env.mailbox, env.outsider, "owner")
	requirePersonalMailboxGone(t, env.app, personal)
}

func TestMailOffboard_DeleteMyDataByAdminMovesToAdmin(t *testing.T) {
	env := setupMailOffboardApp(t)

	if _, err := offboard.OffboardUser(env.app, env.owner.Id,
		offboard.Plan{Mode: offboard.ModeDeleteMyData}, env.admin.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}

	requireMailboxRole(t, env.app, env.mailbox, env.admin, "owner")
	requireMailboxRole(t, env.app, env.mailbox, env.teammate, "member")
	requireMailKept(t, env)
	requireUserAnonymized(t, env.app, env.owner, true)
}

// A self-delete has no one to hand a shared mailbox to. Like core's
// last-owner guard, it is refused with instructions, and nothing changes.
func TestMailOffboard_SelfDeleteMyDataRefusedForSharedMailbox(t *testing.T) {
	env := setupMailOffboardApp(t)

	_, err := offboard.OffboardUser(env.app, env.owner.Id,
		offboard.Plan{Mode: offboard.ModeDeleteMyData}, env.owner.Id)
	if !errors.Is(err, offboard.ErrInvalidPlan) {
		t.Fatalf("err = %v, want ErrInvalidPlan", err)
	}
	requireMailboxRole(t, env.app, env.mailbox, env.owner, "owner")
	requireMailboxRole(t, env.app, env.mailbox, env.teammate, "member")
	requireMailKept(t, env)
	requireUserAnonymized(t, env.app, env.owner, false)
}

// A shared mailbox only the leaver uses costs nobody access, so a self-delete
// in delete-my-data mode goes through and leaves it as it is.
func TestMailOffboard_SelfDeleteMyDataAllowedForUnusedSharedMailbox(t *testing.T) {
	env := setupMailOffboardApp(t)
	solo := newMailbox(t, env, "solo", "shared")
	addMailboxMember(t, env.app, solo, env.outsider, "owner")

	if _, err := offboard.OffboardUser(env.app, env.outsider.Id,
		offboard.Plan{Mode: offboard.ModeDeleteMyData}, env.outsider.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	requireMailboxRole(t, env.app, solo, env.outsider, "owner")
	requireUserAnonymized(t, env.app, env.outsider, true)
}

func TestMailOffboard_GuestSuccessorRefused(t *testing.T) {
	env := setupMailOffboardApp(t)
	guest := mailGuestUser(t, env.app, "roster-guest@test.local", "guest")

	_, err := offboard.OffboardUser(env.app, env.owner.Id, offboard.Plan{
		Mode: offboard.ModeReassign, SuccessorUserID: guest.Id,
	}, env.admin.Id)
	if !errors.Is(err, offboard.ErrInvalidPlan) {
		t.Fatalf("err = %v, want ErrInvalidPlan", err)
	}
	requireMailboxRole(t, env.app, env.mailbox, guest, "")
	requireUserAnonymized(t, env.app, env.owner, false)
}

// D: a direct REST delete of your own account, while you are the only owner of
// a shared mailbox other people use, is refused and points at account
// settings.
func TestMailUserDeleteGuard_RefusesSoleOwnerOfSharedMailbox(t *testing.T) {
	env := setupMailOffboardApp(t)

	(&tests.ApiScenario{
		Method:                http.MethodDelete,
		URL:                   "/api/collections/users/records/" + env.owner.Id,
		Headers:               map[string]string{"Authorization": env.ownerToken},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{`account settings`, `Support`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}).Test(t)

	requireMailboxRole(t, env.app, env.mailbox, env.owner, "owner")
	requireMailboxRole(t, env.app, env.mailbox, env.teammate, "member")
	requireMailKept(t, env)
}

func TestMailUserDeleteGuard_AllowsDeleteWithoutSharedSoleOwnedMailbox(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, env *mailOffboardEnv) (*core.Record, string)
	}{
		{
			name: "no mailboxes",
			setup: func(_ *testing.T, env *mailOffboardEnv) (*core.Record, string) {
				return env.outsider, env.outsiderToken
			},
		},
		{
			name: "personal mailbox only",
			setup: func(t *testing.T, env *mailOffboardEnv) (*core.Record, string) {
				personal := newMailbox(t, env, "outsider-inbox", "personal")
				addMailboxMember(t, env.app, personal, env.outsider, "owner")
				return env.outsider, env.outsiderToken
			},
		},
		{
			name: "shared mailbox with another owner",
			setup: func(t *testing.T, env *mailOffboardEnv) (*core.Record, string) {
				addMailboxMember(t, env.app, env.mailbox, env.outsider, "owner")
				return env.outsider, env.outsiderToken
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupMailOffboardApp(t)
			user, token := tc.setup(t, env)
			(&tests.ApiScenario{
				Method:                http.MethodDelete,
				URL:                   "/api/collections/users/records/" + user.Id,
				Headers:               map[string]string{"Authorization": token},
				ExpectedStatus:        http.StatusNoContent,
				TestAppFactory:        func(testing.TB) *tests.TestApp { return env.app },
				DisableTestAppCleanup: true,
			}).Test(t)
			if _, err := env.app.FindRecordById("users", user.Id); err == nil {
				t.Error("users row still exists after an allowed delete")
			}
		})
	}
}

// The guard does not reach the offboard path: offboarding the sole owner of a
// shared mailbox anonymizes the users row (it is never deleted) and hands the
// mailbox over.
func TestMailUserDeleteGuard_OffboardUnaffected(t *testing.T) {
	env := setupMailOffboardApp(t)

	if _, err := offboard.OffboardUser(env.app, env.owner.Id, offboard.Plan{
		Mode: offboard.ModeReassign, SuccessorUserID: env.teammate.Id,
	}, env.owner.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	requireUserAnonymized(t, env.app, env.owner, true)
	requireMailboxRole(t, env.app, env.mailbox, env.teammate, "owner")
}

// Decision: a personal mailbox is deleted with its user on every path, even
// when the user shared it. The delete cascades to the other users' member
// rows, so no share is left pointing at a mailbox that is gone.

type sharedPersonalMailbox struct {
	mailbox *core.Record
	message *core.Record
	share   *core.Record
}

// newSharedPersonalMailbox gives user a personal mailbox with a message in it,
// shared with viewer as a member.
func newSharedPersonalMailbox(t *testing.T, env *mailOffboardEnv, address string, user, viewer *core.Record) sharedPersonalMailbox {
	t.Helper()
	mb := newMailbox(t, env, address, "personal")
	addMailboxMember(t, env.app, mb, user, "owner")
	share := addMailboxMember(t, env.app, mb, viewer, "member")
	_, message := saveThreadWithMessage(t, env.app, mb, "Private note")
	return sharedPersonalMailbox{mailbox: mb, message: message, share: share}
}

func requirePersonalMailboxGone(t *testing.T, app core.App, mailbox *core.Record) {
	t.Helper()
	if _, err := app.FindRecordById("mail_mailboxes", mailbox.Id); err == nil {
		t.Errorf("personal mailbox %q still exists", mailbox.GetString("address"))
	}
	n, err := app.CountRecords("mail_mailbox_members", dbx.HashExp{"mailbox": mailbox.Id})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d member rows still point at the deleted personal mailbox", n)
	}
}

func requireSharedPersonalMailboxGone(t *testing.T, app core.App, p sharedPersonalMailbox) {
	t.Helper()
	requirePersonalMailboxGone(t, app, p.mailbox)
	if _, err := app.FindRecordById("mail_mailbox_members", p.share.Id); err == nil {
		t.Error("the other user's share row survived")
	}
	if _, err := app.FindRecordById("mail_messages", p.message.Id); err == nil {
		t.Error("a message of the deleted personal mailbox survived")
	}
}

func TestMailPersonalMailbox_RawUserDeleteRemovesMailboxAndShares(t *testing.T) {
	env := setupMailOffboardApp(t)
	personal := newSharedPersonalMailbox(t, env, "outsider-inbox", env.outsider, env.teammate)

	(&tests.ApiScenario{
		Method:                http.MethodDelete,
		URL:                   "/api/collections/users/records/" + env.outsider.Id,
		Headers:               map[string]string{"Authorization": env.outsiderToken},
		ExpectedStatus:        http.StatusNoContent,
		TestAppFactory:        func(testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}).Test(t)

	requireSharedPersonalMailboxGone(t, env.app, personal)
	requireMailKept(t, env)
}

func TestMailPersonalMailbox_OffboardRemovesMailboxAndShares(t *testing.T) {
	cases := []struct {
		name  string
		plan  func(env *mailOffboardEnv) offboard.Plan
		actor func(env *mailOffboardEnv) string
	}{
		{
			name: "reassign",
			plan: func(env *mailOffboardEnv) offboard.Plan {
				return offboard.Plan{Mode: offboard.ModeReassign, SuccessorUserID: env.admin.Id}
			},
			actor: func(env *mailOffboardEnv) string { return env.outsider.Id },
		},
		{
			name:  "delete_my_data",
			plan:  func(*mailOffboardEnv) offboard.Plan { return offboard.Plan{Mode: offboard.ModeDeleteMyData} },
			actor: func(env *mailOffboardEnv) string { return env.outsider.Id },
		},
		{
			name:  "no plan (keep)",
			plan:  func(*mailOffboardEnv) offboard.Plan { return offboard.Plan{Mode: offboard.ModeKeep} },
			actor: func(env *mailOffboardEnv) string { return env.outsider.Id },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupMailOffboardApp(t)
			personal := newSharedPersonalMailbox(t, env, "outsider-inbox", env.outsider, env.teammate)

			if _, err := offboard.OffboardUser(env.app, env.outsider.Id, tc.plan(env), tc.actor(env)); err != nil {
				t.Fatalf("OffboardUser: %v", err)
			}

			requireSharedPersonalMailboxGone(t, env.app, personal)
			requireUserAnonymized(t, env.app, env.outsider, true)
			requireMailKept(t, env)
		})
	}
}

// A later co-owner of someone else's personal mailbox does not take it down
// when they leave: only the mailbox's own user does.
func TestMailPersonalMailbox_LaterCoOwnerLeavingKeepsMailbox(t *testing.T) {
	env := setupMailOffboardApp(t)
	personal := newSharedPersonalMailbox(t, env, "outsider-inbox", env.outsider, env.teammate)
	personal.share.Set("role", "owner")
	if err := env.app.Save(personal.share); err != nil {
		t.Fatal(err)
	}

	if _, err := offboard.OffboardUser(env.app, env.teammate.Id,
		offboard.Plan{Mode: offboard.ModeKeep}, env.teammate.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	if _, err := env.app.FindRecordById("mail_mailboxes", personal.mailbox.Id); err != nil {
		t.Fatalf("personal mailbox of the remaining user was deleted: %v", err)
	}
	requireMailboxRole(t, env.app, personal.mailbox, env.outsider, "owner")
}

// Decision: an account delete with no plan (ModeKeep) is refused while the
// user is the only owner of a shared mailbox other people use, and nothing
// changes; the message says what to do.
func TestMailNoPlanDelete_RefusedForSoleOwnedSharedMailbox(t *testing.T) {
	env := setupMailOffboardApp(t)
	personal := newSharedPersonalMailbox(t, env, "owner-inbox", env.owner, env.teammate)

	_, err := offboard.OffboardUser(env.app, env.owner.Id,
		offboard.Plan{Mode: offboard.ModeKeep}, env.owner.Id)
	if !errors.Is(err, offboard.ErrInvalidPlan) {
		t.Fatalf("err = %v, want ErrInvalidPlan", err)
	}
	for _, want := range []string{"Support", "Make one of them an owner or delete the mailbox"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	requireMailboxRole(t, env.app, env.mailbox, env.owner, "owner")
	requireMailKept(t, env)
	requireUserAnonymized(t, env.app, env.owner, false)
	requireMailboxRole(t, env.app, personal.mailbox, env.teammate, "member")
}

func TestMailNoPlanDelete_AllowedWithoutSoleOwnedSharedMailbox(t *testing.T) {
	env := setupMailOffboardApp(t)
	solo := newMailbox(t, env, "solo", "shared")
	addMailboxMember(t, env.app, solo, env.outsider, "owner")
	addMailboxMember(t, env.app, env.mailbox, env.outsider, "member")

	if _, err := offboard.OffboardUser(env.app, env.outsider.Id,
		offboard.Plan{Mode: offboard.ModeKeep}, env.outsider.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	requireUserAnonymized(t, env.app, env.outsider, true)
	requireMailboxRole(t, env.app, solo, env.outsider, "owner")
	requireMailboxRole(t, env.app, env.mailbox, env.owner, "owner")
}
