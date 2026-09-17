package mail

import (
	"context"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/maildomains"
)

// These tests pin the CALL SITES of reconcileMailDomainsRegistrar, not its
// body. settings_test.go's TestReconcileMailDomainsRegistrar_SwitchesProviderType
// calls the function directly, so deleting every call to it from register.go
// leaves that test green while the entire runtime wiring is gone: a
// deployment that switched provider — or simply booted — would keep whatever
// registrar happened to be installed, which on a switch to SMTP means every
// domain reported "enrolled" from pure DNS lookups.
//
// Modelled on hosting/cmd/serve-router/hooks_test.go, which exists for the
// same reason one level up: exercise the real production symbol so removing
// the call fails here.
//
// The observable signal is maildomains.Current(): a sentinel registrar is
// installed first, the real hook is then fired, and Current() must have been
// replaced by the registrar matching the configured provider (*SMTPRegistrar).
// A removed call leaves the sentinel in place.

// sentinelRegistrar is a Registrar that must never be reached — its only job
// is to be observably still installed when a reconcile call site was deleted.
type sentinelRegistrar struct{}

func (sentinelRegistrar) AddDomain(context.Context, string) (*maildomains.DomainRecords, error) {
	return nil, maildomains.ErrNotConfigured
}

func (sentinelRegistrar) GetDomain(context.Context, string, int64) (*maildomains.DomainRecords, error) {
	return nil, maildomains.ErrNotConfigured
}

// setupReconcileWiringApp builds an app with the collections register.go binds
// hooks to (settings, system_settings) plus the mail collections its other
// hooks resolve, with mail.provider set to smtp so a reconcile that actually
// runs installs an *SMTPRegistrar.
func setupReconcileWiringApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := setupSettingsTestApp(t)
	saveSystemSetting(t, app, "mail.provider", "smtp")
	saveSystemSetting(t, app, "mail.smtp_public_hostname", "mx.operator.example")
	return app
}

// installSentinel resets the seam, installs the sentinel, and registers the
// cleanup. Returns nothing: the assertion is always on maildomains.Current().
func installSentinel(t *testing.T) {
	t.Helper()
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(sentinelRegistrar{})
	if _, ok := maildomains.Current().(sentinelRegistrar); !ok {
		t.Fatalf("precondition: Current() = %T, want the sentinel", maildomains.Current())
	}
}

// Register's OnServe hook must reconcile the registrar at boot. Without it a
// deployment configured for SMTP comes up with core's Postmark registrar (or
// nothing) installed, and stays that way until a settings row happens to be
// written.
func TestRegister_OnServeReconcilesMailDomainsRegistrarAtBoot(t *testing.T) {
	t.Setenv("IMAP_ENABLED", "false")
	t.Setenv("SMTP_ENABLED", "false")

	app := setupReconcileWiringApp(t)
	Register(&pocketbase.PocketBase{App: app})

	installSentinel(t)

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	if err := app.OnServe().Trigger(serveEvent, func(*core.ServeEvent) error { return nil }); err != nil {
		t.Fatalf("OnServe trigger: %v", err)
	}

	if _, ok := maildomains.Current().(*SMTPRegistrar); !ok {
		t.Fatalf("after boot: Current() = %T, want *SMTPRegistrar — "+
			"Register's OnServe hook did not reconcile the maildomains registrar",
			maildomains.Current())
	}
}

// A `settings` row for the mail app changing must reconcile. This is the hook
// an admin toggling mail config in-app fires.
func TestRegister_MailSettingsChangeReconcilesMailDomainsRegistrar(t *testing.T) {
	app := setupReconcileWiringApp(t)
	Register(&pocketbase.PocketBase{App: app})

	installSentinel(t)

	col, err := app.FindCollectionByNameOrId("settings")
	if err != nil {
		t.Fatalf("settings collection missing: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("app", "mail")
	rec.Set("key", "anything")
	rec.Set("value", map[string]any{"x": 1})
	if err := app.Save(rec); err != nil {
		t.Fatalf("save settings record: %v", err)
	}

	if _, ok := maildomains.Current().(*SMTPRegistrar); !ok {
		t.Fatalf("after a mail settings change: Current() = %T, want *SMTPRegistrar — "+
			"the settings-change hook did not reconcile the maildomains registrar",
			maildomains.Current())
	}
}

// A `system_settings` row under the mail.* prefix changing must reconcile.
// This is the hook an operator switching provider in /admin fires, and is the
// path the SMTP↔Postmark switch actually travels.
func TestRegister_SystemMailSettingChangeReconcilesMailDomainsRegistrar(t *testing.T) {
	app := setupReconcileWiringApp(t)
	Register(&pocketbase.PocketBase{App: app})

	installSentinel(t)

	saveSystemSetting(t, app, "mail.smtp_public_hostname", "mx2.operator.example")

	if _, ok := maildomains.Current().(*SMTPRegistrar); !ok {
		t.Fatalf("after a mail.* system_settings change: Current() = %T, want *SMTPRegistrar — "+
			"the system-settings hook did not reconcile the maildomains registrar",
			maildomains.Current())
	}
}

// The prefix filter is load-bearing: an unrelated system setting must NOT
// churn the registrar. Without this the two tests above would also pass a
// register.go that reconciled on every record change anywhere.
func TestRegister_UnrelatedSystemSettingDoesNotReconcile(t *testing.T) {
	app := setupReconcileWiringApp(t)
	Register(&pocketbase.PocketBase{App: app})

	installSentinel(t)

	saveSystemSetting(t, app, "sentry.dsn", "https://example.invalid/1")

	if _, ok := maildomains.Current().(sentinelRegistrar); !ok {
		t.Fatalf("after an unrelated system setting: Current() = %T, want the sentinel "+
			"(a non-mail.* key must not churn the registrar)", maildomains.Current())
	}
}
