package web

import (
	"bytes"

	"mime/multipart"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSaveEbookImportUploadUsesAttachmentStorage(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("epub", "book.epub")
	if err != nil {
		t.Fatalf("create EPUB form file: %v", err)
	}
	if _, err := part.Write([]byte("EPUB content")); err != nil {
		t.Fatalf("write EPUB form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest("POST", "/api/ebooks/import", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(1024); err != nil {
		t.Fatalf("parse multipart form: %v", err)
	}
	files := request.MultipartForm.File["epub"]
	if len(files) != 1 {
		t.Fatalf("uploaded EPUB files = %d, want 1", len(files))
	}

	storagePath := t.TempDir()
	t.Setenv("STORAGE_PATH", storagePath)
	tempPath, cleanup, err := saveEbookImportUpload(files[0])
	if err != nil {
		t.Fatalf("saveEbookImportUpload() error: %v", err)
	}
	defer cleanup()

	content, err := os.ReadFile(tempPath)
	if err != nil {
		t.Fatalf("read staged EPUB: %v", err)
	}
	if got, want := string(content), "EPUB content"; got != want {
		t.Errorf("staged EPUB content = %q, want %q", got, want)
	}
	if _, err := os.Stat(tempPath); err != nil {
		t.Fatalf("stat staged EPUB: %v", err)
	}

	cleanup()
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Errorf("staged EPUB still exists after cleanup: %v", err)
	}

	if err := request.MultipartForm.RemoveAll(); err != nil {
		t.Fatalf("remove multipart form files: %v", err)
	}
}
