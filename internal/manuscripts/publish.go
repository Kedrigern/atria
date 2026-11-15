package manuscripts

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/uuid/v5"

	"atria/internal/core"
	"atria/internal/ebooks"
)

// Publish makes an e-book from the manuscript: an EPUB snapshot with the
// title, author, language and description of the manuscript, linked to it.
// Each call makes a new e-book; the manuscript can change after it.
func Publish(ctx context.Context, db *sql.DB, ownerID uuid.UUID, m *Manuscript) (*core.Ebook, error) {
	dir, err := os.MkdirTemp("", "atria-publish-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	filename := m.Slug + ".epub"
	path := filepath.Join(dir, filename)
	if _, err := WriteEPUB(m, path); err != nil {
		return nil, fmt.Errorf("write EPUB: %w", err)
	}
	return ebooks.Import(ctx, db, ownerID, ebooks.ImportInput{
		Title:           m.Title,
		Description:     m.Description,
		Author:          m.Author,
		Language:        m.Language,
		Path:            path,
		Filename:        filename,
		SourceEntityIDs: []uuid.UUID{m.ID},
	})
}
