package mail

import (
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// setupUIDEnv adds the UID state collection, with the shipped unique index on
// mailbox, to the scope fixture.
func setupUIDEnv(t *testing.T) *scopeEnv {
	t.Helper()
	env := setupScopeEnv(t)
	uidState := core.NewBaseCollection("mail_imap_mailbox_state")
	uidState.Fields.Add(&core.RelationField{
		Name: "mailbox", Required: true, CollectionId: env.mbA.Collection().Id, MaxSelect: 1,
	})
	uidState.Fields.Add(&core.NumberField{Name: "uid_validity"})
	uidState.Fields.Add(&core.NumberField{Name: "uid_next"})
	uidState.AddIndex("idx_imap_mb_state_mailbox", true, "mailbox", "")
	if err := env.app.Save(uidState); err != nil {
		t.Fatal(err)
	}
	return env
}

// newUnsavedMessage builds a message record for assignUID to save.
func newUnsavedMessage(t *testing.T, env *scopeEnv) *core.Record {
	t.Helper()
	threads, err := env.app.FindRecordsByFilter("mail_threads", "mailbox = {:mb}", "", 1, 0, map[string]any{"mb": env.mbA.Id})
	if err != nil || len(threads) == 0 {
		t.Fatalf("no thread in mailbox A: %v", err)
	}
	msgs, err := env.app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	msg := core.NewRecord(msgs)
	msg.Set("thread", threads[0].Id)
	msg.Set("subject", "uid")
	return msg
}

// Inbound delivery and IMAP APPEND allocate inside a transaction, which owns
// SQLite's single write connection; a draft, a send or SMTP allocates outside
// one. The outside allocator must only wait for the connection. Anything it
// holds while waiting blocks the transaction's next allocation, and with it
// every write on the server.
func TestAssignUID_InsideAndOutsideTransaction(t *testing.T) {
	env := setupUIDEnv(t)
	app := env.app
	mailboxID := env.mbA.Id

	outsideMsg := newUnsavedMessage(t, env)
	first := newUnsavedMessage(t, env)
	second := newUnsavedMessage(t, env)

	outside := make(chan error, 1)
	var outsideUID uint32
	var txUIDs []uint32
	err := app.RunInTransaction(func(txApp core.App) error {
		uid, err := assignUID(txApp, mailboxID, first)
		if err != nil {
			return err
		}
		txUIDs = append(txUIDs, uid)
		go func() {
			uid, err := assignUID(app, mailboxID, outsideMsg)
			outsideUID = uid
			outside <- err
		}()
		// Let the outside allocator reach its write before this one continues.
		time.Sleep(300 * time.Millisecond)
		uid, err = assignUID(txApp, mailboxID, second)
		if err != nil {
			return err
		}
		txUIDs = append(txUIDs, uid)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-outside:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the allocation outside the transaction never completed")
	}
	if txUIDs[0] != 1 || txUIDs[1] != 2 || outsideUID != 3 {
		t.Fatalf("uids: tx=%v outside=%d, want 1, 2 and 3", txUIDs, outsideUID)
	}
}

// Concurrent allocators, including the first ones for a mailbox, must get
// distinct consecutive UIDs with no Go lock to serialize them.
func TestAssignUID_ConcurrentAllocatorsAreUnique(t *testing.T) {
	env := setupUIDEnv(t)
	const n = 8
	msgs := make([]*core.Record, n)
	for i := range msgs {
		msgs[i] = newUnsavedMessage(t, env)
	}
	uids := make([]uint32, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uids[i], errs[i] = assignUID(env.app, env.mbA.Id, msgs[i])
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("allocator %d: %v", i, err)
		}
	}
	sort.Slice(uids, func(a, b int) bool { return uids[a] < uids[b] })
	for i, uid := range uids {
		if uid != uint32(i+1) {
			t.Fatalf("uids after sort: %v, want 1..%d", uids, n)
		}
	}
}
