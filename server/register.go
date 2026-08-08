package mail

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/audit"
	"tinycld.org/core/coreserver"
	"tinycld.org/core/quota"
	"tinycld.org/core/search"
	"tinycld.org/packages/mail/api"
)

// appIsLive reports whether the app still has an open database connection.
// The mail thumbnail + notification workers run in background goroutines that
// can outlive the app instance — e.g. the test harness resets the dev DB while
// a job is in flight. Once the app is torn down, ConcurrentDB() is nil and any
// record query (PocketBase v0.38 RecordQuery) panics on the nil DB instead of
// returning an error. Bail out instead of touching the DB in that window.
func appIsLive(app core.App) bool {
	return app != nil && app.ConcurrentDB() != nil
}

// Register composes the mail server — the package's single entry point,
// called by the generator's package_extensions.go in BOTH the single-org app
// and a multi-org tenant. Mail is the rare package whose composition genuinely
// differs hosted, so it DETECTS tenancy (coreserver.GetTenantContext — the
// single-Register contract) to pick where its protocol listeners come from:
//
//   - Single-org app: the process owns its ports, so the IMAP /
//     SMTP-submission / inbound-SMTP listeners bind fixed TCP ports here.
//   - Multi-org tenant: a tenant must NEVER bind a port — the router owns
//     every listening socket. The router terminates TLS on :993/:465 (SNI
//     demux) and fronts :25 (RCPT TO routing), forwarding plaintext over
//     per-org unix sockets injected through the TenantContext
//     (tenant_listeners.go serves exactly those; none injected = no
//     listeners, e.g. a degraded router).
func Register(app *pocketbase.PocketBase) {
	registerShared(app)
	if tc, ok := coreserver.GetTenantContext(app); ok {
		registerInjectedListeners(app, TenantListeners{
			IMAP:       tc.Mail.IMAP,
			Submission: tc.Mail.Submission,
			InboundMX:  tc.Mail.InboundMX,
		})
		return
	}
	registerMailListeners(app)
}

// registerShared is the single source of truth for what BOTH compositions run.
func registerShared(app *pocketbase.PocketBase) {
	// Storage ceilings: message bodies are real disk. No owner field —
	// a mailbox is shared by its members, so a message is not chargeable to
	// any one of them — which means these bytes count toward the ORG total
	// only, never a per-user one. core/quota binds the enforcement.
	// Mail's contribution to the federated GET /api/search.
	search.RegisterSources(searchSource())

	quota.RegisterSources(quota.Source{
		Slug:       "mail",
		Collection: "mail_messages",
		SizeField:  "total_size",
	})

	// Audit logging for mail collections
	audit.RegisterCollection(app, "mail_domains", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("domain"),
	})

	audit.RegisterCollection(app, "mail_mailboxes", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("address"),
	})

	audit.RegisterCollection(app, "mail_mailbox_members", &audit.CollectionConfig{})

	// A shared mailbox must never lose its last owner (self-demotion or row
	// delete) — an ownerless mailbox is unmanageable. Server backstop for the
	// drawer's client-side checks.
	registerMailboxLastOwnerGuard(app)

	audit.RegisterCollection(app, "mail_messages", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("subject"),
	})

	audit.RegisterCollection(app, "mail_thread_state", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("folder"),
	})

	// Single-org: the provider is deployment-wide (system_settings), so every
	// webhook secret resolves to the same adapter. We still look the secret up
	// to reject unknown ones — an unrecognized secret gets a Noop provider so a
	// forged webhook can't reach a live adapter.
	resolveWebhookProvider := func(secret string) Provider {
		records, err := app.FindRecordsByFilter(
			"mail_domains",
			"webhook_secret = {:secret}",
			"",
			1,
			0,
			map[string]any{"secret": secret},
		)
		if err != nil || len(records) == 0 {
			return &NoopProvider{}
		}
		return newProviderFromSystem(app)
	}

	// A settings change to the mail app may toggle provider or inbound mode —
	// reconcile the IMAP fetcher. (The former per-org settings cache is gone;
	// provider config is deployment-wide in system_settings.)
	reconcileOnMailSettings := func(e *core.RecordEvent) error {
		if e.Record.GetString("app") == "mail" && globalIMAPManager != nil {
			globalIMAPManager.onSettingsChanged()
		}
		return e.Next()
	}

	// Auto-create a personal mailbox when a user is created. Single-org: the
	// process IS the org, so membership is the users record itself (the former
	// user_org junction is gone).
	app.OnRecordAfterCreateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		handleUserCreated(app, e.Record)
		return e.Next()
	})

	// Clean up orphaned personal mailboxes when a user is deleted.
	app.OnRecordAfterDeleteSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		handleUserDeleted(app, e.Record)
		return e.Next()
	})

	// Auto-generate webhook_secret for new domains
	app.OnRecordCreate("mail_domains").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("webhook_secret") == "" {
			secret, err := randomHex(16)
			if err != nil {
				return fmt.Errorf("failed to generate webhook secret: %w", err)
			}
			e.Record.Set("webhook_secret", secret)
		}
		return e.Next()
	})

	app.OnRecordAfterCreateSuccess("settings").BindFunc(reconcileOnMailSettings)
	app.OnRecordAfterUpdateSuccess("settings").BindFunc(reconcileOnMailSettings)
	app.OnRecordAfterDeleteSuccess("settings").BindFunc(reconcileOnMailSettings)

	// The mail provider + IMAP config are SYSTEM-WIDE (system_settings), so a
	// system-settings change to a mail.* key may toggle the IMAP fetcher on/off.
	// Reconcile on those changes too (filtered so sentry.*/vapid.* edits don't
	// churn the fetcher).
	reconcileOnSystemMail := func(e *core.RecordEvent) error {
		if globalIMAPManager != nil && strings.HasPrefix(e.Record.GetString("key"), "mail.") {
			globalIMAPManager.onSettingsChanged()
		}
		return e.Next()
	}
	app.OnRecordAfterCreateSuccess("system_settings").BindFunc(reconcileOnSystemMail)
	app.OnRecordAfterUpdateSuccess("system_settings").BindFunc(reconcileOnSystemMail)

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
	// On update: re-index from storage (e.g. draft edits via
	// updateDraftRecord). The recentlyIndexed sentinel lets follow-up
	// saves that don't change indexed content (cid_map after inbound)
	// skip a duplicate reindex.
	app.OnRecordAfterUpdateSuccess("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		if _, ok := recentlyIndexed.LoadAndDelete(e.Record.Id); ok {
			return e.Next()
		}
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
			globalNotifier.Notify(thread.GetString("mailbox"))
		}
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess("mail_thread_state").BindFunc(func(e *core.RecordEvent) error {
		threadID := e.Record.GetString("thread")
		thread, err := app.FindRecordById("mail_threads", threadID)
		if err == nil {
			globalNotifier.Notify(thread.GetString("mailbox"))
		}
		return e.Next()
	})

	// Buffer inbound messages for batched notification delivery
	app.OnRecordAfterCreateSuccess("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		go bufferMailNotification(app, e.Record)
		return e.Next()
	})

	// Generate thumbnails for any supported attachments (PDF, Office docs, HEIC).
	// AfterUpdate fires from our own Save below, so the hook short-circuits when
	// the attachments list hasn't changed since the last time we ran.
	app.OnRecordAfterCreateSuccess("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		go generateAttachmentThumbnails(app, e.Record)
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		if !attachmentsChanged(e.Record) {
			return e.Next()
		}
		go generateAttachmentThumbnails(app, e.Record)
		return e.Next()
	})

	registerAliasHooks(app)
	registerThreadMarkerHooks(app)

	audit.RegisterCollection(app, "mail_mailbox_aliases", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("address"),
	})

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Send endpoint (requires auth, resolves provider from org settings)
		e.Router.POST("/api/mail/send", func(re *core.RequestEvent) error {
			return handleSend(app, re)
		}).BindFunc(requireAuth)

		// Domain verification endpoint (requires auth; handler checks org admin/owner)
		e.Router.POST("/api/mail/domains/{id}/verify", func(re *core.RequestEvent) error {
			return handleVerifyDomain(app, re)
		}).BindFunc(requireAuth)

		reverifyCtx, cancelReverify := context.WithCancel(context.Background())
		app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
			cancelReverify()
			return te.Next()
		})
		go startDomainReverifyLoop(reverifyCtx, app)

		// IMAP fetcher manager — runs one polling worker per org with the
		// self-hosted SMTP provider in "imap" inbound mode. Reconciles on
		// every mail settings record change via globalIMAPManager.
		imapFetcherShutdown := startIMAPFetchers(app)
		app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
			imapFetcherShutdown()
			return te.Next()
		})

		// Draft endpoint (requires auth, saves without sending)
		e.Router.POST("/api/mail/draft", func(re *core.RequestEvent) error {
			return handleDraft(app, re)
		}).BindFunc(requireAuth)

		// Search endpoint (requires auth)
		e.Router.GET("/api/mail/search", func(re *core.RequestEvent) error {
			return handleSearch(app, re)
		}).BindFunc(requireAuth)

		// Inbound webhook (unauthenticated, secured via per-domain secret)
		e.Router.POST("/api/mail/inbound/{token}", func(re *core.RequestEvent) error {
			secret := re.Request.PathValue("token")
			if secret == "" || !isValidDomainWebhookSecret(app, secret) {
				return re.UnauthorizedError("Invalid inbound token", nil)
			}
			return handleInbound(app, resolveWebhookProvider(secret), re, secret)
		})

		// Bounce webhook (unauthenticated, secured via per-domain secret)
		e.Router.POST("/api/mail/bounces/{token}", func(re *core.RequestEvent) error {
			secret := re.Request.PathValue("token")
			if secret == "" || !isValidDomainWebhookSecret(app, secret) {
				return re.ForbiddenError("Invalid token", nil)
			}
			return handleBounce(app, resolveWebhookProvider(secret), re, secret)
		})

		// Webhook URLs endpoint (requires auth; handler checks admin/owner
		// since the URLs embed the domain's webhook secret)
		e.Router.GET("/api/mail/domains/{id}/webhook-urls", func(re *core.RequestEvent) error {
			domainID := re.Request.PathValue("id")
			domain, err := app.FindRecordById("mail_domains", domainID)
			if err != nil {
				return re.NotFoundError("Domain not found", nil)
			}
			if err := verifyAdmin(re.Auth); err != nil {
				return re.ForbiddenError("only admins or owners can view webhook URLs", err)
			}
			secret := domain.GetString("webhook_secret")
			baseURL := app.Settings().Meta.AppURL
			return re.JSON(http.StatusOK, api.WebhookURLsResponse{
				Inbound: fmt.Sprintf("%s/api/mail/inbound/%s", baseURL, secret),
				Bounces: fmt.Sprintf("%s/api/mail/bounces/%s", baseURL, secret),
			})
		}).BindFunc(requireAuth)

		// Image proxy (auth via query token, since sandboxed iframes can't send headers)
		e.Router.GET("/api/mail/image-proxy", func(re *core.RequestEvent) error {
			return handleImageProxyRequest(app, re)
		})

		// Mail notification batcher: drains the per-user buffer every 2min
		// and dispatches one batched notification per user.
		go startMailBatcher(app)

		return e.Next()
	})
}

// registerMailListeners starts the port-binding mail protocol servers on
// OnServe. Host-only: see the tail of Register for why a tenant must not run
// these.
func registerMailListeners(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// In production a failed mail listener is a deploy-breaking
		// misconfiguration (missing/unreadable cert, lost privileged-port
		// capability, port already bound): abort the boot so it's loud rather
		// than coming up healthy on HTTP with mail silently absent. In dev we
		// log and continue, so a missing local cert doesn't block the app.
		failLoud := !app.IsDev()

		// Start IMAP server
		imapShutdown, err := StartIMAPServer(app, e.CertManager)
		if err != nil {
			app.Logger().Error("Failed to start IMAP server", "error", err)
			if failLoud {
				return fmt.Errorf("aborting startup: IMAP server failed to start: %w", err)
			}
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
			if failLoud {
				return fmt.Errorf("aborting startup: SMTP server failed to start: %w", smtpErr)
			}
		} else {
			app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
				smtpShutdown()
				return te.Next()
			})
		}

		// Start inbound SMTP listener (no-op unless MAIL_INBOUND_SMTP_ENABLED=true).
		// This is the public MX target for the self-hosted SMTP provider in
		// "smtp" inbound mode — distinct from the submission server above.
		inboundShutdown, inboundErr := StartSMTPInboundServer(app, e.CertManager)
		if inboundErr != nil {
			app.Logger().Error("Failed to start inbound SMTP server", "error", inboundErr)
			if failLoud {
				return fmt.Errorf("aborting startup: inbound SMTP server failed to start: %w", inboundErr)
			}
		} else {
			app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
				inboundShutdown()
				return te.Next()
			})
		}

		return e.Next()
	})
}

// systemSetting reads a value from the system_settings collection — the
// system-wide config store core owns. Mail reads it directly from the app (not
// via a core import) to stay decoupled, mirroring the per-org `settings` reads.
// Returns "" when the key is unset (or the collection is absent).
func systemSetting(app core.App, key string) string {
	rec, err := app.FindFirstRecordByFilter("system_settings", "key = {:key}", map[string]any{"key": key})
	if err != nil {
		return ""
	}
	return rec.GetString("value")
}

// newProviderFromSystem builds the provider from system settings. The provider
// choice and its credentials/SMTP config are deployment-wide infrastructure,
// configured in the /admin Settings console.
func newProviderFromSystem(app core.App) Provider {
	name := systemSetting(app, "mail.provider")
	if name == "" {
		name = "postmark"
	}
	return newProviderByName(
		name,
		systemSetting(app, "mail.postmark_server_token"),
		systemSetting(app, "mail.postmark_account_token"),
		smtpConfigFromSystem(app),
	)
}

func newProviderByName(name, serverToken, accountToken string, smtpCfg SMTPConfig) Provider {
	switch name {
	case "postmark":
		// Return a PostmarkProvider even without a server token: ParseInbound
		// and VerifyWebhookSignature don't need it (Postmark uses URL-based
		// auth on the inbound webhook, not signed payloads), so test/dev
		// scenarios that exercise the inbound flow without real Postmark
		// credentials still work. Callers on the send/verify paths gate on
		// provider.Configured() (false when the token is empty) and reject
		// early, so a missing token surfaces a clear "not configured" error
		// instead of an opaque API failure.
		return NewPostmarkProvider(serverToken, accountToken)
	case "smtp":
		return NewSMTPProvider(smtpCfg)
	default:
		return &NoopProvider{}
	}
}

// smtpConfigFromSystem reads SMTPConfig from system settings (the deployment-wide
// SMTP/IMAP config). Numeric fields that don't parse are left zero (the
// constructor's applyDefaults handles them).
func smtpConfigFromSystem(app core.App) SMTPConfig {
	port, _ := strconv.Atoi(systemSetting(app, "mail.smtp_imap_port"))
	poll, _ := strconv.Atoi(systemSetting(app, "mail.smtp_imap_poll_interval_seconds"))
	cfg := SMTPConfig{
		PublicHostname:   systemSetting(app, "mail.smtp_public_hostname"),
		InboundMode:      systemSetting(app, "mail.smtp_inbound_mode"),
		IMAPHost:         systemSetting(app, "mail.smtp_imap_host"),
		IMAPPort:         port,
		IMAPUsername:     systemSetting(app, "mail.smtp_imap_username"),
		IMAPPassword:     systemSetting(app, "mail.smtp_imap_password"),
		IMAPUseTLS:       systemSetting(app, "mail.smtp_imap_use_tls") != "false",
		IMAPMailbox:      systemSetting(app, "mail.smtp_imap_mailbox"),
		IMAPPollInterval: time.Duration(poll) * time.Second,
		DKIMSelector:     systemSetting(app, "mail.smtp_dkim_selector"),
	}
	return cfg
}

// indexMessageRecordFromStorage builds FTS body_text from the record's stored
// HTML body and text-based attachments. Used by record hooks when storeMessage()
// wasn't involved or on updates.
func indexMessageRecordFromStorage(app core.App, record *core.Record) {
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

func randomHex(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func isValidDomainWebhookSecret(app core.App, secret string) bool {
	records, err := app.FindRecordsByFilter(
		"mail_domains",
		"webhook_secret = {:secret}",
		"",
		1,
		0,
		map[string]any{"secret": secret},
	)
	return err == nil && len(records) > 0
}

// requireAuth is a middleware that ensures the request has a valid auth token.
func requireAuth(re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Authentication required", nil)
	}
	return re.Next()
}

// bufferMailNotification queues a mail notification for batched delivery.
func bufferMailNotification(app core.App, msgRecord *core.Record) {
	if !appIsLive(app) {
		return
	}

	// Skip outbound messages (sent by the user themselves)
	direction := msgRecord.GetString("direction")
	if direction == "outbound" || direction == "sent" {
		return
	}

	threadID := msgRecord.GetString("thread")
	if threadID == "" {
		return
	}

	thread, err := app.FindRecordById("mail_threads", threadID)
	if err != nil {
		return
	}

	mailboxID := thread.GetString("mailbox")
	if mailboxID == "" {
		return
	}

	// Find mailbox members to notify
	members, err := app.FindRecordsByFilter(
		"mail_mailbox_members",
		"mailbox = {:mailboxId}",
		"",
		0,
		0,
		map[string]any{"mailboxId": mailboxID},
	)
	if err != nil || len(members) == 0 {
		return
	}

	senderName := msgRecord.GetString("sender_name")
	senderEmail := msgRecord.GetString("sender_email")
	subject := msgRecord.GetString("subject")
	sender := senderName
	if sender == "" {
		sender = senderEmail
	}

	for _, member := range members {
		if userID := member.GetString("user"); userID != "" {
			bufferMailNotificationForUser(userID, sender, subject)
		}
	}
}
