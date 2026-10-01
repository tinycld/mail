package mail

import (
	"bytes"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/packages/mail/api"
)

// sent_model_test.go pins the Gmail-style Sent model and the Message-ID form
// it depends on. Replying used to move the whole thread to the 'sent' folder,
// so a received message left the Inbox the moment it was answered and was
// then visible only in All Mail and Sent. And each path stored Message-IDs
// in its own form, so an IMAP client's copy of a sent message never matched
// the stored one and was stored — and shown — twice.

func TestNormalizeMessageID(t *testing.T) {
	cases := map[string]string{
		"":                   "",
		"  ":                 "",
		"<>":                 "",
		"abc@host":           "<abc@host>",
		"<abc@host>":         "<abc@host>",
		" <abc@host> ":       "<abc@host>",
		"<<abc@host>>":       "<abc@host>",
		"0868e75f-34f7-4dd9": "<0868e75f-34f7-4dd9>",
	}
	for in, want := range cases {
		if got := normalizeMessageID(in); got != want {
			t.Errorf("normalizeMessageID(%q) = %q, want %q", in, got, want)
		}
	}
}

// seedSentEnv returns a scope env plus mailbox A's inbox thread, the one
// inbound message in it, and an IMAP session logged in to mailbox A only.
func seedSentEnv(t *testing.T) (*scopeEnv, *core.Record, *imapSession) {
	t.Helper()
	env := setupScopeEnv(t)

	// APPEND assigns IMAP UIDs, which needs the per-mailbox UID state and the
	// raw-headers file field from migration 1713000004.
	mailboxes, err := env.app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	uidState := core.NewBaseCollection("mail_imap_mailbox_state")
	uidState.Fields.Add(&core.RelationField{Name: "mailbox", Required: true, CollectionId: mailboxes.Id, MaxSelect: 1})
	uidState.Fields.Add(&core.NumberField{Name: "uid_validity"})
	uidState.Fields.Add(&core.NumberField{Name: "uid_next"})
	if err := env.app.Save(uidState); err != nil {
		t.Fatal(err)
	}
	messages, err := env.app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	messages.Fields.Add(&core.FileField{Name: "raw_headers", MaxSelect: 1})
	if err := env.app.Save(messages); err != nil {
		t.Fatal(err)
	}

	thread, err := env.app.FindFirstRecordByFilter("mail_threads", "mailbox = {:mb}", map[string]any{"mb": env.mbA.Id})
	if err != nil {
		t.Fatal(err)
	}
	s := newIMAPSession(env.app)
	s.user = env.user
	s.mailboxIndex = []mailboxContext{{name: "personal", mailboxID: env.mbA.Id}}
	return env, thread, s
}

func TestMarkThreadSent_KeepsExistingFolder(t *testing.T) {
	env, thread, _ := seedSentEnv(t)

	if err := markThreadSent(env.app, thread.Id, env.user.Id); err != nil {
		t.Fatal(err)
	}

	state := threadStateFor(t, env.app, thread.Id, env.user.Id)
	if got := state.GetString("folder"); got != "inbox" {
		t.Errorf("folder = %q after a reply, want inbox — replying must not move the thread out of the Inbox", got)
	}
	if !state.GetBool("is_sent") || !state.GetBool("is_read") {
		t.Errorf("is_sent = %v, is_read = %v, want both true", state.GetBool("is_sent"), state.GetBool("is_read"))
	}
}

func TestMarkThreadSent_NewThreadIsFiledUnderSent(t *testing.T) {
	env, _, _ := seedSentEnv(t)
	thread, err := findOrCreateThread(env.app, env.mbA.Id, "brand new", "", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := markThreadSent(env.app, thread.Id, env.user.Id); err != nil {
		t.Fatal(err)
	}

	state := threadStateFor(t, env.app, thread.Id, env.user.Id)
	if got := state.GetString("folder"); got != "sent" {
		t.Errorf("folder = %q for a thread the user started, want sent", got)
	}
	if !state.GetBool("is_sent") {
		t.Error("is_sent = false, want true")
	}
}

const appendedReply = "Message-ID: <reply-1@example.org>\r\n" +
	"In-Reply-To: <inbound-1@elsewhere.test>\r\n" +
	"From: Scoped <personal@example.org>\r\n" +
	"To: someone@elsewhere.test\r\n" +
	"Subject: Re: in personal\r\n" +
	"Date: Wed, 30 Sep 2026 18:16:03 +0000\r\n" +
	"Content-Type: text/plain\r\n" +
	"\r\n" +
	"thanks\r\n"

// An IMAP client that sent a message over SMTP submission then APPENDs its
// copy to Sent. SMTP submission stored the message under the client's
// Message-ID, so the APPEND must find it rather than store a second copy.
func TestIMAPAppendToSent_DedupesAgainstStoredSend(t *testing.T) {
	env, thread, s := seedSentEnv(t)

	stored, err := storeMessage(env.app, thread.Id, &storedMessage{
		MessageID:      "reply-1@example.org", // bare, as go-message parses it
		SentBy:         env.user.Id,
		Subject:        "Re: in personal",
		DeliveryStatus: "sent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.GetString("message_id"); got != "<reply-1@example.org>" {
		t.Fatalf("stored message_id = %q, want the normalized <reply-1@example.org>", got)
	}
	if err := markThreadSent(env.app, thread.Id, env.user.Id); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Append("Sent", literalReader([]byte(appendedReply)), nil); err != nil {
		t.Fatalf("APPEND: %v", err)
	}

	copies, err := env.app.FindRecordsByFilter("mail_messages", "message_id = '<reply-1@example.org>'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(copies) != 1 {
		t.Fatalf("%d copies of the sent message stored, want 1", len(copies))
	}
	if got := threadStateFor(t, env.app, thread.Id, env.user.Id).GetString("folder"); got != "inbox" {
		t.Errorf("folder = %q after APPEND to Sent, want inbox", got)
	}
}

// A reply APPENDed to Sent with no prior copy joins its thread by In-Reply-To
// and leaves the thread in the Inbox.
func TestIMAPAppendToSent_ReplyKeepsThreadInInbox(t *testing.T) {
	env, thread, s := seedSentEnv(t)
	inbound, err := env.app.FindFirstRecordByFilter("mail_messages", "thread = {:t}", map[string]any{"t": thread.Id})
	if err != nil {
		t.Fatal(err)
	}
	inbound.Set("message_id", "<inbound-1@elsewhere.test>")
	if err := env.app.Save(inbound); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Append("Sent", literalReader([]byte(appendedReply)), nil); err != nil {
		t.Fatalf("APPEND: %v", err)
	}

	reply, err := env.app.FindFirstRecordByFilter("mail_messages", "message_id = '<reply-1@example.org>'")
	if err != nil {
		t.Fatalf("appended reply not stored: %v", err)
	}
	if reply.GetString("thread") != thread.Id {
		t.Fatal("appended reply did not join its thread")
	}
	if reply.GetString("sent_by") != env.user.Id {
		t.Errorf("sent_by = %q, want the session user", reply.GetString("sent_by"))
	}
	state := threadStateFor(t, env.app, thread.Id, env.user.Id)
	if state.GetString("folder") != "inbox" || !state.GetBool("is_sent") {
		t.Errorf("folder = %q, is_sent = %v; want inbox and true", state.GetString("folder"), state.GetBool("is_sent"))
	}
}

// Sent lists only the outgoing messages of its threads, and Inbox still
// lists the whole thread. Listing the received message in Sent put it in two
// folders at once, and clients showed it twice.
func TestIMAPSent_ListsOnlyOutgoingMessages(t *testing.T) {
	env, thread, s := seedSentEnv(t)
	if _, err := storeMessage(env.app, thread.Id, &storedMessage{
		MessageID:      "<reply-2@example.org>",
		SentBy:         env.user.Id,
		Subject:        "Re: in personal",
		DeliveryStatus: "sent",
	}); err != nil {
		t.Fatal(err)
	}
	if err := markThreadSent(env.app, thread.Id, env.user.Id); err != nil {
		t.Fatal(err)
	}

	sent, err := s.messagesForFolder("Sent", env.mbA.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0].GetString("message_id") != "<reply-2@example.org>" {
		t.Fatalf("Sent lists %d messages, want only the reply", len(sent))
	}

	inbox, err := s.messagesForFolder("INBOX", env.mbA.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 2 {
		t.Fatalf("INBOX lists %d messages, want the whole thread (2)", len(inbox))
	}
}

func TestIMAPSent_ExcludesTrashedThreads(t *testing.T) {
	env, thread, s := seedSentEnv(t)
	if err := markThreadSent(env.app, thread.Id, env.user.Id); err != nil {
		t.Fatal(err)
	}
	if err := setThreadFolder(env.app, thread.Id, env.user.Id, "trash"); err != nil {
		t.Fatal(err)
	}

	sent, err := s.messagesForFolder("Sent", env.mbA.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 0 {
		t.Fatalf("Sent lists %d messages from a trashed thread, want 0", len(sent))
	}
}

// Stored ids carry their angle brackets; the header and envelope writers add
// their own, so FETCH must hand them the bare id.
func TestIMAPFetch_MessageIDHasSingleBrackets(t *testing.T) {
	env, thread, s := seedSentEnv(t)
	msg, err := storeMessage(env.app, thread.Id, &storedMessage{
		MessageID: "<etPan.1@argosity.test>",
		InReplyTo: "<orig.1@argosity.test>",
		Subject:   "Hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := buildRFC5322(env.app, msg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("Message-Id: <etPan.1@argosity.test>\r\n")) {
		t.Errorf("header Message-Id not single-bracketed:\n%s", raw)
	}
	if bytes.Contains(raw, []byte("<<")) {
		t.Errorf("header has a double-bracketed id:\n%s", raw)
	}

	env2 := s.buildEnvelope(msg)
	if env2.MessageID != "etPan.1@argosity.test" {
		t.Errorf("envelope MessageID = %q, want the bare id (the writer adds brackets)", env2.MessageID)
	}
	if len(env2.InReplyTo) != 1 || strings.ContainsAny(env2.InReplyTo[0], "<>") {
		t.Errorf("envelope InReplyTo = %v, want one bare id", env2.InReplyTo)
	}
}

// Mail a mailbox sends to itself comes back through inbound delivery under the
// Message-ID the send path stored. It must not be stored twice, and it must
// reach the Inbox — the send filed its thread under Sent.
func TestHandleInbound_SelfSentMessageLandsInInboxOnce(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_selfsend_01")
	seedMember(t, app, "mb_selfsend_01", "user_alice")

	thread := newTestThread(t, app, padID("mb_selfsend_01"), "note to self")
	if _, err := storeMessage(app, thread.Id, &storedMessage{
		MessageID:      "<self-1@acme.com>",
		SentBy:         "user_alice",
		Subject:        "note to self",
		DeliveryStatus: "sent",
	}); err != nil {
		t.Fatal(err)
	}
	if err := markThreadSent(app, thread.Id, "user_alice"); err != nil {
		t.Fatal(err)
	}

	body := postmarkPayload(t, []string{"alice@acme.com"}, "note to self", "body", "<self-1@acme.com>")
	re, _ := makeInboundRequest(t, app, "tok-self", body)
	provider := &stubProvider{parse: (&PostmarkProvider{}).ParseInbound}
	if err := handleInbound(app, provider, re, "tok-self"); err != nil {
		t.Fatalf("inbound: %v", err)
	}

	msgs, _ := app.FindRecordsByFilter("mail_messages", "message_id = '<self-1@acme.com>'", "", 10, 0)
	if len(msgs) != 1 {
		t.Fatalf("%d copies stored, want 1", len(msgs))
	}
	state := threadStateFor(t, app, thread.Id, "user_alice")
	if state.GetString("folder") != "inbox" || !state.GetBool("is_sent") {
		t.Errorf("folder = %q, is_sent = %v; want inbox and still sent", state.GetString("folder"), state.GetBool("is_sent"))
	}
}

// A redelivery of a received message is found even when the stored id and
// the incoming one differ only by angle brackets.
func TestHandleInbound_DedupeIgnoresBracketForm(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_brackets_01")
	seedMember(t, app, "mb_brackets_01", "user_alice")

	thread := newTestThread(t, app, padID("mb_brackets_01"), "hello")
	if _, err := storeMessage(app, thread.Id, &storedMessage{MessageID: "plain-1@example.org", Subject: "hello"}); err != nil {
		t.Fatal(err)
	}

	body := postmarkPayload(t, []string{"alice@acme.com"}, "hello", "body", "<plain-1@example.org>")
	re, _ := makeInboundRequest(t, app, "tok-brackets", body)
	provider := &stubProvider{parse: (&PostmarkProvider{}).ParseInbound}
	if err := handleInbound(app, provider, re, "tok-brackets"); err != nil {
		t.Fatalf("inbound: %v", err)
	}

	msgs, _ := app.FindRecordsByFilter("mail_messages", "thread = {:t}", "", 10, 0, map[string]any{"t": thread.Id})
	if len(msgs) != 1 {
		t.Fatalf("%d messages in the thread, want 1", len(msgs))
	}
}

// Search's Sent filter must match the Sent view: the flag, not the folder.
func TestBuildFolderJoin_SentReadsFlag(t *testing.T) {
	params := map[string]any{}
	join := buildFolderJoin(&api.SearchRequest{Folder: "sent"}, "u1", params)
	if !strings.Contains(join, "ts.is_sent = 1") || strings.Contains(join, "ts.folder = {:folder}") {
		t.Fatalf("sent join = %q, want the is_sent flag", join)
	}
	if !strings.Contains(join, "NOT IN ('trash', 'spam')") {
		t.Fatalf("sent join = %q, want trash and spam excluded", join)
	}
}

// Saving a reply draft moves its thread to Drafts; sending it must bring the
// conversation back to the Inbox rather than leave it in Drafts with no draft.
func TestMarkThreadSent_DraftedReplyReturnsToInbox(t *testing.T) {
	env, thread, _ := seedSentEnv(t)
	if err := setThreadFolder(env.app, thread.Id, env.user.Id, "drafts"); err != nil {
		t.Fatal(err)
	}

	if err := markThreadSent(env.app, thread.Id, env.user.Id); err != nil {
		t.Fatal(err)
	}

	if got := threadStateFor(t, env.app, thread.Id, env.user.Id).GetString("folder"); got != "inbox" {
		t.Errorf("folder = %q after sending a drafted reply, want inbox", got)
	}
}

func TestMarkThreadSent_DraftedNewMessageIsFiledUnderSent(t *testing.T) {
	env, _, _ := seedSentEnv(t)
	thread, err := findOrCreateThread(env.app, env.mbA.Id, "drafted first", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeMessage(env.app, thread.Id, &storedMessage{MessageID: "<d-1@example.org>", Subject: "drafted first", DeliveryStatus: "draft"}); err != nil {
		t.Fatal(err)
	}
	if err := ensureThreadState(env.app, thread.Id, env.user.Id, "drafts", true); err != nil {
		t.Fatal(err)
	}

	if err := markThreadSent(env.app, thread.Id, env.user.Id); err != nil {
		t.Fatal(err)
	}

	if got := threadStateFor(t, env.app, thread.Id, env.user.Id).GetString("folder"); got != "sent" {
		t.Errorf("folder = %q after sending a new drafted message, want sent", got)
	}
}

// Clients autosave drafts and expunge the older copies. Expunging one must
// delete that draft, not move its whole conversation to Trash.
func TestIMAPExpunge_DraftDeletesOnlyTheDraft(t *testing.T) {
	env, thread, s := seedSentEnv(t)
	draft, err := storeMessage(env.app, thread.Id, &storedMessage{
		MessageID: "<draft-1@example.org>", Subject: "Re: in personal", DeliveryStatus: "draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := setThreadFolder(env.app, thread.Id, env.user.Id, "drafts"); err != nil {
		t.Fatal(err)
	}
	s.selectedMailboxID = env.mbA.Id
	s.selectedFolderName = "Drafts"
	s.deleted[draft.Id] = true

	if err := s.Expunge(&imapserver.ExpungeWriter{}, nil); err != nil {
		t.Fatalf("EXPUNGE: %v", err)
	}

	if _, err := env.app.FindRecordById("mail_messages", draft.Id); err == nil {
		t.Error("the expunged draft still exists")
	}
	if got := threadStateFor(t, env.app, thread.Id, env.user.Id).GetString("folder"); got == "trash" {
		t.Error("expunging a draft moved its conversation to Trash")
	}
}

// A redirect resends a received message under its original Message-ID. The
// client's copy saved to Sent is a new outgoing message; it must not be
// merged into the received one.
func TestIMAPAppendToSent_DoesNotMergeIntoReceivedMessage(t *testing.T) {
	env, thread, s := seedSentEnv(t)
	inbound, err := env.app.FindFirstRecordByFilter("mail_messages", "thread = {:t}", map[string]any{"t": thread.Id})
	if err != nil {
		t.Fatal(err)
	}
	inbound.Set("message_id", "<reply-1@example.org>")
	inbound.Set("delivery_status", "delivered")
	if err := env.app.Save(inbound); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Append("Sent", literalReader([]byte(appendedReply)), nil); err != nil {
		t.Fatalf("APPEND: %v", err)
	}

	reloaded, err := env.app.FindRecordById("mail_messages", inbound.Id)
	if err != nil {
		t.Fatal(err)
	}
	if isOutgoingMessage(reloaded) {
		t.Error("the received message was turned into a sent one")
	}
	copies, _ := env.app.FindRecordsByFilter("mail_messages", "message_id = '<reply-1@example.org>'", "", 0, 0)
	if len(copies) != 2 {
		t.Fatalf("%d messages with the id, want the received one and the sent copy", len(copies))
	}
}
