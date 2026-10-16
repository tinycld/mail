package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/rlstest"
)

// An existing deployment has mailboxes with addresses, aliases, and domains
// with no coverage or rank. After 1830000014 every mailbox address and alias
// must be a mail_addresses row, so mail keeps arriving, and only the oldest
// verified domain gives everyone an address — today's behavior.
func TestMigrations_AddressesBackfill(t *testing.T) {
	app := rlstest.NewApp(t)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.Fields.Add(&core.SelectField{Name: "role", MaxSelect: 1, Values: []string{"owner", "admin", "member", "guest"}})
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}

	rlstest.Apply(t, app, migrationsBefore(t, "1830000014"))

	save := func(collection string, fields map[string]any) *core.Record {
		t.Helper()
		col, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			t.Fatal(err)
		}
		r := core.NewRecord(col)
		for k, v := range fields {
			r.Set(k, v)
		}
		if err := app.SaveNoValidate(r); err != nil {
			t.Fatalf("seed %s: %v", collection, err)
		}
		return r
	}
	org := save("mail_domains", map[string]any{"domain": "tinycld.org", "verified": true, "created": "2026-09-30 17:11:29.416Z"})
	com := save("mail_domains", map[string]any{"domain": "tinycld.com", "verified": true, "created": "2026-10-15 20:07:36.434Z"})
	personal := save("mail_mailboxes", map[string]any{"address": "nathan", "domain": org.Id, "type": "personal"})
	shared := save("mail_mailboxes", map[string]any{"address": "support", "domain": org.Id, "type": "shared"})
	save("mail_mailbox_aliases", map[string]any{"mailbox": personal.Id, "address": "admin"})

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb-migrations"))

	orgAfter, _ := app.FindRecordById("mail_domains", org.Id)
	comAfter, _ := app.FindRecordById("mail_domains", com.Id)
	if got := orgAfter.GetString("coverage"); got != "everyone" {
		t.Fatalf("oldest verified domain coverage = %q, want everyone", got)
	}
	if got := comAfter.GetString("coverage"); got != "manual" {
		t.Fatalf("newer domain coverage = %q, want manual", got)
	}
	if orgAfter.GetInt("rank") != 1 || comAfter.GetInt("rank") != 2 {
		t.Fatalf("ranks = %d, %d; want 1, 2", orgAfter.GetInt("rank"), comAfter.GetInt("rank"))
	}

	want := []struct {
		local, mailbox string
		primary        bool
	}{
		{"nathan", personal.Id, true},
		{"support", shared.Id, true},
		{"admin", personal.Id, false},
	}
	for _, w := range want {
		row, err := app.FindFirstRecordByFilter("mail_addresses",
			"local_part = {:local} && domain = {:domain}",
			map[string]any{"local": w.local, "domain": org.Id})
		if err != nil {
			t.Fatalf("%s@tinycld.org was not copied into mail_addresses: %v", w.local, err)
		}
		if row.GetString("mailbox") != w.mailbox || row.GetBool("is_primary") != w.primary ||
			row.GetString("source") != "manual" || len(row.Id) != 15 {
			t.Fatalf("%s@tinycld.org row = mailbox %s primary %v source %q id %q",
				w.local, row.GetString("mailbox"), row.GetBool("is_primary"), row.GetString("source"), row.Id)
		}
	}
}
