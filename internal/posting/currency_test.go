package posting_test

import (
	"context"
	"errors"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/posting"
)

// TestForeignCurrencyPostingAndFX posts a EUR invoice and settles it with a EUR
// payment made at a stronger rate, and checks that (a) each entry converts to
// the USD base at its own date's rate and (b) settling the two realizes the
// exchange gain into the FX account, leaving A/R exactly zero in base.
func TestForeignCurrencyPostingAndFX(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	exec, queryID := execAndQueryID(ctx, t, tx)

	// Base currency is USD (the migration's default on an empty ledger); the
	// FX account is the seeded 7000. Rates: EUR is 1.10 from Jun 1, 1.20 from
	// Jun 20 — so an invoice on the 15th converts at 1.10, a payment on the
	// 25th at 1.20.
	exec(`INSERT INTO exchange_rates (currency_code, rate_date, rate) VALUES
	      ('EUR','2026-06-01',1.10), ('EUR','2026-06-20',1.20)`)
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)

	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('EuroCo') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)

	// EUR 100 invoice on Jun 15 → base 110.
	invID := queryID(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
	      VALUES ('INV-EUR',$1,'2026-06-15','EUR') RETURNING id`, custID)
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      VALUES ($1,1,'Service',1,100,(SELECT id FROM accounts WHERE code='4000'))`, invID)
	if _, err := posting.PostSalesInvoice(ctx, tx, invID); err != nil {
		t.Fatalf("post invoice: %v", err)
	}

	// EUR 100 payment on Jun 25 → base 120.
	payID := queryID(`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, deposit_account_id)
	      VALUES ($1,'2026-06-25','EUR',100,(SELECT id FROM accounts WHERE code='1000')) RETURNING id`, custID)
	if _, err := posting.PostCustomerPayment(ctx, tx, payID); err != nil {
		t.Fatalf("post payment: %v", err)
	}
	if _, err := posting.AutoApplyCustomerPayment(ctx, tx, payID); err != nil {
		t.Fatalf("auto-apply: %v", err)
	}

	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Fatalf("a generated journal entry is unbalanced: %v", err)
	}

	// Base-currency balances (debit-positive):
	//   revenue 4000 → -110 (invoice, converted at 1.10)
	//   cash    1000 → +120 (payment, converted at 1.20)
	//   A/R     1100 →    0 (110 raised, 120 relieved, +10 FX true-up)
	//   FX      7000 →  -10 (a gain: credit balance on the expense account)
	want := map[string]string{
		"1000": "120.0000",
		"1100": "0.0000",
		"4000": "-110.0000",
		"7000": "-10.0000",
	}
	for code, expected := range want {
		var bal string
		if err := tx.QueryRow(ctx, `SELECT balance::text FROM trial_balance WHERE code = $1`, code).Scan(&bal); err != nil {
			t.Fatalf("balance %s: %v", code, err)
		}
		if bal != expected {
			t.Errorf("account %s base balance = %s, want %s", code, bal, expected)
		}
	}

	// The whole ledger still nets to zero in base.
	var sum string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(balance),0)::text FROM trial_balance`).Scan(&sum); err != nil {
		t.Fatalf("sum trial balance: %v", err)
	}
	if sum != "0.0000" {
		t.Errorf("trial balance does not net to zero: %s", sum)
	}

	// Within the EUR subledger the transaction-currency A/R clears exactly:
	// 100 EUR invoiced, 100 EUR received. (The base-only FX true-up rides on a
	// separate USD entry, so the transaction column is only summed per
	// currency, never across.)
	var eurAR string
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(sum(jl.debit - jl.credit),0)::text
		 FROM journal_lines jl JOIN journal_entries je ON je.id = jl.journal_entry_id
		 WHERE jl.account_id = (SELECT id FROM accounts WHERE code='1100')
		   AND je.currency_code = 'EUR'`).Scan(&eurAR); err != nil {
		t.Fatalf("EUR A/R: %v", err)
	}
	if eurAR != "0.0000" {
		t.Errorf("EUR transaction-currency A/R = %s, want 0.0000", eurAR)
	}
}

// TestPostWithoutExchangeRate rejects posting a foreign-currency document when
// no rate covers its date.
func TestPostWithoutExchangeRate(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	exec, queryID := execAndQueryID(ctx, t, tx)

	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)
	// A GBP rate exists, but only from Jul — nothing covers a June document.
	exec(`INSERT INTO exchange_rates (currency_code, rate_date, rate) VALUES ('GBP','2026-07-01',1.30)`)

	custID := queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('PoundCo') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)
	invID := queryID(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
	      VALUES ('INV-GBP',$1,'2026-06-15','GBP') RETURNING id`, custID)
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      VALUES ($1,1,'Service',1,100,(SELECT id FROM accounts WHERE code='4000'))`, invID)

	_, err = posting.PostSalesInvoice(ctx, tx, invID)
	if !errors.Is(err, posting.ErrNoExchangeRate) {
		t.Fatalf("post GBP invoice without a covering rate: got %v, want ErrNoExchangeRate", err)
	}
}
