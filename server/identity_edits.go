package mail

import (
	"errors"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

const maxEmailNameLength = 64

// registerIdentityEditHooks lets a person choose their Default From and an
// admin rename a person's email name. The update rule only says who may
// write the row; these hooks say which fields, and which values, are allowed.
func registerIdentityEditHooks(app core.App) {
	app.OnRecordUpdateRequest("mail_identities").BindFunc(func(e *core.RecordRequestEvent) error {
		original := e.Record.Original()
		// A tombstone's name is the server's; the model hook skips retired rows.
		if original.GetBool("retired") {
			return e.ForbiddenError("This person has left. Their email settings cannot change.", nil)
		}
		// Restore rather than refuse: a client that saves an edited row can
		// send the whole record back.
		for _, field := range []string{"user", "inbox"} {
			e.Record.Set(field, original.GetString(field))
		}
		e.Record.Set("retired", original.GetBool("retired"))

		renamed := e.Record.GetString("email_name") != original.GetString("email_name")
		if renamed && !e.HasSuperuserAuth() && !isAdminAuth(e.Auth) {
			return e.ForbiddenError("Only an admin can change an email name.", nil)
		}
		if err := checkDefaultFrom(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})

	// A model hook, so the reconciler's and every other server save is checked
	// too. A retired identity holds a tombstone name the server wrote.
	app.OnRecordUpdate("mail_identities").BindFunc(func(e *core.RecordEvent) error {
		identity := e.Record
		if identity.GetBool("retired") ||
			identity.GetString("email_name") == identity.Original().GetString("email_name") {
			return e.Next()
		}
		name := normalizeLocalPart(identity.GetString("email_name"))
		identity.Set("email_name", name)
		if err := checkEmailName(e.App, identity, name); err != nil {
			return err
		}
		return e.Next()
	})
}

func isAdminAuth(auth *core.Record) bool {
	if auth == nil {
		return false
	}
	role := auth.GetString("role")
	return role == "admin" || role == "owner"
}

// checkDefaultFrom accepts an empty value (the inbox's own primary address
// is used) or an address that delivers to the identity's inbox.
func checkDefaultFrom(app core.App, identity *core.Record) error {
	id := identity.GetString("default_from")
	if id == "" {
		return nil
	}
	address, err := app.FindRecordById("mail_addresses", id)
	if err != nil || address.GetString("mailbox") != identity.GetString("inbox") {
		return onField("default_from", validation.NewError("default_from_foreign",
			"Choose an address of this person's own inbox."))
	}
	return nil
}

func checkEmailName(app core.App, identity *core.Record, name string) error {
	if err := checkMailboxLocalPart(name); err != nil {
		return onField("email_name", err)
	}
	if len(name) > maxEmailNameLength {
		return onField("email_name", validation.NewError("email_name_long",
			"An email name can have at most 64 characters."))
	}
	// Before the taken check, which also fires on a reserved name but cannot
	// say which addresses hold it.
	if err := checkEmailNameFree(app, identity, name); err != nil {
		return err
	}
	taken, err := emailNameTaken(app, name, identity.GetString("inbox"))
	if err != nil {
		return err
	}
	if taken {
		return onField("email_name", validation.NewError("email_name_taken",
			name+" is already in use."))
	}
	return nil
}

// checkEmailNameFree refuses a name that is already an address of another
// inbox on a domain with a rule, naming those addresses.
func checkEmailNameFree(app core.App, identity *core.Record, name string) error {
	rows, err := app.FindRecordsByFilter("mail_addresses",
		"local_part = {:name} && mailbox != {:inbox} && "+ruleDomainRelFilter, "", 0, 0,
		dbx.Params{"name": name, "inbox": identity.GetString("inbox")})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	list := make([]string, 0, len(rows))
	for _, row := range rows {
		domain, err := app.FindRecordById("mail_domains", row.GetString("domain"))
		if err != nil {
			return errors.Join(errors.New("address domain"), err)
		}
		list = append(list, row.GetString("local_part")+"@"+domain.GetString("domain"))
	}
	sort.Strings(list)
	return validation.Errors{"email_name": validation.NewError("email_name_reserved",
		"These addresses already use this name: "+strings.Join(list, ", "))}
}
