package mail

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestDeliveryStatusForResult_SentWhenNothingFailed(t *testing.T) {
	status, reason := deliveryStatusForResult(&SendResult{MessageID: "m1"}, 3)
	if status != "sent" {
		t.Errorf("status = %q, want sent", status)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

// A partial failure still reached someone, so the message is "sent" — but the
// reason is recorded either way so the sender can see which address failed.
func TestDeliveryStatusForResult_PartialFailureIsStillSent(t *testing.T) {
	result := &SendResult{
		MessageID:        "m1",
		FailedRecipients: []RecipientFailure{{Email: "a@example.com", Reason: "550 no such user"}},
	}
	status, reason := deliveryStatusForResult(result, 3)
	if status != "sent" {
		t.Errorf("status = %q, want sent (2 of 3 recipients still got it)", status)
	}
	if !strings.Contains(reason, "a@example.com") {
		t.Errorf("reason should name the failed recipient, got %q", reason)
	}
}

func TestDeliveryStatusForResult_BouncedWhenEveryRecipientFailed(t *testing.T) {
	result := &SendResult{
		MessageID: "m1",
		FailedRecipients: []RecipientFailure{
			{Email: "a@example.com", Reason: "550 no such user"},
			{Email: "b@example.com", Reason: "550 no such user"},
		},
	}
	status, _ := deliveryStatusForResult(result, 2)
	if status != "bounced" {
		t.Errorf("status = %q, want bounced", status)
	}
}

// The bug this pins: an inflated totalRecipients pushes the "every recipient
// failed" threshold out of reach, so a wholly undeliverable message is
// recorded as "sent". The SMTP call site used to count the raw RCPT envelope
// alongside the parsed headers, double-counting every address that appeared
// in both — one recipient counted as two, and 1 >= 2 is false.
//
// Recording a bounce as "sent" is not cosmetic: delivery_status is what the
// automation loop-breaker counts, and what a user reads in their Sent folder.
func TestDeliveryStatusForResult_InflatedTotalHidesABounce(t *testing.T) {
	result := &SendResult{
		MessageID:        "m1",
		FailedRecipients: []RecipientFailure{{Email: "a@example.com", Reason: "550 no such user"}},
	}

	if status, _ := deliveryStatusForResult(result, 1); status != "bounced" {
		t.Fatalf("with the true count of 1, status = %q, want bounced", status)
	}
	if status, _ := deliveryStatusForResult(result, 2); status == "bounced" {
		t.Fatal("an inflated count should not report bounced — this test exists to " +
			"document why the SMTP call site must pass a deduped recipient count")
	}
}

// The unit tests above cannot see which expression the SMTP path actually
// passes, so this guards the call site itself. s.recipients is the full RCPT
// envelope and repeats addresses that also appear in To/Cc; the deduped bcc
// slice is the one that makes the total match the real fan-out.
func TestSMTPData_DeliveryStatusUsesTheDedupedCount(t *testing.T) {
	const filename = "smtp_session.go"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}

	var args string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "deliveryStatusForResult" {
			return true
		}
		last := call.Args[len(call.Args)-1]
		args = string(src[fset.Position(last.Pos()).Offset:fset.Position(last.End()).Offset])
		return false
	})

	if args == "" {
		t.Fatal("no deliveryStatusForResult call found in smtp_session.go — " +
			"if it moved, update this test rather than deleting it")
	}
	if strings.Contains(args, "s.recipients") {
		t.Errorf("must not count the raw RCPT envelope (double-counts To/Cc); got %q", args)
	}
	if !strings.Contains(args, "bcc") {
		t.Errorf("must count the deduped bcc slice; got %q", args)
	}
}
