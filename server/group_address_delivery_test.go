package mail

import (
	"errors"
	"net/http"
	"slices"
	"strings"
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
