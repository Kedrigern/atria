package testutil

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"atria/internal/core"
	"atria/internal/database"
	"atria/internal/users"

	"github.com/joho/godotenv"
)

// SetupTestDB init empty test DB and create users
func SetupTestDB(t *testing.T) (*sql.DB, *core.User) {
	t.Helper()

	// Load .env
	_ = godotenv.Load("../../.env")
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set. Skipping DB test.")
	}

	db, err := database.InitDB(dsn)
	if err != nil {
		t.Fatalf("Failed to connect to test db: %v", err)
	}

	// Reset and migration
	if err := database.ResetDB(db); err != nil {
		t.Fatalf("Failed to reset db: %v", err)
	}
	if err := database.MigrateUp(db); err != nil {
		t.Fatalf("Failed to migrate db: %v", err)
	}

	// Create default user for tests
	user, err := users.CreateUser(context.Background(), db, "shared_tester@atria.local", "Test User", "pass", core.RoleUser)
	if err != nil {
		t.Fatalf("Failed to create shared test user: %v", err)
	}

	return db, user
}
