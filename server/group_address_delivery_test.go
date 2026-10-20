package mail

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
)

type groupDeliverySeed struct {
	app    *tests.TestApp
	domain *core.Record
	ann    *core.Record
	bob    *core.Record
	cy     *core.Record
	sales  *core.Record
	// salesAddr is sales@q.example, the address of the group Sales (ann, bob).
	salesAddr *core.Record
}

// seedGroupDelivery makes an everyone domain q.example, three people with
// their own inboxes (ann, bob, cy), and the group Sales (ann, bob) with the
// address sales@q.example.
func seedGroupDelivery(t *testing.T) *groupDeliverySeed {
	t.Helper()
	app := newGroupAddressTestApp(t)
	registerThreadStateSyncHooks(app)
	s := &groupDeliverySeed{app: app}
	s.domain = saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "q.example", "verified": true})
	s.ann = seedGroupUser(t, app, "ann")
	s.bob = seedGroupUser(t, app, "bob")
	s.cy = seedGroupUser(t, app, "cy")
	s.sales = seedGroup(t, app, "Sales", s.ann, s.bob)
	s.salesAddr = saveGroupAddress(t, app, "sales", s.domain, s.sales)
	return s
}

func (s *groupDeliverySeed) inboxOf(t *testing.T, user *core.Record) *core.Record {
	t.Helper()
	identity := identityOf(t, s.app, user.Id)
	if identity == nil || identity.GetString("inbox") == "" {
		t.Fatalf("seed: %s has no inbox", user.GetString("username"))
	}
	return mustRecord(t, s.app, "mail_mailboxes", identity.GetString("inbox"))
}

func (s *groupDeliverySeed) ownAddress(t *testing.T, user *core.Record) *core.Record {
	t.Helper()
	row, err := findAddressRow(s.app, user.GetString("username"), s.domain.Id)
	if err != nil || row == nil {
		t.Fatalf("seed: %s has no address on q.example: %v", user.GetString("username"), err)
	}
	return row
}

func groupMessage(messageID string, to, cc []string) *InboundMessage {
	msg := &InboundMessage{
		From:      Recipient{Name: "Sender", Email: "sender@sender.example"},
		Subject:   "To the group",
		TextBody:  "body",
		MessageID: messageID,
		Date:      time.Now().UTC().Format(time.RFC3339),
	}
	for _, email := range to {
		msg.To = append(msg.To, Recipient{Email: email})
	}
	for _, email := range cc {
		msg.Cc = append(msg.Cc, Recipient{Email: email})
	}
	return msg
}

// messagesIn lists every message stored in an inbox. It does not filter by
// Message-ID, so it also counts the copies of a message that has none.
func messagesIn(t *testing.T, app core.App, mailbox *core.Record) []*core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_messages", "thread.mailbox = {:mailbox}", "", 0, 0,
		map[string]any{"mailbox": mailbox.Id})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// requireOneCopy checks that the person's inbox holds exactly one message,
// delivered to the given address and filed in the person's Inbox.
func (s *groupDeliverySeed) requireOneCopy(t *testing.T, user, deliveredTo *core.Record) *core.Record {
	t.Helper()
	name := user.GetString("username")
	rows := messagesIn(t, s.app, s.inboxOf(t, user))
	if len(rows) != 1 {
		t.Fatalf("%s has %d copies, want 1", name, len(rows))
	}
	if got := rows[0].GetString("delivered_to"); got != deliveredTo.Id {
		t.Fatalf("%s's copy: delivered_to = %q, want %q (%s)", name, got, deliveredTo.Id, deliveredTo.GetString("local_part"))
	}
	state := findThreadState(s.app, rows[0].GetString("thread"), user.Id)
	if state == nil || state.GetString("folder") != "inbox" {
		t.Fatalf("%s's thread state = %v, want one in inbox", name, state)
	}
	return rows[0]
}

func (s *groupDeliverySeed) requireNoCopy(t *testing.T, user *core.Record) {
	t.Helper()
	if rows := messagesIn(t, s.app, s.inboxOf(t, user)); len(rows) != 0 {
		t.Fatalf("%s has %d copies, want none", user.GetString("username"), len(rows))
	}
}

// postWebhook runs msg through the inbound webhook handler and returns the
// HTTP status it answers with.
func postWebhook(t *testing.T, app core.App, msg *InboundMessage) int {
	t.Helper()
	re, rec := makeInboundRequest(t, app, "tok-group", []byte("{}"))
	provider := &stubProvider{parse: func([]byte) (*InboundMessage, error) { return msg, nil }}
	err := handleInbound(app, provider, re, "tok-group")
	if err == nil {
		return rec.Code
	}
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("handleInbound = %T %v, want an *router.ApiError", err, err)
	}
	return apiErr.Status
}

func TestGroupAddress_WebhookDeliversToEachMember(t *testing.T) {
	s := seedGroupDelivery(t)
	msg := groupMessage("<webhook-group-1@sender.example>", []string{"sales@q.example"}, nil)
	if status := postWebhook(t, s.app, msg); status != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200", status)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireOneCopy(t, s.bob, s.salesAddr)
	s.requireNoCopy(t, s.cy)
}

func TestGroupAddress_SMTPDeliversToEachMember(t *testing.T) {
	t.Parallel()
	s := seedGroupDelivery(t)
	session := &smtpInboundSession{app: s.app, hostname: "mx.example"}
	if err := session.Mail("sender@sender.example", nil); err != nil {
		t.Fatal(err)
	}
	if err := session.Rcpt("sales@q.example", nil); err != nil {
		t.Fatalf("RCPT to a group address was refused: %v", err)
	}
	raw := "From: Sender <sender@sender.example>\r\nTo: sales@q.example\r\nSubject: Via SMTP\r\n" +
		"Message-ID: <smtp-group-1@sender.example>\r\nDate: Thu, 15 Oct 2026 20:00:00 +0000\r\n\r\nBody\r\n"
	if err := session.Data(stringsReader(raw)); err != nil {
		t.Fatalf("DATA: %v", err)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireOneCopy(t, s.bob, s.salesAddr)
	s.requireNoCopy(t, s.cy)
}

func TestGroupAddress_FetcherDeliversToEachMember(t *testing.T) {
	s := seedGroupDelivery(t)
	msg := groupMessage("<fetch-group-1@sender.example>", []string{"sales@q.example"}, nil)
	if err := dispatchInbound(s.app, msg); err != nil {
		t.Fatalf("dispatchInbound: %v", err)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireOneCopy(t, s.bob, s.salesAddr)
	s.requireNoCopy(t, s.cy)
}

// ann is a direct recipient and a member of the list on Cc: one copy, filed
// under her own address (the first that matched). bob gets the list copy.
func TestGroupAddress_DirectAndListStoredOnce(t *testing.T) {
	cases := []struct {
		name      string
		messageID string
	}{
		{"with Message-ID", "<direct-and-list-1@sender.example>"},
		// No Message-ID: the per-inbox Message-ID check cannot see the
		// duplicate, so only the fan-out dedupe keeps it to one copy.
		{"no Message-ID", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := seedGroupDelivery(t)
			msg := groupMessage(c.messageID, []string{"ann@q.example"}, []string{"sales@q.example"})
			if status := postWebhook(t, s.app, msg); status != http.StatusOK {
				t.Fatalf("webhook status = %d, want 200", status)
			}
			s.requireOneCopy(t, s.ann, s.ownAddress(t, s.ann))
			s.requireOneCopy(t, s.bob, s.salesAddr)
		})
	}
}

func TestGroupAddress_TwoListsSharedMemberOnce(t *testing.T) {
	s := seedGroupDelivery(t)
	support := seedGroup(t, s.app, "Support", s.bob, s.cy)
	supportAddr := saveGroupAddress(t, s.app, "support", s.domain, support)

	msg := groupMessage("", []string{"sales@q.example", "support@q.example"}, nil)
	if status := postWebhook(t, s.app, msg); status != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200", status)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireOneCopy(t, s.bob, s.salesAddr)
	s.requireOneCopy(t, s.cy, supportAddr)
}

// A group nobody can receive for is the same as an address that does not
// exist: the sender gets a bounce, not silence.
func TestGroupAddress_EmptyGroupRefused(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(t *testing.T, s *groupDeliverySeed) *core.Record
	}{
		{"no members", func(t *testing.T, s *groupDeliverySeed) *core.Record {
			return seedGroup(t, s.app, "Quiet")
		}},
		{"only a retired member", func(t *testing.T, s *groupDeliverySeed) *core.Record {
			dan := seedGroupUser(t, s.app, "dan")
			identity := identityOf(t, s.app, dan.Id)
			identity.Set("retired", true)
			if err := s.app.Save(identity); err != nil {
				t.Fatalf("retire dan: %v", err)
			}
			return seedGroup(t, s.app, "Quiet", dan)
		}},
	}
	for _, c := range cases {
		t.Run(c.name+"/smtp", func(t *testing.T) {
			s := seedGroupDelivery(t)
			saveGroupAddress(t, s.app, "quiet", s.domain, c.prepare(t, s))
			session := &smtpInboundSession{app: s.app, hostname: "mx.example"}
			if err := session.Mail("sender@sender.example", nil); err != nil {
				t.Fatal(err)
			}
			var refusal *smtp.SMTPError
			err := session.Rcpt("quiet@q.example", nil)
			if !errors.As(err, &refusal) || refusal.Code != 550 || refusal.EnhancedCode != (smtp.EnhancedCode{5, 1, 1}) {
				t.Fatalf("RCPT to an empty group = %v, want 550 5.1.1", err)
			}
		})
		t.Run(c.name+"/webhook", func(t *testing.T) {
			s := seedGroupDelivery(t)
			saveGroupAddress(t, s.app, "quiet", s.domain, c.prepare(t, s))
			msg := groupMessage("<quiet-1@sender.example>", []string{"quiet@q.example"}, nil)
			if status := postWebhook(t, s.app, msg); status != http.StatusForbidden {
				t.Fatalf("webhook status = %d, want 403", status)
			}
			stored, err := s.app.FindAllRecords("mail_messages")
			if err != nil {
				t.Fatal(err)
			}
			if len(stored) != 0 {
				t.Fatalf("%d messages stored, want none", len(stored))
			}
		})
	}
}

// A reply to list mail goes to the person who wrote it: delivery rewrites
// neither From nor Reply-To.
func TestGroupAddress_ReplyGoesToSender(t *testing.T) {
	s := seedGroupDelivery(t)
	msg := groupMessage("<reply-1@sender.example>", []string{"sales@q.example"}, nil)
	replyTo := Header{Name: "Reply-To", Value: "Sender Replies <replies@sender.example>"}
	msg.Headers = []Header{replyTo}
	from := msg.From

	if status := postWebhook(t, s.app, msg); status != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200", status)
	}
	for _, user := range []*core.Record{s.ann, s.bob} {
		stored := s.requireOneCopy(t, user, s.salesAddr)
		if stored.GetString("sender_email") != from.Email || stored.GetString("sender_name") != from.Name {
			t.Fatalf("%s's copy is from %q <%s>, want %q <%s>", user.GetString("username"),
				stored.GetString("sender_name"), stored.GetString("sender_email"), from.Name, from.Email)
		}
	}
	if msg.From != from || !slices.Equal(msg.Headers, []Header{replyTo}) {
		t.Fatalf("delivery changed the message: From %v, headers %v", msg.From, msg.Headers)
	}
}

func TestDeliveryTargets(t *testing.T) {
	t.Parallel()
	s := seedGroupDelivery(t)

	t.Run("inbox address", func(t *testing.T) {
		targets, err := deliveryTargets(s.app, "ann", "q.example")
		if err != nil {
			t.Fatal(err)
		}
		if len(targets) != 1 || targets[0].Mailbox.Id != s.inboxOf(t, s.ann).Id ||
			targets[0].AddressID != s.ownAddress(t, s.ann).Id {
			t.Fatalf("targets = %v, want ann's inbox through her own address", targets)
		}
	})
	t.Run("group address is ordered by user id", func(t *testing.T) {
		targets, err := deliveryTargets(s.app, "Sales", "Q.example")
		if err != nil {
			t.Fatal(err)
		}
		users := []*core.Record{s.ann, s.bob}
		slices.SortFunc(users, func(a, b *core.Record) int { return strings.Compare(a.Id, b.Id) })
		if len(targets) != 2 {
			t.Fatalf("got %d targets, want 2", len(targets))
		}
		for i, user := range users {
			if targets[i].Mailbox.Id != s.inboxOf(t, user).Id || targets[i].AddressID != s.salesAddr.Id {
				t.Fatalf("target %d = %v, want %s's inbox through the group address", i, targets[i], user.GetString("username"))
			}
		}
	})
	t.Run("unknown address", func(t *testing.T) {
		if _, err := deliveryTargets(s.app, "nobody", "q.example"); !errors.Is(err, errAddressNotFound) {
			t.Fatalf("deliveryTargets = %v, want errAddressNotFound", err)
		}
		if _, err := deliveryTargets(s.app, "ann", "other.example"); !errors.Is(err, errAddressNotFound) {
			t.Fatalf("deliveryTargets on an unknown domain = %v, want errAddressNotFound", err)
		}
	})
	t.Run("targetSet keeps the first per inbox", func(t *testing.T) {
		inbox := s.inboxOf(t, s.ann)
		var set targetSet
		if !set.add(deliveryTarget{Mailbox: inbox, AddressID: "first"}) {
			t.Fatal("the first target was not new")
		}
		if set.add(deliveryTarget{Mailbox: inbox, AddressID: "second"}) {
			t.Fatal("a second target for the same inbox was new")
		}
		if !set.add(deliveryTarget{Mailbox: s.inboxOf(t, s.bob), AddressID: "first"}) {
			t.Fatal("another inbox's target was not new")
		}
	})
}

// failStoresIn makes every message store in the inbox fail until the
// returned function is called, so one target of a fan-out fails while the
// others store.
func failStoresIn(t *testing.T, app core.App, inbox *core.Record) (stop func()) {
	t.Helper()
	var failing atomic.Bool
	failing.Store(true)
	app.OnRecordCreate("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		if !failing.Load() {
			return e.Next()
		}
		thread, err := e.App.FindRecordById("mail_threads", e.Record.GetString("thread"))
		if err == nil && thread.GetString("mailbox") == inbox.Id {
			return errors.New("injected storage failure")
		}
		return e.Next()
	})
	return func() { failing.Store(false) }
}

// A store that fails in one inbox leaves the IMAP message unseen, so the next
// tick stores the missing copy. The inbox that stored skips the retry by
// Message-ID.
func TestGroupAddress_FetcherPartialFailureRetries(t *testing.T) {
	s := seedGroupDelivery(t)
	stop := failStoresIn(t, s.app, s.inboxOf(t, s.bob))
	msg := groupMessage("<fetch-partial-1@sender.example>", []string{"sales@q.example"}, nil)

	if err := dispatchInbound(s.app, msg); err == nil {
		t.Fatal("dispatchInbound = nil after a store failed, want the error so the UID stays unseen")
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireNoCopy(t, s.bob)

	stop()
	if err := dispatchInbound(s.app, msg); err != nil {
		t.Fatalf("retry: %v", err)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireOneCopy(t, s.bob, s.salesAddr)
}

// Without a Message-ID a retry would store a second copy for ann, so the
// partial delivery is kept and logged as an error.
func TestGroupAddress_FetcherPartialFailureNoMessageID(t *testing.T) {
	s := seedGroupDelivery(t)
	logs := captureLogs(t)
	failStoresIn(t, s.app, s.inboxOf(t, s.bob))
	msg := groupMessage("", []string{"sales@q.example"}, nil)

	if err := dispatchInbound(s.app, msg); err != nil {
		t.Fatalf("dispatchInbound = %v, want nil: a retry would duplicate ann's copy", err)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireNoCopy(t, s.bob)
	requireErrorLog(t, logs, "imap fetcher: storage failed")
	requireErrorLog(t, logs, "imap fetcher: kept a partial delivery")
}

func requireErrorLog(t *testing.T, logs *bytes.Buffer, text string) {
	t.Helper()
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, text) && strings.Contains(line, `"level":"ERROR"`) {
			return
		}
	}
	t.Fatalf("no error log %q; logs:\n%s", text, logs.String())
}

func smtpDeliver(t *testing.T, app core.App, rcpt, raw string) error {
	t.Helper()
	session := &smtpInboundSession{app: app, hostname: "mx.example"}
	if err := session.Mail("sender@sender.example", nil); err != nil {
		t.Fatal(err)
	}
	if err := session.Rcpt(rcpt, nil); err != nil {
		t.Fatalf("RCPT %s: %v", rcpt, err)
	}
	return session.Data(stringsReader(raw))
}

func salesRaw(messageIDHeader string) string {
	return "From: Sender <sender@sender.example>\r\nTo: sales@q.example\r\nSubject: Via SMTP\r\n" +
		messageIDHeader + "Date: Thu, 15 Oct 2026 20:00:00 +0000\r\n\r\nBody\r\n"
}

// A store that fails in one inbox defers the whole message (451). The MTA's
// retry stores bob's copy, and ann's inbox skips it by Message-ID.
func TestGroupAddress_SMTPPartialFailureDefers(t *testing.T) {
	t.Parallel()
	s := seedGroupDelivery(t)
	stop := failStoresIn(t, s.app, s.inboxOf(t, s.bob))
	raw := salesRaw("Message-ID: <smtp-partial-1@sender.example>\r\n")

	var refusal *smtp.SMTPError
	err := smtpDeliver(t, s.app, "sales@q.example", raw)
	if !errors.As(err, &refusal) || refusal.Code != 451 || refusal.EnhancedCode != (smtp.EnhancedCode{4, 3, 0}) {
		t.Fatalf("DATA with one failed inbox = %v, want 451 4.3.0", err)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireNoCopy(t, s.bob)

	stop()
	if err := smtpDeliver(t, s.app, "sales@q.example", raw); err != nil {
		t.Fatalf("retry: %v", err)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireOneCopy(t, s.bob, s.salesAddr)
}

// Without a Message-ID a retry would store a second copy for ann, so the
// partial delivery is accepted and logged as an error.
func TestGroupAddress_SMTPPartialFailureNoMessageID(t *testing.T) {
	s := seedGroupDelivery(t)
	logs := captureLogs(t)
	failStoresIn(t, s.app, s.inboxOf(t, s.bob))

	if err := smtpDeliver(t, s.app, "sales@q.example", salesRaw("")); err != nil {
		t.Fatalf("DATA = %v, want 250: a retry would duplicate ann's copy", err)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireNoCopy(t, s.bob)
	requireErrorLog(t, logs, "inbound SMTP: storage failed")
	requireLog(t, logs, "ERROR", "inbound SMTP: accepted a partial delivery", `"stored":1`, `"failed":1`)
}

// The fetcher has no sender to bounce to, so a recipient it cannot deliver
// for is skipped. Each skip is logged with the address and the reason, as
// the webhook logs an unknown recipient.
func TestGroupAddress_FetcherLogsSkippedRecipients(t *testing.T) {
	s := seedGroupDelivery(t)
	saveGroupAddress(t, s.app, "quiet", s.domain, seedGroup(t, s.app, "Quiet"))
	logs := captureLogs(t)
	msg := groupMessage("<skip-1@sender.example>",
		[]string{"nobody@q.example", "quiet@q.example", "sales@q.example"}, nil)

	if err := dispatchInbound(s.app, msg); err != nil {
		t.Fatalf("dispatchInbound = %v", err)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	requireLog(t, logs, "INFO", "imap fetcher: skipped recipient",
		`"address":"nobody@q.example"`, `"reason":`)
	requireLog(t, logs, "INFO", "imap fetcher: skipped recipient",
		`"address":"quiet@q.example"`, "a group with no member who can receive mail")
}

// requireLog finds one log line at level with text and every attr in it.
func requireLog(t *testing.T, logs *bytes.Buffer, level, text string, attrs ...string) {
	t.Helper()
	for _, line := range strings.Split(logs.String(), "\n") {
		if !strings.Contains(line, text) || !strings.Contains(line, `"level":"`+level+`"`) {
			continue
		}
		if !slices.ContainsFunc(attrs, func(a string) bool { return !strings.Contains(line, a) }) {
			return
		}
	}
	t.Fatalf("no %s log %q with %v; logs:\n%s", level, text, attrs, logs.String())
}

// Every RCPT runs a lookup and holds the inboxes it reaches, so a session
// cannot repeat a group address without limit.
func TestInboundSMTP_RecipientLimit(t *testing.T) {
	s := seedGroupDelivery(t)
	server := newInboundSMTPServer(s.app, "mx.example", nil)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(ln) }()
	t.Cleanup(func() { _ = server.Close() })

	client, err := smtp.Dial(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Mail("sender@sender.example", nil); err != nil {
		t.Fatal(err)
	}
	for i := range maxInboundRecipients {
		if err := client.Rcpt("sales@q.example", nil); err != nil {
			t.Fatalf("RCPT %d refused: %v", i+1, err)
		}
	}
	var refusal *smtp.SMTPError
	err = client.Rcpt("sales@q.example", nil)
	if !errors.As(err, &refusal) || refusal.Code != 452 {
		t.Fatalf("RCPT %d = %v, want 452 too many recipients", maxInboundRecipients+1, err)
	}
}

// Only people who can receive mail get a group copy: a guest is left out,
// as is a person with no inbox. A disabled person still receives, as for
// every other address (see processInboundForMailbox): the mail waits for
// the account to be enabled again.
func TestGroupAddress_MemberConstraints(t *testing.T) {
	s := seedGroupDelivery(t)

	// gus joins as a member, then is made a guest. In production core's
	// groups hook removes a demoted user from every group, so a guest member
	// row exists only if that removal is missed. This app does not bind
	// core's groups hooks, so the row stays. The demotion keeps his identity
	// and inbox, so only delivery's own guest rule leaves him out.
	gus := seedGroupUser(t, s.app, "gus")
	saveRealRecord(t, s.app, "group_members", map[string]any{"group": s.sales.Id, "user": gus.Id})
	gus.Set("role", "guest")
	if err := s.app.Save(gus); err != nil {
		t.Fatalf("demote gus: %v", err)
	}
	if identity := identityOf(t, s.app, gus.Id); identity == nil || identity.GetString("inbox") == "" || identity.GetBool("retired") {
		t.Fatalf("precondition: gus's identity = %v, want live with an inbox", identity)
	}
	gusInbox := s.inboxOf(t, gus)

	dee := seedGroupUser(t, s.app, "dee")
	saveRealRecord(t, s.app, "group_members", map[string]any{"group": s.sales.Id, "user": dee.Id})
	dee.Set("disabled", true)
	if err := s.app.Save(dee); err != nil {
		t.Fatalf("disable dee: %v", err)
	}
	if !reloaded(t, s.app, dee).GetBool("disabled") {
		t.Fatal("precondition: dee is not disabled")
	}

	// A live identity with no inbox is not made by any server path; the
	// direct save stands for an inbox lost before the repair runs.
	nia := seedGroupUser(t, s.app, "nia")
	saveRealRecord(t, s.app, "group_members", map[string]any{"group": s.sales.Id, "user": nia.Id})
	niaInbox := s.inboxOf(t, nia)
	identity := identityOf(t, s.app, nia.Id)
	identity.Set("inbox", "")
	if err := s.app.Save(identity); err != nil {
		t.Fatalf("clear nia's inbox: %v", err)
	}

	msg := groupMessage("<constraints-1@sender.example>", []string{"sales@q.example"}, nil)
	if status := postWebhook(t, s.app, msg); status != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200", status)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireOneCopy(t, s.bob, s.salesAddr)
	s.requireOneCopy(t, dee, s.salesAddr)
	if rows := messagesIn(t, s.app, gusInbox); len(rows) != 0 {
		t.Fatalf("guest gus has %d copies, want none", len(rows))
	}
	if rows := messagesIn(t, s.app, niaInbox); len(rows) != 0 {
		t.Fatalf("nia's old inbox has %d copies, want none", len(rows))
	}
}

// After a partial fan-out to a large group, a retry must find each inbox's
// own copy. More than 10 copies share the Message-ID, so a capped search
// across all inboxes could miss one and store it twice.
func TestGroupAddress_RetryAfterLargeFanOut(t *testing.T) {
	s := seedGroupDelivery(t)
	members := make([]*core.Record, 12)
	for i := range members {
		members[i] = seedGroupUser(t, s.app, fmt.Sprintf("member%02d", i))
	}
	big := seedGroup(t, s.app, "Big", members...)
	saveGroupAddress(t, s.app, "big", s.domain, big)
	targets, err := deliveryTargets(s.app, "big", "q.example")
	if err != nil || len(targets) != len(members) {
		t.Fatalf("deliveryTargets = %d targets, %v; want %d", len(targets), err, len(members))
	}
	// The last inbox in delivery order fails, so 11 copies exist before
	// the retry reaches the inboxes that stored.
	stop := failStoresIn(t, s.app, targets[len(targets)-1].Mailbox)

	msg := groupMessage("<big-1@sender.example>", []string{"big@q.example"}, nil)
	if status := postWebhook(t, s.app, msg); status != http.StatusInternalServerError {
		t.Fatalf("first try status = %d, want 500 so the provider retries", status)
	}
	stop()
	if status := postWebhook(t, s.app, msg); status != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", status)
	}
	for _, target := range targets {
		if rows := messagesIn(t, s.app, target.Mailbox); len(rows) != 1 {
			t.Fatalf("inbox %s has %d copies after the retry, want 1", target.Mailbox.GetString("address"), len(rows))
		}
	}
}

// The webhook keeps a partial delivery with no Message-ID: a retry would
// store a second copy for ann.
func TestGroupAddress_WebhookPartialFailureNoMessageID(t *testing.T) {
	s := seedGroupDelivery(t)
	logs := captureLogs(t)
	failStoresIn(t, s.app, s.inboxOf(t, s.bob))

	msg := groupMessage("", []string{"sales@q.example"}, nil)
	if status := postWebhook(t, s.app, msg); status != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200: a retry would duplicate ann's copy", status)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireNoCopy(t, s.bob)
	requireErrorLog(t, logs, "inbound: storage failed")
	requireErrorLog(t, logs, "inbound: accepted a partial delivery")
}

// With a Message-ID the webhook answers 500, and the provider's retry adds
// only bob's copy.
func TestGroupAddress_WebhookPartialFailureRetries(t *testing.T) {
	s := seedGroupDelivery(t)
	stop := failStoresIn(t, s.app, s.inboxOf(t, s.bob))
	msg := groupMessage("<webhook-partial-1@sender.example>", []string{"sales@q.example"}, nil)

	if status := postWebhook(t, s.app, msg); status != http.StatusInternalServerError {
		t.Fatalf("webhook status = %d, want 500", status)
	}
	stop()
	if status := postWebhook(t, s.app, msg); status != http.StatusOK {
		t.Fatalf("retry status = %d, want 200", status)
	}
	s.requireOneCopy(t, s.ann, s.salesAddr)
	s.requireOneCopy(t, s.bob, s.salesAddr)
}

// A lookup that fails for one recipient follows the same retry rule as a
// failed store: ann's copy is stored through her own address while the group
// lookup fails (its table is dropped).
func TestGroupAddress_FetcherLookupFailureRetryRule(t *testing.T) {
	dropGroupMembers := func(t *testing.T, app core.App) {
		t.Helper()
		if _, err := app.DB().NewQuery("DROP TABLE group_members").Execute(); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("with Message-ID", func(t *testing.T) {
		s := seedGroupDelivery(t)
		dropGroupMembers(t, s.app)
		msg := groupMessage("<fetch-lookup-1@sender.example>", []string{"ann@q.example", "sales@q.example"}, nil)
		if err := dispatchInbound(s.app, msg); err == nil {
			t.Fatal("dispatchInbound = nil after a lookup failed, want the error so the UID stays unseen")
		}
		s.requireOneCopy(t, s.ann, s.ownAddress(t, s.ann))
	})
	t.Run("no Message-ID", func(t *testing.T) {
		s := seedGroupDelivery(t)
		logs := captureLogs(t)
		dropGroupMembers(t, s.app)
		msg := groupMessage("", []string{"ann@q.example", "sales@q.example"}, nil)
		if err := dispatchInbound(s.app, msg); err != nil {
			t.Fatalf("dispatchInbound = %v, want nil: a retry would duplicate ann's copy", err)
		}
		s.requireOneCopy(t, s.ann, s.ownAddress(t, s.ann))
		requireErrorLog(t, logs, "imap fetcher: kept a partial delivery")
	})
}

// A provider retry can arrive while the first delivery of the message is
// still being stored. The Message-ID check runs inside the delivery's
// transaction, and SQLite allows one writer, so the retry waits for the
// first delivery to commit, finds its copy and stores none.
func TestProcessInbound_ConcurrentRetryStoresOneCopy(t *testing.T) {
	s := seedGroupDelivery(t)
	inbox := s.inboxOf(t, s.ann)
	own := s.ownAddress(t, s.ann)

	firstStoring := make(chan struct{})
	secondStarted := make(chan struct{})
	var stores atomic.Int32
	s.app.OnRecordCreate("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		if stores.Add(1) == 1 {
			close(firstStoring)
			// Hold the first delivery open so the retry runs its check while
			// the first copy is not yet committed.
			<-secondStarted
			time.Sleep(200 * time.Millisecond)
		}
		return e.Next()
	})

	msg := groupMessage("<concurrent-1@sender.example>", []string{"ann@q.example"}, nil)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = processInboundForMailbox(s.app, inbox, msg, own.Id)
	}()
	go func() {
		defer wg.Done()
		<-firstStoring
		close(secondStarted)
		errs[1] = processInboundForMailbox(s.app, inbox, msg, own.Id)
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}
	s.requireOneCopy(t, s.ann, own)
}
