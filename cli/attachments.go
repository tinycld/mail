package cli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/cli/ui"
)

// storedSuffix is PocketBase's 10-char random suffix on stored file names;
// stripping it recovers the original upload name (same regex the app's
// attachment preview uses).
var storedSuffix = regexp.MustCompile(`^(.+)_[a-zA-Z0-9]{10}(\.\w+)$`)

func displayName(stored string) string {
	if m := storedSuffix.FindStringSubmatch(stored); m != nil {
		return m[1] + m[2]
	}
	return stored
}

// attachmentRow is the JSON shape of one listed attachment.
type attachmentRow struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Stored string `json:"stored"`
}

func newAttachmentsCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "attachments <message>",
		Short: "List a message's attachments",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			m, err := client.GetRecord[message](cmd.Context(), c, "mail_messages", args[0])
			if err != nil {
				return err
			}
			rows := make([][]string, len(m.Attachments))
			raw := make([]attachmentRow, len(m.Attachments))
			for i, stored := range m.Attachments {
				raw[i] = attachmentRow{Index: i + 1, Name: displayName(stored), Stored: stored}
				rows[i] = []string{strconv.Itoa(i + 1), displayName(stored)}
			}
			return o.Write(cmd.OutOrStdout(), []string{"N", "NAME"}, rows, raw)
		},
	}
}

func newDownloadCmd(c *client.Client) *cobra.Command {
	var pick, outDir string
	cmd := &cobra.Command{
		Use:   "download <message>",
		Short: "Download attachments",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			m, err := client.GetRecord[message](ctx, c, "mail_messages", args[0])
			if err != nil {
				return err
			}
			if len(m.Attachments) == 0 {
				return fmt.Errorf("%s: no attachments", args[0])
			}
			selected := m.Attachments
			if pick != "all" {
				n, err := strconv.Atoi(pick)
				if err != nil || n < 1 || n > len(m.Attachments) {
					return fmt.Errorf("--attachment must be 1..%d or all", len(m.Attachments))
				}
				selected = m.Attachments[n-1 : n]
			}
			for _, stored := range selected {
				dest := filepath.Join(outDir, displayName(stored))
				prog := ui.NewProgress(o, cmd.ErrOrStderr(), displayName(stored))
				err := client.DownloadToFile(ctx, c,
					client.FileURL("mail_messages", m.ID, stored), dest, prog.Func())
				prog.Done()
				if err != nil {
					return err
				}
				o.Info(cmd.ErrOrStderr(), "saved %s", dest)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pick, "attachment", "all", "which attachment: N (1-based) or all")
	cmd.Flags().StringVar(&outDir, "out", ".", "destination directory")
	return cmd
}
