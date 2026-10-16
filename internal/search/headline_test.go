package search

import (
	"strings"
	"testing"
)

func TestHeadlineHTML(t *testing.T) {
	got := string(HeadlineHTML(`<p>Find ⟦golang⟧ here</p> &amp; <img src=x onerror=alert(1)> <script>x</script> 1 < 2`))

	if !strings.Contains(got, "<mark>golang</mark>") {
		t.Errorf("highlight marker not rendered: %s", got)
	}
	for _, forbidden := range []string{"<img", "<script", "<p>"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("headline contains %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "&amp;") || !strings.Contains(got, "1 &lt; 2") {
		t.Errorf("text not escaped correctly: %s", got)
	}
}
