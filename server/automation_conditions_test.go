package mail

import (
	"encoding/json"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
)

// messageReceivedFields is the field allowlist of the "message-received"
// trigger in tinycld/mail/automation.ts. The engine only evaluates a condition
// on an allowlisted field, so the arrival tests run with the same list a
// deployment generates. tests/automation.test.ts pins the TS side.
var messageReceivedFields = []automation.FieldRef{
	{Key: "subject"},
	{Key: "sender_email", Label: "Sender"},
	{Key: "sender_name", Label: "Sender name"},
	{Key: "has_attachments"},
	{Key: "delivered_to", Label: "Received at address"},
}

func receivedAtCondition(addressID string) map[string]any {
	return map[string]any{
		"match": "all",
		"groups": []map[string]any{{
			"match":      "all",
			"conditions": []map[string]any{{"field": "delivered_to", "op": "is", "value": addressID}},
		}},
	}
}

func (e *loopEnv) receivedAtRule(t *testing.T, owner, address *core.Record) *core.Record {
	t.Helper()
	return saveRealRecord(t, e.app, "rules", map[string]any{
		"name": "received at", "scope": "personal", "owner": owner.Id,
		"trigger": "mail:message-received", "enabled": true,
		"conditions": receivedAtCondition(address.Id),
	})
}

func runSummary(t *testing.T, run *core.Record) map[string]string {
	t.Helper()
	var summary map[string]string
	if err := json.Unmarshal([]byte(run.GetString("trigger_summary")), &summary); err != nil {
		t.Fatalf("decode trigger_summary: %v", err)
	}
	return summary
}

// A rule on "Received at address" must match mail that arrives at that
// address, through the real webhook, router and engine. It once read alias,
// which inbound mail never sets, so the rule never matched anything.
func TestArrivalRule_ReceivedAtAddressMatchesInboundMail(t *testing.T) {
	e := seedLoopEnv(t)
	startRuleEngine(t, e)
	own := e.ownAddress(t, e.ann)
	rule := e.receivedAtRule(t, e.ann, own)

	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Direct", "<direct-1@sender.example>"))

	runs := waitForRuleRuns(t, e.app, rule.Id, 1)
	if !runs[0].GetBool("matched") {
		t.Fatalf("mail to ann@q.example did not match a rule on that address; summary = %v", runSummary(t, runs[0]))
	}
	if got := runSummary(t, runs[0])["delivered_to"]; got != own.Id {
		t.Fatalf("trigger_summary delivered_to = %q, want %q", got, own.Id)
	}
}

// Mail to a group address lands in ann's inbox with delivered_to set to the
// group address, so a rule on her own address must not match it, and a rule
// on the group address must.
func TestArrivalRule_ReceivedAtAddressTellsGroupMailApart(t *testing.T) {
	e := seedLoopEnv(t)
	startRuleEngine(t, e)
	ownRule := e.receivedAtRule(t, e.ann, e.ownAddress(t, e.ann))
	groupRule := e.receivedAtRule(t, e.ann, e.salesAddr)

	postmarkInbound(t, e.app, inboundPayload("sales@q.example", "To sales", "<sales-1@sender.example>"))

	if runs := waitForRuleRuns(t, e.app, ownRule.Id, 1); runs[0].GetBool("matched") {
		t.Fatal("mail to sales@q.example matched a rule on ann@q.example")
	}
	if runs := waitForRuleRuns(t, e.app, groupRule.Id, 1); !runs[0].GetBool("matched") {
		t.Fatal("mail to sales@q.example did not match a rule on sales@q.example")
	}
}
