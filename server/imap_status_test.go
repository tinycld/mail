package mail

import (
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/pocketbase/pocketbase/core"
)

// go-imap writes every requested STATUS item by dereferencing its pointer,
// so a nil pointer for any requested item panics the connection. Apple Mail
// sends STATUS for every folder about once a minute.
func TestIMAPStatus_FillsEveryRequestedItem(t *testing.T) {
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
