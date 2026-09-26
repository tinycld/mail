package mail

import (
	"context"
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase"
	"tinycld.org/core/maildomains"
)

// removingRegistrar is a stubRegistrar that also implements
// maildomains.Remover and records each removal it is asked for.
type removingRegistrar struct {
	stubRegistrar
	removeErr error
	removed   []removeCall
}

type removeCall struct {
	domain string
	id     int64
}

func (r *removingRegistrar) RemoveDomain(_ context.Context, domain string, providerDomainID int64) error {
	r.removed = append(r.removed, removeCall{domain: domain, id: providerDomainID})
	return r.removeErr
}

// Goes through the real Register so a removed call site fails here, not only
// a broken hook body.
func TestDeletedDomainIsReleasedWithItsStoredProviderID(t *testing.T) {
	t.Setenv("IMAP_ENABLED", "false")
	t.Setenv("SMTP_ENABLED", "false")
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	registrar := &removingRegistrar{}
	maildomains.SetResolver(registrar)

	app := setupAddDomainTestApp(t)
	Register(&pocketbase.PocketBase{App: app})
	rec := seedGuardDomain(t, app, "acme.com")

	if err := app.Delete(rec); err != nil {
		t.Fatalf("delete: %v", err)
	}

	want := []removeCall{{domain: "acme.com", id: 7}}
	if len(registrar.removed) != 1 || registrar.removed[0] != want[0] {
		t.Fatalf("removed = %+v, want %+v", registrar.removed, want)
	}
}

func TestDeletedDomainWithoutStoredIDIsReleasedByName(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	registrar := &removingRegistrar{}
	maildomains.SetResolver(registrar)

	app := setupAddDomainTestApp(t)
	registerMailDomainRemovalHook(app)
	rec := newOutboundTestRecord(t, app, "legacy.example", 0)

	if err := app.Delete(rec); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if len(registrar.removed) != 1 || registrar.removed[0] != (removeCall{domain: "legacy.example"}) {
		t.Fatalf("removed = %+v, want one by-name removal of legacy.example", registrar.removed)
	}
}

func TestProviderFailureNeverBlocksTheDelete(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	registrar := &removingRegistrar{removeErr: errors.New("provider down")}
	maildomains.SetResolver(registrar)

	app := setupAddDomainTestApp(t)
	registerMailDomainRemovalHook(app)
	rec := seedGuardDomain(t, app, "acme.com")

	if err := app.Delete(rec); err != nil {
		t.Fatalf("delete must succeed when the provider fails, got %v", err)
	}
	if n := countMailDomains(t, app); n != 0 {
		t.Fatalf("rows left = %d, want 0", n)
	}
	if len(registrar.removed) != 1 {
		t.Fatalf("removal attempts = %d, want 1", len(registrar.removed))
	}
}

func TestRegistrarWithoutRemoverIsSkipped(t *testing.T) {
	maildomains.ResetForTesting()
	t.Cleanup(maildomains.ResetForTesting)
	maildomains.SetResolver(&stubRegistrar{})

	app := setupAddDomainTestApp(t)
	registerMailDomainRemovalHook(app)
	rec := seedGuardDomain(t, app, "acme.com")

	if err := app.Delete(rec); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
