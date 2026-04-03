package database_test

import (
	"os"
	"testing"

	"github.com/joho/godotenv"

	"atria/internal/database"
)

func TestDBInitialization(t *testing.T) {
	_ = godotenv.Load("../../.env")

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set in .env or environment. Skipping database integration test.")
	}

	db, err := database.InitDB(dsn)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	if err := database.ResetDB(db); err != nil {
		t.Fatalf("Failed to reset database: %v", err)
	}

	if err := database.MigrateUp(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
}
