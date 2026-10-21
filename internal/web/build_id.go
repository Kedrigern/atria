package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// uiBuild identifies the templates and static files of this binary. Each page
// carries it in <meta name="atria-build">, and app.js sends it back with each
// htmx request. After a deploy, an old page would put new content into its old
// frame with the old CSS; the server answers with HX-Refresh instead, so the
// browser loads the whole page again.
var uiBuild = sync.OnceValue(func() string {
	h := sha256.New()
	for _, fsys := range []fs.FS{TemplatesFS, StaticFS} {
		_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(fsys, path)
			if err != nil {
				return err
			}
			h.Write([]byte(path))
			h.Write(data)
			return nil
		})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
})

// staleHTMXPage reports whether an htmx GET request comes from a page of an
// older build. It then tells htmx to reload the full page.
func staleHTMXPage(c *gin.Context) bool {
	build := c.GetHeader("X-Atria-Build")
	if c.Request.Method != http.MethodGet || build == "" || build == uiBuild() {
		return false
	}
	c.Header("HX-Refresh", "true")
	c.Status(http.StatusOK)
	return true
}
