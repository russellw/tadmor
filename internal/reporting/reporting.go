// Package reporting provides read-only queries over the accounting views.
//
// Monetary and date values are returned as strings (selected with ::text) so
// exact numeric and date values are never forced through Go floating point.
package reporting

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a requested single record does not exist.
var ErrNotFound = errors.New("not found")

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TrialBalanceRow is one account's totals over posted entries.
type TrialBalanceRow struct {
	AccountID   int    `json:"account_id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	AccountType string `json:"account_type"`
	TotalDebit  string `json:"total_debit"`
	TotalCredit string `json:"total_credit"`
	Balance     string `json:"balance"`
}

// TrialBalance returns every account with its debit/credit totals and balance.
func TrialBalance(ctx context.Context, q Querier) ([]TrialBalanceRow, error) {
	rows, err := q.Query(ctx,
		`SELECT account_id, code, name, account_type,
		        total_debit::numeric(19,4)::text, total_credit::numeric(19,4)::text, balance::numeric(19,4)::text
		 FROM trial_balance ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TrialBalanceRow{}
	for rows.Next() {
		var r TrialBalanceRow
		if err := rows.Scan(&r.AccountID, &r.Code, &r.Name, &r.AccountType,
			&r.TotalDebit, &r.TotalCredit, &r.Balance); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AccountActivityRow is one account's amount on a financial statement, in the
// account's natural sign: debit-positive for assets and expenses,
// credit-positive for liabilities, equity, and revenue. Accounts with no
// posted activity in the report's range are omitted; offsetting activity
// shows as 0.0000.
type AccountActivityRow struct {
	AccountID   int    `json:"account_id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	AccountType string `json:"account_type"`
	Amount      string `json:"amount"`
}

// ProfitAndLoss returns revenue and expense accounts with posted activity in
// the inclusive [from, to] entry-date range. A nil bound is unbounded.
func ProfitAndLoss(ctx context.Context, q Querier, from, to *string) ([]AccountActivityRow, error) {
	return accountActivityRows(ctx, q,
		`SELECT a.id, a.code, a.name, a.account_type,
		        sum(CASE WHEN a.account_type = 'revenue' THEN jl.credit - jl.debit
		                 ELSE jl.debit - jl.credit END)::numeric(19,4)::text
		 FROM journal_lines jl
		 JOIN journal_entries je ON je.id = jl.journal_entry_id
		 JOIN accounts a ON a.id = jl.account_id
		 WHERE je.status = 'posted'
		   AND a.account_type IN ('revenue', 'expense')
		   AND ($1::date IS NULL OR je.entry_date >= $1::date)
		   AND ($2::date IS NULL OR je.entry_date <= $2::date)
		 GROUP BY a.id, a.code, a.name, a.account_type
		 ORDER BY a.code`, from, to)
}

// BalanceSheet is the statement of financial position as of a date.
// CurrentEarnings is credit-positive net income (revenue minus expenses) over
// all posted entries up to the same date; until a year-end close moves it
// into retained earnings it is what makes the sheet balance:
// assets = liabilities + equity + current earnings.
type BalanceSheet struct {
	Rows            []AccountActivityRow `json:"rows"`
	CurrentEarnings string               `json:"current_earnings"`
}

// BalanceSheetAsOf returns asset, liability, and equity balances from posted
// entries dated on or before asOf (nil = all posted entries).
func BalanceSheetAsOf(ctx context.Context, q Querier, asOf *string) (BalanceSheet, error) {
	rows, err := accountActivityRows(ctx, q,
		`SELECT a.id, a.code, a.name, a.account_type,
		        sum(CASE WHEN a.account_type = 'asset' THEN jl.debit - jl.credit
		                 ELSE jl.credit - jl.debit END)::numeric(19,4)::text
		 FROM journal_lines jl
		 JOIN journal_entries je ON je.id = jl.journal_entry_id
		 JOIN accounts a ON a.id = jl.account_id
		 WHERE je.status = 'posted'
		   AND a.account_type IN ('asset', 'liability', 'equity')
		   AND ($1::date IS NULL OR je.entry_date <= $1::date)
		 GROUP BY a.id, a.code, a.name, a.account_type
		 ORDER BY a.code`, asOf)
	if err != nil {
		return BalanceSheet{}, err
	}
	bs := BalanceSheet{Rows: rows}
	err = q.QueryRow(ctx,
		`SELECT COALESCE(sum(jl.credit - jl.debit), 0)::numeric(19,4)::text
		 FROM journal_lines jl
		 JOIN journal_entries je ON je.id = jl.journal_entry_id
		 JOIN accounts a ON a.id = jl.account_id
		 WHERE je.status = 'posted'
		   AND a.account_type IN ('revenue', 'expense')
		   AND ($1::date IS NULL OR je.entry_date <= $1::date)`, asOf).Scan(&bs.CurrentEarnings)
	return bs, err
}

func accountActivityRows(ctx context.Context, q Querier, sql string, args ...any) ([]AccountActivityRow, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AccountActivityRow{}
	for rows.Next() {
		var r AccountActivityRow
		if err := rows.Scan(&r.AccountID, &r.Code, &r.Name, &r.AccountType, &r.Amount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DocumentBalance is the outstanding-balance view of an invoice or bill.
type DocumentBalance struct {
	ID            int     `json:"id"`
	Number        string  `json:"number"`
	PartyID       int     `json:"party_id"`
	Currency      string  `json:"currency_code"`
	Date          string  `json:"date"`
	DueDate       *string `json:"due_date"`
	Status        string  `json:"status"`
	Total         string  `json:"total"`
	AmountApplied string  `json:"amount_applied"`
	Balance       string  `json:"balance"`
	PaymentStatus string  `json:"payment_status"`
}

// SalesInvoiceBalances returns the balance view of every invoice, newest first.
func SalesInvoiceBalances(ctx context.Context, q Querier) ([]DocumentBalance, error) {
	rows, err := q.Query(ctx,
		`SELECT invoice_id, invoice_number, customer_id, currency_code,
		        invoice_date::text, due_date::text, status,
		        total::numeric(19,4)::text, amount_applied::numeric(19,4)::text, balance::numeric(19,4)::text, payment_status
		 FROM sales_invoice_balances ORDER BY invoice_date DESC, invoice_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DocumentBalance{}
	for rows.Next() {
		var d DocumentBalance
		if err := rows.Scan(&d.ID, &d.Number, &d.PartyID, &d.Currency, &d.Date, &d.DueDate,
			&d.Status, &d.Total, &d.AmountApplied, &d.Balance, &d.PaymentStatus); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SalesInvoiceLine is one invoice line with its database-computed money.
type SalesInvoiceLine struct {
	LineNo       int     `json:"line_no"`
	ProductID    *int    `json:"product_id"`
	Description  string  `json:"description"`
	Quantity     string  `json:"quantity"`
	UnitPrice    string  `json:"unit_price"`
	TaxCode      *string `json:"tax_code"`
	TaxRate      string  `json:"tax_rate"`
	LineSubtotal string  `json:"line_subtotal"`
	TaxAmount    string  `json:"tax_amount"`
	LineTotal    string  `json:"line_total"`
}

// SalesInvoiceLines returns an invoice's lines in order, or ErrNotFound when
// the invoice itself does not exist.
func SalesInvoiceLines(ctx context.Context, q Querier, invoiceID int) ([]SalesInvoiceLine, error) {
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM sales_invoices WHERE id = $1)`, invoiceID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := q.Query(ctx,
		`SELECT line_no, product_id, description,
		        quantity::numeric(19,4)::text, unit_price::numeric(19,4)::text, tax_code, tax_rate::numeric(7,4)::text,
		        line_subtotal::numeric(19,4)::text, tax_amount::numeric(19,4)::text, line_total::numeric(19,4)::text
		 FROM sales_invoice_lines WHERE invoice_id = $1 ORDER BY line_no`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SalesInvoiceLine{}
	for rows.Next() {
		var l SalesInvoiceLine
		if err := rows.Scan(&l.LineNo, &l.ProductID, &l.Description,
			&l.Quantity, &l.UnitPrice, &l.TaxCode, &l.TaxRate,
			&l.LineSubtotal, &l.TaxAmount, &l.LineTotal); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SalesInvoiceBalance returns the balance view of a single invoice.
func SalesInvoiceBalance(ctx context.Context, q Querier, invoiceID int) (DocumentBalance, error) {
	return scanDocumentBalance(q.QueryRow(ctx,
		`SELECT invoice_id, invoice_number, customer_id, currency_code,
		        invoice_date::text, due_date::text, status,
		        total::numeric(19,4)::text, amount_applied::numeric(19,4)::text, balance::numeric(19,4)::text, payment_status
		 FROM sales_invoice_balances WHERE invoice_id = $1`, invoiceID))
}

// PurchaseBillBalances returns the balance view of every bill, newest first.
func PurchaseBillBalances(ctx context.Context, q Querier) ([]DocumentBalance, error) {
	rows, err := q.Query(ctx,
		`SELECT bill_id, bill_number, supplier_id, currency_code,
		        bill_date::text, due_date::text, status,
		        total::numeric(19,4)::text, amount_applied::numeric(19,4)::text, balance::numeric(19,4)::text, payment_status
		 FROM purchase_bill_balances ORDER BY bill_date DESC, bill_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DocumentBalance{}
	for rows.Next() {
		var d DocumentBalance
		if err := rows.Scan(&d.ID, &d.Number, &d.PartyID, &d.Currency, &d.Date, &d.DueDate,
			&d.Status, &d.Total, &d.AmountApplied, &d.Balance, &d.PaymentStatus); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PurchaseBillLine is one bill line with its database-computed money.
type PurchaseBillLine struct {
	LineNo       int     `json:"line_no"`
	ProductID    *int    `json:"product_id"`
	Description  string  `json:"description"`
	Quantity     string  `json:"quantity"`
	UnitCost     string  `json:"unit_cost"`
	TaxCode      *string `json:"tax_code"`
	TaxRate      string  `json:"tax_rate"`
	LineSubtotal string  `json:"line_subtotal"`
	TaxAmount    string  `json:"tax_amount"`
	LineTotal    string  `json:"line_total"`
}

// PurchaseBillLines returns a bill's lines in order, or ErrNotFound when the
// bill itself does not exist.
func PurchaseBillLines(ctx context.Context, q Querier, billID int) ([]PurchaseBillLine, error) {
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM purchase_bills WHERE id = $1)`, billID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := q.Query(ctx,
		`SELECT line_no, product_id, description,
		        quantity::numeric(19,4)::text, unit_cost::numeric(19,4)::text, tax_code, tax_rate::numeric(7,4)::text,
		        line_subtotal::numeric(19,4)::text, tax_amount::numeric(19,4)::text, line_total::numeric(19,4)::text
		 FROM purchase_bill_lines WHERE bill_id = $1 ORDER BY line_no`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PurchaseBillLine{}
	for rows.Next() {
		var l PurchaseBillLine
		if err := rows.Scan(&l.LineNo, &l.ProductID, &l.Description,
			&l.Quantity, &l.UnitCost, &l.TaxCode, &l.TaxRate,
			&l.LineSubtotal, &l.TaxAmount, &l.LineTotal); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// PurchaseBillBalance returns the balance view of a single bill.
func PurchaseBillBalance(ctx context.Context, q Querier, billID int) (DocumentBalance, error) {
	return scanDocumentBalance(q.QueryRow(ctx,
		`SELECT bill_id, bill_number, supplier_id, currency_code,
		        bill_date::text, due_date::text, status,
		        total::numeric(19,4)::text, amount_applied::numeric(19,4)::text, balance::numeric(19,4)::text, payment_status
		 FROM purchase_bill_balances WHERE bill_id = $1`, billID))
}

func scanDocumentBalance(row pgx.Row) (DocumentBalance, error) {
	var d DocumentBalance
	err := row.Scan(&d.ID, &d.Number, &d.PartyID, &d.Currency, &d.Date, &d.DueDate,
		&d.Status, &d.Total, &d.AmountApplied, &d.Balance, &d.PaymentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// Payment is a customer or supplier payment with its applied/unapplied split.
// There is no payment view in the schema; the split is computed here from the
// application tables.
type Payment struct {
	ID            int     `json:"id"`
	PartyID       int     `json:"party_id"`
	Date          string  `json:"date"`
	Currency      string  `json:"currency_code"`
	Amount        string  `json:"amount"`
	Method        *string `json:"method"`
	Reference     *string `json:"reference"`
	Status        string  `json:"status"`
	AmountApplied string  `json:"amount_applied"`
	Unapplied     string  `json:"unapplied"`
}

const customerPaymentsSQL = `
	SELECT cp.id, cp.customer_id, cp.payment_date::text, cp.currency_code,
	       cp.amount::numeric(19,4)::text, cp.method, cp.reference, cp.status,
	       COALESCE(pa.applied, 0)::numeric(19,4)::text,
	       (cp.amount - COALESCE(pa.applied, 0))::numeric(19,4)::text
	FROM customer_payments cp
	LEFT JOIN (
	    SELECT payment_id, sum(amount_applied) AS applied
	    FROM payment_applications GROUP BY payment_id
	) pa ON pa.payment_id = cp.id`

const supplierPaymentsSQL = `
	SELECT sp.id, sp.supplier_id, sp.payment_date::text, sp.currency_code,
	       sp.amount::numeric(19,4)::text, sp.method, sp.reference, sp.status,
	       COALESCE(ba.applied, 0)::numeric(19,4)::text,
	       (sp.amount - COALESCE(ba.applied, 0))::numeric(19,4)::text
	FROM supplier_payments sp
	LEFT JOIN (
	    SELECT payment_id, sum(amount_applied) AS applied
	    FROM bill_applications GROUP BY payment_id
	) ba ON ba.payment_id = sp.id`

// CustomerPayments returns every customer payment, newest first.
func CustomerPayments(ctx context.Context, q Querier) ([]Payment, error) {
	return paymentRows(ctx, q, customerPaymentsSQL+` ORDER BY cp.payment_date DESC, cp.id DESC`)
}

// SupplierPayments returns every supplier payment, newest first.
func SupplierPayments(ctx context.Context, q Querier) ([]Payment, error) {
	return paymentRows(ctx, q, supplierPaymentsSQL+` ORDER BY sp.payment_date DESC, sp.id DESC`)
}

// CustomerPayment returns a single customer payment, or ErrNotFound.
func CustomerPayment(ctx context.Context, q Querier, paymentID int) (Payment, error) {
	return scanPayment(q.QueryRow(ctx, customerPaymentsSQL+` WHERE cp.id = $1`, paymentID))
}

// SupplierPayment returns a single supplier payment, or ErrNotFound.
func SupplierPayment(ctx context.Context, q Querier, paymentID int) (Payment, error) {
	return scanPayment(q.QueryRow(ctx, supplierPaymentsSQL+` WHERE sp.id = $1`, paymentID))
}

func paymentRows(ctx context.Context, q Querier, sql string) ([]Payment, error) {
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Payment{}
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.PartyID, &p.Date, &p.Currency, &p.Amount,
			&p.Method, &p.Reference, &p.Status, &p.AmountApplied, &p.Unapplied); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanPayment(row pgx.Row) (Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.PartyID, &p.Date, &p.Currency, &p.Amount,
		&p.Method, &p.Reference, &p.Status, &p.AmountApplied, &p.Unapplied)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// PaymentApplication is one allocation of a payment to an invoice or bill.
type PaymentApplication struct {
	DocumentID     int    `json:"document_id"`
	DocumentNumber string `json:"document_number"`
	AmountApplied  string `json:"amount_applied"`
}

// CustomerPaymentApplications returns what a customer payment was applied to,
// or ErrNotFound when the payment itself does not exist.
func CustomerPaymentApplications(ctx context.Context, q Querier, paymentID int) ([]PaymentApplication, error) {
	return applicationRows(ctx, q, paymentID,
		`SELECT EXISTS (SELECT 1 FROM customer_payments WHERE id = $1)`,
		`SELECT pa.invoice_id, si.invoice_number, pa.amount_applied::numeric(19,4)::text
		 FROM payment_applications pa JOIN sales_invoices si ON si.id = pa.invoice_id
		 WHERE pa.payment_id = $1 ORDER BY pa.id`)
}

// SupplierPaymentApplications returns what a supplier payment was applied to,
// or ErrNotFound when the payment itself does not exist.
func SupplierPaymentApplications(ctx context.Context, q Querier, paymentID int) ([]PaymentApplication, error) {
	return applicationRows(ctx, q, paymentID,
		`SELECT EXISTS (SELECT 1 FROM supplier_payments WHERE id = $1)`,
		`SELECT ba.bill_id, pb.bill_number, ba.amount_applied::numeric(19,4)::text
		 FROM bill_applications ba JOIN purchase_bills pb ON pb.id = ba.bill_id
		 WHERE ba.payment_id = $1 ORDER BY ba.id`)
}

func applicationRows(ctx context.Context, q Querier, paymentID int, existsSQL, sql string) ([]PaymentApplication, error) {
	var exists bool
	if err := q.QueryRow(ctx, existsSQL, paymentID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := q.Query(ctx, sql, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PaymentApplication{}
	for rows.Next() {
		var a PaymentApplication
		if err := rows.Scan(&a.DocumentID, &a.DocumentNumber, &a.AmountApplied); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AgingRow is one party's outstanding balance bucketed by age.
type AgingRow struct {
	PartyID          int    `json:"party_id"`
	PartyName        string `json:"party_name"`
	TotalOutstanding string `json:"total_outstanding"`
	NotYetDue        string `json:"not_yet_due"`
	Days1To30        string `json:"days_1_30"`
	Days31To60       string `json:"days_31_60"`
	Days61To90       string `json:"days_61_90"`
	DaysOver90       string `json:"days_over_90"`
}

// ARaging returns accounts-receivable aging by customer.
func ARaging(ctx context.Context, q Querier) ([]AgingRow, error) {
	return agingRows(ctx, q,
		`SELECT a.customer_id, o.name,
		        COALESCE(a.total_outstanding,0)::numeric(19,4)::text,
		        COALESCE(a.not_yet_due,0)::numeric(19,4)::text, COALESCE(a.days_1_30,0)::numeric(19,4)::text,
		        COALESCE(a.days_31_60,0)::numeric(19,4)::text, COALESCE(a.days_61_90,0)::numeric(19,4)::text,
		        COALESCE(a.days_over_90,0)::numeric(19,4)::text
		 FROM ar_aging a
		 JOIN customers c ON c.id = a.customer_id
		 JOIN organizations o ON o.id = c.organization_id
		 ORDER BY a.customer_id`)
}

// APaging returns accounts-payable aging by supplier.
func APaging(ctx context.Context, q Querier) ([]AgingRow, error) {
	return agingRows(ctx, q,
		`SELECT a.supplier_id, o.name,
		        COALESCE(a.total_outstanding,0)::numeric(19,4)::text,
		        COALESCE(a.not_yet_due,0)::numeric(19,4)::text, COALESCE(a.days_1_30,0)::numeric(19,4)::text,
		        COALESCE(a.days_31_60,0)::numeric(19,4)::text, COALESCE(a.days_61_90,0)::numeric(19,4)::text,
		        COALESCE(a.days_over_90,0)::numeric(19,4)::text
		 FROM ap_aging a
		 JOIN suppliers s ON s.id = a.supplier_id
		 JOIN organizations o ON o.id = s.organization_id
		 ORDER BY a.supplier_id`)
}

func agingRows(ctx context.Context, q Querier, sql string) ([]AgingRow, error) {
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AgingRow{}
	for rows.Next() {
		var r AgingRow
		if err := rows.Scan(&r.PartyID, &r.PartyName, &r.TotalOutstanding,
			&r.NotYetDue, &r.Days1To30, &r.Days31To60, &r.Days61To90, &r.DaysOver90); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// StockMovement is one inventory movement. There is no status column in the
// schema: a movement is posted iff JournalEntryID is set, and only receipts
// and issues ever post.
type StockMovement struct {
	ID             int     `json:"id"`
	ProductID      int     `json:"product_id"`
	WarehouseID    int     `json:"warehouse_id"`
	Date           string  `json:"date"`
	Type           string  `json:"movement_type"`
	Quantity       string  `json:"quantity"`
	UnitCost       string  `json:"unit_cost"`
	TotalCost      string  `json:"total_cost"`
	Reference      *string `json:"reference"`
	Notes          *string `json:"notes"`
	JournalEntryID *int    `json:"journal_entry_id"`
}

const stockMovementsSQL = `
	SELECT id, product_id, warehouse_id, movement_date::text, movement_type,
	       quantity::numeric(19,4)::text, unit_cost::numeric(19,4)::text, total_cost::numeric(19,4)::text,
	       reference, notes, journal_entry_id
	FROM stock_movements`

// StockMovements returns every stock movement, newest first.
func StockMovements(ctx context.Context, q Querier) ([]StockMovement, error) {
	rows, err := q.Query(ctx, stockMovementsSQL+` ORDER BY movement_date DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []StockMovement{}
	for rows.Next() {
		var m StockMovement
		if err := rows.Scan(&m.ID, &m.ProductID, &m.WarehouseID, &m.Date, &m.Type,
			&m.Quantity, &m.UnitCost, &m.TotalCost, &m.Reference, &m.Notes, &m.JournalEntryID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// StockMovementByID returns a single stock movement, or ErrNotFound.
func StockMovementByID(ctx context.Context, q Querier, movementID int) (StockMovement, error) {
	var m StockMovement
	err := q.QueryRow(ctx, stockMovementsSQL+` WHERE id = $1`, movementID).Scan(
		&m.ID, &m.ProductID, &m.WarehouseID, &m.Date, &m.Type,
		&m.Quantity, &m.UnitCost, &m.TotalCost, &m.Reference, &m.Notes, &m.JournalEntryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// StockValuationRow is a product's quantity and value on hand.
type StockValuationRow struct {
	ProductID   int    `json:"product_id"`
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	QtyOnHand   string `json:"qty_on_hand"`
	ValueOnHand string `json:"value_on_hand"`
	AvgUnitCost string `json:"avg_unit_cost"`
}

// StockValuation returns quantity/value on hand per product across warehouses.
func StockValuation(ctx context.Context, q Querier) ([]StockValuationRow, error) {
	rows, err := q.Query(ctx,
		`SELECT v.product_id, p.sku, p.name,
		        v.qty_on_hand::numeric(19,4)::text, v.value_on_hand::numeric(19,4)::text, v.avg_unit_cost::numeric(19,4)::text
		 FROM stock_valuation v JOIN products p ON p.id = v.product_id
		 ORDER BY p.sku`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []StockValuationRow{}
	for rows.Next() {
		var r StockValuationRow
		if err := rows.Scan(&r.ProductID, &r.SKU, &r.Name,
			&r.QtyOnHand, &r.ValueOnHand, &r.AvgUnitCost); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
