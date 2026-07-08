package reporting

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// SalesOrderSummary is the header view of a sales order with its derived
// fulfilment state. InvoicedStatus and ShippedStatus are each one of
// none/partial/invoiced (or shipped), computed over the order's lines.
type SalesOrderSummary struct {
	ID               int     `json:"id"`
	Number           string  `json:"order_number"`
	CustomerID       int     `json:"customer_id"`
	OrderDate        string  `json:"order_date"`
	ExpectedShipDate *string `json:"expected_ship_date"`
	Currency         string  `json:"currency_code"`
	Status           string  `json:"status"`
	Total            string  `json:"total"`
	InvoicedStatus   string  `json:"invoiced_status"`
	ShippedStatus    string  `json:"shipped_status"`
	Reference        *string `json:"reference"`
	Memo             *string `json:"memo"`
}

const salesOrderSQL = `
	SELECT f.order_id, f.order_number, f.customer_id, f.order_date::text,
	       so.expected_ship_date::text, f.currency_code, f.status,
	       f.total::numeric(19,4)::text, f.invoiced_status, f.shipped_status,
	       so.reference, so.memo
	FROM sales_order_fulfilment f JOIN sales_orders so ON so.id = f.order_id`

// SalesOrders returns the header view of every sales order, newest first.
func SalesOrders(ctx context.Context, q Querier) ([]SalesOrderSummary, error) {
	rows, err := q.Query(ctx, salesOrderSQL+` ORDER BY f.order_date DESC, f.order_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SalesOrderSummary{}
	for rows.Next() {
		var o SalesOrderSummary
		if err := rows.Scan(&o.ID, &o.Number, &o.CustomerID, &o.OrderDate, &o.ExpectedShipDate,
			&o.Currency, &o.Status, &o.Total, &o.InvoicedStatus, &o.ShippedStatus,
			&o.Reference, &o.Memo); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// SalesOrder returns a single sales order's header view.
func SalesOrder(ctx context.Context, q Querier, orderID int) (SalesOrderSummary, error) {
	var o SalesOrderSummary
	err := q.QueryRow(ctx, salesOrderSQL+` WHERE f.order_id = $1`, orderID).Scan(
		&o.ID, &o.Number, &o.CustomerID, &o.OrderDate, &o.ExpectedShipDate,
		&o.Currency, &o.Status, &o.Total, &o.InvoicedStatus, &o.ShippedStatus,
		&o.Reference, &o.Memo)
	if err == pgx.ErrNoRows {
		return o, ErrNotFound
	}
	return o, err
}

// SalesOrderLine is one order line with its money and derived fulfilment.
type SalesOrderLine struct {
	LineNo           int     `json:"line_no"`
	OrderLineID      int     `json:"order_line_id"`
	ProductID        *int    `json:"product_id"`
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`
	UnitPrice        string  `json:"unit_price"`
	TaxCode          *string `json:"tax_code"`
	TaxRate          string  `json:"tax_rate"`
	LineSubtotal     string  `json:"line_subtotal"`
	TaxAmount        string  `json:"tax_amount"`
	LineTotal        string  `json:"line_total"`
	RevenueAccountID *int    `json:"revenue_account_id"`
	QtyInvoiced      string  `json:"qty_invoiced"`
	QtyShipped       string  `json:"qty_shipped"`
	QtyToInvoice     string  `json:"qty_to_invoice"`
	QtyToShip        string  `json:"qty_to_ship"`
}

// SalesOrderLines returns an order's lines in order, or ErrNotFound when the
// order itself does not exist.
func SalesOrderLines(ctx context.Context, q Querier, orderID int) ([]SalesOrderLine, error) {
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM sales_orders WHERE id = $1)`, orderID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := q.Query(ctx, `
		SELECT sol.line_no, sol.id, sol.product_id, sol.description,
		       sol.quantity::numeric(19,4)::text, sol.unit_price::numeric(19,4)::text,
		       sol.tax_code, sol.tax_rate::numeric(7,4)::text,
		       sol.line_subtotal::numeric(19,4)::text, sol.tax_amount::numeric(19,4)::text, sol.line_total::numeric(19,4)::text,
		       sol.revenue_account_id,
		       f.qty_invoiced::numeric(19,4)::text, f.qty_shipped::numeric(19,4)::text,
		       f.qty_to_invoice::numeric(19,4)::text, f.qty_to_ship::numeric(19,4)::text
		FROM sales_order_lines sol
		JOIN sales_order_line_fulfilment f ON f.order_line_id = sol.id
		WHERE sol.order_id = $1 ORDER BY sol.line_no`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []SalesOrderLine{}
	for rows.Next() {
		var l SalesOrderLine
		if err := rows.Scan(&l.LineNo, &l.OrderLineID, &l.ProductID, &l.Description,
			&l.Quantity, &l.UnitPrice, &l.TaxCode, &l.TaxRate,
			&l.LineSubtotal, &l.TaxAmount, &l.LineTotal, &l.RevenueAccountID,
			&l.QtyInvoiced, &l.QtyShipped, &l.QtyToInvoice, &l.QtyToShip); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// PurchaseOrderSummary is the purchasing-side mirror of SalesOrderSummary.
type PurchaseOrderSummary struct {
	ID                  int     `json:"id"`
	Number              string  `json:"order_number"`
	SupplierID          int     `json:"supplier_id"`
	OrderDate           string  `json:"order_date"`
	ExpectedReceiptDate *string `json:"expected_receipt_date"`
	Currency            string  `json:"currency_code"`
	Status              string  `json:"status"`
	Total               string  `json:"total"`
	BilledStatus        string  `json:"billed_status"`
	ReceivedStatus      string  `json:"received_status"`
	Reference           *string `json:"reference"`
	Memo                *string `json:"memo"`
}

const purchaseOrderSQL = `
	SELECT f.order_id, f.order_number, f.supplier_id, f.order_date::text,
	       po.expected_receipt_date::text, f.currency_code, f.status,
	       f.total::numeric(19,4)::text, f.billed_status, f.received_status,
	       po.reference, po.memo
	FROM purchase_order_fulfilment f JOIN purchase_orders po ON po.id = f.order_id`

// PurchaseOrders returns the header view of every purchase order, newest first.
func PurchaseOrders(ctx context.Context, q Querier) ([]PurchaseOrderSummary, error) {
	rows, err := q.Query(ctx, purchaseOrderSQL+` ORDER BY f.order_date DESC, f.order_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PurchaseOrderSummary{}
	for rows.Next() {
		var o PurchaseOrderSummary
		if err := rows.Scan(&o.ID, &o.Number, &o.SupplierID, &o.OrderDate, &o.ExpectedReceiptDate,
			&o.Currency, &o.Status, &o.Total, &o.BilledStatus, &o.ReceivedStatus,
			&o.Reference, &o.Memo); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// PurchaseOrder returns a single purchase order's header view.
func PurchaseOrder(ctx context.Context, q Querier, orderID int) (PurchaseOrderSummary, error) {
	var o PurchaseOrderSummary
	err := q.QueryRow(ctx, purchaseOrderSQL+` WHERE f.order_id = $1`, orderID).Scan(
		&o.ID, &o.Number, &o.SupplierID, &o.OrderDate, &o.ExpectedReceiptDate,
		&o.Currency, &o.Status, &o.Total, &o.BilledStatus, &o.ReceivedStatus,
		&o.Reference, &o.Memo)
	if err == pgx.ErrNoRows {
		return o, ErrNotFound
	}
	return o, err
}

// PurchaseOrderLine is the purchasing-side mirror of SalesOrderLine.
type PurchaseOrderLine struct {
	LineNo           int     `json:"line_no"`
	OrderLineID      int     `json:"order_line_id"`
	ProductID        *int    `json:"product_id"`
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`
	UnitCost         string  `json:"unit_cost"`
	TaxCode          *string `json:"tax_code"`
	TaxRate          string  `json:"tax_rate"`
	LineSubtotal     string  `json:"line_subtotal"`
	TaxAmount        string  `json:"tax_amount"`
	LineTotal        string  `json:"line_total"`
	ExpenseAccountID *int    `json:"expense_account_id"`
	QtyBilled        string  `json:"qty_billed"`
	QtyReceived      string  `json:"qty_received"`
	QtyToBill        string  `json:"qty_to_bill"`
	QtyToReceive     string  `json:"qty_to_receive"`
}

// PurchaseOrderLines returns an order's lines in order, or ErrNotFound when the
// order itself does not exist.
func PurchaseOrderLines(ctx context.Context, q Querier, orderID int) ([]PurchaseOrderLine, error) {
	var exists bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM purchase_orders WHERE id = $1)`, orderID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := q.Query(ctx, `
		SELECT pol.line_no, pol.id, pol.product_id, pol.description,
		       pol.quantity::numeric(19,4)::text, pol.unit_cost::numeric(19,4)::text,
		       pol.tax_code, pol.tax_rate::numeric(7,4)::text,
		       pol.line_subtotal::numeric(19,4)::text, pol.tax_amount::numeric(19,4)::text, pol.line_total::numeric(19,4)::text,
		       pol.expense_account_id,
		       f.qty_billed::numeric(19,4)::text, f.qty_received::numeric(19,4)::text,
		       f.qty_to_bill::numeric(19,4)::text, f.qty_to_receive::numeric(19,4)::text
		FROM purchase_order_lines pol
		JOIN purchase_order_line_fulfilment f ON f.order_line_id = pol.id
		WHERE pol.order_id = $1 ORDER BY pol.line_no`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PurchaseOrderLine{}
	for rows.Next() {
		var l PurchaseOrderLine
		if err := rows.Scan(&l.LineNo, &l.OrderLineID, &l.ProductID, &l.Description,
			&l.Quantity, &l.UnitCost, &l.TaxCode, &l.TaxRate,
			&l.LineSubtotal, &l.TaxAmount, &l.LineTotal, &l.ExpenseAccountID,
			&l.QtyBilled, &l.QtyReceived, &l.QtyToBill, &l.QtyToReceive); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
