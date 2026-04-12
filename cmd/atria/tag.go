package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"atria/internal/cli"
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
	Use:   "attach <entity-uuid|short-uuid|\"Title\"> <tag-name>",
	Short: "Attaches a tag to an article, note, or RSS feed",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		entityIDStr, tagName := args[0], args[1]

		targetEntity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", entityIDStr, false)
		if err != nil {
			return err
		}

		err = core.AttachTagByTitle(app.Ctx, app.DB, app.Owner.ID, targetEntity.ID, tagName)
		if err != nil {
			return fmt.Errorf("failed to attach tag: %w", err)
		}

		fmt.Printf("✅ Tag '#%s' attached to %s '%s' (%s)\n", tagName, targetEntity.Type, targetEntity.Title, FormatID(targetEntity.ID, showLong))
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

		headers := []string{"ID", "NAME", "SYSTEM", "COLOR"}
		var rows [][]string
		for _, t := range tags {
			isSys := ""
			if t.IsSystem {
				isSys = "yes"
			}
			color := "-"
			if t.Color != nil {
				color = *t.Color
			}
			rows = append(rows, []string{
				FormatID(t.ID, showLong),
				t.Name,
				isSys,
				color})
		}

		return cli.Render(os.Stdout, listFormat, headers, rows, tags)
	},
}

var tagShowCmd = &cobra.Command{
	Use:   "show <tag-name>",
	Short: "Lists all entities associated with a specific tag",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tagName := args[0]
		entities, err := core.GetTagEntities(app.Ctx, app.DB, app.Owner.ID, tagName)
		if err != nil {
			return fmt.Errorf("failed to fetch entities for tag '%s': %w", tagName, err)
		}

		if len(entities) == 0 {
			fmt.Printf("No entities found with tag '#%s'\n", tagName)
			return nil
		}

		headers := []string{"ID", "TYPE", "TITLE"}
		var rows [][]string
		for _, e := range entities {
			rows = append(rows, []string{FormatID(e.ID, showLong), string(e.Type), e.Title})
		}
		return cli.Render(os.Stdout, listFormat, headers, rows, entities)
	},
}

func init() {
	rootCmd.AddCommand(tagCmd)
	tagCmd.AddCommand(tagAddCmd, tagAttachCmd, tagListCmd)
	tagCmd.AddCommand(tagShowCmd)
	tagListCmd.Flags().BoolVarP(&showLong, "long", "l", false, "Show full UUIDs")
	tagListCmd.Flags().StringVarP(&listFormat, "format", "f", "table", "Output format (table, json, csv, html)")

}
