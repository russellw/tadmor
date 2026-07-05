package httpapi_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"tadmor/internal/httpapi"
)

// TestRoutingSPAandAPI exercises the /api prefix and the SPA fallback without a
// database (the pool is only touched by /readyz, which these cases never hit).
func TestRoutingSPAandAPI(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>SPA</title>")},
		"assets/app.js": {Data: []byte("console.log('app')")},
	}
	h := httpapi.NewServer(nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(dist)
	srv := httptest.NewServer(h)
	defer srv.Close()

	cases := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string // substring
		wantCSP    bool
	}{
		{"health at root", "/healthz", http.StatusOK, `"status":"ok"`, false},
		// Auth wraps the whole API, so without a session an unknown API path
		// is 401 (never the SPA fallback). requireAuth rejects cookie-less
		// requests before touching the (nil) pool.
		{"unknown api path is 401, not SPA", "/api/does-not-exist", http.StatusUnauthorized, "", false},
		{"root serves index", "/", http.StatusOK, "<title>SPA</title>", true},
		{"client route falls back to index", "/customers/42", http.StatusOK, "<title>SPA</title>", true},
		{"static asset is served", "/assets/app.js", http.StatusOK, "console.log('app')", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", resp.StatusCode, tc.wantStatus, body)
			}
			if tc.wantBody != "" && !strings.Contains(string(body), tc.wantBody) {
				t.Errorf("body = %q, want to contain %q", body, tc.wantBody)
			}
			if tc.wantCSP && resp.Header.Get("Content-Security-Policy") == "" {
				t.Errorf("expected a Content-Security-Policy header on %s", tc.path)
			}
		})
	}
}
