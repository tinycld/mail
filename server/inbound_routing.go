package mail

import (
	"errors"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// routingRecipients lists the addresses a message is routed by: the envelope
// recipient first (it is the address the provider delivered this copy for,
// and the only place a BCC appears), then To, then Cc. Lowercased, each once.
func routingRecipients(msg *InboundMessage) []string {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) {
		email := strings.ToLower(strings.TrimSpace(raw))
		if email == "" || seen[email] {
			return
		}
		seen[email] = true
		out = append(out, email)
	}
	add(msg.OriginalRecipient)
	for _, r := range msg.To {
		add(r.Email)
	}
	for _, r := range msg.Cc {
		add(r.Email)
	}
	return out
}

// routeInbound delivers msg to every inbox one of its recipients resolves to.
// A group address resolves to each member's own inbox. One targetSet covers
// all recipients, so an inbox reached through two addresses (its own and a
// list, or two lists) stores one copy, with delivered_to set to the first
// that matched.
func routeInbound(app core.App, msg *InboundMessage) routingResult {
	result := routingResult{}
	var delivered targetSet
	for _, email := range routingRecipients(msg) {
		localPart, domain := splitAddress(email)
		if localPart == "" || domain == "" {
			app.Logger().Warn("inbound: malformed recipient address", "email", email)
			continue
		}
		localPart = stripPlusTag(localPart)

		targets, err := deliveryTargets(app, localPart, domain)
		fullAddr := localPart + "@" + domain
		if errors.Is(err, errAddressNotFound) {
			result.unknownAddrs = append(result.unknownAddrs, fullAddr)
			app.Logger().Info("inbound: unknown recipient", "address", fullAddr, "messageID", msg.MessageID)
			continue
		}
		// A failed lookup is a storage failure: the webhook answers 500 and
		// the provider retries, rather than the message being dropped.
		if err != nil {
			result.storageFailures = append(result.storageFailures, err)
			app.Logger().Error("inbound: recipient lookup failed",
				"address", fullAddr, "messageID", msg.MessageID, "error", err)
			continue
		}

		for _, target := range targets {
			if !delivered.add(target) {
				continue
			}
			if err := processInboundForMailbox(app, target.Mailbox, msg, target.AddressID); err != nil {
				result.storageFailures = append(result.storageFailures, err)
				app.Logger().Error("inbound: storage failed",
					"mailboxID", target.Mailbox.Id, "messageID", msg.MessageID, "error", err)
				continue
			}
			result.storedCount++
		}
	}
	return result
}
