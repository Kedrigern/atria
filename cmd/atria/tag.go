package main

import (
	"atria/internal/core"
	"atria/internal/database"
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"
)

var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Tag management for all entities",
}

var tagAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Creates a new global tag",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		db, _ := database.InitDB(os.Getenv("DATABASE_URL"))
		defer db.Close()
		owner, _ := getActiveUser(context.Background(), db)

		tag, err := core.CreateTag(context.Background(), db, owner.ID, args[0], false)
		if err != nil {
			log.Fatalf("Failed to create tag: %v", err)
		}
		fmt.Printf("✅ Tag created: %s (%s)\n", tag.Name, tag.ID)
	},
}

var tagAttachCmd = &cobra.Command{
	Use:   "attach <entity-identifier> <tag-name>",
	Short: "Attaches a tag to an article, note, or RSS feed",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		entityIDStr, tagName := args[0], args[1]

		db, _ := database.InitDB(os.Getenv("DATABASE_URL"))
		defer db.Close()
		ctx := context.Background()
		owner, _ := getActiveUser(ctx, db)

		// 1. Resolve entity (could be any type)
		// We can use a generic search or a specialized one
		// For now, we'll try to parse as UUID or search entities directly
		var entityID uuid.UUID
		if id, err := core.ParseUUID(entityIDStr); err == nil {
			entityID = id
		} else {
			// Fallback: search for entity by title or short ID
			// (Uses your existing resolveEntityOrExit logic)
			// For generic tagging, we might need a FindAnyEntity helper
			log.Fatal("Please provide a valid full UUID for the entity in this version.")
		}

		// 2. Attach the tag
		err := core.AttachTagByTitle(ctx, db, owner.ID, entityID, tagName)
		if err != nil {
			log.Fatalf("Failed to attach tag: %v", err)
		}

		fmt.Printf("✅ Tag '#%s' attached to entity %s\n", tagName, entityID)
	},
}

var tagListCmd = &cobra.Command{
	Use:   "list",
	Short: "Outputs a tabular list of all your tags",
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

		tags, err := core.ListTags(ctx, db, owner.ID)
		if err != nil {
			log.Fatalf("Failed to fetch tags: %v", err)
		}

		if len(tags) == 0 {
			fmt.Println("No tags found. Use 'atria tag add <name>' to create one.")
			return
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

			fmt.Fprintf(w, "%s\t#%s\t%s\t%s\n",
				ShortID(t.ID),
				t.Name,
				isSys,
				color,
			)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(tagCmd)
	tagCmd.AddCommand(tagAddCmd, tagAttachCmd, tagListCmd)
}
