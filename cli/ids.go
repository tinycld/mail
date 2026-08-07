package cli

import (
	"context"
	"fmt"
	"strings"

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
	// Not an id, so match on the address. mail_mailboxes.address holds only the
	// LOCAL PART, so comparing against it directly would reject the full
	// address every other surface shows; go through the identities, which join
	// the mailbox's domain on.
	ids, err := sendableIdentities(ctx, c)
	if err != nil {
		return "", err
	}
	want := strings.ToLower(strings.TrimSpace(flag))
	for _, id := range ids {
		if id.Primary && strings.ToLower(id.Address) == want {
			return id.MailboxID, nil
		}
	}
	return "", fmt.Errorf("no mailbox %q among your memberships", flag)
}

// mailboxAddresses maps the user's mailbox ids to their full addresses, for
// display columns. The stored address is a local part, so the domain has to be
// joined on — see sendableIdentities.
func mailboxAddresses(ctx context.Context, c *client.Client) (map[string]string, error) {
	ids, err := sendableIdentities(ctx, c)
	if err != nil {
		return nil, err
	}
	addr := map[string]string{}
	for _, id := range ids {
		if id.Primary {
			addr[id.MailboxID] = id.Address
		}
	}
	return addr, nil
}
