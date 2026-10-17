package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"

	"atria/internal/core"
	"atria/internal/rss"
)

// Shared building blocks for commands that change existing content. Each
// command resolves its identifier and makes a single service call, like the
// matching web handler.

// newRenameCmd builds "<group> rename <identifier> <new title>".
func newRenameCmd(entityType core.EntityType) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <uuid|short-uuid|\"Title\"> <new title>",
		Short: fmt.Sprintf("Renames a %s", entityType),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, entityType, args[0], false)
			if err != nil {
				return err
			}
			if err := core.RenameEntity(app.Ctx, app.DB, app.Owner.ID, entity.ID, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ Renamed %q to %q\n", entity.Title, args[1])
			return nil
		},
	}
}

// newVisibilityCmd builds "<group> visibility <identifier> <private|users|public>".
func newVisibilityCmd(entityType core.EntityType, publicPath string) *cobra.Command {
	return &cobra.Command{
		Use:       "visibility <uuid|short-uuid|\"Title\"> <private|users|public>",
		Short:     fmt.Sprintf("Sets who can see a %s (public = shareable link)", entityType),
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{"private", "users", "public"},
		RunE: func(cmd *cobra.Command, args []string) error {
			visibility := core.Visibility(args[1])
			switch visibility {
			case core.VisibilityPrivate, core.VisibilityUsers, core.VisibilityPublic:
			default:
				return fmt.Errorf("unknown visibility %q (use private, users or public)", args[1])
			}
			entity, err := resolveEntity(app.Ctx, app.DB, app.Owner.ID, entityType, args[0], false)
			if err != nil {
				return err
			}
			if err := core.UpdateVisibility(app.Ctx, app.DB, app.Owner.ID, entity.ID, visibility); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "✅ %q is now %s\n", entity.Title, visibility)
			if visibility == core.VisibilityPublic {
				fmt.Fprintf(out, "Public link path: %s/%s\n", publicPath, entity.ID)
			}
			return nil
		},
	}
}

// resolveRSSItem accepts a full UUID or the short ID printed by `rss show`.
func resolveRSSItem(ctx context.Context, db *sql.DB, ownerID uuid.UUID, identifier string) (*rss.ItemSummary, error) {
	items, err := rss.FindItems(ctx, db, ownerID, identifier)
	return resolveSingle(identifier, items, err, "rss item", func(i rss.ItemSummary) string {
		return fmt.Sprintf("%s  %s", ShortID(i.ID), i.Title)
	})
}
