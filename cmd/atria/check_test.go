package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A converted book with one problem of each kind from
// docs/architecture/manuscripts.md §3a, and text that must not be reported.
var convertedBook = []string{
	"# Paměti mlynáře", // 1
	"",
	"<!-- iii -->",
	"Předmluva vydavatele.",
	"<!-- iv -->", // 5
	"",
	"## Kapitola první",
	"",
	"Mlýn stál u řeky a mlynář v něm mlel obilí[^1] pro celou ves.",
	"<!-- 1 -->", // 10
	"Druhého dne přijel kupec z Plzně a přivezl novou mlýnskou ků-",
	"ru, kterou mlynář toužebně čekal.",
	"<!-- 2 -->",
	"Večer se ve mlýně sešli sousedé[^2].",
	"<!-- 4 -->", // 15
	"",
	"### Vzpomínky na otce",
	"",
	"Otec říkal: „Kdo nemele, ten nejí.“ -- a měl pravdu.",
	"<!-- 4 -->", // 20
	"<!-- s. 5 -->",
	"",
	"##### Poznámka",
	"",
	"```", // 25
	"<!-- 99 -->",
	"```",
	"<!-- 6a -->",
	"Konec kapitoly[^3].",
	"<!-- str. 6 -->", // 30
	"",
	"## ",
	"",
	"<!-- ix -->",
	"Dovětek bez čísla strany.", // 35
	"",
	"[^1]: Žito a pšenice.",
	"[^2]: Sousedé z dolní vsi.",
	"[^2]: Druhá definice.",
	"[^4]: Nepoužitá poznámka.", // 40
}

func TestCheckReportsProblemsOfAConvertedBook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paměti.md")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(convertedBook, "\n")+"\n"), 0o644))

	out := mustCLI(t, "check", path)

	assert.Contains(t, out, "2 chapters, 3 footnotes,", "chapters are ## under one # title; footnotes count each label once")
	assert.Contains(t, out, "6 original pages, 10 warnings", "a marker starts its page, so Arabic pages 1 to 6 make 6 pages")
	for _, want := range []string{
		"line 11 [text] a word is broken across lines with a hyphen",
		"line 15 [pages] page 3 is missing between 2 and 4",
		"line 20 [pages] page 4 is marked again",
		"line 23 [headings] the heading level goes from 3 to 5 and skips a level",
		"line 28 [pages] <!-- 6a --> looks like a page marker but has an unknown form",
		"line 29 [footnotes] footnote [^3] has no definition",
		"line 32 [headings] the heading is empty",
		"line 34 [pages] Roman page ix comes after the Arabic page numbers",
		"line 39 [footnotes] footnote [^2] is defined again; the first definition is on line 38",
		"line 40 [footnotes] footnote [^4] is defined but never used",
	} {
		assert.Contains(t, out, want, "each problem of §3a is reported with its line")
	}
	assert.NotContains(t, out, "line 19 ", "a dash -- inside the text is not a broken word")
	// Only the warnings: the summary line has the temporary path, which can
	// contain "99" by chance.
	var warnings []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  line ") {
			warnings = append(warnings, line)
		}
	}
	assert.NotContains(t, strings.Join(warnings, "\n"), "99", "a marker inside a code block is ignored")
	assert.NotContains(t, out, "line 5 ", "Roman front matter pages iii and iv are in order")
}
