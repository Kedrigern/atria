package web

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"atria/internal/ebooks"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSmallEPUB(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	entry, _ := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	_, _ = io.WriteString(entry, "application/epub+zip")
	entry, _ = w.Create("META-INF/container.xml")
	_, _ = io.WriteString(entry, `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)
	entry, _ = w.Create("content.opf")
	_, _ = io.WriteString(entry, `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Small book</dc:title><dc:language>en</dc:language></metadata><manifest></manifest><spine></spine></package>`)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
}

func TestEbookCoverUpload(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	t.Setenv("STORAGE_PATH", t.TempDir())
	epubPath := filepath.Join(t.TempDir(), "small.epub")
	writeSmallEPUB(t, epubPath)
	book, err := ebooks.Import(context.Background(), db, user.ID, ebooks.ImportInput{Path: epubPath, Filename: "small.epub"})
	require.NoError(t, err)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	edit := getWithCookie(r, "/ebooks/"+book.ID.String()+"/edit", nil).Body.String()
	assert.Contains(t, edit, "Replace cover")
	assert.Contains(t, edit, "Replace EPUB file")

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	require.NoError(t, form.WriteField("_csrf", csrfTokenFor(user.Email)))
	part, err := form.CreateFormFile("cover", "cover.png")
	require.NoError(t, err)
	require.NoError(t, png.Encode(part, image.NewRGBA(image.Rect(0, 0, 8, 12))))
	require.NoError(t, form.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/ebooks/"+book.ID.String()+"/cover", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	assert.Equal(t, "/ebooks/"+book.ID.String(), w.Header().Get("Location"))

	detail, err := ebooks.GetDetail(context.Background(), db, user.ID, book.ID)
	require.NoError(t, err)
	assert.NotNil(t, detail.CoverDiskPath, "the e-book without a cover has one now")
}
