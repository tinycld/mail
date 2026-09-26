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

	deliveryEvent, recognised := eventFromBounce(event)
	if !recognised {
		// Not counted, and loud about it. A failure type nobody has mapped is
		// a gap in whatever judges senders by these, and it should be noticed
		// by a person rather than absorbed silently into a bucket.
		app.Logger().Warn("unrecognised bounce type; not classified",
			"record_type", event.RecordType, "bounce_type", event.BounceType,
			"type_code", event.TypeCode, "message_id", event.MessageID)
	}

	handled, err := applyDeliveryEvent(app, deliveryEvent)
	if err != nil {
		return re.InternalServerError(fmt.Sprintf("Failed to apply delivery event for message %s", event.MessageID), err)
	}
	if !handled {
		app.Logger().Warn("bounce received for unknown message",
			"message_id", event.MessageID, "email", event.Email)
		return re.JSON(http.StatusOK, api.WebhookAckResponse{Status: "ignored"})
	}

	// The class, the sender and the provider's own id are logged because they
	// are what anything downstream needs and what an operator reconstructing
	// an incident will look for. The id in particular is the only way to tell
	// a replayed notification from a second real one.
	app.Logger().Info("bounce processed",
		"message_id", event.MessageID, "kind", deliveryEvent.Kind, "class", deliveryEvent.Class,
		"bounce_id", event.ID, "from", event.From, "email", event.Email,
		"inactive", event.Inactive)

	return re.JSON(http.StatusOK, api.WebhookAckResponse{Status: "processed"})
}
