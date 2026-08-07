package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
)

// folders are the mail_thread_state select values. `starred`, `all`, and
// `all-inboxes` are list-filter modes, not folders, so they are not movable
// targets.
var folders = []string{"inbox", "sent", "drafts", "trash", "spam", "archive"}

// stateVerbs are the one-field mutations, each mirroring an app toolbar action.
func newStateCmds(c *client.Client) []*cobra.Command {
	simple := []struct {
		use, short, field string
		value             any
	}{
		{"archive", "Move threads to Archive", "folder", "archive"},
		{"trash", "Move threads to Trash", "folder", "trash"},
		{"spam", "Move threads to Spam", "folder", "spam"},
		{"star", "Star threads", "is_starred", true},
		{"unstar", "Unstar threads", "is_starred", false},
	}
	cmds := make([]*cobra.Command, 0, len(simple)+2)
	for _, s := range simple {
		s := s
		cmds = append(cmds, &cobra.Command{
			Use:   s.use + " <thread...>",
			Short: s.short,
			Args:  cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return applyState(cmd, c, args, map[string]any{s.field: s.value}, s.use)
			},
		})
	}

	cmds = append(cmds, &cobra.Command{
		Use:       "move <folder> <thread...>",
		Short:     "Move threads to a folder",
		Args:      cobra.MinimumNArgs(2),
		ValidArgs: folders,
		RunE: func(cmd *cobra.Command, args []string) error {
			folder := args[0]
			if !validFolder(folder) {
				return fmt.Errorf("unknown folder %q (one of: %v)", folder, folders)
			}
			return applyState(cmd, c, args[1:], map[string]any{"folder": folder}, "moved to "+folder)
		},
	}, &cobra.Command{
		Use:       "mark <read|unread> <thread...>",
		Short:     "Mark threads read or unread",
		Args:      cobra.MinimumNArgs(2),
		ValidArgs: []string{"read", "unread"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "read":
				return applyState(cmd, c, args[1:], map[string]any{"is_read": true}, "marked read")
			case "unread":
				return applyState(cmd, c, args[1:], map[string]any{"is_read": false}, "marked unread")
			}
			return fmt.Errorf("mark takes read or unread, got %q", args[0])
		},
	})
	return cmds
}

// applyState patches one field on the CALLER's state row for each thread. State
// is per-user: archiving a thread in a shared mailbox does not affect
// co-members.
func applyState(cmd *cobra.Command, c *client.Client, threads []string, patch map[string]any, verb string) error {
	o, _, err := output.FromCommand(cmd)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	userID, err := c.UserID(ctx)
	if err != nil {
		return err
	}
	for _, threadID := range threads {
		state, err := threadStateFor(ctx, c, threadID, userID)
		if err != nil {
			return err
		}
		if _, err := client.UpdateRecord[threadState](ctx, c, "mail_thread_state", state.ID, patch); err != nil {
			return err
		}
		o.Info(cmd.ErrOrStderr(), "%s: %s", threadID, verb)
	}
	return nil
}

// threadStateFor finds the caller's state row for a thread. The row is created
// server-side when mail arrives or is sent, so a missing row means the thread
// is not in the caller's mail at all.
func threadStateFor(ctx context.Context, c *client.Client, threadID, userID string) (threadState, error) {
	res, err := client.ListRecords[threadState](ctx, c, "mail_thread_state", client.ListOptions{
		Filter: client.Filter("thread = {:t} && user = {:u}",
			map[string]any{"t": threadID, "u": userID}),
		PerPage:   1,
		SkipTotal: true,
	})
	if err != nil {
		return threadState{}, err
	}
	if len(res.Items) == 0 {
		return threadState{}, fmt.Errorf("%s: not one of your threads", threadID)
	}
	return res.Items[0], nil
}

func validFolder(f string) bool {
	for _, v := range folders {
		if v == f {
			return true
		}
	}
	return false
}
