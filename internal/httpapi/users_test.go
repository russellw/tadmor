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
		!strings.Contains(body, `"full_name":"Renamed User"`) || !strings.Contains(body, `"is_active":false`) {
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
		`{"email":"test@example.com","full_name":"Test User","is_active":false}`); status != http.StatusBadRequest {
		t.Fatalf("self-deactivate: status = %d, want 400 (body: %s)", status, body)
	}

	// Password reset: reactivate, set a new password, and check the swap. The
	// user's sessions are revoked as part of the reset.
	if status, body := putJSON(t, userURL,
		`{"email":"new@example.com","full_name":"Renamed User","is_active":true}`); status != http.StatusNoContent {
		t.Fatalf("reactivate user: status = %d, body = %s", status, body)
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
