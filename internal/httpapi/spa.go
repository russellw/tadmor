package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// contentSecurityPolicy is a conservative policy for the public-facing SPA. The
// production bundle loads scripts and styles same-origin; React and ECharts need
// no eval. 'unsafe-inline' is permitted for styles only (UI libraries set inline
// style attributes); tighten to hashes/nonces later if desired.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"frame-ancestors 'none'"

// spaHandler serves the embedded single-page app: static files when they exist,
// otherwise index.html so the client-side router can handle the route.
func spaHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if f, err := distFS.Open(name); err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback.
		index, err := fs.ReadFile(distFS, "index.html")
		if err != nil {
			http.Error(w, "front-end not built; run: make web-build", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}
