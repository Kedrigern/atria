package notes_test

import (
	"context"
	"testing"

	"atria/internal/notes"
	"atria/internal/testutil"
)

func TestFindNotesByShortIDTitleAndPath(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	note, err := notes.CreateNote(ctx, db, user.ID, "Findable", "/projects/atria", "x")
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	shortID := note.ID.String()[len(note.ID.String())-8:]

	for _, identifier := range []string{shortID, "Findable", note.ID.String()} {
		found, err := notes.FindNotes(ctx, db, user.ID, identifier, false)
		if err != nil {
			t.Fatalf("FindNotes(%q): %v", identifier, err)
		}
		if len(found) != 1 || found[0].ID != note.ID {
			t.Fatalf("FindNotes(%q) = %+v, want the note", identifier, found)
		}
		if found[0].Path != "/projects/atria" {
			t.Errorf("path = %q, want /projects/atria", found[0].Path)
		}
	}

	// LIKE wildcards in the identifier must not match anything.
	if found, _ := notes.FindNotes(ctx, db, user.ID, "%", false); len(found) != 0 {
		t.Errorf("wildcard identifier matched %d notes", len(found))
	}
}
