package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
)

// folderCounts is one row of the mail_folder_counts view. The view's `inbox`
// column is the UNREAD count (folder='inbox' AND is_read=0), not the total.
type folderCounts struct {
	ID      string `json:"id"`
	User    string `json:"user"`
	Mailbox string `json:"mailbox"`
	Inbox   int    `json:"inbox"`
	Drafts  int    `json:"drafts"`
	Sent    int    `json:"sent"`
	Starred int    `json:"starred"`
	Trash   int    `json:"trash"`
	Spam    int    `json:"spam"`
}

// statusRow joins the counts with the mailbox address for output.
type statusRow struct {
	folderCounts
	Address string `json:"address"`
}

func newStatusCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Per-mailbox unread and folder counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			userID, err := c.UserID(ctx)
			if err != nil {
				return err
			}
			counts, err := client.ListAll[folderCounts](ctx, c, "mail_folder_counts",
				client.Filter("user = {:u}", map[string]any{"u": userID}), "")
			if err != nil {
				return err
			}
			addresses, err := mailboxAddresses(ctx, c)
			if err != nil {
				return err
			}
			raw := make([]statusRow, len(counts))
			rows := make([][]string, len(counts))
			for i, fc := range counts {
				raw[i] = statusRow{folderCounts: fc, Address: addresses[fc.Mailbox]}
				rows[i] = []string{
					addresses[fc.Mailbox],
					strconv.Itoa(fc.Inbox), strconv.Itoa(fc.Drafts), strconv.Itoa(fc.Sent),
					strconv.Itoa(fc.Starred), strconv.Itoa(fc.Trash), strconv.Itoa(fc.Spam),
				}
			}
			return o.Write(cmd.OutOrStdout(),
				[]string{"MAILBOX", "UNREAD", "DRAFTS", "SENT", "STARRED", "TRASH", "SPAM"}, rows, raw)
		},
	}
}
