package mail

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/offboard"
)

// Offboarding anonymizes a users row and never deletes it, so a leaver's
// mail_mailbox_members rows survive. A shared mailbox whose only owner leaves
// is then owned by an account nobody can sign in to, and nobody can manage
// its members again. core's flat FK rewrite (RegisterReassignable) cannot fix
// that: it would hand the successor every membership the leaver held, the
// leaver's personal inbox and member rows on other people's mailboxes
// included, and it fails on the unique (mailbox, user) index when the
// successor is already a member. So mail settles ownership itself, mailbox by
// mailbox, inside the offboard transaction.
//
// The leaver's personal mailbox is deleted, in every mode: it is the leaver's
// own address, and it must not stay live on an account nobody can sign in to.
// A reassign plan moves authored content, not an inbox, so the successor gets
// no copy. The delete cascades to the memberships of anyone the leaver shared
// the mailbox with.

// soleOwnedMailbox is a shared mailbox where the user is the only owner.
// OtherMembers counts the members that are not the user: when it is zero,
// nobody else can reach the mailbox.
type soleOwnedMailbox struct {
	ID           string `db:"id"`
	Name         string `db:"name"`
	Address      string `db:"address"`
	OtherMembers int    `db:"other_members"`
}

func (m soleOwnedMailbox) label() string {
	if m.Name != "" {
		return m.Name
	}
	return m.Address
}

func findSoleOwnedSharedMailboxes(app core.App, userID string) ([]soleOwnedMailbox, error) {
	var out []soleOwnedMailbox
	err := app.DB().NewQuery(`
		SELECT mb.id AS id, mb.name AS name, mb.address AS address,
			(SELECT COUNT(*) FROM mail_mailbox_members o
				WHERE o.mailbox = m.mailbox AND o.user != {:user}) AS other_members
		FROM mail_mailbox_members m
		JOIN mail_mailboxes mb ON mb.id = m.mailbox
		WHERE m.user = {:user} AND m.role = 'owner' AND mb.type = 'shared'
			AND NOT EXISTS (SELECT 1 FROM mail_mailbox_members o
				WHERE o.mailbox = m.mailbox AND o.role = 'owner' AND o.user != {:user})
		ORDER BY mb.name, mb.id`,
	).Bind(dbx.Params{"user": userID}).All(&out)
	if err != nil {
		return nil, fmt.Errorf("find sole-owned shared mailboxes: %w", err)
	}
	return out, nil
}

// mailboxHeir picks who takes over the leaver's sole-owned shared mailboxes:
// the successor in reassign mode, the acting admin in delete-my-data mode. It
// returns "" when there is nobody: a self-delete (the actor is the leaver) or
// a system offboard with no actor.
func mailboxHeir(leaverID string, plan offboard.Plan, actorUserID string) string {
	if plan.Mode == offboard.ModeReassign {
		return plan.SuccessorUserID
	}
	if actorUserID == "" || actorUserID == leaverID {
		return ""
	}
	return actorUserID
}

// offboardMail is mail's offboard.Handler. It first settles the shared
// mailboxes, which can refuse the whole offboard, then deletes the leaver's
// personal mailboxes.
func offboardMail(txApp core.App, leaver *core.Record, plan offboard.Plan, actorUserID string) error {
	if err := handOverSoleOwnedMailboxes(txApp, leaver, plan, actorUserID); err != nil {
		return err
	}
	return deletePersonalMailboxesOf(txApp, leaver.Id)
}

// handOverSoleOwnedMailboxes settles the shared mailboxes. It only adds or
// upgrades the heir's membership: it never deletes a mailbox, a message or a
// membership, and it leaves every mailbox that has another owner alone.
//
// With no heir (a self-delete in delete-my-data or keep mode, keep being an
// account delete with no plan) it follows core's last-owner guard: refuse,
// and tell the user what to do first. A shared mailbox nobody else uses is
// left as it is, because no one loses access.
func handOverSoleOwnedMailboxes(txApp core.App, leaver *core.Record, plan offboard.Plan, actorUserID string) error {
	owned, err := findSoleOwnedSharedMailboxes(txApp, leaver.Id)
	if err != nil {
		return err
	}
	if len(owned) == 0 {
		return nil
	}

	heirID := mailboxHeir(leaver.Id, plan, actorUserID)
	if heirID == "" {
		for _, mb := range owned {
			if mb.OtherMembers > 0 {
				return fmt.Errorf("%w: you are the only owner of the shared mailbox %q, which other people use. "+
					"Make one of them an owner or delete the mailbox, or choose a successor for your content, "+
					"before you delete your account",
					offboard.ErrInvalidPlan, mb.label())
			}
		}
		return nil
	}

	heir, err := txApp.FindRecordById("users", heirID)
	if err != nil {
		return fmt.Errorf("load mailbox heir %s: %w", heirID, err)
	}
	// Guests are kept out of mail infrastructure (1830000002 and 1830000003),
	// so mailbox ownership must not reach them through the back door either.
	if heir.GetString("role") == "guest" {
		return fmt.Errorf("%w: a guest cannot take over shared mailboxes; choose a member as the successor",
			offboard.ErrInvalidPlan)
	}

	for _, mb := range owned {
		if err := makeMailboxOwner(txApp, mb.ID, heirID); err != nil {
			return err
		}
	}
	return nil
}

// makeMailboxOwner gives the user an owner membership on the mailbox,
// upgrading the existing row when there is one: the unique (mailbox, user)
// index allows only one.
func makeMailboxOwner(txApp core.App, mailboxID, userID string) error {
	existing, err := txApp.FindFirstRecordByFilter("mail_mailbox_members",
		"mailbox = {:mailbox} && user = {:user}",
		dbx.Params{"mailbox": mailboxID, "user": userID})
	if err == nil {
		if existing.GetString("role") == "owner" {
			return nil
		}
		existing.Set("role", "owner")
		if err := txApp.Save(existing); err != nil {
			return fmt.Errorf("upgrade membership on mailbox %s: %w", mailboxID, err)
		}
		return nil
	}

	col, err := txApp.FindCollectionByNameOrId("mail_mailbox_members")
	if err != nil {
		return fmt.Errorf("mail_mailbox_members collection: %w", err)
	}
	member := core.NewRecord(col)
	member.Set("mailbox", mailboxID)
	member.Set("user", userID)
	member.Set("role", "owner")
	if err := txApp.Save(member); err != nil {
		return fmt.Errorf("add owner to mailbox %s: %w", mailboxID, err)
	}
	return nil
}

// registerSoleOwnerDeleteGuard refuses a direct users delete (the REST
// DELETE that the users deleteRule allows on your own account) while the user
// is the only owner of a shared mailbox other people use. The
// mail_mailbox_members.user cascade would remove the owner's row and leave
// its members with a mailbox nobody can manage; the cascade is not a request
// on mail_mailbox_members, so registerMailboxLastOwnerGuard never sees it.
// The offboard path is not affected: it anonymizes the users row and never
// deletes it.
//
// Superusers step aside, as in core's last-owner guard: they can add an owner
// back from the superuser console.
func registerSoleOwnerDeleteGuard(app core.App) {
	app.OnRecordDeleteRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.Auth != nil && e.Auth.IsSuperuser() {
			return e.Next()
		}
		owned, err := findSoleOwnedSharedMailboxes(e.App, e.Record.Id)
		if err != nil {
			return e.InternalServerError("mailbox ownership check failed", err)
		}
		for _, mb := range owned {
			if mb.OtherMembers > 0 {
				return e.ForbiddenError(fmt.Sprintf(
					"You are the only owner of a shared mailbox %q other people use. "+
						"Make someone else an owner, delete the mailbox, or delete your account "+
						"through account settings and choose a successor.", mb.label()), nil)
			}
		}
		return e.Next()
	})
}
