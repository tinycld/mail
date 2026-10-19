package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/groups"
)

const memberCleanupBatch = 500

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

// userHasMailboxRow reports whether the user has a member row on the mailbox:
// a direct row or a row derived from a group.
func userHasMailboxRow(app core.App, mailboxID, userID string) (bool, error) {
	n, err := app.CountRecords("mail_mailbox_members",
		dbx.HashExp{"mailbox": mailboxID, "user": userID},
	)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// registerMemberAccessCleanup removes a person's thread state for an inbox
// when the person loses the last member row that gave access to it: they were
// removed, they left the last group that has access, or that group or its
// grant was deleted. A person with a direct row who leaves a group, or who
// is in two groups and leaves one, keeps their state.
//
// The cleanup runs as a member job (member_jobs.go) for the reason the
// backfill does (member_backfill.go): an inbox can hold many thousands of
// threads, and one delete per row must not hold up the member delete. The
// root app is captured at registration because the job outlives the request,
// and the event's app can be a finished transaction. For the same reason the
// access check runs in the job with the root app, not in the hook.
func registerMemberAccessCleanup(app core.App) {
	memberJobs := memberJobsFor(app)
	app.OnRecordAfterDeleteSuccess("mail_mailbox_members").BindFunc(func(e *core.RecordEvent) error {
		mailboxID := e.Record.GetString("mailbox")
		userID := e.Record.GetString("user")
		if userID == "" {
			return e.Next()
		}
		runMemberJob(memberJobs, "access cleanup", mailboxID, userID, func(ctx context.Context) error {
			return cleanupLostMailboxAccess(ctx, app, mailboxID, userID)
		})
		return e.Next()
	})
}

// cleanupLostMailboxAccess deletes the user's mail_thread_state rows for the
// mailbox, in batches, and then the user's counts row for it. It does nothing
// while the user still has a member row on the mailbox, and checks again
// before each batch: a person can be added back while a cleanup runs, and
// their new state must stay. Each row is a record delete, so realtime and the
// folder-count hook fire. It stops between batches once ctx ends (the app's
// terminate).
func cleanupLostMailboxAccess(ctx context.Context, app core.App, mailboxID, userID string) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		has, err := userHasMailboxRow(app, mailboxID, userID)
		if err != nil {
			return fmt.Errorf("member rows of user %s on mailbox %s: %w", userID, mailboxID, err)
		}
		if has {
			return nil
		}
		states, err := app.FindRecordsByFilter(
			"mail_thread_state",
			"mailbox = {:mailbox} && user = {:user}",
			"id",
			memberCleanupBatch,
			0,
			map[string]any{"mailbox": mailboxID, "user": userID},
		)
		if err != nil {
			return fmt.Errorf("list state rows of user %s on mailbox %s: %w", userID, mailboxID, err)
		}
		for _, state := range states {
			if err := app.Delete(state); err != nil {
				return fmt.Errorf("delete state row %s: %w", state.Id, err)
			}
		}
		if len(states) < memberCleanupBatch {
			break
		}
	}

	counts, err := app.FindRecordById("mail_folder_counts", folderCountsID(userID, mailboxID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("counts row of user %s on mailbox %s: %w", userID, mailboxID, err)
	}
	if err := app.Delete(counts); err != nil {
		return fmt.Errorf("delete counts row of user %s on mailbox %s: %w", userID, mailboxID, err)
	}
	return nil
}
