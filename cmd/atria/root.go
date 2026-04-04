package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"atria/internal/core"

	"github.com/gofrs/uuid/v5"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var Version = "v0.0.1-dev"

var rootCmd = &cobra.Command{
	Use:     "atria",
	Short:   "Atria - Personal Mind Palace CLI",
	Long:    `Atria is a unified tool for managing your knowledge base, reading list, and RSS feeds.`,
	Version: Version,
}

func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("INFO: No .env file found, relying on system environment variables.")
	}
}

// getActiveUser retrieves the user defined in ATRIA_USER env variable.
func getActiveUser(ctx context.Context, db *sql.DB) (*core.User, error) {
	atriaUser := os.Getenv("ATRIA_USER")
	if atriaUser == "" {
		return nil, fmt.Errorf("ATRIA_USER environment variable is not set")
	}
	return core.FindUser(ctx, db, atriaUser)
}

// resolveEntityOrExit is a shared helper to find an entity or terminate the CLI with a nice error.
func resolveEntityOrExit(ctx context.Context, db *sql.DB, ownerID uuid.UUID, entityType core.EntityType, identifier string, includeDeleted bool) *core.EntitySummary {
	results, err := core.FindEntities(ctx, db, ownerID, entityType, identifier, includeDeleted)
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		log.Fatalf("Error: no %s found matching '%s'", entityType, identifier)
	}

	if len(results) > 1 {
		fmt.Printf("⚠️  Ambiguous identifier '%s'. Please be more specific:\n", identifier)
		for _, r := range results {
			fmt.Printf("  %s  %s\n", r.ID.String()[:8], r.Title)
		}
		os.Exit(1)
	}

	return &results[0]
}

// ShortID extracts the last 8 characters of a UUID for consistent CLI display.
// We use the end of the UUID because for UUIDv7, the beginning is a timestamp
// which can be identical for items created in the same millisecond.
func ShortID(id uuid.UUID) string {
	s := id.String()
	if len(s) < 8 {
		return s
	}
	return s[len(s)-8:]
}
