package auth_test

import (
	"strings"
	"testing"

	"tadmor/internal/auth"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$") {
		t.Fatalf("hash = %q, want pbkdf2-sha256$ prefix", hash)
	}
	if !auth.VerifyPassword(hash, "correct horse battery staple") {
		t.Error("correct password rejected")
	}
	if auth.VerifyPassword(hash, "wrong password") {
		t.Error("wrong password accepted")
	}
}

func TestHashPasswordSalts(t *testing.T) {
	a, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	b, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two hashes of the same password are identical; salt is not random")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	for _, stored := range []string{
		"",
		"plaintext",
		"bcrypt$10$abc$def",
		"pbkdf2-sha256$notanumber$c2FsdA==$a2V5",
		"pbkdf2-sha256$-1$c2FsdA==$a2V5",
		"pbkdf2-sha256$1000$!!$a2V5",
		"pbkdf2-sha256$1000$c2FsdA==$",
	} {
		if auth.VerifyPassword(stored, "pw") {
			t.Errorf("VerifyPassword(%q) = true, want false", stored)
		}
	}
}

func TestDummyHashIsWellFormed(t *testing.T) {
	// The dummy must run the full PBKDF2 path (that is its whole purpose) and
	// must never match a real password attempt.
	if auth.VerifyPassword(auth.DummyHash, "") {
		t.Error("empty password matches the dummy hash")
	}
	if strings.Count(auth.DummyHash, "$") != 3 {
		t.Errorf("DummyHash %q is not scheme$iter$salt$key", auth.DummyHash)
	}
}
