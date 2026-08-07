package cli

import (
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
)

// mailboxRow is the JSON row `mail mailboxes` emits.
type mailboxRow struct {
	mailbox
	Role string `json:"role"`
}

func newMailboxesCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "mailboxes",
		Short: "List the mailboxes you are a member of",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			members, err := userMemberships(ctx, c)
			if err != nil {
				return err
			}
			raw := make([]mailboxRow, len(members))
			rows := make([][]string, len(members))
			for i, m := range members {
				mb, err := client.GetRecord[mailbox](ctx, c, "mail_mailboxes", m.Mailbox)
				if err != nil {
					return err
				}
				raw[i] = mailboxRow{mailbox: mb, Role: m.Role}
				rows[i] = []string{mb.Address, mb.DisplayName, mb.Type, m.Role, mb.ID}
			}
			return o.Write(cmd.OutOrStdout(),
				[]string{"ADDRESS", "NAME", "TYPE", "ROLE", "ID"}, rows, raw)
		},
	}
}
