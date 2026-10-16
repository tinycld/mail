package mail

import (
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

func TestRuleAddresses_UnverifiedDomainGivesNothing(t *testing.T) {
	app := newRuleTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	saveRealRecord(t, app, "users", map[string]any{"email": "a@ext.example", "username": "ann", "name": "Ann", "role": "member"})
	pending := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "pending.example", "verified": false, "coverage": coverageEveryone})

	if got := ruleRows(t, app, pending.Id); len(got) != 0 {
		t.Fatalf("an unverified domain got rule addresses: %v", got)
	}
	pending = reloaded(t, app, pending)
	pending.Set("verified", true)
	if err := app.Save(pending); err != nil {
		t.Fatal(err)
	}
	if got := ruleRows(t, app, pending.Id); got["ann"] == "" {
		t.Fatalf("verifying the domain gave no address: %v", got)
	}
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
