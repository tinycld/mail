package mail

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/offboard"
)

// registerUsernameFollowHook moves a user's mail address when an admin renames
// their login. The old address stays as an added address, so mail sent to it
// still arrives. It runs in the users save's transaction: a new name that mail
// cannot use refuses the rename instead of leaving login and address apart.
func registerUsernameFollowHook(app core.App) {
	app.OnRecordUpdate("users").BindFunc(func(e *core.RecordEvent) error {
		before := e.Record.Original()
		oldName, newName := before.GetString("username"), e.Record.GetString("username")
		if before.Id == "" || oldName == newName || offboard.IsOffboarded(e.Record) {
			return e.Next()
		}
		identity, err := e.App.FindFirstRecordByFilter("mail_identities",
			"user = {:user} && retired != true", dbx.Params{"user": e.Record.Id})
		if errors.Is(err, sql.ErrNoRows) {
			return e.Next()
		}
		if err != nil {
			return err
		}
		oldEmail := identity.GetString("email_name")
		// Follow only a name derived from the username; an admin's own choice stays.
		if oldEmail != usernameEmailBase(oldName) {
			return e.Next()
		}

		original := e.App
		defer func() { e.App = original }()
		return e.App.RunInTransaction(func(txApp core.App) error {
			e.App = txApp
			if err := e.Next(); err != nil {
				return err
			}
			identity.Set("email_name", usernameEmailBase(newName))
			if err := txApp.Save(identity); err != nil {
				return emailNameAsUsername(err)
			}
			return keepOldAddress(txApp, identity, oldEmail)
		})
	})
}

// usernameEmailBase is the email name deriveEmailName tries first.
func usernameEmailBase(username string) string {
	return addressSanitizer.ReplaceAllString(strings.ToLower(strings.TrimSpace(username)), "")
}

// keepOldAddress adds the old main address back to the inbox as an added
// address.
func keepOldAddress(app core.App, identity *core.Record, local string) error {
	inboxID := identity.GetString("inbox")
	if inboxID == "" {
		return nil
	}
	inbox, err := app.FindRecordById("mail_mailboxes", inboxID)
	if err != nil {
		return err
	}
	col, err := app.FindCollectionByNameOrId("mail_addresses")
	if err != nil {
		return err
	}
	row := core.NewRecord(col)
	row.Set("local_part", local)
	row.Set("domain", inbox.GetString("domain"))
	row.Set("mailbox", inbox.Id)
	row.Set("source", addressSourceManual)
	return app.Save(row)
}

// emailNameAsUsername puts a refusal of the new email name on username, the
// field the admin edited.
func emailNameAsUsername(err error) error {
	var fields validation.Errors
	if errors.As(err, &fields) {
		if refusal, ok := fields["email_name"]; ok {
			return validation.Errors{"username": refusal}
		}
	}
	return err
}
