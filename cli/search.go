package cli

import (
	"net/url"
	"regexp"
	"strconv"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/packages/mail/api"
)

// searchFlags mirrors api.SearchRequest field-for-field; searchQuery encodes
// it under the struct's json tags. A test pins the flag set against the
// struct so a contract change here is a compile/test failure, not drift.
type searchFlags struct {
	mailbox       string
	limit, offset int
	from, to      string
	subject       string
	hasWords      string
	dateAfter     string
	dateBefore    string
	folder        string
	hasAttachment bool
}

func newSearchCmd(c *client.Client) *cobra.Command {
	var f searchFlags
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search messages (FTS + structured filters)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			q := ""
			if len(args) == 1 {
				q = args[0]
			}
			mailboxID := ""
			if f.mailbox != "" {
				mailboxID, err = resolveMailbox(ctx, c, f.mailbox)
				if err != nil {
					return err
				}
			}
			req := api.SearchRequest{
				Query:         q,
				MailboxID:     mailboxID,
				Limit:         f.limit,
				Offset:        f.offset,
				From:          f.from,
				To:            f.to,
				Subject:       f.subject,
				HasWords:      f.hasWords,
				DateAfter:     f.dateAfter,
				DateBefore:    f.dateBefore,
				Folder:        f.folder,
				HasAttachment: f.hasAttachment,
			}
			var resp api.SearchResponse
			if err := c.GetJSON(ctx, "/api/mail/search?"+searchQuery(req).Encode(), &resp); err != nil {
				return err
			}
			rows := make([][]string, len(resp.Items))
			for i, it := range resp.Items {
				rows[i] = []string{
					it.LatestDate, stripMarks(it.SubjectHighlight),
					stripMarks(it.SnippetHighlight), strconv.Itoa(it.MessageCount), it.ThreadID,
				}
			}
			if err := o.Write(cmd.OutOrStdout(),
				[]string{"DATE", "SUBJECT", "SNIPPET", "MSGS", "THREAD"}, rows, resp); err != nil {
				return err
			}
			o.Info(cmd.ErrOrStderr(), "%d of %d result(s)", len(resp.Items), resp.Total)
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.mailbox, "mailbox", "", "restrict to one mailbox (id or address)")
	fl.IntVar(&f.limit, "limit", 0, "maximum results")
	fl.IntVar(&f.offset, "offset", 0, "skip this many results")
	fl.StringVar(&f.from, "from", "", "sender contains")
	fl.StringVar(&f.to, "to", "", "recipient contains")
	fl.StringVar(&f.subject, "subject", "", "subject contains")
	fl.StringVar(&f.hasWords, "has-words", "", "body contains words")
	fl.StringVar(&f.dateAfter, "date-after", "", "only messages after (YYYY-MM-DD)")
	fl.StringVar(&f.dateBefore, "date-before", "", "only messages before (YYYY-MM-DD)")
	fl.StringVar(&f.folder, "folder", "", "restrict to a folder (inbox, sent, ...)")
	fl.BoolVar(&f.hasAttachment, "has-attachment", false, "only messages with attachments")
	return cmd
}

// searchQuery encodes the request under api.SearchRequest's json tags — the
// query-string names ARE the contract (the server binds them by tag).
func searchQuery(r api.SearchRequest) url.Values {
	q := url.Values{}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	set("q", r.Query)
	set("mailbox_id", r.MailboxID)
	if r.Limit > 0 {
		q.Set("limit", strconv.Itoa(r.Limit))
	}
	if r.Offset > 0 {
		q.Set("offset", strconv.Itoa(r.Offset))
	}
	set("from", r.From)
	set("to", r.To)
	set("subject", r.Subject)
	set("has_words", r.HasWords)
	set("date_after", r.DateAfter)
	set("date_before", r.DateBefore)
	set("folder", r.Folder)
	if r.HasAttachment {
		q.Set("has_attachment", "true")
	}
	return q
}

var markTags = regexp.MustCompile(`</?mark>`)

func stripMarks(s string) string {
	return markTags.ReplaceAllString(s, "")
}
