package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/rss"

	"github.com/spf13/cobra"
)

var rssShowFormat string

var rssCmd = &cobra.Command{
	Use:               "rss",
	Short:             "RSS feed management and triage",
	PersistentPreRunE: RequireUserContext,
}

var rssFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Manually triggers background fetching for all pending feeds",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := rss.FetchAllActiveFeeds(app.Ctx, app.DB)
		if err != nil {
			return fmt.Errorf("RSS update failed: %w", err)
		}
		fmt.Println("✅ RSS update complete.")
		return nil
	},
}

var rssAddCmd = &cobra.Command{
	Use:   "add <title> <url>",
	Short: "Subscribes to a new RSS/Atom feed",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		title, urlStr := args[0], args[1]

		feed, err := rss.CreateFeed(app.Ctx, app.DB, app.Owner.ID, title, urlStr)
		if err != nil {
			return fmt.Errorf("failed to add feed: %w", err)
		}

		fmt.Printf("✅ Subscribed to feed: %s\nID: %s\n", feed.Title, feed.ID)
		return nil
	},
}

var rssListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all subscribed feeds and their status",
	RunE: func(cmd *cobra.Command, args []string) error {
		feeds, err := rss.ListFeeds(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return fmt.Errorf("failed to list feeds: %w", err)
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
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", FormatID(f.ID, showLong), status, f.FeedURL, lastTime)
		}
		w.Flush()
		return nil
	},
}

var rssRmCmd = &cobra.Command{
	Use:   "rm <uuid|short-uuid|\"Title\">",
	Short: "Removes an RSS subscription",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeRSS, args[0], false)
		if err != nil {
			return err
		}

		err = notes.DeleteEntity(app.Ctx, app.DB, app.Owner.ID, target.ID, false, true)
		if err != nil {
			return fmt.Errorf("failed to remove feed: %w", err)
		}

		fmt.Printf("✅ Feed removed: %s\n", target.Title)
		return nil
	},
}

var rssShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Displays unread items from all subscribed feeds (Triage)",
	RunE: func(cmd *cobra.Command, args []string) error {
		items, err := rss.ListItemsToRead(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return fmt.Errorf("failed to list items: %w", err)
		}

		if len(items) == 0 {
			fmt.Println("No unread items. Your triage is empty! 🎉")
			return nil
		}

		switch rssShowFormat {
		case "json":
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(items); err != nil {
				return fmt.Errorf("failed to encode JSON: %w", err)
			}
			return nil
		case "csv":
			fmt.Println("ITEM_ID,FEED_ID,PUBLISHED,SOURCE,TITLE,LINK")
			for _, i := range items {
				fmt.Printf("%s,%s,%s,%s,\"%s\",%s\n", i.ID, i.FeedID, i.CreatedAt.Format(time.RFC3339), i.SourceName, i.Title, i.Link)
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if showLong {
			fmt.Fprintln(w, "ITEM ID\tFEED ID\tPUBLISHED\tSOURCE\tTITLE\tLINK")
		} else {
			fmt.Fprintln(w, "ID\tPUBLISHED\tSOURCE\tTITLE")
		}

		for _, i := range items {
			published := i.CreatedAt.Format("2006-01-02 15:04")
			if showLong {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", FormatID(i.ID, showLong), FormatID(i.FeedID, showLong), published, i.SourceName, i.Title, i.Link)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", FormatID(i.ID, showLong), published, i.SourceName, i.Title)
			}
		}
		w.Flush()
		return nil
	},
}

var rssSaveCmd = &cobra.Command{
	Use:   "save <item-id>",
	Short: "Converts an RSS item to a Read-it-Later article",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use core.ParseUUID instead of direct uuid.FromString
		itemID, err := core.ParseUUID(args[0])
		if err != nil {
			return fmt.Errorf("invalid UUID format: %w", err)
		}

		article, err := rss.SaveItemAsArticle(app.Ctx, app.DB, app.Owner.ID, itemID)
		if err != nil {
			return fmt.Errorf("failed to save article: %w", err)
		}

		fmt.Printf("✅ Saved as article: %s\n", article.Title)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rssCmd)
	rssCmd.AddCommand(rssFetchCmd, rssAddCmd, rssListCmd, rssRmCmd, rssShowCmd, rssSaveCmd)

	rssShowCmd.Flags().StringVar(&rssShowFormat, "format", "table", "Output format (table, json, csv)")

	rssListCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full UUIDs")
	rssShowCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full IDs and URLs")
}
