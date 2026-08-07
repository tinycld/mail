package cli

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/packages/mail/api"
)

func newSendCmd(c *client.Client) *cobra.Command {
	var to, cc, bcc, attach []string
	var subject, body, bodyFile, mailboxFlag string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if len(to) == 0 {
				return fmt.Errorf("at least one --to recipient is required")
			}
			text, err := readBody(cmd.InOrStdin(), body, bodyFile)
			if err != nil {
				return err
			}
			mailboxID, err := resolveMailbox(ctx, c, mailboxFlag)
			if err != nil {
				return err
			}

			req := api.SendEmailRequest{
				MailboxID: mailboxID,
				To:        parseRecipients(to),
				Cc:        parseRecipients(cc),
				Bcc:       parseRecipients(bcc),
				Subject:   subject,
				TextBody:  text,
				// The web composer always stores an HTML body; mirror it with
				// a minimal escaped rendering so the message displays there.
				HTMLBody: textToHTML(text),
			}

			var resp api.SendEmailResponse
			if len(attach) == 0 {
				if err := c.PostJSON(ctx, "/api/mail/send", req, &resp); err != nil {
					return err
				}
			} else {
				parts := make([]client.FilePart, len(attach))
				for i, p := range attach {
					if _, err := os.Stat(p); err != nil {
						return err
					}
					parts[i] = client.FilePart{
						Field: api.MultipartFieldAttachments,
						Name:  filepath.Base(p),
						Path:  p,
					}
				}
				resp, err = client.PostMultipart[api.SendEmailResponse](ctx, c,
					"/api/mail/send", api.MultipartFieldJSON, req, parts, nil)
				if err != nil {
					return err
				}
			}
			o.Info(cmd.ErrOrStderr(), "sent (message %s, thread %s)", resp.MessageID, resp.ThreadID)
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
	fl.StringVar(&mailboxFlag, "mailbox", "", "send from this mailbox (id or address); default your first")
	return cmd
}

func readBody(stdin io.Reader, body, bodyFile string) (string, error) {
	switch {
	case body != "" && bodyFile != "":
		return "", fmt.Errorf("--body and --body-file are mutually exclusive")
	case body != "":
		return body, nil
	case bodyFile == "-":
		data, err := io.ReadAll(stdin)
		return string(data), err
	case bodyFile != "":
		data, err := os.ReadFile(bodyFile)
		return string(data), err
	default:
		return "", fmt.Errorf("provide --body or --body-file")
	}
}

// parseRecipients accepts "Name <addr@host>" or a bare address.
func parseRecipients(values []string) []api.Recipient {
	if len(values) == 0 {
		return nil
	}
	out := make([]api.Recipient, len(values))
	for i, v := range values {
		v = strings.TrimSpace(v)
		if open := strings.LastIndex(v, "<"); open != -1 && strings.HasSuffix(v, ">") {
			out[i] = api.Recipient{
				Name:  strings.TrimSpace(v[:open]),
				Email: strings.TrimSpace(v[open+1 : len(v)-1]),
			}
			continue
		}
		out[i] = api.Recipient{Email: v}
	}
	return out
}

// textToHTML renders plain text as minimal HTML: escaped, newlines as <br>.
func textToHTML(text string) string {
	escaped := html.EscapeString(text)
	return "<div>" + strings.ReplaceAll(escaped, "\n", "<br>") + "</div>"
}
