package mail

import (
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
)

// registerAutomationResolver installs mail's OwnerResolver for the
// "mail:message-received" trigger so personal automation rules can match
// against an arriving message. mail_messages has no direct user FK — a
// message belongs to whichever users are members of its mailbox — so the
// generic owner auto-detection in automation.ResolveOwners can't find it.
func registerAutomationResolver() {
	automation.RegisterOwnerResolver("mail:message-received", messageOwnerResolver)
}

// messageOwnerResolver maps an arriving mail_messages record to the user ids
// who own it, via the same thread -> mailbox -> mail_mailbox_members path
// bufferMailNotification already uses to pick notification recipients.
// Returns nil (never an error) on any absent or malformed data so org-level
// rules still fire even when a personal-rule owner can't be determined.
func messageOwnerResolver(app core.App, record *core.Record) []string {
	if record == nil {
		return nil
	}

	threadID := record.GetString("thread")
	if threadID == "" {
		return nil
	}
	thread, err := app.FindRecordById("mail_threads", threadID)
	if err != nil {
		return nil
	}

	mailboxID := thread.GetString("mailbox")
	if mailboxID == "" {
		return nil
	}

	members, err := app.FindRecordsByFilter(
		"mail_mailbox_members",
		"mailbox = {:mailboxId}",
		"",
		0,
		0,
		map[string]any{"mailboxId": mailboxID},
	)
	if err != nil || len(members) == 0 {
		return nil
	}

	var owners []string
	for _, member := range members {
		if userID := member.GetString("user"); userID != "" {
			owners = append(owners, userID)
		}
	}
	return owners
}
