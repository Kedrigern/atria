package ebooks

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadEPUBMetadataAndExtractCoverFromCoverImageProperty(t *testing.T) {
	epubPath := writeMinimalEPUB(t, map[string]string{
		"OEBPS/package.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/" version="3.0">
  <metadata>
    <dc:title>Example book</dc:title>
    <dc:creator>Jane Doe</dc:creator>
    <dc:language>en</dc:language>
    <dc:description>An example description.</dc:description>
  </metadata>
  <manifest>
    <item id="chapter" href="text/chapter.xhtml" media-type="application/xhtml+xml"/>
    <item id="cover" href="images/cover.jpg" media-type="image/jpeg" properties="cover-image"/>
  </manifest>
</package>`,
		"OEBPS/images/cover.jpg": "cover image bytes",
	})

	metadata, err := ReadEPUBMetadata(epubPath)
	if err != nil {
		t.Fatalf("ReadEPUBMetadata() error = %v", err)
	}
	if metadata.Title != "Example book" || metadata.Author != "Jane Doe" || metadata.Language != "en" || metadata.Description != "An example description." {
		t.Fatalf("ReadEPUBMetadata() = %+v, want parsed metadata", metadata)
	}

	cover, err := ExtractEPUBCover(epubPath, t.TempDir())
	if err != nil {
		t.Fatalf("ExtractEPUBCover() error = %v", err)
	}
	if cover.Filename != "cover.jpg" {
		t.Errorf("cover filename = %q, want cover.jpg", cover.Filename)
	}
	if filepath.Ext(cover.Path) != ".jpg" {
		t.Errorf("extracted cover extension = %q, want .jpg", filepath.Ext(cover.Path))
	}
	contents, err := os.ReadFile(cover.Path)
	if err != nil {
		t.Fatalf("read extracted cover: %v", err)
	}
	if string(contents) != "cover image bytes" {
		t.Errorf("extracted cover = %q, want %q", contents, "cover image bytes")
	}
}

func TestExtractEPUBCoverUsesLegacyCoverMetadata(t *testing.T) {
	epubPath := writeMinimalEPUB(t, map[string]string{
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata><meta name="cover" content="front-cover"/></metadata>
  <manifest>
    <item id="front-cover" href="assets/front.png" media-type="image/png"/>
  </manifest>
</package>`,
		"assets/front.png": "png image bytes",
	})

	cover, err := ExtractEPUBCover(epubPath, t.TempDir())
	if err != nil {
		t.Fatalf("ExtractEPUBCover() error = %v", err)
	}
	if cover.Filename != "front.png" {
		t.Errorf("cover filename = %q, want front.png", cover.Filename)
	}
	contents, err := os.ReadFile(cover.Path)
	if err != nil {
		t.Fatalf("read extracted cover: %v", err)
	}
	if string(contents) != "png image bytes" {
		t.Errorf("extracted cover = %q, want %q", contents, "png image bytes")
	}
}

func TestExtractEPUBCoverReturnsNotFound(t *testing.T) {
	epubPath := writeMinimalEPUB(t, map[string]string{
		"content.opf": `<package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata/><manifest/></package>`,
	})

	_, err := ExtractEPUBCover(epubPath, t.TempDir())
	if !errors.Is(err, ErrEPUBCoverNotFound) {
		t.Fatalf("ExtractEPUBCover() error = %v, want ErrEPUBCoverNotFound", err)
	}
}

func writeMinimalEPUB(t *testing.T, entries map[string]string) string {
	t.Helper()

	epubPath := filepath.Join(t.TempDir(), "book.epub")
	file, err := os.Create(epubPath)
	if err != nil {
		t.Fatalf("create EPUB: %v", err)
	}
	archive := zip.NewWriter(file)

	mimetype, err := archive.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		t.Fatalf("create EPUB mimetype entry: %v", err)
	}
	if _, err := mimetype.Write([]byte("application/epub+zip")); err != nil {
		t.Fatalf("write EPUB mimetype entry: %v", err)
	}

	rootfilePath := "content.opf"
	if _, ok := entries["OEBPS/package.opf"]; ok {
		rootfilePath = "OEBPS/package.opf"
	}
	writeZIPEntry(t, archive, "META-INF/container.xml", `<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="`+rootfilePath+`" media-type="application/oebps-package+xml"/></rootfiles></container>`)
	for name, contents := range entries {
		writeZIPEntry(t, archive, name, contents)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close EPUB archive: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close EPUB file: %v", err)
	}
	return epubPath
}

func writeZIPEntry(t *testing.T, archive *zip.Writer, name, contents string) {
	t.Helper()
	entry, err := archive.Create(name)
	if err != nil {
		t.Fatalf("create ZIP entry %q: %v", name, err)
	}
	if _, err := entry.Write([]byte(contents)); err != nil {
		t.Fatalf("write ZIP entry %q: %v", name, err)
	}
}
