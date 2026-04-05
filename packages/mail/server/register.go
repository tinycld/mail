package mail

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// settingsCache caches settings per org to avoid DB queries on every request.
// Invalidated by PocketBase record hooks on the settings collection.
var settingsCache sync.Map // map[string]map[string]string  (orgID → {key: value})

func Register(app *pocketbase.PocketBase) {
	// Env-based provider used for unauthenticated webhooks (no org context)
	webhookProvider := newProviderFromEnv()

	// Invalidate settings cache when settings records change
	invalidateSettingsCache := func(e *core.RecordEvent) error {
		orgID := e.Record.GetString("org")
		if orgID != "" {
			settingsCache.Delete(orgID)
		}
		return e.Next()
	}
	// Auto-create personal mailbox when a user joins an org
	app.OnRecordAfterCreateSuccess("user_org").BindFunc(func(e *core.RecordEvent) error {
		handleUserOrgCreated(app, e.Record)
		return e.Next()
	})

	// Clean up orphaned personal mailboxes when a user leaves an org
	app.OnRecordAfterDeleteSuccess("user_org").BindFunc(func(e *core.RecordEvent) error {
		handleUserOrgDeleted(app, e.Record)
		return e.Next()
	})

	app.OnRecordAfterCreateSuccess("settings").BindFunc(invalidateSettingsCache)
	app.OnRecordAfterUpdateSuccess("settings").BindFunc(invalidateSettingsCache)
	app.OnRecordAfterDeleteSuccess("settings").BindFunc(invalidateSettingsCache)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Send endpoint (requires auth, resolves provider from org settings)
		e.Router.POST("/api/mail/send", func(re *core.RequestEvent) error {
			return handleSend(app, re)
		}).BindFunc(requireAuth)

		// Draft endpoint (requires auth, saves without sending)
		e.Router.POST("/api/mail/draft", func(re *core.RequestEvent) error {
			return handleDraft(app, re)
		}).BindFunc(requireAuth)

		// Inbound webhook (unauthenticated, secured via secret token)
		inboundSecret := os.Getenv("MAIL_INBOUND_SECRET")
		e.Router.POST("/api/mail/inbound/{token}", func(re *core.RequestEvent) error {
			return handleInbound(app, webhookProvider, re, inboundSecret)
		})

		// Bounce webhook (unauthenticated, secured via secret token)
		bounceSecret := os.Getenv("MAIL_BOUNCE_SECRET")
		if bounceSecret == "" {
			bounceSecret = inboundSecret
		}
		e.Router.POST("/api/mail/bounces/{token}", func(re *core.RequestEvent) error {
			return handleBounce(app, webhookProvider, re, bounceSecret)
		})

		return e.Next()
	})
}

func newProviderFromEnv() Provider {
	name := os.Getenv("MAIL_PROVIDER")
	if name == "" {
		name = "postmark"
	}
	return newProviderByName(name, os.Getenv("POSTMARK_SERVER_TOKEN"), os.Getenv("POSTMARK_ACCOUNT_TOKEN"))
}

func newProviderByName(name, serverToken, accountToken string) Provider {
	switch name {
	case "postmark":
		if serverToken == "" {
			return &NoopProvider{}
		}
		return NewPostmarkProvider(serverToken, accountToken)
	default:
		return &NoopProvider{}
	}
}

// providerForOrg reads mail provider settings from the settings table for the
// given org, falling back to environment variables when no settings are stored.
func providerForOrg(app *pocketbase.PocketBase, orgID string) Provider {
	settings := getOrgSettings(app, "mail", orgID)

	name := settings["provider"]
	serverToken := settings["postmark_server_token"]
	accountToken := settings["postmark_account_token"]

	// Fall back to env vars when org has no settings
	if name == "" {
		name = os.Getenv("MAIL_PROVIDER")
	}
	if name == "" {
		name = "postmark"
	}
	if serverToken == "" {
		serverToken = os.Getenv("POSTMARK_SERVER_TOKEN")
	}
	if accountToken == "" {
		accountToken = os.Getenv("POSTMARK_ACCOUNT_TOKEN")
	}

	return newProviderByName(name, serverToken, accountToken)
}

// getOrgSettings returns all settings for an app+org as a key→value map.
// Results are cached in memory and invalidated by record hooks.
func getOrgSettings(app *pocketbase.PocketBase, appName, orgID string) map[string]string {
	cacheKey := orgID + ":" + appName

	if cached, ok := settingsCache.Load(cacheKey); ok {
		return cached.(map[string]string)
	}

	result := make(map[string]string)

	records, err := app.FindRecordsByFilter(
		"settings",
		"app = {:app} && org = {:org}",
		"",
		100,
		0,
		map[string]any{"app": appName, "org": orgID},
	)
	if err == nil {
		for _, r := range records {
			key := r.GetString("key")
			val := r.Get("value")
			if s, ok := val.(string); ok {
				result[key] = s
			} else if b, ok := val.(json.RawMessage); ok {
				var s string
				if json.Unmarshal(b, &s) == nil {
					result[key] = s
				}
			}
		}
	}

	settingsCache.Store(cacheKey, result)
	return result
}

// requireAuth is a middleware that ensures the request has a valid auth token.
func requireAuth(re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Authentication required", nil)
	}
	return re.Next()
}
