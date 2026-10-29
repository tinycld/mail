package mail

import (
	"fmt"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/pocketbase/pocketbase/core"
)

// go-imap writes every requested STATUS item by dereferencing its pointer,
// so a nil pointer for any requested item panics the connection. Apple Mail
// sends STATUS for every folder about once a minute.
func TestIMAPStatus_FillsEveryRequestedItem(t *testing.T) {
	t.Parallel()
	env := setupScopeEnv(t)
	s := &imapSession{app: env.app, user: env.user}

	// The shared fixture has no UID state collection; UIDNEXT and UIDVALIDITY need it.
	uidState := core.NewBaseCollection("mail_imap_mailbox_state")
	uidState.Fields.Add(&core.RelationField{Name: "mailbox", Required: true, CollectionId: env.mbA.Collection().Id, MaxSelect: 1})
	uidState.Fields.Add(&core.NumberField{Name: "uid_validity"})
	uidState.Fields.Add(&core.NumberField{Name: "uid_next"})
	if err := env.app.Save(uidState); err != nil {
		t.Fatal(err)
	}

	options := &imap.StatusOptions{
		NumMessages:    true,
		UIDNext:        true,
		UIDValidity:    true,
		NumUnseen:      true,
		NumDeleted:     true,
		Size:           true,
		AppendLimit:    true,
		DeletedStorage: true,
		NumRecent:      true,
	}
	data, err := s.buildStatusData("INBOX", env.mbA.Id, options)
	if err != nil {
		t.Fatal(err)
	}
	if data.NumMessages == nil || *data.NumMessages != 1 {
		t.Fatalf("NumMessages = %v, want 1", data.NumMessages)
	}
	if data.NumUnseen == nil || *data.NumUnseen != 1 {
		t.Fatalf("NumUnseen = %v, want 1", data.NumUnseen)
	}
	if data.NumDeleted == nil {
		t.Fatal("NumDeleted is nil")
	}
	if data.Size == nil {
		t.Fatal("Size is nil")
	}
	if data.NumRecent == nil {
		t.Fatal("NumRecent is nil")
	}
	if data.DeletedStorage == nil {
		t.Fatal("DeletedStorage is nil")
	}
}

// A failed count must reach the client as an IMAP error, not as a nil count.
func TestIMAPStatus_QueryFailureIsReturned(t *testing.T) {
	t.Parallel()
	env := setupScopeEnv(t)
	s := &imapSession{app: env.app, user: env.user}

	if _, err := env.app.DB().NewQuery("DROP TABLE mail_thread_state").Execute(); err != nil {
		t.Fatal(err)
	}
	for name, options := range map[string]*imap.StatusOptions{
		"MESSAGES": {NumMessages: true},
		"UNSEEN":   {NumUnseen: true},
		"SIZE":     {Size: true},
	} {
		if _, err := s.buildStatusData("INBOX", env.mbA.Id, options); err == nil {
			t.Fatalf("STATUS (%s) on a broken store returned no error", name)
		}
	}
}

// STATUS must count what a SELECT of the folder lists. SELECT lists only
// messages with an IMAP UID, and \Seen comes from the thread's read state, so
// UNSEEN is the listed messages in unread threads. STATUS counted every
// message for MESSAGES and unread threads for UNSEEN, so a client saw 181
// messages in INBOX but 12 on SELECT, and more unseen than messages in Spam.
func TestIMAPStatus_CountsWhatSelectLists(t *testing.T) {
	t.Parallel()
	env := setupScopeEnv(t)
	s := &imapSession{app: env.app, user: env.user}

	uidState := core.NewBaseCollection("mail_imap_mailbox_state")
	uidState.Fields.Add(&core.RelationField{Name: "mailbox", Required: true, CollectionId: env.mbA.Collection().Id, MaxSelect: 1})
	uidState.Fields.Add(&core.NumberField{Name: "uid_validity"})
	uidState.Fields.Add(&core.NumberField{Name: "uid_next"})
	if err := env.app.Save(uidState); err != nil {
		t.Fatal(err)
	}

	// The fixture's one unread INBOX thread: give its message a UID, add a
	// second listed message, and a third that has no UID.
	inbox, err := s.messagesForFolder("INBOX", env.mbA.Id)
	if err != nil || len(inbox) != 1 {
		t.Fatalf("precondition: INBOX holds %d messages (%v), want 1", len(inbox), err)
	}
	first := inbox[0]
	threadID := first.GetString("thread")
	first.Set("imap_uid", 101)
	if err := env.app.Save(first); err != nil {
		t.Fatal(err)
	}
	for i, uid := range []int{102, 0} {
		msg := core.NewRecord(first.Collection())
		msg.Set("thread", threadID)
		msg.Set("subject", "more")
		msg.Set("message_id", fmt.Sprintf("<status-%d@example.org>", i))
		if err := env.app.Save(msg); err != nil {
			t.Fatal(err)
		}
		// Set after create: a create may assign a UID of its own.
		msg.Set("imap_uid", uid)
		if err := env.app.Save(msg); err != nil {
			t.Fatal(err)
		}
	}

	data, err := s.buildStatusData("INBOX", env.mbA.Id, &imap.StatusOptions{NumMessages: true, NumUnseen: true})
	if err != nil {
		t.Fatal(err)
	}
	if data.NumMessages == nil || data.NumUnseen == nil {
		t.Fatal("MESSAGES or UNSEEN is nil")
	}
	if *data.NumMessages != 2 {
		t.Errorf("MESSAGES = %d, want 2: the message without a UID is not listed", *data.NumMessages)
	}
	if *data.NumUnseen != 2 {
		t.Errorf("UNSEEN = %d, want 2: both listed messages are in the unread thread", *data.NumUnseen)
	}
}
