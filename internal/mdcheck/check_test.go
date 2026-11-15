package mdcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The marker forms from docs/architecture/manuscripts.md §3: an HTML comment
// with an Arabic number, optionally after a page word, or a Roman number for
// front matter. Other text is not a marker; a comment with a digit in an
// unknown form gets a warning.
func TestPageMarkerForms(t *testing.T) {
	cases := []struct {
		marker        string
		originalPages int  // the marker is page 15 (Arabic) and text follows it
		nearWarning   bool // "looks like a page marker"
	}{
		{"<!-- 15 -->", 16, false},
		{"<!--15-->", 16, false},
		{"<!-- p. 15 -->", 16, false},
		{"<!-- P. 15 -->", 16, false},
		{"<!-- s. 15 -->", 16, false},
		{"<!-- str. 15 -->", 16, false},
		{"<!-- page 15 -->", 16, false},
		{"<!-- xiv -->", 0, false},
		{"<!-- p. XIV -->", 0, false},
		{"[15]", 0, false},
		{"<!-- poznámka redakce -->", 0, false},
		{"<!-- 15a -->", 0, true},
		{"<!-- see page 15 -->", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.marker, func(t *testing.T) {
			report := Check([]byte("# Kniha\n\nText strany.\n" + tc.marker + "\nDalší text.\n"))
			assert.Equal(t, tc.originalPages, report.OriginalPages, "original pages for marker %q", tc.marker)
			near := false
			for _, w := range report.Warnings {
				if w.Rule == RulePages {
					near = true
				}
			}
			assert.Equal(t, tc.nearWarning, near, "page warning for %q", tc.marker)
		})
	}
}
