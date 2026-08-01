package mail

import (
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// imap_mailbox_scope_test.go pins that IMAP folder contents are scoped to ONE
// mailbox. mail_thread_state rows are per-user, so filtering by user+folder
// alone unions every mailbox the user belongs to: someone in Personal +
// support@ would see support threads inside Personal/INBOX, with unseen
// counts inflated to match — while the multi-mailbox namespace UI implies a
// separation that is not enforced.

type scopeEnv struct {
	app  *tests.TestApp
	user *core.Record
	mbA  *core.Record
	mbB  *core.Record
}

// setupScopeEnv builds two mailboxes on one domain, one user who is a member
// of both, and one unread INBOX thread (with a message) in each mailbox.
func setupScopeEnv(t *testing.T) *scopeEnv {
	t.Helper()
	app := setupInboundTestApp(t)

	// The shipped schema orders messages by imap_uid; the shared fixture
	// predates the field, and without it the sort errors and every folder
	// reads as empty — masking exactly what this file tests.
	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	messages.Fields.Add(&core.NumberField{Name: "imap_uid"})
	if err := app.Save(messages); err != nil {
		t.Fatal(err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	user := core.NewRecord(users)
	user.SetEmail("scoped@example.org")
	user.SetPassword("Password123!")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	domains, _ := app.FindCollectionByNameOrId("mail_domains")
	domain := core.NewRecord(domains)
	domain.Set("domain", "example.org")
	if err := app.Save(domain); err != nil {
		t.Fatal(err)
	}

	mailboxes, _ := app.FindCollectionByNameOrId("mail_mailboxes")
	newMailbox := func(address string) *core.Record {
		mb := core.NewRecord(mailboxes)
		mb.Set("address", address)
		mb.Set("domain", domain.Id)
		mb.Set("type", "shared")
		if err := app.Save(mb); err != nil {
			t.Fatal(err)
		}
		seedMember(t, app, mb.Id, user.Id)
		return mb
	}
	mbA, mbB := newMailbox("personal"), newMailbox("support")

	threads, _ := app.FindCollectionByNameOrId("mail_threads")
	states, _ := app.FindCollectionByNameOrId("mail_thread_state")
	msgs, _ := app.FindCollectionByNameOrId("mail_messages")
	seedInboxThread := func(mb *core.Record, subject string) {
		thread := core.NewRecord(threads)
		thread.Set("mailbox", mb.Id)
		thread.Set("subject", subject)
		if err := app.Save(thread); err != nil {
			t.Fatal(err)
		}
		state := core.NewRecord(states)
		state.Set("thread", thread.Id)
		state.Set("user", user.Id)
		state.Set("folder", "inbox")
		state.Set("is_read", false)
		if err := app.Save(state); err != nil {
			t.Fatal(err)
		}
		msg := core.NewRecord(msgs)
		msg.Set("thread", thread.Id)
		msg.Set("subject", subject)
		msg.Set("imap_uid", 1)
		if err := app.Save(msg); err != nil {
			t.Fatal(err)
		}
	}
	seedInboxThread(mbA, "in personal")
	seedInboxThread(mbB, "in support")

	return &scopeEnv{app: app, user: user, mbA: mbA, mbB: mbB}
}

func TestIMAPFolder_MessagesScopedToMailbox(t *testing.T) {
	env := setupScopeEnv(t)
	s := &imapSession{app: env.app, user: env.user}

	msgs, err := s.messagesForFolder("INBOX", env.mbA.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("INBOX of mailbox A holds %d messages, want 1 — folder contents "+
			"must not union every mailbox the user belongs to", len(msgs))
	}
	if got := msgs[0].GetString("subject"); got != "in personal" {
		t.Fatalf("mailbox A's INBOX shows %q", got)
	}
}

func TestIMAPStatus_UnseenScopedToMailbox(t *testing.T) {
	env := setupScopeEnv(t)
	s := &imapSession{app: env.app, user: env.user}

	data, err := s.buildStatusData("INBOX", env.mbA.Id, &imap.StatusOptions{NumUnseen: true})
	if err != nil {
		t.Fatal(err)
	}
	if data.NumUnseen == nil || *data.NumUnseen != 1 {
		got := uint32(0)
		if data.NumUnseen != nil {
			got = *data.NumUnseen
		}
		t.Fatalf("unseen = %d, want 1 — counts must not include sibling mailboxes", got)
	}
}

// Virtual folders (Starred, All Mail) are per-mailbox in the namespaced UI
// too — the same scoping applies.
func TestIMAPFolder_AllMailScopedToMailbox(t *testing.T) {
	env := setupScopeEnv(t)
	s := &imapSession{app: env.app, user: env.user}

	msgs, err := s.messagesForFolder("All Mail", env.mbB.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("All Mail of mailbox B holds %d messages, want 1", len(msgs))
	}
	if got := msgs[0].GetString("subject"); got != "in support" {
		t.Fatalf("mailbox B's All Mail shows %q", got)
	}
}
