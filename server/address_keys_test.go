package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/syscfg"
	"tinycld.org/packages/mail/api"
)

// address_keys lists every address a thread used, on the thread and on each
// state row, so an address view is one filter on the thread list. These tests
// pin that each writer keeps it current and that a client cannot write it.

type addressKeysEnv struct {
	app       *tests.TestApp
	desk      *core.Record
	ann       *core.Record
	annAddr   *core.Record
	salesAddr *core.Record
}

// setupAddressKeysEnv: a shared inbox on q.example with the addresses
// ann@q.example and sales@q.example, and ann as its owner.
func setupAddressKeysEnv(t *testing.T) *addressKeysEnv {
	t.Helper()
	app := newRealSchemaApp(t)
	registerAddressHooks(app)
	registerThreadStateSyncHooks(app)
	registerThreadOwnedFieldsGuard(app)
	registerThreadMarkerHooks(app)
	q := saveRealRecord(t, app, "mail_domains", map[string]any{
		"domain": "q.example", "verified": true,
		"spf_verified": true, "dkim_verified": true, "return_path_verified": true,
	})
	desk := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "desk", "domain": q.Id, "type": "shared", "display_name": "Desk", "name": "Desk",
	})
	annAddr := saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": "ann", "domain": q.Id, "mailbox": desk.Id, "source": addressSourceManual,
	})
	salesAddr := saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": "sales", "domain": q.Id, "mailbox": desk.Id, "source": addressSourceManual,
	})
	// is_demo skips the provider call on send; the rest of the send path runs.
	ann := saveRealRecord(t, app, "users", map[string]any{
		"email": "ann@ext.example", "username": "ann", "name": "Ann", "role": "member", "is_demo": true,
	})
	saveRealRecord(t, app, "mail_mailbox_members", map[string]any{"mailbox": desk.Id, "user": ann.Id, "role": "owner"})
	return &addressKeysEnv{app: app, desk: desk, ann: ann, annAddr: annAddr, salesAddr: salesAddr}
}

// deliverTo routes one inbound message to the address and returns the stored
// message.
func (env *addressKeysEnv) deliverTo(t *testing.T, to, messageID, inReplyTo string) *core.Record {
	t.Helper()
	msg := &InboundMessage{
		MessageID: messageID,
		InReplyTo: inReplyTo,
		From:      Recipient{Email: "sender@sender.example"},
		To:        []Recipient{{Email: to}},
		Subject:   "Hello",
		TextBody:  "hi",
		Date:      "2026-10-16T10:00:00Z",
	}
	if result := routeInbound(env.app, msg); result.storedCount != 1 {
		t.Fatalf("delivery to %s stored %d copies (unknown %v), want 1", to, result.storedCount, result.unknownAddrs)
	}
	rows := deliveredMessages(t, env.app, env.desk.Id, messageID)
	if len(rows) != 1 {
		t.Fatalf("found %d copies of %s, want 1", len(rows), messageID)
	}
	return rows[0]
}

func threadKeys(t *testing.T, app core.App, threadID string) string {
	t.Helper()
	return mustRecord(t, app, "mail_threads", threadID).GetString("address_keys")
}

func stateKeys(t *testing.T, app core.App, threadID, userID string) string {
	t.Helper()
	state := findThreadState(app, threadID, userID)
	if state == nil {
		t.Fatalf("no state row for thread %s, user %s", threadID, userID)
	}
	return state.GetString("address_keys")
}

func sortedKeys(ids ...string) string {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	return strings.Join(sorted, " ")
}

func TestAddressKeys_ParseFormatRoundTrip(t *testing.T) {
	t.Parallel()
	if got := parseAddressKeys(""); len(got) != 0 {
		t.Errorf("parseAddressKeys(\"\") = %v, want empty", got)
	}
	if got := formatAddressKeys(nil); got != "" {
		t.Errorf("formatAddressKeys(nil) = %q, want empty", got)
	}
	if got := formatAddressKeys(parseAddressKeys(" b a  b ")); got != "a b" {
		t.Errorf("round trip = %q, want sorted with no duplicates %q", got, "a b")
	}
	if got := formatAddressKeys(parseAddressKeys("a b")); got != "a b" {
		t.Errorf("round trip of a formatted set = %q, want it unchanged", got)
	}
}

func TestAddressKeys_InboundAddsDeliveredTo(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	msg := env.deliverTo(t, "ann@q.example", "<in-1@sender.example>", "")
	threadID := msg.GetString("thread")

	if got := threadKeys(t, env.app, threadID); got != env.annAddr.Id {
		t.Errorf("thread address_keys = %q, want %q", got, env.annAddr.Id)
	}
	if got := stateKeys(t, env.app, threadID, env.ann.Id); got != env.annAddr.Id {
		t.Errorf("ann's state address_keys = %q, want %q", got, env.annAddr.Id)
	}
}

func TestAddressKeys_SendAddsSentFrom(t *testing.T) {
	env := setupAddressKeysEnv(t)
	syscfg.SetResolver(env.app, func(key string) string {
		if key == "mail.postmark_server_token" {
			return "test-token"
		}
		return ""
	})
	inbound := env.deliverTo(t, "ann@q.example", "<in-2@sender.example>", "")

	result, err := sendMessage(env.app, sendParams{
		Ctx:                context.Background(),
		UserID:             env.ann.Id,
		MailboxID:          env.desk.Id,
		FromAddressID:      env.salesAddr.Id,
		InReplyToMessageID: inbound.Id,
		To:                 []api.Recipient{{Email: "sender@sender.example"}},
		Subject:            "Re: Hello",
		TextBody:           "reply",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != inbound.GetString("thread") {
		t.Fatalf("reply landed on thread %s, want the inbound thread %s", result.ThreadID, inbound.GetString("thread"))
	}

	want := sortedKeys(env.annAddr.Id, env.salesAddr.Id)
	if got := threadKeys(t, env.app, result.ThreadID); got != want {
		t.Errorf("thread address_keys = %q, want %q", got, want)
	}
	if got := stateKeys(t, env.app, result.ThreadID, env.ann.Id); got != want {
		t.Errorf("ann's state address_keys = %q, want %q", got, want)
	}
}

func TestAddressKeys_NewStateCopiesThread(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	msg := env.deliverTo(t, "ann@q.example", "<in-3@sender.example>", "")
	bob := saveRealRecord(t, env.app, "users", map[string]any{
		"email": "bob@ext.example", "username": "bob", "name": "Bob", "role": "member",
	})
	saveRealRecord(t, env.app, "mail_mailbox_members", map[string]any{"mailbox": env.desk.Id, "user": bob.Id, "role": "member"})

	if err := backfillMemberThreadState(context.Background(), env.app, env.desk.Id, bob.Id); err != nil {
		t.Fatal(err)
	}

	if got := stateKeys(t, env.app, msg.GetString("thread"), bob.Id); got != env.annAddr.Id {
		t.Errorf("bob's new state address_keys = %q, want the thread's %q", got, env.annAddr.Id)
	}
}

func TestAddressKeys_ClientCannotWrite(t *testing.T) {
	t.Parallel()
	var env *addressKeysEnv
	var threadID string
	scenario := tests.ApiScenario{
		Name:            "a member PATCHes address_keys on their own state row",
		Method:          http.MethodPatch,
		Body:            strings.NewReader(`{"address_keys":"zzz","is_starred":true}`),
		Headers:         map[string]string{"Content-Type": "application/json"},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"is_starred":true`},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		env = setupAddressKeysEnv(t)
		threadID = env.deliverTo(t, "ann@q.example", "<in-4@sender.example>", "").GetString("thread")
		state := findThreadState(env.app, threadID, env.ann.Id)
		if state == nil {
			t.Fatal("no state row for ann")
		}
		scenario.URL = "/api/collections/mail_thread_state/records/" + state.Id
		scenario.Headers["Authorization"] = authTokenFor(t, env.ann)
		return env.app
	}
	scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
		if got := stateKeys(t, app, threadID, env.ann.Id); got != env.annAddr.Id {
			t.Errorf("address_keys = %q after the PATCH, want the server's %q", got, env.annAddr.Id)
		}
	}
	scenario.Test(t)
}

// A full set must not fail the thread save: that would fail the delivery.
// The set is kept as it is, and the message is still stored.
func TestAddressKeys_FullSetKeepsExistingAndDelivers(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	first := env.deliverTo(t, "ann@q.example", "<in-5@sender.example>", "")
	threadID := first.GetString("thread")

	fill := make([]string, 0, 124)
	for i := range 124 {
		fill = append(fill, fmt.Sprintf("%015d", i))
	}
	full := formatAddressKeys(append(fill, env.annAddr.Id))
	if len(full)+1+len(env.salesAddr.Id) <= addressKeysMaxLen {
		t.Fatalf("precondition: a set of %d characters still has room for one more id", len(full))
	}
	thread := mustRecord(t, env.app, "mail_threads", threadID)
	thread.Set("address_keys", full)
	if err := env.app.Save(thread); err != nil {
		t.Fatal(err)
	}

	if err := addAddressKeys(env.app, threadID, env.salesAddr.Id); err != nil {
		t.Fatalf("addAddressKeys on a full set: %v", err)
	}
	if got := threadKeys(t, env.app, threadID); got != full {
		t.Errorf("full set changed to %d characters, want it kept", len(got))
	}

	reply := env.deliverTo(t, "sales@q.example", "<in-6@sender.example>", "<in-5@sender.example>")
	if reply.GetString("thread") != threadID {
		t.Fatalf("reply landed on thread %s, want %s", reply.GetString("thread"), threadID)
	}
	if got := threadKeys(t, env.app, threadID); got != full {
		t.Errorf("delivery changed the full set to %d characters, want it kept", len(got))
	}
	if got := stateKeys(t, env.app, threadID, env.ann.Id); got != full {
		t.Errorf("ann's state address_keys has %d characters, want the thread's full set", len(got))
	}
}

// A draft saved again under another From address adds that address, so a
// view of either address lists the draft's thread.
func TestAddressKeys_DraftUpdateAddsNewFrom(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	re, rec := draftEvent(t, env.app, env.ann, map[string]any{
		"mailbox_id": env.desk.Id, "from_address_id": env.annAddr.Id, "subject": "Plan", "text_body": "draft",
	})
	if err := handleDraft(env.app, re); err != nil {
		t.Fatal(err)
	}
	var created api.SendEmailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if got := threadKeys(t, env.app, created.ThreadID); got != env.annAddr.Id {
		t.Fatalf("new draft's thread address_keys = %q, want %q", got, env.annAddr.Id)
	}

	re, _ = draftEvent(t, env.app, env.ann, map[string]any{
		"mailbox_id": env.desk.Id, "message_id": created.MessageID, "from_address_id": env.salesAddr.Id,
		"subject": "Plan", "text_body": "draft 2",
	})
	if err := handleDraft(env.app, re); err != nil {
		t.Fatal(err)
	}

	want := sortedKeys(env.annAddr.Id, env.salesAddr.Id)
	if got := threadKeys(t, env.app, created.ThreadID); got != want {
		t.Errorf("thread address_keys after the From change = %q, want %q", got, want)
	}
	if got := stateKeys(t, env.app, created.ThreadID, env.ann.Id); got != want {
		t.Errorf("ann's state address_keys = %q, want %q", got, want)
	}
}

// An SMTP send whose Message-ID the client already APPENDed to Sent is
// recorded on that stored copy, so its thread gains the address it was sent
// from.
func TestAddressKeys_SMTPDedupAddsSentFrom(t *testing.T) {
	env := setupAddressKeysEnv(t)
	syscfg.SetResolver(env.app, func(key string) string {
		if key == "mail.postmark_server_token" {
			return "test-token"
		}
		return ""
	})
	thread, err := findOrCreateThread(env.app, env.desk.Id, "Plan", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeMessage(env.app, thread.Id, &storedMessage{
		MessageID: "<dup-1@q.example>", SentBy: env.ann.Id, SenderEmail: "sales@q.example",
		Subject: "Plan", Date: "2026-10-16T10:00:00Z", DeliveryStatus: "sent",
	}); err != nil {
		t.Fatal(err)
	}
	if got := threadKeys(t, env.app, thread.Id); got != "" {
		t.Fatalf("precondition: the APPENDed copy names no address, but the thread has %q", got)
	}

	s := &smtpSession{app: env.app, user: env.ann}
	if err := s.Mail("sales@q.example", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Rcpt("friend@example.net", nil); err != nil {
		t.Fatal(err)
	}
	raw := "From: sales@q.example\r\nTo: friend@example.net\r\nSubject: Plan\r\nMessage-ID: <dup-1@q.example>\r\n\r\nbody\r\n"
	if err := s.Data(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	if got := threadKeys(t, env.app, thread.Id); got != env.salesAddr.Id {
		t.Errorf("thread address_keys = %q, want the sending address %q", got, env.salesAddr.Id)
	}
}

// A member may write mail_threads, and a thread's keys reach every member's
// state row, so the thread column is server-owned too: an update keeps the
// stored value and a create starts empty.
func TestAddressKeys_ClientCannotWriteThread(t *testing.T) {
	t.Parallel()
	var env *addressKeysEnv
	var threadID string
	update := tests.ApiScenario{
		Name:            "a member PATCHes address_keys on a thread",
		Method:          http.MethodPatch,
		Body:            strings.NewReader(`{"address_keys":"zzz","subject":"edited"}`),
		Headers:         map[string]string{"Content-Type": "application/json"},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"subject":"edited"`},
	}
	update.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		env = setupAddressKeysEnv(t)
		threadID = env.deliverTo(t, "ann@q.example", "<in-7@sender.example>", "").GetString("thread")
		update.URL = "/api/collections/mail_threads/records/" + threadID
		update.Headers["Authorization"] = authTokenFor(t, env.ann)
		return env.app
	}
	update.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
		if got := threadKeys(t, app, threadID); got != env.annAddr.Id {
			t.Errorf("thread address_keys = %q after the PATCH, want the server's %q", got, env.annAddr.Id)
		}
		if got := stateKeys(t, app, threadID, env.ann.Id); got != env.annAddr.Id {
			t.Errorf("ann's state address_keys = %q after the PATCH, want %q", got, env.annAddr.Id)
		}
	}
	update.Test(t)

	create := tests.ApiScenario{
		Name:            "a member creates a thread with address_keys",
		Method:          http.MethodPost,
		URL:             "/api/collections/mail_threads/records",
		Headers:         map[string]string{"Content-Type": "application/json"},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"address_keys":""`},
	}
	create.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		env = setupAddressKeysEnv(t)
		create.Body = strings.NewReader(`{"mailbox":"` + env.desk.Id + `","subject":"New","address_keys":"zzz"}`)
		create.Headers["Authorization"] = authTokenFor(t, env.ann)
		return env.app
	}
	create.Test(t)
}

// threadBroadcast is what the last mail_threads update carried. The realtime
// broadcast sends that in-memory record as it is, so a stale copy saved last
// would show clients stale markers and keys although the database is right.
type threadBroadcast struct {
	hasDraft, hasAttachments bool
	addressKeys              string
}

func captureLastThreadUpdate(app core.App) map[string]threadBroadcast {
	last := map[string]threadBroadcast{}
	app.OnRecordAfterUpdateSuccess("mail_threads").BindFunc(func(e *core.RecordEvent) error {
		last[e.Record.Id] = threadBroadcast{
			hasDraft:       e.Record.GetBool("has_draft"),
			hasAttachments: e.Record.GetBool("has_attachments"),
			addressKeys:    e.Record.GetString("address_keys"),
		}
		return e.Next()
	})
	return last
}

func TestAddressKeys_SendBroadcastCarriesMarkersAndKeys(t *testing.T) {
	env := setupAddressKeysEnv(t)
	syscfg.SetResolver(env.app, func(key string) string {
		if key == "mail.postmark_server_token" {
			return "test-token"
		}
		return ""
	})
	last := captureLastThreadUpdate(env.app)

	result, err := sendMessage(env.app, sendParams{
		Ctx:           context.Background(),
		UserID:        env.ann.Id,
		MailboxID:     env.desk.Id,
		FromAddressID: env.salesAddr.Id,
		To:            []api.Recipient{{Email: "friend@example.net"}},
		Subject:       "Report",
		TextBody:      "attached",
		Attachments: []Attachment{{
			Name: "report.txt", ContentType: "text/plain", Content: base64.StdEncoding.EncodeToString([]byte("numbers")),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, ok := last[result.ThreadID]
	if !ok {
		t.Fatal("the send never updated the thread")
	}
	if !got.hasAttachments {
		t.Error("the last thread update carried has_attachments=false; clients would show no attachment marker")
	}
	if got.addressKeys != env.salesAddr.Id {
		t.Errorf("the last thread update carried address_keys %q, want %q", got.addressKeys, env.salesAddr.Id)
	}
}

func TestAddressKeys_NewDraftBroadcastCarriesMarkersAndKeys(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	last := captureLastThreadUpdate(env.app)
	re, rec := draftEvent(t, env.app, env.ann, map[string]any{
		"mailbox_id": env.desk.Id, "from_address_id": env.annAddr.Id, "subject": "Plan", "text_body": "draft",
	})
	if err := handleDraft(env.app, re); err != nil {
		t.Fatal(err)
	}
	var created api.SendEmailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	got, ok := last[created.ThreadID]
	if !ok {
		t.Fatal("the draft save never updated the thread")
	}
	if !got.hasDraft {
		t.Error("the last thread update carried has_draft=false; clients would show no draft marker")
	}
	if got.addressKeys != env.annAddr.Id {
		t.Errorf("the last thread update carried address_keys %q, want %q", got.addressKeys, env.annAddr.Id)
	}
}

// The draft update in place saves the thread too; its last update must carry
// the marker and the new From address.
func TestAddressKeys_DraftUpdateBroadcastCarriesMarkersAndKeys(t *testing.T) {
	t.Parallel()
	env := setupAddressKeysEnv(t)
	re, rec := draftEvent(t, env.app, env.ann, map[string]any{
		"mailbox_id": env.desk.Id, "from_address_id": env.annAddr.Id, "subject": "Plan", "text_body": "draft",
	})
	if err := handleDraft(env.app, re); err != nil {
		t.Fatal(err)
	}
	var created api.SendEmailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	last := captureLastThreadUpdate(env.app)

	re, _ = draftEvent(t, env.app, env.ann, map[string]any{
		"mailbox_id": env.desk.Id, "message_id": created.MessageID, "from_address_id": env.salesAddr.Id,
		"subject": "Plan", "text_body": "draft 2",
	})
	if err := handleDraft(env.app, re); err != nil {
		t.Fatal(err)
	}

	got, ok := last[created.ThreadID]
	if !ok {
		t.Fatal("the draft update never updated the thread")
	}
	if !got.hasDraft {
		t.Error("the last thread update carried has_draft=false")
	}
	if want := sortedKeys(env.annAddr.Id, env.salesAddr.Id); got.addressKeys != want {
		t.Errorf("the last thread update carried address_keys %q, want %q", got.addressKeys, want)
	}
}
