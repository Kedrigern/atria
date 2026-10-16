package core

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

// htmlPolicy is the allowlist applied to third-party HTML (readability output,
// RSS item content) before it is rendered. It is safe for concurrent use.
var htmlPolicy = newHTMLPolicy()

func newHTMLPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowElements("figure", "figcaption", "picture", "mark", "small", "time")
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^language-[\w-]+$`)).OnElements("code")
	p.AllowAttrs("loading").Matching(regexp.MustCompile(`^(lazy|eager)$`)).OnElements("img")
	p.AllowAttrs("datetime").OnElements("time")
	return p
}

// SanitizeHTML strips scripts, event handlers, dangerous URLs and any markup
// outside the allowlist from untrusted HTML.
func SanitizeHTML(s string) string {
	return htmlPolicy.Sanitize(s)
}
