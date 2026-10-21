package mail

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// addressOwner is mail's authidentity resolver: the person whose own inbox
// an address reaches, so they can sign in with any of their mail addresses —
// in a mail app, which puts the address in the user name field, and on the
// sign-in screen. An address of a shared inbox or of a group names no one
// person and signs no one in.
func addressOwner(app core.App, address string) (*core.Record, error) {
	local, domain := splitAddress(address)
	target, err := resolveAddress(app, stripPlusTag(local), domain)
	if errors.Is(err, errAddressNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	identity, err := app.FindFirstRecordByFilter("mail_identities", "inbox = {:inbox} && retired != true", dbx.Params{"inbox": target.Mailbox.Id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	user, err := app.FindRecordById("users", identity.GetString("user"))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return user, err
}
