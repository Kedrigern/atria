package export

import (
	"context"
	"database/sql"
	"fmt"

	"atria/internal/articles"
	"atria/internal/core"
	"atria/internal/notes"

	"github.com/bmaupin/go-epub"
	"github.com/russross/blackfriday/v2"
)

// ExportEPUB bundles multiple articles and notes into a single EPUB file.
func ExportEPUB(ctx context.Context, db *sql.DB, items []core.EntitySummary, outPath string) error {
	e := epub.NewEpub("Atria Export")
	e.SetAuthor("Atria Mind Palace")

	for _, item := range items {
		var htmlBody string

		if item.Type == core.TypeArticle {
			// Získáme vyčištěné HTML článku
			content, err := articles.GetArticleHTML(ctx, db, item.ID)
			if err != nil {
				fmt.Printf("⚠️ Skipping '%s': %v\n", item.Title, err)
				continue
			}
			htmlBody = content

		} else if item.Type == core.TypeNote {
			// Získáme Markdown poznámky a převedeme ho na HTML
			content, err := notes.GetNoteContent(ctx, db, item.ID)
			if err != nil {
				fmt.Printf("⚠️ Skipping '%s': %v\n", item.Title, err)
				continue
			}
			htmlBody = string(blackfriday.Run([]byte(content)))
		} else {
			// Tento fallback by neměl nastat, protože to filtrujeme už v CLI
			continue
		}

		// EPUB vyžaduje, aby obsah každé kapitoly byl validní (X)HTML uvnitř body tagu.
		// Obalíme obsah a přidáme nadpis <h1>, aby kapitola vypadala hezky i ve čtečce.
		sectionHTML := fmt.Sprintf("<h1>%s</h1>\n<div>%s</div>", item.Title, htmlBody)

		// Přidáme sekci (kapitolu) do knihy
		_, err := e.AddSection(sectionHTML, item.Title, "", "")
		if err != nil {
			fmt.Printf("⚠️ Failed to process section '%s': %v\n", item.Title, err)
			continue
		}
	}

	// Uložíme výsledný soubor
	return e.Write(outPath)
}
