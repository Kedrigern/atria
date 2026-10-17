package export

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"log"
	"strings"

	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/notes"

	"github.com/bmaupin/go-epub"
	"github.com/gofrs/uuid/v5"
)

// EbookMetadata contains the EPUB metadata supplied by an e-book creator.
type EbookMetadata struct {
	Title              string
	Description        string
	Author             string
	Language           string
	CoverPath          string
	IncludeChapterTags bool
}

// ExportEPUB bundles multiple articles and notes into a single EPUB file.
func ExportEPUB(ctx context.Context, db *sql.DB, ownerID uuid.UUID, items []core.EntitySummary, outPath string) error {
	return ExportEbookEPUB(ctx, db, ownerID, items, outPath, EbookMetadata{
		Title:  "Atria Export",
		Author: "Atria Mind Palace",
	})
}

// ExportEbookEPUB bundles article and note snapshots into an EPUB with custom
// metadata. Items not owned by ownerID are skipped.
func ExportEbookEPUB(ctx context.Context, db *sql.DB, ownerID uuid.UUID, items []core.EntitySummary, outPath string, metadata EbookMetadata) error {
	title := strings.TrimSpace(metadata.Title)
	if title == "" {
		return fmt.Errorf("e-book title is required")
	}

	e := epub.NewEpub(title)
	if metadata.Author != "" {
		e.SetAuthor(metadata.Author)
	}
	if metadata.Description != "" {
		e.SetDescription(metadata.Description)
	}
	if metadata.Language != "" {
		e.SetLang(metadata.Language)
	}
	if metadata.CoverPath != "" {
		coverPath, err := e.AddImage(metadata.CoverPath, "")
		if err != nil {
			return fmt.Errorf("add cover image: %w", err)
		}
		e.SetCover(coverPath, "")
	}

	chapterCount := 0
	for _, item := range items {
		var htmlBody string

		switch item.Type {
		case core.TypeArticle:
			content, err := articles.GetArticleHTML(ctx, db, ownerID, item.ID)
			if err != nil {
				log.Printf("WARNING: EPUB export: Skipping '%s': %v", item.Title, err)
				continue
			}
			// Saved pages are third-party HTML; e-readers may run scripts.
			htmlBody = core.SanitizeHTML(content)
		case core.TypeNote:
			content, err := notes.GetNoteContent(ctx, db, ownerID, item.ID)
			if err != nil {
				log.Printf("WARNING: EPUB export: Skipping '%s': %v", item.Title, err)
				continue
			}
			htmlStr, _, err := core.RenderMarkdown([]byte(content))
			if err != nil {
				log.Printf("WARNING: EPUB export: Skipping '%s': markdown render failed: %v", item.Title, err)
				continue
			}
			htmlBody = htmlStr
		default:
			continue
		}

		metadataHTML := ""
		if metadata.IncludeChapterTags {
			metadataHTML = chapterTags(ctx, db, ownerID, item.ID)
		}
		sectionHTML := fmt.Sprintf("<h1>%s</h1>%s<div>%s</div>", html.EscapeString(item.Title), metadataHTML, htmlBody)
		if _, err := e.AddSection(sectionHTML, item.Title, "", ""); err != nil {
			log.Printf("WARNING: EPUB export: Failed to process section '%s': %v", item.Title, err)
			continue
		}
		chapterCount++
	}
	if chapterCount == 0 {
		return fmt.Errorf("no source content could be exported")
	}

	return e.Write(outPath)
}

func chapterTags(ctx context.Context, db *sql.DB, ownerID, entityID uuid.UUID) string {
	tags, err := core.GetEntityTags(ctx, db, ownerID, entityID)
	if err != nil || len(tags) == 0 {
		return ""
	}

	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, "#"+tag.Name)
	}
	return fmt.Sprintf("<p><em>Tags: %s</em></p>", html.EscapeString(strings.Join(names, ", ")))
}
