package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyEbookFile(t *testing.T) {
	storagePath := t.TempDir()
	t.Setenv("STORAGE_PATH", storagePath)

	sourceDirectory := filepath.Join(storagePath, "2026", "08")
	if err := os.MkdirAll(sourceDirectory, 0755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	sourcePath := filepath.Join(sourceDirectory, "book.epub")
	if err := os.WriteFile(sourcePath, []byte("epub content"), 0644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	destination := filepath.Join(t.TempDir(), "download.epub")
	if err := copyEbookFile(filepath.Join("2026", "08", "book.epub"), destination); err != nil {
		t.Fatalf("copyEbookFile: %v", err)
	}

	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination file: %v", err)
	}
	if string(content) != "epub content" {
		t.Errorf("destination content = %q, want %q", content, "epub content")
	}
}

func TestEbookOutputFilename(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		filename string
		want     string
	}{
		{
			name:  "derives EPUB filename from title",
			title: "Atria Reading List",
			want:  "atria-reading-list.epub",
		},
		{
			name:     "normalizes EPUB extension",
			title:    "Ignored",
			filename: "reading.EPUB",
			want:     "reading.EPUB",
		},
		{
			name:  "removes path separators from derived filename",
			title: "Notes / Archive",
			want:  "notes---archive.epub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ebookOutputFilename(tt.title, tt.filename); got != tt.want {
				t.Errorf("ebookOutputFilename(%q, %q) = %q, want %q", tt.title, tt.filename, got, tt.want)
			}
		})
	}
}
