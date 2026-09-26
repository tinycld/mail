package mail

import (
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// serverOwnedDomainFields are mail_domains columns a client must never write
// through the generic record API.
//
// provider_domain_metadata is the load-bearing one. It carries the provider's
// numeric domain id, which can be a handle into a Postmark ACCOUNT SHARED BY
// MANY DEPLOYMENTS. A client that can choose that id can ask whatever holds
// the account token to dereference another deployment's enrollment — the
// disclosure this guard exists to close. The field is
// written only by handleAddDomain (from the provider's own enrollment
// response) and by checkOutbound (from the provider's own status response).
//
// The *_verified flags and `verified` are derived state, computed by
// verifyDomainRecord from live DNS and provider checks. A client-set flag is
// a green badge on a domain nothing has actually verified — the exact
// regression this branch already fixed once at the enrollment path.
var serverOwnedDomainFields = []string{
	"provider_domain_metadata",
	"verified",
	"mx_verified",
	"inbound_domain_verified",
	"spf_verified",
	"dkim_verified",
	"return_path_verified",
	"verification_details",
	"last_checked_at",
}

// registerMailDomainWriteGuard refuses request-driven creates of mail_domains
// rows, and request-driven writes to the server-owned columns above.
//
// MECHANISM: OnRecordCreateRequest / OnRecordUpdateRequest fire ONLY from
// PocketBase's generic record CRUD API (apis/record_crud.go). An internal
// app.Save — which is how handleAddDomain, verifyDomainRecord and the hourly
// ticker write — runs the OnRecordCreate / OnRecordUpdate family instead and
// never reaches these hooks. That is precisely the request-vs-internal
// distinction this guard needs, so the server's own writes keep working
// untouched and no field has to be hidden from readers to protect it.
//
// The collection's admin/owner API rules are NOT a substitute: the attacker
// here IS an org admin, legitimately authenticated. These fields are not
// admin-restricted, they are not client state at all.
func registerMailDomainWriteGuard(app *pocketbase.PocketBase) {
	app.OnRecordCreateRequest("mail_domains").BindFunc(func(e *core.RecordRequestEvent) error {
		// Enrollment with the provider must happen BEFORE a row exists, and
		// only POST /api/mail/domains does that. A row created directly is a
		// domain the provider has never heard of: it renders as configured
		// and every send from it fails.
		return router.NewApiError(http.StatusForbidden,
			"Mail domains must be created through POST /api/mail/domains so they are enrolled with the mail provider.", nil)
	})

	app.OnRecordUpdateRequest("mail_domains").BindFunc(func(e *core.RecordRequestEvent) error {
		info, err := e.RequestInfo()
		if err != nil {
			return e.BadRequestError("Failed to read request body", err)
		}
		for _, field := range serverOwnedDomainFields {
			if _, present := info.Body[field]; present {
				return router.NewApiError(http.StatusForbidden,
					"The field '"+field+"' is maintained by the server and cannot be set directly.", nil)
			}
		}
		return e.Next()
	})
}
