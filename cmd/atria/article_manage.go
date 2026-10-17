package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"atria/internal/articles"
	"atria/internal/core"
)

var articleArchiveCmd = &cobra.Command{
	Use:   "archive <uuid|short-uuid|\"Title\">",
	Short: "Moves an article out of the Inbox into the archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeArticle, args[0], false)
		if err != nil {
			return err
		}
		if err := articles.ArchiveArticle(app.Ctx, app.DB, app.Owner.ID, a.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Archived %q\n", a.Title)
		return nil
	},
}

var articleRmCmd = &cobra.Command{
	Use:   "rm <uuid|short-uuid|\"Title\">",
	Short: "Moves an article to the trash",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeArticle, args[0], false)
		if err != nil {
			return err
		}
		if err := articles.DeleteArticle(app.Ctx, app.DB, app.Owner.ID, a.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🗑️  Moved %q to the trash\n", a.Title)
		return nil
	},
}

var articleRefetchCmd = &cobra.Command{
	Use:   "refetch <uuid|short-uuid|\"Title\">",
	Short: "Downloads and extracts the article again from its original URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeArticle, args[0], false)
		if err != nil {
			return err
		}
		if err := articles.RefetchArticle(app.Ctx, app.DB, app.Owner.ID, a.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Refetched %q\n", a.Title)
		return nil
	},
}

var articleNoteCmd = &cobra.Command{
	Use:   "note <uuid|short-uuid|\"Title\"> <text>",
	Short: "Sets your personal note on an article (\"\" clears it)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeArticle, args[0], false)
		if err != nil {
			return err
		}
		if err := articles.UpdateUserNote(app.Ctx, app.DB, app.Owner.ID, a.ID, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Note on %q updated\n", a.Title)
		return nil
	},
}

func init() {
	articleCmd.AddCommand(articleArchiveCmd, articleRmCmd, articleRefetchCmd, articleNoteCmd,
		newRenameCmd(core.TypeArticle), newVisibilityCmd(core.TypeArticle, "/public/articles"))
}
