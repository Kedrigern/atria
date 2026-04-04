package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"atria/internal/core"
)

var tagCmd = &cobra.Command{
	Use:               "tag",
	Short:             "Tag management for all entities",
	PersistentPreRunE: RequireUserContext,
}

var tagAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Creates a new global tag",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tag, err := core.CreateTag(app.Ctx, app.DB, app.Owner.ID, args[0], false)
		if err != nil {
			return fmt.Errorf("failed to create tag: %w", err)
		}
		fmt.Printf("✅ Tag created: %s (%s)\n", tag.Name, tag.ID)
		return nil
	},
}

var tagAttachCmd = &cobra.Command{
	Use:   "attach <entity-uuid> <tag-name>",
	Short: "Attaches a tag to an article, note, or RSS feed",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		entityIDStr, tagName := args[0], args[1]

		// For now, require the exact UUID. Searching across all types requires
		// extending FindEntities to accept empty type (TypeAny).
		entityID, err := core.ParseUUID(entityIDStr)
		if err != nil {
			return fmt.Errorf("please provide a valid full UUID for the entity in this version: %w", err)
		}

		err = core.AttachTagByTitle(app.Ctx, app.DB, app.Owner.ID, entityID, tagName)
		if err != nil {
			return fmt.Errorf("failed to attach tag: %w", err)
		}

		fmt.Printf("✅ Tag '#%s' attached to entity %s\n", tagName, entityID)
		return nil
	},
}

var tagListCmd = &cobra.Command{
	Use:   "list",
	Short: "Outputs a tabular list of all your tags",
	RunE: func(cmd *cobra.Command, args []string) error {
		tags, err := core.ListTags(app.Ctx, app.DB, app.Owner.ID)
		if err != nil {
			return fmt.Errorf("failed to fetch tags: %w", err)
		}

		if len(tags) == 0 {
			fmt.Println("No tags found. Use 'atria tag add <name>' to create one.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSYSTEM\tCOLOR")
		for _, t := range tags {
			isSys := ""
			if t.IsSystem {
				isSys = "yes"
			}
			color := "-"
			if t.Color != nil {
				color = *t.Color
			}
			fmt.Fprintf(w, "%s\t#%s\t%s\t%s\n", FormatID(t.ID, showLong), t.Name, isSys, color)
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tagCmd)
	tagCmd.AddCommand(tagAddCmd, tagAttachCmd, tagListCmd)
	tagListCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full UUIDs")
}
