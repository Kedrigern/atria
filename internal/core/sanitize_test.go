package core

import (
	"strings"
	"testing"
)

func TestSanitizeHTML(t *testing.T) {
	input := `<p onclick="alert(1)">Hello <b>world</b></p>` +
		`<script>alert(2)</script>` +
		`<img src="https://example.com/a.png" onerror="alert(3)" alt="a">` +
		`<a href="javascript:alert(4)">bad</a>` +
		`<a href="https://example.com">good</a>` +
		`<iframe src="https://evil.example"></iframe>` +
		`<pre><code class="language-go">x := 1</code></pre>`

	out := SanitizeHTML(input)

	for _, forbidden := range []string{"onclick", "<script", "alert(2)", "onerror", "javascript:", "<iframe"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("sanitized output still contains %q: %s", forbidden, out)
		}
	}
	for _, kept := range []string{"<b>world</b>", `src="https://example.com/a.png"`, `href="https://example.com"`, `class="language-go"`} {
		if !strings.Contains(out, kept) {
			t.Errorf("sanitized output lost %q: %s", kept, out)
		}
	}
}
