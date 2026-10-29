// Package frontend serves the built single page app.
package frontend

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ddns-by-dnspod/internal/api/middleware"
)

//go:embed all:dist
var distFS embed.FS

// dist returns the filesystem with the dist prefix stripped
func dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("embed the frontend assets: " + err.Error())
	}
	return sub
}

// Register registers static assets and the SPA fallback route
func Register(r *gin.Engine) {
	staticFS := dist()
	fileServer := http.FileServer(http.FS(staticFS))

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Unmatched API paths return JSON instead of the frontend page. The path
		// is not echoed back: it comes from the request and there is nothing to
		// gain from reflecting it into the response.
		if strings.HasPrefix(path, "/api/") {
			body := map[string]string{"error": "no such endpoint"}
			if rid := middleware.RequestIDFrom(c); rid != "" {
				body["request_id"] = rid
			}
			c.JSON(http.StatusNotFound, body)
			return
		}

		// Serve the asset directly when it exists
		if trimmed := strings.TrimPrefix(path, "/"); trimmed != "" {
			if f, err := staticFS.Open(trimmed); err == nil {
				_ = f.Close()
				cacheControl(c, path)
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
		}

		// Everything else falls back to index.html, handled by the frontend router.
		// It must never be cached, or a browser would keep showing a stale bundle
		// after an upgrade.
		//
		// Only the placeholder is embedded (a build that never ran `npm run
		// build`): saying so is more useful than the file server's 404, which
		// looks like a missing route.
		if _, err := staticFS.Open("index.html"); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "the frontend has not been built"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}

// cacheControl sets the response header for a static asset.
//
// Files under /assets/ carry a content hash in their name, so a change is a new
// URL and the old one is safe to cache forever. Everything else is short-lived
// at best, since it has no hash to distinguish versions.
func cacheControl(c *gin.Context, path string) {
	if strings.HasPrefix(path, "/assets/") {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	c.Header("Cache-Control", "no-cache")
}
