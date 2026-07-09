package posting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// reverseEntry posts a mirror of journal entry origID — every line with debit
// and credit swapped — dated in the same period, and links it back via
// reverses_entry_id. The original entry is left in place (both entries remain
// posted and net to zero), preserving the audit trail. An entry may only be
// reversed once.
func reverseEntry(ctx context.Context, tx pgx.Tx, origID int) (int, error) {
	var date, currency string
	err := tx.QueryRow(ctx,
		`SELECT entry_date::text, currency_code FROM journal_entries WHERE id = $1`, origID).
		Scan(&date, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: journal entry %d: %w", origID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}

	var already bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM journal_entries WHERE reverses_entry_id = $1)`, origID).Scan(&already); err != nil {
		return 0, err
	}
	if already {
		return 0, fmt.Errorf("posting: journal entry %d: %w", origID, ErrAlreadyReversed)
	}

	// A line matched on a bank statement pins the entry: reversing it would
	// leave the reconciliation pointing at history the books have disowned.
	// The match must be released (or its statement reopened) first.
	var matched bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM bank_statement_lines b
		               JOIN journal_lines jl ON jl.id = b.journal_line_id
		               WHERE jl.journal_entry_id = $1)`, origID).Scan(&matched); err != nil {
		return 0, err
	}
	if matched {
		return 0, fmt.Errorf("posting: journal entry %d: %w", origID, ErrBankMatched)
	}

	// Reverse into the open period containing the original date; if that period
	// is now closed, periodForDate reports no open period.
	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}

	// The reversal inherits is_closing so that reversing a year-end closing
	// entry stays invisible to income statements, like the entry it undoes,
	// and the original's exchange rate — it must undo the exact base amounts
	// that were posted, not restate them at today's rate.
	var rev int
	if err := tx.QueryRow(ctx,
		`INSERT INTO journal_entries (entry_date, period_id, currency_code, exchange_rate, memo, reverses_entry_id, status, posted_at, is_closing)
		 SELECT $1::date, $2, $3, exchange_rate, $4, $5, 'posted', now(), is_closing
		 FROM journal_entries WHERE id = $5
		 RETURNING id`,
		date, period, currency, fmt.Sprintf("Reversal of journal entry %d", origID), origID).Scan(&rev); err != nil {
		return 0, err
	}

	// Mirror the lines with debit and credit swapped, in both currencies.
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo, base_debit, base_credit)
		 SELECT $1, line_no, account_id, credit, debit, memo, base_credit, base_debit
		 FROM journal_lines WHERE journal_entry_id = $2`, rev, origID); err != nil {
		return 0, err
	}
	return rev, nil
}

// reverseApplicationFX reverses the realized-FX entries linked to a settling
// document's applications, ahead of those applications being deleted. query
// selects the fx_journal_entry_id values to reverse.
func reverseApplicationFX(ctx context.Context, tx pgx.Tx, query string, id int) error {
	rows, err := tx.Query(ctx, query, id)
	if err != nil {
		return err
	}
	var entries []int
	for rows.Next() {
		var e int
		if err := rows.Scan(&e); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := reverseEntry(ctx, tx, e); err != nil {
			return err
		}
	}
	return nil
}

// UnpostSalesInvoice reverses a posted sales invoice's journal entry and returns
// the invoice to draft. It refuses if any payment or credit note has been
// applied to the invoice (those applications must be unwound first).
func UnpostSalesInvoice(ctx context.Context, tx pgx.Tx, invoiceID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM sales_invoices WHERE id = $1`, invoiceID, "sales invoice")
	if err != nil {
		return 0, err
	}
	var hasApps bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM payment_applications WHERE invoice_id = $1)
		     OR EXISTS(SELECT 1 FROM sales_credit_applications WHERE invoice_id = $1)`, invoiceID).Scan(&hasApps); err != nil {
		return 0, err
	}
	if hasApps {
		return 0, fmt.Errorf("posting: sales invoice %d: %w", invoiceID, ErrHasApplications)
	}
	rev, err := reverseEntry(ctx, tx, je)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE sales_invoices SET status = 'draft', journal_entry_id = NULL, period_id = NULL WHERE id = $1`,
		invoiceID); err != nil {
		return 0, err
	}
	return rev, nil
}

// UnpostPurchaseBill reverses a posted bill's journal entry and returns it to
// draft, refusing if any payment or credit note has been applied to it.
func UnpostPurchaseBill(ctx context.Context, tx pgx.Tx, billID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM purchase_bills WHERE id = $1`, billID, "purchase bill")
	if err != nil {
		return 0, err
	}
	var hasApps bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM bill_applications WHERE bill_id = $1)
		     OR EXISTS(SELECT 1 FROM purchase_credit_applications WHERE bill_id = $1)`, billID).Scan(&hasApps); err != nil {
		return 0, err
	}
	if hasApps {
		return 0, fmt.Errorf("posting: purchase bill %d: %w", billID, ErrHasApplications)
	}
	rev, err := reverseEntry(ctx, tx, je)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE purchase_bills SET status = 'draft', journal_entry_id = NULL, period_id = NULL WHERE id = $1`,
		billID); err != nil {
		return 0, err
	}
	return rev, nil
}

// UnpostCustomerPayment reverses a posted customer payment, deletes its
// applications (reversing any realized-FX entries they carried), and returns
// it to draft.
func UnpostCustomerPayment(ctx context.Context, tx pgx.Tx, paymentID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM customer_payments WHERE id = $1`, paymentID, "customer payment")
	if err != nil {
		return 0, err
	}
	rev, err := reverseEntry(ctx, tx, je)
	if err != nil {
		return 0, err
	}
	if err := reverseApplicationFX(ctx, tx,
		`SELECT fx_journal_entry_id FROM payment_applications
		 WHERE payment_id = $1 AND fx_journal_entry_id IS NOT NULL`, paymentID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM payment_applications WHERE payment_id = $1`, paymentID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE customer_payments SET status = 'draft', journal_entry_id = NULL, period_id = NULL WHERE id = $1`,
		paymentID); err != nil {
		return 0, err
	}
	return rev, nil
}

// UnpostSupplierPayment reverses a posted supplier payment, deletes its
// applications (reversing any realized-FX entries they carried), and returns
// it to draft.
func UnpostSupplierPayment(ctx context.Context, tx pgx.Tx, paymentID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM supplier_payments WHERE id = $1`, paymentID, "supplier payment")
	if err != nil {
		return 0, err
	}
	rev, err := reverseEntry(ctx, tx, je)
	if err != nil {
		return 0, err
	}
	if err := reverseApplicationFX(ctx, tx,
		`SELECT fx_journal_entry_id FROM bill_applications
		 WHERE payment_id = $1 AND fx_journal_entry_id IS NOT NULL`, paymentID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bill_applications WHERE payment_id = $1`, paymentID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE supplier_payments SET status = 'draft', journal_entry_id = NULL, period_id = NULL WHERE id = $1`,
		paymentID); err != nil {
		return 0, err
	}
	return rev, nil
}

// UnpostStockMovement reverses a posted stock movement's journal entry and
// unlinks it, so the movement can be re-posted. The movement itself (the
// quantity record) is left intact.
func UnpostStockMovement(ctx context.Context, tx pgx.Tx, movementID int) (int, error) {
	var je *int
	err := tx.QueryRow(ctx, `SELECT journal_entry_id FROM stock_movements WHERE id = $1`, movementID).Scan(&je)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: stock movement %d: %w", movementID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if je == nil {
		return 0, fmt.Errorf("posting: stock movement %d: %w", movementID, ErrNotPosted)
	}
	rev, err := reverseEntry(ctx, tx, *je)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE stock_movements SET journal_entry_id = NULL, period_id = NULL WHERE id = $1`, movementID); err != nil {
		return 0, err
	}
	return rev, nil
}

// postedEntryOf reads a document's (status, journal_entry_id) via the given
// query and returns the journal entry id, requiring the document to be posted.
func postedEntryOf(ctx context.Context, tx pgx.Tx, query string, id int, kind string) (int, error) {
	var status string
	var je *int
	err := tx.QueryRow(ctx, query, id).Scan(&status, &je)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: %s %d: %w", kind, id, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if status != "posted" || je == nil {
		return 0, fmt.Errorf("posting: %s %d: %w", kind, id, ErrNotPosted)
	}
	return *je, nil
}
