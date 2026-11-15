package manuscripts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"atria/internal/mdcheck"
)

// Formats that need pandoc. See docs/architecture/manuscripts.md §5.
const (
	FormatDOCX = "docx"
	FormatPDF  = "pdf"
)

// PDFFont is a font that typst has built in, so the server needs no fonts.
// pandoc 3.7 with typst 0.15 fails without a font ("font fallback list must
// not be empty").
const PDFFont = "Libertinus Serif"

// convertTimeout limits one conversion. A book of 300 pages takes about a second.
const convertTimeout = 2 * time.Minute

// safeFilter removes images and raw HTML or other raw formats before pandoc
// writes a DOCX file. Without images pandoc reads no files and fetches no
// URLs. pandoc --sandbox does this for PDF, but the pandoc of Fedora cannot
// write DOCX in the sandbox.
const safeFilter = `function Image(el) return {} end
function RawInline(el) return {} end
function RawBlock(el) return {} end
`

// Formats returns the formats that the installed tools can write. pandoc and
// typst are optional; without them, Atria offers Markdown and EPUB only.
func Formats() []string {
	if _, err := exec.LookPath("pandoc"); err != nil {
		return nil
	}
	formats := []string{FormatDOCX}
	if _, err := exec.LookPath("typst"); err == nil {
		formats = append(formats, FormatPDF)
	}
	return formats
}

// Convert writes the manuscript as DOCX or PDF with pandoc. Page markers
// become page numbers in brackets, for example [15], at the start of the
// paragraph that starts the page or inside it.
func Convert(ctx context.Context, m *Manuscript, format, outPath string) error {
	if format != FormatDOCX && format != FormatPDF {
		return fmt.Errorf("unknown format %q", format)
	}
	dir, err := os.MkdirTemp("", "atria-pandoc-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "book.md")
	if err := os.WriteFile(input, []byte(pageNumbersAsText(m.Markdown)), 0o600); err != nil {
		return err
	}
	output := filepath.Join(dir, "book."+format)
	args := []string{input, "-f", "markdown", "-o", output, "--toc"}
	if singleTitle(m.Markdown) {
		// The # title becomes the title of the document and ## the chapters.
		args = append(args, "--shift-heading-level-by=-1")
	}
	switch format {
	case FormatPDF:
		args = append(args, "--sandbox", "--pdf-engine=typst", "-V", "mainfont="+PDFFont)
	case FormatDOCX:
		filter := filepath.Join(dir, "safe.lua")
		if err := os.WriteFile(filter, []byte(safeFilter), 0o600); err != nil {
			return err
		}
		args = append(args, "--lua-filter="+filter)
	}

	ctx, cancel := context.WithTimeout(ctx, convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pandoc", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pandoc %s: %w: %s", format, err, strings.TrimSpace(lastLines(stderr.String(), 5)))
	}
	return os.Rename(output, outPath)
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// singleTitle reports whether the book has one # title over ## chapters.
func singleTitle(markdown string) bool {
	_, body, _ := splitFrontMatter(markdown)
	return chapterLevel(strings.Split(body, "\n")) == 2
}

// pageNumbersAsText replaces page markers with the page number in brackets.
// A marker on its own line moves to the start of the next paragraph; before
// a heading or another block it stays a paragraph of its own. Markers of
// unknown pages go away.
func pageNumbersAsText(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	pending := ""
	inFence := false
	for _, line := range lines {
		if mdcheck.IsFence(line) {
			inFence = !inFence
		}
		if inFence {
			out = append(out, line)
			continue
		}
		replaced := replaceMarkers(line, func(label string) string {
			if label == "" {
				return ""
			}
			return `\[` + label + `\]`
		})
		if replaced != line && strings.TrimSpace(replaced) == strings.TrimSpace(strings.Join(labelsOf(line), " ")) {
			// Only markers on this line.
			if s := strings.TrimSpace(replaced); s != "" {
				pending = strings.TrimSpace(pending + " " + s)
			}
			continue
		}
		if pending != "" && strings.TrimSpace(line) != "" {
			if startsParagraph(line) {
				replaced = pending + " " + strings.TrimLeft(replaced, " ")
			} else {
				out = append(out, pending, "")
			}
			pending = ""
		}
		out = append(out, replaced)
	}
	if pending != "" {
		out = append(out, "", pending)
	}
	return strings.Join(out, "\n")
}

// labelsOf returns the replacement text of the markers of a line, to tell a
// line with only markers from a line with text.
func labelsOf(line string) []string {
	var labels []string
	for _, m := range mdcheck.FindMarkers(line) {
		if m.Label != "" {
			labels = append(labels, `\[`+m.Label+`\]`)
		}
	}
	return labels
}

// startsParagraph reports whether a line starts a plain paragraph, not a
// heading, list, quote, table, footnote definition or other block.
func startsParagraph(line string) bool {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
		return false
	}
	for _, prefix := range []string{"#", ">", "-", "*", "+", "|", "[^", "<", "`", "~", "---", "==="} {
		if strings.HasPrefix(t, prefix) {
			return false
		}
	}
	if i := strings.IndexAny(t, ".)"); i > 0 && i < 4 && strings.Trim(t[:i], "0123456789") == "" {
		return false // ordered list
	}
	return true
}
