package mail

import (
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/groups"
)

// A group can be a member of a shared inbox. mail_mailbox_members is a core
// groups grant table (migration 2040000013):
//   - a person row has `user` set and `group` empty;
//   - a grant row has `group` set and `user` empty;
//   - a derived row has both set. Core's expander writes one per group member
//     and removes it when the member leaves or the grant goes.
//
// The rules let an owner write person rows and grants. The hooks here keep a
// group's access at role member and refuse every client edit of a derived row,
// which only core's expander may change.

// isGrantRow reports a group grant: group set, no user.
func isGrantRow(r *core.Record) bool {
	return r.GetString("user") == "" && r.GetString("group") != ""
}

// isDerivedRow reports a row core's expander made from a grant.
func isDerivedRow(r *core.Record) bool {
	return r.GetString("user") != "" && r.GetString("group") != ""
}

// registerMailboxMemberGroups declares the grant table to core and binds the
// role and derived-row guards. It must run before core's groups.Register, which
// binds the expander for every registered table: packages register in
// RegisterExtras, before RegisterSharedCore. Bind it before
// registerMailboxLastOwnerGuard so a derived row is refused with its group's
// name before the last-owner check runs.
func registerMailboxMemberGroups(app core.App) {
	groups.RegisterGrantTable(groups.GrantTable{Collection: "mail_mailbox_members", ResourceField: "mailbox"})

	// A group never owns an inbox: an owner adds people and grants, and the
	// people in a group are not the owner's choice. The expander copies the
	// grant's role, so this also covers derived rows.
	forceGroupRoleMember := func(e *core.RecordEvent) error {
		if e.Record.GetString("group") != "" {
			e.Record.Set("role", "member")
		}
		return e.Next()
	}
	app.OnRecordCreate("mail_mailbox_members").BindFunc(forceGroupRoleMember)
	app.OnRecordUpdate("mail_mailbox_members").BindFunc(forceGroupRoleMember)

	app.OnRecordUpdateRequest("mail_mailbox_members").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		original := e.Record.Original()
		if isDerivedRow(original) {
			return refuseDerivedRow(e, original)
		}
		// A row keeps what it is: a person row cannot become a grant or another
		// person's row, and a grant cannot move to another group or inbox. Only
		// the role can change.
		for _, field := range []string{"mailbox", "user", "group"} {
			e.Record.Set(field, original.Get(field))
		}
		return e.Next()
	})

	app.OnRecordDeleteRequest("mail_mailbox_members").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.HasSuperuserAuth() || !isDerivedRow(e.Record) {
			return e.Next()
		}
		return refuseDerivedRow(e, e.Record)
	})
}

// refuseDerivedRow answers 403 with the name of the group that gives the
// access, so the reader knows where the access comes from and who can end it.
func refuseDerivedRow(e *core.RecordRequestEvent, row *core.Record) error {
	group, err := e.App.FindRecordById("groups", row.GetString("group"))
	if err != nil {
		return e.InternalServerError("could not load the group that gives this access", err)
	}
	name := group.GetString("name")
	if e.Auth != nil && e.Auth.Id == row.GetString("user") {
		return e.ForbiddenError("You have access through the group "+name+". Ask an admin to remove you from the group.", nil)
	}
	return e.ForbiddenError("This person has access through the group "+name+". Remove the group from the inbox, or ask an admin to remove the person from the group.", nil)
}
