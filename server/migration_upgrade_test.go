package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/offboard"
	"tinycld.org/core/rlstest"
)

// An install upgrading to 1830000014 keeps every address it had. Legacy data
// is seeded at 1830000013, the new migration runs, then the server boots with
// the mail hooks (adopt-only identity repair and a full rule reconcile).
// Every legacy address and alias must still deliver to its mailbox, and the
// person's default From must still be their old address.
func TestUpgrade_LegacyAddressesKeepWorkingAfterBoot(t *testing.T) {
	t.Setenv("IMAP_ENABLED", "false")
	t.Setenv("SMTP_ENABLED", "false")
	app := rlstest.NewAssembledApp(t, migrationsBefore(t, "1830000014"))

	seed := func(collection string, fields map[string]any) *core.Record {
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
	// tinycld.com is older but was verified after tinycld.org, so the
	// inboxes were made on tinycld.org. The migration gives the older
	// domain the everyone rule and rank 1.
	com := seed("mail_domains", map[string]any{"domain": "tinycld.com", "verified": true})
	org := seed("mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})
	for id, created := range map[string]string{com.Id: "2026-09-01 10:00:00.000Z", org.Id: "2026-09-30 17:11:29.416Z"} {
		if _, err := app.DB().NewQuery("UPDATE mail_domains SET created = {:c} WHERE id = {:id}").
			Bind(map[string]any{"c": created, "id": id}).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	nathan := saveRealRecord(t, app, "users", map[string]any{"email": "nathan@argosity.com", "username": "nathan", "name": "Nathan", "role": "owner"})
	ada := saveRealRecord(t, app, "users", map[string]any{"email": "ada@ext.example", "username": "ada", "name": "Ada", "role": "member"})
	personal := seed("mail_mailboxes", map[string]any{"address": "nathan", "domain": org.Id, "type": "personal", "display_name": "Nathan", "name": "Nathan"})
	seed("mail_mailbox_members", map[string]any{"mailbox": personal.Id, "user": nathan.Id, "role": "owner"})
	shared := seed("mail_mailboxes", map[string]any{"address": "support", "domain": org.Id, "type": "shared", "display_name": "Support", "name": "Support"})
	seed("mail_mailbox_members", map[string]any{"mailbox": shared.Id, "user": nathan.Id, "role": "owner"})
	seed("mail_mailbox_members", map[string]any{"mailbox": shared.Id, "user": ada.Id, "role": "member"})
	// No members: the owner left before personal mailboxes were cleaned up.
	ownerless := seed("mail_mailboxes", map[string]any{"address": "orphan", "domain": com.Id, "type": "personal", "display_name": "Orphan", "name": "Orphan"})
	alias := seed("mail_mailbox_aliases", map[string]any{"mailbox": personal.Id, "address": "admin"})
	seed("mail_mailbox_aliases", map[string]any{"mailbox": shared.Id, "address": "help"})

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb-migrations"))
	offboard.ResetHandlersForTesting()
	t.Cleanup(offboard.ResetHandlersForTesting)
	Register(&pocketbase.PocketBase{App: app})
	triggerBoot(t, app)

	for _, tc := range []struct {
		local, domain, mailbox string
	}{
		{"nathan", "tinycld.org", personal.Id},
		{"support", "tinycld.org", shared.Id},
		{"admin", "tinycld.org", personal.Id},
		{"help", "tinycld.org", shared.Id},
		{"orphan", "tinycld.com", ownerless.Id},
	} {
		target, err := resolveAddress(app, tc.local, tc.domain)
		if err != nil {
			t.Fatalf("%s@%s no longer resolves: %v", tc.local, tc.domain, err)
		}
		if target.Mailbox.Id != tc.mailbox {
			t.Fatalf("%s@%s delivers to %s, want %s", tc.local, tc.domain, target.Mailbox.Id, tc.mailbox)
		}
	}

	identity := identityOf(t, app, nathan.Id)
	if identity == nil || identity.GetString("inbox") != personal.Id || identity.GetString("email_name") != "nathan" {
		t.Fatalf("boot did not adopt nathan's inbox: %v", identity)
	}
	if got := identityOf(t, app, ada.Id); got != nil {
		t.Fatalf("boot created an identity for a user with no personal inbox: %v", got)
	}

	if row, _ := findAddressRow(app, "nathan", com.Id); row == nil || row.GetString("mailbox") != personal.Id {
		t.Fatalf("the everyone rule gave nathan no address on tinycld.com: %v", row)
	}
	from, err := defaultSendAddress(app, personal)
	if err != nil || from.Email() != "nathan@tinycld.org" {
		t.Fatalf("default From = %s (err %v), want nathan@tinycld.org", emailOf(from), err)
	}
	legacy, err := resolveSendAddress(app, personal, "", alias.Id)
	if err != nil || legacy.Email() != "admin@tinycld.org" {
		t.Fatalf("legacy alias_id sends as %s (err %v), want admin@tinycld.org", emailOf(legacy), err)
	}
}

func emailOf(r *resolvedAddress) string {
	if r == nil {
		return "<none>"
	}
	return r.Email()
}
