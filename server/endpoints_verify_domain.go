package mail

import (
	"fmt"
	"os"

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
	// TODO(debug): remove. Entry point for the whole verify flow.
	fmt.Fprintf(os.Stdout, "MAIL DEBUG handleVerifyDomain: domainID=%q domain=%q orgID=%q authUser=%q\n",
		domainID, record.GetString("domain"), orgID, re.Auth.Id)

	if err := verifyOrgAdmin(app, re.Auth.Id, orgID); err != nil {
		return re.ForbiddenError("only org admins or owners can verify domains", err)
	}

	if !providerForOrg(app, orgID).Configured() {
		// TODO(debug): remove.
		fmt.Fprintf(os.Stdout, "MAIL DEBUG handleVerifyDomain: provider NOT configured for orgID=%q — rejecting\n", orgID)
		return re.BadRequestError(
			"configure the mail provider (Postmark server token) in settings before verifying",
			errProviderNotConfigured,
		)
	}

	details, saveErr := verifyDomainRecord(re.Request.Context(), app, record)

	body := map[string]any{
		"id":                      record.Id,
		"verified":                record.GetBool("verified"),
		"mx_verified":             record.GetBool("mx_verified"),
		"inbound_domain_verified": record.GetBool("inbound_domain_verified"),
		"spf_verified":            record.GetBool("spf_verified"),
		"dkim_verified":           record.GetBool("dkim_verified"),
		"return_path_verified":    record.GetBool("return_path_verified"),
		"last_checked_at":         record.GetString("last_checked_at"),
		"verification_details":    details,
		"saved":                   saveErr == nil,
	}
	if saveErr != nil {
		app.Logger().Warn("mail: failed to persist domain verification",
			"domain", record.GetString("domain"), "error", saveErr)
		body["save_error"] = saveErr.Error()
	}

	return re.JSON(200, body)
}
