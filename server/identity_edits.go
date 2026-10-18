package mail

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

const maxEmailNameLength = 64

// registerIdentityEditHooks lets a person choose their Default From and their
// address views, and an admin rename a person's email name. The update rule only says who may
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
		if err := checkHiddenAddressViews(e.App, e.Record); err != nil {
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
		// One transaction, so a move that fails also undoes the rename. The
		// move runs before the identity's reconcile (an after-success hook),
		// which would otherwise add the new name as a rule row and block it.
		original := e.App
		defer func() { e.App = original }()
		return e.App.RunInTransaction(func(txApp core.App) error {
			if err := moveMainAddress(txApp, identity, name); err != nil {
				return err
			}
			e.App = txApp
			return e.Next()
		})
	})
}

// moveMainAddress makes the email name the address of the identity's inbox.
// The primary row moves in place (same id), so a Default From that pointed at
// the main address follows the new name. A row of the same inbox that already
// holds the name (an old name kept as an added address) is taken over: it is
// deleted, and a Default From that pointed at it moves to the main address.
func moveMainAddress(app core.App, identity *core.Record, name string) error {
	inboxID := identity.GetString("inbox")
	if inboxID == "" {
		return nil
	}
	inbox, err := app.FindRecordById("mail_mailboxes", inboxID)
	if err != nil {
		return fmt.Errorf("identity inbox: %w", err)
	}
	if inbox.GetString("address") == name {
		return nil
	}
	held, err := findAddressRow(app, name, inbox.GetString("domain"))
	if err != nil {
		return err
	}
	takenOver := ""
	if held != nil && held.GetString("mailbox") == inboxID && !held.GetBool("is_primary") {
		takenOver = held.Id
		if err := app.Delete(held); err != nil {
			return fmt.Errorf("take over %s: %w", name, err)
		}
	}
	// A row of another inbox makes the inbox save refuse the address;
	// checkEmailName has already refused that name, so this is a race.
	inbox.Set("address", name)
	if err := app.Save(inbox); err != nil {
		return inboxAddressAsEmailName(err)
	}
	if err := savePrimaryAddress(app, inbox); err != nil {
		return fmt.Errorf("move the main address: %w", err)
	}
	if takenOver != "" && identity.GetString("default_from") == takenOver {
		primary, err := app.FindFirstRecordByFilter("mail_addresses",
			"mailbox = {:inbox} && is_primary = true", dbx.Params{"inbox": inboxID})
		if err != nil {
			return fmt.Errorf("main address lookup: %w", err)
		}
		identity.Set("default_from", primary.Id)
	}
	return nil
}

// inboxAddressAsEmailName puts the inbox's refusal of its new address under
// email_name, the field the admin edited.
func inboxAddressAsEmailName(err error) error {
	var fields validation.Errors
	if errors.As(err, &fields) {
		if refusal, ok := fields["address"]; ok {
			return validation.Errors{"email_name": refusal}
		}
	}
	return err
}

func isAdminAuth(auth *core.Record) bool {
	if auth == nil {
		return false
	}
	role := auth.GetString("role")
	return role == "admin" || role == "owner"
}

// checkDefaultFrom accepts an empty value (mail goes out from the inbox's
// address on the highest-ranked verified domain, see defaultSendAddress) or an
// address that delivers to the identity's inbox.
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

// checkHiddenAddressViews accepts only addresses of the identity's inbox as
// hidden views. Only the ids this save adds are checked: an id saved before
// stays valid if its address later moves, so a client that sends the whole
// list back is not refused for it.
func checkHiddenAddressViews(app core.App, identity *core.Record) error {
	before := identity.Original().GetStringSlice("hidden_address_views")
	for _, id := range identity.GetStringSlice("hidden_address_views") {
		if slices.Contains(before, id) {
			continue
		}
		address, err := app.FindRecordById("mail_addresses", id)
		if err != nil || address.GetString("mailbox") != identity.GetString("inbox") {
			return onField("hidden_address_views", validation.NewError("hidden_address_view_foreign",
				"Choose only addresses of this person's own inbox."))
		}
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
// inbox on a domain with a rule, or on the domain of the identity's main
// address (the name becomes that address, whatever the domain's coverage). It
// names those addresses.
func checkEmailNameFree(app core.App, identity *core.Record, name string) error {
	inboxID := identity.GetString("inbox")
	domains := ruleDomainRelFilter
	params := dbx.Params{"name": name, "inbox": inboxID}
	if inboxID != "" {
		inbox, err := app.FindRecordById("mail_mailboxes", inboxID)
		if err != nil {
			return fmt.Errorf("identity inbox: %w", err)
		}
		domains = "(" + ruleDomainRelFilter + " || domain = {:main})"
		params["main"] = inbox.GetString("domain")
	}
	rows, err := app.FindRecordsByFilter("mail_addresses",
		"local_part = {:name} && mailbox != {:inbox} && "+domains, "", 0, 0, params)
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
