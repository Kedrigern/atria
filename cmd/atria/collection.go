package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"atria/internal/cli"
	"atria/internal/collections"
	"atria/internal/core"
)

var (
	collectionOrder       string
	collectionDescription string
)

var collectionCmd = &cobra.Command{
	Use:               "collection",
	Short:             "Lists of e-books and manuscripts, for example a book series",
	PersistentPreRunE: RequireUserContext,
}

var collectionCreateCmd = &cobra.Command{
	Use:   "create <title> [--order=manual|title|author|added] [--description=TEXT]",
	Short: "Creates an empty collection",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := collections.Create(app.Ctx, app.DB, app.Owner.ID, args[0], collectionDescription, collections.SortOrder(collectionOrder))
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Collection %s  %s\n", ShortID(c.ID), c.Title)
		return nil
	},
}

var collectionListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists collections",
	RunE: func(cmd *cobra.Command, args []string) error {
		list, err := collections.List(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return err
		}
		rows := make([][]string, 0, len(list))
		for _, c := range list {
			rows = append(rows, []string{FormatID(c.ID, showLong), c.Title, string(c.SortOrder), strconv.Itoa(c.ItemCount)})
		}
		return cli.Render(cmd.OutOrStdout(), listFormat, []string{"ID", "TITLE", "ORDER", "ITEMS"}, rows, list)
	},
}

var collectionShowCmd = &cobra.Command{
	Use:   "show <collection>",
	Short: "Lists the items of a collection in its order",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeCollection, args[0], false)
		if err != nil {
			return err
		}
		items, err := collections.Items(app.Ctx, app.DB, app.Owner.ID, c.ID)
		if err != nil {
			return err
		}
		rows := make([][]string, 0, len(items))
		for _, it := range items {
			number := ""
			if it.Number > 0 {
				number = strconv.Itoa(it.Number)
			}
			rows = append(rows, []string{number, FormatID(it.ID, showLong), string(it.Type), it.Title, it.Author})
		}
		return cli.Render(cmd.OutOrStdout(), listFormat, []string{"#", "ID", "TYPE", "TITLE", "AUTHOR"}, rows, items)
	},
}

var collectionAddCmd = &cobra.Command{
	Use:   "add <collection> <item>...",
	Short: "Adds e-books or manuscripts to the end of a collection",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeCollection, args[0], false)
		if err != nil {
			return err
		}
		for _, identifier := range args[1:] {
			item, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", identifier, false)
			if err != nil {
				return err
			}
			if err := collections.Add(app.Ctx, app.DB, app.Owner.ID, c.ID, item.ID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added %q to %q\n", item.Title, c.Title)
		}
		return nil
	},
}

var collectionRemoveCmd = &cobra.Command{
	Use:   "remove <collection> <item>",
	Short: "Takes an item out of a collection; the item stays",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeCollection, args[0], false)
		if err != nil {
			return err
		}
		item, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", args[1], false)
		if err != nil {
			return err
		}
		if err := collections.Remove(app.Ctx, app.DB, app.Owner.ID, c.ID, item.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed %q from %q\n", item.Title, c.Title)
		return nil
	},
}

var collectionMoveCmd = &cobra.Command{
	Use:   "move <collection> <item> <up|down>",
	Short: "Moves an item of a manual collection one place up or down",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		step := map[string]int{"up": -1, "down": 1}[args[2]]
		if step == 0 {
			return fmt.Errorf("use up or down, not %q", args[2])
		}
		c, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeCollection, args[0], false)
		if err != nil {
			return err
		}
		item, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", args[1], false)
		if err != nil {
			return err
		}
		return collections.Move(app.Ctx, app.DB, app.Owner.ID, c.ID, item.ID, step)
	},
}

var collectionRmCmd = &cobra.Command{
	Use:   "rm <collection>",
	Short: "Moves a collection to the trash; its items stay",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, core.TypeCollection, args[0], false)
		if err != nil {
			return err
		}
		if err := core.SoftDeleteEntity(app.Ctx, app.DB, app.Owner.ID, c.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "🗑️  Moved %q to the trash\n", c.Title)
		return nil
	},
}

func init() {
	collectionCreateCmd.Flags().StringVar(&collectionOrder, "order", string(collections.Manual), "manual, title, author or added")
	collectionCreateCmd.Flags().StringVar(&collectionDescription, "description", "", "description of the collection")
	for _, c := range []*cobra.Command{collectionListCmd, collectionShowCmd} {
		c.Flags().StringVarP(&listFormat, "format", "f", "table", "output format: table, csv or json")
	}
	collectionCmd.AddCommand(collectionCreateCmd, collectionListCmd, collectionShowCmd, collectionAddCmd,
		collectionRemoveCmd, collectionMoveCmd, collectionRmCmd, newRenameCmd(core.TypeCollection))
	rootCmd.AddCommand(collectionCmd)
}
