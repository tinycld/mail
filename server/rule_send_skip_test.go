package mail

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
	"tinycld.org/core/mailer"
)

// rule_send_skip_test.go runs "a message arrives" rules through the real
// automation engine, so the run history shows what the user sees: a send in
// reply to automatic mail is "skipped", not "ok".

// startRuleEngine starts an engine with core's send-email action and mail's
// trigger and send actions, as a deployment registers them. Core's send goes
// through the core mailer to the same fake Postmark as mail's sends.
func startRuleEngine(t *testing.T, e *loopEnv) {
	t.Helper()
	t.Cleanup(automation.ResetRegistriesForTest)
	t.Cleanup(automation.ResetRunStateForTest)

	previous := mailer.ConfigResolver
	mailer.ConfigResolver = func(key string) string {
		if key == "mail.postmark_server_token" {
			return "test-token"
		}
		return previous(key)
	}
	t.Cleanup(func() { mailer.ConfigResolver = previous })

	text := func(keys ...string) []automation.ParamDef {
		params := make([]automation.ParamDef, 0, len(keys))
		for _, key := range keys {
			params = append(params, automation.ParamDef{Key: key, Type: "text"})
		}
		return params
	}
	defs := &automation.Defs{Packages: []automation.PackageDefs{
		{Slug: "core", Actions: []automation.ActionDef{
			{ID: "send-email", Label: "Send an email", Kind: "native", Params: text("to", "subject", "body")},
		}},
		{
			Slug: "mail",
			Triggers: []automation.TriggerDef{
				{ID: "message-received", Label: "A message arrives", Collection: "mail_messages", On: "create"},
			},
			Actions: []automation.ActionDef{
				{ID: "forward-message", Label: "Forward", Kind: "native", Params: text("to")},
				{ID: "send-message", Label: "Send", Kind: "native", Params: text("to", "subject", "body")},
			},
		},
	}}
	automation.RegisterCoreNativeActions()
	registerAutomationResolver()
	registerAutomationActions()
	automation.NewEngine(e.app, defs).Start()
}

func (e *loopEnv) arrivalRule(t *testing.T, owner *core.Record, actions ...map[string]any) *core.Record {
	t.Helper()
	return saveRealRecord(t, e.app, "rules", map[string]any{
		"name": "reply on arrival", "scope": "personal", "owner": owner.Id,
		"trigger": "mail:message-received", "enabled": true, "actions": actions,
	})
}

func waitForRuleRuns(t *testing.T, app core.App, ruleID string, want int) []*core.Record {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "-fired_at", 0, 0, map[string]any{"id": ruleID})
		if len(runs) >= want {
			return runs
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d runs of rule %s", want, ruleID)
	return nil
}

func ruleRunResults(t *testing.T, run *core.Record) []automation.ActionResult {
	t.Helper()
	var results []automation.ActionResult
	if err := json.Unmarshal([]byte(run.GetString("results")), &results); err != nil {
		t.Fatalf("decode results: %v", err)
	}
	return results
}

var coreSendEmail = map[string]any{
	"ref":    "core:send-email",
	"params": map[string]any{"to": "sender@sender.example", "subject": "Re: {{subject}}", "body": "Thanks"},
}

func TestArrivalRule_CoreSendEmailSkipsAutomaticMail(t *testing.T) {
	e := seedLoopEnv(t)
	startRuleEngine(t, e)
	rule := e.arrivalRule(t, e.ann, coreSendEmail)

	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Out of office", "<ooo-1@sender.example>",
		Header{Name: "Auto-Submitted", Value: "auto-replied"}))

	runs := waitForRuleRuns(t, e.app, rule.Id, 1)
	want := automation.ActionResult{Ref: "core:send-email", Status: "skipped", Message: automaticMailReason}
	if got := ruleRunResults(t, runs[0]); len(got) != 1 || got[0] != want {
		t.Fatalf("results = %+v, want [%+v]", got, want)
	}
	requireSends(t, e.sent, 0)
}

func TestArrivalRule_CoreSendEmailRepliesToPersonMail(t *testing.T) {
	e := seedLoopEnv(t)
	startRuleEngine(t, e)
	rule := e.arrivalRule(t, e.ann, coreSendEmail)

	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Question", "<q-1@sender.example>"))

	runs := waitForRuleRuns(t, e.app, rule.Id, 1)
	if got := ruleRunResults(t, runs[0]); len(got) != 1 || got[0].Status != "ok" {
		t.Fatalf("results = %+v, want one ok", got)
	}
	requireSends(t, e.sent, 1)
	if got := sentHeader(t, (*e.sent)[0], "Auto-Submitted"); got != "auto-generated" {
		t.Fatalf("Auto-Submitted = %q, want auto-generated", got)
	}
	if got := sentHeader(t, (*e.sent)[0], "X-TinyCld-Loop"); got != "mail.q.example" {
		t.Fatalf("X-TinyCld-Loop = %q, want mail.q.example", got)
	}
}

func TestArrivalRule_MailSendActionsShowSkipped(t *testing.T) {
	e := seedLoopEnv(t)
	startRuleEngine(t, e)
	rule := e.arrivalRule(t, e.ann,
		map[string]any{"ref": "mail:forward-message", "params": map[string]any{"to": "accounts@ext.example"}},
		map[string]any{"ref": "mail:send-message", "params": map[string]any{
			"to": "sender@sender.example", "subject": "Got it", "body": "Thanks",
		}},
	)

	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Newsletter", "<news-1@sender.example>",
		Header{Name: "Precedence", Value: "list"}))

	runs := waitForRuleRuns(t, e.app, rule.Id, 1)
	want := []automation.ActionResult{
		{Ref: "mail:forward-message", Status: "skipped", Message: automaticMailReason},
		{Ref: "mail:send-message", Status: "skipped", Message: automaticMailReason},
	}
	got := ruleRunResults(t, runs[0])
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("results = %+v, want %+v", got, want)
	}
	requireSends(t, e.sent, 0)
}
