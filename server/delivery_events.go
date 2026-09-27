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
		record.Set("bounce_class", validBounceClassOrEmpty(e.Class))
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

// validBounceClassOrEmpty guards the mail_messages bounce_class SelectField,
// whose schema only accepts "soft", "hard" or "complaint". e.Class arrives
// over the wire — from this package's own webhook, but also handed over by
// core's deliveryevents registry from a composition where the notification
// landed elsewhere — so an unexpected value must not reach Save, where the
// SelectField's own values check would reject the record and turn the whole
// call into a 500 instead of simply storing no class.
func validBounceClassOrEmpty(class string) string {
	switch class {
	case "soft", "hard", "complaint":
		return class
	default:
		return ""
	}
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
// package-agnostic deliveryevents.Event.
//
// ok reports whether the payload identifies a message at all (a non-empty
// MessageID), AND whether there is an event to apply — false also covers a
// RECOGNISED non-failure (an auto-responder, an unsubscribe: classifyBounce
// returns recognised=true with bounceClassNone), matching the hosted router,
// which attributes and logs these but never forwards them because the
// message has no outcome to record. recognised is narrower still: it is
// classifyBounce's own verdict on whether this specific failure type is one
// mail knows how to classify, kept separate from ok so an UNMAPPED bounce
// type still gets applied as a Bounced event (with an empty Class) AND still
// gets its own warning logged by the caller — folding the two into one bool
// silenced that warning for every unrecognised type.
func eventFromBounce(b *BounceEvent) (event deliveryevents.Event, recognised bool, ok bool) {
	if b == nil || b.MessageID == "" {
		return deliveryevents.Event{}, false, false
	}

	if b.RecordType == "Delivery" {
		return deliveryevents.Event{
			Kind:              deliveryevents.Delivered,
			ProviderMessageID: b.MessageID,
			At:                bouncedAtOrNow(b.DeliveredAt),
		}, true, true
	}

	if b.RecordType == "SpamComplaint" {
		return deliveryevents.Event{
			Kind:              deliveryevents.Complaint,
			ProviderMessageID: b.MessageID,
			Class:             "complaint",
			Reason:            b.Description,
			At:                bouncedAtOrNow(b.BouncedAt),
		}, true, true
	}

	class, recognised := classifyBounce(b)
	if recognised && class == bounceClassNone {
		// A recognised non-failure: nothing to apply. Returning ok=false here
		// — rather than a Bounced event with an empty class — is what keeps
		// an auto-responder or unsubscribe notice from showing as "Bounced"
		// on the address step and blocking a later real Delivered event.
		return deliveryevents.Event{}, true, false
	}
	return deliveryevents.Event{
		Kind:              deliveryevents.Bounced,
		ProviderMessageID: b.MessageID,
		Class:             bounceClassToString(class),
		Reason:            b.Description,
		At:                bouncedAtOrNow(b.BouncedAt),
	}, recognised, true
}

// bounceClassToString maps the internal classification onto the wire values
// applyDeliveryEvent and deliveryevents.Event use. bounceClassNone reaches
// here only for an UNRECOGNISED type — a recognised non-failure is filtered
// out earlier in eventFromBounce — and carries no class of its own.
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
