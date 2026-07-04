package db

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// schemaMigrationsDDL creates the bookkeeping table that records which
// migrations have already run. It is intentionally idempotent.
const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    text        PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);`

// Apply runs every "*.up.sql" file at the root of fsys (normally the embedded
// db/migrations tree), in lexical (version) order, that has not yet been
// recorded in schema_migrations. Each file is executed atomically together
// with the row that records it, so a failed migration leaves no trace.
// It returns the versions that were newly applied. An fsys with no migration
// files at all is an error: it means a broken build, not an up-to-date schema.
//
// Multi-statement files (including pl/pgsql function bodies) are sent as a
// single simple-query batch, which pgx supports for argument-less Exec.
func Apply(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) ([]string, error) {
	if _, err := pool.Exec(ctx, schemaMigrationsDDL); err != nil {
		return nil, fmt.Errorf("db: ensure schema_migrations: %w", err)
	}

	done, err := appliedVersions(ctx, pool)
	if err != nil {
		return nil, err
	}

	files, err := fs.Glob(fsys, "*.up.sql")
	if err != nil {
		return nil, fmt.Errorf("db: list migrations: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("db: no *.up.sql migration files found")
	}
	sort.Strings(files)

	var applied []string
	for _, f := range files {
		version := strings.TrimSuffix(f, ".up.sql")
		if done[version] {
			continue
		}
		sql, err := fs.ReadFile(fsys, f)
		if err != nil {
			return applied, fmt.Errorf("db: read %s: %w", f, err)
		}
		if err := applyOne(ctx, pool, version, string(sql)); err != nil {
			return applied, fmt.Errorf("db: apply %s: %w", version, err)
		}
		applied = append(applied, version)
	}
	return applied, nil
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("db: read applied migrations: %w", err)
	}
	defer rows.Close()

	done := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		done[v] = true
	}
	return done, rows.Err()
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, version, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
