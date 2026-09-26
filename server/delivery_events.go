package mail

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/deliveryevents"
)

// senderDomainMetadataKey names the metadata field mail's own sends carry so
// a receiver of the provider's delivery/bounce notification can attribute it
// back to a sender without a From address to go on — see endpoints_send.go.
const senderDomainMetadataKey = "sender_domain"

// domainOf returns the part of an email address after the last "@", or ""
// if addr has none. addr@b@c.com resolves as c.com: the last "@" wins,
// matching how a mail server itself parses the address.
func domainOf(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == '@' {
			return addr[i+1:]
		}
	}
	return ""
}

// applyDeliveryEvent is the one place a provider delivery/bounce/complaint
// notification is applied to a stored mail_messages row, whether it arrived
// through this package's own webhook (handleBounce) or was handed over by
// core's deliveryevents registry from a composition where the webhook landed
// elsewhere.
//
// Registered as mail's deliveryevents.Sink (register.go) so both paths run
// the exact same rules.
func applyDeliveryEvent(app core.App, e deliveryevents.Event) (handled bool, err error) {
	messages, err := app.FindRecordsByFilter(
		"mail_messages",
		"message_id = {:id}",
		"",
		1,
		0,
		map[string]any{"id": e.ProviderMessageID},
	)
	if err != nil {
		return false, err
	}
	if len(messages) == 0 {
		return false, nil
	}
	record := messages[0]

	switch e.Kind {
	case deliveryevents.Delivered:
		// A late per-recipient delivery notification must not erase a
		// failure the message has already recorded — that would hide a
		// bounce or complaint behind a stray success for someone else on the
		// send. Recognised and dropped, not "unhandled": nothing else should
		// get a turn at this event either.
		status := record.GetString("delivery_status")
		if status == "bounced" || status == "spam_complaint" {
			return true, nil
		}
		record.Set("delivered_at", e.At)

	case deliveryevents.Bounced:
		record.Set("delivery_status", "bounced")
		record.Set("bounce_class", e.Class)
		record.Set("bounce_reason", truncateRunes(e.Reason, 500))

	case deliveryevents.Complaint:
		record.Set("delivery_status", "spam_complaint")
		record.Set("bounce_class", "complaint")
		record.Set("bounce_reason", truncateRunes(e.Reason, 500))

	default:
		return false, nil
	}

	if err := app.Save(record); err != nil {
		return false, err
	}
	return true, nil
}

// truncateRunes caps s at n runes, matching the bounce_reason field's schema
// limit (500) — a byte-based cut could split a multi-byte character.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// eventFromBounce maps a parsed provider bounce/delivery payload onto the
// package-agnostic deliveryevents.Event, or reports ok = false for a
// notification that doesn't correspond to a delivery outcome at all — an
// auto-responder or unsubscribe notice, say (classifyBounce's
// bounceClassNone with recognised = false is the "unmapped" case; recognised
// = true with bounceClassNone never reaches here as a failure at all, since
// that only occurs for notifications that classifyBounce already treats as
// not a delivery failure — see its NonFailures case, which this function
// still reports as a Bounced event with an empty Class, matching the brief's
// requirement that only genuinely unrecognised types carry Class "").
func eventFromBounce(b *BounceEvent) (deliveryevents.Event, bool) {
	if b == nil || b.MessageID == "" {
		return deliveryevents.Event{}, false
	}

	if b.RecordType == "Delivery" {
		at, _ := time.Parse(time.RFC3339, b.DeliveredAt)
		return deliveryevents.Event{
			Kind:              deliveryevents.Delivered,
			ProviderMessageID: b.MessageID,
			At:                at,
		}, true
	}

	if b.RecordType == "SpamComplaint" {
		return deliveryevents.Event{
			Kind:              deliveryevents.Complaint,
			ProviderMessageID: b.MessageID,
			Class:             "complaint",
			Reason:            b.Description,
			At:                bouncedAtOrNow(b.BouncedAt),
		}, true
	}

	class, _ := classifyBounce(b)
	return deliveryevents.Event{
		Kind:              deliveryevents.Bounced,
		ProviderMessageID: b.MessageID,
		Class:             bounceClassToString(class),
		Reason:            b.Description,
		At:                bouncedAtOrNow(b.BouncedAt),
	}, true
}

// bounceClassToString maps the internal classification onto the wire values
// applyDeliveryEvent and deliveryevents.Event use. bounceClassNone —
// including both an unrecognised type and a recognised non-failure, e.g. an
// unsubscribe notice — carries no class of its own.
func bounceClassToString(c bounceClass) string {
	if c == bounceClassNone {
		return ""
	}
	return c.String()
}

// bouncedAtOrNow parses a provider timestamp, falling back to the current
// time so a malformed or missing BouncedAt never blocks recording the event
// itself — the outcome matters more than exactly when it happened.
func bouncedAtOrNow(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Now().UTC()
}
