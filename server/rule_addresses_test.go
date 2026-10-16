package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func newRuleTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newRealSchemaApp(t)
	registerAddressHooks(app)
	registerIdentityHooks(app)
	registerRuleAddressHooks(app)
	return app
}

// reloaded returns the record as loaded from the database, so hooks see its
// Original() values (saveRealRecord's result has none).
func reloaded(t *testing.T, app core.App, r *core.Record) *core.Record {
	t.Helper()
	fresh, err := app.FindRecordById(r.Collection().Name, r.Id)
	if err != nil {
		t.Fatal(err)
	}
	return fresh
}

func ruleRows(t *testing.T, app core.App, domainID string) map[string]string {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_addresses", "source = 'rule' && domain = {:d}", "", 0, 0, map[string]any{"d": domainID})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, r := range rows {
		out[r.GetString("local_part")] = r.GetString("mailbox")
	}
	return out
}

// The argosity case: tinycld.org gives everyone an address, tinycld.com was
// added later. Switching .com to everyone gives each person name@tinycld.com.
func TestRuleAddresses_SwitchingADomainToEveryone(t *testing.T) {
	app := newRuleTestApp(t)
	org := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})
	nathan := saveRealRecord(t, app, "users", map[string]any{"email": "nathan@argosity.com", "username": "nathan", "name": "Nathan", "role": "owner"})
	nas := saveRealRecord(t, app, "users", map[string]any{"email": "nas@stitt.org", "username": "nas", "name": "Nas", "role": "admin"})
	com := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.com", "verified": true})
	if com.GetString("coverage") != coverageManual {
		t.Fatalf("a second domain must start as manual, got %q", com.GetString("coverage"))
	}
	if got := ruleRows(t, app, com.Id); len(got) != 0 {
		t.Fatalf("a manual domain got rule addresses: %v", got)
	}

	com = reloaded(t, app, com)
	com.Set("coverage", coverageEveryone)
	if err := app.Save(com); err != nil {
		t.Fatal(err)
	}
	got := ruleRows(t, app, com.Id)
	nathanInbox := identityOf(t, app, nathan.Id).GetString("inbox")
	nasInbox := identityOf(t, app, nas.Id).GetString("inbox")
	if got["nathan"] != nathanInbox || got["nas"] != nasInbox || len(got) != 2 {
		t.Fatalf("rule rows on tinycld.com = %v", got)
	}
	// The main domain's address is the inbox's primary row, so no rule row
	// duplicates it.
	if extra := ruleRows(t, app, org.Id); len(extra) != 0 {
		t.Fatalf("rule rows duplicated primary addresses on tinycld.org: %v", extra)
	}

	com = reloaded(t, app, com)
	com.Set("coverage", coverageManual)
	if err := app.Save(com); err != nil {
		t.Fatal(err)
	}
	if got := ruleRows(t, app, com.Id); len(got) != 0 {
		t.Fatalf("switching back to manual left rule rows: %v", got)
	}
}

// Rule rows depend on coverage only. A failed re-verify must not delete the
// addresses that messages and identities link to.
func TestRuleAddresses_VerificationDoesNotGateRules(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	saveRealRecord(t, app, "users", map[string]any{"email": "a@ext.example", "username": "ann", "name": "Ann", "role": "member"})
	pending := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "pending.example", "verified": false, "coverage": coverageEveryone})
	if got := ruleRows(t, app, pending.Id); got["ann"] == "" {
		t.Fatalf("an unverified everyone domain gave no rule address: %v", got)
	}

	pending = reloaded(t, app, pending)
	pending.Set("verified", true)
	if err := app.Save(pending); err != nil {
		t.Fatal(err)
	}
	pending = reloaded(t, app, pending)
	pending.Set("verified", false)
	if err := app.Save(pending); err != nil {
		t.Fatal(err)
	}
	if got := ruleRows(t, app, pending.Id); got["ann"] == "" {
		t.Fatalf("losing verification deleted the rule rows: %v", got)
	}
}

func TestRuleAddresses_ManualRowOfSameInboxSurvivesRuleRemoval(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	user := saveRealRecord(t, app, "users", map[string]any{"email": "n@ext.example", "username": "nina", "name": "Nina", "role": "member"})
	com := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "com.example", "verified": true})
	identity := identityOf(t, app, user.Id)
	saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": identity.GetString("email_name"), "domain": com.Id,
		"mailbox": identity.GetString("inbox"), "source": addressSourceManual,
	})

	for _, coverage := range []string{coverageEveryone, coverageManual} {
		com = reloaded(t, app, com)
		com.Set("coverage", coverage)
		if err := app.Save(com); err != nil {
			t.Fatal(err)
		}
	}
	row, _ := findAddressRow(app, identity.GetString("email_name"), com.Id)
	if row == nil || row.GetString("source") != addressSourceManual {
		t.Fatal("a manual address was lost or rewritten by switching the rule on and off")
	}
}

func TestRuleAddresses_AddressHeldByAnotherInboxIsLeftAlone(t *testing.T) {
	app := newRuleTestApp(t)
	logs := captureLogs(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	user := saveRealRecord(t, app, "users", map[string]any{"email": "s@ext.example", "username": "sam", "name": "Sam", "role": "member"})
	other := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "other.example", "verified": true})
	desk := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "sam", "domain": other.Id, "type": "shared", "display_name": "Desk", "name": "Desk"})
	// Raw SQL skips the coverage guard, standing in for drift.
	if _, err := app.DB().NewQuery("UPDATE mail_domains SET coverage = 'everyone' WHERE id = {:id}").Bind(map[string]any{"id": other.Id}).Execute(); err != nil {
		t.Fatal(err)
	}
	if err := reconcileRuleAddresses(app, ruleScope{}); err != nil {
		t.Fatal(err)
	}
	row, _ := findAddressRow(app, "sam", other.Id)
	if row == nil || row.GetString("mailbox") != desk.Id || row.GetString("source") == addressSourceRule {
		t.Fatal("the reconciler overwrote an address held by another inbox")
	}
	if !strings.Contains(logs.String(), "held by another inbox") {
		t.Fatalf("the conflict was not logged: %s", logs.String())
	}
	_ = user
}

// A user created before any domain was verified gets an identity and
// addresses when the first domain is verified.
func TestRuleAddresses_FirstDomainVerifiedLater(t *testing.T) {
	app := newRuleTestApp(t)
	early := saveRealRecord(t, app, "users", map[string]any{"email": "e@ext.example", "username": "early", "name": "Early", "role": "member"})
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "first.example", "verified": false})
	if identityOf(t, app, early.Id) != nil {
		t.Fatal("an identity was made with no verified domain")
	}
	domain = reloaded(t, app, domain)
	domain.Set("verified", true)
	if err := app.Save(domain); err != nil {
		t.Fatal(err)
	}
	identity := identityOf(t, app, early.Id)
	if identity == nil {
		t.Fatal("verifying the first domain gave the early user no identity")
	}
	if row, _ := findAddressRow(app, "early", domain.Id); row == nil || row.GetString("mailbox") != identity.GetString("inbox") {
		t.Fatal("the early user has no address on the first domain")
	}
}

func TestRuleAddresses_GuestDemotionRemovesThem(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	extra := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "extra.example", "verified": true, "coverage": coverageEveryone})
	user := saveRealRecord(t, app, "users", map[string]any{"email": "d@ext.example", "username": "dan", "name": "Dan", "role": "member"})
	if got := ruleRows(t, app, extra.Id); got["dan"] == "" {
		t.Fatalf("no rule address before demotion: %v", got)
	}
	user = reloaded(t, app, user)
	user.Set("role", "guest")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	if got := ruleRows(t, app, extra.Id); got["dan"] != "" {
		t.Fatal("a member demoted to guest kept a rule address")
	}
}

func TestCoverageConflict_IsRefusedAndNamed(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	saveRealRecord(t, app, "users", map[string]any{"email": "s@ext.example", "username": "sam", "name": "Sam", "role": "member"})
	other := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "other.example", "verified": true})
	saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "sam", "domain": other.Id, "type": "shared", "display_name": "Sam's desk", "name": "Sam's desk"})

	other = reloaded(t, app, other)
	other.Set("coverage", coverageEveryone)
	err := app.Save(other)
	if err == nil || !strings.Contains(err.Error(), "sam@other.example") {
		t.Fatalf("want a refusal naming sam@other.example, got %v", err)
	}
}

func TestReconcile_BootRepairsDrift(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	extra := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "extra.example", "verified": true, "coverage": coverageEveryone})
	saveRealRecord(t, app, "users", map[string]any{"email": "r@ext.example", "username": "rue", "name": "Rue", "role": "member"})

	row, _ := findAddressRow(app, "rue", extra.Id)
	if _, err := app.DB().NewQuery("DELETE FROM mail_addresses WHERE id = {:id}").Bind(map[string]any{"id": row.Id}).Execute(); err != nil {
		t.Fatal(err)
	}
	if err := reconcileRuleAddresses(app, ruleScope{}); err != nil {
		t.Fatal(err)
	}
	if row, _ := findAddressRow(app, "rue", extra.Id); row == nil {
		t.Fatal("a full reconcile did not restore a missing rule address")
	}
}

type ruleAPISeed struct {
	admin  *core.Record
	main   *core.Record
	other  *core.Record
	sam    *core.Record
	mailbx *core.Record
}

// seedRuleAPI makes a conflict: sam's email name is already a shared inbox's
// address on other.example, which is still manual.
func seedRuleAPI(t *testing.T, app *tests.TestApp) *ruleAPISeed {
	t.Helper()
	s := &ruleAPISeed{}
	s.admin = saveRealRecord(t, app, "users", map[string]any{"email": "admin@example.net", "username": "adminuser", "name": "Admin", "role": "admin"})
	s.main = saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	s.sam = saveRealRecord(t, app, "users", map[string]any{"email": "s@ext.example", "username": "sam", "name": "Sam", "role": "member"})
	s.other = saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "other.example", "verified": true})
	s.mailbx = saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "sam", "domain": s.other.Id, "type": "shared", "display_name": "Desk", "name": "Desk"})
	for _, name := range []string{"mail_domains", "mail_addresses"} {
		col := mustCollection(t, app, name)
		rule := `@request.auth.id != "" && (@request.auth.role = "admin" || @request.auth.role = "owner")`
		col.UpdateRule, col.CreateRule = &rule, &rule
		if err := app.Save(col); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func runRuleAPIScenario(t *testing.T, method, body string, expect []string, url func(*ruleAPISeed) string, payload func(*ruleAPISeed) string) {
	t.Helper()
	var seed *ruleAPISeed
	scenario := tests.ApiScenario{
		Name:            method,
		Method:          method,
		ExpectedStatus:  http.StatusBadRequest,
		ExpectedContent: expect,
		Headers:         map[string]string{"Content-Type": "application/json"},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		app := newRuleTestApp(t)
		seed = seedRuleAPI(t, app)
		scenario.URL = url(seed)
		scenario.Body = strings.NewReader(payload(seed))
		return app
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		scenario.Headers["Authorization"] = authTokenFor(t, seed.admin)
	}
	scenario.Test(t)
}

func TestRuleAPI_CoverageConflictNamesTheAddress(t *testing.T) {
	runRuleAPIScenario(t, http.MethodPatch, "", []string{`"coverage":{"code":"coverage_conflict"`, "sam@other.example"},
		func(s *ruleAPISeed) string { return "/api/collections/mail_domains/records/" + s.other.Id },
		func(*ruleAPISeed) string { return `{"coverage":"everyone"}` })
}

func TestRuleAPI_TakenAddressIsReportedOnLocalPart(t *testing.T) {
	runRuleAPIScenario(t, http.MethodPost, "", []string{`"local_part":{"code":"address_taken"`},
		func(*ruleAPISeed) string { return "/api/collections/mail_addresses/records" },
		func(s *ruleAPISeed) string {
			return `{"local_part":"sam","domain":"` + s.other.Id + `","mailbox":"` + s.mailbx.Id + `"}`
		})
}
