package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// mailbox_owner_guard_test.go proves a shared mailbox cannot lose its last
// owner. The drawer's remove button already excludes the last owner
// (canRemove), and the shipped help says removing the last owner is blocked —
// but the ROLE TOGGLE had no such check, and no server guard existed for
// either path: the last owner could self-demote to member, leaving a mailbox
// nobody can manage (ownerCanAdd requires an owner row, and the bootstrap
// branch only fires on a memberless mailbox).

func setupMailboxOwnerGuardApp(t *testing.T) *mailShareEnv {
	t.Helper()
	env := setupMailShareApp(t)
	registerMailboxLastOwnerGuard(env.app)
	return env
}

// triggerMemberUpdate invokes the OnRecordUpdateRequest hook chain the way
// the API would (see users_guard_test.go in core for the pattern rationale).
func triggerMemberUpdate(
	t *testing.T,
	app *tests.TestApp,
	caller *core.Record,
	rowID string,
	mutate func(*core.Record),
) error {
	t.Helper()
	fresh, err := app.FindRecordById("mail_mailbox_members", rowID)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	mutate(fresh)
	col, err := app.FindCollectionByNameOrId("mail_mailbox_members")
	if err != nil {
		t.Fatal(err)
	}
	e := &core.RecordRequestEvent{
		RequestEvent: &core.RequestEvent{Auth: caller, App: app},
		Record:       fresh,
	}
	e.Collection = col
	return app.OnRecordUpdateRequest("mail_mailbox_members").
		Trigger(e, func(_ *core.RecordRequestEvent) error {
			return app.Save(fresh)
		})
}

func triggerMemberDelete(
	t *testing.T,
	app *tests.TestApp,
	caller *core.Record,
	rowID string,
) error {
	t.Helper()
	fresh, err := app.FindRecordById("mail_mailbox_members", rowID)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	col, err := app.FindCollectionByNameOrId("mail_mailbox_members")
	if err != nil {
		t.Fatal(err)
	}
	e := &core.RecordRequestEvent{
		RequestEvent: &core.RequestEvent{Auth: caller, App: app},
		Record:       fresh,
	}
	e.Collection = col
	return app.OnRecordDeleteRequest("mail_mailbox_members").
		Trigger(e, func(_ *core.RecordRequestEvent) error {
			return app.Delete(fresh)
		})
}

func ownerRowOf(t *testing.T, env *mailShareEnv, user *core.Record) *core.Record {
	t.Helper()
	row, err := env.app.FindFirstRecordByFilter(
		"mail_mailbox_members",
		"mailbox = {:mb} && user = {:u}",
		map[string]any{"mb": env.mailbox.Id, "u": user.Id},
	)
	if err != nil {
		t.Fatalf("find membership row: %v", err)
	}
	return row
}

func TestMailboxOwnerGuard_BlocksSelfDemotionOfLastOwner(t *testing.T) {
	env := setupMailboxOwnerGuardApp(t)
	row := ownerRowOf(t, env, env.owner)

	err := triggerMemberUpdate(t, env.app, env.owner, row.Id, func(r *core.Record) {
		r.Set("role", "member")
	})
	if err == nil {
		t.Fatal("demoting the mailbox's last owner should be rejected")
	}

	fresh, _ := env.app.FindRecordById("mail_mailbox_members", row.Id)
	if fresh.GetString("role") != "owner" {
		t.Errorf("role should still be owner, got %q", fresh.GetString("role"))
	}
}

func TestMailboxOwnerGuard_AllowsDemotionWithSecondOwner(t *testing.T) {
	env := setupMailboxOwnerGuardApp(t)
	addMailboxMember(t, env.app, env.mailbox, env.teammate, "owner")
	row := ownerRowOf(t, env, env.owner)

	if err := triggerMemberUpdate(t, env.app, env.owner, row.Id, func(r *core.Record) {
		r.Set("role", "member")
	}); err != nil {
		t.Fatalf("demoting one of two owners should be allowed: %v", err)
	}
}

func TestMailboxOwnerGuard_BlocksDeletingLastOwnerRow(t *testing.T) {
	env := setupMailboxOwnerGuardApp(t)
	row := ownerRowOf(t, env, env.owner)

	if err := triggerMemberDelete(t, env.app, env.owner, row.Id); err == nil {
		t.Fatal("deleting the mailbox's last owner row should be rejected")
	}
	if _, err := env.app.FindRecordById("mail_mailbox_members", row.Id); err != nil {
		t.Fatalf("owner row should still exist: %v", err)
	}
}

func TestMailboxOwnerGuard_AllowsRemovingAPlainMember(t *testing.T) {
	env := setupMailboxOwnerGuardApp(t)
	row := addMailboxMember(t, env.app, env.mailbox, env.teammate, "member")

	if err := triggerMemberDelete(t, env.app, env.owner, row.Id); err != nil {
		t.Fatalf("removing a plain member should be allowed: %v", err)
	}
}

func TestMailboxOwnerGuard_AllowsOwnerLeaveWithSecondOwner(t *testing.T) {
	env := setupMailboxOwnerGuardApp(t)
	addMailboxMember(t, env.app, env.mailbox, env.teammate, "owner")
	row := ownerRowOf(t, env, env.owner)

	if err := triggerMemberDelete(t, env.app, env.owner, row.Id); err != nil {
		t.Fatalf("an owner leaving with a second owner present should be allowed: %v", err)
	}
}
