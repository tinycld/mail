package mail

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/audit"
)

// settingsCache caches settings per org to avoid DB queries on every request.
// Invalidated by PocketBase record hooks on the settings collection.
var settingsCache sync.Map // map[string]map[string]string  (orgID → {key: value})

func Register(app *pocketbase.PocketBase) {
	// Audit logging for mail collections
	audit.RegisterCollection(app, "mail_domains", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("domain"),
	})

	resolveOrgViaMailbox := func(a core.App, mailboxID string) string {
		mailbox, err := a.FindRecordById("mail_mailboxes", mailboxID)
		if err != nil {
			return ""
		}
		domainID := mailbox.GetString("domain")
		if domainID == "" {
			return ""
		}
		return audit.ResolveViaRelation(a, "mail_domains", domainID, "org")
	}

	audit.RegisterCollection(app, "mail_mailboxes", &audit.CollectionConfig{
		ResolveOrg: func(a core.App, record *core.Record) string {
			domainID := record.GetString("domain")
			if domainID == "" {
				return ""
			}
			return audit.ResolveViaRelation(a, "mail_domains", domainID, "org")
		},
		ExtractLabel: audit.LabelFromField("address"),
	})

	audit.RegisterCollection(app, "mail_mailbox_members", &audit.CollectionConfig{
		ResolveOrg: func(a core.App, record *core.Record) string {
			mailboxID := record.GetString("mailbox")
			if mailboxID == "" {
				return ""
			}
			return resolveOrgViaMailbox(a, mailboxID)
		},
	})

	resolveOrgViaThread := func(a core.App, threadID string) string {
		thread, err := a.FindRecordById("mail_threads", threadID)
		if err != nil {
			return ""
		}
		mailboxID := thread.GetString("mailbox")
		if mailboxID == "" {
			return ""
		}
		return resolveOrgViaMailbox(a, mailboxID)
	}

	audit.RegisterCollection(app, "mail_messages", &audit.CollectionConfig{
		ResolveOrg: func(a core.App, record *core.Record) string {
			threadID := record.GetString("thread")
			if threadID == "" {
				return ""
			}
			return resolveOrgViaThread(a, threadID)
		},
		ExtractLabel: audit.LabelFromField("subject"),
	})

	audit.RegisterCollection(app, "mail_thread_state", &audit.CollectionConfig{
		ResolveOrg: func(a core.App, record *core.Record) string {
			threadID := record.GetString("thread")
			if threadID == "" {
				return ""
			}
			return resolveOrgViaThread(a, threadID)
		},
		ExtractLabel: audit.LabelFromField("folder"),
	})

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

	// FTS sync hooks for mail_threads
	app.OnRecordAfterCreateSuccess("mail_threads").BindFunc(func(e *core.RecordEvent) error {
		syncThreadToFTS(app, e.Record, "create")
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess("mail_threads").BindFunc(func(e *core.RecordEvent) error {
		syncThreadToFTS(app, e.Record, "update")
		return e.Next()
	})
	app.OnRecordAfterDeleteSuccess("mail_threads").BindFunc(func(e *core.RecordEvent) error {
		syncThreadToFTS(app, e.Record, "delete")
		return e.Next()
	})

	// FTS sync hooks for mail_messages
	// On create: skip if storeMessage() already indexed inline (full body + attachments).
	// Otherwise load HTML + attachments from storage for a complete index.
	app.OnRecordAfterCreateSuccess("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		if _, ok := recentlyIndexed.LoadAndDelete(e.Record.Id); ok {
			return e.Next()
		}
		indexMessageRecordFromStorage(app, e.Record)
		return e.Next()
	})
	// On update: always re-index from storage (e.g. draft edits via updateDraftRecord).
	app.OnRecordAfterUpdateSuccess("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		indexMessageRecordFromStorage(app, e.Record)
		return e.Next()
	})
	app.OnRecordAfterDeleteSuccess("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		deleteMessageFromFTS(app, e.Record.Id)
		return e.Next()
	})

	// IMAP IDLE notifications: notify when messages are created or thread state changes
	app.OnRecordAfterCreateSuccess("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		threadID := e.Record.GetString("thread")
		thread, err := app.FindRecordById("mail_threads", threadID)
		if err == nil {
			globalNotifier.notify(thread.GetString("mailbox"))
		}
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess("mail_thread_state").BindFunc(func(e *core.RecordEvent) error {
		threadID := e.Record.GetString("thread")
		thread, err := app.FindRecordById("mail_threads", threadID)
		if err == nil {
			globalNotifier.notify(thread.GetString("mailbox"))
		}
		return e.Next()
	})

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Start IMAP server
		imapShutdown, err := StartIMAPServer(app, e.CertManager)
		if err != nil {
			app.Logger().Error("Failed to start IMAP server", "error", err)
		} else {
			app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
				imapShutdown()
				return te.Next()
			})
		}

		// Start SMTP submission server
		smtpShutdown, smtpErr := StartSMTPServer(app, e.CertManager)
		if smtpErr != nil {
			app.Logger().Error("Failed to start SMTP server", "error", smtpErr)
		} else {
			app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
				smtpShutdown()
				return te.Next()
			})
		}

		// Send endpoint (requires auth, resolves provider from org settings)
		e.Router.POST("/api/mail/send", func(re *core.RequestEvent) error {
			return handleSend(app, re)
		}).BindFunc(requireAuth)

		// Draft endpoint (requires auth, saves without sending)
		e.Router.POST("/api/mail/draft", func(re *core.RequestEvent) error {
			return handleDraft(app, re)
		}).BindFunc(requireAuth)

		// Search endpoint (requires auth)
		e.Router.GET("/api/mail/search", func(re *core.RequestEvent) error {
			return handleSearch(app, re)
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

		// Image proxy (auth via query token, since sandboxed iframes can't send headers)
		e.Router.GET("/api/mail/image-proxy", func(re *core.RequestEvent) error {
			return handleImageProxyRequest(app, re)
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

// indexMessageRecordFromStorage builds FTS body_text from the record's stored
// HTML body and text-based attachments. Used by record hooks when storeMessage()
// wasn't involved or on updates.
func indexMessageRecordFromStorage(app *pocketbase.PocketBase, record *core.Record) {
	bodyText := record.GetString("snippet") // fallback

	html := loadHTMLBody(app, record)
	if html != "" {
		bodyText = stripHTMLToText(html)
	}

	attachmentText := loadTextAttachments(app, record)

	syncMessageToFTS(app, record.Id, &storedMessage{
		Subject:     record.GetString("subject"),
		SenderName:  record.GetString("sender_name"),
		SenderEmail: record.GetString("sender_email"),
		TextBody:    bodyText,
	}, attachmentText)
}

// requireAuth is a middleware that ensures the request has a valid auth token.
func requireAuth(re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Authentication required", nil)
	}
	return re.Next()
}
