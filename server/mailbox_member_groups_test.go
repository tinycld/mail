package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/groups"
	"tinycld.org/core/rlstest"
)

// mailbox_member_groups_test.go proves that a group can be a member of a
// shared inbox: core's expander turns a grant row into one derived row per
// group member, only an inbox owner manages grants, and nobody but a superuser
// edits a derived row.

// newInboxGroupTestApp applies the shipped migrations and binds the hooks the
// way production does: mail registers its grant table and hooks first, then
// core's groups.Register binds the expander for every registered table. Core's
// registry is process global, so it is cleared before and after.
func newInboxGroupTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newRealSchemaApp(t)
	groups.ResetForTesting()
	t.Cleanup(groups.ResetForTesting)
	registerMailboxMemberGroups(app)
	registerMailboxLastOwnerGuard(app)
	groups.Register(&pocketbase.PocketBase{App: app})
	return app
}

type inboxGroupSeed struct {
	shared   *core.Record
	personal *core.Record
	owner    *core.Record
	member   *core.Record
	ann      *core.Record
	bob      *core.Record
	sales    *core.Record
}

// seedInboxGroup makes a shared inbox with an owner and a plain member, the
// owner's personal inbox, and the group Sales with ann and bob in it.
func seedInboxGroup(t *testing.T, app core.App) *inboxGroupSeed {
	t.Helper()
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "inbox.example", "verified": true})
	s := &inboxGroupSeed{
		owner:  seedGroupUser(t, app, "olga"),
		member: seedGroupUser(t, app, "mike"),
		ann:    seedGroupUser(t, app, "ann"),
		bob:    seedGroupUser(t, app, "bob"),
	}
	s.shared = saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "sales", "domain": domain.Id, "type": "shared", "name": "Sales", "display_name": "Sales",
	})
	s.personal = saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "olga", "domain": domain.Id, "type": "personal", "name": "Olga", "display_name": "Olga",
	})
	saveRealRecord(t, app, "mail_mailbox_members", map[string]any{"mailbox": s.shared.Id, "user": s.owner.Id, "role": "owner"})
	saveRealRecord(t, app, "mail_mailbox_members", map[string]any{"mailbox": s.shared.Id, "user": s.member.Id, "role": "member"})
	saveRealRecord(t, app, "mail_mailbox_members", map[string]any{"mailbox": s.personal.Id, "user": s.owner.Id, "role": "owner"})
	s.sales = seedGroup(t, app, "Sales", s.ann, s.bob)
	return s
}

func saveInboxGrant(t *testing.T, app core.App, mailbox, group *core.Record, role string) *core.Record {
	t.Helper()
	return saveRealRecord(t, app, "mail_mailbox_members", map[string]any{
		"mailbox": mailbox.Id, "group": group.Id, "role": role,
	})
}

// derivedRow returns the row the grant of group gives user on mailbox, or nil.
func derivedRow(t *testing.T, app core.App, mailbox, group, user *core.Record) *core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_mailbox_members", "mailbox = {:m} && group = {:g} && user = {:u}", "", 0, 0,
		map[string]any{"m": mailbox.Id, "g": group.Id, "u": user.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func grantBody(mailboxID, groupID, role string) string {
	return `{"mailbox":"` + mailboxID + `","group":"` + groupID + `","role":"` + role + `"}`
}

// inboxGroupCall sends one API request against a fresh seeded app. Each call
// gets its own app: the vendored PocketBase binds a new OnServe handler on
// every tests.ApiScenario run.
type inboxGroupCall struct {
	method  string
	url     func(t *testing.T, s *inboxGroupSeed, app core.App) string
	body    func(s *inboxGroupSeed) string
	caller  func(s *inboxGroupSeed) *core.Record
	before  func(t *testing.T, app core.App, s *inboxGroupSeed)
	status  int
	content []string
	after   func(t *testing.T, app core.App, s *inboxGroupSeed)
}

func (c inboxGroupCall) run(t *testing.T) {
	t.Helper()
	var seed *inboxGroupSeed
	scenario := tests.ApiScenario{
		Name:            c.method,
		Method:          c.method,
		ExpectedStatus:  c.status,
		ExpectedContent: c.content,
		Headers:         map[string]string{"Content-Type": "application/json"},
	}
	if len(scenario.ExpectedContent) == 0 && c.status != http.StatusNoContent {
		scenario.ExpectedContent = []string{`"message"`}
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		app := newInboxGroupTestApp(t)
		seed = seedInboxGroup(t, app)
		if c.before != nil {
			c.before(t, app, seed)
		}
		scenario.URL = c.url(t, seed, app)
		if c.body != nil {
			scenario.Body = strings.NewReader(c.body(seed))
		}
		return app
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		scenario.Headers["Authorization"] = authTokenFor(t, c.caller(seed))
	}
	scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
		if c.after != nil {
			c.after(t, app, seed)
		}
	}
	scenario.Test(t)
}

func membersURL(*testing.T, *inboxGroupSeed, core.App) string { return listURL("mail_mailbox_members") }

func withSalesGrant(t *testing.T, app core.App, s *inboxGroupSeed) {
	saveInboxGrant(t, app, s.shared, s.sales, "member")
}

func grantURL(t *testing.T, s *inboxGroupSeed, app core.App) string {
	t.Helper()
	grant, err := app.FindFirstRecordByFilter("mail_mailbox_members", `mailbox = {:m} && group = {:g} && user = ""`,
		map[string]any{"m": s.shared.Id, "g": s.sales.Id})
	if err != nil {
		t.Fatal(err)
	}
	return recordURL("mail_mailbox_members", grant.Id)
}

func derivedURL(user func(s *inboxGroupSeed) *core.Record) func(t *testing.T, s *inboxGroupSeed, app core.App) string {
	return func(t *testing.T, s *inboxGroupSeed, app core.App) string {
		t.Helper()
		row, err := app.FindFirstRecordByFilter("mail_mailbox_members", "mailbox = {:m} && group = {:g} && user = {:u}",
			map[string]any{"m": s.shared.Id, "g": s.sales.Id, "u": user(s).Id})
		if err != nil {
			t.Fatal(err)
		}
		return recordURL("mail_mailbox_members", row.Id)
	}
}

func ownerOf(s *inboxGroupSeed) *core.Record  { return s.owner }
func memberOf(s *inboxGroupSeed) *core.Record { return s.member }
func annOf(s *inboxGroupSeed) *core.Record    { return s.ann }

func requireNoGrant(t *testing.T, app core.App, s *inboxGroupSeed) {
	t.Helper()
	n, err := app.CountRecords("mail_mailbox_members")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := app.FindRecordsByFilter("mail_mailbox_members", `group != ""`, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("%d group rows exist (of %d member rows), want none", len(rows), n)
	}
}

func TestInboxGroup_GrantExpandsToMembers(t *testing.T) {
	app := newInboxGroupTestApp(t)
	s := seedInboxGroup(t, app)
	saveInboxGrant(t, app, s.shared, s.sales, "member")

	for _, user := range []*core.Record{s.ann, s.bob} {
		row := derivedRow(t, app, s.shared, s.sales, user)
		if row == nil {
			t.Fatalf("%s has no derived row after the grant", user.GetString("username"))
		}
		if row.GetString("role") != "member" {
			t.Fatalf("%s derived role = %q, want member", user.GetString("username"), row.GetString("role"))
		}
	}

	carl := seedGroupUser(t, app, "carl")
	saveRealRecord(t, app, "group_members", map[string]any{"group": s.sales.Id, "user": carl.Id})
	if derivedRow(t, app, s.shared, s.sales, carl) == nil {
		t.Fatal("carl has no derived row after he joins Sales")
	}

	if err := app.Delete(membershipOf(t, app, s.sales, s.ann)); err != nil {
		t.Fatal(err)
	}
	if row := derivedRow(t, app, s.shared, s.sales, s.ann); row != nil {
		t.Fatalf("ann keeps derived row %s after she leaves Sales", row.Id)
	}
}

func TestInboxGroup_GrantRoleIsAlwaysMember(t *testing.T) {
	t.Run("owner POSTs an owner grant", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodPost, url: membersURL, caller: ownerOf,
			body:   func(s *inboxGroupSeed) string { return grantBody(s.shared.Id, s.sales.Id, "owner") },
			status: http.StatusBadRequest,
			after:  requireNoGrant,
		}.run(t)
	})

	t.Run("superuser save stores member", func(t *testing.T) {
		app := newInboxGroupTestApp(t)
		s := seedInboxGroup(t, app)
		grant := saveInboxGrant(t, app, s.shared, s.sales, "owner")
		if got := reloaded(t, app, grant).GetString("role"); got != "member" {
			t.Fatalf("grant role = %q, want member", got)
		}
		if row := derivedRow(t, app, s.shared, s.sales, s.ann); row == nil || row.GetString("role") != "member" {
			t.Fatalf("ann's derived row = %v, want role member", row)
		}

		grant = reloaded(t, app, grant)
		grant.Set("role", "owner")
		if err := app.Save(grant); err != nil {
			t.Fatal(err)
		}
		if got := reloaded(t, app, grant).GetString("role"); got != "member" {
			t.Fatalf("grant role after update = %q, want member", got)
		}
	})
}

func TestInboxGroup_NoGrantOnPersonalInbox(t *testing.T) {
	inboxGroupCall{
		method: http.MethodPost, url: membersURL, caller: ownerOf,
		body:   func(s *inboxGroupSeed) string { return grantBody(s.personal.Id, s.sales.Id, "member") },
		status: http.StatusBadRequest,
		after:  requireNoGrant,
	}.run(t)
}

func TestInboxGroup_DerivedRowIsReadOnly(t *testing.T) {
	t.Run("own user DELETEs it", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodDelete, url: derivedURL(annOf), caller: annOf, before: withSalesGrant,
			status:  http.StatusForbidden,
			content: []string{"You have access through the group Sales. Ask an admin to remove you from the group."},
			after: func(t *testing.T, app core.App, s *inboxGroupSeed) {
				if derivedRow(t, app, s.shared, s.sales, s.ann) == nil {
					t.Fatal("ann's derived row was deleted")
				}
			},
		}.run(t)
	})

	t.Run("owner DELETEs it", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodDelete, url: derivedURL(annOf), caller: ownerOf, before: withSalesGrant,
			status:  http.StatusForbidden,
			content: []string{"the group Sales"},
		}.run(t)
	})

	t.Run("owner PATCHes its role", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodPatch, url: derivedURL(annOf), caller: ownerOf, before: withSalesGrant,
			body:    func(*inboxGroupSeed) string { return `{"role":"owner"}` },
			status:  http.StatusForbidden,
			content: []string{"the group Sales"},
			after: func(t *testing.T, app core.App, s *inboxGroupSeed) {
				if row := derivedRow(t, app, s.shared, s.sales, s.ann); row == nil || row.GetString("role") != "member" {
					t.Fatalf("ann's derived row = %v, want role member", row)
				}
			},
		}.run(t)
	})
}

func TestInboxGroup_OnlyOwnerManagesGrants(t *testing.T) {
	t.Run("member POSTs a grant", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodPost, url: membersURL, caller: memberOf,
			body:   func(s *inboxGroupSeed) string { return grantBody(s.shared.Id, s.sales.Id, "member") },
			status: http.StatusBadRequest,
			after:  requireNoGrant,
		}.run(t)
	})

	t.Run("member DELETEs a grant", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodDelete, url: grantURL, caller: memberOf, before: withSalesGrant,
			status: http.StatusNotFound,
			after: func(t *testing.T, app core.App, s *inboxGroupSeed) {
				if derivedRow(t, app, s.shared, s.sales, s.ann) == nil {
					t.Fatal("the grant's derived rows went away")
				}
			},
		}.run(t)
	})

	t.Run("owner POSTs a grant", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodPost, url: membersURL, caller: ownerOf,
			body:    func(s *inboxGroupSeed) string { return grantBody(s.shared.Id, s.sales.Id, "member") },
			status:  http.StatusOK,
			content: []string{`"role":"member"`, `"user":""`},
			after: func(t *testing.T, app core.App, s *inboxGroupSeed) {
				if derivedRow(t, app, s.shared, s.sales, s.bob) == nil {
					t.Fatal("bob has no derived row after the owner's grant")
				}
			},
		}.run(t)
	})

	t.Run("owner DELETEs a grant", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodDelete, url: grantURL, caller: ownerOf, before: withSalesGrant,
			status: http.StatusNoContent,
			after:  requireNoGrant,
		}.run(t)
	})

	t.Run("owner POSTs a row with both user and group", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodPost, url: membersURL, caller: ownerOf,
			body: func(s *inboxGroupSeed) string {
				return `{"mailbox":"` + s.shared.Id + `","user":"` + s.ann.Id + `","group":"` + s.sales.Id + `","role":"member"}`
			},
			status: http.StatusBadRequest,
			after:  requireNoGrant,
		}.run(t)
	})

	t.Run("owner PATCHes a person row with a group", func(t *testing.T) {
		inboxGroupCall{
			method: http.MethodPatch, caller: ownerOf,
			url: func(t *testing.T, s *inboxGroupSeed, app core.App) string {
				row, err := app.FindFirstRecordByFilter("mail_mailbox_members", "mailbox = {:m} && user = {:u}",
					map[string]any{"m": s.shared.Id, "u": s.member.Id})
				if err != nil {
					t.Fatal(err)
				}
				return recordURL("mail_mailbox_members", row.Id)
			},
			body:    func(s *inboxGroupSeed) string { return `{"group":"` + s.sales.Id + `","user":"` + s.ann.Id + `"}` },
			status:  http.StatusOK,
			content: []string{`"group":""`},
			after: func(t *testing.T, app core.App, s *inboxGroupSeed) {
				requireNoGrant(t, app, s)
				row, err := app.FindFirstRecordByFilter("mail_mailbox_members", "mailbox = {:m} && user = {:u}",
					map[string]any{"m": s.shared.Id, "u": s.member.Id})
				if err != nil {
					t.Fatalf("mike's row lost its user: %v", err)
				}
				if row.GetString("group") != "" {
					t.Fatalf("mike's row group = %q, want empty", row.GetString("group"))
				}
			},
		}.run(t)
	})
}

func TestInboxGroup_RulesGuardGrantUser(t *testing.T) {
	app := newRealSchemaApp(t)
	rlstest.RequireAuthGuardOnGrantRules(t, app, "mail_mailbox_members")
}
