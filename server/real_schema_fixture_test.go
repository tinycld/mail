package mail

import (
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/rlstest"
)

// newRealSchemaApp applies core's and this package's shipped migrations. The
// hand-built fixtures (setupInboundTestApp and friends) relax required fields
// and omit collections, so a bug that only the real schema triggers — a
// required field, a 200-expression filter limit over real label rows — passes
// against them.
func newRealSchemaApp(t testing.TB) *tests.TestApp {
	t.Helper()
	return rlstest.NewAssembledApp(t, rlstest.MigrationsDir(t, "../pb-migrations"))
}

// newMailRulesApp applies this package's shipped migrations, without core's,
// to PocketBase's bundled test data. The template is built once per test
// binary; each test gets a private copy.
func newMailRulesApp(t testing.TB) *tests.TestApp {
	t.Helper()
	return rlstest.Fixture{
		Base:           rlstest.BundledData,
		Prepare:        stubCoreSchemaForMailRules,
		MigrationsDirs: []string{rlstest.MigrationsDir(t, "../pb-migrations")},
	}.NewApp(t)
}

// stubCoreSchemaForMailRules adds the parts of core's schema that the mail
// migrations reference and that this module does not carry: users.role and
// users.disabled, which the shipped rules read, and the groups collection,
// which 2040000010 relates to.
func stubCoreSchemaForMailRules(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		return fmt.Errorf("add users fields: %w", err)
	}
	return addGroupsCollection(app)
}

// saveRealRecord saves one record with the given fields and fails the test on
// any validation error. A users record gets a password so it passes the auth
// collection's validation.
func saveRealRecord(t testing.TB, app core.App, collection string, fields map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatalf("collection %s: %v", collection, err)
	}
	record := core.NewRecord(col)
	for k, v := range fields {
		record.Set(k, v)
	}
	if col.IsAuth() {
		rlstest.SetPassword(record, "Password123!")
	}
	if err := app.Save(record); err != nil {
		t.Fatalf("save %s: %v", collection, err)
	}
	return record
}

// seedRealMailbox creates one domain and one personal mailbox on it.
func seedRealMailbox(t *testing.T, app core.App) *core.Record {
	t.Helper()
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	return saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address":      "owner",
		"domain":       domain.Id,
		"type":         "personal",
		"display_name": "Owner",
		"name":         "Owner",
	})
}
