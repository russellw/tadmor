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

	// Reverse into the open period containing the original date; if that period
	// is now closed, periodForDate reports no open period.
	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}

	// The reversal inherits is_closing so that reversing a year-end closing
	// entry stays invisible to income statements, like the entry it undoes.
	var rev int
	if err := tx.QueryRow(ctx,
		`INSERT INTO journal_entries (entry_date, period_id, currency_code, memo, reverses_entry_id, status, posted_at, is_closing)
		 SELECT $1::date, $2, $3, $4, $5, 'posted', now(), is_closing
		 FROM journal_entries WHERE id = $5
		 RETURNING id`,
		date, period, currency, fmt.Sprintf("Reversal of journal entry %d", origID), origID).Scan(&rev); err != nil {
		return 0, err
	}

	// Mirror the lines with debit and credit swapped.
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, line_no, account_id, credit, debit, memo
		 FROM journal_lines WHERE journal_entry_id = $2`, rev, origID); err != nil {
		return 0, err
	}
	return rev, nil
}

// UnpostSalesInvoice reverses a posted sales invoice's journal entry and returns
// the invoice to draft. It refuses if any payment has been applied to the
// invoice (those applications must be unwound first).
func UnpostSalesInvoice(ctx context.Context, tx pgx.Tx, invoiceID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM sales_invoices WHERE id = $1`, invoiceID, "sales invoice")
	if err != nil {
		return 0, err
	}
	var hasApps bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM payment_applications WHERE invoice_id = $1)`, invoiceID).Scan(&hasApps); err != nil {
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
// draft, refusing if any payment has been applied to it.
func UnpostPurchaseBill(ctx context.Context, tx pgx.Tx, billID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM purchase_bills WHERE id = $1`, billID, "purchase bill")
	if err != nil {
		return 0, err
	}
	var hasApps bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM bill_applications WHERE bill_id = $1)`, billID).Scan(&hasApps); err != nil {
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
// applications, and returns it to draft.
func UnpostCustomerPayment(ctx context.Context, tx pgx.Tx, paymentID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM customer_payments WHERE id = $1`, paymentID, "customer payment")
	if err != nil {
		return 0, err
	}
	rev, err := reverseEntry(ctx, tx, je)
	if err != nil {
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
// applications, and returns it to draft.
func UnpostSupplierPayment(ctx context.Context, tx pgx.Tx, paymentID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM supplier_payments WHERE id = $1`, paymentID, "supplier payment")
	if err != nil {
		return 0, err
	}
	rev, err := reverseEntry(ctx, tx, je)
	if err != nil {
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
