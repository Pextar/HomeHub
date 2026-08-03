package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaHandler serves files from `dir`, falling back to index.html for
// any path that doesn't map to an actual file. This is what makes the
// Svelte SPA's hash-free deep links work on a hard refresh and PWA
// navigation requests.
//
// Without explicit Cache-Control, browsers fall back to heuristic
// freshness for index.html — and iOS's standalone "Add to Home Screen"
// mode leans on that heuristic especially hard, routinely serving a
// stale shell for days across reopens (even a fresh reload or removing
// and re-adding the icon doesn't reliably bust it). index.html itself
// is tiny and never worth caching; the content-hashed files under
// assets/ are immutable by construction (a new build gets new
// filenames), so those get a long-lived cache instead.
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	indexPath := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes are matched before this handler, so we never see
		// them here; just guard against a missing build with a clear
		// message.
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			http.Error(w,
				"frontend/dist/index.html is missing — run `npm install && npm run build` in ./frontend.",
				http.StatusServiceUnavailable)
			return
		}

		// Try the literal file first.
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fs.ServeHTTP(w, r)
			return
		}

		// Fallback: serve the SPA shell.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, indexPath)
	})
}
