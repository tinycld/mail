package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jaytaylor/html2text"
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/packages/mail/api"
)

func newDraftCmd(c *client.Client) *cobra.Command {
	var to, cc, bcc, attach []string
	var subject, body, bodyFile, mailboxFlag, from, messageID string
	cmd := &cobra.Command{
		Use:   "draft",
		Short: "Save a draft, or update one with --message-id",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			text := ""
			if body != "" || bodyFile != "" {
				text, err = readBody(cmd.InOrStdin(), body, bodyFile)
				if err != nil {
					return err
				}
			}
			mailboxID, aliasID, err := resolveSender(ctx, c, from, mailboxFlag)
			if err != nil {
				return err
			}

			req := api.SaveDraftRequest{
				MailboxID: mailboxID,
				AliasID:   aliasID,
				MessageID: messageID,
				To:        parseRecipients(to),
				Cc:        parseRecipients(cc),
				Bcc:       parseRecipients(bcc),
				Subject:   subject,
				TextBody:  text,
				HTMLBody:  textToHTML(text),
			}
			// The save endpoint replaces the whole draft rather than patching
			// it, so an update has to resend the fields the user didn't name —
			// otherwise `--subject x` alone silently drops the recipients and
			// body already on the draft.
			if messageID != "" {
				if err := fillUnsetDraftFields(ctx, c, cmd, &req); err != nil {
					return err
				}
			}
			resp, err := postDraft(ctx, c, req, attach)
			if err != nil {
				return err
			}
			verb := "saved"
			if messageID != "" {
				verb = "updated"
			}
			o.Info(cmd.ErrOrStderr(), "%s draft %s", verb, resp.MessageID)
			if o.Format != output.Table {
				return o.Write(cmd.OutOrStdout(), nil, nil, resp)
			}
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringArrayVar(&to, "to", nil, `recipient ("Name <a@b>" or bare address); repeatable`)
	fl.StringArrayVar(&cc, "cc", nil, "carbon copy; repeatable")
	fl.StringArrayVar(&bcc, "bcc", nil, "blind carbon copy; repeatable")
	fl.StringVar(&subject, "subject", "", "subject line")
	fl.StringVar(&body, "body", "", "message text")
	fl.StringVar(&bodyFile, "body-file", "", "read the message text from a file (- for stdin)")
	fl.StringArrayVar(&attach, "attach", nil, "attach a local file; repeatable")
	fl.StringVar(&mailboxFlag, "mailbox", "", "mailbox to draft in (id or address); default your first")
	fl.StringVar(&from, "from", "", "draft from this full address, including an alias")
	fl.StringVar(&messageID, "message-id", "", "update this existing draft instead of creating one")

	cmd.AddCommand(newDraftSendCmd(c))
	return cmd
}

func postDraft(ctx context.Context, c *client.Client, req api.SaveDraftRequest, attach []string) (api.SendEmailResponse, error) {
	var resp api.SendEmailResponse
	if len(attach) == 0 {
		err := c.PostJSON(ctx, "/api/mail/draft", req, &resp)
		return resp, err
	}
	parts, err := fileParts(attach)
	if err != nil {
		return resp, err
	}
	return client.PostMultipart[api.SendEmailResponse](ctx, c,
		"/api/mail/draft", api.MultipartFieldJSON, req, parts, nil)
}

func newDraftSendCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "send <message|thread>",
		Short: "Send a saved draft",
		Long: "Sends a draft and removes the saved copy. There is no server-side " +
			"send-draft operation, so this sends the draft's contents as a new " +
			"message (re-uploading its attachments) and then deletes the draft, " +
			"which is exactly what the app does.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			draft, err := resolveDraft(ctx, c, args[0])
			if err != nil {
				return err
			}
			th, err := client.GetRecord[thread](ctx, c, "mail_threads", draft.Thread)
			if err != nil {
				return err
			}

			text, err := draftText(ctx, c, draft)
			if err != nil {
				return err
			}
			// Attachments live on the draft record, and the send endpoint takes
			// files only as uploads — so they round-trip through temp files.
			paths, cleanup, err := downloadAttachments(ctx, c, draft)
			defer cleanup()
			if err != nil {
				return err
			}

			req := api.SendEmailRequest{
				MailboxID: th.Mailbox,
				AliasID:   draft.Alias,
				To:        parsedRecipients(draft.RecipientsTo),
				Cc:        parsedRecipients(draft.RecipientsCc),
				Bcc:       parsedRecipients(draft.RecipientsBcc),
				Subject:   draft.Subject,
				TextBody:  text,
				HTMLBody:  textToHTML(text),
			}
			if len(req.To) == 0 {
				return fmt.Errorf("%s: the draft has no recipients", draft.ID)
			}
			resp, err := postSend(ctx, c, req, paths)
			if err != nil {
				return err
			}
			// The sent copy and the draft share a thread, so a failed delete
			// leaves a confusing duplicate — say so loudly rather than warn.
			if err := client.DeleteRecord(ctx, c, "mail_messages", draft.ID); err != nil {
				return fmt.Errorf(
					"sent as %s, but the draft %s could not be deleted (delete it in the app): %w",
					resp.MessageID, draft.ID, err)
			}
			o.Info(cmd.ErrOrStderr(), "sent (message %s, thread %s); draft removed",
				resp.MessageID, resp.ThreadID)
			if o.Format != output.Table {
				return o.Write(cmd.OutOrStdout(), nil, nil, resp)
			}
			return nil
		},
	}
}

// resolveDraft accepts a draft message id or a thread id, and refuses anything
// that is not actually a draft — the draft endpoint does not check, so a typo
// could otherwise rewrite or resend a sent message.
func resolveDraft(ctx context.Context, c *client.Client, ref string) (message, error) {
	if m, err := client.GetRecord[message](ctx, c, "mail_messages", ref); err == nil {
		if m.DeliveryStatus != "draft" {
			return message{}, fmt.Errorf("%s is not a draft (delivery status %q)", ref, m.DeliveryStatus)
		}
		return m, nil
	}
	msgs, err := client.ListAll[message](ctx, c, "mail_messages",
		client.Filter(`thread = {:t} && delivery_status = "draft"`, map[string]any{"t": ref}), "-date")
	if err != nil {
		return message{}, err
	}
	if len(msgs) == 0 {
		return message{}, fmt.Errorf("%s: no draft found", ref)
	}
	return msgs[0], nil
}

// fillUnsetDraftFields merges the stored draft into an update request for every
// field the user did not name on the command line.
//
// Flags.Changed rather than emptiness is the point: `--subject ""` must be able
// to clear the subject, while an unmentioned --subject must keep what the draft
// already has. Attachments are not touched here — they live on the record and
// the save endpoint leaves them alone unless new files are uploaded.
func fillUnsetDraftFields(
	ctx context.Context,
	c *client.Client,
	cmd *cobra.Command,
	req *api.SaveDraftRequest,
) error {
	existing, err := resolveDraft(ctx, c, req.MessageID)
	if err != nil {
		return err
	}
	fl := cmd.Flags()
	if !fl.Changed("to") {
		req.To = parsedRecipients(existing.RecipientsTo)
	}
	if !fl.Changed("cc") {
		req.Cc = parsedRecipients(existing.RecipientsCc)
	}
	if !fl.Changed("bcc") {
		req.Bcc = parsedRecipients(existing.RecipientsBcc)
	}
	if !fl.Changed("subject") {
		req.Subject = existing.Subject
	}
	// The body needs no merge: the save endpoint keeps the stored body when
	// TextBody is empty, which is exactly the "user didn't mention it" case.
	// The flip side is that `--body ""` cannot blank a body through this
	// endpoint — say so rather than reporting a no-op as an update.
	if (fl.Changed("body") || fl.Changed("body-file")) && req.TextBody == "" {
		return fmt.Errorf("--body \"\" cannot clear a draft's body; edit it in the app")
	}
	return nil
}

// draftText recovers the draft's plain text from its stored HTML body.
func draftText(ctx context.Context, c *client.Client, draft message) (string, error) {
	body, err := fetchBody(ctx, c, draft)
	if err != nil {
		return "", err
	}
	if body == "" {
		return draft.Snippet, nil
	}
	text, err := html2text.FromString(body, html2text.Options{TextOnly: false})
	if err != nil {
		return body, nil
	}
	return text, nil
}

// downloadAttachments saves the draft's attachments to temp files so they can
// be re-uploaded with the send. The returned cleanup always runs.
func downloadAttachments(ctx context.Context, c *client.Client, draft message) ([]string, func(), error) {
	if len(draft.Attachments) == 0 {
		return nil, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "tinycld-draft-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	paths := make([]string, 0, len(draft.Attachments))
	for _, stored := range draft.Attachments {
		dest := filepath.Join(dir, displayName(stored))
		err := client.DownloadToFile(ctx, c,
			client.FileURL("mail_messages", draft.ID, stored), dest, nil)
		if err != nil {
			return nil, cleanup, fmt.Errorf("fetching attachment %s: %w", displayName(stored), err)
		}
		paths = append(paths, dest)
	}
	return paths, cleanup, nil
}
