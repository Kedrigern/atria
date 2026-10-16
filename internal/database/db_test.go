package database_test

import (
	"testing"

	"atria/internal/database"
	"atria/internal/testutil"
)

func TestDBInitialization(t *testing.T) {
	dsn := testutil.TestDatabaseURL(t)

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

func TestMigrationsRoundTrip(t *testing.T) {
	dsn := testutil.TestDatabaseURL(t)

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

	// Every Down section must succeed, and leave nothing behind that would
	// break a fresh Up.
	if err := database.MigrateDownAll(db); err != nil {
		t.Fatalf("Failed to roll back all migrations: %v", err)
	}

	var leftovers int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name <> 'goose_db_version'`).Scan(&leftovers)
	if err != nil {
		t.Fatalf("Failed to inspect schema: %v", err)
	}
	if leftovers != 0 {
		t.Errorf("expected an empty schema after rolling back, found %d tables/views", leftovers)
	}

	if err := database.MigrateUp(db); err != nil {
		t.Fatalf("Failed to re-run migrations after rollback: %v", err)
	}
}
