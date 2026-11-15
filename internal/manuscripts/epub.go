package manuscripts

import (
	"archive/zip"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/bmaupin/go-epub"

	"atria/internal/core"
	"atria/internal/mdcheck"
)

// A page marker becomes this token before rendering, and the token becomes an
// EPUB page break after it. Goldmark keeps letters, digits and hyphens as they are.
const pageToken = "ATRIAPAGEBREAK"

var (
	pageTokenRe      = regexp.MustCompile(pageToken + `-([0-9a-z]+)-` + pageToken)
	pageTokenParaRe  = regexp.MustCompile(`<p>\s*((?:` + pageToken + `-[0-9a-z]+-` + pageToken + `\s*)+)</p>`)
	footnoteDefRe    = regexp.MustCompile(`^ {0,3}\[\^[^\]\s]+\]:`)
	atxHeadingLineRe = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*?)[ \t#]*$`)
	// A manuscript is one file, so images that it links are not in Atria.
	imageRe = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
)

// chapter is one section of the EPUB.
type chapter struct {
	title string
	lines []string
}

// page is one entry of the EPUB page list.
type page struct {
	label, file, id string
}

// WriteEPUB writes the manuscript as an EPUB 3 file. Each chapter is one
// section with its own footnotes. Each page marker of the original becomes a
// page break, and the navigation gets a page list, so that e-book readers can
// show the page numbers of the printed book. It returns the number of pages
// in the page list.
func WriteEPUB(m *Manuscript, outPath string) (int, error) {
	e := epub.NewEpub(m.Title)
	if m.Author != "" {
		e.SetAuthor(m.Author)
	}
	if m.Description != "" {
		e.SetDescription(m.Description)
	}
	if m.Language != "" {
		e.SetLang(m.Language)
	}

	_, body, _ := splitFrontMatter(m.Markdown)
	chapters, footnotes := splitBook(m.Title, body)
	var pages []page
	usedIDs := map[string]int{}
	for i, ch := range chapters {
		file := fmt.Sprintf("chapter-%03d.xhtml", i+1)
		src := withFootnotes(markPages(ch.lines), footnotes)
		rendered, _, err := core.RenderMarkdownXHTML([]byte(src))
		if err != nil {
			return 0, fmt.Errorf("render chapter %q: %w", ch.title, err)
		}
		rendered = pageTokenParaRe.ReplaceAllString(rendered, "$1")
		rendered = pageTokenRe.ReplaceAllStringFunc(rendered, func(token string) string {
			label := pageTokenRe.FindStringSubmatch(token)[1]
			id := "page-" + label
			if n := usedIDs[id]; n > 0 {
				id = fmt.Sprintf("%s-%d", id, n+1)
			}
			usedIDs["page-"+label]++
			pages = append(pages, page{label: label, file: file, id: id})
			return fmt.Sprintf(`<span epub:type="pagebreak" role="doc-pagebreak" id="%s" title="%s"></span>`, id, label)
		})
		if _, err := e.AddSection(rendered, ch.title, file, ""); err != nil {
			return 0, fmt.Errorf("add chapter %q: %w", ch.title, err)
		}
	}

	if len(pages) == 0 {
		return 0, e.Write(outPath)
	}
	tmp := outPath + ".tmp"
	if err := e.Write(tmp); err != nil {
		return 0, err
	}
	defer os.Remove(tmp)
	if err := addPageList(tmp, outPath, pages); err != nil {
		return 0, err
	}
	return len(pages), nil
}

// splitBook splits the body into chapters and takes out the footnote
// definitions. Chapters start at ## when the book has one # title and ##
// chapters, otherwise at #. Text before the first chapter is a section with
// the title of the book.
func splitBook(title, body string) ([]chapter, []string) {
	lines := strings.Split(body, "\n")
	level := chapterLevel(lines)
	var footnotes []string
	var chapters []chapter
	current := chapter{title: title}
	inFence := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if mdcheck.IsFence(line) {
			inFence = !inFence
		}
		if !inFence {
			if footnoteDefRe.MatchString(line) {
				// A definition goes on over indented and blank lines.
				def := []string{line}
				for i+1 < len(lines) && (strings.TrimSpace(lines[i+1]) == "" || strings.HasPrefix(lines[i+1], "    ") || strings.HasPrefix(lines[i+1], "\t")) {
					if strings.TrimSpace(lines[i+1]) == "" && (i+2 >= len(lines) || !(strings.HasPrefix(lines[i+2], "    ") || strings.HasPrefix(lines[i+2], "\t"))) {
						break
					}
					i++
					def = append(def, lines[i])
				}
				footnotes = append(footnotes, strings.Join(def, "\n"))
				continue
			}
			if h := atxHeadingLineRe.FindStringSubmatch(line); h != nil && len(h[1]) == level {
				if hasText(current.lines) {
					chapters = append(chapters, current)
				}
				current = chapter{title: strings.TrimSpace(h[2])}
			}
		}
		current.lines = append(current.lines, line)
	}
	if hasText(current.lines) || len(chapters) == 0 {
		chapters = append(chapters, current)
	}
	return chapters, footnotes
}

func chapterLevel(lines []string) int {
	h1, h2 := 0, 0
	inFence := false
	for _, line := range lines {
		if mdcheck.IsFence(line) {
			inFence = !inFence
			continue
		}
		if h := atxHeadingLineRe.FindStringSubmatch(line); h != nil && !inFence {
			switch len(h[1]) {
			case 1:
				h1++
			case 2:
				h2++
			}
		}
	}
	if h1 <= 1 && h2 > 0 {
		return 2
	}
	return 1
}

// hasText reports whether the lines have more than blank lines and page markers.
func hasText(lines []string) bool {
	for _, line := range lines {
		for _, m := range mdcheck.FindMarkers(line) {
			line = line[:m.Start] + strings.Repeat(" ", m.End-m.Start) + line[m.End:]
		}
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}

// markPages replaces page markers with tokens; markers of unknown pages go
// away. It also removes images, which are not part of a manuscript.
func markPages(lines []string) string {
	out := make([]string, len(lines))
	inFence := false
	for i, line := range lines {
		if mdcheck.IsFence(line) {
			inFence = !inFence
		}
		if !inFence {
			line = imageRe.ReplaceAllString(line, "")
			line = replaceMarkers(line, func(label string) string {
				if label == "" {
					return ""
				}
				return pageToken + "-" + label + "-" + pageToken
			})
		}
		out[i] = line
	}
	return strings.Join(out, "\n")
}

func replaceMarkers(line string, replace func(label string) string) string {
	markers := mdcheck.FindMarkers(line)
	for i := len(markers) - 1; i >= 0; i-- {
		m := markers[i]
		line = line[:m.Start] + replace(m.Label) + line[m.End:]
	}
	return line
}

// withFootnotes appends the footnote definitions of the book to a chapter.
// The renderer keeps only the footnotes that the chapter uses.
func withFootnotes(src string, footnotes []string) string {
	if len(footnotes) == 0 {
		return src
	}
	return src + "\n\n" + strings.Join(footnotes, "\n\n") + "\n"
}

// addPageList copies the EPUB src to dst and adds the page list to its
// navigation document.
func addPageList(src, dst string, pages []page) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open EPUB: %w", err)
	}
	defer r.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	w := zip.NewWriter(out)

	var list strings.Builder
	list.WriteString(`<nav epub:type="page-list" hidden="hidden"><h2>Pages</h2><ol>`)
	for _, p := range pages {
		fmt.Fprintf(&list, `<li><a href="xhtml/%s#%s">%s</a></li>`, p.file, p.id, html.EscapeString(p.label))
	}
	list.WriteString("</ol></nav>\n</body>")

	for _, f := range r.File {
		if f.Name != "EPUB/nav.xhtml" {
			if err := w.Copy(f); err != nil {
				out.Close()
				return fmt.Errorf("copy %s: %w", f.Name, err)
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		nav, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			out.Close()
			return err
		}
		updated := strings.Replace(string(nav), "</body>", list.String(), 1)
		fw, err := w.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate})
		if err != nil {
			out.Close()
			return err
		}
		if _, err := io.WriteString(fw, updated); err != nil {
			out.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
