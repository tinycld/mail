package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
)

// label is a core `labels` row — labels are shared across packages (mail and
// contacts), not owned by mail.
type label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	User  string `json:"user"`
}

// labelAssignment joins a label to a record. For mail, record_id is the
// caller's mail_thread_state row id (NOT the thread id) and collection is
// "mail_thread_state".
type labelAssignment struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	RecordID   string `json:"record_id"`
	Collection string `json:"collection"`
	User       string `json:"user"`
}

const threadStateCollection = "mail_thread_state"

func newLabelsCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "labels",
		Short: "List the labels available to you",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			labels, err := visibleLabels(cmd.Context(), c)
			if err != nil {
				return err
			}
			rows := make([][]string, len(labels))
			for i, l := range labels {
				scope := "shared"
				if l.User != "" {
					scope = "yours"
				}
				rows[i] = []string{l.Name, l.Color, scope, l.ID}
			}
			return o.Write(cmd.OutOrStdout(), []string{"NAME", "COLOR", "SCOPE", "ID"}, rows, labels)
		},
	}
}

func newLabelCmd(c *client.Client) *cobra.Command {
	label := &cobra.Command{
		Use:   "label",
		Short: "Add or remove a thread's labels",
	}
	label.AddCommand(
		&cobra.Command{
			Use:   "add <thread> <label>",
			Short: "Add a label to a thread",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return changeLabel(cmd, c, args[0], args[1], true)
			},
		},
		&cobra.Command{
			Use:   "remove <thread> <label>",
			Short: "Remove a label from a thread",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return changeLabel(cmd, c, args[0], args[1], false)
			},
		},
	)
	return label
}

func changeLabel(cmd *cobra.Command, c *client.Client, threadID, labelRef string, add bool) error {
	o, _, err := output.FromCommand(cmd)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	userID, err := c.UserID(ctx)
	if err != nil {
		return err
	}
	state, err := threadStateFor(ctx, c, threadID, userID)
	if err != nil {
		return err
	}
	l, err := resolveLabel(ctx, c, labelRef)
	if err != nil {
		return err
	}

	existing, found, err := findAssignment(ctx, c, l.ID, state.ID, userID)
	if err != nil {
		return err
	}
	if add {
		// The web insert has no dedup guard, so a repeated add would leave two
		// rows for one label; check first, as the IMAP path does.
		if found {
			o.Info(cmd.ErrOrStderr(), "%s already has the %q label", threadID, l.Name)
			return nil
		}
		_, err = client.CreateRecord[labelAssignment](ctx, c, "label_assignments", map[string]any{
			"label": l.ID, "record_id": state.ID,
			"collection": threadStateCollection, "user": userID,
		})
		if err != nil {
			return err
		}
		o.Info(cmd.ErrOrStderr(), "labelled %s %q", threadID, l.Name)
		return nil
	}
	if !found {
		o.Info(cmd.ErrOrStderr(), "%s does not have the %q label", threadID, l.Name)
		return nil
	}
	if err := client.DeleteRecord(ctx, c, "label_assignments", existing.ID); err != nil {
		return err
	}
	o.Info(cmd.ErrOrStderr(), "removed %q from %s", l.Name, threadID)
	return nil
}

// visibleLabels lists org-shared labels (user = "") plus the caller's own,
// matching the app's useLabels.
func visibleLabels(ctx context.Context, c *client.Client) ([]label, error) {
	userID, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListAll[label](ctx, c, "labels",
		client.Filter(`user = "" || user = {:u}`, map[string]any{"u": userID}), "name")
}

// resolveLabel accepts a label name (case-insensitive) or an id.
func resolveLabel(ctx context.Context, c *client.Client, ref string) (label, error) {
	labels, err := visibleLabels(ctx, c)
	if err != nil {
		return label{}, err
	}
	for _, l := range labels {
		if l.ID == ref || strings.EqualFold(l.Name, ref) {
			return l, nil
		}
	}
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return label{}, fmt.Errorf("no label %q (available: %s — create labels in the app)",
		ref, strings.Join(names, ", "))
}

func findAssignment(ctx context.Context, c *client.Client, labelID, recordID, userID string) (labelAssignment, bool, error) {
	res, err := client.ListRecords[labelAssignment](ctx, c, "label_assignments", client.ListOptions{
		Filter: client.Filter(
			`label = {:l} && record_id = {:r} && collection = {:c} && user = {:u}`,
			map[string]any{"l": labelID, "r": recordID, "c": threadStateCollection, "u": userID}),
		PerPage:   1,
		SkipTotal: true,
	})
	if err != nil {
		return labelAssignment{}, false, err
	}
	if len(res.Items) == 0 {
		return labelAssignment{}, false, nil
	}
	return res.Items[0], true, nil
}
