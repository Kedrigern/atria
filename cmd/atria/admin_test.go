package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"atria/internal/core"
	"atria/internal/notes"
	"atria/internal/testutil"
	"atria/internal/users"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	listFormat, usageUser, userRmForce, pruneDays, pruneDryRun = "table", "", false, 0, false
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func TestAdminCommands(t *testing.T) {
	db, owner := testutil.SetupTestDB(t)
	t.Setenv("DATABASE_URL", testutil.TestDatabaseURL(t))
	t.Setenv("STORAGE_PATH", t.TempDir())
	ctx := context.Background()

	other, err := users.CreateUser(ctx, db, "other@atria.local", "Other", "pass", core.RoleUser)
	require.NoError(t, err)
	_, err = notes.CreateNote(ctx, db, owner.ID, "Owner note", "/", "some text")
	require.NoError(t, err)

	t.Run("db usage json", func(t *testing.T) {
		out, err := runCLI(t, "db", "usage", "-f", "json")
		require.NoError(t, err)
		var got struct {
			Users []struct {
				Email     string `json:"email"`
				NoteCount int    `json:"note_count"`
			} `json:"users"`
			DatabaseBytes int64 `json:"database_bytes"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &got), out)
		require.Len(t, got.Users, 2)
		assert.Equal(t, owner.Email, got.Users[0].Email, "largest user first")
		assert.Equal(t, 1, got.Users[0].NoteCount)
		assert.Positive(t, got.DatabaseBytes)
	})

	t.Run("db usage for one user", func(t *testing.T) {
		out, err := runCLI(t, "db", "usage", "--user", other.Email)
		require.NoError(t, err)
		assert.Contains(t, out, other.Email)
		assert.NotContains(t, out, owner.Email)
		assert.Contains(t, out, "Database size")
	})

	t.Run("user rm needs --force", func(t *testing.T) {
		out, err := runCLI(t, "user", "rm", other.Email)
		require.NoError(t, err)
		assert.Contains(t, out, "Nothing deleted")
		_, err = core.FindUser(ctx, db, other.Email)
		assert.NoError(t, err, "user must still exist")

		out, err = runCLI(t, "user", "rm", other.Email, "--force")
		require.NoError(t, err)
		assert.Contains(t, out, "were deleted")
		_, err = core.FindUser(ctx, db, other.Email)
		assert.Error(t, err)
	})

	t.Run("rss prune dry run", func(t *testing.T) {
		out, err := runCLI(t, "rss", "prune", "--dry-run", "--days", "30")
		require.NoError(t, err)
		assert.Contains(t, out, "Would shrink 0 read items older than 30 days")
	})
}
