package ebooks

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gofrs/uuid/v5"
)

func TestImportRejectsNonEPUBBeforeDatabaseAccess(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-an-ebook-*.txt")
	if err != nil {
		t.Fatalf("create temporary file: %v", err)
	}
	path := file.Name()
	if _, err := file.WriteString("not an EPUB"); err != nil {
		t.Fatalf("write temporary file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temporary file: %v", err)
	}

	_, err = Import(context.Background(), nil, uuid.Nil, ImportInput{
		Title:    "Invalid import",
		Path:     path,
		Filename: "not-an-ebook.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "must be an EPUB") {
		t.Fatalf("Import() error = %v, want EPUB validation error", err)
	}
}
