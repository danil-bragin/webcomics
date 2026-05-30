// SPA bundle served from the same Go binary that exposes the API.
// The dist/ directory is produced by `make ui-build` in web-ui/ (vite.config.ts
// writes here directly via UI_OUT_DIR). For dev, use `make ui-dev`, which runs
// Vite on :5173 with /api proxied to :8080.
package http

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed embedded
var spaFS embed.FS

// mountSPA serves the embedded SPA at "/" with a hash-router-friendly fallback:
// any GET that isn't a real file under embedded/ falls back to index.html so
// client-side routes (/runs/:id, /templates, …) render in the browser.
func mountSPA(r interface {
	Get(pattern string, h http.HandlerFunc)
}) {
	sub, err := fs.Sub(spaFS, "embedded")
	if err != nil {
		// Embed produced nothing — shouldn't happen, placeholder always exists.
		return
	}
	fsys := http.FS(sub)
	fileServer := http.FileServer(fsys)

	handler := func(w http.ResponseWriter, req *http.Request) {
		p := strings.TrimPrefix(req.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := sub.Open(p); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, req)
			return
		}
		// Not a file — return index.html for the SPA router. But skip if the
		// request clearly wants a different content type (e.g. .json, .png).
		if path.Ext(p) != "" {
			http.NotFound(w, req)
			return
		}
		req.URL.Path = "/"
		fileServer.ServeHTTP(w, req)
	}

	r.Get("/", handler)
	r.Get("/*", handler)
}
