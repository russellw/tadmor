package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"tadmor/internal/auth"
)

// User administration. The auth model is flat — any signed-in user may manage
// users — matching the rest of the API. Password hashes never leave the auth
// package; passwords arrive only over dedicated create/reset requests.

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
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	in.FullName = strings.TrimSpace(in.FullName)
	if msg := validEmailAndName(in.Email, in.FullName); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if len(in.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.log.Error("hash password failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	id, err := auth.CreateUser(r.Context(), s.pool, in.Email, in.FullName, hash)
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
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	in.FullName = strings.TrimSpace(in.FullName)
	if msg := validEmailAndName(in.Email, in.FullName); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	// Refuse self-deactivation: with a flat auth model this is the one
	// guardrail against locking every admin out one click at a time.
	if me, ok := requestUser(r.Context()); ok && me.ID == id && !in.IsActive {
		writeError(w, http.StatusBadRequest, "you cannot deactivate your own account")
		return
	}
	if err := auth.UpdateUser(r.Context(), s.pool, id, in.Email, in.FullName, in.IsActive); err != nil {
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
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
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

func validEmailAndName(email, fullName string) string {
	switch {
	case email == "":
		return "email is required"
	case !strings.Contains(email, "@"):
		return "email must contain @"
	case fullName == "":
		return "full_name is required"
	}
	return ""
}
