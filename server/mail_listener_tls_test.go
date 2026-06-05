package mail

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
)

// newProdApp builds a non-dev (production-mode) *pocketbase.PocketBase without
// bootstrapping the database. StartIMAPServer/StartSMTPServer consult IsDev()
// and Logger() before they ever touch the DB, so the no-TLS error path these
// tests exercise needs no data dir bootstrap.
func newProdApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()
	return pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDev:      false, // production mode — the case we guard
		DefaultDataDir:  t.TempDir(),
		HideStartBanner: true,
	})
}

// In production with no TLS source (no cert env vars, no autocert manager),
// the listeners must refuse to start rather than silently fall through to the
// plain-text dev ports (:1143/:1587). A returned error here is what aborts the
// deploy loudly instead of coming up healthy on HTTP with mail absent.
func TestStartIMAPServer_ProductionNoTLS_Errors(t *testing.T) {
	t.Setenv("IMAP_TLS_CERT", "")
	t.Setenv("IMAP_TLS_KEY", "")

	_, err := StartIMAPServer(newProdApp(t), nil)
	if err == nil {
		t.Fatal("expected StartIMAPServer to error in production without TLS, got nil")
	}
	if !strings.Contains(err.Error(), ":993") {
		t.Errorf("error should name the IMAPS port :993, got: %v", err)
	}
}

func TestStartSMTPServer_ProductionNoTLS_Errors(t *testing.T) {
	t.Setenv("SMTP_TLS_CERT", "")
	t.Setenv("SMTP_TLS_KEY", "")
	t.Setenv("IMAP_TLS_CERT", "") // SMTP falls back to the IMAP pair
	t.Setenv("IMAP_TLS_KEY", "")

	_, err := StartSMTPServer(newProdApp(t), nil)
	if err == nil {
		t.Fatal("expected StartSMTPServer to error in production without TLS, got nil")
	}
	if !strings.Contains(err.Error(), ":465") {
		t.Errorf("error should name the SMTPS port :465, got: %v", err)
	}
}

// The explicit opt-out (IMAP_ENABLED/SMTP_ENABLED=false) must still short-
// circuit cleanly in production — disabling a listener is not a failure.
func TestStartIMAPServer_ProductionDisabled_NoError(t *testing.T) {
	t.Setenv("IMAP_ENABLED", "false")

	shutdown, err := StartIMAPServer(newProdApp(t), nil)
	if err != nil {
		t.Fatalf("IMAP_ENABLED=false should not error, got: %v", err)
	}
	shutdown()
}

func TestStartSMTPServer_ProductionDisabled_NoError(t *testing.T) {
	t.Setenv("SMTP_ENABLED", "false")

	shutdown, err := StartSMTPServer(newProdApp(t), nil)
	if err != nil {
		t.Fatalf("SMTP_ENABLED=false should not error, got: %v", err)
	}
	shutdown()
}
