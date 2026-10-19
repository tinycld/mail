package mail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
	"tinycld.org/core/rlstest"
)

// Rules saved before 2040000017 put the "Received at address" condition on
// alias, which inbound mail never sets. The migration must move each one to
// delivered_to, and turn an alias id into the id of the address row the alias
// was copied to.
func TestMigrationRulesReceivedAddress(t *testing.T) {
	app := rlstest.NewAssembledApp(t, migrationsBefore(t, "2040000017"))

	mailbox := seedRealMailbox(t, app)
	domainID := mailbox.GetString("domain")
	alias := saveRealRecord(t, app, "mail_mailbox_aliases", map[string]any{"mailbox": mailbox.Id, "address": "sales"})
	saveRealRecord(t, app, "mail_addresses", map[string]any{"local_part": "owner", "domain": domainID, "mailbox": mailbox.Id, "source": "manual", "is_primary": true})
	salesAddr := saveRealRecord(t, app, "mail_addresses", map[string]any{"local_part": "sales", "domain": domainID, "mailbox": mailbox.Id, "source": "manual"})
	owner := saveRealRecord(t, app, "users", map[string]any{"email": "owner@example.org", "name": "Owner", "username": "owner", "role": "member"})

	rule := func(trigger string, conditions ...map[string]any) *core.Record {
		return saveRealRecord(t, app, "rules", map[string]any{
			"name": "r", "scope": "personal", "owner": owner.Id, "trigger": trigger, "enabled": true,
			"conditions": map[string]any{"match": "all", "groups": []map[string]any{{"match": "all", "conditions": conditions}}},
		})
	}
	cond := func(field, op string, value any) map[string]any {
		return map[string]any{"field": field, "op": op, "value": value}
	}
	isAlias := rule("mail:message-received", cond("subject", "contains", "invoice"), cond("alias", "is", alias.Id))
	notAlias := rule("mail:message-received", cond("alias", "is_not", alias.Id))
	deletedAlias := rule("mail:message-received", cond("alias", "is", "deletedalias123"))
	emptyAlias := rule("mail:message-received", cond("alias", "is_empty", nil))
	otherTrigger := rule("mail:message-bounced", cond("alias", "is", alias.Id))

	src := rlstest.MigrationsDir(t, "../pb-migrations")
	body, err := os.ReadFile(filepath.Join(src, "2040000017_mail_rules_received_address.js"))
	if err != nil {
		t.Fatal(err)
	}
	only := t.TempDir()
	if err := os.WriteFile(filepath.Join(only, "2040000017_mail_rules_received_address.js"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	rlstest.Apply(t, app, only)

	conditionsOf := func(r *core.Record) []automation.Condition {
		t.Helper()
		got, err := app.FindRecordById("rules", r.Id)
		if err != nil {
			t.Fatal(err)
		}
		ast, err := automation.DecodeConditions(got.Get("conditions"))
		if err != nil {
			t.Fatal(err)
		}
		return ast.Groups[0].Conditions
	}
	want := func(label string, r *core.Record, expected ...automation.Condition) {
		t.Helper()
		got := conditionsOf(r)
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(expected)
		if string(gotJSON) != string(wantJSON) {
			t.Fatalf("%s conditions = %s, want %s", label, gotJSON, wantJSON)
		}
	}

	want("is", isAlias,
		automation.Condition{Field: "subject", Op: "contains", Value: "invoice"},
		automation.Condition{Field: "delivered_to", Op: "is", Value: salesAddr.Id})
	want("is_not", notAlias, automation.Condition{Field: "delivered_to", Op: "is_not", Value: salesAddr.Id})
	want("deleted alias", deletedAlias, automation.Condition{Field: "delivered_to", Op: "is", Value: "deletedalias123"})
	want("is_empty", emptyAlias, automation.Condition{Field: "delivered_to", Op: "is_empty"})
	want("other trigger", otherTrigger, automation.Condition{Field: "alias", Op: "is", Value: alias.Id})
}
