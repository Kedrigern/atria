// Package manuscripts stores books as Markdown files: one file per book,
// kept exactly as uploaded. See docs/architecture/manuscripts.md.
package manuscripts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofrs/uuid/v5"
	"gopkg.in/yaml.v3"

	"atria/internal/core"
)

// MaxBytes limits the size of one manuscript. The longest sample book has
// about 0.7 MB.
const MaxBytes = 10 << 20

// Input is the content and metadata of a manuscript.
type Input struct {
	Title       string
	Author      string
	Language    string
	Description string
	Markdown    string
}

// Manuscript is a stored manuscript with its content.
type Manuscript struct {
	core.Entity
	Author      string
	Language    string
	Description string
	Markdown    string
}

// Summary is a manuscript without its content, for lists.
type Summary struct {
	ID         uuid.UUID
	Title      string
	Path       string
	Author     string
	Language   string
	Characters int
	UpdatedAt  time.Time
}

// ImportState says what Import did with a file.
type ImportState string

const (
	Created  ImportState = "created"
	Replaced ImportState = "replaced"
)

// FromFile reads the metadata of a Markdown file. The title comes from the
// front matter, then from the first # heading, then from the file name. The
// content stays as it is, front matter included, so that an export gives
// back the same file.
func FromFile(filename string, src []byte) Input {
	in := Input{Markdown: string(src)}
	body := in.Markdown
	if meta, rest, ok := splitFrontMatter(in.Markdown); ok {
		body = rest
		in.Title = metaString(meta, "title")
		in.Author = metaString(meta, "author")
		in.Language = metaString(meta, "lang", "language")
		in.Description = metaString(meta, "description", "subtitle")
	}
	if in.Title == "" {
		in.Title = firstTitle(body)
	}
	if in.Title == "" {
		in.Title = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	return in
}

func splitFrontMatter(src string) (map[string]any, string, bool) {
	if !strings.HasPrefix(src, "---\n") && !strings.HasPrefix(src, "---\r\n") {
		return nil, src, false
	}
	lines := strings.SplitAfter(src, "\n")
	for i := 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r\n")
		if line == "---" || line == "..." {
			var meta map[string]any
			if err := yaml.Unmarshal([]byte(strings.Join(lines[1:i], "")), &meta); err != nil {
				return nil, src, false
			}
			return meta, strings.Join(lines[i+1:], ""), true
		}
	}
	return nil, src, false
}

// metaString returns the first of the keys that has a value. A list, such
// as several authors, is joined with commas.
func metaString(meta map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := meta[key].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case []any:
			var parts []string
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					parts = append(parts, strings.TrimSpace(s))
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, ", ")
			}
		}
	}
	return ""
}

func firstTitle(body string) string {
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimRight(trimmed[2:], "#"))
		}
	}
	return ""
}

func (in *Input) validate() error {
	in.Title = core.LimitRunes(strings.TrimSpace(in.Title), core.MaxTitleLength)
	in.Author = core.LimitRunes(strings.TrimSpace(in.Author), 255)
	in.Language = core.LimitRunes(strings.TrimSpace(in.Language), 35)
	in.Description = strings.TrimSpace(in.Description)
	switch {
	case in.Title == "":
		return core.NewUserError("manuscript title is required")
	case strings.TrimSpace(in.Markdown) == "":
		return core.NewUserError("the manuscript is empty")
	case len(in.Markdown) > MaxBytes:
		return core.NewUserError("the manuscript is larger than %d MB", MaxBytes>>20)
	case !utf8.ValidString(in.Markdown) || strings.ContainsRune(in.Markdown, 0):
		return core.NewUserError("the manuscript is not UTF-8 text")
	}
	return nil
}

func slug(title string) string {
	s := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	return core.LimitRunes(s, core.MaxSlugLength)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Create stores a new manuscript in the folder path.
func Create(ctx context.Context, db *sql.DB, ownerID uuid.UUID, path string, in Input) (*Manuscript, error) {
	m, _, err := Import(ctx, db, ownerID, path, in, false)
	return m, err
}

// Import stores a manuscript in the folder path. If a manuscript with the
// same title is in that folder, Import fails, or with replace it replaces the
// content and metadata of that manuscript; its ID, tags and links stay.
func Import(ctx context.Context, db *sql.DB, ownerID uuid.UUID, path string, in Input, replace bool) (*Manuscript, ImportState, error) {
	if err := in.validate(); err != nil {
		return nil, "", err
	}
	path = cleanPath(path)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("begin manuscript import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	parentID, err := core.EnsurePath(ctx, tx, ownerID, path)
	if err != nil {
		return nil, "", err
	}

	var existing uuid.UUID
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM entities
		WHERE owner_id = $1 AND type = $2 AND title = $3 AND deleted_at IS NULL
		  AND parent_id IS NOT DISTINCT FROM $4`,
		ownerID, core.TypeManuscript, in.Title, parentID).Scan(&existing)
	switch {
	case err == nil && !replace:
		return nil, "", core.NewUserError("a manuscript named %q is already in %s", in.Title, path)
	case err == nil:
		if err := updateTx(ctx, tx, ownerID, existing, path, parentID, in); err != nil {
			return nil, "", err
		}
		if err := tx.Commit(); err != nil {
			return nil, "", fmt.Errorf("commit manuscript import: %w", err)
		}
		m, err := Get(ctx, db, ownerID, existing)
		return m, Replaced, err
	case !errors.Is(err, sql.ErrNoRows):
		return nil, "", fmt.Errorf("find manuscript: %w", err)
	}

	id := core.NewUUID()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO entities (id, parent_id, owner_id, type, visibility, title, slug, path, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		id, parentID, ownerID, core.TypeManuscript, core.VisibilityPrivate, in.Title, slug(in.Title), path, now); err != nil {
		return nil, "", fmt.Errorf("insert manuscript entity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO manuscripts (id, author, language, description, markdown_content)
		VALUES ($1, $2, $3, $4, $5)`,
		id, nullable(in.Author), nullable(in.Language), nullable(in.Description), in.Markdown); err != nil {
		return nil, "", fmt.Errorf("insert manuscript: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("commit manuscript import: %w", err)
	}
	m, err := Get(ctx, db, ownerID, id)
	return m, Created, err
}

// Update replaces the content, metadata and folder of a manuscript.
func Update(ctx context.Context, db *sql.DB, ownerID, id uuid.UUID, path string, in Input) error {
	if err := in.validate(); err != nil {
		return err
	}
	path = cleanPath(path)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin manuscript update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	parentID, err := core.EnsurePath(ctx, tx, ownerID, path)
	if err != nil {
		return err
	}
	if err := updateTx(ctx, tx, ownerID, id, path, parentID, in); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit manuscript update: %w", err)
	}
	return nil
}

func updateTx(ctx context.Context, tx *sql.Tx, ownerID, id uuid.UUID, path string, parentID *uuid.UUID, in Input) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE entities
		SET title = $1, slug = $2, path = $3, parent_id = $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = $5 AND owner_id = $6 AND type = $7 AND deleted_at IS NULL`,
		in.Title, slug(in.Title), path, parentID, id, ownerID, core.TypeManuscript)
	if err != nil {
		return fmt.Errorf("update manuscript entity: %w", err)
	}
	if err := core.RequireAffected(result, "manuscript not found"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE manuscripts SET author = $1, language = $2, description = $3, markdown_content = $4
		WHERE id = $5`,
		nullable(in.Author), nullable(in.Language), nullable(in.Description), in.Markdown, id); err != nil {
		return fmt.Errorf("update manuscript: %w", err)
	}
	return nil
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	return path
}

// Get loads a manuscript of the owner, with its content.
func Get(ctx context.Context, db *sql.DB, ownerID, id uuid.UUID) (*Manuscript, error) {
	var m Manuscript
	var author, language, description sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT e.id, e.short_id, e.parent_id, e.owner_id, e.visibility, e.title, e.slug,
		       COALESCE(entity_path(e.parent_id), '/'), e.created_at, e.updated_at,
		       m.author, m.language, m.description, m.markdown_content
		FROM manuscripts m JOIN entities e ON e.id = m.id
		WHERE e.id = $1 AND e.owner_id = $2 AND e.deleted_at IS NULL`, id, ownerID).Scan(
		&m.ID, &m.ShortID, &m.ParentID, &m.OwnerID, &m.Visibility, &m.Title, &m.Slug,
		&m.Path, &m.CreatedAt, &m.UpdatedAt,
		&author, &language, &description, &m.Markdown)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NewUserError("manuscript not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load manuscript: %w", err)
	}
	m.Type = core.TypeManuscript
	m.Author, m.Language, m.Description = author.String, language.String, description.String
	return &m, nil
}

// List returns the owner's manuscripts by folder and title.
func List(ctx context.Context, db *sql.DB, ownerID uuid.UUID) ([]Summary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT e.id, e.title, COALESCE(entity_path(e.parent_id), '/') AS path,
		       COALESCE(m.author, ''), COALESCE(m.language, ''),
		       char_length(m.markdown_content), e.updated_at
		FROM manuscripts m JOIN entities e ON e.id = m.id
		WHERE e.owner_id = $1 AND e.deleted_at IS NULL
		ORDER BY path, e.title`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list manuscripts: %w", err)
	}
	defer rows.Close()
	var list []Summary
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.ID, &s.Title, &s.Path, &s.Author, &s.Language, &s.Characters, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan manuscript: %w", err)
		}
		list = append(list, s)
	}
	return list, rows.Err()
}
