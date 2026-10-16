package testutil

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/database"
	"atria/internal/users"

	"github.com/joho/godotenv"
)

// ValidateTestDatabaseURL refuses DSNs that do not look like a dedicated test
// database. Tests drop the whole public schema, so pointing TEST_DATABASE_URL
// at a real database by mistake would destroy its data.
func ValidateTestDatabaseURL(testDSN, appDSN string) error {
	u, err := url.Parse(testDSN)
	if err != nil {
		return fmt.Errorf("TEST_DATABASE_URL is not a valid URL: %w", err)
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		dbName = u.Query().Get("dbname")
	}
	if !strings.Contains(strings.ToLower(dbName), "test") {
		return fmt.Errorf("refusing to wipe database %q: TEST_DATABASE_URL must point to a database whose name contains \"test\"", dbName)
	}
	if appDSN != "" && appDSN == testDSN {
		return errors.New("refusing to wipe database: TEST_DATABASE_URL is the same as DATABASE_URL")
	}
	return nil
}

// TestDatabaseURL returns the DSN of the disposable test database, skipping
// the test when none is configured and failing it when the DSN looks unsafe.
func TestDatabaseURL(t *testing.T) string {
	t.Helper()

	// Load .env
	_ = godotenv.Load("../../.env")
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set. Skipping DB test.")
	}
	if err := ValidateTestDatabaseURL(dsn, os.Getenv("DATABASE_URL")); err != nil {
		t.Fatal(err)
	}
	return dsn
}

// SetupTestDB init empty test DB and create users
func SetupTestDB(t *testing.T) (*sql.DB, *core.User) {
	t.Helper()

	dsn := TestDatabaseURL(t)

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
