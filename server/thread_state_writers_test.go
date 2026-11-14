package mail

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// raceWithPropagation runs a thread save whose address_keys change propagates
// to the state rows, and a second writer of the same state row, so that the
// second writer finishes while the propagation sits between its read of the
// row and its save of it. A propagation that reads and saves outside one
// transaction then saves the row it read before the second writer changed it,
// and writes that change away. With one transaction the second writer cannot
// run until the propagation commits, so the barrier below gives up waiting.
func raceWithPropagation(t *testing.T, env *addressKeysEnv, threadID string, writer func()) {
	t.Helper()
	var armed atomic.Bool
	armed.Store(true)
	inGap := make(chan struct{})
	writerDone := make(chan struct{})

	env.app.OnRecordUpdate("mail_thread_state").Bind(&hook.Handler[*core.RecordEvent]{
		Priority: -1,
		Func: func(e *core.RecordEvent) error {
			if armed.CompareAndSwap(true, false) {
				close(inGap)
				select {
				case <-writerDone:
				case <-time.After(500 * time.Millisecond):
				}
			}
			return e.Next()
		},
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		thread := mustRecord(t, env.app, "mail_threads", threadID)
		thread.Set("address_keys", "propagated-key")
		if err := env.app.Save(thread); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		defer close(writerDone)
		<-inGap
		writer()
	}()
	wg.Wait()
}

func deliveredThread(t *testing.T, env *addressKeysEnv, messageID string) (msg *core.Record, threadID string) {
	t.Helper()
	msg = env.deliverTo(t, "ann@q.example", messageID, "")
	return msg, msg.GetString("thread")
}

func TestThreadStateWriters_RuleStarSurvivesAddressKeyPropagation(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	_, threadID := deliveredThread(t, env, "<race-star@sender.example>")

	raceWithPropagation(t, env, threadID, func() {
		if err := setThreadStateField(context.Background(), env.app, threadID, env.ann.Id, "is_starred", true); err != nil {
			t.Error(err)
		}
	})

	state := findThreadState(env.app, threadID, env.ann.Id)
	if !state.GetBool("is_starred") {
		t.Error("the star was written away by the address_keys propagation")
	}
	if got := state.GetString("address_keys"); got != "propagated-key" {
		t.Errorf("address_keys = %q, the propagation was written away", got)
	}
}

func TestThreadStateWriters_IMAPFlagSurvivesAddressKeyPropagation(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	msg, threadID := deliveredThread(t, env, "<race-flag@sender.example>")
	session := &imapSession{app: env.app, user: env.ann, deleted: map[string]bool{}}

	raceWithPropagation(t, env, threadID, func() {
		session.applyFlags(msg, &imap.StoreFlags{
			Op:    imap.StoreFlagsAdd,
			Flags: []imap.Flag{imap.FlagSeen, imap.FlagFlagged},
		})
	})

	state := findThreadState(env.app, threadID, env.ann.Id)
	if !state.GetBool("is_read") || !state.GetBool("is_starred") {
		t.Errorf("IMAP flags lost: is_read=%v is_starred=%v", state.GetBool("is_read"), state.GetBool("is_starred"))
	}
	if got := state.GetString("address_keys"); got != "propagated-key" {
		t.Errorf("address_keys = %q, the propagation was written away", got)
	}
}

func TestModifyThreadState_CreatesInInboxAndKeepsOtherColumns(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	_, threadID := deliveredThread(t, env, "<modify@sender.example>")
	other := saveRealRecord(t, env.app, "users", map[string]any{
		"email": "bo@ext.example", "username": "bo", "name": "Bo", "role": "member",
	})

	if err := setThreadStateFields(context.Background(), env.app, threadID, other.Id, map[string]any{"is_starred": true}); err != nil {
		t.Fatal(err)
	}
	created := findThreadState(env.app, threadID, other.Id)
	if created == nil || created.GetString("folder") != "inbox" || !created.GetBool("is_starred") || created.GetBool("is_read") {
		t.Fatalf("a missing row should be created in the Inbox with only the given field set, got %+v", created)
	}

	if err := setThreadStateFields(context.Background(), env.app, threadID, env.ann.Id, map[string]any{"folder": "archive", "is_read": true}); err != nil {
		t.Fatal(err)
	}
	state := findThreadState(env.app, threadID, env.ann.Id)
	if state.GetString("folder") != "archive" || !state.GetBool("is_read") || state.GetString("address_keys") == "" {
		t.Errorf("fields not set, or another column was changed: folder=%q is_read=%v address_keys=%q",
			state.GetString("folder"), state.GetBool("is_read"), state.GetString("address_keys"))
	}
}

func TestModifyThreadState_MissingRowIsLeftAloneWithoutCreate(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	_, threadID := deliveredThread(t, env, "<nocreate@sender.example>")
	other := saveRealRecord(t, env.app, "users", map[string]any{
		"email": "cy@ext.example", "username": "cy", "name": "Cy", "role": "member",
	})

	err := modifyThreadState(context.Background(), env.app, threadID, other.Id, false, func(core.App, *core.Record) error {
		t.Error("callback ran for a missing row")
		return nil
	})
	if err != nil || findThreadState(env.app, threadID, other.Id) != nil {
		t.Errorf("a missing row must stay missing, err=%v", err)
	}
}

func TestModifyThreadState_UnchangedSkipsTheSave(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	_, threadID := deliveredThread(t, env, "<unchanged@sender.example>")
	before := findThreadState(env.app, threadID, env.ann.Id).GetString("updated")
	time.Sleep(5 * time.Millisecond)

	err := modifyThreadState(context.Background(), env.app, threadID, env.ann.Id, true, func(core.App, *core.Record) error {
		return errThreadStateUnchanged
	})
	if err != nil {
		t.Fatal(err)
	}
	if after := findThreadState(env.app, threadID, env.ann.Id).GetString("updated"); after != before {
		t.Errorf("row saved although the callback reported no change (%q -> %q)", before, after)
	}
}
