package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"atria/internal/core"
	"atria/internal/database"

	"github.com/gofrs/uuid/v5"
	"github.com/spf13/cobra"
)

var Version = "v0.0.1-dev"

type AppContext struct {
	DB    *sql.DB
	Ctx   context.Context
	Owner *core.User
}

var app *AppContext
var globalUserFlag string
var showLong bool

var rootCmd = &cobra.Command{
	Use:           "atria",
	Short:         "Atria - Personal Mind Palace CLI",
	Long:          `Atria is a unified tool for managing your knowledge base, reading list, and RSS feeds.`,
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// RequireUserContext is an opt-in PreRun function for commands needing DB and User.
func RequireUserContext(cmd *cobra.Command, args []string) error {
	db, err := database.InitDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	ctx := context.Background()
	owner, err := getActiveUser(ctx, db)
	if err != nil {
		db.Close()
		return fmt.Errorf("authentication failed: %w", err)
	}

	app = &AppContext{
		DB:    db,
		Ctx:   ctx,
		Owner: owner,
	}
	return nil
}

// getActiveUser retrieves the user defined by flag or ATRIA_USER env variable.
func getActiveUser(ctx context.Context, db *sql.DB) (*core.User, error) {
	identifier := globalUserFlag
	if identifier == "" {
		identifier = os.Getenv("ATRIA_USER")
	}
	if identifier == "" {
		return nil, fmt.Errorf("user context required. Use --user flag or set ATRIA_USER environment variable")
	}
	return core.FindUser(ctx, db, identifier)
}

// resolveEntity is a shared helper to find an entity or return an error.
func resolveEntity(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityType core.EntityType, identifier string, includeDeleted bool) (*core.EntitySummary, error) {
	results, err := core.FindEntities(ctx, db, ownerID, entityType, identifier, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no %s found matching '%s'", entityType, identifier)
	}

	if len(results) > 1 {
		errMsg := fmt.Sprintf("Ambiguous identifier '%s'. Please be more specific:\n", identifier)
		for _, r := range results {
			errMsg += fmt.Sprintf("  %s  %s\n", r.ID.String()[:8], r.Title)
		}
		return nil, fmt.Errorf(errMsg)
	}

	return &results[0], nil
}

// ShortID extracts the last 8 characters of a UUID for consistent CLI display.
func ShortID(id uuid.UUID) string {
	s := id.String()
	if len(s) < 8 {
		return s
	}
	return s[len(s)-8:]
}

// FormatID returns full UUID or 8 chars short version
func FormatID(id uuid.UUID, long bool) string {
	if long {
		return id.String()
	}
	return ShortID(id)
}
