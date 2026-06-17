package db_test

import (
	"context"
	"testing"

	"tadmor/internal/db"
	"tadmor/internal/dbtest"
)

func TestMigrationsAndInvariants(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()

	dir := dbtest.MigrationsDir(t)

	// Reset to a clean slate. Safe because the advisory lock serializes DB tests.
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}

	applied, err := db.Apply(ctx, pool, dir)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("expected migrations to be applied, got none")
	}
	t.Logf("applied %d migrations: %v", len(applied), applied)

	// Running again must be a no-op (idempotent).
	again, err := db.Apply(ctx, pool, dir)
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

	t.Run("unbalanced posted entry is rejected", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		stmts := []string{
			`INSERT INTO fiscal_years (name, start_date, end_date)
			 VALUES ('FY2026', '2026-01-01', '2026-12-31')`,
			`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
			 SELECT id, '2026-06', '2026-06-01', '2026-06-30' FROM fiscal_years WHERE name = 'FY2026'`,
			`INSERT INTO journal_entries (entry_date, period_id, currency_code, memo, status)
			 SELECT '2026-06-15', (SELECT id FROM accounting_periods WHERE name = '2026-06'),
			        'USD', 'bad entry', 'draft'`,
			`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit)
			 SELECT (SELECT id FROM journal_entries WHERE memo = 'bad entry'),
			        1, (SELECT id FROM accounts WHERE code = '1000'), 100, 0`,
			`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit)
			 SELECT (SELECT id FROM journal_entries WHERE memo = 'bad entry'),
			        2, (SELECT id FROM accounts WHERE code = '4000'), 0, 90`,
			`UPDATE journal_entries SET status = 'posted' WHERE memo = 'bad entry'`,
		}
		for _, s := range stmts {
			if _, err := tx.Exec(ctx, s); err != nil {
				t.Fatalf("setup exec failed: %v\nsql: %s", err, s)
			}
		}

		// The deferred balance check must now reject the unbalanced entry.
		if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err == nil {
			t.Fatal("expected unbalanced posted entry to be rejected, but it was accepted")
		}
	})
}
