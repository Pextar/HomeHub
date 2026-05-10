package api

import (
	"net/http"
	"os"
	"path/filepath"
)

// spaHandler serves files from `dir`, falling back to index.html for
// any path that doesn't map to an actual file. This is what makes the
// Svelte SPA's hash-free deep links work on a hard refresh and PWA
// navigation requests.
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
			fs.ServeHTTP(w, r)
			return
		}

		// Fallback: serve the SPA shell.
		http.ServeFile(w, r, indexPath)
	})
}
