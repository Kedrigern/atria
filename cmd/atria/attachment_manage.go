package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"atria/internal/attachments"
)

var attachmentRenameCmd = &cobra.Command{
	Use:   "rename <attachment> <new filename>",
	Short: "Renames an attachment (the display name; the stored file is unchanged)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		att, err := resolveAttachment(app.Ctx, app.DB, app.Owner.ID, args[0])
		if err != nil {
			return err
		}
		if err := attachments.RenameAttachment(app.Ctx, app.DB, app.Owner.ID, att.ID, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Renamed %q to %q\n", att.Filename, args[1])
		return nil
	},
}

var attachmentRmCmd = &cobra.Command{
	Use:   "rm <attachment>",
	Short: "Permanently deletes an attachment that is no longer linked to anything",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		att, err := resolveAttachment(app.Ctx, app.DB, app.Owner.ID, args[0])
		if err != nil {
			return err
		}
		if err := attachments.DeleteAttachment(app.Ctx, app.DB, app.Owner.ID, att.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🗑️  Deleted %q\n", att.Filename)
		return nil
	},
}

func init() {
	attachmentCmd.AddCommand(attachmentRenameCmd, attachmentRmCmd)
}
