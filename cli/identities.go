package cli

import (
	"context"
	"fmt"
	"strings"

	"tinycld.org/cli/client"
)

// domain is the mail_domains row shape; only the domain name is needed to
// build a full address from a mailbox or alias local part.
type domain struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

// alias is a mail_mailbox_aliases row. `address` is the LOCAL PART only — the
// full address is address@<the mailbox's domain>.
type alias struct {
	ID      string `json:"id"`
	Mailbox string `json:"mailbox"`
	Address string `json:"address"`
}

// identity is one address the caller can send from: a mailbox's own address,
// or one of its aliases. AliasID is empty for the mailbox's primary address.
type identity struct {
	MailboxID   string `json:"mailbox_id"`
	AliasID     string `json:"alias_id,omitempty"`
	Address     string `json:"address"`
	DisplayName string `json:"display_name"`
	Primary     bool   `json:"primary"`
}

// sendableIdentities enumerates every address the caller can send from,
// mailboxes in membership order with each mailbox's primary address first.
// Mirrors the app's useSendableIdentities/groupSendableIdentities.
func sendableIdentities(ctx context.Context, c *client.Client) ([]identity, error) {
	members, err := userMemberships(ctx, c)
	if err != nil {
		return nil, err
	}
	var out []identity
	for _, m := range members {
		mb, err := client.GetRecord[mailboxWithDomain](ctx, c, "mail_mailboxes", m.Mailbox)
		if err != nil {
			return nil, err
		}
		dom, err := client.GetRecord[domain](ctx, c, "mail_domains", mb.Domain)
		if err != nil {
			return nil, err
		}
		out = append(out, identity{
			MailboxID: mb.ID, Address: mb.Address + "@" + dom.Domain,
			DisplayName: mb.DisplayName, Primary: true,
		})
		aliases, err := client.ListAll[alias](ctx, c, "mail_mailbox_aliases",
			client.Filter("mailbox = {:m}", map[string]any{"m": mb.ID}), "address")
		if err != nil {
			return nil, err
		}
		for _, a := range aliases {
			out = append(out, identity{
				MailboxID: mb.ID, AliasID: a.ID,
				Address: a.Address + "@" + dom.Domain, DisplayName: mb.DisplayName,
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("you are not a member of any mailbox")
	}
	return out, nil
}

// mailboxWithDomain extends the mailbox row with its domain relation, needed
// to build full addresses.
type mailboxWithDomain struct {
	ID          string `json:"id"`
	Address     string `json:"address"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	Domain      string `json:"domain"`
}

// resolveFrom turns a --from value into the mailbox and alias ids a send needs.
// Accepts a full address, and (for convenience) a mailbox id.
func resolveFrom(ctx context.Context, c *client.Client, from string) (mailboxID, aliasID string, err error) {
	id, err := resolveFromIdentity(ctx, c, from)
	if err != nil {
		return "", "", err
	}
	return id.MailboxID, id.AliasID, nil
}

// resolveFromIdentity is resolveFrom's whole answer, including Address.
//
// A caller that only needs the two ids should use resolveFrom; a caller that
// needs to know WHICH ADDRESS it is sending as needs this. `reply --all` is the
// motivating case: it drops the caller's own address from the recipient list by
// comparing against identity.Address, so being handed an identity with that
// field empty silently made the caller mail themselves.
func resolveFromIdentity(ctx context.Context, c *client.Client, from string) (identity, error) {
	ids, err := sendableIdentities(ctx, c)
	if err != nil {
		return identity{}, err
	}
	if from == "" {
		return ids[0], nil
	}
	want := strings.ToLower(strings.TrimSpace(from))
	for _, id := range ids {
		if strings.ToLower(id.Address) == want || id.MailboxID == from {
			return id, nil
		}
	}
	addrs := make([]string, len(ids))
	for i, id := range ids {
		addrs[i] = id.Address
	}
	return identity{}, fmt.Errorf("--from %q is not one of your addresses (%s)",
		from, strings.Join(addrs, ", "))
}

// replyIdentity picks the identity to answer from, preferring one the original
// was addressed to. Mirrors the app's pickDefaultFrom: primary addresses are
// matched across all mailboxes before any alias is considered.
func replyIdentity(ctx context.Context, c *client.Client, addressedTo []string) (identity, error) {
	ids, err := sendableIdentities(ctx, c)
	if err != nil {
		return identity{}, err
	}
	seen := map[string]bool{}
	for _, a := range addressedTo {
		seen[strings.ToLower(bareAddress(a))] = true
	}
	for _, id := range ids {
		if id.Primary && seen[strings.ToLower(id.Address)] {
			return id, nil
		}
	}
	for _, id := range ids {
		if !id.Primary && seen[strings.ToLower(id.Address)] {
			return id, nil
		}
	}
	return ids[0], nil
}

// bareAddress strips a display name: `Ada <ada@x>` -> `ada@x`.
func bareAddress(s string) string {
	s = strings.TrimSpace(s)
	if open := strings.LastIndex(s, "<"); open != -1 && strings.HasSuffix(s, ">") {
		return strings.TrimSpace(s[open+1 : len(s)-1])
	}
	return s
}
