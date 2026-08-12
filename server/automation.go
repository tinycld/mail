package mail

import (
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
)

// registerAutomationResolver installs mail's OwnerResolver and TriggerFilter
// for the "mail:message-received" trigger.
//
// mail_messages has no direct user FK — a message belongs to whichever users
// are members of its mailbox — so the generic owner auto-detection in
// automation.ResolveOwners can't find it (OwnerResolver).
//
// The mail_messages create hook fires for every kind of message row, not
// just genuinely received ones: composing a draft, sending outbound mail,
// and a bounce/spam-complaint webhook update all pass through it too. A
// trigger filter gates the event at the source so drafts and outbound sends
// never enqueue "message-received" runs, for org-scoped rules as well as
// personal ones (TriggerFilter, unlike OwnerResolver, applies to both).
func registerAutomationResolver() {
	automation.RegisterOwnerResolver("mail:message-received", messageOwnerResolver)
	automation.RegisterTriggerFilter("mail:message-received", messageIsInbound)
}

// messageOwnerResolver maps an arriving mail_messages record to the user ids
// who own it, via the same thread -> mailbox -> mail_mailbox_members path
// bufferMailNotification uses to pick notification recipients
// (mailboxMembersForMessage, register.go). Returns nil (never an error) on
// any absent or malformed data so org-level rules still fire even when a
// personal-rule owner can't be determined.
func messageOwnerResolver(app core.App, record *core.Record) []string {
	if record == nil {
		return nil
	}

	members := mailboxMembersForMessage(app, record)
	if len(members) == 0 {
		return nil
	}

	var owners []string
	for _, member := range members {
		if userID := member.GetString("user"); userID != "" {
			owners = append(owners, userID)
		}
	}
	return owners
}

// outboundOrNonReceivedDeliveryStatuses are the delivery_status values a
// genuinely received message never has: "draft" (compose, endpoints_draft.go),
// "sent"/"bounced" (outbound send outcome, deliveryStatusForResult in
// endpoints_send.go / smtp_session.go — a synchronous send result is never
// left as "sending"), and "spam_complaint" (bounce webhook update,
// endpoints_bounce.go). A message that just arrived over the inbound webhook
// or IMAP sync reads as "sending" (storeMessage's default when unset) or
// "delivered" (APPEND-to-inbox) — anything not in this set counts as
// received.
//
// bufferMailNotification's own outbound guard reads a "direction" field that
// mail_messages does not have in the shipped schema (no migration ever adds
// it, no Go code ever sets it) — that check is dead and never skips
// anything. delivery_status is the field mail's own send/draft/inbound code
// paths actually set, so it is the real signal to gate on here.
var outboundOrNonReceivedDeliveryStatuses = map[string]bool{
	"draft":          true,
	"sent":           true,
	"bounced":        true,
	"spam_complaint": true,
}

// messageIsInbound reports whether record is a genuinely received message —
// the TriggerFilter for "mail:message-received", so drafts, outbound sends,
// and bounce/spam-complaint updates never enqueue a run for this trigger.
func messageIsInbound(app core.App, record *core.Record) bool {
	if record == nil {
		return false
	}
	return !outboundOrNonReceivedDeliveryStatuses[record.GetString("delivery_status")]
}
