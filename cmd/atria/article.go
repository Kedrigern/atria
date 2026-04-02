package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/database"
)

var articleShowFormat string // Used to capture the --format flag

func resolveArticle(ctx context.Context, db *sql.DB, ownerID uuid.UUID, identifier string) (*core.EntitySummary, error) {
	results, err := core.FindEntities(ctx, db, ownerID, core.TypeArticle, identifier, false)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no article found matching '%s'", identifier)
	}

	if len(results) > 1 {
		fmt.Println("⚠️  This identifier is not unique. Please re-run with a specific UUID:")
		for _, r := range results {
			fmt.Printf("  %s  %s\n", r.ID.String()[:8], r.Title)
		}
		return nil, fmt.Errorf("ambiguous identifier")
	}

	return &results[0], nil
}

var articleCmd = &cobra.Command{
	Use:   "article",
	Short: "Read-it-Later and article management",
}

var articleAddCmd = &cobra.Command{
	Use:   "add <url>",
	Short: "Fetches a URL, runs Readability extraction, and saves it to the Inbox",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		urlStr := args[0]

		atriaUser := os.Getenv("ATRIA_USER")
		if atriaUser == "" {
			log.Fatal("ERROR: ATRIA_USER environment variable is not set.")
		}

		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()
		ctx := context.Background()

		owner, err := getActiveUser(ctx, db)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}

		entity, err := articles.CreateArticle(ctx, db, owner.ID, urlStr)
		if err != nil {
			log.Fatalf("Failed to save article: %v", err)
		}

		fmt.Printf("✅ Article saved successfully!\nID: %s\nTitle: %s\n", entity.ID, entity.Title)
	},
}

var articleShowCmd = &cobra.Command{
	Use:   "show <uuid|short-uuid|\"Title\">",
	Short: "Displays the extracted text of an article",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		identifier := args[0]

		atriaUser := os.Getenv("ATRIA_USER")
		if atriaUser == "" {
			log.Fatal("ERROR: ATRIA_USER environment variable is not set.")
		}

		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()
		ctx := context.Background()

		owner, err := getActiveUser(ctx, db)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}

		targetArticle, err := resolveArticle(ctx, db, owner.ID, identifier)
		if err != nil {
			os.Exit(1)
		}

		var content string
		var fetchErr error

		// Dynamically fetch format based on flag
		switch articleShowFormat {
		case "html":
			content, fetchErr = articles.GetArticleHTML(ctx, db, targetArticle.ID)
		case "plain":
			content, fetchErr = articles.GetArticleText(ctx, db, targetArticle.ID)
		case "md":
			fallthrough
		default:
			content, fetchErr = articles.GetArticleMarkdown(ctx, db, targetArticle.ID)
		}

		if fetchErr != nil {
			log.Fatalf("Failed to fetch content: %v", fetchErr)
		}

		fmt.Println("--- ARTICLE START ---")
		fmt.Println(content)
		fmt.Println("--- ARTICLE END ---")
	},
}

var articleListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists saved articles in the Inbox",
	Run: func(cmd *cobra.Command, args []string) {
		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()
		ctx := context.Background()

		owner, err := getActiveUser(ctx, db)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}

		articleList, err := articles.ListArticles(ctx, db, owner.ID)
		if err != nil {
			log.Fatalf("Failed to list articles: %v", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
		fmt.Fprintln(w, "ID\tDOMAIN\tTITLE\tSAVED")
		for _, a := range articleList {
			shortID := a.ID.String()[:8]
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				shortID,
				a.Domain,
				a.Title,
				a.CreatedAt.Format("2006-01-02 15:04"),
			)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(articleCmd)
	articleCmd.AddCommand(articleAddCmd, articleShowCmd, articleListCmd)

	articleShowCmd.Flags().StringVar(&articleShowFormat, "format", "md", "Output format (md, html, plain)")
}
