package mail

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/packages/mail/api"
)

func handleBounce(app core.App, provider Provider, re *core.RequestEvent, secret string) error {
	token := re.Request.PathValue("token")
	if secret == "" || subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
		return re.ForbiddenError("Invalid token", nil)
	}

	body, err := io.ReadAll(io.LimitReader(re.Request.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return re.BadRequestError("Failed to read request body", err)
	}

	event, err := provider.ParseBounce(body)
	if err != nil {
		return re.BadRequestError("Failed to parse bounce payload", err)
	}

	if event.MessageID == "" {
		return re.BadRequestError("Missing message ID in bounce payload", nil)
	}

	// Look up the message by provider message_id
	messages, err := app.FindRecordsByFilter(
		"mail_messages",
		"message_id = {:messageID}",
		"",
		1,
		0,
		map[string]any{"messageID": event.MessageID},
	)
	if err != nil || len(messages) == 0 {
		app.Logger().Warn("bounce received for unknown message",
			"message_id", event.MessageID, "email", event.Email)
		return re.JSON(http.StatusOK, api.WebhookAckResponse{Status: "ignored"})
	}

	record := messages[0]

	// The stored status and the classification answer different questions and
	// are deliberately kept apart. delivery_status is what the sender sees in
	// their Sent folder — "this bounced" — and its values are pinned by the
	// collection's schema and read by the automation loop-breaker. The class
	// is what the failure MEANS, and only some meanings say anything about
	// the sender. Collapsing one into the other would either change what a
	// user sees or lose the distinction that makes the failure interpretable.
	status := "bounced"
	if event.RecordType == "SpamComplaint" {
		status = "spam_complaint"
	}

	class, recognised := classifyBounce(event)
	if !recognised {
		// Not counted, and loud about it. A failure type nobody has mapped is
		// a gap in whatever judges senders by these, and it should be noticed
		// by a person rather than absorbed silently into a bucket.
		app.Logger().Warn("unrecognised bounce type; not classified",
			"record_type", event.RecordType, "bounce_type", event.BounceType,
			"type_code", event.TypeCode, "message_id", event.MessageID)
	}

	record.Set("delivery_status", status)

	reason := event.Description
	if len([]rune(reason)) > 500 {
		reason = string([]rune(reason)[:500])
	}
	record.Set("bounce_reason", reason)

	if err := app.Save(record); err != nil {
		return re.InternalServerError(fmt.Sprintf("Failed to update message %s", record.Id), err)
	}

	// The class, the sender and the provider's own id are logged because they
	// are what anything downstream needs and what an operator reconstructing
	// an incident will look for. The id in particular is the only way to tell
	// a replayed notification from a second real one.
	app.Logger().Info("bounce processed",
		"message_id", event.MessageID, "status", status, "class", class.String(),
		"bounce_id", event.ID, "from", event.From, "email", event.Email,
		"inactive", event.Inactive)

	return re.JSON(http.StatusOK, api.WebhookAckResponse{Status: "processed"})
}
