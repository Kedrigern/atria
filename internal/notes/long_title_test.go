package notes_test

import (
	"context"
	"strings"
	"testing"

	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/testutil"
)

// A Czech title whose slug crosses the 100-byte mark in the middle of a
// two-byte character. Byte slicing used to produce invalid UTF-8, which
// PostgreSQL rejects.
var longCzechTitle = strings.Repeat("a", 99) + "čeština, příliš žluťoučký kůň úpěl ďábelské ódy"

func TestLongMultibyteTitles(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	note, err := notes.CreateNote(ctx, db, user.ID, "Short title", "/", "content")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	if err := core.RenameEntity(ctx, db, user.ID, note.ID, longCzechTitle); err != nil {
		t.Fatalf("rename to a long Czech title failed: %v", err)
	}

	var title, slug string
	if err := db.QueryRowContext(ctx, `SELECT title, slug FROM entities WHERE id = $1`, note.ID).Scan(&title, &slug); err != nil {
		t.Fatalf("load entity: %v", err)
	}
	if title != longCzechTitle {
		t.Errorf("title not stored intact: %q", title)
	}
	if n := len([]rune(slug)); n != core.MaxSlugLength {
		t.Errorf("slug should be cut to %d characters, got %d", core.MaxSlugLength, n)
	}

	if _, err := notes.CreateNote(ctx, db, user.ID, "Child", "/"+longCzechTitle, "x"); err != nil {
		t.Fatalf("create note in a folder with a long Czech name failed: %v", err)
	}
}
