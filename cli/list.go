package cli

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
)

// thread is the mail_threads row shape the CLI reads.
type thread struct {
	ID           string `json:"id"`
	Mailbox      string `json:"mailbox"`
	Subject      string `json:"subject"`
	Snippet      string `json:"snippet"`
	MessageCount int    `json:"message_count"`
	LatestDate   string `json:"latest_date"`
	Participants any    `json:"participants"`
}

// threadState is the per-user mail_thread_state row.
type threadState struct {
	ID        string `json:"id"`
	Thread    string `json:"thread"`
	User      string `json:"user"`
	Folder    string `json:"folder"`
	IsRead    bool   `json:"is_read"`
	IsStarred bool   `json:"is_starred"`
}

// listedThread is the JSON row `mail list` emits: the thread joined with the
// caller's state flags.
type listedThread struct {
	thread
	IsRead    bool `json:"is_read"`
	IsStarred bool `json:"is_starred"`
}

func newListCmd(c *client.Client) *cobra.Command {
	var folder, mailboxFlag string
	var limit, page int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List threads in a folder",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			userID, err := c.UserID(ctx)
			if err != nil {
				return err
			}
			mailboxIDs, err := listMailboxScope(ctx, c, mailboxFlag)
			if err != nil {
				return err
			}

			threads, err := client.ListRecords[thread](ctx, c, "mail_threads", client.ListOptions{
				Filter:  threadsFilter(mailboxIDs, userID, folder),
				Sort:    "-latest_date",
				Page:    page,
				PerPage: limit,
			})
			if err != nil {
				return err
			}
			states, err := statesForThreads(ctx, c, userID, threads.Items)
			if err != nil {
				return err
			}

			listed := make([]listedThread, len(threads.Items))
			rows := make([][]string, len(threads.Items))
			for i, th := range threads.Items {
				st := states[th.ID]
				listed[i] = listedThread{thread: th, IsRead: st.IsRead, IsStarred: st.IsStarred}
				rows[i] = []string{
					flags(st), th.LatestDate, participantsCell(th.Participants),
					th.Subject, th.ID,
				}
			}
			if err := o.Write(cmd.OutOrStdout(),
				[]string{"FLAGS", "DATE", "PARTICIPANTS", "SUBJECT", "THREAD"}, rows, listed); err != nil {
				return err
			}
			o.Info(cmd.ErrOrStderr(), "page %d of %d (%d thread(s))",
				threads.Page, threads.TotalPages, threads.TotalItems)
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&folder, "folder", "inbox", "folder: inbox, sent, drafts, trash, spam, archive, starred, all")
	fl.StringVar(&mailboxFlag, "mailbox", "", "restrict to one mailbox (id or address); default all memberships")
	fl.IntVar(&limit, "limit", 25, "threads per page")
	fl.IntVar(&page, "page", 1, "page number")
	return cmd
}

// listMailboxScope picks the mailbox ids to list: one resolved mailbox, or
// every membership (the app's unified "all inboxes" scope).
func listMailboxScope(ctx context.Context, c *client.Client, flag string) ([]string, error) {
	if flag != "" {
		id, err := resolveMailbox(ctx, c, flag)
		if err != nil {
			return nil, err
		}
		return []string{id}, nil
	}
	members, err := userMemberships(ctx, c)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.Mailbox
	}
	return ids, nil
}

// threadsFilter mirrors the web client's buildThreadsFilter
// (useThreadListItems.ts): mailbox OR-clause, a back-relation join to the
// caller's state rows, and the folder clause with the app's starred/all
// special cases.
func threadsFilter(mailboxIDs []string, userID, folder string) string {
	var clauses []string

	boxes := make([]string, len(mailboxIDs))
	for i, id := range mailboxIDs {
		boxes[i] = client.Filter("mailbox = {:id}", map[string]any{"id": id})
	}
	if len(boxes) == 1 {
		clauses = append(clauses, boxes[0])
	} else {
		clauses = append(clauses, "("+strings.Join(boxes, " || ")+")")
	}

	clauses = append(clauses, client.Filter(
		"mail_thread_state_via_thread.user ?= {:u}", map[string]any{"u": userID}))

	switch folder {
	case "starred":
		clauses = append(clauses, "mail_thread_state_via_thread.is_starred ?= true")
	case "all", "all-inboxes":
		// No folder restriction beyond having a state row.
	default:
		clauses = append(clauses, client.Filter(
			"mail_thread_state_via_thread.folder ?= {:f}", map[string]any{"f": folder}))
	}
	return strings.Join(clauses, " && ")
}

// statesForThreads fetches the caller's state rows for one page of threads.
func statesForThreads(ctx context.Context, c *client.Client, userID string, threads []thread) (map[string]threadState, error) {
	out := map[string]threadState{}
	if len(threads) == 0 {
		return out, nil
	}
	ors := make([]string, len(threads))
	for i, th := range threads {
		ors[i] = client.Filter("thread = {:t}", map[string]any{"t": th.ID})
	}
	filter := client.Filter("user = {:u}", map[string]any{"u": userID}) +
		" && (" + strings.Join(ors, " || ") + ")"
	states, err := client.ListAll[threadState](ctx, c, "mail_thread_state", filter, "")
	if err != nil {
		return nil, err
	}
	for _, s := range states {
		out[s.Thread] = s
	}
	return out, nil
}

func flags(st threadState) string {
	f := ""
	if !st.IsRead {
		f += "●"
	}
	if st.IsStarred {
		f += "★"
	}
	return f
}

// participantsCell renders the JSON participants column ([{name,email},...])
// as a compact name list.
func participantsCell(v any) string {
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
	names := make([]string, 0, len(people))
	for _, p := range people {
		if p.Name != "" {
			names = append(names, p.Name)
			continue
		}
		names = append(names, p.Email)
	}
	const maxShown = 3
	if len(names) > maxShown {
		return strings.Join(names[:maxShown], ", ") + ", …"
	}
	return strings.Join(names, ", ")
}
