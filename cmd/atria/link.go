package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"atria/internal/links"
)

var linkContext string

var linkCmd = &cobra.Command{
	Use:               "link",
	Short:             "Knowledge graph and entity relations management",
	PersistentPreRunE: RequireUserContext,
}

var linkAddCmd = &cobra.Command{
	Use:   "add <source-entity> <target-entity>",
	Short: "Creates a directional link from the source entity to the target entity",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourceIdentifier, targetIdentifier := args[0], args[1]

		// 1. Resolve Source
		sourceEntity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", sourceIdentifier, false)
		if err != nil {
			return fmt.Errorf("source error: %w", err)
		}

		// 2. Resolve Target
		targetEntity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, "", targetIdentifier, false)
		if err != nil {
			return fmt.Errorf("target error: %w", err)
		}

		// 3. Create Link
		err = links.AddLink(app.Ctx, app.DB, sourceEntity.ID, targetEntity.ID, linkContext)
		if err != nil {
			return fmt.Errorf("failed to link entities: %w", err)
		}

		fmt.Printf("🔗 Link created: [%s] -> [%s]\n", sourceEntity.Title, targetEntity.Title)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(linkCmd)
	linkCmd.AddCommand(linkAddCmd)

	linkAddCmd.Flags().StringVarP(&linkContext, "context", "c", "", "Optional context describing why the link exists")
}
