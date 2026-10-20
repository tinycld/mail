package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/rlstest"
)

// coverage 'groups' and the two group relations must exist once every
// migration has run, and coverage must still reject unknown values.
func TestMigrations_GroupCoverage(t *testing.T) {
	t.Parallel()
	app := rlstest.NewAssembledApp(t, rlstest.MigrationsDir(t, "../pb-migrations"))

	groups, err := app.FindCollectionByNameOrId("groups")
	if err != nil {
		t.Fatal(err)
	}
	g := core.NewRecord(groups)
	g.Set("name", "Staff")
	if err := app.Save(g); err != nil {
		t.Fatal(err)
	}

	domains, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatal(err)
	}
	d := core.NewRecord(domains)
	d.Set("domain", "groups.example")
	d.Set("coverage", "groups")
	d.Set("coverage_groups", []string{g.Id})
	if err := app.Save(d); err != nil {
		t.Fatalf("domain with coverage groups: %v", err)
	}

	bad := core.NewRecord(domains)
	bad.Set("domain", "bogus.example")
	bad.Set("coverage", "bogus")
	if err := app.Save(bad); err == nil {
		t.Fatal("coverage bogus must fail validation")
	}

	mailboxes, err := app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	mb := core.NewRecord(mailboxes)
	mb.Set("address", "staff")
	mb.Set("domain", d.Id)
	mb.Set("type", "shared")
	if err := app.SaveNoValidate(mb); err != nil {
		t.Fatal(err)
	}

	addresses, err := app.FindCollectionByNameOrId("mail_addresses")
	if err != nil {
		t.Fatal(err)
	}
	a := core.NewRecord(addresses)
	a.Set("local_part", "pat")
	a.Set("domain", d.Id)
	a.Set("mailbox", mb.Id)
	a.Set("source", "rule")
	a.Set("via_group", g.Id)
	if err := app.Save(a); err != nil {
		t.Fatalf("address with via_group: %v", err)
	}
	got, err := app.FindRecordById("mail_addresses", a.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetString("via_group") != g.Id {
		t.Fatalf("via_group = %q, want %q", got.GetString("via_group"), g.Id)
	}
}

// seedGroupsCollection stands in for core's groups collection in tests that
// run the mail migrations on a bare app. 2040000010 relates to it by id, as
// it does in a real install where core's migrations run first.
func seedGroupsCollection(t *testing.T, app core.App) {
	t.Helper()
	if err := addGroupsCollection(app); err != nil {
		t.Fatal(err)
	}
}

func addGroupsCollection(app core.App) error {
	c := core.NewBaseCollection("groups", "pbc_groups_01")
	c.Fields.Add(&core.TextField{Name: "name", Required: true})
	return app.Save(c)
}
