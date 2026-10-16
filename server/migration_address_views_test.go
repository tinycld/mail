package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tinycld.org/core/rlstest"
)

// Threads that existed before 2040000014 have no address_keys. The migration
// must derive them from the messages' delivered_to and sent_from, each address
// once, and copy them onto the thread's state rows.
func TestMigrationAddressViews(t *testing.T) {
	app := rlstest.NewAssembledApp(t, migrationsBefore(t, "2040000014"))

	mailbox := seedRealMailbox(t, app)
	domainID := mailbox.GetString("domain")
	user := saveRealRecord(t, app, "users", map[string]any{"email": "owner@example.org", "name": "Owner", "username": "owner", "role": "member"})
	addrA := saveRealRecord(t, app, "mail_addresses", map[string]any{"local_part": "a", "domain": domainID, "mailbox": mailbox.Id, "source": "manual"})
	addrB := saveRealRecord(t, app, "mail_addresses", map[string]any{"local_part": "b", "domain": domainID, "mailbox": mailbox.Id, "source": "manual"})
	thread := saveRealRecord(t, app, "mail_threads", map[string]any{"mailbox": mailbox.Id, "subject": "hi", "latest_date": "2026-10-01 10:00:00.000Z"})
	saveRealRecord(t, app, "mail_messages", map[string]any{"thread": thread.Id, "date": "2026-10-01 10:00:00.000Z", "sender_email": "x@example.org", "delivered_to": addrA.Id})
	saveRealRecord(t, app, "mail_messages", map[string]any{"thread": thread.Id, "date": "2026-10-01 10:00:00.000Z", "sender_email": "x@example.org", "sent_from": addrB.Id})
	saveRealRecord(t, app, "mail_messages", map[string]any{"thread": thread.Id, "date": "2026-10-01 10:00:00.000Z", "sender_email": "x@example.org", "delivered_to": addrA.Id})
	state := saveRealRecord(t, app, "mail_thread_state", map[string]any{"thread": thread.Id, "user": user.Id, "folder": "inbox"})

	src := rlstest.MigrationsDir(t, "../pb-migrations")
	body, err := os.ReadFile(filepath.Join(src, "2040000014_mail_address_views.js"))
	if err != nil {
		t.Fatal(err)
	}
	only := t.TempDir()
	if err := os.WriteFile(filepath.Join(only, "2040000014_mail_address_views.js"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	rlstest.Apply(t, app, only)

	want := map[string]bool{addrA.Id: true, addrB.Id: true}
	check := func(label, keys string) {
		t.Helper()
		got := strings.Fields(keys)
		if len(got) != 2 {
			t.Fatalf("%s address_keys = %q, want two ids", label, keys)
		}
		for _, id := range got {
			if !want[id] {
				t.Fatalf("%s address_keys = %q has unexpected %q", label, keys, id)
			}
		}
		if got[0] == got[1] {
			t.Fatalf("%s address_keys = %q repeats an id", label, keys)
		}
	}
	gotThread, err := app.FindRecordById("mail_threads", thread.Id)
	if err != nil {
		t.Fatal(err)
	}
	check("thread", gotThread.GetString("address_keys"))
	gotState, err := app.FindRecordById("mail_thread_state", state.Id)
	if err != nil {
		t.Fatal(err)
	}
	check("state", gotState.GetString("address_keys"))

	for collection, field := range map[string]string{
		"mail_identities":    "address_views",
		"mail_folder_counts": "inbox_by_address",
	} {
		c, err := app.FindCollectionByNameOrId(collection)
		if err != nil {
			t.Fatal(err)
		}
		if c.Fields.GetByName(field) == nil {
			t.Fatalf("%s lacks %s", collection, field)
		}
	}
	identities, _ := app.FindCollectionByNameOrId("mail_identities")
	if identities.Fields.GetByName("hidden_address_views") == nil {
		t.Fatal("mail_identities lacks hidden_address_views")
	}
}
