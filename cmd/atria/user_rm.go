package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"atria/internal/core"
	"atria/internal/storage"
	"atria/internal/users"
)

var userRmForce bool

var userRmCmd = &cobra.Command{
	Use:   "rm <email|uuid>",
	Short: "Deletes a user and all of their data (requires --force)",
	Long: `Deletes a user together with all of their notes, articles, feeds, tags,
attachments, e-books, sessions and game progress. Attachment files that no
other user shares are removed from disk.

Without --force it only prints what would be deleted.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openDatabase()
		if err != nil {
			return err
		}
		defer db.Close()
		ctx := context.Background()
		out := cmd.OutOrStdout()

		user, err := core.FindUser(ctx, db, args[0])
		if err != nil {
			return err
		}
		usage, err := storage.ForUser(ctx, db, user.ID)
		if err != nil {
			return err
		}

		fmt.Fprintf(out, "User:        %s <%s> (%s)\n", user.DisplayName, user.Email, user.Role)
		fmt.Fprintf(out, "Notes:       %d\n", usage.NoteCount)
		fmt.Fprintf(out, "Articles:    %d\n", usage.ArticleCount)
		fmt.Fprintf(out, "RSS items:   %d\n", usage.RSSItemCount)
		fmt.Fprintf(out, "Attachments: %d (%s)\n", usage.AttachmentCount, core.FormatBytes(usage.AttachmentBytes))
		fmt.Fprintf(out, "Total:       %s\n", core.FormatBytes(usage.TotalBytes()))

		if !userRmForce {
			fmt.Fprintln(out, "\nNothing deleted. Re-run with --force to permanently delete this user and all of the above.")
			return nil
		}

		if err := users.DeleteUser(ctx, db, user.Email); err != nil {
			return err
		}
		fmt.Fprintf(out, "\n✅ User %s and all of their data were deleted.\n", user.Email)
		return nil
	},
}

func init() {
	userCmd.AddCommand(userRmCmd)
	userRmCmd.Flags().BoolVar(&userRmForce, "force", false, "Actually delete (otherwise only a summary is printed)")
}
