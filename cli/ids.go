package cli

import (
	"context"
	"fmt"

	"tinycld.org/cli/client"
)

// mailbox is the mail_mailboxes row shape the CLI reads.
type mailbox struct {
	ID          string `json:"id"`
	Address     string `json:"address"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

// membership is the mail_mailbox_members row binding a user to a mailbox.
type membership struct {
	ID      string `json:"id"`
	Mailbox string `json:"mailbox"`
	User    string `json:"user"`
	Role    string `json:"role"`
}

// userMemberships lists the caller's mailbox memberships, oldest first — the
// same ordering useDefaultMailbox relies on for the default.
func userMemberships(ctx context.Context, c *client.Client) ([]membership, error) {
	userID, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListAll[membership](ctx, c, "mail_mailbox_members",
		client.Filter("user = {:u}", map[string]any{"u": userID}), "created")
}

// resolveMailbox turns the --mailbox flag into a mailbox id. Empty selects
// the user's first membership (the app's default-mailbox rule); otherwise the
// value may be a mailbox id or an address.
func resolveMailbox(ctx context.Context, c *client.Client, flag string) (string, error) {
	members, err := userMemberships(ctx, c)
	if err != nil {
		return "", err
	}
	if len(members) == 0 {
		return "", fmt.Errorf("you are not a member of any mailbox")
	}
	if flag == "" {
		return members[0].Mailbox, nil
	}
	for _, m := range members {
		if m.Mailbox == flag {
			return m.Mailbox, nil
		}
	}
	// Not an id — try to match an address among the user's mailboxes.
	for _, m := range members {
		mb, err := client.GetRecord[mailbox](ctx, c, "mail_mailboxes", m.Mailbox)
		if err != nil {
			return "", err
		}
		if mb.Address == flag {
			return mb.ID, nil
		}
	}
	return "", fmt.Errorf("no mailbox %q among your memberships", flag)
}

// mailboxAddresses maps the user's mailbox ids to their addresses, for
// display columns.
func mailboxAddresses(ctx context.Context, c *client.Client) (map[string]string, error) {
	members, err := userMemberships(ctx, c)
	if err != nil {
		return nil, err
	}
	addr := map[string]string{}
	for _, m := range members {
		mb, err := client.GetRecord[mailbox](ctx, c, "mail_mailboxes", m.Mailbox)
		if err != nil {
			return nil, err
		}
		addr[mb.ID] = mb.Address
	}
	return addr, nil
}
