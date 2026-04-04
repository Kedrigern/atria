package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"atria/internal/articles"
	"atria/internal/core"
)

var articleShowFormat string

var articleCmd = &cobra.Command{
	Use:               "article",
	Short:             "Read-it-Later and article management",
	PersistentPreRunE: RequireUserContext,
}

var articleAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Fetches a URL, runs Readability extraction, and saves it to the Inbox",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		urlStr := args[0]

		entity, err := articles.CreateArticle(app.Ctx, app.DB, app.Owner.ID, urlStr)
		if err != nil {
			return fmt.Errorf("failed to save article: %w", err)
		}

		fmt.Printf("✅ Article saved successfully!\nID: %s\nTitle: %s\n", entity.ID, entity.Title)
		return nil
	},
}

var articleShowCmd = &cobra.Command{
	Use:   "show <uuid|short-uuid|\"Title\">",
	Short: "Displays the extracted text of an article",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		targetArticle, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeArticle, identifier, false)
		if err != nil {
			return err
		}

		var content string
		var fetchErr error

		switch articleShowFormat {
		case "html":
			content, fetchErr = articles.GetArticleHTML(app.Ctx, app.DB, targetArticle.ID)
		case "plain":
			content, fetchErr = articles.GetArticleText(app.Ctx, app.DB, targetArticle.ID)
		case "md":
			fallthrough
		default:
			content, fetchErr = articles.GetArticleMarkdown(app.Ctx, app.DB, targetArticle.ID)
		}

		if fetchErr != nil {
			return fmt.Errorf("failed to fetch content: %w", fetchErr)
		}

		fmt.Println("--- ARTICLE START ---")
		fmt.Println(content)
		fmt.Println("--- ARTICLE END ---")
		return nil
	},
}

var articleListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists saved articles in the Inbox",
	RunE: func(cmd *cobra.Command, args []string) error {
		articleList, err := articles.ListArticles(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return fmt.Errorf("failed to list articles: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
		fmt.Fprintln(w, "ID\tDOMAIN\tTITLE\tSAVED")
		for _, a := range articleList {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", FormatID(a.ID, showLong), a.Domain, a.Title, a.CreatedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(articleCmd)
	articleCmd.AddCommand(articleAddCmd, articleShowCmd, articleListCmd)

	articleShowCmd.Flags().StringVar(&articleShowFormat, "format", "md", "Output format (md, html, plain)")
	articleListCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full UUIDs")
}
