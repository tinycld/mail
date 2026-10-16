package mail

import (
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// loginIdentifier maps one of a person's mail addresses to their username, so
// IMAP and SMTP accept it as the user name. Mail apps put the email address
// in that field, and a person's mail address is usually not their account
// email. Anything else is returned unchanged for davauth.VerifyCredentials to
// accept or refuse with its usual timing.
func loginIdentifier(app core.App, identifier string) string {
	if !strings.Contains(identifier, "@") {
		return identifier
	}
	if _, err := app.FindAuthRecordByEmail("users", identifier); err == nil {
		return identifier
	}
	local, domain := splitAddress(identifier)
	target, err := resolveAddress(app, stripPlusTag(local), domain)
	if err != nil {
		return identifier
	}
	identity, err := app.FindFirstRecordByFilter("mail_identities", "inbox = {:inbox}", dbx.Params{"inbox": target.Mailbox.Id})
	if err != nil {
		return identifier
	}
	user, err := app.FindRecordById("users", identity.GetString("user"))
	if err != nil || user.GetString("username") == "" {
		return identifier
	}
	return user.GetString("username")
}
