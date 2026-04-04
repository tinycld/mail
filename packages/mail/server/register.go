package mail

import (
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func Register(app *pocketbase.PocketBase) {
	provider := newProviderFromEnv()

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Send endpoint (requires auth)
		e.Router.POST("/api/mail/send", func(re *core.RequestEvent) error {
			return handleSend(app, provider, re)
		}).BindFunc(requireAuth)

		// Inbound webhook (unauthenticated, secured via secret token)
		inboundSecret := os.Getenv("MAIL_INBOUND_SECRET")
		e.Router.POST("/api/mail/inbound/{token}", func(re *core.RequestEvent) error {
			return handleInbound(app, provider, re, inboundSecret)
		})

		return e.Next()
	})
}

func newProviderFromEnv() Provider {
	name := os.Getenv("MAIL_PROVIDER")
	if name == "" {
		name = "postmark"
	}

	switch name {
	case "postmark":
		serverToken := os.Getenv("POSTMARK_SERVER_TOKEN")
		accountToken := os.Getenv("POSTMARK_ACCOUNT_TOKEN")
		if serverToken == "" {
			return &NoopProvider{}
		}
		return NewPostmarkProvider(serverToken, accountToken)
	default:
		return &NoopProvider{}
	}
}

// requireAuth is a middleware that ensures the request has a valid auth token.
func requireAuth(re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Authentication required", nil)
	}
	return re.Next()
}
