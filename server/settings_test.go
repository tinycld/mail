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
	return app
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

// providerForOrg should hand back a configured Postmark provider built from
// the org's stored server token (no env fallback in play). A regression in
// getOrgSettings would drop the token, leaving the provider unconfigured —
// which the send/verify paths now reject early — so assert Configured().
func TestProviderForOrg_UsesOrgPostmarkToken(t *testing.T) {
	t.Setenv("POSTMARK_SERVER_TOKEN", "")
	t.Setenv("MAIL_PROVIDER", "")

	app := setupSettingsTestApp(t)
	const orgID = "org222222222222"
	saveSetting(t, app, "mail", "provider", "postmark", orgID)
	saveSetting(t, app, "mail", "postmark_server_token", "tok-org-only", orgID)

	provider := providerForOrg(app, orgID)
	if _, ok := provider.(*PostmarkProvider); !ok {
		t.Fatalf("expected *PostmarkProvider from org settings, got %T", provider)
	}
	if !provider.Configured() {
		t.Error("provider built from org token should report Configured() == true")
	}
}

// With no org token and no env fallback, the provider is a PostmarkProvider but
// reports Configured() == false — the signal the send/verify endpoints use to
// reject with a clear "not configured" error instead of an opaque API failure.
func TestProviderForOrg_NoToken_NotConfigured(t *testing.T) {
	t.Setenv("POSTMARK_SERVER_TOKEN", "")
	t.Setenv("POSTMARK_ACCOUNT_TOKEN", "")
	t.Setenv("MAIL_PROVIDER", "")

	app := setupSettingsTestApp(t)
	const orgID = "org444444444444"
	// provider configured as postmark, but no token saved
	saveSetting(t, app, "mail", "provider", "postmark", orgID)

	provider := providerForOrg(app, orgID)
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
