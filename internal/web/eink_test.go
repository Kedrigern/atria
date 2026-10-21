package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// E-ink readers are a main target (docs/architecture/ui.md, section 8):
// no motion, and the large rendering libraries load only where they are used.
func TestEinkRules(t *testing.T) {
	base, err := fs.ReadFile(TemplatesFS, "templates/base.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, lib := range []string{"highlight.js", "mathjax", "mermaid"} {
		if strings.Contains(strings.ToLower(string(base)), lib) {
			t.Errorf("base.html loads %s on every page; load it on demand in static/plugins.js", lib)
		}
	}

	css, err := fs.ReadFile(StaticFS, "static/style.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "transition: none !important") || !strings.Contains(string(css), "animation: none !important") {
		t.Error("style.css lost the global no-motion rule")
	}
	for _, m := range regexp.MustCompile(`(?:^|[\s;{])(transition|animation|scroll-behavior)\s*:\s*([^;}]*)`).FindAllStringSubmatch(string(css), -1) {
		if v := strings.Fields(m[2]); len(v) == 0 || (v[0] != "none" && v[0] != "auto") {
			t.Errorf("style.css adds motion: %s: %s", m[1], m[2])
		}
	}

	err = fs.WalkDir(TemplatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(TemplatesFS, path)
		if err != nil {
			return err
		}
		if regexp.MustCompile(`swap:[0-9.]+m?s|settle:[0-9.]+m?s`).Match(b) {
			t.Errorf("%s: htmx swap/settle delay animates content", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
