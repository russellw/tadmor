// Package dbtest provides shared setup for database integration tests.
package dbtest

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"tadmor/internal/db"
)

// advisoryLockKey serializes all DB integration tests. `go test ./...` runs
// package test binaries concurrently; since they share one database, each holds
// this session-level advisory lock for its duration so they don't trample each
// other (e.g. one resetting the schema while another reads it).
const advisoryLockKey int64 = 0x7461646d6f72 // "tadmor"

// MigrationsDir returns the absolute path to db/migrations, resolved relative to
// this source file so it works regardless of the test's working directory.
func MigrationsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations")
}

// Acquire connects to TEST_DATABASE_URL (skipping the test if unset), takes the
// shared advisory lock, and returns a pool plus a cleanup function that releases
// the lock and closes the pool.
func Acquire(ctx context.Context, t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Hold the lock on a dedicated connection for the whole test.
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		t.Fatalf("acquire connection: %v", err)
	}
	if _, err := lockConn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		lockConn.Release()
		pool.Close()
		t.Fatalf("advisory lock: %v", err)
	}

	cleanup := func() {
		_, _ = lockConn.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockKey)
		lockConn.Release()
		pool.Close()
	}
	return pool, cleanup
}

// Reset drops and recreates the public schema, then applies all migrations,
// leaving the database at a clean, fully-migrated state. Safe to call from a
// test holding the advisory lock from Acquire.
func Reset(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if _, err := db.Apply(ctx, pool, MigrationsDir(t)); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
}
