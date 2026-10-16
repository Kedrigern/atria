package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// The CSP forbids inline scripts, so templates must not contain any inline
// JavaScript: put behaviour in static/app.js and wire it with data-* attributes.
func TestTemplatesHaveNoInlineJavaScript(t *testing.T) {
	forbidden := map[string]*regexp.Regexp{
		"inline event handler (on*=)":  regexp.MustCompile(`(?i)\son[a-z]+\s*=`),
		"javascript: URL":              regexp.MustCompile(`(?i)javascript:`),
		"hx-on attribute (needs eval)": regexp.MustCompile(`(?i)hx-on`),
	}
	scriptTag := regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)

	err := fs.WalkDir(TemplatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		content, err := fs.ReadFile(TemplatesFS, path)
		if err != nil {
			return err
		}
		lineAt := func(offset int) int { return strings.Count(string(content[:offset]), "\n") + 1 }
		for name, re := range forbidden {
			if loc := re.FindIndex(content); loc != nil {
				t.Errorf("%s:%d: %s", path, lineAt(loc[0]), name)
			}
		}
		for _, m := range scriptTag.FindAllSubmatchIndex(content, -1) {
			attrs, body := string(content[m[2]:m[3]]), strings.TrimSpace(string(content[m[4]:m[5]]))
			if !strings.Contains(attrs, "src=") || body != "" {
				t.Errorf("%s:%d: inline <script>", path, lineAt(m[0]))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCSPForbidsInlineScripts(t *testing.T) {
	scriptSrc := ""
	for _, directive := range strings.Split(contentSecurityPolicy, ";") {
		if d := strings.TrimSpace(directive); strings.HasPrefix(d, "script-src ") {
			scriptSrc = d
		}
	}
	if scriptSrc == "" {
		t.Fatal("CSP has no script-src directive")
	}
	for _, keyword := range []string{"'unsafe-inline'", "'unsafe-eval'", "*"} {
		if strings.Contains(scriptSrc, keyword) {
			t.Errorf("script-src must not contain %s: %s", keyword, scriptSrc)
		}
	}
}
