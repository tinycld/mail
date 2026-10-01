package mail

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/rlstest"
)

// migrationsBefore copies the shipped migrations that sort before `first` into
// a temp dir, so a test can seed rows in the schema an existing deployment
// has and then apply the rest.
func migrationsBefore(t *testing.T, first string) string {
	t.Helper()
	src := rlstest.MigrationsDir(t, "../pb-migrations")
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".js" || e.Name() >= first {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

// An existing deployment has sent rows whose message_id is Postmark's bare
// tracking id, inbound rows with bracketed ids, and threads filed under
// 'sent'. The migrations must move the provider id to provider_message_id,
// normalize every id to one form, and flag the sent threads.
func TestMigrations_SentModelBackfill(t *testing.T) {
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

	rlstest.Apply(t, app, migrationsBefore(t, "1830000010"))

	user := core.NewRecord(users)
	user.SetEmail("nas@example.org")
	user.SetPassword("Password123!")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
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
	domain := save("mail_domains", map[string]any{"domain": "example.org"})
	mailbox := save("mail_mailboxes", map[string]any{"address": "nas", "domain": domain.Id, "type": "personal"})
	received := save("mail_threads", map[string]any{"mailbox": mailbox.Id, "subject": "Hello"})
	started := save("mail_threads", map[string]any{"mailbox": mailbox.Id, "subject": "Mine"})
	inbound := save("mail_messages", map[string]any{
		"thread": received.Id, "message_id": "<etPan.1@argosity.test>", "delivery_status": "sending",
	})
	reply := save("mail_messages", map[string]any{
		"thread": received.Id, "message_id": "522c353a-uuid", "in_reply_to": "etPan.1@argosity.test",
		"sent_by": user.Id, "delivery_status": "sent",
	})
	receivedState := save("mail_thread_state", map[string]any{"thread": received.Id, "user": user.Id, "folder": "inbox"})
	startedState := save("mail_thread_state", map[string]any{"thread": started.Id, "user": user.Id, "folder": "sent"})

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb-migrations"))

	reload := func(collection, id string) *core.Record {
		t.Helper()
		r, err := app.FindRecordById(collection, id)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := reload("mail_messages", reply.Id); r.GetString("provider_message_id") != "522c353a-uuid" ||
		r.GetString("message_id") != "<522c353a-uuid>" || r.GetString("in_reply_to") != "<etPan.1@argosity.test>" {
		t.Errorf("sent row: provider_message_id=%q message_id=%q in_reply_to=%q",
			r.GetString("provider_message_id"), r.GetString("message_id"), r.GetString("in_reply_to"))
	}
	if r := reload("mail_messages", inbound.Id); r.GetString("provider_message_id") != "" ||
		r.GetString("message_id") != "<etPan.1@argosity.test>" {
		t.Errorf("inbound row: provider_message_id=%q message_id=%q", r.GetString("provider_message_id"), r.GetString("message_id"))
	}
	if reload("mail_thread_state", receivedState.Id).GetBool("is_sent") {
		t.Error("an inbox thread was flagged sent")
	}
	if !reload("mail_thread_state", startedState.Id).GetBool("is_sent") {
		t.Error("a thread filed under sent was not flagged sent")
	}

	counts, err := app.FindRecordsByFilter("mail_folder_counts", "user = {:u}", "", 0, 0, map[string]any{"u": user.Id})
	if err != nil || len(counts) != 1 {
		t.Fatalf("mail_folder_counts rows = %d, err = %v", len(counts), err)
	}
	// PocketBase types the view's SUM columns as JSON, so read the raw value.
	if got := fmt.Sprint(counts[0].Get("sent")); got != "1" {
		t.Errorf("sent count = %s, want 1", got)
	}
	if !strings.Contains(counts[0].Collection().ViewQuery, "is_sent") {
		t.Error("mail_folder_counts does not read is_sent")
	}
}
