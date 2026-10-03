package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"tadmor/internal/auth"
)

// User administration, reachable only through the admin wrapper (see
// server.go). Password hashes never leave the auth package; passwords arrive
// only over dedicated create/reset requests.

const minPasswordLen = 8 // matches the CLI's -adduser rule

func (s *Server) writeUserError(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrNoUser) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	// Postgres error mapping (23505 duplicate email -> 409, ...) plus the
	// logged 500 fallback are shared with the master-data handlers.
	s.writeMasterError(w, err)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := auth.ListUsers(r.Context(), s.pool)
	if err != nil {
		s.writeUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, err := auth.GetUser(r.Context(), s.pool, id)
	if err != nil {
		s.writeUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	in.FullName = strings.TrimSpace(in.FullName)
	if !validEmailAndName(w, in.Email, in.FullName) || !validPassword(w, in.Password) {
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.log.Error("hash password failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	id, err := auth.CreateUser(r.Context(), s.pool, in.Email, in.FullName, hash, in.IsAdmin)
	s.created(w, id, err)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		IsActive bool   `json:"is_active"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	in.FullName = strings.TrimSpace(in.FullName)
	if !validEmailAndName(w, in.Email, in.FullName) {
		return
	}
	// Refuse self-deactivation and self-demotion: the guardrails against
	// locking every administrator out one click at a time.
	if me, ok := requestUser(r.Context()); ok && me.ID == id {
		if !in.IsActive {
			writeError(w, http.StatusUnprocessableEntity, "you cannot deactivate your own account")
			return
		}
		if !in.IsAdmin {
			writeError(w, http.StatusUnprocessableEntity, "you cannot remove your own administrator access")
			return
		}
	}
	if err := auth.UpdateUser(r.Context(), s.pool, id, in.Email, in.FullName, in.IsActive, in.IsAdmin); err != nil {
		s.writeUserError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setUserPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) || !validPassword(w, in.Password) {
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.log.Error("hash password failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := auth.SetPassword(r.Context(), s.pool, id, hash); err != nil {
		s.writeUserError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validEmailAndName writes the response and reports false when the email or
// name is unacceptable: a missing field is a 400, a malformed email a 422.
func validEmailAndName(w http.ResponseWriter, email, fullName string) bool {
	switch {
	case email == "":
		writeError(w, http.StatusBadRequest, "email is required")
	case fullName == "":
		writeError(w, http.StatusBadRequest, "full_name is required")
	case !strings.Contains(email, "@"):
		writeError(w, http.StatusUnprocessableEntity, "email must contain @")
	default:
		return true
	}
	return false
}

// validPassword writes the response and reports false when the password is
// missing (400) or too short (422).
func validPassword(w http.ResponseWriter, password string) bool {
	switch {
	case password == "":
		writeError(w, http.StatusBadRequest, "password is required")
	case len(password) < minPasswordLen:
		writeError(w, http.StatusUnprocessableEntity, "password must be at least 8 characters")
	default:
		return true
	}
	return false
}
