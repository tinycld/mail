package mail

import (
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
// processInboundForMailbox dedups by Message-ID per inbox, so two addresses of
// one inbox store one copy, with delivered_to set to the first that matched.
func routeInbound(app core.App, msg *InboundMessage) routingResult {
	result := routingResult{}
	for _, email := range routingRecipients(msg) {
		localPart, domain := splitAddress(email)
		if localPart == "" || domain == "" {
			app.Logger().Warn("inbound: malformed recipient address", "email", email)
			continue
		}
		localPart = stripPlusTag(localPart)

		target, err := resolveAddress(app, localPart, domain)
		if err != nil {
			fullAddr := localPart + "@" + domain
			result.unknownAddrs = append(result.unknownAddrs, fullAddr)
			app.Logger().Info("inbound: unknown recipient", "address", fullAddr, "messageID", msg.MessageID)
			continue
		}

		if err := processInboundForMailbox(app, target.Mailbox, msg, target.Address.Id); err != nil {
			result.storageFailures = append(result.storageFailures, err)
			app.Logger().Error("inbound: storage failed",
				"mailboxID", target.Mailbox.Id, "messageID", msg.MessageID, "error", err)
			continue
		}
		result.storedCount++
	}
	return result
}
