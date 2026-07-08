// Package documents creates draft subledger documents (invoices, bills,
// payments, stock movements) from request inputs.
//
// Decimal fields are accepted as strings and cast to numeric in SQL, so values
// are exact and never pass through Go (or JSON) floating point. Creation always
// produces a draft; posting to the GL is a separate step (see package posting).
// Invoice and bill header totals are maintained from their lines by database
// triggers, so they are not set here.
package documents

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ---------------------------------------------------------------------------
// Sales invoices
// ---------------------------------------------------------------------------

type SalesInvoiceLineInput struct {
	ProductID        *int    `json:"product_id"`
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`   // decimal; default "1"
	UnitPrice        string  `json:"unit_price"` // decimal; default "0"
	RevenueAccountID *int    `json:"revenue_account_id"`
	TaxCode          *string `json:"tax_code"`
	TaxRate          string  `json:"tax_rate"` // percent; default "0"
}

type SalesInvoiceInput struct {
	InvoiceNumber string                  `json:"invoice_number"`
	CustomerID    int                     `json:"customer_id"`
	InvoiceDate   string                  `json:"invoice_date"` // YYYY-MM-DD
	DueDate       *string                 `json:"due_date"`
	CurrencyCode  string                  `json:"currency_code"`
	Reference     *string                 `json:"reference"`
	Memo          *string                 `json:"memo"`
	Lines         []SalesInvoiceLineInput `json:"lines"`
}

// Validate returns a message describing the first validation problem, or "".
func (in SalesInvoiceInput) Validate() string {
	switch {
	case in.InvoiceNumber == "":
		return "invoice_number is required"
	case in.CustomerID <= 0:
		return "customer_id is required"
	case in.InvoiceDate == "":
		return "invoice_date is required"
	case in.CurrencyCode == "":
		return "currency_code is required"
	}
	for i, l := range in.Lines {
		if l.Description == "" {
			return "line " + strconv.Itoa(i+1) + ": description is required"
		}
	}
	return ""
}

// CreateSalesInvoice inserts a draft invoice and its lines, returning the id.
func CreateSalesInvoice(ctx context.Context, tx pgx.Tx, in SalesInvoiceInput) (int, error) {
	var id int
	if err := tx.QueryRow(ctx,
		`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, due_date, currency_code, reference, memo)
		 VALUES ($1, $2, $3::date, $4::date, $5, $6, $7)
		 RETURNING id`,
		in.InvoiceNumber, in.CustomerID, in.InvoiceDate, in.DueDate, in.CurrencyCode, in.Reference, in.Memo).Scan(&id); err != nil {
		return 0, err
	}
	return id, insertSalesInvoiceLines(ctx, tx, id, in.Lines)
}

func insertSalesInvoiceLines(ctx context.Context, tx pgx.Tx, id int, lines []SalesInvoiceLineInput) error {
	for i, l := range lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO sales_invoice_lines
			   (invoice_id, line_no, product_id, description, quantity, unit_price, revenue_account_id, tax_code, tax_rate)
			 VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7, $8, $9::numeric)`,
			id, i+1, l.ProductID, l.Description,
			orDefault(l.Quantity, "1"), orDefault(l.UnitPrice, "0"), l.RevenueAccountID, l.TaxCode, orDefault(l.TaxRate, "0")); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Purchase bills
// ---------------------------------------------------------------------------

type PurchaseBillLineInput struct {
	ProductID        *int    `json:"product_id"`
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`  // decimal; default "1"
	UnitCost         string  `json:"unit_cost"` // decimal; default "0"
	ExpenseAccountID *int    `json:"expense_account_id"`
	TaxCode          *string `json:"tax_code"`
	TaxRate          string  `json:"tax_rate"` // percent; default "0"
}

type PurchaseBillInput struct {
	BillNumber   string                  `json:"bill_number"`
	SupplierID   int                     `json:"supplier_id"`
	BillDate     string                  `json:"bill_date"` // YYYY-MM-DD
	DueDate      *string                 `json:"due_date"`
	CurrencyCode string                  `json:"currency_code"`
	Reference    *string                 `json:"reference"`
	Memo         *string                 `json:"memo"`
	Lines        []PurchaseBillLineInput `json:"lines"`
}

func (in PurchaseBillInput) Validate() string {
	switch {
	case in.BillNumber == "":
		return "bill_number is required"
	case in.SupplierID <= 0:
		return "supplier_id is required"
	case in.BillDate == "":
		return "bill_date is required"
	case in.CurrencyCode == "":
		return "currency_code is required"
	}
	for i, l := range in.Lines {
		if l.Description == "" {
			return "line " + strconv.Itoa(i+1) + ": description is required"
		}
	}
	return ""
}

// CreatePurchaseBill inserts a draft bill and its lines, returning the id.
func CreatePurchaseBill(ctx context.Context, tx pgx.Tx, in PurchaseBillInput) (int, error) {
	var id int
	if err := tx.QueryRow(ctx,
		`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, due_date, currency_code, reference, memo)
		 VALUES ($1, $2, $3::date, $4::date, $5, $6, $7)
		 RETURNING id`,
		in.BillNumber, in.SupplierID, in.BillDate, in.DueDate, in.CurrencyCode, in.Reference, in.Memo).Scan(&id); err != nil {
		return 0, err
	}
	return id, insertPurchaseBillLines(ctx, tx, id, in.Lines)
}

func insertPurchaseBillLines(ctx context.Context, tx pgx.Tx, id int, lines []PurchaseBillLineInput) error {
	for i, l := range lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO purchase_bill_lines
			   (bill_id, line_no, product_id, description, quantity, unit_cost, expense_account_id, tax_code, tax_rate)
			 VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7, $8, $9::numeric)`,
			id, i+1, l.ProductID, l.Description,
			orDefault(l.Quantity, "1"), orDefault(l.UnitCost, "0"), l.ExpenseAccountID, l.TaxCode, orDefault(l.TaxRate, "0")); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Credit notes
// ---------------------------------------------------------------------------

// SalesCreditNoteInput reuses the invoice line shape: a credit-note line
// records what is being credited exactly as an invoice line records what was
// billed.
type SalesCreditNoteInput struct {
	CreditNoteNumber string                  `json:"credit_note_number"`
	CustomerID       int                     `json:"customer_id"`
	CreditNoteDate   string                  `json:"credit_note_date"` // YYYY-MM-DD
	CurrencyCode     string                  `json:"currency_code"`
	Reference        *string                 `json:"reference"`
	Memo             *string                 `json:"memo"`
	Lines            []SalesInvoiceLineInput `json:"lines"`
}

func (in SalesCreditNoteInput) Validate() string {
	switch {
	case in.CreditNoteNumber == "":
		return "credit_note_number is required"
	case in.CustomerID <= 0:
		return "customer_id is required"
	case in.CreditNoteDate == "":
		return "credit_note_date is required"
	case in.CurrencyCode == "":
		return "currency_code is required"
	}
	for i, l := range in.Lines {
		if l.Description == "" {
			return "line " + strconv.Itoa(i+1) + ": description is required"
		}
	}
	return ""
}

// CreateSalesCreditNote inserts a draft credit note and its lines, returning
// the id.
func CreateSalesCreditNote(ctx context.Context, tx pgx.Tx, in SalesCreditNoteInput) (int, error) {
	var id int
	if err := tx.QueryRow(ctx,
		`INSERT INTO sales_credit_notes (credit_note_number, customer_id, credit_note_date, currency_code, reference, memo)
		 VALUES ($1, $2, $3::date, $4, $5, $6)
		 RETURNING id`,
		in.CreditNoteNumber, in.CustomerID, in.CreditNoteDate, in.CurrencyCode, in.Reference, in.Memo).Scan(&id); err != nil {
		return 0, err
	}
	return id, insertSalesCreditNoteLines(ctx, tx, id, in.Lines)
}

func insertSalesCreditNoteLines(ctx context.Context, tx pgx.Tx, id int, lines []SalesInvoiceLineInput) error {
	for i, l := range lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO sales_credit_note_lines
			   (credit_note_id, line_no, product_id, description, quantity, unit_price, revenue_account_id, tax_code, tax_rate)
			 VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7, $8, $9::numeric)`,
			id, i+1, l.ProductID, l.Description,
			orDefault(l.Quantity, "1"), orDefault(l.UnitPrice, "0"), l.RevenueAccountID, l.TaxCode, orDefault(l.TaxRate, "0")); err != nil {
			return err
		}
	}
	return nil
}

// PurchaseCreditNoteInput reuses the bill line shape for the same reason
// SalesCreditNoteInput reuses the invoice's.
type PurchaseCreditNoteInput struct {
	CreditNoteNumber string                  `json:"credit_note_number"`
	SupplierID       int                     `json:"supplier_id"`
	CreditNoteDate   string                  `json:"credit_note_date"` // YYYY-MM-DD
	CurrencyCode     string                  `json:"currency_code"`
	Reference        *string                 `json:"reference"`
	Memo             *string                 `json:"memo"`
	Lines            []PurchaseBillLineInput `json:"lines"`
}

func (in PurchaseCreditNoteInput) Validate() string {
	switch {
	case in.CreditNoteNumber == "":
		return "credit_note_number is required"
	case in.SupplierID <= 0:
		return "supplier_id is required"
	case in.CreditNoteDate == "":
		return "credit_note_date is required"
	case in.CurrencyCode == "":
		return "currency_code is required"
	}
	for i, l := range in.Lines {
		if l.Description == "" {
			return "line " + strconv.Itoa(i+1) + ": description is required"
		}
	}
	return ""
}

// CreatePurchaseCreditNote inserts a draft credit note and its lines,
// returning the id.
func CreatePurchaseCreditNote(ctx context.Context, tx pgx.Tx, in PurchaseCreditNoteInput) (int, error) {
	var id int
	if err := tx.QueryRow(ctx,
		`INSERT INTO purchase_credit_notes (credit_note_number, supplier_id, credit_note_date, currency_code, reference, memo)
		 VALUES ($1, $2, $3::date, $4, $5, $6)
		 RETURNING id`,
		in.CreditNoteNumber, in.SupplierID, in.CreditNoteDate, in.CurrencyCode, in.Reference, in.Memo).Scan(&id); err != nil {
		return 0, err
	}
	return id, insertPurchaseCreditNoteLines(ctx, tx, id, in.Lines)
}

func insertPurchaseCreditNoteLines(ctx context.Context, tx pgx.Tx, id int, lines []PurchaseBillLineInput) error {
	for i, l := range lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO purchase_credit_note_lines
			   (credit_note_id, line_no, product_id, description, quantity, unit_cost, expense_account_id, tax_code, tax_rate)
			 VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7, $8, $9::numeric)`,
			id, i+1, l.ProductID, l.Description,
			orDefault(l.Quantity, "1"), orDefault(l.UnitCost, "0"), l.ExpenseAccountID, l.TaxCode, orDefault(l.TaxRate, "0")); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Sales orders
// ---------------------------------------------------------------------------

// SalesOrderLineInput mirrors a sales-invoice line: an order line records what
// the customer agreed to buy, priced exactly as it will later be invoiced.
type SalesOrderLineInput struct {
	ProductID        *int    `json:"product_id"`
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`   // decimal; default "1"
	UnitPrice        string  `json:"unit_price"` // decimal; default "0"
	RevenueAccountID *int    `json:"revenue_account_id"`
	TaxCode          *string `json:"tax_code"`
	TaxRate          string  `json:"tax_rate"` // percent; default "0"
}

type SalesOrderInput struct {
	OrderNumber      string                `json:"order_number"`
	CustomerID       int                   `json:"customer_id"`
	OrderDate        string                `json:"order_date"` // YYYY-MM-DD
	ExpectedShipDate *string               `json:"expected_ship_date"`
	CurrencyCode     string                `json:"currency_code"`
	Reference        *string               `json:"reference"`
	Memo             *string               `json:"memo"`
	Lines            []SalesOrderLineInput `json:"lines"`
}

func (in SalesOrderInput) Validate() string {
	switch {
	case in.OrderNumber == "":
		return "order_number is required"
	case in.CustomerID <= 0:
		return "customer_id is required"
	case in.OrderDate == "":
		return "order_date is required"
	case in.CurrencyCode == "":
		return "currency_code is required"
	}
	for i, l := range in.Lines {
		if l.Description == "" {
			return "line " + strconv.Itoa(i+1) + ": description is required"
		}
	}
	return ""
}

// CreateSalesOrder inserts a draft sales order and its lines, returning the id.
func CreateSalesOrder(ctx context.Context, tx pgx.Tx, in SalesOrderInput) (int, error) {
	var id int
	if err := tx.QueryRow(ctx,
		`INSERT INTO sales_orders (order_number, customer_id, order_date, expected_ship_date, currency_code, reference, memo)
		 VALUES ($1, $2, $3::date, $4::date, $5, $6, $7)
		 RETURNING id`,
		in.OrderNumber, in.CustomerID, in.OrderDate, in.ExpectedShipDate, in.CurrencyCode, in.Reference, in.Memo).Scan(&id); err != nil {
		return 0, err
	}
	return id, insertSalesOrderLines(ctx, tx, id, in.Lines)
}

func insertSalesOrderLines(ctx context.Context, tx pgx.Tx, id int, lines []SalesOrderLineInput) error {
	for i, l := range lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO sales_order_lines
			   (order_id, line_no, product_id, description, quantity, unit_price, revenue_account_id, tax_code, tax_rate)
			 VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7, $8, $9::numeric)`,
			id, i+1, l.ProductID, l.Description,
			orDefault(l.Quantity, "1"), orDefault(l.UnitPrice, "0"), l.RevenueAccountID, l.TaxCode, orDefault(l.TaxRate, "0")); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Purchase orders
// ---------------------------------------------------------------------------

// PurchaseOrderLineInput mirrors a purchase-bill line.
type PurchaseOrderLineInput struct {
	ProductID        *int    `json:"product_id"`
	Description      string  `json:"description"`
	Quantity         string  `json:"quantity"`  // decimal; default "1"
	UnitCost         string  `json:"unit_cost"` // decimal; default "0"
	ExpenseAccountID *int    `json:"expense_account_id"`
	TaxCode          *string `json:"tax_code"`
	TaxRate          string  `json:"tax_rate"` // percent; default "0"
}

type PurchaseOrderInput struct {
	OrderNumber         string                   `json:"order_number"`
	SupplierID          int                      `json:"supplier_id"`
	OrderDate           string                   `json:"order_date"` // YYYY-MM-DD
	ExpectedReceiptDate *string                  `json:"expected_receipt_date"`
	CurrencyCode        string                   `json:"currency_code"`
	Reference           *string                  `json:"reference"`
	Memo                *string                  `json:"memo"`
	Lines               []PurchaseOrderLineInput `json:"lines"`
}

func (in PurchaseOrderInput) Validate() string {
	switch {
	case in.OrderNumber == "":
		return "order_number is required"
	case in.SupplierID <= 0:
		return "supplier_id is required"
	case in.OrderDate == "":
		return "order_date is required"
	case in.CurrencyCode == "":
		return "currency_code is required"
	}
	for i, l := range in.Lines {
		if l.Description == "" {
			return "line " + strconv.Itoa(i+1) + ": description is required"
		}
	}
	return ""
}

// CreatePurchaseOrder inserts a draft purchase order and its lines, returning
// the id.
func CreatePurchaseOrder(ctx context.Context, tx pgx.Tx, in PurchaseOrderInput) (int, error) {
	var id int
	if err := tx.QueryRow(ctx,
		`INSERT INTO purchase_orders (order_number, supplier_id, order_date, expected_receipt_date, currency_code, reference, memo)
		 VALUES ($1, $2, $3::date, $4::date, $5, $6, $7)
		 RETURNING id`,
		in.OrderNumber, in.SupplierID, in.OrderDate, in.ExpectedReceiptDate, in.CurrencyCode, in.Reference, in.Memo).Scan(&id); err != nil {
		return 0, err
	}
	return id, insertPurchaseOrderLines(ctx, tx, id, in.Lines)
}

func insertPurchaseOrderLines(ctx context.Context, tx pgx.Tx, id int, lines []PurchaseOrderLineInput) error {
	for i, l := range lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO purchase_order_lines
			   (order_id, line_no, product_id, description, quantity, unit_cost, expense_account_id, tax_code, tax_rate)
			 VALUES ($1, $2, $3, $4, $5::numeric, $6::numeric, $7, $8, $9::numeric)`,
			id, i+1, l.ProductID, l.Description,
			orDefault(l.Quantity, "1"), orDefault(l.UnitCost, "0"), l.ExpenseAccountID, l.TaxCode, orDefault(l.TaxRate, "0")); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Payments
// ---------------------------------------------------------------------------

type CustomerPaymentInput struct {
	CustomerID       int     `json:"customer_id"`
	PaymentDate      string  `json:"payment_date"` // YYYY-MM-DD
	CurrencyCode     string  `json:"currency_code"`
	Amount           string  `json:"amount"` // decimal
	Method           *string `json:"method"`
	Reference        *string `json:"reference"`
	DepositAccountID *int    `json:"deposit_account_id"`
}

func (in CustomerPaymentInput) Validate() string {
	switch {
	case in.CustomerID <= 0:
		return "customer_id is required"
	case in.PaymentDate == "":
		return "payment_date is required"
	case in.CurrencyCode == "":
		return "currency_code is required"
	case in.Amount == "":
		return "amount is required"
	}
	return ""
}

// CreateCustomerPayment inserts a draft customer payment, returning the id.
func CreateCustomerPayment(ctx context.Context, tx pgx.Tx, in CustomerPaymentInput) (int, error) {
	var id int
	err := tx.QueryRow(ctx,
		`INSERT INTO customer_payments (customer_id, payment_date, currency_code, amount, method, reference, deposit_account_id)
		 VALUES ($1, $2::date, $3, $4::numeric, $5, $6, $7)
		 RETURNING id`,
		in.CustomerID, in.PaymentDate, in.CurrencyCode, in.Amount, in.Method, in.Reference, in.DepositAccountID).Scan(&id)
	return id, err
}

type SupplierPaymentInput struct {
	SupplierID       int     `json:"supplier_id"`
	PaymentDate      string  `json:"payment_date"` // YYYY-MM-DD
	CurrencyCode     string  `json:"currency_code"`
	Amount           string  `json:"amount"` // decimal
	Method           *string `json:"method"`
	Reference        *string `json:"reference"`
	PaymentAccountID *int    `json:"payment_account_id"`
}

func (in SupplierPaymentInput) Validate() string {
	switch {
	case in.SupplierID <= 0:
		return "supplier_id is required"
	case in.PaymentDate == "":
		return "payment_date is required"
	case in.CurrencyCode == "":
		return "currency_code is required"
	case in.Amount == "":
		return "amount is required"
	}
	return ""
}

// CreateSupplierPayment inserts a draft supplier payment, returning the id.
func CreateSupplierPayment(ctx context.Context, tx pgx.Tx, in SupplierPaymentInput) (int, error) {
	var id int
	err := tx.QueryRow(ctx,
		`INSERT INTO supplier_payments (supplier_id, payment_date, currency_code, amount, method, reference, payment_account_id)
		 VALUES ($1, $2::date, $3, $4::numeric, $5, $6, $7)
		 RETURNING id`,
		in.SupplierID, in.PaymentDate, in.CurrencyCode, in.Amount, in.Method, in.Reference, in.PaymentAccountID).Scan(&id)
	return id, err
}

// ---------------------------------------------------------------------------
// Stock movements
// ---------------------------------------------------------------------------

type StockMovementInput struct {
	ProductID    int     `json:"product_id"`
	WarehouseID  int     `json:"warehouse_id"`
	MovementType string  `json:"movement_type"`
	MovementDate *string `json:"movement_date"` // YYYY-MM-DD; defaults to today
	Quantity     string  `json:"quantity"`      // signed decimal
	UnitCost     string  `json:"unit_cost"`     // decimal; default "0"
	Reference    *string `json:"reference"`
	Notes        *string `json:"notes"`
}

func (in StockMovementInput) Validate() string {
	switch {
	case in.ProductID <= 0:
		return "product_id is required"
	case in.WarehouseID <= 0:
		return "warehouse_id is required"
	case in.MovementType == "":
		return "movement_type is required"
	case in.Quantity == "":
		return "quantity is required"
	}
	return ""
}

// CreateStockMovement inserts a stock movement, returning the id.
func CreateStockMovement(ctx context.Context, tx pgx.Tx, in StockMovementInput) (int, error) {
	var id int
	err := tx.QueryRow(ctx,
		`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, movement_date, quantity, unit_cost, reference, notes)
		 VALUES ($1, $2, $3, COALESCE($4::date, current_date), $5::numeric, $6::numeric, $7, $8)
		 RETURNING id`,
		in.ProductID, in.WarehouseID, in.MovementType, in.MovementDate,
		in.Quantity, orDefault(in.UnitCost, "0"), in.Reference, in.Notes).Scan(&id)
	return id, err
}
