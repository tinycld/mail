package mail

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/rlstest"
)

// member_share_rls_test.go proves the shared-mailbox roster works end to end
// against the SHIPPED migrations (rlstest), not rule constants declared here.
//
// Until 1830000004 the mail_mailbox_members list/view/delete rules were the
// self-only `user = @request.auth.id` while create/update were owner-gated —
// so an owner saw "1 member" (their own row), a teammate they added vanished
// on reload (the row existed but was invisible), and removing them 404'd.
// Calendar hit and fixed the identical bug in its 1830000007; this is the
// mail port, and the same suite shape guards it.

type mailShareEnv struct {
	app           *tests.TestApp
	mailbox       *core.Record
	owner         *core.Record
	teammate      *core.Record
	outsider      *core.Record
	ownerToken    string
	teammateToken string
	outsiderToken string
}

func setupMailShareApp(t *testing.T) *mailShareEnv {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	// `role` and `disabled` belong to core's users schema, which this module
	// does not carry; the shipped rules read both.
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		t.Fatalf("add users fields: %v", err)
	}

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb-migrations"))

	owner := mailGuestUser(t, app, "roster-owner@test.local", "member")
	teammate := mailGuestUser(t, app, "roster-teammate@test.local", "member")
	outsider := mailGuestUser(t, app, "roster-outsider@test.local", "member")

	domains, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatal(err)
	}
	domain := core.NewRecord(domains)
	domain.Set("domain", "roster.test")
	domain.Set("verified", true)
	if err := app.Save(domain); err != nil {
		t.Fatalf("save domain: %v", err)
	}

	mailboxes, err := app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	mailbox := core.NewRecord(mailboxes)
	mailbox.Set("address", "support")
	mailbox.Set("name", "Support")
	mailbox.Set("domain", domain.Id)
	mailbox.Set("type", "shared")
	if err := app.Save(mailbox); err != nil {
		t.Fatalf("save mailbox: %v", err)
	}

	addMailboxMember(t, app, mailbox, owner, "owner")

	ownerToken, err := owner.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	teammateToken, err := teammate.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	outsiderToken, err := outsider.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	return &mailShareEnv{
		app: app, mailbox: mailbox,
		owner: owner, teammate: teammate, outsider: outsider,
		ownerToken: ownerToken, teammateToken: teammateToken, outsiderToken: outsiderToken,
	}
}

func addMailboxMember(t *testing.T, app core.App, mailbox, user *core.Record, role string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_mailbox_members")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("mailbox", mailbox.Id)
	r.Set("user", user.Id)
	r.Set("role", role)
	if err := app.Save(r); err != nil {
		t.Fatalf("save membership: %v", err)
	}
	return r
}

func listMailboxMembers(t *testing.T, env *mailShareEnv, token string, content ...string) {
	t.Helper()
	(&tests.ApiScenario{
		Name:   "LIST mail_mailbox_members",
		Method: http.MethodGet,
		URL:    "/api/collections/mail_mailbox_members/records",
		Headers: map[string]string{
			"Authorization": token,
		},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       content,
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}).Test(t)
}

func deleteMailboxMember(t *testing.T, env *mailShareEnv, token, rowID string, wantStatus int) {
	t.Helper()
	scenario := &tests.ApiScenario{
		Name:   "DELETE mail_mailbox_members row",
		Method: http.MethodDelete,
		URL:    "/api/collections/mail_mailbox_members/records/" + rowID,
		Headers: map[string]string{
			"Authorization": token,
		},
		ExpectedStatus:        wantStatus,
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	if wantStatus != http.StatusNoContent {
		scenario.ExpectedContent = []string{`"message"`}
	}
	scenario.Test(t)
}

// THE ROSTER. After adding a teammate, the owner's member list must contain
// both rows, or the drawer shows "1 member" forever and there is no row to
// remove the teammate from.
func TestMailShareRLS_OwnerListsTheMemberTheyAdded(t *testing.T) {
	env := setupMailShareApp(t)
	addMailboxMember(t, env.app, env.mailbox, env.teammate, "member")
	listMailboxMembers(t, env, env.ownerToken,
		`"totalItems":2`,
		`"user":"`+env.teammate.Id+`"`,
		`"user":"`+env.owner.Id+`"`)
}

// A non-owner member sees who else shares the mailbox — same product shape
// as calendar; a row reveals only (mailbox, user, role).
func TestMailShareRLS_MemberListsCoMembers(t *testing.T) {
	env := setupMailShareApp(t)
	addMailboxMember(t, env.app, env.mailbox, env.teammate, "member")
	listMailboxMembers(t, env, env.teammateToken,
		`"totalItems":2`,
		`"user":"`+env.owner.Id+`"`)
}

// Control: widening to co-members must not leak rows across mailboxes.
func TestMailShareRLS_OutsiderListsNothing(t *testing.T) {
	env := setupMailShareApp(t)
	addMailboxMember(t, env.app, env.mailbox, env.teammate, "member")
	listMailboxMembers(t, env, env.outsiderToken, `"totalItems":0`)
}

// THE REMOVE HALF. A mailbox owner removes a teammate's row — the action the
// drawer's remove button performs, which 404'd under the self-only rule.
func TestMailShareRLS_OwnerRemovesAMember(t *testing.T) {
	env := setupMailShareApp(t)
	row := addMailboxMember(t, env.app, env.mailbox, env.teammate, "member")
	deleteMailboxMember(t, env, env.ownerToken, row.Id, http.StatusNoContent)
}

// Self-leave keeps working: a member can delete their own row.
func TestMailShareRLS_MemberCanLeave(t *testing.T) {
	env := setupMailShareApp(t)
	row := addMailboxMember(t, env.app, env.mailbox, env.teammate, "member")
	deleteMailboxMember(t, env, env.teammateToken, row.Id, http.StatusNoContent)
}

// A non-owner member must not remove OTHER members' rows.
func TestMailShareRLS_MemberCannotRemoveCoMember(t *testing.T) {
	env := setupMailShareApp(t)
	addMailboxMember(t, env.app, env.mailbox, env.teammate, "member")
	ownerRow, err := env.app.FindFirstRecordByFilter(
		"mail_mailbox_members", "user = {:u}", map[string]any{"u": env.owner.Id})
	if err != nil {
		t.Fatal(err)
	}
	deleteMailboxMember(t, env, env.teammateToken, ownerRow.Id, http.StatusNotFound)
}

// Drift guards: the shipped list/view rules must admit mailbox co-members,
// and delete must carry the owner clause alongside self-leave.
func TestMailShareRLS_ShippedRulesAdmitCoMembers(t *testing.T) {
	env := setupMailShareApp(t)
	for _, verb := range []string{"list", "view"} {
		rlstest.RequireRuleContains(t, env.app, "mail_mailbox_members", verb,
			`mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id`)
	}
	rlstest.RequireRuleContains(t, env.app, "mail_mailbox_members", "delete",
		`mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"`)
}
