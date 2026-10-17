package mail

import (
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

// isGroupAddress reports whether an address delivers to a group (every
// member's own inbox) instead of one inbox.
func isGroupAddress(r *core.Record) bool {
	return r.GetString("group") != ""
}

// checkAddressTarget refuses a row that names both an inbox and a group, or
// neither. The schema cannot say "exactly one", so this hook does.
func checkAddressTarget(r *core.Record) error {
	hasMailbox := r.GetString("mailbox") != ""
	if hasMailbox && isGroupAddress(r) {
		return onField("group", validation.NewError("address_target_both", "Choose an inbox or a group, not both."))
	}
	if !hasMailbox && !isGroupAddress(r) {
		return onField("group", validation.NewError("address_target_missing", "Choose an inbox or a group."))
	}
	return nil
}

// registerGroupAddressHooks guards the rows that deliver to a group. Call it
// before registerAddressHooks: a row with no valid target is refused before
// the availability check names a conflict on it.
//
// A group address is always added by hand and is never a primary address:
// rule rows and primary rows belong to one person's inbox. via_group says
// which group gave a rule address, so it has no meaning here.
func registerGroupAddressHooks(app core.App) {
	guard := func(e *core.RecordEvent) error {
		if err := checkAddressTarget(e.Record); err != nil {
			return err
		}
		if isGroupAddress(e.Record) {
			e.Record.Set("source", addressSourceManual)
			e.Record.Set("is_primary", false)
			e.Record.Set("via_group", "")
		}
		return e.Next()
	}
	app.OnRecordCreate("mail_addresses").BindFunc(guard)
	app.OnRecordUpdate("mail_addresses").BindFunc(guard)
}
