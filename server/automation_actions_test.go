package mail

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"tinycld.org/core/automation"
)

// automation_actions_test.go covers the four native actions mail registers:
//
//   - the audience rule that decides who an action acts for — an org rule
//     applies to every mailbox member, a personal rule only to its owner, and
//     neither may touch a non-member;
//   - move-to-folder and mark-as-read each changing only their own field, so
//     filing a message doesn't silently mark it read (or vice versa);
//   - the folder allow-list rejecting destinations the UI has no view for;
//   - the per-rule hourly send cap that bounds an auto-reply loop.
//
// Sending itself is not exercised here: sendMessage needs a configured
// provider, which the endpoint tests already cover.

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

func TestCheckSendRateLimit(t *testing.T) {
	tests := []struct {
		name       string
		recentRuns int
		age        time.Duration
		wantErr    bool
	}{
		{name: "no history sends", recentRuns: 0, age: time.Minute, wantErr: false},
		{name: "under the cap sends", recentRuns: maxSendsPerRulePerHour - 1, age: time.Minute, wantErr: false},
		{name: "at the cap is blocked", recentRuns: maxSendsPerRulePerHour, age: time.Minute, wantErr: true},
		{
			// The cap is a rolling hour, not a lifetime total: yesterday's
			// burst must not stop today's mail.
			name: "old runs fall outside the window", recentRuns: maxSendsPerRulePerHour + 5,
			age: 2 * time.Hour, wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := setupInboundTestApp(t)
			newRulesCollections(t, app)
			rule := newTestRule(t, app, "user_alice_0000", "personal")

			for range tc.recentRuns {
				seedRun(t, app, rule.Id, tc.age)
			}

			err := checkSendRateLimit(app, automation.ActionRequest{Rule: rule})
			if tc.wantErr && err == nil {
				t.Error("expected the send to be blocked, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected the send to be allowed, got %v", err)
			}
		})
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
