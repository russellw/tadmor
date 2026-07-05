package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// UserRecord is a user row as the admin screens see it. The password hash
// never leaves this package.
type UserRecord struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	IsActive bool   `json:"is_active"`
}

// ListUsers returns every user, active or not, ordered by email.
func ListUsers(ctx context.Context, db DB) ([]UserRecord, error) {
	rows, err := db.Query(ctx,
		`SELECT id, email, full_name, is_active FROM users ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UserRecord{}
	for rows.Next() {
		var u UserRecord
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.IsActive); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetUser returns one user, or ErrNoUser.
func GetUser(ctx context.Context, db DB, id int) (UserRecord, error) {
	var u UserRecord
	err := db.QueryRow(ctx,
		`SELECT id, email, full_name, is_active FROM users WHERE id = $1`,
		id).Scan(&u.ID, &u.Email, &u.FullName, &u.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNoUser
	}
	return u, err
}

// CreateUser inserts a new user and returns its id. Unlike UpsertUser (the
// CLI's create-or-reset), a duplicate email is an error here so the admin
// screen cannot silently take over an existing account.
func CreateUser(ctx context.Context, db DB, email, fullName, passwordHash string) (int, error) {
	var id int
	err := db.QueryRow(ctx,
		`INSERT INTO users (email, full_name, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		email, fullName, passwordHash).Scan(&id)
	return id, err
}

// UpdateUser replaces a user's email, name, and active flag, or ErrNoUser.
// Deactivation needs no session cleanup: session lookup already requires the
// user to be active.
func UpdateUser(ctx context.Context, db DB, id int, email, fullName string, isActive bool) error {
	tag, err := db.Exec(ctx,
		`UPDATE users SET email=$2, full_name=$3, is_active=$4 WHERE id=$1`,
		id, email, fullName, isActive)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNoUser
	}
	return err
}

// SetPassword replaces a user's password hash and revokes all of the user's
// sessions (whoever held the old password stops being logged in), or ErrNoUser.
func SetPassword(ctx context.Context, db DB, id int, passwordHash string) error {
	tag, err := db.Exec(ctx,
		`UPDATE users SET password_hash=$2 WHERE id=$1`, id, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoUser
	}
	_, err = db.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
	return err
}
