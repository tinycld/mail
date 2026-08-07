package cli

import (
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
)

// mailboxRow is the JSON row `mail mailboxes` emits: one line per address you
// can send as, so aliases are visible alongside each mailbox's own address.
type mailboxRow struct {
	identity
	Role string `json:"role"`
	Kind string `json:"kind"`
}

func newMailboxesCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "mailboxes",
		Short: "List the addresses you can read and send as",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			ids, err := sendableIdentities(ctx, c)
			if err != nil {
				return err
			}
			members, err := userMemberships(ctx, c)
			if err != nil {
				return err
			}
			roles := make(map[string]string, len(members))
			for _, m := range members {
				roles[m.Mailbox] = m.Role
			}

			raw := make([]mailboxRow, len(ids))
			rows := make([][]string, len(ids))
			for i, id := range ids {
				kind := "alias"
				if id.Primary {
					kind = "mailbox"
				}
				raw[i] = mailboxRow{identity: id, Role: roles[id.MailboxID], Kind: kind}
				rows[i] = []string{
					id.Address, id.DisplayName, kind, roles[id.MailboxID], id.MailboxID,
				}
			}
			return o.Write(cmd.OutOrStdout(),
				[]string{"ADDRESS", "NAME", "KIND", "ROLE", "MAILBOX"}, rows, raw)
		},
	}
}
