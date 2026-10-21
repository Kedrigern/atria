package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"atria/internal/games"
)

var gamesImportReplace bool

var gamesCmd = &cobra.Command{
	Use:   "games",
	Short: "Game catalog: import cryptogram campaigns",
}

var gamesImportCmd = &cobra.Command{
	Use:   "import <file.json>",
	Short: "Imports the cryptogram campaigns of a campaign file",
	Long: `Imports the campaigns of a campaign file (one book with its stories).
See docs/architecture/cryptogram-campaigns.md for the format.

The import adds the campaigns that are not in the database yet and keeps the
campaigns that did not change. It stops if a campaign in the database has
other passages. With --replace it deletes such a campaign, together with the
progress of all users on it, and imports it again.

The campaigns are a catalog for all users, so the command needs no user.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer f.Close()
		file, err := games.ReadImportFile(f)
		if err != nil {
			return err
		}

		db, err := openDatabase()
		if err != nil {
			return err
		}
		defer db.Close()
		ctx := context.Background()
		out := cmd.OutOrStdout()

		plan, importErr := games.Import(ctx, db, file, gamesImportReplace)
		for _, item := range plan {
			switch {
			case item.State == games.PlanChanged && item.Players > 0:
				fmt.Fprintf(out, "%-9s %s (%d users lose their progress)\n", item.State, item.Slug, item.Players)
			default:
				fmt.Fprintf(out, "%-9s %s\n", item.State, item.Slug)
			}
		}
		if importErr != nil {
			return importErr
		}
		fmt.Fprintln(out, "Import complete.")
		return nil
	},
}

func init() {
	gamesImportCmd.Flags().BoolVar(&gamesImportReplace, "replace", false, "delete and import again the campaigns with other passages (users lose their progress on them)")
	gamesCmd.AddCommand(gamesImportCmd)
	rootCmd.AddCommand(gamesCmd)
}
