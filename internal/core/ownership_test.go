package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"atria/internal/attachments"
	"atria/internal/core"
	"atria/internal/links"
	"atria/internal/notes"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Read helpers take the owner and must not return another user's data even
// when given a valid ID.
func TestReadHelpersAreOwnerScoped(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	t.Setenv("STORAGE_PATH", t.TempDir())

	other, err := users.CreateUser(ctx, db, "other@atria.local", "Other", "pass", core.RoleUser)
	require.NoError(t, err)

	note, err := notes.CreateNote(ctx, db, owner.ID, "Private", "/", "secret body")
	require.NoError(t, err)
	target, err := notes.CreateNote(ctx, db, owner.ID, "Target", "/", "x")
	require.NoError(t, err)
	foreign, err := notes.CreateNote(ctx, db, other.ID, "Foreign", "/", "y")
	require.NoError(t, err)

	require.NoError(t, core.AttachTagByTitle(ctx, db, owner.ID, note.ID, "secret"))
	require.NoError(t, links.AddLink(ctx, db, note.ID, target.ID, "user_defined"))
	// A cross-user link, as the old missing ownership check could create.
	_, err = db.Exec(`INSERT INTO rel_entity_links (source_id, target_id, context) VALUES ($1, $2, 'x')`, foreign.ID, note.ID)
	require.NoError(t, err)

	file := filepath.Join(t.TempDir(), "a.txt")
	require.NoError(t, os.WriteFile(file, []byte("attachment"), 0o600))
	att, err := attachments.AddAttachment(ctx, db, owner.ID, file, "a.txt")
	require.NoError(t, err)
	require.NoError(t, attachments.LinkAttachment(ctx, db, note.ID, att.ID))

	t.Run("owner sees everything", func(t *testing.T) {
		content, err := notes.GetNoteContent(ctx, db, owner.ID, note.ID)
		require.NoError(t, err)
		assert.Equal(t, "secret body", content)

		tags, err := core.GetEntityTags(ctx, db, owner.ID, note.ID)
		require.NoError(t, err)
		assert.Len(t, tags, 1)

		out, in, err := links.GetEntityLinks(ctx, db, owner.ID, note.ID)
		require.NoError(t, err)
		assert.Len(t, out, 1)
		assert.Empty(t, in, "the foreign user's link must not reveal their note title")

		atts, err := attachments.GetEntityAttachments(ctx, db, owner.ID, note.ID)
		require.NoError(t, err)
		assert.Len(t, atts, 1)
	})

	t.Run("other user sees nothing", func(t *testing.T) {
		_, err := notes.GetNoteContent(ctx, db, other.ID, note.ID)
		assert.Error(t, err)

		tags, err := core.GetEntityTags(ctx, db, other.ID, note.ID)
		require.NoError(t, err)
		assert.Empty(t, tags)

		out, in, err := links.GetEntityLinks(ctx, db, other.ID, note.ID)
		require.NoError(t, err)
		assert.Empty(t, out)
		assert.Empty(t, in)

		atts, err := attachments.GetEntityAttachments(ctx, db, other.ID, note.ID)
		require.NoError(t, err)
		assert.Empty(t, atts)
	})
}

// Updates that match no row used to report success.
func TestUpdatesReportMissingEntities(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	other, err := users.CreateUser(ctx, db, "other2@atria.local", "Other", "pass", core.RoleUser)
	require.NoError(t, err)
	note, err := notes.CreateNote(ctx, db, owner.ID, "Mine", "/", "x")
	require.NoError(t, err)

	notFound := func(t *testing.T, err error) {
		t.Helper()
		require.Error(t, err)
		msg, ok := core.UserMessage(err)
		assert.True(t, ok, "expected a user-facing error, got %v", err)
		assert.Contains(t, msg, "not found")
	}

	notFound(t, core.RenameEntity(ctx, db, other.ID, note.ID, "Hijacked"))
	notFound(t, core.UpdateVisibility(ctx, db, other.ID, note.ID, core.VisibilityPublic))
	notFound(t, core.SoftDeleteEntity(ctx, db, other.ID, note.ID))
	notFound(t, core.RenameEntity(ctx, db, owner.ID, core.NewUUID(), "Unknown"))

	require.NoError(t, core.SoftDeleteEntity(ctx, db, owner.ID, note.ID))
	notFound(t, core.SoftDeleteEntity(ctx, db, owner.ID, note.ID))
	notFound(t, core.RenameEntity(ctx, db, owner.ID, note.ID, "Renamed in trash"))
}
