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

// SalesInvoiceBalance returns the balance view of a single invoice.
func SalesInvoiceBalance(ctx context.Context, q Querier, invoiceID int) (DocumentBalance, error) {
	return scanDocumentBalance(q.QueryRow(ctx,
		`SELECT invoice_id, invoice_number, customer_id, currency_code,
		        invoice_date::text, due_date::text, status,
		        total::numeric(19,4)::text, amount_applied::numeric(19,4)::text, balance::numeric(19,4)::text, payment_status
		 FROM sales_invoice_balances WHERE invoice_id = $1`, invoiceID))
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
