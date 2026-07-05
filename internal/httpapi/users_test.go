package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"tadmor/internal/auth"
	"tadmor/internal/dbtest"
	"tadmor/internal/httpapi"
)

func TestUserAdminEndpoints(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	resetAuthed(ctx, t, pool)

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	// The list shows the seeded login user.
	if status, body := get(t, srv.URL+"/api/users"); status != http.StatusOK ||
		!strings.Contains(body, `"email":"test@example.com"`) {
		t.Fatalf("list users: status = %d, body = %s", status, body)
	}

	// Create a user.
	status, body := postJSON(t, srv.URL+"/api/users",
		`{"email":"new@example.com","full_name":"New User","password":"s3cret-pw"}`)
	if status != http.StatusCreated {
		t.Fatalf("create user: status = %d, body = %s", status, body)
	}
	var created struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.ID <= 0 {
		t.Fatalf("decode created id %q: %v", body, err)
	}
	userURL := srv.URL + "/api/users/" + strconv.Itoa(created.ID)

	// Guardrails: short password and duplicate email.
	if status, body := postJSON(t, srv.URL+"/api/users",
		`{"email":"short@example.com","full_name":"S","password":"short"}`); status != http.StatusBadRequest {
		t.Fatalf("short password: status = %d, want 400 (body: %s)", status, body)
	}
	if status, body := postJSON(t, srv.URL+"/api/users",
		`{"email":"new@example.com","full_name":"Dup","password":"s3cret-pw"}`); status != http.StatusConflict {
		t.Fatalf("duplicate email: status = %d, want 409 (body: %s)", status, body)
	}

	// The new user can log in.
	if status, body := postJSON(t, srv.URL+"/api/auth/login",
		`{"email":"new@example.com","password":"s3cret-pw"}`); status != http.StatusOK {
		t.Fatalf("login as new user: status = %d, body = %s", status, body)
	}

	// Update: rename and deactivate; the login stops working.
	if status, body := putJSON(t, userURL,
		`{"email":"new@example.com","full_name":"Renamed User","is_active":false}`); status != http.StatusNoContent {
		t.Fatalf("update user: status = %d, body = %s", status, body)
	}
	if status, body := get(t, userURL); status != http.StatusOK ||
		!strings.Contains(body, `"full_name":"Renamed User"`) || !strings.Contains(body, `"is_active":false`) ||
		!strings.Contains(body, `"is_admin":false`) {
		t.Fatalf("get updated user: status = %d, body = %s", status, body)
	}
	if status, _ := postJSON(t, srv.URL+"/api/auth/login",
		`{"email":"new@example.com","password":"s3cret-pw"}`); status != http.StatusUnauthorized {
		t.Fatalf("login as deactivated user: status = %d, want 401", status)
	}

	// Self-deactivation is refused.
	var me struct {
		ID int `json:"id"`
	}
	_, meBody := get(t, srv.URL+"/api/auth/me")
	if err := json.Unmarshal([]byte(meBody), &me); err != nil {
		t.Fatalf("decode me %q: %v", meBody, err)
	}
	if status, body := putJSON(t, srv.URL+"/api/users/"+strconv.Itoa(me.ID),
		`{"email":"test@example.com","full_name":"Test User","is_active":false,"is_admin":true}`); status != http.StatusBadRequest {
		t.Fatalf("self-deactivate: status = %d, want 400 (body: %s)", status, body)
	}

	// Self-demotion is refused too, for the same lockout reason.
	if status, body := putJSON(t, srv.URL+"/api/users/"+strconv.Itoa(me.ID),
		`{"email":"test@example.com","full_name":"Test User","is_active":true,"is_admin":false}`); status != http.StatusBadRequest {
		t.Fatalf("self-demote: status = %d, want 400 (body: %s)", status, body)
	}

	// Password reset: reactivate (and promote, checking the is_admin
	// roundtrip), set a new password, and check the swap. The user's sessions
	// are revoked as part of the reset.
	if status, body := putJSON(t, userURL,
		`{"email":"new@example.com","full_name":"Renamed User","is_active":true,"is_admin":true}`); status != http.StatusNoContent {
		t.Fatalf("reactivate user: status = %d, body = %s", status, body)
	}
	if status, body := get(t, userURL); status != http.StatusOK ||
		!strings.Contains(body, `"is_admin":true`) {
		t.Fatalf("get promoted user: status = %d, body = %s", status, body)
	}
	if status, body := postJSON(t, userURL+"/password", `{"password":"a-new-secret"}`); status != http.StatusNoContent {
		t.Fatalf("set password: status = %d, body = %s", status, body)
	}
	if status, _ := postJSON(t, srv.URL+"/api/auth/login",
		`{"email":"new@example.com","password":"s3cret-pw"}`); status != http.StatusUnauthorized {
		t.Fatalf("login with old password: status = %d, want 401", status)
	}
	if status, body := postJSON(t, srv.URL+"/api/auth/login",
		`{"email":"new@example.com","password":"a-new-secret"}`); status != http.StatusOK {
		t.Fatalf("login with new password: status = %d, body = %s", status, body)
	}
	var sessions int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE u.email = 'new@example.com'`).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	// Only the post-reset login's session survives; the pre-reset one is gone.
	if sessions != 1 {
		t.Errorf("sessions after reset = %d, want 1 (reset revokes earlier sessions)", sessions)
	}

	// Missing user -> 404 across the endpoints.
	if status, _ := get(t, srv.URL+"/api/users/999999"); status != http.StatusNotFound {
		t.Errorf("get missing user: status = %d, want 404", status)
	}
	if status, _ := putJSON(t, srv.URL+"/api/users/999999",
		`{"email":"x@example.com","full_name":"X","is_active":true}`); status != http.StatusNotFound {
		t.Errorf("update missing user: status = %d, want 404", status)
	}
	if status, _ := postJSON(t, srv.URL+"/api/users/999999/password", `{"password":"a-new-secret"}`); status != http.StatusNotFound {
		t.Errorf("password of missing user: status = %d, want 404", status)
	}
}

// TestAdminGate ensures a non-administrator session gets 403 on the
// admin-only routes while the day-to-day API stays open to it.
func TestAdminGate(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	resetAuthed(ctx, t, pool)

	var userID int
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, full_name, password_hash)
		 VALUES ('plain@example.com', 'Plain User', 'unused') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed non-admin: %v", err)
	}
	token, err := auth.CreateSession(ctx, pool, userID)
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	request := func(method, path, body string) (int, string) {
		t.Helper()
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, srv.URL+path, rd)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "tadmor_session", Value: token})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	// The gate fires before any handler logic, so nonexistent ids still 403.
	forbidden := []struct{ method, path, body string }{
		{"GET", "/api/users", ""},
		{"GET", "/api/users/1", ""},
		{"POST", "/api/users", `{"email":"x@example.com","full_name":"X","password":"s3cret-pw"}`},
		{"PUT", "/api/users/1", `{"email":"x@example.com","full_name":"X","is_active":true}`},
		{"POST", "/api/users/1/password", `{"password":"a-new-secret"}`},
		{"POST", "/api/sales-invoices/1/unpost", ""},
		{"POST", "/api/purchase-bills/1/unpost", ""},
		{"POST", "/api/customer-payments/1/unpost", ""},
		{"POST", "/api/supplier-payments/1/unpost", ""},
		{"POST", "/api/stock-movements/1/unpost", ""},
	}
	for _, tc := range forbidden {
		if status, body := request(tc.method, tc.path, tc.body); status != http.StatusForbidden {
			t.Errorf("%s %s as non-admin: status = %d, want 403 (body: %s)", tc.method, tc.path, status, body)
		}
	}

	// The rest of the API still works for the non-admin.
	if status, body := request("GET", "/api/accounts", ""); status != http.StatusOK {
		t.Errorf("GET /api/accounts as non-admin: status = %d, want 200 (body: %s)", status, body)
	}
	if status, body := request("GET", "/api/auth/me", ""); status != http.StatusOK ||
		!strings.Contains(body, `"is_admin":false`) {
		t.Errorf("GET /api/auth/me as non-admin: status = %d, body = %s (want is_admin false)", status, body)
	}
}
