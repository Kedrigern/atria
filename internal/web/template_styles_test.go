package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Templates use the CSS classes in static/style.css, not inline styles
// (docs/architecture/ui.md, section 11). The only exception is a value that the
// server calculates: it goes into a CSS variable, for example
// style="--content-scale: {{.ContentFontScale}}".
func TestTemplatesHaveNoInlineStyles(t *testing.T) {
	styleAttr := regexp.MustCompile(`(?is)\sstyle\s*=\s*"([^"]*)"`)
	styleTag := regexp.MustCompile(`(?i)<style\b`)
	customProperty := regexp.MustCompile(`^--[a-z0-9-]+\s*:`)

	err := fs.WalkDir(TemplatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		content, err := fs.ReadFile(TemplatesFS, path)
		if err != nil {
			return err
		}
		lineAt := func(offset int) int { return strings.Count(string(content[:offset]), "\n") + 1 }
		if loc := styleTag.FindIndex(content); loc != nil {
			t.Errorf("%s:%d: <style> element", path, lineAt(loc[0]))
		}
		for _, m := range styleAttr.FindAllSubmatchIndex(content, -1) {
			for _, declaration := range strings.Split(string(content[m[2]:m[3]]), ";") {
				declaration = strings.TrimSpace(declaration)
				if declaration != "" && !customProperty.MatchString(declaration) {
					t.Errorf("%s:%d: inline style %q, use a CSS class or a CSS variable", path, lineAt(m[0]), declaration)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The interface is in English only (docs/architecture/ui.md, section 3).
// Czech letters in a template are an untranslated text.
func TestTemplatesAreInEnglish(t *testing.T) {
	err := fs.WalkDir(TemplatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		content, err := fs.ReadFile(TemplatesFS, path)
		if err != nil {
			return err
		}
		if loc := czechLetters.FindIndex(content); loc != nil {
			line := strings.Count(string(content[:loc[0]]), "\n") + 1
			t.Errorf("%s:%d: Czech text", path, line)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
