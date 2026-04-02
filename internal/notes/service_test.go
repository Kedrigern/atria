package notes_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/joho/godotenv"

	"atria/internal/core"
	"atria/internal/database"
	"atria/internal/notes"
	"atria/internal/users"
)

// setupTestDB prepares a clean database and creates a dummy user for testing notes.
func setupTestDB(t *testing.T) (*sql.DB, *core.User) {
	_ = godotenv.Load("../../.env")

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set in .env. Skipping notes integration tests.")
	}

	db, err := database.InitDB(dsn)
	if err != nil {
		t.Fatalf("Failed to connect to test db: %v", err)
	}

	_ = database.ResetDB(db)
	_ = database.MigrateUp(db)

	// We need a valid user to own the notes
	user, err := users.CreateUser(context.Background(), db, "notes@atria.local", "Note Tester", "pass", core.RoleUser)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return db, user
}

func TestNoteLifecycle(t *testing.T) {
	db, user := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// --- 1. Create a Note (with deep virtual path) ---
	title := "Solar Power Config"
	path := "/home/solar"
	content := "# Solar Config\nOutput: 5kW"

	noteEntity, err := notes.CreateNote(ctx, db, user.ID, title, path, content)
	if err != nil {
		t.Fatalf("Expected no error creating note, got: %v", err)
	}
	if noteEntity.Title != title {
		t.Errorf("Expected title %s, got %s", title, noteEntity.Title)
	}

	// --- 2. Retrieve Content ---
	fetchedContent, err := notes.GetNoteContent(ctx, db, noteEntity.ID)
	if err != nil {
		t.Fatalf("Failed to get note content: %v", err)
	}
	if fetchedContent != content {
		t.Errorf("Content mismatch. Expected %q, got %q", content, fetchedContent)
	}

	// --- 3. List Notes (Verify Path Generation via CTE) ---
	list, err := notes.ListNotes(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("Failed to list notes: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("Expected exactly 1 note in list, got %d", len(list))
	}
	// The path should be exactly what we requested
	if list[0].Path != path {
		t.Errorf("Expected path %s, got %s", path, list[0].Path)
	}

	// --- 4. Find Note (By Exact Title) ---
	found, err := notes.FindNotes(ctx, db, user.ID, title, false)
	if err != nil {
		t.Fatalf("Failed to find note: %v", err)
	}
	if len(found) != 1 || found[0].ID != noteEntity.ID {
		t.Errorf("Expected to find note by title, got %d results", len(found))
	}

	// --- 5. Soft Delete ---
	err = notes.DeleteEntity(ctx, db, user.ID, noteEntity.ID, false, false)
	if err != nil {
		t.Fatalf("Failed to soft delete note: %v", err)
	}

	// Verify it's hidden from normal listing
	activeList, _ := notes.ListNotes(ctx, db, user.ID)
	if len(activeList) != 0 {
		t.Errorf("Expected list to be empty after soft delete, got %d items", len(activeList))
	}

	// Verify it CAN be found if we include the trash
	trashList, _ := notes.FindNotes(ctx, db, user.ID, noteEntity.ID.String(), true)
	if len(trashList) != 1 {
		t.Errorf("Expected to find note in trash, got %d results", len(trashList))
	}

	// --- 6. Hard Delete ---
	err = notes.DeleteEntity(ctx, db, user.ID, noteEntity.ID, false, true)
	if err != nil {
		t.Fatalf("Failed to hard delete note: %v", err)
	}

	// Verify it's permanently gone, even from the trash
	finalList, _ := notes.FindNotes(ctx, db, user.ID, noteEntity.ID.String(), true)
	if len(finalList) != 0 {
		t.Errorf("Expected note to be permanently deleted, but it was still found")
	}
}
