package mail

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

type routingEnv struct {
	app   core.App
	inbox *core.Record
	org   *core.Record
	com   *core.Record
}

// setupRoutingEnv: one inbox with nathan@example.org (primary) and
// nathan@example.com (added), on the fixture schema.
func setupRoutingEnv(t *testing.T) *routingEnv {
	t.Helper()
	app := setupInboundTestApp(t)
	org := saveFixtureRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	com := saveFixtureRecord(t, app, "mail_domains", map[string]any{"domain": "example.com"})
	inbox := saveFixtureRecord(t, app, "mail_mailboxes", map[string]any{"address": "nathan", "domain": org.Id, "type": "personal"})
	saveFixtureRecord(t, app, "mail_addresses", map[string]any{"local_part": "nathan", "domain": com.Id, "mailbox": inbox.Id, "source": addressSourceManual})
	seedMember(t, app, inbox.Id, "user_routing_01")
	return &routingEnv{app: app, inbox: inbox, org: org, com: com}
}

func saveFixtureRecord(t *testing.T, app core.App, collection string, fields map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	for k, v := range fields {
		r.Set(k, v)
	}
	if err := app.Save(r); err != nil {
		t.Fatalf("save %s: %v", collection, err)
	}
	return r
}

func deliveredMessages(t *testing.T, app core.App, mailboxID, messageID string) []*core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_messages",
		"thread.mailbox = {:mailbox} && message_id = {:mid}", "", 0, 0,
		map[string]any{"mailbox": mailboxID, "mid": normalizeMessageID(messageID)})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestRoutingRecipients_EnvelopeFirstAndUnique(t *testing.T) {
	t.Parallel()
	msg := &InboundMessage{
		OriginalRecipient: "Nathan@Example.com",
		To:                []Recipient{{Email: "someone@else.example"}, {Email: "nathan@example.com"}},
		Cc:                []Recipient{{Email: "nathan@example.org"}},
	}
	got := routingRecipients(msg)
	want := []string{"nathan@example.com", "someone@else.example", "nathan@example.org"}
	if len(got) != len(want) {
		t.Fatalf("routingRecipients = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("routingRecipients = %v, want %v", got, want)
		}
	}
}

// A BCC to a hosted address is only in the envelope; it must be delivered.
func TestRouteInbound_DeliversEnvelopeOnlyRecipient(t *testing.T) {
	t.Parallel()
	env := setupRoutingEnv(t)
	msg := &InboundMessage{
		MessageID:         "<bcc-1@sender.example>",
		From:              Recipient{Email: "sender@sender.example"},
		OriginalRecipient: "nathan@example.com",
		To:                []Recipient{{Email: "someone@else.example"}},
		Subject:           "Hello",
		Date:              "2026-10-15T20:00:00Z",
	}
	result := routeInbound(env.app, msg)
	if result.storedCount == 0 {
		t.Fatalf("envelope-only recipient was not delivered: unknown %v", result.unknownAddrs)
	}
	rows := deliveredMessages(t, env.app, env.inbox.Id, msg.MessageID)
	if len(rows) != 1 {
		t.Fatalf("stored %d copies, want 1", len(rows))
	}
	com, _ := findAddressRow(env.app, "nathan", env.com.Id)
	if rows[0].GetString("delivered_to") != com.Id {
		t.Fatalf("delivered_to = %q, want the example.com address %q", rows[0].GetString("delivered_to"), com.Id)
	}
}

// Two of the same inbox's addresses in one message: one copy, delivered_to
// the first address that matched.
func TestRouteInbound_TwoAddressesOfOneInboxStoreOnce(t *testing.T) {
	t.Parallel()
	env := setupRoutingEnv(t)
	msg := &InboundMessage{
		MessageID: "<two-1@sender.example>",
		From:      Recipient{Email: "sender@sender.example"},
		To:        []Recipient{{Email: "nathan@example.org"}},
		Cc:        []Recipient{{Email: "nathan@example.com"}},
		Subject:   "Both",
		Date:      "2026-10-15T20:00:00Z",
	}
	routeInbound(env.app, msg)
	rows := deliveredMessages(t, env.app, env.inbox.Id, msg.MessageID)
	if len(rows) != 1 {
		t.Fatalf("stored %d copies, want 1", len(rows))
	}
	org, _ := findAddressRow(env.app, "nathan", env.org.Id)
	if rows[0].GetString("delivered_to") != org.Id {
		t.Fatalf("delivered_to = %q, want the first match (example.org) %q", rows[0].GetString("delivered_to"), org.Id)
	}
}

func TestSMTPInbound_SetsDeliveredTo(t *testing.T) {
	t.Parallel()
	env := setupRoutingEnv(t)
	s := &smtpInboundSession{app: env.app, hostname: "mx.example"}
	if err := s.Mail("sender@sender.example", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Rcpt("nathan+news@example.com", nil); err != nil {
		t.Fatalf("RCPT to a plus-tagged added address was refused: %v", err)
	}
	raw := "From: sender@sender.example\r\nTo: nathan@example.com\r\nSubject: Via SMTP\r\nMessage-ID: <smtp-1@sender.example>\r\nDate: Thu, 15 Oct 2026 20:00:00 +0000\r\n\r\nBody\r\n"
	if err := s.Data(stringsReader(raw)); err != nil {
		t.Fatal(err)
	}
	rows := deliveredMessages(t, env.app, env.inbox.Id, "<smtp-1@sender.example>")
	com, _ := findAddressRow(env.app, "nathan", env.com.Id)
	if len(rows) != 1 || rows[0].GetString("delivered_to") != com.Id {
		t.Fatalf("SMTP delivery did not record delivered_to: %d rows", len(rows))
	}
}

func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }
