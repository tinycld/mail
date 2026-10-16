package mail

import (
	"testing"

	"github.com/pocketbase/dbx"
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
	// created is an autodate field: a Set is ignored, so the dates go in by SQL.
	seedDomain := func(domain string, verified bool, created string) *core.Record {
		t.Helper()
		r := save("mail_domains", map[string]any{"domain": domain, "verified": verified})
		_, err := app.DB().NewQuery("UPDATE mail_domains SET created = {:c} WHERE id = {:id}").
			Bind(dbx.Params{"c": created, "id": r.Id}).Execute()
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	old := seedDomain("old.example", false, "2026-09-01 10:00:00.000Z")
	org := seedDomain("tinycld.org", true, "2026-09-30 17:11:29.416Z")
	com := seedDomain("tinycld.com", true, "2026-10-15 20:07:36.434Z")
	personal := save("mail_mailboxes", map[string]any{"address": "nathan", "domain": org.Id, "type": "personal"})
	shared := save("mail_mailboxes", map[string]any{"address": "support", "domain": org.Id, "type": "shared"})
	adminAlias := save("mail_mailbox_aliases", map[string]any{"mailbox": personal.Id, "address": " Admin "})
	// Collides with the shared mailbox's own address: the mailbox must win.
	supportAlias := save("mail_mailbox_aliases", map[string]any{"mailbox": personal.Id, "address": "support"})
	// Drafts and sent mail from an alias record the alias only. After the
	// migration they must record the copied address as sent_from, or a
	// reopened draft sends from the default address.
	aliasDraft := save("mail_messages", map[string]any{"subject": "from admin", "alias": adminAlias.Id})
	// The colliding alias has no row of its own: sent_from stays unset rather
	// than pointing at the shared mailbox's address.
	collidedDraft := save("mail_messages", map[string]any{"subject": "from support", "alias": supportAlias.Id})
	plainDraft := save("mail_messages", map[string]any{"subject": "no alias"})
	messy := save("mail_mailboxes", map[string]any{"address": "  Mixed.Case ", "domain": org.Id, "type": "shared"})
	// Legacy addresses were case-sensitive: Foo and foo are two mailboxes that
	// both become foo@tinycld.com. The older one keeps the address.
	upperFoo := save("mail_mailboxes", map[string]any{"address": "Foo", "domain": com.Id, "type": "shared"})
	lowerFoo := save("mail_mailboxes", map[string]any{"address": "foo", "domain": com.Id, "type": "shared"})
	for id, created := range map[string]string{upperFoo.Id: "2026-10-01 09:00:00.000Z", lowerFoo.Id: "2026-10-02 09:00:00.000Z"} {
		if _, err := app.DB().NewQuery("UPDATE mail_mailboxes SET created = {:c} WHERE id = {:id}").
			Bind(dbx.Params{"c": created, "id": id}).Execute(); err != nil {
			t.Fatal(err)
		}
	}

	seedGroupsCollection(t, app)
	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb-migrations"))

	orgAfter, _ := app.FindRecordById("mail_domains", org.Id)
	comAfter, _ := app.FindRecordById("mail_domains", com.Id)
	if got := orgAfter.GetString("coverage"); got != "everyone" {
		t.Fatalf("oldest verified domain coverage = %q, want everyone", got)
	}
	if got := comAfter.GetString("coverage"); got != "manual" {
		t.Fatalf("newer domain coverage = %q, want manual", got)
	}
	oldAfter, _ := app.FindRecordById("mail_domains", old.Id)
	if got := oldAfter.GetString("coverage"); got != "manual" {
		t.Fatalf("older unverified domain coverage = %q, want manual", got)
	}
	// The everyone domain is first, so it stays the main domain; the others
	// follow by age.
	if orgAfter.GetInt("rank") != 1 || oldAfter.GetInt("rank") != 2 || comAfter.GetInt("rank") != 3 {
		t.Fatalf("ranks org %d, old %d, com %d; want 1, 2, 3",
			orgAfter.GetInt("rank"), oldAfter.GetInt("rank"), comAfter.GetInt("rank"))
	}
	main, err := mainDomain(app)
	if err != nil || main == nil || main.Id != org.Id {
		t.Fatalf("main domain = %v (err %v), want tinycld.org", main, err)
	}

	want := []struct {
		local, mailbox string
		primary        bool
	}{
		{"nathan", personal.Id, true},
		{"support", shared.Id, true},
		{"admin", personal.Id, false},
		{"mixed.case", messy.Id, true},
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
	n, err := app.CountRecords("mail_addresses", dbx.HashExp{"local_part": "support", "domain": org.Id})
	if err != nil || n != 1 {
		t.Fatalf("support@tinycld.org rows = %d (err %v), want 1", n, err)
	}

	adminRow, err := app.FindFirstRecordByFilter("mail_addresses",
		"local_part = 'admin' && domain = {:domain}", dbx.Params{"domain": org.Id})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, id, want string
	}{
		{"alias draft", aliasDraft.Id, adminRow.Id},
		{"collided alias draft", collidedDraft.Id, ""},
		{"draft with no alias", plainDraft.Id, ""},
	} {
		msg, err := app.FindRecordById("mail_messages", c.id)
		if err != nil {
			t.Fatal(err)
		}
		if got := msg.GetString("sent_from"); got != c.want {
			t.Fatalf("%s sent_from = %q, want %q", c.name, got, c.want)
		}
	}

	foos, err := app.FindRecordsByFilter("mail_addresses", "local_part = 'foo' && domain = {:domain}", "", 0, 0,
		dbx.Params{"domain": com.Id})
	if err != nil || len(foos) != 1 {
		t.Fatalf("foo@tinycld.com rows = %d (err %v), want 1", len(foos), err)
	}
	if foos[0].GetString("mailbox") != upperFoo.Id {
		t.Fatalf("foo@tinycld.com delivers to %s, want the older mailbox %s", foos[0].GetString("mailbox"), upperFoo.Id)
	}
	if n, _ := app.CountRecords("mail_addresses", dbx.HashExp{"mailbox": lowerFoo.Id}); n != 0 {
		t.Fatalf("the newer colliding mailbox has %d address rows, want 0", n)
	}
}
