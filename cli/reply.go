package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/packages/mail/api"
)

func newReplyCmd(c *client.Client) *cobra.Command {
	var body, bodyFile, from string
	var attach []string
	var all bool
	cmd := &cobra.Command{
		Use:   "reply <message|thread>",
		Short: "Reply to a message, or to a thread's latest message",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			text, err := readBody(cmd.InOrStdin(), body, bodyFile)
			if err != nil {
				return err
			}
			msgs, err := resolveMessages(ctx, c, args[0])
			if err != nil {
				return err
			}
			// A thread argument answers its most recent message, as the app's
			// thread-level reply does.
			target := msgs[len(msgs)-1]

			ident, err := replyFromIdentity(ctx, c, target, from)
			if err != nil {
				return err
			}
			recipients := replyRecipients(target, ident, all)
			if len(recipients) == 0 {
				return fmt.Errorf("no one to reply to (the message has no sender or recipients)")
			}

			req := api.SendEmailRequest{
				MailboxID:          ident.MailboxID,
				AliasID:            ident.AliasID,
				To:                 recipients,
				Subject:            replySubject(target.Subject),
				TextBody:           text,
				HTMLBody:           textToHTML(text),
				InReplyToMessageID: target.ID,
			}
			resp, err := postSend(ctx, c, req, attach)
			if err != nil {
				return err
			}
			o.Info(cmd.ErrOrStderr(), "replied to %d recipient(s) (message %s, thread %s)",
				len(recipients), resp.MessageID, resp.ThreadID)
			if o.Format != output.Table {
				return o.Write(cmd.OutOrStdout(), nil, nil, resp)
			}
			return nil
		},
	}
	fl := cmd.Flags()
	fl.BoolVar(&all, "all", false, "reply to everyone on the message, not just the sender")
	fl.StringVar(&body, "body", "", "message text")
	fl.StringVar(&bodyFile, "body-file", "", "read the message text from a file (- for stdin)")
	fl.StringArrayVar(&attach, "attach", nil, "attach a local file; repeatable")
	fl.StringVar(&from, "from", "", "send from this address (see `tinycld mail mailboxes`)")
	return cmd
}

// replyFromIdentity honors an explicit --from, else picks the identity the
// original was addressed to.
func replyFromIdentity(ctx context.Context, c *client.Client, target message, from string) (identity, error) {
	if from != "" {
		mailboxID, aliasID, err := resolveFrom(ctx, c, from)
		if err != nil {
			return identity{}, err
		}
		return identity{MailboxID: mailboxID, AliasID: aliasID}, nil
	}
	addressed := append(recipientAddresses(target.RecipientsTo), recipientAddresses(target.RecipientsCc)...)
	return replyIdentity(ctx, c, addressed)
}

// replySubject prefixes "Re: " unless it is already there — same idempotent,
// case-sensitive check the app's compose form makes.
func replySubject(subject string) string {
	if strings.HasPrefix(subject, "Re:") {
		return subject
	}
	return "Re: " + subject
}

// replyRecipients returns the sender alone, or (with --all) the sender plus
// everyone in To and Cc minus the caller's own addresses. Everything goes in
// To, matching the app — it never re-populates Cc on a reply.
func replyRecipients(target message, ident identity, all bool) []api.Recipient {
	sender := api.Recipient{Name: target.SenderName, Email: target.SenderEmail}
	if !all {
		if sender.Email == "" {
			return nil
		}
		return []api.Recipient{sender}
	}
	var candidates []api.Recipient
	if sender.Email != "" {
		candidates = append(candidates, sender)
	}
	candidates = append(candidates, parsedRecipients(target.RecipientsTo)...)
	candidates = append(candidates, parsedRecipients(target.RecipientsCc)...)

	mine := strings.ToLower(ident.Address)
	seen := map[string]bool{}
	var out []api.Recipient
	for _, r := range candidates {
		key := strings.ToLower(r.Email)
		if key == "" || key == mine || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}
