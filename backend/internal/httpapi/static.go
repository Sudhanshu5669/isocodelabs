package httpapi

import (
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// contentSecurityPolicy tells the browser what the page may load. Only our
// own scripts run (no inline scripts, nothing from other sites), so injected
// HTML cannot execute code. Fonts and their stylesheet come from Google Fonts.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com; " +
	"img-src 'self' data: blob:; " +
	"connect-src 'self'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// spaHandler serves the built React app (frontend/dist) from dir.
//
// A path that is a real file (/assets/index-3f2a.js, /images/rooms/a.jpg) is
// served as is. Any other path (/visit/abc123, /staff/leads) gets index.html,
// so React Router can show the right screen when such a link is opened
// directly. This is the usual setup for a single-page app.
func spaHandler(dir string) http.Handler {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		slog.Warn("STATIC_DIR has no index.html; build the frontend first", "dir", dir)
	}
	root := http.Dir(dir) // http.Dir refuses paths that climb out of dir ("../")
	files := http.FileServer(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)

		p := path.Clean("/" + r.URL.Path)
		if f, err := root.Open(p); err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				if strings.HasPrefix(p, "/assets/") {
					// Vite puts a content hash in these names, so they never change.
					h.Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					h.Set("Cache-Control", "no-cache")
				}
				files.ServeHTTP(w, r)
				return
			}
		}

		// Not a file: let the React app route it. index.html is never cached,
		// so a new deploy is picked up on the next visit.
		h.Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}
