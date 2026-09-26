package mail

import (
	"context"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
	"tinycld.org/core/maildomains"
)

var domainRemovalLog = logging.ForPackage("mail")

// domainRemovalTimeout bounds the provider call. The hook is synchronous, so
// a slow provider holds the delete response open for up to this long (or the
// registrar's own shorter timeout); the delete itself has already committed.
const domainRemovalTimeout = 30 * time.Second

// registerMailDomainRemovalHook releases a deleted domain at the provider
// through the maildomains seam. Without it a deleted domain stays enrolled
// in the provider account, still able to send and still counted there.
//
// It runs only after the delete has committed and never returns an error:
// the org's choice to delete the domain stands even when the provider is
// down. Nothing retries a failed removal: the domain stays enrolled at the
// provider, and held by whatever registered it there, until the owner adds
// the domain again and deletes it again.
func registerMailDomainRemovalHook(app core.App) {
	app.OnRecordAfterDeleteSuccess("mail_domains").BindFunc(func(e *core.RecordEvent) error {
		releaseDeletedDomain(e.Record)
		return e.Next()
	})
}

func releaseDeletedDomain(record *core.Record) {
	remover, ok := maildomains.Current().(maildomains.Remover)
	if !ok {
		return
	}
	domain := record.GetString("domain")
	var providerDomainID int64
	if pm := readProviderDomainMetadata(record).Postmark; pm != nil {
		providerDomainID = pm.DomainID
	}
	// A background context: the request that deleted the row may already be
	// finished, and the removal must not be cut short with it.
	ctx, cancel := context.WithTimeout(context.Background(), domainRemovalTimeout)
	defer cancel()
	if err := remover.RemoveDomain(ctx, domain, providerDomainID); err != nil {
		domainRemovalLog.Warn("could not release a deleted domain at the mail provider",
			"domain", domain, "providerDomainID", providerDomainID, "error", err)
	}
}
