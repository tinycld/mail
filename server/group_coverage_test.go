package mail

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/groups"
	"tinycld.org/core/offboard"
)

// seedGroup makes a group with the given members.
func seedGroup(t testing.TB, app core.App, name string, members ...*core.Record) *core.Record {
	t.Helper()
	group := saveRealRecord(t, app, "groups", map[string]any{"name": name})
	for _, member := range members {
		saveRealRecord(t, app, "group_members", map[string]any{"group": group.Id, "user": member.Id})
	}
	return group
}

func seedGroupUser(t testing.TB, app core.App, username string) *core.Record {
	t.Helper()
	return saveRealRecord(t, app, "users", map[string]any{
		"email": username + "@ext.example", "username": username, "name": username, "role": "member",
	})
}

// ruleRow returns the rule row for local on the domain, or nil.
func ruleRow(t *testing.T, app core.App, local, domainID string) *core.Record {
	t.Helper()
	row, err := findAddressRow(app, local, domainID)
	if err != nil {
		t.Fatal(err)
	}
	if row == nil || row.GetString("source") != addressSourceRule {
		return nil
	}
	return row
}

func saveGroupsDomain(t testing.TB, app core.App, name string, groups ...*core.Record) *core.Record {
	t.Helper()
	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = g.Id
	}
	return saveRealRecord(t, app, "mail_domains", map[string]any{
		"domain": name, "verified": true, "coverage": coverageGroups, "coverage_groups": ids,
	})
}

func TestGroupCoverage_OnlyMembersGetAddresses(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	seedGroupUser(t, app, "bob")
	sales := seedGroup(t, app, "Sales", ann)
	q := saveGroupsDomain(t, app, "q.example", sales)

	row := ruleRow(t, app, "ann", q.Id)
	if row == nil || row.GetString("mailbox") != identityOf(t, app, ann.Id).GetString("inbox") {
		t.Fatalf("ann@q.example = %v, want a rule row to ann's inbox", row)
	}
	if row.GetString("via_group") != sales.Id {
		t.Fatalf("via_group = %q, want sales", row.GetString("via_group"))
	}
	if got := ruleRows(t, app, q.Id); len(got) != 1 {
		t.Fatalf("rule rows on q.example = %v, want ann only", got)
	}
}

func TestGroupCoverage_TwoGroupsViaGroupFollowsOrder(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	sales := seedGroup(t, app, "Sales", ann)
	support := seedGroup(t, app, "Support", ann)
	q := saveGroupsDomain(t, app, "q.example", sales, support)

	before := ruleRow(t, app, "ann", q.Id)
	if before == nil || before.GetString("via_group") != sales.Id {
		t.Fatalf("ann@q.example = %v, want via_group sales", before)
	}

	membership, err := app.FindFirstRecordByFilter("group_members", "group = {:g} && user = {:u}",
		map[string]any{"g": sales.Id, "u": ann.Id})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(membership); err != nil {
		t.Fatal(err)
	}
	if err := reconcileRuleAddresses(app, ruleScope{UserID: ann.Id}); err != nil {
		t.Fatal(err)
	}
	after := ruleRow(t, app, "ann", q.Id)
	if after == nil || after.GetString("via_group") != support.Id {
		t.Fatalf("ann@q.example after leaving sales = %v, want via_group support", after)
	}
	if after.Id != before.Id {
		t.Fatalf("the rule row was made again (id %s, was %s); the move must keep the row", after.Id, before.Id)
	}
}

// Switching a groups domain to everyone keeps the members' rows and drops
// their group; switching it on to manual removes every rule row.
func TestGroupCoverage_SwitchToEveryoneThenManual(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	seedGroupUser(t, app, "bob")
	q := saveGroupsDomain(t, app, "q.example", seedGroup(t, app, "Sales", ann))
	before := ruleRow(t, app, "ann", q.Id)
	if before == nil {
		t.Fatal("precondition: ann has an address on q.example")
	}

	q = reloaded(t, app, q)
	q.Set("coverage", coverageEveryone)
	if err := app.Save(q); err != nil {
		t.Fatal(err)
	}
	row := ruleRow(t, app, "ann", q.Id)
	if row == nil || row.Id != before.Id || row.GetString("via_group") != "" {
		t.Fatalf("ann@q.example on an everyone domain = %v, want row %s kept with no via_group", row, before.Id)
	}
	if row := ruleRow(t, app, "bob", q.Id); row == nil || row.GetString("via_group") != "" {
		t.Fatalf("bob@q.example on an everyone domain = %v, want a rule row with no via_group", row)
	}

	q = reloaded(t, app, q)
	q.Set("coverage", coverageManual)
	if err := app.Save(q); err != nil {
		t.Fatal(err)
	}
	if got := ruleRows(t, app, q.Id); len(got) != 0 {
		t.Fatalf("rule rows on a manual domain: %v", got)
	}
}

func TestGroupCoverage_ReservedForEveryone(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	seedGroupUser(t, app, "bob")
	q := saveGroupsDomain(t, app, "q.example", seedGroup(t, app, "Sales", ann))
	desk := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "desk", "domain": q.Id, "type": "shared", "display_name": "Desk", "name": "Desk"})

	row := core.NewRecord(mustCollection(t, app, "mail_addresses"))
	row.Set("local_part", "bob")
	row.Set("domain", q.Id)
	row.Set("mailbox", desk.Id)
	row.Set("source", addressSourceManual)
	err := app.Save(row)
	var refusal validation.Errors
	if !errors.As(err, &refusal) || refusal["local_part"] == nil {
		t.Fatalf("bob@q.example for a shared inbox: want a local_part refusal, got %v", err)
	}
}

func TestGroupCoverage_RequiresAGroup(t *testing.T) {
	runRuleAPIScenario(t, http.MethodPatch, []string{`"coverage_groups":{"code":"coverage_groups_required"`},
		func(s *ruleAPISeed) string { return "/api/collections/mail_domains/records/" + s.main.Id },
		func(*ruleAPISeed) string { return `{"coverage":"groups","coverage_groups":[]}` })
}

func TestGroupCoverage_TooManyGroups(t *testing.T) {
	ids := make([]string, maxCoverageGroups+1)
	for i := range ids {
		ids[i] = fmt.Sprintf(`"g%014d"`, i)
	}
	runRuleAPIScenario(t, http.MethodPatch, []string{`"coverage_groups":{"code":"coverage_groups_too_many"`},
		func(s *ruleAPISeed) string { return "/api/collections/mail_domains/records/" + s.main.Id },
		func(*ruleAPISeed) string {
			return `{"coverage":"groups","coverage_groups":[` + strings.Join(ids, ",") + `]}`
		})
}

func TestGroupCoverage_ChangingGroupsReconciles(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	carol := seedGroupUser(t, app, "carol")
	sales := seedGroup(t, app, "Sales", ann)
	support := seedGroup(t, app, "Support", carol)
	q := saveGroupsDomain(t, app, "q.example", sales)

	q = reloaded(t, app, q)
	q.Set("coverage_groups", []string{support.Id})
	if err := app.Save(q); err != nil {
		t.Fatal(err)
	}
	if row := ruleRow(t, app, "ann", q.Id); row != nil {
		t.Fatal("ann kept her address after sales was taken off the domain")
	}
	if row := ruleRow(t, app, "carol", q.Id); row == nil || row.GetString("via_group") != support.Id {
		t.Fatalf("carol@q.example = %v, want a rule row via support", row)
	}
}

// The name is reserved for every person, so a switch to groups is refused
// when any person's name is taken, member or not (spec 5.4).
func TestGroupCoverage_ConflictCheckCoversAllPeople(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	seedGroupUser(t, app, "bob")
	sales := seedGroup(t, app, "Sales", ann)
	other := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "other.example", "verified": true})
	saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "bob", "domain": other.Id, "type": "shared", "display_name": "Bob's desk", "name": "Bob's desk"})

	other = reloaded(t, app, other)
	other.Set("coverage", coverageGroups)
	other.Set("coverage_groups", []string{sales.Id})
	err := app.Save(other)
	if err == nil || !strings.Contains(err.Error(), "bob@other.example") {
		t.Fatalf("want a refusal naming bob@other.example, got %v", err)
	}
}

// Deleting a domain's last group makes PocketBase clear coverage_groups with
// a plain save. That save must go through, and nobody keeps an address.
func TestGroupCoverage_DeletingTheLastGroup(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	sales := seedGroup(t, app, "Sales", ann)
	q := saveGroupsDomain(t, app, "q.example", sales)
	if ruleRow(t, app, "ann", q.Id) == nil {
		t.Fatal("precondition: ann has no address on q.example")
	}

	if err := app.Delete(reloaded(t, app, sales)); err != nil {
		t.Fatalf("deleting the domain's last group: %v", err)
	}
	q = reloaded(t, app, q)
	if q.GetString("coverage") != coverageGroups || len(q.GetStringSlice("coverage_groups")) != 0 {
		t.Fatalf("domain after group delete: coverage %q, groups %v", q.GetString("coverage"), q.GetStringSlice("coverage_groups"))
	}
	if got := ruleRows(t, app, q.Id); len(got) != 0 {
		t.Fatalf("rule rows left after the last group was deleted: %v", got)
	}
	if err := reconcileRuleAddresses(app, ruleScope{}); err != nil {
		t.Fatalf("a full reconcile with an empty group list: %v", err)
	}
}

func TestGroupMembers_OrderFollowsGroupIDs(t *testing.T) {
	app := newRuleTestApp(t)
	ann := seedGroupUser(t, app, "ann")
	bob := seedGroupUser(t, app, "bob")
	sales := seedGroup(t, app, "Sales", ann)
	support := seedGroup(t, app, "Support", ann, bob)
	seedGroup(t, app, "Other", bob)

	got, err := groupMembers(app, []string{support.Id, sales.Id})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{support.Id, sales.Id}; strings.Join(got[ann.Id], ",") != strings.Join(want, ",") {
		t.Fatalf("ann's groups = %v, want %v", got[ann.Id], want)
	}
	if strings.Join(got[bob.Id], ",") != support.Id || len(got) != 2 {
		t.Fatalf("members = %v, want ann and bob (support only)", got)
	}
	if empty, err := groupMembers(app, nil); err != nil || len(empty) != 0 {
		t.Fatalf("no groups: %v, %v", empty, err)
	}
}

// An address of a shared inbox on a groups domain blocks that email name for
// a new person, as it does on an everyone domain.
func TestGroupCoverage_EmailNameTakenOnGroupsDomain(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	q := saveGroupsDomain(t, app, "q.example", seedGroup(t, app, "Sales", ann))
	saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "desk", "domain": q.Id, "type": "shared", "display_name": "Desk", "name": "Desk"})

	taken, err := emailNameTaken(app, "desk", "")
	if err != nil {
		t.Fatal(err)
	}
	if !taken {
		t.Fatal("desk@q.example is a shared inbox on a groups domain, but the name desk was free")
	}
}

// newMembershipTestApp is the rule test app with core's group hooks and the
// mail membership listener bound, so group joins and leaves reach the
// reconciler as they do in production. The listener registry is process
// global, so it is cleared before and after.
func newMembershipTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newRuleTestApp(t)
	groups.ResetForTesting()
	t.Cleanup(groups.ResetForTesting)
	groups.Register(&pocketbase.PocketBase{App: app})
	bindGroupMembershipListener()
	return app
}

func membershipOf(t *testing.T, app core.App, group, user *core.Record) *core.Record {
	t.Helper()
	m, err := app.FindFirstRecordByFilter("group_members", "group = {:g} && user = {:u}",
		map[string]any{"g": group.Id, "u": user.Id})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestGroupMembership_JoinAndLeave(t *testing.T) {
	app := newMembershipTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	sales := seedGroup(t, app, "Sales")
	q := saveGroupsDomain(t, app, "q.example", sales)
	if ruleRow(t, app, "ann", q.Id) != nil {
		t.Fatal("precondition: ann has an address on q.example before she joins sales")
	}

	saveRealRecord(t, app, "group_members", map[string]any{"group": sales.Id, "user": ann.Id})
	row := ruleRow(t, app, "ann", q.Id)
	if row == nil || row.GetString("via_group") != sales.Id {
		t.Fatalf("ann@q.example after joining sales = %v, want a rule row via sales", row)
	}

	if err := app.Delete(membershipOf(t, app, sales, ann)); err != nil {
		t.Fatal(err)
	}
	if row := ruleRow(t, app, "ann", q.Id); row != nil {
		t.Fatalf("ann@q.example after leaving sales = %v, want no row", row)
	}
}

// The listener is registered once per process, so it must reconcile through
// the app in the event. A listener that held the app it was registered with
// would write to that app and leave a second app with no address.
func TestGroupMembership_UsesTheEventsApp(t *testing.T) {
	first := newMembershipTestApp(t)
	second := newRuleTestApp(t)
	groups.Register(&pocketbase.PocketBase{App: second})
	saveRealRecord(t, second, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, second, "ann")
	sales := seedGroup(t, second, "Sales")
	q := saveGroupsDomain(t, second, "q.example", sales)

	saveRealRecord(t, second, "group_members", map[string]any{"group": sales.Id, "user": ann.Id})
	if row := ruleRow(t, second, "ann", q.Id); row == nil || row.GetString("via_group") != sales.Id {
		t.Fatalf("ann@q.example in the second app = %v, want a rule row via sales", row)
	}
	if n, _ := first.CountRecords("mail_addresses"); n != 0 {
		t.Fatalf("first app has %d addresses, want none", n)
	}
}

func TestGroupMembership_GroupDeleted(t *testing.T) {
	app := newMembershipTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	carol := seedGroupUser(t, app, "carol")
	sales := seedGroup(t, app, "Sales", ann)
	support := seedGroup(t, app, "Support", carol)
	q := saveGroupsDomain(t, app, "q.example", sales, support)
	if ruleRow(t, app, "ann", q.Id) == nil || ruleRow(t, app, "carol", q.Id) == nil {
		t.Fatal("precondition: ann and carol have addresses on q.example")
	}

	if err := app.Delete(reloaded(t, app, sales)); err != nil {
		t.Fatalf("deleting sales: %v", err)
	}
	if row := ruleRow(t, app, "ann", q.Id); row != nil {
		t.Fatalf("ann@q.example after sales was deleted = %v, want no row", row)
	}
	if row := ruleRow(t, app, "carol", q.Id); row == nil || row.GetString("via_group") != support.Id {
		t.Fatalf("carol@q.example after sales was deleted = %v, want a rule row via support", row)
	}
	q = reloaded(t, app, q)
	if got := q.GetStringSlice("coverage_groups"); len(got) != 1 || got[0] != support.Id {
		t.Fatalf("coverage_groups after sales was deleted = %v, want [support]", got)
	}
	if err := app.Save(q); err != nil {
		t.Fatalf("saving the domain after sales was deleted: %v", err)
	}
}

func TestGroupMembership_LastGroupDeleted(t *testing.T) {
	app := newMembershipTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	sales := seedGroup(t, app, "Sales", ann)
	q := reloaded(t, app, saveGroupsDomain(t, app, "q.example", sales))
	if ruleRow(t, app, "ann", q.Id) == nil {
		t.Fatal("precondition: ann has an address on q.example")
	}

	if err := app.Delete(reloaded(t, app, sales)); err != nil {
		t.Fatalf("deleting the domain's last group: %v", err)
	}
	if got := ruleRows(t, app, q.Id); len(got) != 0 {
		t.Fatalf("rule rows on q.example after its last group was deleted: %v", got)
	}
	after := reloaded(t, app, q)
	if got := after.GetStringSlice("coverage_groups"); len(got) != 0 {
		t.Fatalf("coverage_groups after the last group was deleted = %v, want []", got)
	}
	for _, field := range q.Collection().Fields {
		name := field.GetName()
		if name == "coverage_groups" || name == "updated" {
			continue
		}
		if fmt.Sprint(q.Get(name)) != fmt.Sprint(after.Get(name)) {
			t.Fatalf("domain field %s changed from %v to %v", name, q.Get(name), after.Get(name))
		}
	}
}

// Offboarding removes the leaver from every group inside its transaction, so
// the leave events arrive after the identity is retired and the inbox is
// gone. The listener must make nothing and return no error. Core logs a
// listener's error and goes on, so the test binds the listener through a
// recorder that keeps each event and what the listener returned.
func TestGroupMembership_OffboardCreatesNothing(t *testing.T) {
	app := newRegisteredMailApp(t)
	groups.ResetForTesting()
	t.Cleanup(groups.ResetForTesting)
	groups.Register(&pocketbase.PocketBase{App: app})
	var passed []groups.MembershipEvent
	groups.OnMembershipChange(func(e groups.MembershipEvent) error {
		err := onGroupMembershipChange(e)
		if err != nil {
			t.Errorf("mail listener for %+v: %v", e, err)
		} else {
			passed = append(passed, e)
		}
		return err
	})

	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	admin := saveRealRecord(t, app, "users", map[string]any{"email": "ada@ext.example", "username": "ada", "name": "Ada", "role": "admin"})
	ann := seedGroupUser(t, app, "ann")
	sales := seedGroup(t, app, "Sales", ann)
	q := saveGroupsDomain(t, app, "q.example", sales)
	if ruleRow(t, app, "ann", q.Id) == nil {
		t.Fatal("precondition: ann has an address on q.example")
	}
	passed = nil

	if _, err := offboard.OffboardUser(app, ann.Id, offboard.Plan{Mode: offboard.ModeDeleteMyData}, admin.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	requireNoMailFor(t, app, ann, "ann", "after offboard")
	if identity := identityOf(t, app, ann.Id); identity == nil || !identity.GetBool("retired") {
		t.Fatalf("identity after offboard = %v, want retired", identity)
	}
	if len(passed) != 1 || passed[0].Joined || passed[0].UserID != ann.Id || passed[0].GroupID != sales.Id {
		t.Fatalf("events that passed the mail listener = %+v, want ann's one leave from sales", passed)
	}
}

// Only the reconciler sets via_group. An admin who sends it on a hand-made
// address through the API has it cleared, as source and is_primary are;
// otherwise the address would later say it came from a group it did not.
func TestGroupCoverage_APICreateClearsViaGroup(t *testing.T) {
	var seed *ruleAPISeed
	var sales *core.Record
	scenario := tests.ApiScenario{
		Name:           "POST mail_addresses with via_group",
		Method:         http.MethodPost,
		URL:            "/api/collections/mail_addresses/records",
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"local_part":"desk"`,
			`"via_group":""`,
			`"source":"manual"`,
		},
		Headers: map[string]string{"Content-Type": "application/json"},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		app := newRuleTestApp(t)
		seed = seedRuleAPI(t, app)
		sales = seedGroup(t, app, "Sales")
		scenario.Body = strings.NewReader(`{"local_part":"desk","domain":"` + seed.other.Id +
			`","mailbox":"` + seed.mailbx.Id + `","via_group":"` + sales.Id + `"}`)
		return app
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		scenario.Headers["Authorization"] = authTokenFor(t, seed.admin)
	}
	scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
		row, err := findAddressRow(app, "desk", seed.other.Id)
		if err != nil || row == nil {
			t.Fatalf("desk@other.example was not saved: %v", err)
		}
		if row.GetString("via_group") != "" {
			t.Fatalf("via_group = %q, want it cleared on an API write", row.GetString("via_group"))
		}
	}
	scenario.Test(t)
}

// runGroupsDomainPatch PATCHes a groups domain as an admin. The domain's one
// group is deleted first when dropGroup is true, as an admin does in the
// groups settings, which leaves coverage=groups with no group.
func runGroupsDomainPatch(t *testing.T, body string, dropGroup bool, status int, expect []string) {
	t.Helper()
	var seed *ruleAPISeed
	scenario := tests.ApiScenario{
		Name:            "PATCH groups domain " + body,
		Method:          http.MethodPatch,
		Body:            strings.NewReader(body),
		ExpectedStatus:  status,
		ExpectedContent: expect,
		Headers:         map[string]string{"Content-Type": "application/json"},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		app := newRuleTestApp(t)
		seed = seedRuleAPI(t, app)
		sales := seedGroup(t, app, "Sales", seed.sam)
		q := saveGroupsDomain(t, app, "q.example", sales)
		if dropGroup {
			if err := app.Delete(reloaded(t, app, sales)); err != nil {
				t.Fatal(err)
			}
		}
		scenario.URL = "/api/collections/mail_domains/records/" + q.Id
		return app
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		scenario.Headers["Authorization"] = authTokenFor(t, seed.admin)
	}
	scenario.Test(t)
}

// A groups domain whose last group was deleted must still save when the
// group list does not change, or reordering the domain list fails for all.
func TestGroupCoverage_NoGroupLeftStillSavesOtherFields(t *testing.T) {
	runGroupsDomainPatch(t, `{"rank":7}`, true, http.StatusOK, []string{`"rank":7`})
}

func TestGroupCoverage_ClearingTheGroupsIsRefused(t *testing.T) {
	runGroupsDomainPatch(t, `{"coverage_groups":[]}`, false, http.StatusBadRequest,
		[]string{`"coverage_groups":{"code":"coverage_groups_required"`})
}

// A leave from one of two covering groups reaches the reconciler through the
// listener alone, and the row moves to the other group in place.
func TestGroupMembership_LeaveOneOfTwoGroupsKeepsTheRow(t *testing.T) {
	app := newMembershipTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	ann := seedGroupUser(t, app, "ann")
	sales := seedGroup(t, app, "Sales", ann)
	support := seedGroup(t, app, "Support", ann)
	q := saveGroupsDomain(t, app, "q.example", sales, support)
	before := ruleRow(t, app, "ann", q.Id)
	if before == nil || before.GetString("via_group") != sales.Id {
		t.Fatalf("ann@q.example = %v, want via_group sales", before)
	}

	if err := app.Delete(membershipOf(t, app, sales, ann)); err != nil {
		t.Fatal(err)
	}
	after := ruleRow(t, app, "ann", q.Id)
	if after == nil || after.GetString("via_group") != support.Id {
		t.Fatalf("ann@q.example after leaving sales = %v, want via_group support", after)
	}
	if after.Id != before.Id {
		t.Fatalf("the rule row was made again (id %s, was %s); the move must keep the row", after.Id, before.Id)
	}
}
