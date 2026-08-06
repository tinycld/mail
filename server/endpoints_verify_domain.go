package mail

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/packages/mail/api"
)

func handleVerifyDomain(app core.App, re *core.RequestEvent) error {
	domainID := re.Request.PathValue("id")
	if domainID == "" {
		return re.BadRequestError("domain id is required", nil)
	}

	record, err := app.FindRecordById("mail_domains", domainID)
	if err != nil {
		return re.NotFoundError("domain not found", err)
	}

	if err := verifyAdmin(re.Auth); err != nil {
		return re.ForbiddenError("only admins or owners can verify domains", err)
	}

	if !newProviderFromSystem(app).Configured() {
		return re.BadRequestError(
			"configure the mail provider in settings before verifying",
			errProviderNotConfigured,
		)
	}

	details, saveErr := verifyDomainRecord(re.Request.Context(), app, record)

	body := api.VerifyDomainResponse{
		ID:                    record.Id,
		Verified:              record.GetBool("verified"),
		MXVerified:            record.GetBool("mx_verified"),
		InboundDomainVerified: record.GetBool("inbound_domain_verified"),
		SPFVerified:           record.GetBool("spf_verified"),
		DKIMVerified:          record.GetBool("dkim_verified"),
		ReturnPathVerified:    record.GetBool("return_path_verified"),
		LastCheckedAt:         record.GetString("last_checked_at"),
		VerificationDetails:   details,
		Saved:                 saveErr == nil,
	}
	if saveErr != nil {
		app.Logger().Warn("mail: failed to persist domain verification",
			"domain", record.GetString("domain"), "error", saveErr)
		body.SaveError = saveErr.Error()
	}

	return re.JSON(http.StatusOK, body)
}
