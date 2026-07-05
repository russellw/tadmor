package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SessionTTL is how long a login session stays valid. Fixed, not sliding: a
// session ends SessionTTL after login no matter how active it was.
const SessionTTL = 30 * 24 * time.Hour

// ErrNoUser is returned when no active user matches the given email.
var ErrNoUser = errors.New("no such user")

// ErrNoSession is returned when a token matches no live session.
var ErrNoSession = errors.New("no such session")

// DB is satisfied by both *pgxpool.Pool and pgx.Tx.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// User is the authenticated identity attached to a request.
type User struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
}

// Credentials returns the active user for the email along with the stored
// password hash, or ErrNoUser. The caller verifies the password; the hash is
// never returned to clients.
func Credentials(ctx context.Context, db DB, email string) (User, string, error) {
	var u User
	var hash string
	err := db.QueryRow(ctx,
		`SELECT id, email, full_name, password_hash FROM users WHERE email = $1 AND is_active`,
		email).Scan(&u.ID, &u.Email, &u.FullName, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNoUser
	}
	if err != nil {
		return User{}, "", err
	}
	return u, hash, nil
}

// UpsertUser creates a user or, when the email already exists, resets its name
// and password and reactivates it. Returns the user id.
func UpsertUser(ctx context.Context, db DB, email, fullName, passwordHash string) (int, error) {
	var id int
	err := db.QueryRow(ctx,
		`INSERT INTO users (email, full_name, password_hash) VALUES ($1, $2, $3)
		 ON CONFLICT (email) DO UPDATE
		 SET full_name = EXCLUDED.full_name, password_hash = EXCLUDED.password_hash, is_active = true
		 RETURNING id`,
		email, fullName, passwordHash).Scan(&id)
	return id, err
}

// CreateSession mints a session for the user and returns the bearer token the
// client stores in its cookie. Only the token's SHA-256 lands in the database.
// Expired rows are pruned here, so no background job is needed.
func CreateSession(ctx context.Context, db DB, userID int) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(token))
	_, err := db.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		h[:], userID, time.Now().Add(SessionTTL))
	if err != nil {
		return "", err
	}
	return token, nil
}

// SessionUser resolves a bearer token to its user, or ErrNoSession if the
// token is unknown, expired, or belongs to a deactivated user.
func SessionUser(ctx context.Context, db DB, token string) (User, error) {
	h := sha256.Sum256([]byte(token))
	var u User
	err := db.QueryRow(ctx,
		`SELECT u.id, u.email, u.full_name
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now() AND u.is_active`,
		h[:]).Scan(&u.ID, &u.Email, &u.FullName)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNoSession
	}
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// DeleteSession revokes the session for the token. Deleting an unknown token
// is not an error, so logout is idempotent.
func DeleteSession(ctx context.Context, db DB, token string) error {
	h := sha256.Sum256([]byte(token))
	_, err := db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, h[:])
	return err
}
