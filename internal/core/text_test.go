package core

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLimitRunes(t *testing.T) {
	if got := LimitRunes("short", 10); got != "short" {
		t.Errorf("unchanged string expected, got %q", got)
	}

	// 99 ASCII bytes followed by "č" (2 bytes): byte slicing at 100 would
	// split the "č" and produce invalid UTF-8.
	s := strings.Repeat("a", 99) + "čšž"
	got := LimitRunes(s, 100)
	if !utf8.ValidString(got) {
		t.Fatalf("result is not valid UTF-8: %q", got)
	}
	if want := strings.Repeat("a", 99) + "č"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	if got := LimitRunes("žluťoučký kůň", 4); got != "žluť" {
		t.Errorf("got %q, want %q", got, "žluť")
	}
}
