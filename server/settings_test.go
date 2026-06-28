package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupSettingsTestApp builds a test app with a `settings` collection whose
// `value` field is a json type — matching the production schema
// (core pb_migrations: settings_value is type:'json'). This is what makes the
// regression real: app.Get("value") returns a types.JSONRaw, not a string, so
// the value can only be read via marshal/unmarshal.
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
	settings.Fields.Add(&core.TextField{Name: "org"})
	if err := app.Save(settings); err != nil {
		t.Fatalf("failed to save settings collection: %v", err)
	}

	// system_settings is system-wide (no org); mail reads provider/creds from it.
	// `value` is a plain text field here, matching the core migration (systemSetting
	// reads it via GetString).
	sys := core.NewBaseCollection("system_settings")
	sys.Fields.Add(&core.TextField{Name: "key", Required: true})
	sys.Fields.Add(&core.TextField{Name: "value"})
	sys.Fields.Add(&core.BoolField{Name: "is_secret"})
	if err := app.Save(sys); err != nil {
		t.Fatalf("failed to save system_settings collection: %v", err)
	}
	return app
}

func saveSystemSetting(t *testing.T, app *tests.TestApp, key, value string) {
	t.Helper()
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

func saveSetting(t *testing.T, app *tests.TestApp, appName, key, value, orgID string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("settings")
	if err != nil {
		t.Fatalf("settings collection missing: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("app", appName)
	rec.Set("key", key)
	rec.Set("value", value)
	rec.Set("org", orgID)
	if err := app.Save(rec); err != nil {
		t.Fatalf("failed to save setting %s: %v", key, err)
	}
}

// Regression: settings.value is a json field, so a string value round-trips as
// types.JSONRaw. getOrgSettings must decode it; the old code asserted on
// string/json.RawMessage and silently dropped every value, which fell back to
// env-var provider credentials (wrong Postmark server).
func TestGetOrgSettings_ReadsJSONStringValues(t *testing.T) {
	app := setupSettingsTestApp(t)
	const orgID = "org123456789012"

	saveSetting(t, app, "mail", "provider", "postmark", orgID)
	saveSetting(t, app, "mail", "postmark_server_token", "tok-abc-123", orgID)

	got := getOrgSettings(app, "mail", orgID)

	if got["postmark_server_token"] != "tok-abc-123" {
		t.Errorf("postmark_server_token = %q, want %q", got["postmark_server_token"], "tok-abc-123")
	}
	if got["provider"] != "postmark" {
		t.Errorf("provider = %q, want %q", got["provider"], "postmark")
	}
}

// Provider + credentials are SYSTEM-WIDE (system_settings), not per-org. A token
// stored in system_settings yields a configured Postmark provider; the orgID no
// longer affects selection. (An org-stored 'provider'/token is ignored.)
func TestProviderForOrg_UsesSystemPostmarkToken(t *testing.T) {
	app := setupSettingsTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "postmark")
	saveSystemSetting(t, app, "mail.postmark_server_token", "tok-system")

	provider := providerForOrg(app, "org222222222222")
	if _, ok := provider.(*PostmarkProvider); !ok {
		t.Fatalf("expected *PostmarkProvider from system settings, got %T", provider)
	}
	if !provider.Configured() {
		t.Error("provider built from the system token should report Configured() == true")
	}
}

// Provider/creds are system-wide: a token set only in an ORG's settings must NOT
// configure the provider (org settings no longer carry provider credentials).
func TestProviderForOrg_IgnoresOrgPostmarkToken(t *testing.T) {
	app := setupSettingsTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "postmark")
	// Org-level provider/token (legacy shape) — must be ignored now.
	saveSetting(t, app, "mail", "postmark_server_token", "tok-org-only", "org222222222222")

	provider := providerForOrg(app, "org222222222222")
	if provider.Configured() {
		t.Error("an org-stored token must NOT configure the provider (system-wide only)")
	}
}

// With no system token, the provider is a PostmarkProvider but reports
// Configured() == false — the signal send/verify use to reject with a clear
// "not configured" error instead of an opaque API failure.
func TestProviderForOrg_NoToken_NotConfigured(t *testing.T) {
	app := setupSettingsTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "postmark")

	provider := providerForOrg(app, "org444444444444")
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

// Settings are cached per org+app. Updating a setting must invalidate that
// cache entry, or stale (or empty) values survive until restart. This mirrors
// the invalidateSettingsCache hook's contract: it deletes "orgID:appName".
func TestSettingsCacheInvalidation_DeletesCompositeKey(t *testing.T) {
	app := setupSettingsTestApp(t)
	const orgID = "org333333333333"
	saveSetting(t, app, "mail", "postmark_server_token", "tok-old", orgID)

	// Prime the cache.
	if got := getOrgSettings(app, "mail", orgID)["postmark_server_token"]; got != "tok-old" {
		t.Fatalf("priming read = %q, want %q", got, "tok-old")
	}

	// Simulate the invalidation hook deleting the composite key, then change
	// the stored value and confirm the fresh value is read.
	settingsCache.Delete(orgID + ":mail")

	rec, err := app.FindFirstRecordByFilter("settings",
		"app = 'mail' && key = 'postmark_server_token' && org = {:org}",
		map[string]any{"org": orgID})
	if err != nil {
		t.Fatalf("find setting: %v", err)
	}
	rec.Set("value", "tok-new")
	if err := app.Save(rec); err != nil {
		t.Fatalf("update setting: %v", err)
	}

	if got := getOrgSettings(app, "mail", orgID)["postmark_server_token"]; got != "tok-new" {
		t.Errorf("after invalidation, postmark_server_token = %q, want %q", got, "tok-new")
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
