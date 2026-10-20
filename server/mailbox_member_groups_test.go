package mail

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/groups"
	"tinycld.org/core/offboard"
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
	t.Parallel()
	app := newRealSchemaApp(t)
	rlstest.RequireAuthGuardOnGrantRules(t, app, "mail_mailbox_members")
}

// The tests below cover what a person's several rows on one inbox mean for
// the server: a person with a direct row and one derived row per group is one
// member, a grant row is no person at all, and losing the last row ends the
// person's thread state for the inbox.

// newInboxStateTestApp adds the thread-state hooks to newInboxGroupTestApp:
// the state row takes its mailbox from the thread, the counts row follows the
// state rows, and the cleanup runs when a member row goes. The backfill hook
// is left out: its goroutine would write state rows at a time the test does
// not control.
func newInboxStateTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newInboxGroupTestApp(t)
	registerThreadStateSyncHooks(app)
	registerFolderCountHooks(app)
	registerMemberAccessCleanup(app)
	return app
}

func addInboxPerson(t *testing.T, app core.App, mailbox, user *core.Record, role string) *core.Record {
	t.Helper()
	return saveRealRecord(t, app, "mail_mailbox_members", map[string]any{
		"mailbox": mailbox.Id, "user": user.Id, "role": role,
	})
}

// deliverToInbox runs one inbound message through the delivery path and
// returns the thread it went to.
func deliverToInbox(t *testing.T, app core.App, mailbox *core.Record, id string) *core.Record {
	t.Helper()
	msg := &InboundMessage{
		From:      Recipient{Email: "sender@sender.example"},
		To:        []Recipient{{Email: mailbox.GetString("address") + "@inbox.example"}},
		Subject:   "Message " + id,
		TextBody:  "body",
		MessageID: "<" + id + "@sender.example>",
		Date:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := processInboundForMailbox(app, mailbox, msg, ""); err != nil {
		t.Fatalf("deliver %s: %v", id, err)
	}
	message, err := app.FindFirstRecordByFilter("mail_messages", "message_id = {:id}",
		map[string]any{"id": msg.MessageID})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := app.FindRecordById("mail_threads", message.GetString("thread"))
	if err != nil {
		t.Fatal(err)
	}
	return thread
}

func inboxStateCount(t *testing.T, app core.App, mailbox *core.Record, userID string) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_thread_state", "mailbox = {:m} && user = {:u}", "", 0, 0,
		map[string]any{"m": mailbox.Id, "u": userID})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func hasCountsRow(app core.App, mailbox, user *core.Record) bool {
	_, err := app.FindRecordById("mail_folder_counts", folderCountsID(user.Id, mailbox.Id))
	return err == nil
}

// waitFor polls cond for up to two seconds. The cleanup runs in a goroutine,
// so its result can only be seen by polling for it.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("after 2s: %s", what)
	}
}

func TestInboxGroup_InboundSkipsGrantAndDuplicates(t *testing.T) {
	app := newInboxStateTestApp(t)
	s := seedInboxGroup(t, app)
	addInboxPerson(t, app, s.shared, s.ann, "member")
	saveRealRecord(t, app, "group_members", map[string]any{"group": s.sales.Id, "user": s.owner.Id})
	saveInboxGrant(t, app, s.shared, s.sales, "member")
	saveInboxGrant(t, app, s.shared, seedGroup(t, app, "Empty"), "member")

	members, err := getMailboxMembers(app, s.shared.Id)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	for _, m := range members {
		user := m.GetString("user")
		if _, seen := roles[user]; seen {
			t.Errorf("getMailboxMembers returns user %q twice", user)
		}
		roles[user] = m.GetString("role")
	}
	want := map[string]string{s.owner.Id: "owner", s.member.Id: "member", s.ann.Id: "member", s.bob.Id: "member"}
	if len(roles) != len(want) {
		t.Errorf("getMailboxMembers users = %v, want %v", roles, want)
	}
	for user, role := range want {
		if roles[user] != role {
			t.Errorf("getMailboxMembers role of %s = %q, want %q (the highest of the person's rows)", user, roles[user], role)
		}
	}

	thread := deliverToInbox(t, app, s.shared, "inbound-1")
	for _, user := range []*core.Record{s.owner, s.member, s.ann, s.bob} {
		if n := inboxStateCount(t, app, s.shared, user.Id); n != 1 {
			t.Errorf("%s has %d state rows, want 1", user.GetString("username"), n)
		}
	}
	if n := inboxStateCount(t, app, s.shared, ""); n != 0 {
		t.Errorf("%d state rows with no user, want 0", n)
	}
	if state := findThreadState(app, thread.Id, s.ann.Id); state == nil || state.GetString("folder") != "inbox" {
		t.Errorf("ann's state row = %v, want one in inbox", state)
	}
}

func TestInboxGroup_MessageOwnersOnce(t *testing.T) {
	app := newInboxStateTestApp(t)
	s := seedInboxGroup(t, app)
	addInboxPerson(t, app, s.shared, s.ann, "member")
	saveInboxGrant(t, app, s.shared, s.sales, "member")
	thread := deliverToInbox(t, app, s.shared, "owners-1")

	message, err := app.FindFirstRecordByFilter("mail_messages", "thread = {:t}", map[string]any{"t": thread.Id})
	if err != nil {
		t.Fatal(err)
	}
	owners := messageOwnerResolver(app, message)
	seen := map[string]bool{}
	for _, id := range owners {
		if id == "" || seen[id] {
			t.Errorf("messageOwnerResolver = %v, want each person once and no empty id", owners)
			break
		}
		seen[id] = true
	}
	if len(seen) != 4 {
		t.Errorf("messageOwnerResolver gives %d people, want 4 (olga, mike, ann, bob)", len(seen))
	}
}

func TestInboxGroup_BackfillIgnoresGrantRow(t *testing.T) {
	app := newInboxStateTestApp(t)
	s := seedInboxGroup(t, app)
	deliverToInbox(t, app, s.shared, "backfill-1")

	if err := backfillMemberThreadState(context.Background(), app, s.shared.Id, ""); err != nil {
		t.Fatalf("backfill for a grant row = %v, want nil", err)
	}
	if n := inboxStateCount(t, app, s.shared, ""); n != 0 {
		t.Fatalf("%d state rows with no user, want 0", n)
	}
}

func TestInboxGroup_LeaveKeepsDirectAccess(t *testing.T) {
	app := newInboxStateTestApp(t)
	s := seedInboxGroup(t, app)
	addInboxPerson(t, app, s.shared, s.ann, "member")
	saveInboxGrant(t, app, s.shared, s.sales, "member")
	deliverToInbox(t, app, s.shared, "keep-1")

	if err := app.Delete(membershipOf(t, app, s.sales, s.ann)); err != nil {
		t.Fatal(err)
	}
	has, err := userHasMailboxRow(app, s.shared.Id, s.ann.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("userHasMailboxRow = false for ann, who keeps her direct row")
	}
	// The cleanup checks again before it deletes, so a cleanup that starts
	// after access came back leaves the rows alone.
	if err := cleanupLostMailboxAccess(context.Background(), app, s.shared.Id, s.ann.Id); err != nil {
		t.Fatal(err)
	}
	if n := inboxStateCount(t, app, s.shared, s.ann.Id); n != 1 {
		t.Errorf("ann has %d state rows after she leaves Sales, want 1", n)
	}
	if !hasCountsRow(app, s.shared, s.ann) {
		t.Error("ann's counts row was deleted")
	}
}

func TestInboxGroup_LastRowGoneRemovesState(t *testing.T) {
	app := newInboxStateTestApp(t)
	s := seedInboxGroup(t, app)
	saveInboxGrant(t, app, s.shared, s.sales, "member")
	addInboxPerson(t, app, s.personal, s.bob, "member")
	deliverToInbox(t, app, s.shared, "gone-1")
	deliverToInbox(t, app, s.personal, "gone-2")
	if n := inboxStateCount(t, app, s.shared, s.bob.Id); n != 1 {
		t.Fatalf("bob has %d state rows before he leaves, want 1", n)
	}

	if err := app.Delete(membershipOf(t, app, s.sales, s.bob)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob keeps his state for the inbox he left", func() bool {
		return inboxStateCount(t, app, s.shared, s.bob.Id) == 0 && !hasCountsRow(app, s.shared, s.bob)
	})
	if n := inboxStateCount(t, app, s.personal, s.bob.Id); n != 1 {
		t.Errorf("bob has %d state rows on the inbox he still uses, want 1", n)
	}
	if !hasCountsRow(app, s.personal, s.bob) {
		t.Error("bob lost the counts row of the inbox he still uses")
	}
	if n := inboxStateCount(t, app, s.shared, s.ann.Id); n != 1 {
		t.Errorf("ann, still in Sales, has %d state rows, want 1", n)
	}
}

// The cleanup and the backfill for one person must not interleave. The
// cleanup lists the person's state rows, and the person gets access again
// while it deletes them. A backfill that ran beside it would skip every
// thread whose old row still exists, and the cleanup would then delete those
// rows: the person would have access and no state. The test holds the
// cleanup after its first delete, adds the person back, and checks that the
// backfill waits and then gives the person every row.
func TestInboxGroup_CleanupAndBackfillDoNotInterleave(t *testing.T) {
	app := newInboxStateTestApp(t)
	registerMemberBackfillHooks(app)
	s := seedInboxGroup(t, app)
	direct := addInboxPerson(t, app, s.shared, s.ann, "member")
	const threads = 3
	for i := range threads {
		deliverToInbox(t, app, s.shared, "interleave-"+string(rune('a'+i)))
	}
	if n := inboxStateCount(t, app, s.shared, s.ann.Id); n != threads {
		t.Fatalf("ann has %d state rows before the cleanup, want %d", n, threads)
	}

	cleanupDeleting := make(chan struct{})
	gate := make(chan struct{})
	var holdOnce, wroteOnce sync.Once
	backfillWrote := make(chan struct{})
	app.OnRecordAfterDeleteSuccess("mail_thread_state").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("user") == s.ann.Id {
			holdOnce.Do(func() {
				close(cleanupDeleting)
				<-gate
			})
		}
		return e.Next()
	})
	app.OnRecordAfterCreateSuccess("mail_thread_state").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("user") == s.ann.Id {
			wroteOnce.Do(func() { close(backfillWrote) })
		}
		return e.Next()
	})

	if err := app.Delete(direct); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cleanupDeleting:
	case <-time.After(2 * time.Second):
		t.Fatal("the cleanup did not delete a row in 2s")
	}
	addInboxPerson(t, app, s.shared, s.ann, "member")

	// The cleanup holds ann's key, so her backfill must not write yet.
	select {
	case <-backfillWrote:
		t.Error("the backfill wrote a state row while the cleanup for the same person ran")
	case <-time.After(300 * time.Millisecond):
	}
	close(gate)

	waitFor(t, "ann has access again but not every state row", func() bool {
		return inboxStateCount(t, app, s.shared, s.ann.Id) == threads && hasCountsRow(app, s.shared, s.ann)
	})
}

func TestInboxGroup_GroupDeleteCascades(t *testing.T) {
	app := newInboxStateTestApp(t)
	s := seedInboxGroup(t, app)
	saveInboxGrant(t, app, s.shared, s.sales, "member")
	deliverToInbox(t, app, s.shared, "cascade-1")

	if err := app.Delete(reloaded(t, app, s.sales)); err != nil {
		t.Fatal(err)
	}
	rows, err := app.FindRecordsByFilter("mail_mailbox_members", "group = {:g}", "", 0, 0,
		map[string]any{"g": s.sales.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("%d member rows of the deleted group remain, want 0", len(rows))
	}
	waitFor(t, "bob keeps his state after his only group is deleted", func() bool {
		return inboxStateCount(t, app, s.shared, s.bob.Id) == 0 && !hasCountsRow(app, s.shared, s.bob)
	})
	if n := inboxStateCount(t, app, s.shared, s.member.Id); n != 1 {
		t.Errorf("mike, a direct member, has %d state rows, want 1", n)
	}
}

func TestInboxGroup_SearchAndIMAPListOnce(t *testing.T) {
	app := newInboxStateTestApp(t)
	s := seedInboxGroup(t, app)
	addInboxPerson(t, app, s.shared, s.ann, "member")
	saveInboxGrant(t, app, s.shared, s.sales, "member")

	ids, err := getUserMailboxIDs(app, s.ann.Id, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != s.shared.Id {
		t.Errorf("getUserMailboxIDs = %v, want [%s]", ids, s.shared.Id)
	}

	session := newIMAPSession(app)
	if err := session.Login(s.ann.Email(), "Password123!"); err != nil {
		t.Fatal(err)
	}
	if len(session.mailboxIndex) != 1 || session.mailboxIndex[0].mailboxID != s.shared.Id {
		t.Errorf("IMAP mailbox index = %v, want the shared inbox once", session.mailboxIndex)
	}
	if session.multiMailbox {
		t.Error("multiMailbox = true for a person with one inbox")
	}
}

func TestInboxGroup_OffboardIgnoresGrantRow(t *testing.T) {
	app := newInboxStateTestApp(t)
	s := seedInboxGroup(t, app)
	solo := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "solo", "domain": s.shared.GetString("domain"), "type": "shared", "name": "Solo", "display_name": "Solo",
	})
	addInboxPerson(t, app, solo, s.owner, "owner")
	saveInboxGrant(t, app, solo, seedGroup(t, app, "Empty"), "member")

	owned, err := findSoleOwnedSharedMailboxes(app, s.owner.Id)
	if err != nil {
		t.Fatal(err)
	}
	var found *soleOwnedMailbox
	for i := range owned {
		if owned[i].ID == solo.Id {
			found = &owned[i]
		}
	}
	if found == nil {
		t.Fatalf("findSoleOwnedSharedMailboxes = %v, want the inbox Solo", owned)
	}
	if found.OtherMembers != 0 {
		t.Errorf("Solo OtherMembers = %d, want 0: a grant with no people is no other member", found.OtherMembers)
	}

	// A self-delete with no heir refuses only for an inbox other people use.
	// Shared still has mike, so it is the only reason to refuse here; without
	// it the grant alone must not refuse.
	if err := app.Delete(reloadedMember(t, app, s.shared, s.member)); err != nil {
		t.Fatal(err)
	}
	if err := handOverSoleOwnedMailboxes(app, s.owner, offboard.Plan{Mode: offboard.ModeDeleteMyData}, s.owner.Id); err != nil {
		t.Errorf("handOverSoleOwnedMailboxes = %v, want nil: nobody else uses Solo or Shared", err)
	}
}

func TestInboxGroup_HeirWithGroupAccessGetsDirectOwnerRow(t *testing.T) {
	app := newInboxStateTestApp(t)
	s := seedInboxGroup(t, app)
	saveInboxGrant(t, app, s.shared, s.sales, "member")

	if err := makeMailboxOwner(app, s.shared.Id, s.bob.Id); err != nil {
		t.Fatal(err)
	}
	direct, err := app.FindFirstRecordByFilter("mail_mailbox_members", `mailbox = {:m} && user = {:u} && group = ""`,
		map[string]any{"m": s.shared.Id, "u": s.bob.Id})
	if err != nil {
		t.Fatalf("bob has no direct row after makeMailboxOwner: %v", err)
	}
	if direct.GetString("role") != "owner" {
		t.Errorf("bob's direct role = %q, want owner", direct.GetString("role"))
	}
	if row := derivedRow(t, app, s.shared, s.sales, s.bob); row == nil || row.GetString("role") != "member" {
		t.Errorf("bob's derived row = %v, want it kept at role member", row)
	}

	// A second call finds the direct owner row and changes nothing.
	if err := makeMailboxOwner(app, s.shared.Id, s.bob.Id); err != nil {
		t.Fatal(err)
	}
}

func reloadedMember(t *testing.T, app core.App, mailbox, user *core.Record) *core.Record {
	t.Helper()
	row, err := app.FindFirstRecordByFilter("mail_mailbox_members", `mailbox = {:m} && user = {:u} && group = ""`,
		map[string]any{"m": mailbox.Id, "u": user.Id})
	if err != nil {
		t.Fatal(err)
	}
	return row
}
