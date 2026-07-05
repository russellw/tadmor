// Package auth implements password hashing and database-backed login
// sessions. It sticks to the standard library: PBKDF2-HMAC-SHA256 (crypto/pbkdf2,
// stdlib since Go 1.24) for passwords, crypto/rand tokens hashed with SHA-256
// for sessions.
package auth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// PBKDF2-HMAC-SHA256 parameters (OWASP-recommended iteration count). The
// iteration count is stored inside each hash, so it can be raised later
// without invalidating existing credentials.
const (
	hashScheme     = "pbkdf2-sha256"
	hashIterations = 600_000
	saltLen        = 16
	keyLen         = 32
)

// DummyHash is a valid hash of a random, discarded password. The login handler
// verifies against it when the email matches no user, so response timing does
// not reveal which emails exist.
const DummyHash = "pbkdf2-sha256$600000$wa0Jw4cjxBPg3+gUn0BROw==$1plOTJAJ8tzeDZtf9333ufRXoUpy1yRiBzWhjJzqB0s="

// HashPassword derives a storable hash of the password. The result encodes the
// scheme, iteration count, salt, and derived key, separated by '$'.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, hashIterations, keyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%d$%s$%s", hashScheme, hashIterations,
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether the password matches the stored hash.
func VerifyPassword(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != hashScheme {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return hmac.Equal(got, want)
}
