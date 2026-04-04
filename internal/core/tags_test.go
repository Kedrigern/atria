package core_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"atria/internal/core"
	"atria/internal/database"
	"atria/internal/notes"
	"atria/internal/users"

	"github.com/joho/godotenv"
)

func setupTagDB(t *testing.T) (*sql.DB, *core.User) {
	_ = godotenv.Load("../../.env")
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	db, err := database.InitDB(dsn)
	if err != nil {
		t.Fatalf("Failed to connect to test db: %v", err)
	}

	_ = database.ResetDB(db)
	_ = database.MigrateUp(db)

	user, _ := users.CreateUser(context.Background(), db, "tags@test.local", "Tag Tester", "pass", core.RoleUser)
	return db, user
}

func TestTagLifecycle(t *testing.T) {
	db, user := setupTagDB(t)
	defer db.Close()
	ctx := context.Background()

	// 1. Create a dummy note to tag
	note, err := notes.CreateNote(ctx, db, user.ID, "Tagging Test Note", "/", "Content")
	if err != nil {
		t.Fatalf("Failed to create test note: %v", err)
	}

	// 2. Test Tag Creation (User Tag)
	tagName := "golang"
	tag, err := core.CreateTag(ctx, db, user.ID, tagName, false)
	if err != nil {
		t.Fatalf("Failed to create tag: %v", err)
	}
	if tag.Name != tagName || tag.IsSystem != false {
		t.Errorf("Tag metadata mismatch: got name %s, is_system %v", tag.Name, tag.IsSystem)
	}

	// 3. Test Tag Creation (System Tag)
	sysTag, err := core.CreateTag(ctx, db, user.ID, "inbox", true)
	if err != nil {
		t.Fatalf("Failed to create system tag: %v", err)
	}
	if sysTag.IsSystem != true {
		t.Error("Tag should be marked as system")
	}

	// 4. Test Attaching Tag
	err = core.AttachTag(ctx, db, note.ID, tag.ID)
	if err != nil {
		t.Fatalf("Failed to attach tag: %v", err)
	}

	// Test idempotency (Attach same tag again)
	err = core.AttachTag(ctx, db, note.ID, tag.ID)
	if err != nil {
		t.Errorf("AttachTag should be idempotent (ON CONFLICT DO NOTHING), got: %v", err)
	}

	// 5. Test AttachTagByTitle (including auto-creation)
	newTagName := "database"
	err = core.AttachTagByTitle(ctx, db, user.ID, note.ID, newTagName)
	if err != nil {
		t.Fatalf("AttachTagByTitle failed: %v", err)
	}

	// 6. Verify Tags Retrieval
	tags, err := core.GetEntityTags(ctx, db, note.ID)
	if err != nil {
		t.Fatalf("GetEntityTags failed: %v", err)
	}

	// We expect 2 tags: "golang" and "database"
	if len(tags) != 2 {
		t.Errorf("Expected 2 tags for entity, got %d", len(tags))
	}

	foundGolang := false
	for _, t := range tags {
		if t.Name == "golang" {
			foundGolang = true
		}
	}
	if !foundGolang {
		t.Error("Tag 'golang' not found in entity tags")
	}
}
