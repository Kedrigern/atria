package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func serveEmbeddedAsset(c *gin.Context, path, contentType string) {
	content, err := StaticFS.ReadFile(path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Data(http.StatusOK, contentType, content)
}

func (s *Server) handleWebAppManifest(c *gin.Context) {
	serveEmbeddedAsset(c, "static/manifest.webmanifest", "application/manifest+json; charset=utf-8")
}

func (s *Server) handleServiceWorker(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	serveEmbeddedAsset(c, "static/sw.js", "application/javascript; charset=utf-8")
}
