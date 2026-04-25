package core_test

import (
	"strings"
	"testing"

	"atria/internal/core"
)

func TestRenderMarkdown_FrontMatter(t *testing.T) {
	input := []byte(`---
title: "Test Note"
tags:
  - go
  - htmx
---
# Heading
This is content.`)

	html, meta, err := core.RenderMarkdown(input)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	// 1. Metadata musí být vyparsována
	if meta["title"] != "Test Note" {
		t.Errorf("Expected title 'Test Note', got %v", meta["title"])
	}

	// 2. YAML blok nesmí být součástí vyrenderovaného HTML
	if strings.Contains(html, "title: \"Test Note\"") {
		t.Errorf("HTML output should not contain raw front matter. Got: %s", html)
	}
}

func TestRenderMarkdown_HeadingShifter(t *testing.T) {
	// Scénář A: Dokument MÁ H1 -> všechny nadpisy se posunou
	t.Run("With H1", func(t *testing.T) {
		input := []byte("# Main Title\n## Subtitle\n### Sub-subtitle")
		html, _, err := core.RenderMarkdown(input)
		if err != nil {
			t.Fatalf("RenderMarkdown failed: %v", err)
		}

		if !strings.Contains(html, "<h2 id=\"main-title\">Main Title</h2>") {
			t.Errorf("Expected H1 to be shifted to H2, got: %s", html)
		}
		if !strings.Contains(html, "<h3 id=\"subtitle\">Subtitle</h3>") {
			t.Errorf("Expected H2 to be shifted to H3, got: %s", html)
		}
	})

	// Scénář B: Dokument NEMÁ H1 -> nadpisy zůstanou zachovány
	t.Run("Without H1", func(t *testing.T) {
		input := []byte("## Subtitle\n### Sub-subtitle")
		html, _, err := core.RenderMarkdown(input)
		if err != nil {
			t.Fatalf("RenderMarkdown failed: %v", err)
		}

		if !strings.Contains(html, "<h2 id=\"subtitle\">Subtitle</h2>") {
			t.Errorf("Expected H2 to remain H2, got: %s", html)
		}
		if !strings.Contains(html, "<h3 id=\"sub-subtitle\">Sub-subtitle</h3>") {
			t.Errorf("Expected H3 to remain H3, got: %s", html)
		}
	})
}

func TestRenderMarkdown_Callouts(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Standard Note",
			input:    "> [!NOTE]\n> This is a note.",
			expected: `<blockquote class="callout callout-note">`,
		},
		{
			name:     "Warning with formatting",
			input:    "> [!WARNING]\n> **Danger!**",
			expected: `<blockquote class="callout callout-warning">`,
		},
		{
			name:     "Title Generation",
			input:    "> [!TIP]\n> Use Go.",
			expected: `<strong>Tip</strong>`,
		},
		{
			name:     "Removal of raw tag",
			input:    "> [!IMPORTANT]\n> Don't forget this.",
			expected: "Don't forget this",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html, _, err := core.RenderMarkdown([]byte(tc.input))
			if err != nil {
				t.Fatalf("RenderMarkdown failed: %v", err)
			}

			if !strings.Contains(html, tc.expected) {
				t.Errorf("Expected output to contain '%s', got:\n%s", tc.expected, html)
			}

			// Taky ověříme, že ten syrový text už ve výstupu není
			if strings.Contains(html, "[!") {
				t.Errorf("Raw callout tag '[!TYPE]' should be removed from output, got:\n%s", html)
			}
		})
	}
}

func TestRenderMarkdown_GFM_Tables(t *testing.T) {
	input := []byte(`
| A | B |
|---|---|
| 1 | 2 |
`)
	html, _, err := core.RenderMarkdown(input)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	if !strings.Contains(html, "<table>") || !strings.Contains(html, "<tbody>") {
		t.Errorf("Expected table to be rendered, got: %s", html)
	}
}
