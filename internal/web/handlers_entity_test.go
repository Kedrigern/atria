package web

import "testing"

func TestDecodePromptTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "decodes percent-encoded UTF-8",
			title: "%20%2398%20%C5%BDlu%C5%A5ou%C4%8Dk%C3%BD%20k%C5%AF%C5%88%20%C3%BAp%C4%9Bl%20%C4%8F%C3%A1belsk%C3%A9%20%C3%B3dy%20",
			want:  "#98 Žluťoučký kůň úpěl ďábelské ódy",
		},
		{
			name:  "preserves invalid encoding",
			title: "Title%ZZ",
			want:  "Title%ZZ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodePromptTitle(tt.title); got != tt.want {
				t.Errorf("decodePromptTitle(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}
