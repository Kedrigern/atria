package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"atria/internal/core"
	"atria/internal/rss"
)

var (
	pruneDays   int
	pruneDryRun bool
)

var rssPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Shrinks old read RSS items now, for all users (the server also does this daily)",
	Long: `Clears the stored content of RSS items that were read more than the
retention period ago (RSS_RETENTION_DAYS, default 180; override with --days).
Title, link and read state stay, so items are not re-imported. Unread items
are never touched. Applies to every user's feeds.`,
	Args:              cobra.NoArgs,
	PersistentPreRunE: noUserContext,
	RunE: func(cmd *cobra.Command, args []string) error {
		retention := rss.RetentionFromEnv()
		if pruneDays > 0 {
			retention = time.Duration(pruneDays) * 24 * time.Hour
		}
		days := int(retention.Hours() / 24)

		db, err := openDatabase()
		if err != nil {
			return err
		}
		defer db.Close()
		ctx := context.Background()
		out := cmd.OutOrStdout()

		items, bytes, err := rss.CountPrunable(ctx, db, retention)
		if err != nil {
			return err
		}
		if pruneDryRun {
			fmt.Fprintf(out, "Would shrink %d read items older than %d days, freeing about %s.\n", items, days, core.FormatBytes(bytes))
			return nil
		}

		n, err := rss.PruneReadItems(ctx, db, retention)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "✅ Shrunk %d read items older than %d days, freeing about %s.\n", n, days, core.FormatBytes(bytes))
		return nil
	},
}

func init() {
	rssCmd.AddCommand(rssPruneCmd)
	rssPruneCmd.Flags().IntVar(&pruneDays, "days", 0, "Retention in days (default: RSS_RETENTION_DAYS or 180)")
	rssPruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "Only report what would be shrunk")
}
