// Package cli contributes the `tinycld mail` command group. Commands are pure
// HTTP clients: PocketBase record REST for reads and the typed endpoints
// declared in tinycld.org/packages/mail/api for search, send, and draft.
package cli

import (
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
)

func Register(root *cobra.Command, c *client.Client) {
	mail := &cobra.Command{
		Use:   "mail",
		Short: "Mail: search, read, and send",
	}
	mail.AddCommand(
		newSearchCmd(c),
		newListCmd(c),
		newReadCmd(c),
		newAttachmentsCmd(c),
		newDownloadCmd(c),
		newSendCmd(c),
		newReplyCmd(c),
		newDraftCmd(c),
		newLabelsCmd(c),
		newLabelCmd(c),
		newMailboxesCmd(c),
		newStatusCmd(c),
	)
	// The one-field state mutations (archive/trash/spam/star/move/mark) share
	// an implementation, so they are built as a group.
	mail.AddCommand(newStateCmds(c)...)
	root.AddCommand(mail)
}
