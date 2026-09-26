package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/maildomains"
	"tinycld.org/core/syscfg"
)

// setupSettingsTestApp builds a test app with the `settings` and
// `system_settings` collections. Mail's provider config is deployment-wide and
// lives in system_settings; the per-app `settings` collection is retained here
// because mail still binds record hooks to it (the IMAP-fetcher reconcile).
func setupSettingsTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	settings := core.NewBaseCollection("settings")
	settings.Fields.Add(&core.TextField{Name: "app", Required: true})
	settings.Fields.Add(&core.TextField{Name: "key", Required: true})
	settings.Fields.Add(&core.JSONField{Name: "value", MaxSize: 1 << 20})
	if err := app.Save(settings); err != nil {
		t.Fatalf("failed to save settings collection: %v", err)
	}

	// system_settings holds the deployment-wide provider/credentials mail reads.
	// `value` is a plain text field here, matching the core migration (systemSetting
	// reads it via GetString).
	sys := core.NewBaseCollection("system_settings")
	sys.Fields.Add(&core.TextField{Name: "key", Required: true})
	sys.Fields.Add(&core.TextField{Name: "value"})
	sys.Fields.Add(&core.BoolField{Name: "is_secret"})
	if err := app.Save(sys); err != nil {
		t.Fatalf("failed to save system_settings collection: %v", err)
	}

	// Mail reads deployment-wide config through core's syscfg seam, not by
	// querying the collection. In the app this is wired by coreserver; here,
	// point it at this test app's rows so saveSystemSetting still drives what
	// the code under test reads.
	syscfg.SetResolver(func(key string) string {
		rec, err := app.FindFirstRecordByFilter(
			"system_settings", "key = {:key}", map[string]any{"key": key})
		if err != nil {
			return ""
		}
		return rec.GetString("value")
	})
	t.Cleanup(func() { syscfg.SetResolver(func(string) string { return "" }) })

	return app
}

func saveSystemSetting(t *testing.T, app *tests.TestApp, key, value string) {
	t.Helper()
	if rec, err := app.FindFirstRecordByFilter(
		"system_settings", "key = {:key}", map[string]any{"key": key}); err == nil {
		rec.Set("value", value)
		if err := app.Save(rec); err != nil {
			t.Fatalf("failed to update system setting %s: %v", key, err)
		}
		return
	}
	col, err := app.FindCollectionByNameOrId("system_settings")
	if err != nil {
		t.Fatalf("system_settings collection missing: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("key", key)
	rec.Set("value", value)
	if err := app.Save(rec); err != nil {
		t.Fatalf("failed to save system setting %s: %v", key, err)
	}
}

// Provider + credentials are deployment-wide (system_settings). A token stored
// there yields a configured Postmark provider.
func TestNewProviderFromSystem_UsesSystemPostmarkToken(t *testing.T) {
	app := setupSettingsTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "postmark")
	saveSystemSetting(t, app, "mail.postmark_server_token", "tok-system")

	provider := newProviderFromSystem(app)
	if _, ok := provider.(*PostmarkProvider); !ok {
		t.Fatalf("expected *PostmarkProvider from system settings, got %T", provider)
	}
	if !provider.Configured() {
		t.Error("provider built from the system token should report Configured() == true")
	}
}

// With no system token, the provider is a PostmarkProvider but reports
// Configured() == false — the signal send/verify use to reject with a clear
// "not configured" error instead of an opaque API failure.
func TestNewProviderFromSystem_NoToken_NotConfigured(t *testing.T) {
	app := setupSettingsTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "postmark")

	provider := newProviderFromSystem(app)
	if provider.Configured() {
		t.Error("provider with no server token should report Configured() == false")
	}
}

// newProviderByName returns a usable (inbound-parsing) PostmarkProvider even
// without a token, but it must report Configured() == false so credentialed
// paths gate on it.
func TestNewProviderByName_PostmarkConfiguredReflectsToken(t *testing.T) {
	if got := newProviderByName("postmark", "", "", SMTPConfig{}).Configured(); got {
		t.Error("postmark provider with empty token: Configured() = true, want false")
	}
	if got := newProviderByName("postmark", "tok", "", SMTPConfig{}).Configured(); !got {
		t.Error("postmark provider with token: Configured() = false, want true")
	}
	if got := newProviderByName("unknown", "tok", "", SMTPConfig{}).Configured(); got {
		t.Error("noop provider: Configured() = true, want false")
	}
	if got := newProviderByName("smtp", "", "", SMTPConfig{}).Configured(); !got {
		t.Error("smtp provider: Configured() = false, want true (smtp needs no credentials)")
	}
}

// Provider config is read fresh from system_settings on every call (there is no
// cache), so an operator edit in /admin applies without a restart. This is the
// contract the deleted per-org settings cache used to complicate.
func TestNewProviderFromSystem_ReflectsLiveEdits(t *testing.T) {
	app := setupSettingsTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "postmark")

	if newProviderFromSystem(app).Configured() {
		t.Fatal("provider should start unconfigured with no token")
	}

	saveSystemSetting(t, app, "mail.postmark_server_token", "tok-new")

	if !newProviderFromSystem(app).Configured() {
		t.Error("provider should pick up a newly-saved token without a restart")
	}
}

// The IMAP fetcher decides whether to run from the SYSTEM provider config (it no
// longer queries org settings — the regression that left the fetcher never
// starting). With system settings set to self-hosted SMTP in imap inbound mode
// and a host, smtpConfigFromSystem must yield a config the fetcher treats as
// "want" (InboundMode == "imap" && IMAPHost != ""). org settings are irrelevant.
func TestSmtpConfigFromSystem_DrivesImapFetcher(t *testing.T) {
	app := setupSettingsTestApp(t)

	// Not configured for imap → fetcher should NOT want to run.
	if cfg := smtpConfigFromSystem(app); cfg.InboundMode == "imap" && cfg.IMAPHost != "" {
		t.Fatal("empty system config should not request an imap fetcher")
	}

	saveSystemSetting(t, app, "mail.provider", "smtp")
	saveSystemSetting(t, app, "mail.smtp_inbound_mode", "imap")
	saveSystemSetting(t, app, "mail.smtp_imap_host", "imap.example.com")

	cfg := smtpConfigFromSystem(app)
	if cfg.InboundMode != "imap" {
		t.Errorf("InboundMode = %q, want imap", cfg.InboundMode)
	}
	if cfg.IMAPHost != "imap.example.com" {
		t.Errorf("IMAPHost = %q, want imap.example.com", cfg.IMAPHost)
	}
}

// Switching mail.provider away from SMTP must swap out the maildomains
// registrar, not leave SMTPRegistrar installed. SetResolver is
// last-writer-wins with no "uninstall", so before this fix
// reconcileMailDomainsRegistrar only ever wrote on the smtp branch — a
// deployment moved from SMTP to Postmark kept reporting every domain
// "enrolled" from pure DNS lookups forever, including across restarts (I1
// from the final whole-branch review).
func TestReconcileMailDomainsRegistrar_SwitchesProviderType(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)

	app := setupSettingsTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "smtp")
	saveSystemSetting(t, app, "mail.smtp_host", "smtp.example.com")

	reconcileMailDomainsRegistrar(app)
	if _, ok := maildomains.Current().(*SMTPRegistrar); !ok {
		t.Fatalf("after smtp: Current() = %T, want *SMTPRegistrar", maildomains.Current())
	}

	saveSystemSetting(t, app, "mail.provider", "postmark")
	saveSystemSetting(t, app, "mail.postmark_account_token", "acct-token")

	reconcileMailDomainsRegistrar(app)
	if _, ok := maildomains.Current().(*maildomains.PostmarkRegistrar); !ok {
		t.Fatalf("after switch to postmark: Current() = %T, want *maildomains.PostmarkRegistrar", maildomains.Current())
	}
}
