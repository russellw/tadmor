package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tadmor/internal/auth"
	"tadmor/internal/dbtest"
	"tadmor/internal/httpapi"
)

// TestAuthFlow drives the whole session lifecycle over HTTP: reject without a
// session, login (bad and good), authenticated calls, logout, and rejection
// after logout.
func TestAuthFlow(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	hash, err := auth.HashPassword("s3cret-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := auth.UpsertUser(ctx, pool, "alice@example.com", "Alice", hash); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	// request issues a call with an optional session cookie and returns the
	// response (body already read and closed).
	request := func(method, path, body, token string) (*http.Response, string) {
		t.Helper()
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, srv.URL+path, r)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "tadmor_session", Value: token})
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	// No session: protected routes are 401, health stays open.
	if resp, body := request("GET", "/api/accounts", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/accounts: status = %d, want 401 (body: %s)", resp.StatusCode, body)
	}
	if resp, _ := request("GET", "/healthz", "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz should not require auth, got %d", resp.StatusCode)
	}

	// Wrong password and unknown email both fail with the same message.
	resp, badBody := request("POST", "/api/auth/login", `{"email":"alice@example.com","password":"wrong"}`, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: status = %d, want 401", resp.StatusCode)
	}
	resp, unknownBody := request("POST", "/api/auth/login", `{"email":"nobody@example.com","password":"wrong"}`, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown email: status = %d, want 401", resp.StatusCode)
	}
	if badBody != unknownBody {
		t.Errorf("login errors differ (%q vs %q); they must not reveal which emails exist", badBody, unknownBody)
	}

	// Successful login returns the user and sets the session cookie.
	resp, body := request("POST", "/api/auth/login", `{"email":"alice@example.com","password":"s3cret-password"}`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status = %d, want 200 (body: %s)", resp.StatusCode, body)
	}
	var user struct {
		ID       int    `json:"id"`
		Email    string `json:"email"`
		FullName string `json:"full_name"`
	}
	if err := json.Unmarshal([]byte(body), &user); err != nil || user.Email != "alice@example.com" {
		t.Fatalf("login body %q: err=%v user=%+v", body, err, user)
	}
	var token string
	for _, c := range resp.Cookies() {
		if c.Name == "tadmor_session" {
			token = c.Value
			if !c.HttpOnly {
				t.Error("session cookie is not HttpOnly")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("session cookie SameSite = %v, want Lax", c.SameSite)
			}
		}
	}
	if token == "" {
		t.Fatal("login did not set the session cookie")
	}

	// The session opens the API and identifies the user.
	if resp, body := request("GET", "/api/accounts", "", token); resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated GET /api/accounts: status = %d (body: %s)", resp.StatusCode, body)
	}
	if _, body := request("GET", "/api/auth/me", "", token); !strings.Contains(body, `"alice@example.com"`) {
		t.Fatalf("GET /api/auth/me body = %s, want alice@example.com", body)
	}

	// A made-up token is rejected.
	if resp, _ := request("GET", "/api/accounts", "", "forged-token"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("forged token: status = %d, want 401", resp.StatusCode)
	}

	// Logout revokes the session server-side.
	if resp, body := request("POST", "/api/auth/logout", "", token); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: status = %d, want 204 (body: %s)", resp.StatusCode, body)
	}
	if resp, _ := request("GET", "/api/accounts", "", token); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after logout: status = %d, want 401", resp.StatusCode)
	}
	// Logging out again is a no-op, not an error.
	if resp, _ := request("POST", "/api/auth/logout", "", token); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("second logout: status = %d, want 204", resp.StatusCode)
	}
}

// TestSessionOfDeactivatedUser ensures deactivating a user kills their live
// sessions immediately.
func TestSessionOfDeactivatedUser(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	id, err := auth.UpsertUser(ctx, pool, "bob@example.com", "Bob", "unused")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	token, err := auth.CreateSession(ctx, pool, id)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := auth.SessionUser(ctx, pool, token); err != nil {
		t.Fatalf("live session rejected: %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE users SET is_active = false WHERE id = $1`, id); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := auth.SessionUser(ctx, pool, token); err != auth.ErrNoSession {
		t.Fatalf("deactivated user's session: err = %v, want ErrNoSession", err)
	}
}
