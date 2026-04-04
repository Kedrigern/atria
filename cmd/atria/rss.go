package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"text/tabwriter"
	"time"

	"atria/internal/core"
	"atria/internal/database"
	"atria/internal/notes"
	"atria/internal/rss"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"
)

var rssCmd = &cobra.Command{
	Use:   "rss",
	Short: "RSS feed management and triage",
}

var rssFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Manually triggers background fetching for all pending feeds",
	Run: func(cmd *cobra.Command, args []string) {
		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()

		err = rss.FetchAllActiveFeeds(context.Background(), db)
		if err != nil {
			log.Fatalf("RSS update failed: %v", err)
		}
		fmt.Println("✅ RSS update complete.")
	},
}

var rssAddCmd = &cobra.Command{
	Use:   "add <title> <url>",
	Short: "Subscribes to a new RSS/Atom feed",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		title, urlStr := args[0], args[1]
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

		feed, err := rss.CreateFeed(ctx, db, owner.ID, title, urlStr)
		if err != nil {
			log.Fatalf("Failed to add feed: %v", err)
		}

		fmt.Printf("✅ Subscribed to feed: %s\nID: %s\n", feed.Title, feed.ID)
	},
}

var rssListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all subscribed feeds and their status",
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

		feeds, err := rss.ListFeeds(ctx, db, owner.ID)
		if err != nil {
			log.Fatalf("Failed to list feeds: %v", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tURL\tLAST FETCH")
		for _, f := range feeds {
			status := "Never"
			if f.LastFetchStatus != nil {
				status = fmt.Sprintf("%d", *f.LastFetchStatus)
			}
			lastTime := "N/A"
			if f.LastFetchedAt != nil {
				lastTime = f.LastFetchedAt.Format("2006-01-02 15:04")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ShortID(f.ID), status, f.FeedURL, lastTime)
		}
		w.Flush()
	},
}

var rssRmCmd = &cobra.Command{
	Use:   "rm <uuid|short-uuid>",
	Short: "Removes an RSS subscription",
	Args:  cobra.ExactArgs(1),
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

		target := resolveEntityOrExit(ctx, db, owner.ID, core.TypeRSS, args[0], false)

		// RSS feeds are entities, so we use the shared DeleteEntity logic
		err = notes.DeleteEntity(ctx, db, owner.ID, target.ID, false, true)
		if err != nil {
			log.Fatalf("Failed to remove feed: %v", err)
		}

		fmt.Printf("✅ Feed removed: %s\n", target.Title)
	},
}

var (
	rssShowFormat string
	rssShowLong   bool
)

var rssShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Displays unread items from all subscribed feeds (Triage)",
	Run: func(cmd *cobra.Command, args []string) {
		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()

		owner, err := getActiveUser(context.Background(), db)
		if err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}

		items, err := rss.ListItemsToRead(context.Background(), db, owner.ID)
		if err != nil {
			log.Fatalf("Failed to list items: %v", err)
		}

		if len(items) == 0 {
			fmt.Println("No unread items. Your triage is empty! 🎉")
			return
		}

		// Handle non-table formats
		switch rssShowFormat {
		case "json":
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(items); err != nil {
				log.Fatalf("Failed to encode JSON: %v", err)
			}
			return
		case "csv":
			fmt.Println("ITEM_ID,FEED_ID,PUBLISHED,SOURCE,TITLE,LINK")
			for _, i := range items {
				fmt.Printf("%s,%s,%s,%s,\"%s\",%s\n", i.ID, i.FeedID, i.CreatedAt.Format(time.RFC3339), i.SourceName, i.Title, i.Link)
			}
			return
		}

		// Table format (default)
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if rssShowLong {
			fmt.Fprintln(w, "ITEM ID\tFEED ID\tPUBLISHED\tSOURCE\tTITLE\tLINK")
		} else {
			fmt.Fprintln(w, "ID\tPUBLISHED\tSOURCE\tTITLE")
		}

		for _, i := range items {
			published := i.CreatedAt.Format("2006-01-02 15:04")
			if rssShowLong {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", i.ID, i.FeedID, published, i.SourceName, i.Title, i.Link)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ShortID(i.ID), published, i.SourceName, i.Title)
			}
		}
		w.Flush()
	},
}

var rssSaveCmd = &cobra.Command{
	Use:   "save <item-id>",
	Short: "Converts an RSS item to a Read-it-Later article",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		db, _ := database.InitDB(os.Getenv("DATABASE_URL"))
		defer db.Close()
		ctx := context.Background()
		owner, _ := getActiveUser(ctx, db)

		// We need to resolve the short ID to a full UUID
		// For simplicity in this step, we assume the user provides a full or resolvable ID
		itemID, err := uuid.FromString(args[0])
		if err != nil {
			log.Fatalf("Invalid UUID format: %v", err)
		}

		article, err := rss.SaveItemAsArticle(ctx, db, owner.ID, itemID)
		if err != nil {
			log.Fatalf("Failed to save article: %v", err)
		}

		fmt.Printf("✅ Saved as article: %s\n", article.Title)
	},
}

func init() {
	rootCmd.AddCommand(rssCmd)
	rssCmd.AddCommand(rssFetchCmd, rssAddCmd, rssListCmd, rssRmCmd, rssShowCmd, rssSaveCmd)

	rssShowCmd.Flags().StringVar(&rssShowFormat, "format", "table", "Output format (table, json, csv)")
	rssShowCmd.Flags().BoolVarP(&rssShowLong, "long", "l", false, "Show detailed output including full IDs and URLs")
}
