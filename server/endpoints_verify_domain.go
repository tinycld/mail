package mail

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func handleVerifyDomain(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	domainID := re.Request.PathValue("id")
	if domainID == "" {
		return re.BadRequestError("domain id is required", nil)
	}

	record, err := app.FindRecordById("mail_domains", domainID)
	if err != nil {
		return re.NotFoundError("domain not found", err)
	}

	orgID := record.GetString("org")
	if err := verifyOrgAdmin(app, re.Auth.Id, orgID); err != nil {
		return re.ForbiddenError("only org admins or owners can verify domains", err)
	}

	details, err := verifyDomainRecord(re.Request.Context(), app, record)
	if err != nil {
		app.Logger().Warn("mail: failed to persist domain verification",
			"domain", record.GetString("domain"), "error", err)
		return re.InternalServerError("failed to save verification result", err)
	}

	return re.JSON(200, map[string]any{
		"id":                      record.Id,
		"verified":                record.GetBool("verified"),
		"mx_verified":             record.GetBool("mx_verified"),
		"inbound_domain_verified": record.GetBool("inbound_domain_verified"),
		"spf_verified":            record.GetBool("spf_verified"),
		"dkim_verified":           record.GetBool("dkim_verified"),
		"return_path_verified":    record.GetBool("return_path_verified"),
		"last_checked_at":         record.GetString("last_checked_at"),
		"verification_details":    details,
	})
}
