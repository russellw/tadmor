package db

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationsDir resolves db/migrations relative to this source file so the test
// works regardless of the working directory.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations")
}

// testPool connects to the dedicated test database, skipping the test when none
// is configured so `go test ./...` stays green without a database.
func testPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return pool
}

func TestMigrationsAndInvariants(t *testing.T) {
	ctx := context.Background()
	pool := testPool(ctx, t)
	defer pool.Close()

	// Reset the (dedicated) test database to a clean slate.
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}

	dir := migrationsDir(t)

	applied, err := Apply(ctx, pool, dir)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("expected migrations to be applied, got none")
	}
	t.Logf("applied %d migrations: %v", len(applied), applied)

	// Running again must be a no-op (idempotent).
	again, err := Apply(ctx, pool, dir)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected 0 migrations on second run, got %d", len(again))
	}

	// The full schema should be present.
	var tables int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`).Scan(&tables); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tables < 20 {
		t.Fatalf("expected the full schema, only found %d tables", tables)
	}

	t.Run("balanced entry posts and reaches the trial balance", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		exec := func(sql string) {
			t.Helper()
			if _, err := tx.Exec(ctx, sql); err != nil {
				t.Fatalf("exec failed: %v\nsql: %s", err, sql)
			}
		}
		exec(`INSERT INTO fiscal_years (name, start_date, end_date)
		      VALUES ('FY2026', '2026-01-01', '2026-12-31')`)
		exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
		      SELECT id, '2026-06', '2026-06-01', '2026-06-30' FROM fiscal_years WHERE name = 'FY2026'`)
		exec(`INSERT INTO journal_entries (entry_date, period_id, currency_code, memo, status)
		      SELECT '2026-06-15', (SELECT id FROM accounting_periods WHERE name = '2026-06'),
		             'USD', 'test entry', 'draft'`)
		exec(`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit)
		      SELECT (SELECT id FROM journal_entries WHERE memo = 'test entry'),
		             1, (SELECT id FROM accounts WHERE code = '1000'), 100, 0`)
		exec(`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit)
		      SELECT (SELECT id FROM journal_entries WHERE memo = 'test entry'),
		             2, (SELECT id FROM accounts WHERE code = '4000'), 0, 100`)
		exec(`UPDATE journal_entries SET status = 'posted', posted_at = now() WHERE memo = 'test entry'`)
		// Force the deferred balance constraint to run now (before commit).
		exec(`SET CONSTRAINTS ALL IMMEDIATE`)

		var balance string
		if err := tx.QueryRow(ctx,
			`SELECT balance::text FROM trial_balance WHERE code = '1000'`).Scan(&balance); err != nil {
			t.Fatalf("read trial balance: %v", err)
		}
		if balance != "100.0000" {
			t.Fatalf("expected cash balance 100.0000, got %s", balance)
		}
	})

	t.Run("unbalanced posted entry is rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		mustExec := func(sql string) {
			t.Helper()
			if _, err := tx.Exec(ctx, sql); err != nil {
				t.Fatalf("setup exec failed: %v\nsql: %s", err, sql)
			}
		}
		mustExec(`INSERT INTO fiscal_years (name, start_date, end_date)
		          VALUES ('FY2026', '2026-01-01', '2026-12-31')`)
		mustExec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
		          SELECT id, '2026-06', '2026-06-01', '2026-06-30' FROM fiscal_years WHERE name = 'FY2026'`)
		mustExec(`INSERT INTO journal_entries (entry_date, period_id, currency_code, memo, status)
		          SELECT '2026-06-15', (SELECT id FROM accounting_periods WHERE name = '2026-06'),
		                 'USD', 'bad entry', 'draft'`)
		mustExec(`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit)
		          SELECT (SELECT id FROM journal_entries WHERE memo = 'bad entry'),
		                 1, (SELECT id FROM accounts WHERE code = '1000'), 100, 0`)
		mustExec(`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit)
		          SELECT (SELECT id FROM journal_entries WHERE memo = 'bad entry'),
		                 2, (SELECT id FROM accounts WHERE code = '4000'), 0, 90`)
		mustExec(`UPDATE journal_entries SET status = 'posted' WHERE memo = 'bad entry'`)

		// The deferred balance check must now reject the unbalanced entry.
		if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err == nil {
			t.Fatal("expected unbalanced posted entry to be rejected, but it was accepted")
		}
	})
}
