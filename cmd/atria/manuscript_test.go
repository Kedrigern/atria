package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A converted book as the PDF converter writes it: front matter, a title,
// page markers that start their page, and a missing page 4.
const honneth = `---
title: "Pracující suverén"
subtitle: "Normativní teorie práce"
author: "Axel Honneth"
translator: "Martin Profant"
lang: "cs"
---

# Pracující suverén: Normativní teorie práce

Normativní teorie práce

<!-- page 2 -->

Z německého originálu přeložil Martin Profant.

<!-- page 3 -->

## Úvod

V liberalismu se mluví o právech, svobodách a společenskosti.

<!-- page 5 -->

Práce je pro většinu lidí hlavní částí bdělého života.
`

// The user imports a folder of books in one command. Each book gets its
// check; a file that is not text does not stop the others. Importing again
// fails unless the user asks to replace, and the replaced book keeps its ID.
func TestManuscriptImportOfSeveralBooks(t *testing.T) {
	_, user := testutil.SetupTestDB(t)
	t.Setenv("DATABASE_URL", testutil.TestDatabaseURL(t))
	t.Setenv("ATRIA_USER", user.Email)
	t.Setenv("STORAGE_PATH", t.TempDir())

	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		return path
	}
	book := write("Honneth-Pracujici_suveren-2026.md", honneth)
	notes := write("zapisky.md", "# Zápisky z mlýna\n\nMlýn stál u řeky.\n")
	untitled := write("Vynaseni_smrti.md", "Vynášení smrti o Smrtné neděli.\n")
	scan := write("sken.md", "Stra\xffna\n")

	out, err := runCLI(t, "manuscript", "import", "--path", "/knihy", book, notes, untitled, scan)
	require.Error(t, err, "a file that was not imported makes the command fail")
	assert.Contains(t, err.Error(), "1 of 4 files were not imported")
	assert.Contains(t, out, "  Pracující suverén\n", "the title comes from the front matter, not from the longer # heading")
	assert.Contains(t, out, "Zápisky z mlýna", "without front matter the title is the first # heading")
	assert.Contains(t, out, "Vynaseni_smrti", "without a heading the title is the file name")
	assert.Contains(t, out, book+": 1 chapters, 0 footnotes,", "each imported book gets the check of 'atria check'")
	assert.Contains(t, out, "5 original pages, 1 warnings", "pages 1 (title) to 5, with page 4 missing")
	assert.Contains(t, out, "page 4 is missing between 3 and 5")
	assert.Contains(t, out, "failed   "+scan+": the manuscript is not UTF-8 text")

	list := mustCLI(t, "manuscript", "list", "-f", "csv")
	assert.Contains(t, list, "Pracující suverén,/knihy,Axel Honneth,cs,", "author and language come from the front matter")
	assert.Contains(t, list, "Zápisky z mlýna,/knihy,,,")
	assert.NotContains(t, list, "sken", "the failed file is not stored")

	shown := mustCLI(t, "manuscript", "show", "Pracující suverén")
	assert.Equal(t, honneth, shown, "the file is stored exactly as imported, front matter included")

	out, err = runCLI(t, "manuscript", "import", "--path", "/knihy", book)
	require.Error(t, err, "the same title in the same folder is not imported twice")
	assert.Contains(t, out, `a manuscript named "Pracující suverén" is already in /knihy`)

	// The list is sorted by title, so the first row after the header is Honneth.
	firstRow := strings.SplitN(list, "\n", 3)[1]
	require.Contains(t, firstRow, "Pracující suverén")
	firstID := strings.SplitN(firstRow, ",", 2)[0]
	corrected := strings.Replace(honneth, "<!-- page 5 -->", "<!-- page 4 -->\n\nChybějící strana.\n\n<!-- page 5 -->", 1)
	require.NoError(t, os.WriteFile(book, []byte(corrected), 0o644))
	out = mustCLI(t, "manuscript", "import", "--path", "/knihy", "--replace", book)
	assert.Contains(t, out, "replaced", "--replace updates the existing manuscript")
	assert.Contains(t, out, "5 original pages, 0 warnings", "the corrected file has no gap")
	assert.Equal(t, corrected, mustCLI(t, "manuscript", "show", "Pracující suverén"), "the new content replaced the old one")
	assert.Contains(t, mustCLI(t, "manuscript", "list", "-f", "csv"), firstID, "the replaced manuscript keeps its ID")
}
