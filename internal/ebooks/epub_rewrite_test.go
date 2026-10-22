package ebooks

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testOPF = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="id">urn:test</dc:identifier>
    <dc:title>Old title</dc:title>
    <dc:creator id="author">Old author</dc:creator>
    <dc:language>en</dc:language>
    <dc:description>Old description</dc:description>
    %COVERMETA%
  </metadata>
  <manifest>
    <item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/>
    %COVERITEM%
  </manifest>
  <spine><itemref idref="chapter"/></spine>
</package>`

func testImage(t *testing.T, w, h int) image.Image {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.RGBA{R: 200, A: 255})
	}
	return img
}

// writeTestEPUB writes a small EPUB. With withCover, it has a JPEG cover of 10 × 20 pixels.
func writeTestEPUB(t *testing.T, path string, withCover bool) {
	t.Helper()
	opf := testOPF
	if withCover {
		opf = strings.ReplaceAll(opf, "%COVERMETA%", `<meta name="cover" content="cover-img"/>`)
		opf = strings.ReplaceAll(opf, "%COVERITEM%", `<item id="cover-img" href="images/cover.jpg" media-type="image/jpeg" properties="cover-image"/>`)
	} else {
		opf = strings.ReplaceAll(strings.ReplaceAll(opf, "%COVERMETA%", ""), "%COVERITEM%", "")
	}
	f, err := os.Create(path)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	entry, _ := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	_, _ = io.WriteString(entry, "application/epub+zip")
	entry, _ = w.Create("META-INF/container.xml")
	_, _ = io.WriteString(entry, `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)
	entry, _ = w.Create("OEBPS/content.opf")
	_, _ = io.WriteString(entry, opf)
	entry, _ = w.Create("OEBPS/chapter.xhtml")
	_, _ = io.WriteString(entry, `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>Text</p></body></html>`)
	if withCover {
		entry, _ = w.Create("OEBPS/images/cover.jpg")
		require.NoError(t, jpeg.Encode(entry, testImage(t, 10, 20), nil))
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
}

func readEntry(t *testing.T, path, name string) []byte {
	t.Helper()
	archive, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer archive.Close()
	for _, f := range archive.File {
		if f.Name == name {
			r, err := f.Open()
			require.NoError(t, err)
			defer r.Close()
			data, err := io.ReadAll(r)
			require.NoError(t, err)
			return data
		}
	}
	t.Fatalf("%s is not in %s", name, path)
	return nil
}

func TestRewriteEPUBMetadataAndCover(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "in.epub"), filepath.Join(dir, "out.epub")
	writeTestEPUB(t, src, true)

	cover, err := RewriteEPUB(src, dst, &EPUBMetadata{Title: "Tom & Jerry", Author: "New author", Language: "", Description: ""}, testImage(t, 30, 40))
	require.NoError(t, err)
	require.NotNil(t, cover)
	assert.Equal(t, "image/jpeg", cover.MediaType, "the cover keeps the declared type")

	archive, err := zip.OpenReader(dst)
	require.NoError(t, err)
	assert.Equal(t, "mimetype", archive.File[0].Name, "mimetype is the first entry")
	assert.Equal(t, zip.Store, archive.File[0].Method, "mimetype is not compressed")
	archive.Close()

	metadata, err := ReadEPUBMetadata(dst)
	require.NoError(t, err)
	assert.Equal(t, "Tom & Jerry", metadata.Title)
	assert.Equal(t, "New author", metadata.Author)
	assert.Equal(t, "en", metadata.Language, "an empty language keeps the value, because it is required")
	assert.Empty(t, metadata.Description, "an empty description removes it")
	assert.Contains(t, string(readEntry(t, dst, "OEBPS/content.opf")), `<dc:creator id="author">New author</dc:creator>`, "attributes stay")

	img, err := jpeg.Decode(bytes.NewReader(readEntry(t, dst, "OEBPS/images/cover.jpg")))
	require.NoError(t, err)
	assert.Equal(t, 30, img.Bounds().Dx())
	assert.Equal(t, 40, img.Bounds().Dy())
	assert.Contains(t, string(readEntry(t, dst, "OEBPS/chapter.xhtml")), "<p>Text</p>", "other entries are copied")
}

func TestRewriteEPUBAddsMissingCover(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "in.epub"), filepath.Join(dir, "out.epub")
	writeTestEPUB(t, src, false)

	_, err := RewriteEPUB(src, dst, nil, testImage(t, 5, 5))
	require.NoError(t, err)
	extracted, err := ExtractEPUBCover(dst, dir)
	require.NoError(t, err, "the added cover is declared in the manifest")
	data, err := os.ReadFile(extracted.Path)
	require.NoError(t, err)
	_, err = jpeg.Decode(bytes.NewReader(data))
	assert.NoError(t, err)
	metadata, err := ReadEPUBMetadata(dst)
	require.NoError(t, err)
	assert.Equal(t, "Old title", metadata.Title, "nil metadata keeps the metadata")
}

func TestDecodeCoverRejectsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "cover.txt")
	require.NoError(t, os.WriteFile(text, []byte("not an image"), 0o600))
	_, err := DecodeCover(text)
	assert.Error(t, err)

	pngPath := filepath.Join(dir, "cover.png")
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, testImage(t, 3, 3)))
	require.NoError(t, os.WriteFile(pngPath, buf.Bytes(), 0o600))
	_, err = DecodeCover(pngPath)
	assert.NoError(t, err)
}
