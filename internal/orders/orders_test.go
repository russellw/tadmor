package orders_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"tadmor/internal/dbtest"
	"tadmor/internal/orders"
)

// setup builds a rolled-back transaction with a customer, a supplier, a service
// product, an inventory product, and a warehouse, and returns helpers plus the
// ids the tests need.
type fixture struct {
	t       *testing.T
	ctx     context.Context
	tx      pgx.Tx
	exec    func(string, ...any)
	queryID func(string, ...any) int
	cust    int
	sup     int
	svcProd int
	invProd int
	wh      int
}

func setup(t *testing.T) (fixture, func()) {
	t.Helper()
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	dbtest.Reset(ctx, t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		cleanup()
		t.Fatalf("begin: %v", err)
	}

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("setup exec: %v\nsql: %s", err, sql)
		}
	}
	queryID := func(sql string, args ...any) int {
		t.Helper()
		var id int
		if err := tx.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("setup query: %v\nsql: %s", err, sql)
		}
		return id
	}

	f := fixture{t: t, ctx: ctx, tx: tx, exec: exec, queryID: queryID}
	f.cust = queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme Customer') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`)
	f.sup = queryID(`WITH o AS (INSERT INTO organizations (name) VALUES ('Beta Supplier') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='2000') FROM o RETURNING id`)
	f.svcProd = queryID(`INSERT INTO products (sku, name, revenue_account_id)
	      VALUES ('P-SVC','Service',(SELECT id FROM accounts WHERE code='4000')) RETURNING id`)
	f.invProd = queryID(`INSERT INTO products (sku, name, track_inventory, inventory_account_id, cogs_account_id)
	      VALUES ('P-INV','Widget',true,(SELECT id FROM accounts WHERE code='1200'),(SELECT id FROM accounts WHERE code='5000')) RETURNING id`)
	f.wh = queryID(`INSERT INTO warehouses (code, name) VALUES ('MAIN','Main') RETURNING id`)

	return f, func() {
		_ = tx.Rollback(ctx)
		cleanup()
	}
}

// str reads a single text/text-cast column so decimal comparisons stay exact.
func (f fixture) str(sql string, args ...any) string {
	f.t.Helper()
	var s string
	if err := f.tx.QueryRow(f.ctx, sql, args...).Scan(&s); err != nil {
		f.t.Fatalf("query: %v\nsql: %s", err, sql)
	}
	return s
}

func TestSalesOrderInvoiceAndShip(t *testing.T) {
	f, done := setup(t)
	defer done()

	soID := f.queryID(`INSERT INTO sales_orders (order_number, customer_id, order_date, currency_code)
	      VALUES ('SO-1',$1,'2026-06-10','USD') RETURNING id`, f.cust)
	svcLine := f.queryID(`INSERT INTO sales_order_lines (order_id, line_no, product_id, description, quantity, unit_price)
	      VALUES ($1,1,$2,'Service',10,5) RETURNING id`, soID, f.svcProd)
	f.exec(`INSERT INTO sales_order_lines (order_id, line_no, product_id, description, quantity, unit_price)
	      VALUES ($1,2,$2,'Widget',4,25)`, soID, f.invProd)

	// The header total is maintained by trigger: 10*5 + 4*25 = 150.
	if got := f.str(`SELECT total::text FROM sales_orders WHERE id=$1`, soID); got != "150.0000" {
		t.Fatalf("order total = %s, want 150.0000", got)
	}

	// Cannot invoice a draft order.
	if _, err := orders.CreateInvoiceFromSalesOrder(f.ctx, f.tx, soID, orders.InvoiceFromOrderInput{
		InvoiceNumber: "X", InvoiceDate: "2026-06-11",
	}); !errors.Is(err, orders.ErrNotOpen) {
		t.Fatalf("invoice draft order: got %v, want ErrNotOpen", err)
	}

	if err := orders.ConfirmSalesOrder(f.ctx, f.tx, soID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Partial invoice: 6 of the 10 service units.
	inv1, err := orders.CreateInvoiceFromSalesOrder(f.ctx, f.tx, soID, orders.InvoiceFromOrderInput{
		InvoiceNumber: "INV-1", InvoiceDate: "2026-06-11",
		Lines: []orders.LineQty{{OrderLineID: svcLine, Quantity: "6"}},
	})
	if err != nil {
		t.Fatalf("partial invoice: %v", err)
	}
	if got := f.str(`SELECT quantity::text FROM sales_invoice_lines WHERE invoice_id=$1 AND order_line_id=$2`, inv1, svcLine); got != "6.0000" {
		t.Fatalf("invoiced qty = %s, want 6.0000", got)
	}
	if got := f.str(`SELECT qty_to_invoice::text FROM sales_order_line_fulfilment WHERE order_line_id=$1`, svcLine); got != "4.0000" {
		t.Fatalf("remaining to invoice = %s, want 4.0000", got)
	}

	// Invoice everything else (service remainder + all 4 widgets).
	inv2, err := orders.CreateInvoiceFromSalesOrder(f.ctx, f.tx, soID, orders.InvoiceFromOrderInput{
		InvoiceNumber: "INV-2", InvoiceDate: "2026-06-12",
	})
	if err != nil {
		t.Fatalf("invoice remainder: %v", err)
	}
	if n := f.str(`SELECT count(*)::text FROM sales_invoice_lines WHERE invoice_id=$1`, inv2); n != "2" {
		t.Fatalf("remainder invoice line count = %s, want 2", n)
	}
	if got := f.str(`SELECT invoiced_status FROM sales_order_fulfilment WHERE order_id=$1`, soID); got != "invoiced" {
		t.Fatalf("invoiced_status = %s, want invoiced", got)
	}

	// Nothing left to invoice.
	if _, err := orders.CreateInvoiceFromSalesOrder(f.ctx, f.tx, soID, orders.InvoiceFromOrderInput{
		InvoiceNumber: "INV-3", InvoiceDate: "2026-06-13",
	}); !errors.Is(err, orders.ErrNothingToFulfil) {
		t.Fatalf("invoice with nothing left: got %v, want ErrNothingToFulfil", err)
	}

	// Cannot cancel once fulfilled.
	if err := orders.CancelSalesOrder(f.ctx, f.tx, soID); !errors.Is(err, orders.ErrFulfilled) {
		t.Fatalf("cancel fulfilled order: got %v, want ErrFulfilled", err)
	}

	// Ship the inventory line (the service line is not stocked, so it is
	// skipped). Only the widget line yields a movement.
	movs, err := orders.ShipSalesOrder(f.ctx, f.tx, soID, orders.ShipInput{WarehouseID: f.wh})
	if err != nil {
		t.Fatalf("ship: %v", err)
	}
	if len(movs) != 1 {
		t.Fatalf("ship produced %d movements, want 1", len(movs))
	}
	if got := f.str(`SELECT movement_type||' '||quantity::text||' '||source_type FROM stock_movements WHERE id=$1`, movs[0]); got != "issue -4.0000 sales_order_line" {
		t.Fatalf("shipment movement = %q", got)
	}
	if got := f.str(`SELECT shipped_status FROM sales_order_fulfilment WHERE order_id=$1`, soID); got != "shipped" {
		t.Fatalf("shipped_status = %s, want shipped", got)
	}
}

func TestPurchaseOrderReceiveAndBill(t *testing.T) {
	f, done := setup(t)
	defer done()

	poID := f.queryID(`INSERT INTO purchase_orders (order_number, supplier_id, order_date, currency_code)
	      VALUES ('PO-1',$1,'2026-06-10','USD') RETURNING id`, f.sup)
	invLine := f.queryID(`INSERT INTO purchase_order_lines (order_id, line_no, product_id, description, quantity, unit_cost)
	      VALUES ($1,1,$2,'Widget',10,7) RETURNING id`, poID, f.invProd)
	f.exec(`INSERT INTO purchase_order_lines (order_id, line_no, product_id, description, quantity, unit_cost, expense_account_id)
	      VALUES ($1,2,$2,'Freight',1,100,(SELECT id FROM accounts WHERE code='6000'))`, poID, f.svcProd)

	if err := orders.ConfirmPurchaseOrder(f.ctx, f.tx, poID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Receive: only the inventory line yields a movement, at the PO unit cost.
	movs, err := orders.ReceivePurchaseOrder(f.ctx, f.tx, poID, orders.ReceiveInput{WarehouseID: f.wh})
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if len(movs) != 1 {
		t.Fatalf("receive produced %d movements, want 1", len(movs))
	}
	if got := f.str(`SELECT movement_type||' '||quantity::text||' @'||unit_cost::text FROM stock_movements WHERE id=$1`, movs[0]); got != "receipt 10.0000 @7.0000" {
		t.Fatalf("receipt movement = %q", got)
	}
	if got := f.str(`SELECT qty_to_receive::text FROM purchase_order_line_fulfilment WHERE order_line_id=$1`, invLine); got != "0.0000" {
		t.Fatalf("qty_to_receive = %s, want 0.0000", got)
	}
	if got := f.str(`SELECT received_status FROM purchase_order_fulfilment WHERE order_id=$1`, poID); got != "received" {
		t.Fatalf("received_status = %s, want received (service lines are not received)", got)
	}

	// Bill everything: both lines, linked back to the order.
	billID, err := orders.CreateBillFromPurchaseOrder(f.ctx, f.tx, poID, orders.BillFromOrderInput{
		BillNumber: "BILL-1", BillDate: "2026-06-12",
	})
	if err != nil {
		t.Fatalf("bill: %v", err)
	}
	if n := f.str(`SELECT count(*)::text FROM purchase_bill_lines WHERE bill_id=$1 AND order_line_id IS NOT NULL`, billID); n != "2" {
		t.Fatalf("linked bill lines = %s, want 2", n)
	}
	if got := f.str(`SELECT total::text FROM purchase_bills WHERE id=$1`, billID); got != "170.0000" {
		t.Fatalf("bill total = %s, want 170.0000 (10*7 + 1*100)", got)
	}
	if got := f.str(`SELECT billed_status FROM purchase_order_fulfilment WHERE order_id=$1`, poID); got != "billed" {
		t.Fatalf("billed_status = %s, want billed", got)
	}
}

func TestOrderGuards(t *testing.T) {
	f, done := setup(t)
	defer done()

	// Confirming an order with no lines fails.
	empty := f.queryID(`INSERT INTO sales_orders (order_number, customer_id, order_date, currency_code)
	      VALUES ('SO-EMPTY',$1,'2026-06-10','USD') RETURNING id`, f.cust)
	if err := orders.ConfirmSalesOrder(f.ctx, f.tx, empty); !errors.Is(err, orders.ErrNoLines) {
		t.Fatalf("confirm empty: got %v, want ErrNoLines", err)
	}

	// Unknown order → ErrNotFound.
	if err := orders.ConfirmSalesOrder(f.ctx, f.tx, 999999); !errors.Is(err, orders.ErrNotFound) {
		t.Fatalf("confirm missing: got %v, want ErrNotFound", err)
	}

	// The database blocks over-invoicing an order line even by direct insert.
	soID := f.queryID(`INSERT INTO sales_orders (order_number, customer_id, order_date, currency_code)
	      VALUES ('SO-2',$1,'2026-06-10','USD') RETURNING id`, f.cust)
	line := f.queryID(`INSERT INTO sales_order_lines (order_id, line_no, product_id, description, quantity, unit_price)
	      VALUES ($1,1,$2,'Service',5,5) RETURNING id`, soID, f.svcProd)
	if err := orders.ConfirmSalesOrder(f.ctx, f.tx, soID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	inv := f.queryID(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
	      VALUES ('INV-OVER',$1,'2026-06-11','USD') RETURNING id`, f.cust)
	// 8 > 5 ordered; the deferred constraint should reject it at flush time.
	f.exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, order_line_id)
	      VALUES ($1,1,'Service',8,5,$2)`, inv, line)
	_, err := f.tx.Exec(f.ctx, `SET CONSTRAINTS ALL IMMEDIATE`)
	if err == nil {
		t.Fatal("over-invoicing an order line was allowed")
	}
}
