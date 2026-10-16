package rss

import (
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
)

func TestNextFetchDelay(t *testing.T) {
	cases := map[int]time.Duration{
		0:  time.Hour,
		1:  2 * time.Hour,
		2:  4 * time.Hour,
		4:  16 * time.Hour,
		5:  24 * time.Hour,
		50: 24 * time.Hour,
	}
	for failures, want := range cases {
		if got := nextFetchDelay(failures); got != want {
			t.Errorf("nextFetchDelay(%d) = %s, want %s", failures, got, want)
		}
	}
}

func TestItemKey(t *testing.T) {
	if got := itemKey(&gofeed.Item{GUID: " abc ", Link: "https://x"}); got != "abc" {
		t.Errorf("GUID should win, got %q", got)
	}
	if got := itemKey(&gofeed.Item{Link: "https://x/1"}); got != "https://x/1" {
		t.Errorf("link fallback, got %q", got)
	}
	a := itemKey(&gofeed.Item{Title: "a"})
	b := itemKey(&gofeed.Item{Title: "b"})
	if a == b || a == "" || a != itemKey(&gofeed.Item{Title: "a"}) {
		t.Errorf("content hash must be stable and distinct: %q %q", a, b)
	}
}
