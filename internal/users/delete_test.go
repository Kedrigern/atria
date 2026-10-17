package users_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"atria/internal/attachments"
	"atria/internal/core"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteUserRemovesUnsharedAttachmentFiles(t *testing.T) {
	db, keeper := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	storage := t.TempDir()
	t.Setenv("STORAGE_PATH", storage)

	leaver, err := users.CreateUser(ctx, db, "leaver@atria.local", "Leaver", "pass", core.RoleUser)
	require.NoError(t, err)

	write := func(name, content string) string {
		p := filepath.Join(t.TempDir(), name)
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
		return p
	}
	private, err := attachments.AddAttachment(ctx, db, leaver.ID, write("private.txt", "only mine"), "private.txt")
	require.NoError(t, err)
	shared, err := attachments.AddAttachment(ctx, db, leaver.ID, write("shared.txt", "same bytes"), "shared.txt")
	require.NoError(t, err)
	_, err = attachments.AddAttachment(ctx, db, keeper.ID, write("copy.txt", "same bytes"), "copy.txt")
	require.NoError(t, err)

	require.NoError(t, users.DeleteUser(ctx, db, "LEAVER@atria.local"))

	assert.NoFileExists(t, filepath.Join(storage, private.DiskPath), "unshared file must be removed")
	assert.FileExists(t, filepath.Join(storage, shared.DiskPath), "file still used by another user must stay")

	var remaining int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE owner_id = $1`, leaver.ID).Scan(&remaining))
	assert.Zero(t, remaining)

	err = users.DeleteUser(ctx, db, "leaver@atria.local")
	msg, ok := core.UserMessage(err)
	assert.True(t, ok)
	assert.Contains(t, msg, "user not found")
}
