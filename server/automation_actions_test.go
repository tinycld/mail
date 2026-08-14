package mail

import (
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/types"
	"tinycld.org/core/automation"
)

// automation_actions_test.go covers the five native actions mail registers:
//
//   - the audience rule that decides who an action acts for — an org rule
//     applies to every mailbox member, a personal rule only to its owner, and
//     neither may touch a non-member;
//   - move-to-folder and mark-as-read each changing only their own field, so
//     filing a message doesn't silently mark it read (or vice versa);
//   - the folder allow-list rejecting destinations the UI has no view for;
//   - the hourly send cap that bounds an auto-reply loop, including the three
//     properties that make it a real bound: it counts messages rather than
//     matched runs, it is shared across every rule on a mailbox, and it fails
//     closed when the count is unavailable;
//   - recipient validation — the "to" param arrives after template
//     substitution, so {{sender_email}} lets whoever mails the mailbox choose
//     it; malformed addresses and the mailbox's own address are refused;
//   - forwarding sending the stored body rather than the 200-char snippet.
//
// The provider handoff itself is not exercised here: sendMessage needs a
// configured provider, which the endpoint tests already cover. Everything
// upstream of that handoff — the limiter, recipient validation, and body
// rendering — is covered above, since that is where a loop or a leak starts.

// newRulesCollections adds the two core automation collections the handlers
// read. The mail test app builds its collections synthetically (see
// setupInboundTestApp) rather than applying core's migrations, so the pieces
// under test have to be declared here too — matching the shipped shape of
// core/server/pb_migrations/1990000000 and 1990000001.
func newRulesCollections(t *testing.T, app core.App) {
	t.Helper()

	rules := core.NewBaseCollection("rules")
	rules.Fields.Add(&core.TextField{Name: "name"})
	rules.Fields.Add(&core.TextField{Name: "owner"})
	rules.Fields.Add(&core.TextField{Name: "scope"})
	rules.Fields.Add(&core.TextField{Name: "trigger"})
	rules.Fields.Add(&core.BoolField{Name: "enabled"})
	if err := app.Save(rules); err != nil {
		t.Fatalf("failed to save rules collection: %v", err)
	}

	runs := core.NewBaseCollection("rule_runs")
	runs.Fields.Add(&core.RelationField{
		Name:          "rule",
		Required:      true,
		CollectionId:  rules.Id,
		CascadeDelete: true,
		MaxSelect:     1,
	})
	runs.Fields.Add(&core.BoolField{Name: "matched"})
	runs.Fields.Add(&core.DateField{Name: "fired_at"})
	if err := app.Save(runs); err != nil {
		t.Fatalf("failed to save rule_runs collection: %v", err)
	}
}

func newTestRule(t *testing.T, app core.App, ownerID, scope string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("rules")
	if err != nil {
		t.Fatalf("rules collection missing: %v", err)
	}
	rule := core.NewRecord(col)
	rule.Set("name", "test rule")
	rule.Set("owner", ownerID)
	rule.Set("scope", scope)
	rule.Set("trigger", "mail:message-received")
	rule.Set("enabled", true)
	if err := app.Save(rule); err != nil {
		t.Fatalf("failed to save rule: %v", err)
	}
	return rule
}

// seedRun writes a matched rule_runs row aged by the given offset.
func seedRun(t *testing.T, app core.App, ruleID string, age time.Duration) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("rule_runs")
	if err != nil {
		t.Fatalf("rule_runs collection missing: %v", err)
	}
	run := core.NewRecord(col)
	run.Set("rule", ruleID)
	run.Set("matched", true)
	run.Set("fired_at", types.NowDateTime().Add(-age))
	if err := app.Save(run); err != nil {
		t.Fatalf("failed to save rule_run: %v", err)
	}
}

// actionEnv is one mailbox with two members and a message to act on.
type actionEnv struct {
	app       core.App
	mailboxID string
	threadID  string
	msg       *core.Record
	alice     string
	bob       string
}

func setupActionEnv(t *testing.T, domain, mailboxID string) actionEnv {
	t.Helper()
	app := setupInboundTestApp(t)
	newRulesCollections(t, app)

	seedDomainAndMailbox(t, app, domain, "alice", mailboxID)
	padded := padID(mailboxID)
	seedMember(t, app, mailboxID, "user_alice_0000")
	seedMember(t, app, mailboxID, "user_bob_000000")

	thread := newTestThread(t, app, padded, "hello")
	msg := newTestMessage(t, app, thread.Id, "hello")

	return actionEnv{
		app:       app,
		mailboxID: padded,
		threadID:  thread.Id,
		msg:       msg,
		alice:     "user_alice_0000",
		bob:       "user_bob_000000",
	}
}

func threadStateFor(t *testing.T, app core.App, threadID, userID string) *core.Record {
	t.Helper()
	state := findThreadState(app, threadID, userID)
	if state == nil {
		t.Fatalf("no mail_thread_state row for user %s on thread %s", userID, threadID)
	}
	return state
}

func TestActionAudience_OrgRuleCoversEveryMailboxMember(t *testing.T) {
	env := setupActionEnv(t, "audience-org.test", "mb_audience_org1")
	rule := newTestRule(t, env.app, env.alice, "org")

	got := actionAudience(env.app, automation.ActionRequest{
		Rule: rule, OwnerID: env.alice, Record: env.msg,
	})

	if len(got) != 2 {
		t.Fatalf("org audience = %v, want both mailbox members", got)
	}
	seen := map[string]bool{got[0]: true, got[1]: true}
	if !seen[env.alice] || !seen[env.bob] {
		t.Fatalf("org audience = %v, want alice and bob", got)
	}
}

func TestActionAudience_PersonalRuleIsOwnerOnly(t *testing.T) {
	env := setupActionEnv(t, "audience-personal.test", "mb_audience_per1")
	rule := newTestRule(t, env.app, env.alice, "personal")

	got := actionAudience(env.app, automation.ActionRequest{
		Rule: rule, OwnerID: env.alice, Record: env.msg,
	})

	if len(got) != 1 || got[0] != env.alice {
		t.Fatalf("personal audience = %v, want [%s]", got, env.alice)
	}
}

func TestActionAudience_PersonalRuleOfNonMemberIsEmpty(t *testing.T) {
	env := setupActionEnv(t, "audience-outsider.test", "mb_audience_out1")
	rule := newTestRule(t, env.app, "user_carol_0000", "personal")

	// Carol owns the rule but isn't a member of this mailbox. A native
	// handler is not pkgaccess-gated, so this intersection IS the access
	// check — it must not hand back a user with no claim to the message.
	got := actionAudience(env.app, automation.ActionRequest{
		Rule: rule, OwnerID: "user_carol_0000", Record: env.msg,
	})

	if len(got) != 0 {
		t.Fatalf("audience = %v, want empty for a non-member rule owner", got)
	}
}

func TestActionMoveToFolder_OrgRuleFilesForEveryMember(t *testing.T) {
	env := setupActionEnv(t, "move-org.test", "mb_move_org00001")
	rule := newTestRule(t, env.app, env.alice, "org")

	err := actionMoveToFolder(env.app, automation.ActionRequest{
		Rule: rule, OwnerID: env.alice, Record: env.msg,
		Params: map[string]string{"folder": "archive"},
	})
	if err != nil {
		t.Fatalf("actionMoveToFolder: %v", err)
	}

	for _, userID := range []string{env.alice, env.bob} {
		state := threadStateFor(t, env.app, env.threadID, userID)
		if folder := state.GetString("folder"); folder != "archive" {
			t.Errorf("user %s folder = %q, want archive", userID, folder)
		}
	}
}

func TestActionMoveToFolder_PersonalRuleLeavesOtherMembersAlone(t *testing.T) {
	env := setupActionEnv(t, "move-personal.test", "mb_move_per00001")
	rule := newTestRule(t, env.app, env.alice, "personal")

	err := actionMoveToFolder(env.app, automation.ActionRequest{
		Rule: rule, OwnerID: env.alice, Record: env.msg,
		Params: map[string]string{"folder": "trash"},
	})
	if err != nil {
		t.Fatalf("actionMoveToFolder: %v", err)
	}

	if folder := threadStateFor(t, env.app, env.threadID, env.alice).GetString("folder"); folder != "trash" {
		t.Errorf("alice folder = %q, want trash", folder)
	}
	if state := findThreadState(env.app, env.threadID, env.bob); state != nil {
		t.Errorf("bob got a thread state from alice's personal rule: folder=%q", state.GetString("folder"))
	}
}

func TestActionMoveToFolder_RejectsFolderOutsideTheAllowList(t *testing.T) {
	env := setupActionEnv(t, "move-bad.test", "mb_move_bad00001")
	rule := newTestRule(t, env.app, env.alice, "personal")

	// "sent" and "drafts" are valid mail_thread_state values but are not
	// rule destinations — filing an arriving message into Sent would put it
	// somewhere the user never looks for received mail.
	for _, folder := range []string{"sent", "drafts", "", "nonsense"} {
		err := actionMoveToFolder(env.app, automation.ActionRequest{
			Rule: rule, OwnerID: env.alice, Record: env.msg,
			Params: map[string]string{"folder": folder},
		})
		if err == nil {
			t.Errorf("folder %q was accepted, want rejection", folder)
		}
	}
}

func TestActionMoveToFolder_PreservesReadState(t *testing.T) {
	env := setupActionEnv(t, "move-keeps-read.test", "mb_move_keepread")
	rule := newTestRule(t, env.app, env.alice, "personal")

	if err := ensureThreadState(env.app, env.threadID, env.alice, "inbox", true); err != nil {
		t.Fatalf("seed thread state: %v", err)
	}

	err := actionMoveToFolder(env.app, automation.ActionRequest{
		Rule: rule, OwnerID: env.alice, Record: env.msg,
		Params: map[string]string{"folder": "archive"},
	})
	if err != nil {
		t.Fatalf("actionMoveToFolder: %v", err)
	}

	state := threadStateFor(t, env.app, env.threadID, env.alice)
	if folder := state.GetString("folder"); folder != "archive" {
		t.Errorf("folder = %q, want archive", folder)
	}
	if !state.GetBool("is_read") {
		t.Error("moving a thread cleared is_read; the move must not touch read state")
	}
}

func TestActionMarkAsRead_PreservesFolder(t *testing.T) {
	env := setupActionEnv(t, "read-keeps-folder.test", "mb_read_keepfldr")
	rule := newTestRule(t, env.app, env.alice, "personal")

	if err := ensureThreadState(env.app, env.threadID, env.alice, "archive", false); err != nil {
		t.Fatalf("seed thread state: %v", err)
	}

	err := actionMarkAsRead(env.app, automation.ActionRequest{
		Rule: rule, OwnerID: env.alice, Record: env.msg,
	})
	if err != nil {
		t.Fatalf("actionMarkAsRead: %v", err)
	}

	state := threadStateFor(t, env.app, env.threadID, env.alice)
	if !state.GetBool("is_read") {
		t.Error("is_read = false, want true")
	}
	if folder := state.GetString("folder"); folder != "archive" {
		t.Errorf("folder = %q, want archive — marking read must not refile the thread", folder)
	}
}

// newTestMessageWithBody stores a message with a real body_html file, so the
// forward path exercises the actual file read rather than the snippet.
func newTestMessageWithBody(t *testing.T, app core.App, threadID, subject, html string) *core.Record {
	t.Helper()
	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	msg := core.NewRecord(messages)
	msg.Set("thread", threadID)
	msg.Set("subject", subject)
	msg.Set("snippet", truncateSnippet(stripHTMLToText(html), 200))

	file, err := filesystem.NewFileFromBytes([]byte(html), "body.html")
	if err != nil {
		t.Fatalf("failed to build body file: %v", err)
	}
	msg.Set("body_html", file)

	if err := app.Save(msg); err != nil {
		t.Fatalf("failed to save message: %v", err)
	}
	return msg
}

// seedSentMessage writes a message the mailbox has already sent, aged by the
// given offset — what the rate limiter counts.
func seedSentMessage(t *testing.T, app core.App, threadID string, age time.Duration) {
	t.Helper()
	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	msg := core.NewRecord(messages)
	msg.Set("thread", threadID)
	msg.Set("subject", "already sent")
	msg.Set("delivery_status", "sent")
	msg.Set("date", types.NowDateTime().Add(-age))
	if err := app.Save(msg); err != nil {
		t.Fatalf("failed to save sent message: %v", err)
	}
}

func TestCheckSendRateLimit(t *testing.T) {
	tests := []struct {
		name      string
		sentCount int
		age       time.Duration
		wantErr   bool
	}{
		{name: "no history sends", sentCount: 0, age: time.Minute, wantErr: false},
		{name: "under the cap sends", sentCount: maxSendsPerMailboxPerHour - 2, age: time.Minute, wantErr: false},
		{name: "at the cap is blocked", sentCount: maxSendsPerMailboxPerHour, age: time.Minute, wantErr: true},
		{
			// The cap is a rolling hour, not a lifetime total: yesterday's
			// burst must not stop today's mail.
			name: "old sends fall outside the window", sentCount: maxSendsPerMailboxPerHour + 5,
			age: 2 * time.Hour, wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := setupActionEnv(t, "ratelimit.test", "mb_ratelimit_001")
			rule := newTestRule(t, env.app, env.alice, "personal")

			for range tc.sentCount {
				seedSentMessage(t, env.app, env.threadID, tc.age)
			}

			err := checkSendRateLimit(env.app, automation.ActionRequest{Rule: rule, Record: env.msg})
			if tc.wantErr && err == nil {
				t.Error("expected the send to be blocked, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected the send to be allowed, got %v", err)
			}
		})
	}
}

// The limiter counts messages, not matched runs. A rule with several send
// actions writes ONE rule_runs row per dispatch but emits one message per
// action, so a run-based count bounded len(actions) × the ceiling instead of
// the ceiling. Runs alone must not move the limiter at all.
func TestCheckSendRateLimit_CountsMessagesNotRuns(t *testing.T) {
	env := setupActionEnv(t, "ratelimit-runs.test", "mb_ratelimit_runs")
	rule := newTestRule(t, env.app, env.alice, "personal")

	for range maxSendsPerMailboxPerHour * 3 {
		seedRun(t, env.app, rule.Id, time.Minute)
	}

	if err := checkSendRateLimit(env.app, automation.ActionRequest{Rule: rule, Record: env.msg}); err != nil {
		t.Errorf("matched runs must not consume send budget, got %v", err)
	}
}

// The ceiling is per mailbox, not per rule. Two rules on one mailbox share
// one budget — a per-rule cap let N rules multiply the mailbox's real ceiling
// by N, which is precisely the loop the cap exists to bound.
func TestCheckSendRateLimit_IsSharedAcrossRulesOnAMailbox(t *testing.T) {
	env := setupActionEnv(t, "ratelimit-shared.test", "mb_ratelimit_shar")
	first := newTestRule(t, env.app, env.alice, "personal")
	second := newTestRule(t, env.app, env.alice, "personal")

	for range maxSendsPerMailboxPerHour {
		seedSentMessage(t, env.app, env.threadID, time.Minute)
	}

	if err := checkSendRateLimit(env.app, automation.ActionRequest{Rule: first, Record: env.msg}); err == nil {
		t.Error("the rule that exhausted the budget should be blocked")
	}
	if err := checkSendRateLimit(env.app, automation.ActionRequest{Rule: second, Record: env.msg}); err == nil {
		t.Error("a second rule on the same mailbox must not get a fresh budget")
	}
}

// Fails CLOSED. This is the only control that bounds a cross-dispatch loop —
// the depth cap cannot see a hop through an external autoresponder, which
// returns as new inbound mail at depth 0. An unreadable count means we cannot
// know whether we are mid-loop, so the send is refused.
func TestCheckSendRateLimit_BlocksWhenTheMailboxCannotBeResolved(t *testing.T) {
	env := setupActionEnv(t, "ratelimit-closed.test", "mb_ratelimit_clos")
	rule := newTestRule(t, env.app, env.alice, "personal")

	// A record whose thread cannot be resolved: mailboxIDForMessage fails, so
	// there is no mailbox to count sends against.
	messages, err := env.app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	orphan := core.NewRecord(messages)
	orphan.Set("subject", "no thread")

	if err := checkSendRateLimit(env.app, automation.ActionRequest{Rule: rule, Record: orphan}); err == nil {
		t.Error("an unresolvable mailbox must block the send, not allow it")
	}
}

func TestRuleRecipient_RejectsTheMailboxsOwnAddress(t *testing.T) {
	env := setupActionEnv(t, "selfsend.test", "mb_selfsend_00001")

	// {{sender_email}} is a documented forward target, so an attacker who can
	// mail the mailbox chooses this string. Addressing the mailbox itself is a
	// direct loop the depth cap cannot see: the message re-enters as new
	// inbound mail at depth 0.
	if _, err := ruleRecipient(env.app, "alice@selfsend.test", env.mailboxID); err == nil {
		t.Error("sending to the mailbox's own address must be refused")
	}

	// Case must not be a way around it.
	if _, err := ruleRecipient(env.app, "ALICE@SelfSend.test", env.mailboxID); err == nil {
		t.Error("the self-send check must be case-insensitive")
	}

	// A different address on the same domain is legitimate.
	if _, err := ruleRecipient(env.app, "accounting@selfsend.test", env.mailboxID); err != nil {
		t.Errorf("a different address must be allowed, got %v", err)
	}
}

// Received mail must not consume the send budget. storeMessage defaults an
// unset delivery_status to "sending", so every inbound message carries it —
// counting that status charged arriving mail against the mailbox's ability to
// reply. Combined with fail-closed, a mailbox receiving 20 messages an hour
// would have gone silent with no signal beyond its run history.
func TestCheckSendRateLimit_InboundMailDoesNotConsumeTheBudget(t *testing.T) {
	env := setupActionEnv(t, "ratelimit-inbound.test", "mb_ratelimit_inbd")
	rule := newTestRule(t, env.app, env.alice, "personal")

	messages, err := env.app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	for range maxSendsPerMailboxPerHour * 2 {
		msg := core.NewRecord(messages)
		msg.Set("thread", env.threadID)
		msg.Set("subject", "arrived")
		msg.Set("date", types.NowDateTime())
		// storeMessage's default for inbound mail — deliberately not set here,
		// mirroring what the inbound webhook produces.
		msg.Set("delivery_status", "sending")
		if err := env.app.Save(msg); err != nil {
			t.Fatalf("failed to save inbound message: %v", err)
		}
	}

	if err := checkSendRateLimit(env.app, automation.ActionRequest{Rule: rule, Record: env.msg}); err != nil {
		t.Errorf("received mail must not consume send budget, got %v", err)
	}
}

// A bounce still consumed a send, so it counts toward the ceiling.
func TestCheckSendRateLimit_BouncedSendsCount(t *testing.T) {
	env := setupActionEnv(t, "ratelimit-bounced.test", "mb_ratelimit_bnc")
	rule := newTestRule(t, env.app, env.alice, "personal")

	messages, err := env.app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("mail_messages collection missing: %v", err)
	}
	for range maxSendsPerMailboxPerHour {
		msg := core.NewRecord(messages)
		msg.Set("thread", env.threadID)
		msg.Set("subject", "bounced")
		msg.Set("date", types.NowDateTime())
		msg.Set("delivery_status", "bounced")
		if err := env.app.Save(msg); err != nil {
			t.Fatalf("failed to save bounced message: %v", err)
		}
	}

	if err := checkSendRateLimit(env.app, automation.ActionRequest{Rule: rule, Record: env.msg}); err == nil {
		t.Error("bounced sends must count toward the ceiling")
	}
}

// Mail to an alias is delivered to the same mailbox (findMailboxViaAlias), so
// forwarding to one of the mailbox's own aliases is a self-loop — just a
// less obvious one than using the primary address.
func TestRuleRecipient_RejectsTheMailboxsAliases(t *testing.T) {
	env := setupActionEnv(t, "alias-loop.test", "mb_alias_loop0001")

	aliasCol, err := env.app.FindCollectionByNameOrId("mail_mailbox_aliases")
	if err != nil {
		t.Fatalf("mail_mailbox_aliases collection missing: %v", err)
	}
	alias := core.NewRecord(aliasCol)
	alias.Set("mailbox", env.mailboxID)
	alias.Set("address", "sales")
	if err := env.app.Save(alias); err != nil {
		t.Fatalf("failed to save alias: %v", err)
	}

	if _, err := ruleRecipient(env.app, "sales@alias-loop.test", env.mailboxID); err == nil {
		t.Error("sending to the mailbox's own alias must be refused")
	}
	if _, err := ruleRecipient(env.app, "SALES@Alias-Loop.test", env.mailboxID); err == nil {
		t.Error("the alias self-send check must be case-insensitive")
	}

	// An address that is neither the primary nor an alias stays allowed.
	if _, err := ruleRecipient(env.app, "accounting@alias-loop.test", env.mailboxID); err != nil {
		t.Errorf("an unrelated address must be allowed, got %v", err)
	}
}

func TestHTMLToForwardText_SeparatesTableCells(t *testing.T) {
	// Invoices are commonly tables, and "forward invoices to accounting" is
	// the documented recipe — cells running together corrupts the amounts.
	got := htmlToForwardText("<table><tr><td>Widget</td><td>12.00</td></tr></table>")

	if !strings.Contains(got, "Widget 12.00") {
		t.Errorf("htmlToForwardText = %q, want the cells separated", got)
	}
}

func TestRuleRecipient_RejectsMalformedAddresses(t *testing.T) {
	env := setupActionEnv(t, "recipient.test", "mb_recipient_0001")

	for _, tc := range []struct {
		name string
		to   string
	}{
		{name: "empty", to: ""},
		{name: "whitespace only", to: "   "},
		{name: "not an address", to: "not-an-address"},
		{name: "header injection via newline", to: "a@b.test\nBcc: victim@c.test"},
		{name: "comma list", to: "a@b.test, c@d.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ruleRecipient(env.app, tc.to, env.mailboxID); err == nil {
				t.Errorf("recipient %q must be rejected", tc.to)
			}
		})
	}

	// A display-name form is valid and should yield the bare address.
	got, err := ruleRecipient(env.app, "Accounting <accounting@elsewhere.test>", env.mailboxID)
	if err != nil {
		t.Fatalf("a display-name address must be accepted, got %v", err)
	}
	if got != "accounting@elsewhere.test" {
		t.Errorf("recipient = %q, want the bare address", got)
	}
}

func TestForwardedBody_SendsTheFullBodyNotTheSnippet(t *testing.T) {
	env := setupActionEnv(t, "forward-body.test", "mb_forward_body01")

	// Longer than the 200-char snippet truncation, so a snippet-based forward
	// is visibly short. This is the "forward invoices to accounting" case: the
	// old implementation delivered the first two lines of the invoice.
	long := strings.Repeat("Invoice line item. ", 40)
	msg := newTestMessageWithBody(t, env.app, env.threadID, "Invoice", "<p>"+long+"</p>")

	body := forwardedBody(env.app, msg)

	if len(body) < len(long) {
		t.Errorf("forwarded body is %d chars, want at least the %d-char message — the snippet truncation leaked back in",
			len(body), len(long))
	}
	if !strings.Contains(body, "---------- Forwarded message ----------") {
		t.Error("forwarded body is missing its header block")
	}
	if !strings.Contains(body, "Invoice line item.") {
		t.Error("forwarded body does not contain the message text")
	}
}

func TestHTMLToForwardText_PreservesLineStructure(t *testing.T) {
	got := htmlToForwardText("<p>First line.</p><p>Second line.</p>")

	// stripHTMLToText alone collapses every whitespace run to one space, which
	// would deliver the forward as a single paragraph.
	if !strings.Contains(got, "First line.\nSecond line.") {
		t.Errorf("htmlToForwardText = %q, want the two paragraphs on separate lines", got)
	}
}

func TestActionStarMessage_StarsForTheAudienceOnly(t *testing.T) {
	env := setupActionEnv(t, "star-personal.test", "mb_star_personal")
	rule := newTestRule(t, env.app, env.alice, "personal")

	err := actionStarMessage(env.app, automation.ActionRequest{
		Rule: rule, OwnerID: env.alice, Record: env.msg,
	})
	if err != nil {
		t.Fatalf("actionStarMessage: %v", err)
	}

	if !threadStateFor(t, env.app, env.threadID, env.alice).GetBool("is_starred") {
		t.Error("alice's thread state is not starred")
	}
	if state := findThreadState(env.app, env.threadID, env.bob); state != nil {
		t.Errorf("bob got thread state from alice's personal rule: starred=%v",
			state.GetBool("is_starred"))
	}
}

// Starring must not refile the thread or mark it read — the same
// independence move-to-folder and mark-as-read hold between them.
func TestActionStarMessage_PreservesFolderAndReadState(t *testing.T) {
	env := setupActionEnv(t, "star-preserves.test", "mb_star_preserve")
	rule := newTestRule(t, env.app, env.alice, "personal")

	if err := ensureThreadState(env.app, env.threadID, env.alice, "archive", true); err != nil {
		t.Fatalf("seed thread state: %v", err)
	}

	if err := actionStarMessage(env.app, automation.ActionRequest{
		Rule: rule, OwnerID: env.alice, Record: env.msg,
	}); err != nil {
		t.Fatalf("actionStarMessage: %v", err)
	}

	state := threadStateFor(t, env.app, env.threadID, env.alice)
	if !state.GetBool("is_starred") {
		t.Error("is_starred = false, want true")
	}
	if folder := state.GetString("folder"); folder != "archive" {
		t.Errorf("folder = %q, want archive — starring must not refile", folder)
	}
	if !state.GetBool("is_read") {
		t.Error("starring cleared is_read")
	}
}
