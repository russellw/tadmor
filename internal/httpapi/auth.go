package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"tadmor/internal/auth"
)

// sessionCookie names the login-session cookie. The raw token inside it is
// stored server-side only as a SHA-256 hash (see internal/auth).
const sessionCookie = "tadmor_session"

// ctxKey is unexported so only this package can attach values to the request
// context.
type ctxKey int

const userKey ctxKey = 0

// requestUser returns the authenticated user requireAuth attached, if any.
func requestUser(ctx context.Context) (auth.User, bool) {
	u, ok := ctx.Value(userKey).(auth.User)
	return u, ok
}

// requireAuth rejects requests that lack a live session and attaches the
// session's user to the request context for the wrapped handler.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		u, err := auth.SessionUser(r.Context(), s.pool, c.Value)
		if errors.Is(err, auth.ErrNoSession) {
			clearSessionCookie(w, r)
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}
		if err != nil {
			s.log.Error("session lookup failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if in.Email == "" || in.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, hash, err := auth.Credentials(r.Context(), s.pool, in.Email)
	if errors.Is(err, auth.ErrNoUser) {
		// Burn the same PBKDF2 work as a real check so timing does not
		// reveal which emails exist, then fail with the same message.
		auth.VerifyPassword(auth.DummyHash, in.Password)
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		s.log.Error("credential lookup failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !auth.VerifyPassword(hash, in.Password) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := auth.CreateSession(r.Context(), s.pool, user.ID)
	if err != nil {
		s.log.Error("create session failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(auth.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	s.log.Info("login", "user", user.Email)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := auth.DeleteSession(r.Context(), s.pool, c.Value); err != nil {
			s.log.Error("delete session failed", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// me reports the logged-in user; the SPA calls it on load to decide between
// the app shell and the login screen.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := requestUser(r.Context())
	if !ok {
		// Unreachable when routed through requireAuth; defensive.
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// requestIsHTTPS reports whether the client connection is HTTPS, either
// directly or via the reverse proxy (Caddy sets X-Forwarded-Proto).
func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
