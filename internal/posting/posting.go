// Package posting turns subledger documents (invoices, bills, payments, stock
// movements) into balanced general-ledger journal entries.
//
// Every function runs against a pgx.Tx supplied by the caller: posting must be
// atomic, and the GL's balance constraint is DEFERRED, so it is verified when
// the caller commits. All monetary arithmetic happens in SQL against numeric
// columns — money never passes through Go floating point.
//
// The mapping of each document to debits/credits:
//
//	sales invoice      Dr A/R (gross)        Cr revenue (net) + Cr tax
//	purchase bill      Dr expense + Dr tax   Cr A/P (gross)
//	customer payment   Dr cash               Cr A/R
//	supplier payment   Dr A/P                Cr cash
//	inventory issue    Dr COGS               Cr inventory
package posting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Errors returned for documents that cannot be posted. Callers may test these
// with errors.Is.
var (
	ErrNotFound        = errors.New("document not found")
	ErrNotDraft        = errors.New("document is not in draft status")
	ErrNotPosted       = errors.New("document is not posted")
	ErrAlreadyPosted   = errors.New("document is already posted")
	ErrAlreadyReversed = errors.New("journal entry is already reversed")
	ErrHasApplications = errors.New("document has payment applications that must be unwound first")
	ErrNotPostable     = errors.New("document is not eligible for this posting")
	ErrNoOpenPeriod    = errors.New("no open accounting period for the document date")
	ErrMissingAccount  = errors.New("a required GL account is not configured")
	ErrNothingToPost   = errors.New("document total is zero or negative")
	ErrYearNotOpen     = errors.New("fiscal year is not open")
	ErrYearNotClosed   = errors.New("fiscal year is not closed")
	ErrPriorYearOpen   = errors.New("an earlier fiscal year is still open")
	ErrLaterYearClosed = errors.New("a later fiscal year has already been closed")
	ErrBankMatched     = errors.New("journal entry has lines matched to a bank statement")
)

// periodForDate returns the id of the open accounting period containing date.
//
// If no period covers the date at all but an open fiscal year does, the
// calendar month containing the date is created as a new open period (clipped
// to the fiscal year's bounds), so posting keeps working across a month
// rollover without manual period creation. A closed period covering the date,
// or a date outside every open fiscal year, still fails with ErrNoOpenPeriod.
func periodForDate(ctx context.Context, tx pgx.Tx, date string) (int, error) {
	const selectOpen = `SELECT id FROM accounting_periods
		 WHERE $1::date BETWEEN start_date AND end_date AND status = 'open'
		 ORDER BY id LIMIT 1`
	var id int
	err := tx.QueryRow(ctx, selectOpen, date).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	// A closed period covering the date means the books for that date are
	// deliberately shut — never create a sibling period around it.
	var covered bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM accounting_periods WHERE $1::date BETWEEN start_date AND end_date)`,
		date).Scan(&covered); err != nil {
		return 0, err
	}
	if covered {
		return 0, ErrNoOpenPeriod
	}

	// ON CONFLICT DO NOTHING absorbs both a concurrent auto-create of the
	// same month and an existing non-month-aligned period the month would
	// overlap; the re-select decides the outcome either way.
	if _, err := tx.Exec(ctx,
		`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
		 SELECT fy.id, to_char($1::date, 'YYYY-MM'),
		        GREATEST(date_trunc('month', $1::date)::date, fy.start_date),
		        LEAST((date_trunc('month', $1::date) + interval '1 month - 1 day')::date, fy.end_date)
		 FROM fiscal_years fy
		 WHERE $1::date BETWEEN fy.start_date AND fy.end_date AND fy.status = 'open'
		 ORDER BY fy.start_date LIMIT 1
		 ON CONFLICT DO NOTHING`, date); err != nil {
		return 0, err
	}
	err = tx.QueryRow(ctx, selectOpen, date).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoOpenPeriod
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// createEntry inserts a posted journal-entry header and returns its id.
func createEntry(ctx context.Context, tx pgx.Tx, date string, periodID int, currency, memo, reference string) (int, error) {
	var id int
	err := tx.QueryRow(ctx,
		`INSERT INTO journal_entries (entry_date, period_id, currency_code, memo, reference, status, posted_at)
		 VALUES ($1::date, $2, $3, $4, $5, 'posted', now())
		 RETURNING id`, date, periodID, currency, memo, reference).Scan(&id)
	return id, err
}

// PostSalesInvoice posts a draft sales invoice to the GL:
// Dr A/R (gross), Cr revenue per account, Cr sales tax per account.
func PostSalesInvoice(ctx context.Context, tx pgx.Tx, invoiceID int) (int, error) {
	var (
		status, currency, date, number string
		arAccount                      *int
		hasTotal                       bool
	)
	err := tx.QueryRow(ctx,
		`SELECT si.status, si.currency_code, si.invoice_date::text, si.invoice_number,
		        c.ar_account_id, (si.total > 0)
		 FROM sales_invoices si JOIN customers c ON c.id = si.customer_id
		 WHERE si.id = $1`, invoiceID).Scan(&status, &currency, &date, &number, &arAccount, &hasTotal)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: sales invoice %d: %w", invoiceID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if status != "draft" {
		return 0, fmt.Errorf("posting: sales invoice %d: %w", invoiceID, ErrNotDraft)
	}
	if !hasTotal {
		return 0, fmt.Errorf("posting: sales invoice %d: %w", invoiceID, ErrNothingToPost)
	}
	if arAccount == nil {
		return 0, fmt.Errorf("posting: sales invoice %d: customer A/R account: %w", invoiceID, ErrMissingAccount)
	}

	var bad int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM sales_invoice_lines l LEFT JOIN products p ON p.id = l.product_id
		 WHERE l.invoice_id = $1 AND l.line_subtotal <> 0
		   AND COALESCE(l.revenue_account_id, p.revenue_account_id) IS NULL`, invoiceID).Scan(&bad); err != nil {
		return 0, err
	}
	if bad > 0 {
		return 0, fmt.Errorf("posting: sales invoice %d: %d line(s) missing a revenue account: %w", invoiceID, bad, ErrMissingAccount)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM sales_invoice_lines l LEFT JOIN tax_codes tc ON tc.code = l.tax_code
		 WHERE l.invoice_id = $1 AND l.tax_amount <> 0 AND tc.tax_account_id IS NULL`, invoiceID).Scan(&bad); err != nil {
		return 0, err
	}
	if bad > 0 {
		return 0, fmt.Errorf("posting: sales invoice %d: taxed line(s) missing a tax account: %w", invoiceID, ErrMissingAccount)
	}

	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}
	je, err := createEntry(ctx, tx, date, period, currency, "Sales invoice "+number, number)
	if err != nil {
		return 0, err
	}

	// Dr accounts receivable for the gross total.
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 1, c.ar_account_id, si.total, 0, 'Accounts receivable'
		 FROM sales_invoices si JOIN customers c ON c.id = si.customer_id
		 WHERE si.id = $2`, je, invoiceID); err != nil {
		return 0, err
	}
	// Cr revenue per account, then Cr sales tax per account.
	if _, err := tx.Exec(ctx,
		`WITH rev AS (
		     SELECT COALESCE(l.revenue_account_id, p.revenue_account_id) AS account_id,
		            sum(l.line_subtotal) AS amount
		     FROM sales_invoice_lines l LEFT JOIN products p ON p.id = l.product_id
		     WHERE l.invoice_id = $2 GROUP BY 1 HAVING sum(l.line_subtotal) <> 0
		 ), tax AS (
		     SELECT tc.tax_account_id AS account_id, sum(l.tax_amount) AS amount
		     FROM sales_invoice_lines l JOIN tax_codes tc ON tc.code = l.tax_code
		     WHERE l.invoice_id = $2 AND l.tax_amount <> 0 GROUP BY 1 HAVING sum(l.tax_amount) <> 0
		 ), credits AS (
		     SELECT 0 AS ord, account_id, amount, 'Revenue'::text AS memo FROM rev
		     UNION ALL
		     SELECT 1 AS ord, account_id, amount, 'Sales tax'::text FROM tax
		 )
		 INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 1 + row_number() OVER (ORDER BY ord, account_id), account_id, 0, amount, memo
		 FROM credits`, je, invoiceID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE sales_invoices SET status = 'posted', journal_entry_id = $1, period_id = $2 WHERE id = $3`,
		je, period, invoiceID); err != nil {
		return 0, err
	}
	return je, nil
}

// PostPurchaseBill posts a draft vendor bill to the GL:
// Dr expense/inventory per account, Dr input tax, Cr A/P (gross).
func PostPurchaseBill(ctx context.Context, tx pgx.Tx, billID int) (int, error) {
	var (
		status, currency, date, number string
		apAccount                      *int
		hasTotal                       bool
	)
	err := tx.QueryRow(ctx,
		`SELECT pb.status, pb.currency_code, pb.bill_date::text, pb.bill_number,
		        s.ap_account_id, (pb.total > 0)
		 FROM purchase_bills pb JOIN suppliers s ON s.id = pb.supplier_id
		 WHERE pb.id = $1`, billID).Scan(&status, &currency, &date, &number, &apAccount, &hasTotal)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: purchase bill %d: %w", billID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if status != "draft" {
		return 0, fmt.Errorf("posting: purchase bill %d: %w", billID, ErrNotDraft)
	}
	if !hasTotal {
		return 0, fmt.Errorf("posting: purchase bill %d: %w", billID, ErrNothingToPost)
	}
	if apAccount == nil {
		return 0, fmt.Errorf("posting: purchase bill %d: supplier A/P account: %w", billID, ErrMissingAccount)
	}

	var bad int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM purchase_bill_lines l LEFT JOIN products p ON p.id = l.product_id
		 WHERE l.bill_id = $1 AND l.line_subtotal <> 0
		   AND COALESCE(l.expense_account_id, p.inventory_account_id) IS NULL`, billID).Scan(&bad); err != nil {
		return 0, err
	}
	if bad > 0 {
		return 0, fmt.Errorf("posting: purchase bill %d: %d line(s) missing an expense/inventory account: %w", billID, bad, ErrMissingAccount)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM purchase_bill_lines l LEFT JOIN tax_codes tc ON tc.code = l.tax_code
		 WHERE l.bill_id = $1 AND l.tax_amount <> 0 AND tc.tax_account_id IS NULL`, billID).Scan(&bad); err != nil {
		return 0, err
	}
	if bad > 0 {
		return 0, fmt.Errorf("posting: purchase bill %d: taxed line(s) missing a tax account: %w", billID, ErrMissingAccount)
	}

	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}
	je, err := createEntry(ctx, tx, date, period, currency, "Purchase bill "+number, number)
	if err != nil {
		return 0, err
	}

	// Dr expense/inventory per account, then Dr input tax per account.
	if _, err := tx.Exec(ctx,
		`WITH exp AS (
		     SELECT COALESCE(l.expense_account_id, p.inventory_account_id) AS account_id,
		            sum(l.line_subtotal) AS amount
		     FROM purchase_bill_lines l LEFT JOIN products p ON p.id = l.product_id
		     WHERE l.bill_id = $2 GROUP BY 1 HAVING sum(l.line_subtotal) <> 0
		 ), tax AS (
		     SELECT tc.tax_account_id AS account_id, sum(l.tax_amount) AS amount
		     FROM purchase_bill_lines l JOIN tax_codes tc ON tc.code = l.tax_code
		     WHERE l.bill_id = $2 AND l.tax_amount <> 0 GROUP BY 1 HAVING sum(l.tax_amount) <> 0
		 ), debits AS (
		     SELECT 0 AS ord, account_id, amount, 'Expense'::text AS memo FROM exp
		     UNION ALL
		     SELECT 1 AS ord, account_id, amount, 'Input tax'::text FROM tax
		 )
		 INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, row_number() OVER (ORDER BY ord, account_id), account_id, amount, 0, memo
		 FROM debits`, je, billID); err != nil {
		return 0, err
	}
	// Cr accounts payable for the gross total (numbered after the debit lines).
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1,
		        (SELECT COALESCE(max(line_no), 0) FROM journal_lines WHERE journal_entry_id = $1) + 1,
		        s.ap_account_id, 0, pb.total, 'Accounts payable'
		 FROM purchase_bills pb JOIN suppliers s ON s.id = pb.supplier_id
		 WHERE pb.id = $2`, je, billID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE purchase_bills SET status = 'posted', journal_entry_id = $1, period_id = $2 WHERE id = $3`,
		je, period, billID); err != nil {
		return 0, err
	}
	return je, nil
}

// PostCustomerPayment posts a draft customer receipt: Dr cash, Cr A/R.
func PostCustomerPayment(ctx context.Context, tx pgx.Tx, paymentID int) (int, error) {
	var (
		status, currency, date    string
		arAccount, depositAccount *int
		hasAmount                 bool
	)
	err := tx.QueryRow(ctx,
		`SELECT cp.status, cp.currency_code, cp.payment_date::text,
		        c.ar_account_id, cp.deposit_account_id, (cp.amount > 0)
		 FROM customer_payments cp JOIN customers c ON c.id = cp.customer_id
		 WHERE cp.id = $1`, paymentID).Scan(&status, &currency, &date, &arAccount, &depositAccount, &hasAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: customer payment %d: %w", paymentID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if status != "draft" {
		return 0, fmt.Errorf("posting: customer payment %d: %w", paymentID, ErrNotDraft)
	}
	if !hasAmount {
		return 0, fmt.Errorf("posting: customer payment %d: %w", paymentID, ErrNothingToPost)
	}
	if depositAccount == nil {
		return 0, fmt.Errorf("posting: customer payment %d: deposit account: %w", paymentID, ErrMissingAccount)
	}
	if arAccount == nil {
		return 0, fmt.Errorf("posting: customer payment %d: customer A/R account: %w", paymentID, ErrMissingAccount)
	}

	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}
	je, err := createEntry(ctx, tx, date, period, currency, "Customer payment", "")
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 1, cp.deposit_account_id, cp.amount, 0, 'Cash received'
		 FROM customer_payments cp WHERE cp.id = $2`, je, paymentID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 2, c.ar_account_id, 0, cp.amount, 'Accounts receivable'
		 FROM customer_payments cp JOIN customers c ON c.id = cp.customer_id
		 WHERE cp.id = $2`, je, paymentID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE customer_payments SET status = 'posted', journal_entry_id = $1, period_id = $2 WHERE id = $3`,
		je, period, paymentID); err != nil {
		return 0, err
	}
	return je, nil
}

// PostSupplierPayment posts a draft supplier payment: Dr A/P, Cr cash.
func PostSupplierPayment(ctx context.Context, tx pgx.Tx, paymentID int) (int, error) {
	var (
		status, currency, date    string
		apAccount, paymentAccount *int
		hasAmount                 bool
	)
	err := tx.QueryRow(ctx,
		`SELECT sp.status, sp.currency_code, sp.payment_date::text,
		        s.ap_account_id, sp.payment_account_id, (sp.amount > 0)
		 FROM supplier_payments sp JOIN suppliers s ON s.id = sp.supplier_id
		 WHERE sp.id = $1`, paymentID).Scan(&status, &currency, &date, &apAccount, &paymentAccount, &hasAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: supplier payment %d: %w", paymentID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if status != "draft" {
		return 0, fmt.Errorf("posting: supplier payment %d: %w", paymentID, ErrNotDraft)
	}
	if !hasAmount {
		return 0, fmt.Errorf("posting: supplier payment %d: %w", paymentID, ErrNothingToPost)
	}
	if paymentAccount == nil {
		return 0, fmt.Errorf("posting: supplier payment %d: payment account: %w", paymentID, ErrMissingAccount)
	}
	if apAccount == nil {
		return 0, fmt.Errorf("posting: supplier payment %d: supplier A/P account: %w", paymentID, ErrMissingAccount)
	}

	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}
	je, err := createEntry(ctx, tx, date, period, currency, "Supplier payment", "")
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 1, s.ap_account_id, sp.amount, 0, 'Accounts payable'
		 FROM supplier_payments sp JOIN suppliers s ON s.id = sp.supplier_id
		 WHERE sp.id = $2`, je, paymentID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 2, sp.payment_account_id, 0, sp.amount, 'Cash paid'
		 FROM supplier_payments sp WHERE sp.id = $2`, je, paymentID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE supplier_payments SET status = 'posted', journal_entry_id = $1, period_id = $2 WHERE id = $3`,
		je, period, paymentID); err != nil {
		return 0, err
	}
	return je, nil
}

// PostInventoryIssue posts the cost of an inventory issue (a 'issue' stock
// movement): Dr COGS, Cr inventory, valued at the movement's cost. currency is
// the functional currency to record the entry in, since stock movements are
// quantity/cost records without a currency of their own.
func PostInventoryIssue(ctx context.Context, tx pgx.Tx, movementID int, currency string) (int, error) {
	var (
		movementType, date      string
		cogsAccount, invAccount *int
		journalEntryID          *int
		hasCost                 bool
	)
	err := tx.QueryRow(ctx,
		`SELECT sm.movement_type, sm.movement_date::text, sm.journal_entry_id,
		        p.cogs_account_id, p.inventory_account_id, (sm.total_cost <> 0)
		 FROM stock_movements sm JOIN products p ON p.id = sm.product_id
		 WHERE sm.id = $1`, movementID).Scan(&movementType, &date, &journalEntryID, &cogsAccount, &invAccount, &hasCost)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: stock movement %d: %w", movementID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if journalEntryID != nil {
		return 0, fmt.Errorf("posting: stock movement %d: %w", movementID, ErrAlreadyPosted)
	}
	if movementType != "issue" {
		return 0, fmt.Errorf("posting: stock movement %d: only 'issue' movements post COGS (got %q): %w", movementID, movementType, ErrNotPostable)
	}
	if !hasCost {
		return 0, fmt.Errorf("posting: stock movement %d: %w", movementID, ErrNothingToPost)
	}
	if cogsAccount == nil || invAccount == nil {
		return 0, fmt.Errorf("posting: stock movement %d: product COGS/inventory account: %w", movementID, ErrMissingAccount)
	}

	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}
	je, err := createEntry(ctx, tx, date, period, currency, "Inventory issue", "")
	if err != nil {
		return 0, err
	}

	// total_cost is negative for an issue; abs() gives the cost moved.
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 1, p.cogs_account_id, abs(sm.total_cost), 0, 'Cost of goods sold'
		 FROM stock_movements sm JOIN products p ON p.id = sm.product_id
		 WHERE sm.id = $2`, je, movementID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 2, p.inventory_account_id, 0, abs(sm.total_cost), 'Inventory'
		 FROM stock_movements sm JOIN products p ON p.id = sm.product_id
		 WHERE sm.id = $2`, je, movementID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE stock_movements SET journal_entry_id = $1, period_id = $2 WHERE id = $3`,
		je, period, movementID); err != nil {
		return 0, err
	}
	return je, nil
}

// PostInventoryReceipt posts the cost of an inventory receipt (a 'receipt' stock
// movement): Dr inventory, Cr the supplied clearing account. That credit
// account is typically a Goods-Received-Not-Invoiced account which the matching
// purchase bill later debits, so it nets to zero once the goods are invoiced.
// currency is the functional currency to record the entry in.
func PostInventoryReceipt(ctx context.Context, tx pgx.Tx, movementID int, currency string, creditAccountID int) (int, error) {
	var (
		movementType, date string
		invAccount         *int
		journalEntryID     *int
		hasCost            bool
	)
	err := tx.QueryRow(ctx,
		`SELECT sm.movement_type, sm.movement_date::text, sm.journal_entry_id,
		        p.inventory_account_id, (sm.total_cost <> 0)
		 FROM stock_movements sm JOIN products p ON p.id = sm.product_id
		 WHERE sm.id = $1`, movementID).Scan(&movementType, &date, &journalEntryID, &invAccount, &hasCost)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: stock movement %d: %w", movementID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	if journalEntryID != nil {
		return 0, fmt.Errorf("posting: stock movement %d: %w", movementID, ErrAlreadyPosted)
	}
	if movementType != "receipt" {
		return 0, fmt.Errorf("posting: stock movement %d: only 'receipt' movements post an inventory receipt (got %q): %w", movementID, movementType, ErrNotPostable)
	}
	if !hasCost {
		return 0, fmt.Errorf("posting: stock movement %d: %w", movementID, ErrNothingToPost)
	}
	if invAccount == nil {
		return 0, fmt.Errorf("posting: stock movement %d: product inventory account: %w", movementID, ErrMissingAccount)
	}

	// The supplied credit account must exist and be postable.
	var postable bool
	err = tx.QueryRow(ctx, `SELECT is_postable AND is_active FROM accounts WHERE id = $1`, creditAccountID).Scan(&postable)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: credit account %d: %w", creditAccountID, ErrMissingAccount)
	}
	if err != nil {
		return 0, err
	}
	if !postable {
		return 0, fmt.Errorf("posting: credit account %d is not postable/active: %w", creditAccountID, ErrMissingAccount)
	}

	period, err := periodForDate(ctx, tx, date)
	if err != nil {
		return 0, err
	}
	je, err := createEntry(ctx, tx, date, period, currency, "Inventory receipt", "")
	if err != nil {
		return 0, err
	}

	// total_cost is positive for a receipt.
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 1, p.inventory_account_id, sm.total_cost, 0, 'Inventory'
		 FROM stock_movements sm JOIN products p ON p.id = sm.product_id
		 WHERE sm.id = $2`, je, movementID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO journal_lines (journal_entry_id, line_no, account_id, debit, credit, memo)
		 SELECT $1, 2, $3, 0, sm.total_cost, 'Goods received not invoiced'
		 FROM stock_movements sm WHERE sm.id = $2`, je, movementID, creditAccountID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE stock_movements SET journal_entry_id = $1, period_id = $2 WHERE id = $3`,
		je, period, movementID); err != nil {
		return 0, err
	}
	return je, nil
}

// PostStockMovement posts a stock movement to the GL, dispatching on its type:
// an 'issue' posts COGS, a 'receipt' posts inventory against creditAccountID.
// creditAccountID is ignored for issues and required (> 0) for receipts.
func PostStockMovement(ctx context.Context, tx pgx.Tx, movementID int, currency string, creditAccountID int) (int, error) {
	var movementType string
	err := tx.QueryRow(ctx, `SELECT movement_type FROM stock_movements WHERE id = $1`, movementID).Scan(&movementType)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("posting: stock movement %d: %w", movementID, ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	switch movementType {
	case "issue":
		return PostInventoryIssue(ctx, tx, movementID, currency)
	case "receipt":
		if creditAccountID <= 0 {
			return 0, fmt.Errorf("posting: stock movement %d: a credit account is required to post a receipt: %w", movementID, ErrMissingAccount)
		}
		return PostInventoryReceipt(ctx, tx, movementID, currency, creditAccountID)
	default:
		return 0, fmt.Errorf("posting: stock movement %d: type %q is not postable to the GL: %w", movementID, movementType, ErrNotPostable)
	}
}
