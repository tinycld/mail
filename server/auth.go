package mail

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// verifyMailboxMembership checks that a user has access to a mailbox. Single-org:
// membership rows reference the users collection directly (the former user_org
// junction is gone), so the caller's user id IS the membership key.
func verifyMailboxMembership(app core.App, mailboxID, userID string) (*core.Record, error) {
	records, err := app.FindRecordsByFilter(
		"mail_mailbox_members",
		"mailbox = {:mailbox} && user = {:user}",
		"", // sort
		1,  // limit
		0,  // offset
		map[string]any{"mailbox": mailboxID, "user": userID},
	)
	if err != nil || len(records) == 0 {
		return nil, fmt.Errorf("user %s is not a member of mailbox %s", userID, mailboxID)
	}
	return records[0], nil
}

// verifyAdmin checks that the authenticated user holds an administrative role.
// Single-org: role is a field on the users auth record, so this reads the
// already-authenticated record rather than a membership junction.
func verifyAdmin(auth *core.Record) error {
	if auth == nil {
		return fmt.Errorf("authentication required")
	}
	switch auth.GetString("role") {
	case "owner", "admin":
		return nil
	default:
		return fmt.Errorf("user %s is not an admin", auth.Id)
	}
}
