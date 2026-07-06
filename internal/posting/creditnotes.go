package posting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Credit notes post as the mirror image of the document they offset:
//
//	sales credit note     Dr revenue (net) + Dr tax   Cr A/R (gross)
//	purchase credit note  Dr A/P (gross)              Cr expense (net) + Cr tax
//
// Applying a credit note to an invoice/bill is a pure subledger allocation,
// exactly like a payment application: the GL already carries both documents,
// so applications create no journal entries.

// PostSalesCreditNote posts a draft sales credit note to the GL:
// Dr revenue per account, Dr sales tax per account, Cr A/R (gross).
func PostSalesCreditNote(ctx context.Context, tx pgx.Tx, noteID int) (int, error) {
	var (
		status, currency, date, number string
		arAccount                      *int
		hasTotal                       bool
	)
	err := tx.QueryRow(ctx,
		`SELECT cn.status, cn.currency_code, cn.credit_note_date::text, cn.credit_note_number,
		        c.ar_account_id, (cn.total > 0)
		 FROM sales_credit_notes cn JOIN customers c ON c.id = cn.customer_id
		 WHERE cn.id = $1`, noteID).Scan(&status, &currency, &date, &number, &arAccount, &hasTotal)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: sales credit note %d: %w", noteID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if status != "draft" {
		return 0, fmt.Errorf("posting: sales credit note %d: %w", noteID, ErrNotDraft)
	}
	if !hasTotal {
		return 0, fmt.Errorf("posting: sales credit note %d: %w", noteID, ErrNothingToPost)
	}
	if arAccount == nil {
		return 0, fmt.Errorf("posting: sales credit note %d: customer A/R account: %w", noteID, ErrMissingAccount)
	}

	var bad int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM sales_credit_note_lines l LEFT JOIN products p ON p.id = l.product_id
		 WHERE l.credit_note_id = $1 AND l.line_subtotal <> 0
		   AND COALESCE(l.revenue_account_id, p.revenue_account_id) IS NULL`, noteID).Scan(&bad); err != nil {
		return 0, err
	}
	if bad > 0 {
		return 0, fmt.Errorf("posting: sales credit note %d: %d line(s) missing a revenue account: %w", noteID, bad, ErrMissingAccount)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM sales_credit_note_lines l LEFT JOIN tax_codes tc ON tc.code = l.tax_code
		 WHERE l.credit_note_id = $1 AND l.tax_amount <> 0 AND tc.tax_account_id IS NULL`, noteID).Scan(&bad); err != nil {
		return 0, err
	}
	if bad > 0 {
		return 0, fmt.Errorf("posting: sales credit note %d: taxed line(s) missing a tax account: %w", noteID, ErrMissingAccount)
	}

	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}
	je, err := createEntry(ctx, tx, date, period, currency, "Sales credit note "+number, number)
	if err != nil {
		return 0, err
	}

	// Dr revenue per account, then Dr sales tax per account.
	if _, err := tx.Exec(ctx,
		`WITH rev AS (
		     SELECT COALESCE(l.revenue_account_id, p.revenue_account_id) AS account_id,
		            sum(l.line_subtotal) AS amount
		     FROM sales_credit_note_lines l LEFT JOIN products p ON p.id = l.product_id
		     WHERE l.credit_note_id = $2 GROUP BY 1 HAVING sum(l.line_subtotal) <> 0
		 ), tax AS (
		     SELECT tc.tax_account_id AS account_id, sum(l.tax_amount) AS amount
		     FROM sales_credit_note_lines l JOIN tax_codes tc ON tc.code = l.tax_code
		     WHERE l.credit_note_id = $2 AND l.tax_amount <> 0 GROUP BY 1 HAVING sum(l.tax_amount) <> 0
		 ), debits AS (
		     SELECT 0 AS ord, account_id, amount, 'Revenue credited'::text AS memo FROM rev
		     UNION ALL
		     SELECT 1 AS ord, account_id, amount, 'Sales tax credited'::text FROM tax
		 )
		 INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, row_number() OVER (ORDER BY ord, account_id), account_id, amount, 0, memo
		 FROM debits`, je, noteID); err != nil {
		return 0, err
	}
	// Cr accounts receivable for the gross total (numbered after the debits).
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1,
		        (SELECT COALESCE(max(line_no), 0) FROM journal_lines WHERE journal_entry_id = $1) + 1,
		        c.ar_account_id, 0, cn.total, 'Accounts receivable'
		 FROM sales_credit_notes cn JOIN customers c ON c.id = cn.customer_id
		 WHERE cn.id = $2`, je, noteID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE sales_credit_notes SET status = 'posted', journal_entry_id = $1, period_id = $2 WHERE id = $3`,
		je, period, noteID); err != nil {
		return 0, err
	}
	return je, nil
}

// PostPurchaseCreditNote posts a draft purchase credit note to the GL:
// Dr A/P (gross), Cr expense/inventory per account, Cr input tax per account.
func PostPurchaseCreditNote(ctx context.Context, tx pgx.Tx, noteID int) (int, error) {
	var (
		status, currency, date, number string
		apAccount                      *int
		hasTotal                       bool
	)
	err := tx.QueryRow(ctx,
		`SELECT cn.status, cn.currency_code, cn.credit_note_date::text, cn.credit_note_number,
		        s.ap_account_id, (cn.total > 0)
		 FROM purchase_credit_notes cn JOIN suppliers s ON s.id = cn.supplier_id
		 WHERE cn.id = $1`, noteID).Scan(&status, &currency, &date, &number, &apAccount, &hasTotal)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: purchase credit note %d: %w", noteID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if status != "draft" {
		return 0, fmt.Errorf("posting: purchase credit note %d: %w", noteID, ErrNotDraft)
	}
	if !hasTotal {
		return 0, fmt.Errorf("posting: purchase credit note %d: %w", noteID, ErrNothingToPost)
	}
	if apAccount == nil {
		return 0, fmt.Errorf("posting: purchase credit note %d: supplier A/P account: %w", noteID, ErrMissingAccount)
	}

	var bad int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM purchase_credit_note_lines l LEFT JOIN products p ON p.id = l.product_id
		 WHERE l.credit_note_id = $1 AND l.line_subtotal <> 0
		   AND COALESCE(l.expense_account_id, p.inventory_account_id) IS NULL`, noteID).Scan(&bad); err != nil {
		return 0, err
	}
	if bad > 0 {
		return 0, fmt.Errorf("posting: purchase credit note %d: %d line(s) missing an expense/inventory account: %w", noteID, bad, ErrMissingAccount)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM purchase_credit_note_lines l LEFT JOIN tax_codes tc ON tc.code = l.tax_code
		 WHERE l.credit_note_id = $1 AND l.tax_amount <> 0 AND tc.tax_account_id IS NULL`, noteID).Scan(&bad); err != nil {
		return 0, err
	}
	if bad > 0 {
		return 0, fmt.Errorf("posting: purchase credit note %d: taxed line(s) missing a tax account: %w", noteID, ErrMissingAccount)
	}

	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}
	je, err := createEntry(ctx, tx, date, period, currency, "Purchase credit note "+number, number)
	if err != nil {
		return 0, err
	}

	// Dr accounts payable for the gross total.
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 1, s.ap_account_id, cn.total, 0, 'Accounts payable'
		 FROM purchase_credit_notes cn JOIN suppliers s ON s.id = cn.supplier_id
		 WHERE cn.id = $2`, je, noteID); err != nil {
		return 0, err
	}
	// Cr expense/inventory per account, then Cr input tax per account.
	if _, err := tx.Exec(ctx,
		`WITH exp AS (
		     SELECT COALESCE(l.expense_account_id, p.inventory_account_id) AS account_id,
		            sum(l.line_subtotal) AS amount
		     FROM purchase_credit_note_lines l LEFT JOIN products p ON p.id = l.product_id
		     WHERE l.credit_note_id = $2 GROUP BY 1 HAVING sum(l.line_subtotal) <> 0
		 ), tax AS (
		     SELECT tc.tax_account_id AS account_id, sum(l.tax_amount) AS amount
		     FROM purchase_credit_note_lines l JOIN tax_codes tc ON tc.code = l.tax_code
		     WHERE l.credit_note_id = $2 AND l.tax_amount <> 0 GROUP BY 1 HAVING sum(l.tax_amount) <> 0
		 ), credits AS (
		     SELECT 0 AS ord, account_id, amount, 'Expense credited'::text AS memo FROM exp
		     UNION ALL
		     SELECT 1 AS ord, account_id, amount, 'Input tax credited'::text FROM tax
		 )
		 INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 1 + row_number() OVER (ORDER BY ord, account_id), account_id, 0, amount, memo
		 FROM credits`, je, noteID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE purchase_credit_notes SET status = 'posted', journal_entry_id = $1, period_id = $2 WHERE id = $3`,
		je, period, noteID); err != nil {
		return 0, err
	}
	return je, nil
}

// AutoApplySalesCreditNote allocates a sales credit note's unapplied remainder
// across the customer's open invoices (posted, same currency, with an
// outstanding balance), oldest first — the credit-note analog of
// AutoApplyCustomerPayment. Invoice availability counts payments and credit
// notes combined, via the invoice_amount_settled helper (000012).
func AutoApplySalesCreditNote(ctx context.Context, tx pgx.Tx, noteID int) ([]Application, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM sales_credit_notes WHERE id = $1`, noteID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("posting: sales credit note %d: %w", noteID, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if status == "void" {
		return nil, fmt.Errorf("posting: sales credit note %d is void: %w", noteID, ErrNotPostable)
	}

	rows, err := tx.Query(ctx, `
WITH n AS (
    SELECT cn.id AS credit_note_id, cn.customer_id, cn.currency_code,
           cn.total - COALESCE((SELECT sum(amount_applied) FROM sales_credit_applications
                                WHERE credit_note_id = cn.id), 0) AS remaining
    FROM sales_credit_notes cn WHERE cn.id = $1
),
open_inv AS (
    SELECT si.id AS invoice_id, si.invoice_date,
           si.total - invoice_amount_settled(si.id) AS available
    FROM sales_invoices si JOIN n ON si.customer_id = n.customer_id
                                 AND si.currency_code = n.currency_code
    WHERE si.status = 'posted'
),
ranked AS (
    SELECT invoice_id, available,
           COALESCE(sum(available) OVER (ORDER BY invoice_date, invoice_id
                                         ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING), 0) AS prior
    FROM open_inv WHERE available > 0
),
alloc AS (
    SELECT invoice_id,
           LEAST(available, GREATEST((SELECT remaining FROM n) - prior, 0)) AS amount
    FROM ranked
)
INSERT INTO sales_credit_applications (credit_note_id, invoice_id, amount_applied)
SELECT (SELECT credit_note_id FROM n), invoice_id, amount
FROM alloc WHERE amount > 0
RETURNING invoice_id, amount_applied::text`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.DocumentID, &a.Amount); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

// AutoApplyPurchaseCreditNote is the purchasing-side mirror of
// AutoApplySalesCreditNote: it allocates a purchase credit note across the
// supplier's open bills, oldest first.
func AutoApplyPurchaseCreditNote(ctx context.Context, tx pgx.Tx, noteID int) ([]Application, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM purchase_credit_notes WHERE id = $1`, noteID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("posting: purchase credit note %d: %w", noteID, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	if status == "void" {
		return nil, fmt.Errorf("posting: purchase credit note %d is void: %w", noteID, ErrNotPostable)
	}

	rows, err := tx.Query(ctx, `
WITH n AS (
    SELECT cn.id AS credit_note_id, cn.supplier_id, cn.currency_code,
           cn.total - COALESCE((SELECT sum(amount_applied) FROM purchase_credit_applications
                                WHERE credit_note_id = cn.id), 0) AS remaining
    FROM purchase_credit_notes cn WHERE cn.id = $1
),
open_bill AS (
    SELECT pb.id AS bill_id, pb.bill_date,
           pb.total - bill_amount_settled(pb.id) AS available
    FROM purchase_bills pb JOIN n ON pb.supplier_id = n.supplier_id
                                 AND pb.currency_code = n.currency_code
    WHERE pb.status = 'posted'
),
ranked AS (
    SELECT bill_id, available,
           COALESCE(sum(available) OVER (ORDER BY bill_date, bill_id
                                         ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING), 0) AS prior
    FROM open_bill WHERE available > 0
),
alloc AS (
    SELECT bill_id,
           LEAST(available, GREATEST((SELECT remaining FROM n) - prior, 0)) AS amount
    FROM ranked
)
INSERT INTO purchase_credit_applications (credit_note_id, bill_id, amount_applied)
SELECT (SELECT credit_note_id FROM n), bill_id, amount
FROM alloc WHERE amount > 0
RETURNING bill_id, amount_applied::text`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.DocumentID, &a.Amount); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

// UnpostSalesCreditNote reverses a posted sales credit note's journal entry
// and returns the note to draft. It refuses if the note has been applied to
// any invoice (those applications must be unwound first).
func UnpostSalesCreditNote(ctx context.Context, tx pgx.Tx, noteID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM sales_credit_notes WHERE id = $1`, noteID, "sales credit note")
	if err != nil {
		return 0, err
	}
	var hasApps bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM sales_credit_applications WHERE credit_note_id = $1)`, noteID).Scan(&hasApps); err != nil {
		return 0, err
	}
	if hasApps {
		return 0, fmt.Errorf("posting: sales credit note %d: %w", noteID, ErrHasApplications)
	}
	rev, err := reverseEntry(ctx, tx, je)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE sales_credit_notes SET status = 'draft', journal_entry_id = NULL, period_id = NULL WHERE id = $1`,
		noteID); err != nil {
		return 0, err
	}
	return rev, nil
}

// UnpostPurchaseCreditNote reverses a posted purchase credit note's journal
// entry and returns it to draft, refusing if it has been applied to any bill.
func UnpostPurchaseCreditNote(ctx context.Context, tx pgx.Tx, noteID int) (int, error) {
	je, err := postedEntryOf(ctx, tx, `SELECT status, journal_entry_id FROM purchase_credit_notes WHERE id = $1`, noteID, "purchase credit note")
	if err != nil {
		return 0, err
	}
	var hasApps bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM purchase_credit_applications WHERE credit_note_id = $1)`, noteID).Scan(&hasApps); err != nil {
		return 0, err
	}
	if hasApps {
		return 0, fmt.Errorf("posting: purchase credit note %d: %w", noteID, ErrHasApplications)
	}
	rev, err := reverseEntry(ctx, tx, je)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE purchase_credit_notes SET status = 'draft', journal_entry_id = NULL, period_id = NULL WHERE id = $1`,
		noteID); err != nil {
		return 0, err
	}
	return rev, nil
}
