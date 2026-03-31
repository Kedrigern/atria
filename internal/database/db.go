package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"time"

	// Anonymous import of the PostgreSQL driver so it registers with database/sql
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// InitDB establishes a connection to the database and verifies it is reachable.
func InitDB(dsn string) (*sql.DB, error) {
	log.Println("Connecting to PostgreSQL...")

	// Open the connection pool
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// sql.Open only prepares the connection but doesn't test it.
	// We must ping the database with a timeout to ensure it's actually reachable.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("database is unreachable (ping failed): %w", err)
	}

	log.Println("Successfully connected to the database.")

	// Configure the connection pool for optimal performance
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// MigrateUp runs all pending Goose migrations embedded within the binary.
func MigrateUp(db *sql.DB) error {
	log.Println("Checking and applying database migrations...")

	// Tell Goose to read SQL files from our embed.FS
	goose.SetBaseFS(embedMigrations)

	// Explicitly set the dialect so Goose writes to the goose_db_version table correctly
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set Goose dialect: %w", err)
	}

	// Run migrations from the "migrations" directory (matches the go:embed path)
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}

	log.Println("Database is up to date.")
	return nil
}

// MigrateDown rolls back the single most recent migration.
// Useful for tests and the `atria db drop` CLI command.
func MigrateDown(db *sql.DB) error {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	// goose.Down rolls back only ONE (the latest) migration.
	// For a complete reset in tests, goose.DownTo(db, "migrations", 0) can be used.
	if err := goose.Down(db, "migrations"); err != nil {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	return nil
}

// ResetDB performs a "nuclear" wipe of the entire database (drops the public schema).
// This is intended ONLY for development and testing!
func ResetDB(db *sql.DB) error {
	log.Println("WARNING: Initiating complete database wipe...")

	// In PostgreSQL, the cleanest way to reset is dropping and recreating the public schema.
	// CASCADE ensures all tables and types inside are deleted as well.
	_, err := db.Exec(`
		DROP SCHEMA public CASCADE;
		CREATE SCHEMA public;
		GRANT ALL ON SCHEMA public TO public;
	`)
	if err != nil {
		return fmt.Errorf("failed to reset database schema: %w", err)
	}

	log.Println("Database successfully wiped (reset to a clean state).")
	return nil
}
