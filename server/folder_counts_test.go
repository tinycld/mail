package mail

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func countsRow(t *testing.T, env *scopeEnv, mailboxID string) *core.Record {
	t.Helper()
	row, err := env.app.FindRecordById("mail_folder_counts", folderCountsID(env.user.Id, mailboxID))
	if err != nil {
		t.Fatalf("no counts row for %s: %v", mailboxID, err)
	}
	return row
}

func TestFolderCounts_RowFollowsStateChanges(t *testing.T) {
	env := setupScopeEnv(t)
	// setupScopeEnv seeds one unread inbox thread per mailbox.
	row := countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox") != 1 || row.GetInt("inbox_total") != 1 || row.GetInt("total") != 1 {
		t.Fatalf("after seed: inbox=%d inbox_total=%d total=%d, want 1 1 1",
			row.GetInt("inbox"), row.GetInt("inbox_total"), row.GetInt("total"))
	}

	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	if err := setThreadRead(env.app, thread.Id, env.user.Id, true); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox") != 0 || row.GetInt("inbox_total") != 1 {
		t.Errorf("after read: inbox=%d inbox_total=%d, want 0 1", row.GetInt("inbox"), row.GetInt("inbox_total"))
	}

	if err := setThreadFolder(env.app, thread.Id, env.user.Id, "archive"); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox_total") != 0 || row.GetInt("archive") != 1 || row.GetInt("total") != 1 {
		t.Errorf("after archive: inbox_total=%d archive=%d total=%d, want 0 1 1",
			row.GetInt("inbox_total"), row.GetInt("archive"), row.GetInt("total"))
	}

	if err := markThreadSent(env.app, thread.Id, env.user.Id); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("sent") != 1 {
		t.Errorf("after send: sent=%d, want 1", row.GetInt("sent"))
	}

	if err := setThreadFolder(env.app, thread.Id, env.user.Id, "trash"); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("sent") != 0 || row.GetInt("trash") != 1 {
		t.Errorf("after trash: sent=%d trash=%d, want 0 1 (sent excludes trash)", row.GetInt("sent"), row.GetInt("trash"))
	}

	state := findThreadState(env.app, thread.Id, env.user.Id)
	if err := env.app.Delete(state); err != nil {
		t.Fatal(err)
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("total") != 0 || row.GetInt("trash") != 0 {
		t.Errorf("after delete: total=%d trash=%d, want 0 0", row.GetInt("total"), row.GetInt("trash"))
	}
}

func TestFolderCounts_BulkArchiveLeavesCorrectRow(t *testing.T) {
	env := setupScopeEnv(t)
	threads, _ := env.app.FindCollectionByNameOrId("mail_threads")
	for i := 0; i < 25; i++ {
		thread := core.NewRecord(threads)
		thread.Set("mailbox", env.mbA.Id)
		thread.Set("subject", "bulk")
		thread.Set("latest_date", "2026-10-05 00:00:00.000Z")
		if err := env.app.Save(thread); err != nil {
			t.Fatal(err)
		}
		if err := ensureThreadState(env.app, thread.Id, env.user.Id, "inbox", false); err != nil {
			t.Fatal(err)
		}
	}
	row := countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox") != 26 {
		t.Fatalf("inbox=%d, want 26", row.GetInt("inbox"))
	}

	states, err := env.app.FindRecordsByFilter("mail_thread_state", "user = {:u} && mailbox = {:mb}", "", 0, 0,
		map[string]any{"u": env.user.Id, "mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		state.Set("folder", "archive")
		if err := env.app.Save(state); err != nil {
			t.Fatal(err)
		}
	}
	row = countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox") != 0 || row.GetInt("archive") != 26 || row.GetInt("total") != 26 {
		t.Errorf("inbox=%d archive=%d total=%d, want 0 26 26", row.GetInt("inbox"), row.GetInt("archive"), row.GetInt("total"))
	}
}

func TestFolderCounts_OtherMailboxUntouched(t *testing.T) {
	env := setupScopeEnv(t)
	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	if err := setThreadFolder(env.app, thread.Id, env.user.Id, "trash"); err != nil {
		t.Fatal(err)
	}
	other := countsRow(t, env, env.mbB.Id)
	if other.GetInt("inbox") != 1 || other.GetInt("trash") != 0 {
		t.Errorf("mailbox B inbox=%d trash=%d, want 1 0", other.GetInt("inbox"), other.GetInt("trash"))
	}
}

// Deleting a mailbox cascades to its threads, their state rows, and its counts
// row. The state delete hooks then run with the counts row gone; they must not
// recreate it, since the new row would point at the deleted mailbox and fail
// relation validation.
func TestFolderCounts_MailboxDeleteDoesNotRecreateRow(t *testing.T) {
	env := setupScopeEnv(t)
	countsRow(t, env, env.mbA.Id)

	if err := env.app.Delete(env.mbA); err != nil {
		t.Fatal(err)
	}
	id := folderCountsID(env.user.Id, env.mbA.Id)
	if _, err := env.app.FindRecordById("mail_folder_counts", id); err == nil {
		t.Fatal("counts row for the deleted mailbox was recreated")
	}
	// The hook only logs a failure, so call the recompute directly to see the
	// error the hook would have swallowed.
	if err := recomputeFolderCounts(env.app, env.user.Id, env.mbA.Id); err != nil {
		t.Errorf("recompute after mailbox delete: %v", err)
	}
	if _, err := env.app.FindRecordById("mail_folder_counts", id); err == nil {
		t.Error("recompute created a counts row for a mailbox with no state rows")
	}
}

// Two state changes of one mailbox can both find no counts row and both
// create it. The one that loses must still leave the row correct, not fail.
func TestFolderCounts_LosingTheCreateRaceUpdatesTheRow(t *testing.T) {
	env := setupScopeEnv(t)
	if err := env.app.Delete(countsRow(t, env, env.mbA.Id)); err != nil {
		t.Fatal(err)
	}
	rival := false
	env.app.OnRecordCreate("mail_folder_counts").BindFunc(func(e *core.RecordEvent) error {
		if !rival {
			// Another change of the same mailbox creates the row first.
			rival = true
			other := core.NewRecord(e.Record.Collection())
			other.Id = e.Record.Id
			other.Set("user", e.Record.GetString("user"))
			other.Set("mailbox", e.Record.GetString("mailbox"))
			if err := e.App.Save(other); err != nil {
				t.Fatalf("rival create: %v", err)
			}
		}
		return e.Next()
	})

	if err := recomputeFolderCounts(env.app, env.user.Id, env.mbA.Id); err != nil {
		t.Fatalf("recompute after losing the create: %v", err)
	}
	row := countsRow(t, env, env.mbA.Id)
	if row.GetInt("inbox") != 1 || row.GetInt("total") != 1 {
		t.Fatalf("after the race: inbox=%d total=%d, want 1 1", row.GetInt("inbox"), row.GetInt("total"))
	}
}

type addressCountsEnv struct {
	app           *tests.TestApp
	user          *core.Record
	inbox, shared *core.Record
	addrA, addrB  *core.Record
	sharedAddr    *core.Record
}

func setupAddressCountsEnv(t *testing.T) *addressCountsEnv {
	t.Helper()
	app := newRealSchemaApp(t)
	registerThreadStateSyncHooks(app)
	registerFolderCountHooks(app)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "c.example", "verified": true})
	user := saveRealRecord(t, app, "users", map[string]any{"email": "cy@ext.example", "username": "cy", "name": "Cy", "role": "member"})
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "cy", "domain": domain.Id, "type": "personal", "display_name": "Cy", "name": "Cy",
	})
	saveRealRecord(t, app, "mail_identities", map[string]any{"user": user.Id, "inbox": inbox.Id, "email_name": "cy"})
	shared := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "desk", "domain": domain.Id, "type": "shared", "display_name": "Desk", "name": "Desk",
	})
	newAddr := func(mailbox *core.Record, local string) *core.Record {
		return saveRealRecord(t, app, "mail_addresses", map[string]any{
			"local_part": local, "domain": domain.Id, "mailbox": mailbox.Id, "source": addressSourceManual,
		})
	}
	return &addressCountsEnv{
		app: app, user: user, inbox: inbox, shared: shared,
		addrA: newAddr(inbox, "a"), addrB: newAddr(inbox, "b"), sharedAddr: newAddr(shared, "sales"),
	}
}

// seedThread stores a thread with the given address ids and the user's state row.
func (env *addressCountsEnv) seedThread(t *testing.T, mailbox *core.Record, read bool, addrs ...*core.Record) *core.Record {
	t.Helper()
	ids := make([]string, len(addrs))
	for i, a := range addrs {
		ids[i] = a.Id
	}
	thread := saveRealRecord(t, env.app, "mail_threads", map[string]any{
		"mailbox": mailbox.Id, "subject": "s", "latest_date": "2026-10-01 09:00:00.000Z",
		"address_keys": strings.Join(ids, " "),
	})
	if state := findThreadState(env.app, thread.Id, env.user.Id); state != nil {
		state.Set("is_read", read)
		if err := env.app.Save(state); err != nil {
			t.Fatal(err)
		}
		return thread
	}
	saveRealRecord(t, env.app, "mail_thread_state", map[string]any{
		"thread": thread.Id, "user": env.user.Id, "folder": "inbox", "is_read": read,
	})
	return thread
}

func (env *addressCountsEnv) byAddress(t *testing.T, mailbox *core.Record) map[string]addressCount {
	t.Helper()
	row, err := env.app.FindRecordById("mail_folder_counts", folderCountsID(env.user.Id, mailbox.Id))
	if err != nil {
		t.Fatalf("no counts row: %v", err)
	}
	out := map[string]addressCount{}
	raw := row.GetString("inbox_by_address")
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("inbox_by_address %q: %v", raw, err)
	}
	return out
}

func TestFolderCounts_CountsEachAddressOfThePersonalInbox(t *testing.T) {
	env := setupAddressCountsEnv(t)
	env.seedThread(t, env.inbox, false, env.addrA)
	env.seedThread(t, env.inbox, true, env.addrA, env.addrB)

	got := env.byAddress(t, env.inbox)
	want := map[string]addressCount{
		env.addrA.Id: {Unread: 1, Total: 2},
		env.addrB.Id: {Unread: 0, Total: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inbox_by_address = %v, want %v", got, want)
	}
}

func TestFolderCounts_ArchiveDropsThreadFromAddressNumbers(t *testing.T) {
	env := setupAddressCountsEnv(t)
	only := env.seedThread(t, env.inbox, false, env.addrA)
	env.seedThread(t, env.inbox, true, env.addrA, env.addrB)

	if err := setThreadFolder(env.app, only.Id, env.user.Id, "archive"); err != nil {
		t.Fatal(err)
	}
	got := env.byAddress(t, env.inbox)
	want := map[string]addressCount{
		env.addrA.Id: {Unread: 0, Total: 1},
		env.addrB.Id: {Unread: 0, Total: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after archive inbox_by_address = %v, want %v", got, want)
	}
}

func TestFolderCounts_SharedInboxHasNoAddressNumbers(t *testing.T) {
	env := setupAddressCountsEnv(t)
	env.seedThread(t, env.shared, false, env.sharedAddr)

	if got := env.byAddress(t, env.shared); len(got) != 0 {
		t.Fatalf("shared inbox inbox_by_address = %v, want empty", got)
	}
	row, _ := env.app.FindRecordById("mail_folder_counts", folderCountsID(env.user.Id, env.shared.Id))
	if row.GetString("inbox_by_address") != "{}" {
		t.Errorf("shared inbox stores %q, want {}", row.GetString("inbox_by_address"))
	}
}

func TestFolderCounts_UnchangedAddressMapDoesNotSave(t *testing.T) {
	env := setupAddressCountsEnv(t)
	env.seedThread(t, env.inbox, false, env.addrA)

	saves := 0
	env.app.OnRecordUpdate("mail_folder_counts").BindFunc(func(e *core.RecordEvent) error {
		saves++
		return e.Next()
	})
	if err := recomputeFolderCounts(env.app, env.user.Id, env.inbox.Id); err != nil {
		t.Fatal(err)
	}
	if saves != 0 {
		t.Errorf("an unchanged recompute saved %d times, want 0", saves)
	}
}
