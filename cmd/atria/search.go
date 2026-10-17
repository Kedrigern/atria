package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"atria/internal/cli"
	"atria/internal/search"
)

var (
	searchIn  string
	searchAll bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query...>",
	Short: "Full-text search across notes, articles and RSS items",
	Long: `Full-text search, the same as the web search. Matching words are shown
between *asterisks*. Archived articles and read RSS items are skipped unless
--all is given.`,
	Example:           `  atria search solar panels --in notes`,
	Args:              cobra.MinimumNArgs(1),
	PersistentPreRunE: RequireUserContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch searchIn {
		case "", "all", "notes", "articles", "rss":
		default:
			return fmt.Errorf("unknown --in %q (use notes, articles or rss)", searchIn)
		}

		results, err := search.Search(app.Ctx, app.DB, app.Owner.ID, strings.Join(args, " "), searchIn, searchAll)
		if err != nil {
			return err
		}
		if len(results) == 0 && listFormat == "table" {
			fmt.Fprintln(cmd.OutOrStdout(), "No results.")
			return nil
		}

		headers := []string{"TYPE", "ID", "TITLE", "MATCH"}
		rows := make([][]string, 0, len(results))
		for _, r := range results {
			rows = append(rows, []string{r.Type, FormatID(r.ID, showLong), r.Title, search.HeadlineText(r.Headline)})
		}
		return cli.Render(cmd.OutOrStdout(), listFormat, headers, rows, results)
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().StringVar(&searchIn, "in", "", "Limit to notes, articles or rss")
	searchCmd.Flags().BoolVar(&searchAll, "all", false, "Include archived articles and read RSS items")
	searchCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full UUIDs")
	searchCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "Output format (table, json, csv, html)")
}
