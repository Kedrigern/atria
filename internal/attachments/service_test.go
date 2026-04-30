package attachments_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"atria/internal/attachments"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddAttachmentDeduplication(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "test_image.txt")
	err := os.WriteFile(testFilePath, []byte("fake binary data"), 0644)
	require.NoError(t, err)

	t.Setenv("STORAGE_PATH", filepath.Join(tempDir, "storage"))

	att1, err := attachments.AddAttachment(ctx, db, user.ID, testFilePath)
	require.NoError(t, err)
	assert.NotEmpty(t, att1.ID)
	assert.Equal(t, "test_image.txt", att1.Filename)

	att2, err := attachments.AddAttachment(ctx, db, user.ID, testFilePath)
	require.NoError(t, err)

	assert.Equal(t, att1.ID, att2.ID)
	assert.Equal(t, att1.FileHash, att2.FileHash)

	list, err := attachments.ListAttachments(ctx, db, user.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1, "Očekáváme pouze jeden záznam v DB díky deduplikaci")
}
