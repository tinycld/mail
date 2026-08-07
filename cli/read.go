package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jaytaylor/html2text"
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/packages/mail/api"
)

// message is the mail_messages row shape the CLI reads. BodyHTML, RawHeaders,
// and Attachments hold PocketBase's stored file names — content comes from a
// second, file-URL fetch, exactly like the app's EmailBody.
type message struct {
	ID             string   `json:"id"`
	Thread         string   `json:"thread"`
	MessageID      string   `json:"message_id"`
	SenderName     string   `json:"sender_name"`
	SenderEmail    string   `json:"sender_email"`
	RecipientsTo   any      `json:"recipients_to"`
	RecipientsCc   any      `json:"recipients_cc"`
	RecipientsBcc  any      `json:"recipients_bcc"`
	Date           string   `json:"date"`
	Subject        string   `json:"subject"`
	Snippet        string   `json:"snippet"`
	HasAttach      bool     `json:"has_attachments"`
	BodyHTML       string   `json:"body_html"`
	RawHeaders     string   `json:"raw_headers"`
	Attachments    []string `json:"attachments"`
	Alias          string   `json:"alias"`
	DeliveryStatus string   `json:"delivery_status"`
}

func newReadCmd(c *client.Client) *cobra.Command {
	var asHTML, asRaw, noMark bool
	cmd := &cobra.Command{
		Use:   "read <message|thread>",
		Short: "Print a message, or every message in a thread",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			msgs, err := resolveMessages(ctx, c, args[0])
			if err != nil {
				return err
			}
			if o.Format == output.JSON {
				if err := o.Write(cmd.OutOrStdout(), nil, nil, msgs); err != nil {
					return err
				}
			} else if asRaw {
				for _, m := range msgs {
					if err := printRawHeaders(ctx, c, cmd.OutOrStdout(), cmd.ErrOrStderr(), m); err != nil {
						return err
					}
				}
			} else {
				for i, m := range msgs {
					if i > 0 {
						fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("─", 60))
					}
					if err := printMessage(ctx, c, cmd.OutOrStdout(), m, asHTML); err != nil {
						return err
					}
				}
			}

			if !noMark {
				markThreadRead(ctx, c, msgs[0].Thread, o, cmd.ErrOrStderr())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asHTML, "html", false, "print the raw HTML body")
	cmd.Flags().BoolVar(&asRaw, "raw", false, "print the stored RFC 5322 headers instead of the body")
	cmd.Flags().BoolVar(&noMark, "no-mark", false, "leave the thread's unread state alone")
	return cmd
}

// markThreadRead mirrors the app, which marks a thread read when you open it.
// Failures are reported but never fail the command: the mail was still printed,
// and a script piping output should not see a non-zero exit for a bookkeeping
// write. Pass --no-mark to skip it entirely.
func markThreadRead(ctx context.Context, c *client.Client, threadID string, o output.Options, warn io.Writer) {
	userID, err := c.UserID(ctx)
	if err != nil {
		o.Info(warn, "could not mark the thread read: %v", err)
		return
	}
	state, err := threadStateFor(ctx, c, threadID, userID)
	if err != nil {
		o.Info(warn, "could not mark the thread read: %v", err)
		return
	}
	if state.IsRead {
		return
	}
	if _, err := client.UpdateRecord[threadState](ctx, c, "mail_thread_state", state.ID,
		map[string]any{"is_read": true}); err != nil {
		o.Info(warn, "could not mark the thread read: %v", err)
	}
}

// printRawHeaders prints the stored RFC 5322 headers. Only messages that
// arrived through IMAP APPEND have them — everything sent, received by webhook,
// or fetched over IMAP stores none, so say so rather than printing nothing.
func printRawHeaders(ctx context.Context, c *client.Client, w, warn io.Writer, m message) error {
	if m.RawHeaders == "" {
		fmt.Fprintf(warn, "%s: no stored raw headers (only messages appended over IMAP carry them)\n", m.ID)
		return nil
	}
	body, _, err := c.Get(ctx, client.FileURL("mail_messages", m.ID, m.RawHeaders))
	if err != nil {
		return fmt.Errorf("fetching headers of %s: %w", m.ID, err)
	}
	defer body.Close()
	if _, err := io.Copy(w, body); err != nil {
		return err
	}
	fmt.Fprintln(w)
	return nil
}

// resolveMessages accepts either a message id (one message) or a thread id
// (all its messages, oldest first — the same order the thread screen renders).
func resolveMessages(ctx context.Context, c *client.Client, id string) ([]message, error) {
	if m, err := client.GetRecord[message](ctx, c, "mail_messages", id); err == nil {
		return []message{m}, nil
	}
	msgs, err := client.ListAll[message](ctx, c, "mail_messages",
		client.Filter("thread = {:t}", map[string]any{"t": id}), "date")
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("%s: no message or thread found", id)
	}
	return msgs, nil
}

func printMessage(ctx context.Context, c *client.Client, w io.Writer, m message, asHTML bool) error {
	fmt.Fprintf(w, "From: %s\n", formatAddress(m.SenderName, m.SenderEmail))
	if to := recipientList(m.RecipientsTo); to != "" {
		fmt.Fprintf(w, "To: %s\n", to)
	}
	if cc := recipientList(m.RecipientsCc); cc != "" {
		fmt.Fprintf(w, "Cc: %s\n", cc)
	}
	fmt.Fprintf(w, "Date: %s\nSubject: %s\n", m.Date, m.Subject)
	if len(m.Attachments) > 0 {
		fmt.Fprintf(w, "Attachments: %d (see `tinycld mail attachments %s`)\n", len(m.Attachments), m.ID)
	}
	fmt.Fprintln(w)

	body, err := fetchBody(ctx, c, m)
	if err != nil {
		return err
	}
	if body == "" {
		// No stored body (e.g. an empty message) — the snippet is all there is.
		fmt.Fprintln(w, m.Snippet)
		return nil
	}
	if asHTML {
		fmt.Fprintln(w, body)
		return nil
	}
	text, err := html2text.FromString(body, html2text.Options{TextOnly: false})
	if err != nil {
		// Unparseable HTML: show it raw rather than nothing.
		fmt.Fprintln(w, body)
		return nil
	}
	fmt.Fprintln(w, text)
	return nil
}

// fetchBody is the second step of the two-step body read: the record stores
// only the file name; the bytes come from the file URL.
func fetchBody(ctx context.Context, c *client.Client, m message) (string, error) {
	if m.BodyHTML == "" {
		return "", nil
	}
	body, _, err := c.Get(ctx, client.FileURL("mail_messages", m.ID, m.BodyHTML))
	if err != nil {
		return "", fmt.Errorf("fetching body of %s: %w", m.ID, err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func formatAddress(name, email string) string {
	if name == "" {
		return email
	}
	return fmt.Sprintf("%s <%s>", name, email)
}

// parsedRecipients decodes a recipients_to/_cc JSON column into the same
// Recipient shape a send request carries.
func parsedRecipients(v any) []api.Recipient {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var people []api.Recipient
	if err := json.Unmarshal(data, &people); err != nil {
		return nil
	}
	return people
}

// recipientAddresses is just the addresses, for From-identity matching.
func recipientAddresses(v any) []string {
	people := parsedRecipients(v)
	out := make([]string, 0, len(people))
	for _, p := range people {
		if p.Email != "" {
			out = append(out, p.Email)
		}
	}
	return out
}

func recipientList(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var people []struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(data, &people); err != nil {
		return ""
	}
	parts := make([]string, 0, len(people))
	for _, p := range people {
		parts = append(parts, formatAddress(p.Name, p.Email))
	}
	return strings.Join(parts, ", ")
}
