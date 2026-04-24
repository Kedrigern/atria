package export

import (
	"context"
	"database/sql"
	"fmt"
	"html"

	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/notes"

	"github.com/bmaupin/go-epub"
)

// ExportEPUB bundles multiple articles and notes into a single EPUB file.
func ExportEPUB(ctx context.Context, db *sql.DB, items []core.EntitySummary, outPath string) error {
	e := epub.NewEpub("Atria Export")
	e.SetAuthor("Atria Mind Palace")

	for _, item := range items {
		var htmlBody string

		if item.Type == core.TypeArticle {
			// Load cleaned article HTML.
			content, err := articles.GetArticleHTML(ctx, db, item.ID)
			if err != nil {
				fmt.Printf("⚠️ Skipping '%s': %v\n", item.Title, err)
				continue
			}
			htmlBody = content

		} else if item.Type == core.TypeNote {
			// Load note Markdown and render it to HTML.
			content, err := notes.GetNoteContent(ctx, db, item.ID)
			if err != nil {
				fmt.Printf("⚠️ Skipping '%s': %v\n", item.Title, err)
				continue
			}
			htmlStr, _, err := core.RenderMarkdown([]byte(content))
			if err != nil {
				fmt.Printf("⚠️ Skipping '%s': markdown render failed: %v\n", item.Title, err)
				continue
			}
			htmlBody = htmlStr
		} else {
			// This should not happen because unsupported types are filtered earlier.
			continue
		}

		// EPUB sections must contain valid (X)HTML body content.
		escapedTitle := html.EscapeString(item.Title)
		sectionHTML := fmt.Sprintf("<h1>%s</h1>\n<div>%s</div>", escapedTitle, htmlBody)

		// Přidáme sekci (kapitolu) do knihy
		_, err := e.AddSection(sectionHTML, item.Title, "", "")
		if err != nil {
			fmt.Printf("⚠️ Failed to process section '%s': %v\n", item.Title, err)
			continue
		}
	}

	// Save the final EPUB file.
	return e.Write(outPath)
}
