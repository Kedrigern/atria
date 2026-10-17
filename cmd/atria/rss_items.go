package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"atria/internal/core"
	"atria/internal/rss"
)

var rssReadCmd = &cobra.Command{
	Use:   "read <item-id> [item-id...]",
	Short: "Marks RSS items as read (IDs as printed by `rss show`)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, identifier := range args {
			item, err := resolveRSSItem(app.Ctx, app.DB, app.Owner.ID, identifier)
			if err != nil {
				return err
			}
			if err := rss.MarkAsRead(app.Ctx, app.DB, app.Owner.ID, item.ID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ Read: %s\n", item.Title)
		}
		return nil
	},
}

var rssReadAllCmd = &cobra.Command{
	Use:   "read-all <feed>",
	Short: "Marks every unread item of a feed as read",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		feed, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeRSS, args[0], false)
		if err != nil {
			return err
		}
		if err := rss.MarkFeedAsRead(app.Ctx, app.DB, app.Owner.ID, feed.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ All items of %q marked as read\n", feed.Title)
		return nil
	},
}

func init() {
	rssCmd.AddCommand(rssReadCmd, rssReadAllCmd, newRenameCmd(core.TypeRSS))
}
