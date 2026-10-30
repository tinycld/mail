package mail

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	netmail "net/mail"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
	"tinycld.org/core/syscfg"
)

func TestIsAutoSubmitted(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		headers []Header
		want    bool
	}{
		{"no headers", nil, false},
		{"plain mail", []Header{{Name: "Subject", Value: "hi"}}, false},
		{"Auto-Submitted: no", []Header{{Name: "Auto-Submitted", Value: "no"}}, false},
		{"Auto-Submitted: No (any case)", []Header{{Name: "auto-submitted", Value: " No "}}, false},
		{"Auto-Submitted: auto-replied", []Header{{Name: "Auto-Submitted", Value: "auto-replied"}}, true},
		{"Auto-Submitted: auto-generated", []Header{{Name: "Auto-Submitted", Value: "auto-generated"}}, true},
		{"Auto-Submitted: auto-forwarded", []Header{{Name: "Auto-Submitted", Value: "auto-forwarded"}}, true},
		{"Auto-Submitted with a parameter", []Header{{Name: "Auto-Submitted", Value: "auto-replied; owner-email=a@b.example"}}, true},
		{"Auto-Submitted name in lower case", []Header{{Name: "auto-submitted", Value: "Auto-Replied"}}, true},
		{"Auto-Submitted with an empty value", []Header{{Name: "Auto-Submitted", Value: ""}}, true},
		{"X-TinyCld-Loop", []Header{{Name: "X-TinyCld-Loop", Value: "mail.q.example"}}, true},
		{"X-TinyCld-Loop with an empty value", []Header{{Name: "x-tinycld-loop", Value: ""}}, true},
		{"Precedence: bulk", []Header{{Name: "Precedence", Value: "bulk"}}, true},
		{"Precedence: list", []Header{{Name: "Precedence", Value: "List"}}, true},
		{"Precedence: junk", []Header{{Name: "precedence", Value: " junk "}}, true},
		{"Precedence: first-class", []Header{{Name: "Precedence", Value: "first-class"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAutoSubmitted(tc.headers); got != tc.want {
				t.Fatalf("isAutoSubmitted(%v) = %v, want %v", tc.headers, got, tc.want)
			}
		})
	}
}

// loopEnv is the group-delivery seed (ann, bob, cy, Sales = ann + bob) with a
// domain that may send, a configured provider, and a fake Postmark that
// records each send.
type loopEnv struct {
	*groupDeliverySeed
	sent *[]map[string]any
}

func seedLoopEnv(t *testing.T) *loopEnv {
	t.Helper()
	s := seedGroupDelivery(t)
	s.domain.Set("spf_verified", true)
	s.domain.Set("dkim_verified", true)
	s.domain.Set("return_path_verified", true)
	if err := s.app.Save(s.domain); err != nil {
		t.Fatal(err)
	}
	syscfg.SetResolver(s.app, func(key string) string {
		if key == "mail.postmark_server_token" {
			return "test-token"
		}
		return ""
	})
	s.app.Settings().Meta.AppURL = "https://mail.q.example:8443"
	return &loopEnv{groupDeliverySeed: s, sent: fakePostmark(t)}
}

// rule saves a personal rule owned by owner. The action handlers read only
// its id and scope; the params come with the request.
func (e *loopEnv) rule(t *testing.T, owner *core.Record) *core.Record {
	t.Helper()
	return saveRealRecord(t, e.app, "rules", map[string]any{
		"name": "loop test", "scope": "personal", "owner": owner.Id,
		"trigger": "mail:message-received", "enabled": true,
	})
}

func (e *loopEnv) forward(t *testing.T, owner, message *core.Record, to string) error {
	t.Helper()
	return actionForwardMessage(e.app, automation.ActionRequest{
		Rule: e.rule(t, owner), OwnerID: owner.Id, Params: map[string]string{"to": to}, Record: message,
	})
}

// received returns the one received copy with the subject in the user's
// inbox. Sent copies (sent_by set) are left out.
func (e *loopEnv) received(t *testing.T, user *core.Record, subject string) *core.Record {
	t.Helper()
	rows, err := e.app.FindRecordsByFilter("mail_messages",
		"thread.mailbox = {:mailbox} && subject = {:subject} && sent_by = ''", "", 0, 0,
		map[string]any{"mailbox": e.inboxOf(t, user).Id, "subject": subject})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%s has %d received copies of %q, want 1", user.GetString("username"), len(rows), subject)
	}
	return rows[0]
}

// postmarkInbound posts a Postmark inbound payload through the real webhook
// handler and Postmark parser.
func postmarkInbound(t *testing.T, app core.App, payload map[string]any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	re, rec := makeInboundRequest(t, app, "tok-loop", body)
	if err := handleInbound(app, NewPostmarkProvider("", ""), re, "tok-loop"); err != nil {
		t.Fatalf("handleInbound: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200", rec.Code)
	}
}

// inboundPayload is a Postmark inbound payload from an outside sender to one
// recipient, with extra headers.
func inboundPayload(to, subject, messageID string, headers ...Header) map[string]any {
	list := []map[string]string{{"Name": "Message-ID", "Value": messageID}}
	for _, h := range headers {
		list = append(list, map[string]string{"Name": h.Name, "Value": h.Value})
	}
	return map[string]any{
		"FromFull": map[string]string{"Name": "Sender", "Email": "sender@sender.example"},
		"ToFull":   []map[string]string{{"Email": to}},
		"Subject":  subject,
		"TextBody": "body",
		"Date":     "Thu, 15 Oct 2026 20:00:00 +0000",
		"Headers":  list,
	}
}

// redeliver feeds a captured send back in through the webhook to one
// recipient, as the provider does when the recipient is a local address. The
// headers named in drop are left out, as a relay that strips them would.
func redeliver(t *testing.T, app core.App, sent map[string]any, to string, drop ...string) {
	t.Helper()
	from, err := netmail.ParseAddress(sent["From"].(string))
	if err != nil {
		t.Fatalf("captured From %q: %v", sent["From"], err)
	}
	var headers []map[string]string
	raw, _ := sent["Headers"].([]any)
	for _, item := range raw {
		h := item.(map[string]any)
		name := h["Name"].(string)
		if containsFold(drop, name) {
			continue
		}
		headers = append(headers, map[string]string{"Name": name, "Value": h["Value"].(string)})
	}
	postmarkInbound(t, app, map[string]any{
		"FromFull": map[string]string{"Name": from.Name, "Email": from.Address},
		"ToFull":   []map[string]string{{"Email": to}},
		"Subject":  sent["Subject"],
		"TextBody": sent["TextBody"],
		"Date":     "Thu, 15 Oct 2026 20:00:00 +0000",
		"Headers":  headers,
	})
}

func containsFold(list []string, s string) bool {
	for _, item := range list {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}

func sentHeader(t *testing.T, sent map[string]any, name string) string {
	t.Helper()
	raw, _ := sent["Headers"].([]any)
	for _, item := range raw {
		h := item.(map[string]any)
		if strings.EqualFold(h["Name"].(string), name) {
			return h["Value"].(string)
		}
	}
	t.Fatalf("send has no %s header; headers: %v", name, sent["Headers"])
	return ""
}

func requireSends(t *testing.T, sent *[]map[string]any, want int) {
	t.Helper()
	if len(*sent) != want {
		t.Fatalf("provider got %d sends, want %d: %v", len(*sent), want, *sent)
	}
}

// --- every inbound path records automatic mail ---

const autoRepliedHeader = "Auto-Submitted: auto-replied\r\n"

func rawTo(to, subject, messageID, extra string) string {
	return "From: Sender <sender@sender.example>\r\nTo: " + to + "\r\nSubject: " + subject + "\r\n" +
		"Message-ID: " + messageID + "\r\nDate: Thu, 15 Oct 2026 20:00:00 +0000\r\n" + extra +
		"Content-Type: text/plain; charset=utf-8\r\n\r\nBody\r\n"
}

// fetchViaIMAP runs one IMAP fetcher tick against an in-memory server that
// holds raw as its one unseen message.
func fetchViaIMAP(t *testing.T, app core.App, raw string) {
	t.Helper()
	server, addr, cleanup := newMemIMAPServer(t)
	t.Cleanup(cleanup)
	user := imapmemserver.NewUser("system", "secret")
	if err := user.Create("INBOX", &imap.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := user.Append("INBOX", literalReader([]byte(raw)), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	server.AddUser(user)
	previous := imapDialer
	imapDialer = func(context.Context, string, bool) (*imapclient.Client, error) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return nil, err
		}
		return imapclient.New(conn, nil), nil
	}
	t.Cleanup(func() { imapDialer = previous })
	cfg := SMTPConfig{
		InboundMode: "imap", IMAPHost: "imap.test", IMAPPort: 143,
		IMAPUsername: "system", IMAPPassword: "secret", IMAPMailbox: "INBOX",
	}
	if err := (&imapFetcherManager{app: app}).tick(context.Background(), cfg); err != nil {
		t.Fatalf("tick: %v", err)
	}
}

func TestInboundPaths_RecordAutomaticMail(t *testing.T) {
	paths := []struct {
		name    string
		deliver func(t *testing.T, s *groupDeliverySeed, subject, messageID string, auto bool)
	}{
		{"webhook", func(t *testing.T, s *groupDeliverySeed, subject, messageID string, auto bool) {
			var headers []Header
			if auto {
				headers = append(headers, Header{Name: "Auto-Submitted", Value: "auto-replied"})
			}
			postmarkInbound(t, s.app, inboundPayload("ann@q.example", subject, messageID, headers...))
		}},
		{"smtp", func(t *testing.T, s *groupDeliverySeed, subject, messageID string, auto bool) {
			extra := ""
			if auto {
				extra = autoRepliedHeader
			}
			if err := smtpDeliver(t, s.app, "ann@q.example", rawTo("ann@q.example", subject, messageID, extra)); err != nil {
				t.Fatalf("DATA: %v", err)
			}
		}},
		{"imap fetcher", func(t *testing.T, s *groupDeliverySeed, subject, messageID string, auto bool) {
			extra := ""
			if auto {
				extra = autoRepliedHeader
			}
			fetchViaIMAP(t, s.app, rawTo("ann@q.example", subject, messageID, extra))
		}},
		{"imap append", func(t *testing.T, s *groupDeliverySeed, subject, messageID string, auto bool) {
			extra := ""
			if auto {
				extra = autoRepliedHeader
			}
			session := newIMAPSession(s.app)
			session.user = s.ann
			session.mailboxIndex = []mailboxContext{{name: "personal", mailboxID: s.inboxOf(t, s.ann).Id}}
			raw := rawTo("ann@q.example", subject, messageID, extra)
			if _, err := session.Append("INBOX", literalReader([]byte(raw)), nil); err != nil {
				t.Fatalf("APPEND: %v", err)
			}
		}},
	}
	for _, path := range paths {
		for _, auto := range []bool{true, false} {
			name := path.name + "/plain"
			if auto {
				name = path.name + "/auto-replied"
			}
			t.Run(name, func(t *testing.T) {
				s := seedGroupDelivery(t)
				path.deliver(t, s, "Path test", "<path-1@sender.example>", auto)
				rows := messagesIn(t, s.app, s.inboxOf(t, s.ann))
				if len(rows) != 1 {
					t.Fatalf("ann has %d messages, want 1", len(rows))
				}
				if got := rows[0].GetBool("is_auto_submitted"); got != auto {
					t.Fatalf("is_auto_submitted = %v, want %v", got, auto)
				}
			})
		}
	}
}

// --- rule sends are marked ---

func TestRuleForward_SendsWithLoopHeaders(t *testing.T) {
	e := seedLoopEnv(t)
	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Invoice", "<inv-1@sender.example>"))

	if err := e.forward(t, e.ann, e.received(t, e.ann, "Invoice"), "accounts@ext.example"); err != nil {
		t.Fatalf("forward: %v", err)
	}
	requireSends(t, e.sent, 1)
	if got := sentHeader(t, (*e.sent)[0], "Auto-Submitted"); got != "auto-forwarded" {
		t.Fatalf("Auto-Submitted = %q, want auto-forwarded", got)
	}
	if got := sentHeader(t, (*e.sent)[0], "X-TinyCld-Loop"); got != "mail.q.example" {
		t.Fatalf("X-TinyCld-Loop = %q, want mail.q.example", got)
	}
	// The stored sent copy is automatic mail too.
	copies, err := e.app.FindRecordsByFilter("mail_messages", "subject = 'Fwd: Invoice'", "", 0, 0)
	if err != nil || len(copies) != 1 {
		t.Fatalf("sent copies = %d (%v), want 1", len(copies), err)
	}
	if !copies[0].GetBool("is_auto_submitted") {
		t.Fatal("the stored sent copy of a rule forward is not marked automatic")
	}
}

func TestRuleSendMessage_SendsAutoGenerated(t *testing.T) {
	e := seedLoopEnv(t)
	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Question", "<q-1@sender.example>"))

	err := actionSendMessage(e.app, automation.ActionRequest{
		Rule: e.rule(t, e.ann), OwnerID: e.ann.Id, Record: e.received(t, e.ann, "Question"),
		Params: map[string]string{"to": "sender@sender.example", "subject": "Got it", "body": "Thanks"},
	})
	if err != nil {
		t.Fatalf("send-message: %v", err)
	}
	requireSends(t, e.sent, 1)
	if got := sentHeader(t, (*e.sent)[0], "Auto-Submitted"); got != "auto-generated" {
		t.Fatalf("Auto-Submitted = %q, want auto-generated", got)
	}
	if got := sentHeader(t, (*e.sent)[0], "X-TinyCld-Loop"); got != "mail.q.example" {
		t.Fatalf("X-TinyCld-Loop = %q, want mail.q.example", got)
	}
}

// --- rules do not send in reply to automatic mail ---

func TestRuleSends_SkipAutomaticMail(t *testing.T) {
	e := seedLoopEnv(t)
	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Out of office", "<ooo-1@sender.example>",
		Header{Name: "Auto-Submitted", Value: "auto-replied"}))
	message := e.received(t, e.ann, "Out of office")

	if err := e.forward(t, e.ann, message, "accounts@ext.example"); !automation.IsSkip(err) {
		t.Fatalf("forward on automatic mail = %v, want a skip", err)
	}
	err := actionSendMessage(e.app, automation.ActionRequest{
		Rule: e.rule(t, e.ann), OwnerID: e.ann.Id, Record: message,
		Params: map[string]string{"to": "sender@sender.example", "subject": "Got it", "body": "Thanks"},
	})
	if !automation.IsSkip(err) || err.Error() != "skipped: The message was sent automatically." {
		t.Fatalf("send-message on automatic mail = %v, want the automatic-mail skip", err)
	}
	requireSends(t, e.sent, 0)
}

func TestAutomaticMailSendGuard(t *testing.T) {
	e := seedLoopEnv(t)
	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Out of office", "<ooo-2@sender.example>",
		Header{Name: "Auto-Submitted", Value: "auto-replied"}))
	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Question", "<q-2@sender.example>"))

	skip, reason := automaticMailSendGuard(e.app, e.received(t, e.ann, "Out of office"))
	if !skip || reason != "The message was sent automatically." {
		t.Fatalf("guard on automatic mail = %v, %q; want a skip with the reason", skip, reason)
	}
	if skip, _ := automaticMailSendGuard(e.app, e.received(t, e.ann, "Question")); skip {
		t.Fatal("guard skipped mail from a person")
	}
}

func TestRuleMove_RunsOnAutomaticMail(t *testing.T) {
	e := seedLoopEnv(t)
	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Newsletter", "<news-1@sender.example>",
		Header{Name: "Precedence", Value: "list"}))
	message := e.received(t, e.ann, "Newsletter")

	err := actionMoveToFolder(e.app, automation.ActionRequest{
		Rule: e.rule(t, e.ann), OwnerID: e.ann.Id, Record: message,
		Params: map[string]string{"folder": "archive"},
	})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	state := findThreadState(e.app, message.GetString("thread"), e.ann.Id)
	if state == nil || state.GetString("folder") != "archive" {
		t.Fatalf("thread state = %v, want archive", state)
	}
}

// --- loops ---

func TestRuleRecipient_RefusesGroupThatDeliversToThisInbox(t *testing.T) {
	e := seedLoopEnv(t)
	annInbox := e.inboxOf(t, e.ann).Id
	want := "recipient sales@q.example is a group address that delivers to this inbox — refusing to create a mail loop"

	_, err := ruleRecipient(e.app, "sales@q.example", annInbox)
	if err == nil || err.Error() != want {
		t.Fatalf("ruleRecipient(sales@) for ann = %v, want %q", err, want)
	}
	_, err = ruleRecipient(e.app, "Sales+tag@Q.example", annInbox)
	if err == nil || !strings.Contains(err.Error(), "is a group address that delivers to this inbox") {
		t.Fatalf("ruleRecipient(sales+tag@) for ann = %v, want the group-loop refusal", err)
	}
	// cy is not in Sales: the group does not deliver to her inbox.
	if got, err := ruleRecipient(e.app, "sales@q.example", e.inboxOf(t, e.cy).Id); err != nil || got != "sales@q.example" {
		t.Fatalf("ruleRecipient(sales@) for cy = %q, %v; want it allowed", got, err)
	}
	// A plus tag on the inbox's own address is still a self-send.
	_, err = ruleRecipient(e.app, "ann+tag@q.example", annInbox)
	if err == nil || !strings.Contains(err.Error(), "is delivered to the sending mailbox") {
		t.Fatalf("ruleRecipient(ann+tag@) for ann = %v, want the self-send refusal", err)
	}
}

// ann and bob (the Sales group) each have a rule "forward to sales@". The
// direct loop is refused (the group delivers to each of their inboxes). When
// cy, who is not in Sales, forwards to sales@, the copy arrives marked
// automatic, so neither member forwards it again.
func TestRuleLoop_GroupMembersForwardingToTheirGroup(t *testing.T) {
	e := seedLoopEnv(t)
	postmarkInbound(t, e.app, inboundPayload("sales@q.example", "Lead", "<lead-1@sender.example>"))
	for _, member := range []*core.Record{e.ann, e.bob} {
		err := e.forward(t, member, e.received(t, member, "Lead"), "sales@q.example")
		if err == nil || !strings.Contains(err.Error(), "is a group address that delivers to this inbox") {
			t.Fatalf("%s's forward to sales@ = %v, want the group-loop refusal", member.GetString("username"), err)
		}
	}
	requireSends(t, e.sent, 0)

	postmarkInbound(t, e.app, inboundPayload("cy@q.example", "Referral", "<ref-1@sender.example>"))
	if err := e.forward(t, e.cy, e.received(t, e.cy, "Referral"), "sales@q.example"); err != nil {
		t.Fatalf("cy's forward to sales@: %v", err)
	}
	requireSends(t, e.sent, 1)

	redeliver(t, e.app, (*e.sent)[0], "sales@q.example")
	for _, member := range []*core.Record{e.ann, e.bob} {
		fwd := e.received(t, member, "Fwd: Referral")
		if !fwd.GetBool("is_auto_submitted") {
			t.Fatalf("%s's copy of the forward is not marked automatic", member.GetString("username"))
		}
		if err := e.forward(t, member, fwd, "sales@q.example"); !automation.IsSkip(err) {
			t.Fatalf("%s's forward of automatic mail = %v, want a skip", member.GetString("username"), err)
		}
	}
	requireSends(t, e.sent, 1)
}

// ann forwards to bob and bob forwards to ann. A relay on the way drops
// Auto-Submitted, but the X-TinyCld-Loop header still marks the copy, so bob
// does not forward it back.
func TestRuleLoop_LoopHeaderStopsTheSecondForward(t *testing.T) {
	e := seedLoopEnv(t)
	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Ping", "<ping-1@sender.example>"))
	if err := e.forward(t, e.ann, e.received(t, e.ann, "Ping"), "bob@q.example"); err != nil {
		t.Fatalf("ann's forward to bob: %v", err)
	}
	requireSends(t, e.sent, 1)

	redeliver(t, e.app, (*e.sent)[0], "bob@q.example", "Auto-Submitted")
	fwd := e.received(t, e.bob, "Fwd: Ping")
	if !fwd.GetBool("is_auto_submitted") {
		t.Fatal("bob's copy with only X-TinyCld-Loop is not marked automatic")
	}
	if err := e.forward(t, e.bob, fwd, "ann@q.example"); !automation.IsSkip(err) {
		t.Fatalf("bob's forward back = %v, want a skip", err)
	}
	requireSends(t, e.sent, 1)
}
