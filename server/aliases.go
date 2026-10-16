package mail

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// verifyAliasBelongsToMailbox returns an error if alias is non-nil and does
// not belong to mailboxID. Nil alias is OK.
func verifyAliasBelongsToMailbox(alias *core.Record, mailboxID string) error {
	if alias == nil {
		return nil
	}
	if alias.GetString("mailbox") != mailboxID {
		return fmt.Errorf("alias %s does not belong to mailbox %s", alias.Id, mailboxID)
	}
	return nil
}
