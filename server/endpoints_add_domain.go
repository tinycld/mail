package mail

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/maildomains"
	"tinycld.org/packages/mail/api"
)

// handleAddDomain enrolls a domain with the mail provider and then creates the
// mail_domains row.
//
// Enrollment runs FIRST and a failure creates nothing. The previous flow was a
// bare client-side insert that never contacted the provider, which left rows
// the provider had never heard of: they displayed as verified and every send
// from them failed. A row that exists is now a row the provider knows.
//
// On a hosted deployment the enrollment call travels over ctl.sock to the
// router, which holds the account token. This handler cannot tell the
// difference, and must not be able to.
func handleAddDomain(app core.App) func(*core.RequestEvent) error {
	return func(re *core.RequestEvent) error {
		if err := verifyAdmin(re.Auth); err != nil {
			return re.ForbiddenError("only admins or owners can add domains", err)
		}

		var body struct {
			Domain string `json:"domain"`
		}
		if err := re.BindBody(&body); err != nil {
			return re.BadRequestError("Invalid request body", err)
		}
		domain := strings.ToLower(strings.TrimSpace(body.Domain))
		if domain == "" {
			return re.BadRequestError("domain is required", nil)
		}

		rec, err := maildomains.Current().AddDomain(re.Request.Context(), domain)
		switch {
		case errors.Is(err, maildomains.ErrDomainAlreadyEnrolled):
			return router.NewApiError(http.StatusConflict,
				"That domain is already configured on this host.", err)
		case errors.Is(err, maildomains.ErrNotConfigured):
			return router.NewApiError(http.StatusServiceUnavailable,
				"Mail domain provisioning is not configured for this deployment.", err)
		case err != nil:
			// Don't leak raw provider text for the generic case — it may name
			// the provider or its API shape, neither of which an org admin can
			// act on.
			return router.NewApiError(http.StatusBadGateway,
				"Failed to enroll the domain with the mail provider.", err)
		}

		col, err := app.FindCollectionByNameOrId("mail_domains")
		if err != nil {
			return re.InternalServerError("mail_domains collection missing", err)
		}
		record := core.NewRecord(col)
		record.Set("domain", domain)
		// Enrollment succeeded but nothing has been verified yet — MX and the
		// provider's inbound-domain check haven't run, so those stay false
		// until the admin publishes DNS and verifies. The outbound flags below
		// are what the provider itself reported at enrollment time.
		record.Set("verified", false)
		record.Set("mx_verified", false)
		record.Set("inbound_domain_verified", false)
		record.Set("spf_verified", rec.SPFVerified)
		record.Set("dkim_verified", rec.DKIMVerified)
		record.Set("return_path_verified", rec.ReturnPathVerified)
		record.Set("provider_domain_metadata", providerDomainMetadata{
			Postmark: &postmarkDomainMetadata{
				DomainID:           rec.ID,
				CheckedAt:          time.Now().UTC().Format(time.RFC3339Nano),
				Enrolled:           boolPtr(true),
				SPFVerified:        boolPtr(rec.SPFVerified),
				DKIMVerified:       boolPtr(rec.DKIMVerified),
				ReturnPathVerified: boolPtr(rec.ReturnPathVerified),
			},
		})
		if err := app.Save(record); err != nil {
			return re.InternalServerError("Failed to save domain", err)
		}

		return re.JSON(http.StatusOK, api.AddDomainResponse{
			ID:     record.Id,
			Domain: domain,
			Records: &api.OutboundCheckResult{
				Enrolled:             "yes",
				SPF:                  rec.SPFVerified,
				DKIM:                 rec.DKIMVerified,
				ReturnPath:           rec.ReturnPathVerified,
				DKIMHost:             rec.DKIMHost,
				DKIMTextValue:        rec.DKIMTextValue,
				ReturnPathDomain:     rec.ReturnPathDomain,
				ReturnPathCNAMEValue: rec.ReturnPathCNAMEValue,
			},
		})
	}
}
