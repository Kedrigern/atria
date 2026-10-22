package core_test

import (
	"context"
	"testing"

	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeTags(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	ctx := context.Background()

	books, err := core.CreateTag(ctx, db, owner.ID, "books", false)
	require.NoError(t, err)
	reading, err := core.CreateTag(ctx, db, owner.ID, "reading", false)
	require.NoError(t, err)
	only, err := notes.CreateNote(ctx, db, owner.ID, "Only books", "/", "x")
	require.NoError(t, err)
	both, err := notes.CreateNote(ctx, db, owner.ID, "Both tags", "/", "x")
	require.NoError(t, err)
	require.NoError(t, core.AttachTag(ctx, db, only.ID, books.ID))
	require.NoError(t, core.AttachTag(ctx, db, both.ID, books.ID))
	require.NoError(t, core.AttachTag(ctx, db, both.ID, reading.ID))

	result, err := core.MergeTags(ctx, db, owner.ID, "#books", "reading")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Moved)
	assert.Equal(t, 1, result.AlreadyTagged)

	_, err = core.FindTagByName(ctx, db, owner.ID, "books")
	assert.Error(t, err, "the merged tag is deleted")
	tagged, err := core.GetTagEntities(ctx, db, owner.ID, "reading")
	require.NoError(t, err)
	assert.Len(t, tagged, 2)
}

func TestMergeTagsRefusesBadInput(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	ctx := context.Background()
	_, err := core.CreateTag(ctx, db, owner.ID, "inbox", true)
	require.NoError(t, err)
	_, err = core.CreateTag(ctx, db, owner.ID, "later", false)
	require.NoError(t, err)
	other, err := users.CreateUser(ctx, db, "other@atria.local", "Other", "a-long-password-1", core.RoleUser)
	require.NoError(t, err)
	_, err = core.CreateTag(ctx, db, other.ID, "secret", false)
	require.NoError(t, err)

	for name, args := range map[string][2]string{
		"same tag":          {"later", "later"},
		"missing tag":       {"nothing", "later"},
		"system tag":        {"inbox", "later"},
		"tag of a user":     {"secret", "later"},
		"into a user's tag": {"later", "secret"},
	} {
		_, err := core.MergeTags(ctx, db, owner.ID, args[0], args[1])
		assert.Error(t, err, name)
	}
	_, err = core.FindTagByName(ctx, db, other.ID, "secret")
	assert.NoError(t, err, "the tag of another user stays")
}
