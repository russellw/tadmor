package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/httpapi"
)

// del issues a DELETE and returns the status and body.
func del(t *testing.T, url string) (int, string) {
	t.Helper()
	return do(t, http.MethodDelete, url, "")
}

// TestDraftEditAndDeleteOverHTTP exercises the PUT/DELETE surface on invoices,
// payments, and stock movements: drafts are rewritable and removable, posted
// documents are neither.
func TestDraftEditAndDeleteOverHTTP(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	resetAuthed(ctx, t, pool)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("setup exec: %v\nsql: %s", err, sql)
		}
	}
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)
	exec(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='1100') FROM o`)
	exec(`INSERT INTO products (sku, name, track_inventory, inventory_account_id)
	      VALUES ('P-INV','Widget',true,(SELECT id FROM accounts WHERE code='1200'))`)
	exec(`INSERT INTO warehouses (code, name) VALUES ('MAIN','Main')`)

	var custID, revAcct, prodID, whID int
	if err := pool.QueryRow(ctx, `SELECT id FROM customers LIMIT 1`).Scan(&custID); err != nil {
		t.Fatalf("customer id: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM accounts WHERE code='4000'`).Scan(&revAcct); err != nil {
		t.Fatalf("revenue account: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM products WHERE sku='P-INV'`).Scan(&prodID); err != nil {
		t.Fatalf("product id: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM warehouses WHERE code='MAIN'`).Scan(&whID); err != nil {
		t.Fatalf("warehouse id: %v", err)
	}

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	idOf := func(body string) int {
		t.Helper()
		var r struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatalf("decode id from %q: %v", body, err)
		}
		return r.ID
	}

	// --- sales invoice --------------------------------------------------------

	status, body := postJSON(t, srv.URL+"/api/sales-invoices", `{
		"invoice_number":"INV-1","customer_id":`+itoa(custID)+`,"invoice_date":"2026-06-15","currency_code":"USD",
		"lines":[{"description":"Service","quantity":"10","unit_price":"5","revenue_account_id":`+itoa(revAcct)+`}]}`)
	if status != http.StatusCreated {
		t.Fatalf("create invoice: status %d (body %s)", status, body)
	}
	invID := idOf(body)
	invURL := srv.URL + "/api/sales-invoices/" + itoa(invID)

	// Rewrite the draft: new number, new date, two lines.
	status, body = putJSON(t, invURL, `{
		"invoice_number":"INV-1-FIXED","customer_id":`+itoa(custID)+`,"invoice_date":"2026-06-16","currency_code":"USD",
		"lines":[{"description":"Service","quantity":"4","unit_price":"5","revenue_account_id":`+itoa(revAcct)+`},
		         {"description":"Extra","quantity":"1","unit_price":"30","revenue_account_id":`+itoa(revAcct)+`}]}`)
	if status != http.StatusOK {
		t.Fatalf("edit draft invoice: status %d (body %s)", status, body)
	}
	var number, total string
	var lineCount int
	if err := pool.QueryRow(ctx,
		`SELECT invoice_number, total::text, (SELECT count(*) FROM sales_invoice_lines l WHERE l.invoice_id = si.id)
		 FROM sales_invoices si WHERE si.id = $1`, invID).Scan(&number, &total, &lineCount); err != nil {
		t.Fatalf("read invoice: %v", err)
	}
	if number != "INV-1-FIXED" || total != "50.0000" || lineCount != 2 {
		t.Errorf("after edit: number=%s total=%s lines=%d, want INV-1-FIXED/50.0000/2", number, total, lineCount)
	}

	// Once posted, the invoice is immutable.
	if status, body := post(t, invURL+"/post"); status != http.StatusOK {
		t.Fatalf("post invoice: status %d (body %s)", status, body)
	}
	if status, body := putJSON(t, invURL, `{
		"invoice_number":"INV-1-FIXED","customer_id":`+itoa(custID)+`,"invoice_date":"2026-06-16","currency_code":"USD",
		"lines":[]}`); status != http.StatusConflict {
		t.Fatalf("edit posted invoice: status %d, want 409 (body %s)", status, body)
	}
	if status, body := del(t, invURL); status != http.StatusConflict {
		t.Fatalf("delete posted invoice: status %d, want 409 (body %s)", status, body)
	}

	// A second draft deletes cleanly.
	status, body = postJSON(t, srv.URL+"/api/sales-invoices", `{
		"invoice_number":"INV-2","customer_id":`+itoa(custID)+`,"invoice_date":"2026-06-15","currency_code":"USD",
		"lines":[{"description":"Oops","quantity":"1","unit_price":"1","revenue_account_id":`+itoa(revAcct)+`}]}`)
	if status != http.StatusCreated {
		t.Fatalf("create second invoice: status %d (body %s)", status, body)
	}
	inv2URL := srv.URL + "/api/sales-invoices/" + itoa(idOf(body))
	if status, body := del(t, inv2URL); status != http.StatusOK {
		t.Fatalf("delete draft invoice: status %d (body %s)", status, body)
	}
	if status, _ := get(t, inv2URL); status != http.StatusNotFound {
		t.Fatalf("deleted invoice still readable: status %d, want 404", status)
	}
	if status, _ := del(t, inv2URL); status != http.StatusNotFound {
		t.Fatalf("delete missing invoice: status %d, want 404", status)
	}

	// --- customer payment -----------------------------------------------------

	status, body = postJSON(t, srv.URL+"/api/customer-payments", `{
		"customer_id":`+itoa(custID)+`,"payment_date":"2026-06-20","currency_code":"USD","amount":"100"}`)
	if status != http.StatusCreated {
		t.Fatalf("create payment: status %d (body %s)", status, body)
	}
	payURL := srv.URL + "/api/customer-payments/" + itoa(idOf(body))
	if status, body := putJSON(t, payURL, `{
		"customer_id":`+itoa(custID)+`,"payment_date":"2026-06-21","currency_code":"USD","amount":"120"}`); status != http.StatusOK {
		t.Fatalf("edit draft payment: status %d (body %s)", status, body)
	}
	if status, body := get(t, payURL); status != http.StatusOK || !strings.Contains(body, `"amount":"120.0000"`) {
		t.Fatalf("payment after edit: status %d body %s", status, body)
	}
	if status, body := del(t, payURL); status != http.StatusOK {
		t.Fatalf("delete draft payment: status %d (body %s)", status, body)
	}

	// --- stock movement ---------------------------------------------------------

	status, body = postJSON(t, srv.URL+"/api/stock-movements", `{
		"product_id":`+itoa(prodID)+`,"warehouse_id":`+itoa(whID)+`,"movement_type":"receipt",
		"movement_date":"2026-06-18","quantity":"10","unit_cost":"7"}`)
	if status != http.StatusCreated {
		t.Fatalf("create movement: status %d (body %s)", status, body)
	}
	mvID := idOf(body)
	mvURL := srv.URL + "/api/stock-movements/" + itoa(mvID)
	if status, body := putJSON(t, mvURL, `{
		"product_id":`+itoa(prodID)+`,"warehouse_id":`+itoa(whID)+`,"movement_type":"receipt",
		"movement_date":"2026-06-18","quantity":"12","unit_cost":"7"}`); status != http.StatusOK {
		t.Fatalf("edit draft movement: status %d (body %s)", status, body)
	}
	var qty string
	if err := pool.QueryRow(ctx,
		`SELECT quantity::text FROM stock_movements WHERE id = $1`, mvID).Scan(&qty); err != nil {
		t.Fatalf("read movement: %v", err)
	}
	if qty != "12.0000" {
		t.Errorf("movement quantity after edit = %s, want 12.0000", qty)
	}
	if status, body := del(t, mvURL); status != http.StatusOK {
		t.Fatalf("delete draft movement: status %d (body %s)", status, body)
	}
}

// TestOrderLinkedDocumentGuards verifies the fulfilment rules: a confirmed
// order is no longer editable or deletable, a fulfilment invoice cannot be
// edited, and deleting a draft fulfilment invoice returns its quantity to the
// order.
func TestOrderLinkedDocumentGuards(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	resetAuthed(ctx, t, pool)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("setup exec: %v\nsql: %s", err, sql)
		}
	}
	exec(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o`)

	var custID int
	if err := pool.QueryRow(ctx, `SELECT id FROM customers LIMIT 1`).Scan(&custID); err != nil {
		t.Fatalf("customer id: %v", err)
	}

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	idOf := func(body string) int {
		t.Helper()
		var r struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatalf("decode id from %q: %v", body, err)
		}
		return r.ID
	}

	status, body := postJSON(t, srv.URL+"/api/sales-orders", `{
		"order_number":"SO-1","customer_id":`+itoa(custID)+`,"order_date":"2026-06-10","currency_code":"USD",
		"lines":[{"description":"Widget","quantity":"10","unit_price":"25"}]}`)
	if status != http.StatusCreated {
		t.Fatalf("create SO: status %d (body %s)", status, body)
	}
	soID := idOf(body)
	soURL := srv.URL + "/api/sales-orders/" + itoa(soID)

	// Draft orders are editable.
	if status, body := putJSON(t, soURL, `{
		"order_number":"SO-1","customer_id":`+itoa(custID)+`,"order_date":"2026-06-10","currency_code":"USD",
		"lines":[{"description":"Widget","quantity":"8","unit_price":"25"}]}`); status != http.StatusOK {
		t.Fatalf("edit draft SO: status %d (body %s)", status, body)
	}

	// Confirmed orders are not editable or deletable.
	if status, body := post(t, soURL+"/confirm"); status != http.StatusOK {
		t.Fatalf("confirm SO: status %d (body %s)", status, body)
	}
	if status, body := putJSON(t, soURL, `{
		"order_number":"SO-1","customer_id":`+itoa(custID)+`,"order_date":"2026-06-10","currency_code":"USD",
		"lines":[]}`); status != http.StatusConflict {
		t.Fatalf("edit open SO: status %d, want 409 (body %s)", status, body)
	}
	if status, body := del(t, soURL); status != http.StatusConflict {
		t.Fatalf("delete open SO: status %d, want 409 (body %s)", status, body)
	}

	// Fulfil part of the order into a draft invoice.
	var soLine int
	if err := pool.QueryRow(ctx, `SELECT id FROM sales_order_lines WHERE order_id = $1`, soID).Scan(&soLine); err != nil {
		t.Fatalf("order line id: %v", err)
	}
	status, body = postJSON(t, soURL+"/invoice", `{
		"invoice_number":"INV-SO","invoice_date":"2026-06-11",
		"lines":[{"order_line_id":`+itoa(soLine)+`,"quantity":"3"}]}`)
	if status != http.StatusCreated {
		t.Fatalf("invoice SO: status %d (body %s)", status, body)
	}
	var created struct {
		InvoiceID int `json:"invoice_id"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode invoice id from %q: %v", body, err)
	}
	invURL := srv.URL + "/api/sales-invoices/" + itoa(created.InvoiceID)

	// The fulfilment invoice draws down the order, so it cannot be edited...
	if status, body := putJSON(t, invURL, `{
		"invoice_number":"INV-SO","customer_id":`+itoa(custID)+`,"invoice_date":"2026-06-11","currency_code":"USD",
		"lines":[{"description":"Widget","quantity":"999","unit_price":"25"}]}`); status != http.StatusConflict {
		t.Fatalf("edit order-linked invoice: status %d, want 409 (body %s)", status, body)
	}
	if status, body := get(t, soURL); status != http.StatusOK || !strings.Contains(body, `"invoiced_status":"partial"`) {
		t.Fatalf("SO after partial invoice: status %d body %s", status, body)
	}

	// ...but deleting it while draft returns the quantity to the order.
	if status, body := del(t, invURL); status != http.StatusOK {
		t.Fatalf("delete order-linked draft invoice: status %d (body %s)", status, body)
	}
	if status, body := get(t, soURL); status != http.StatusOK || !strings.Contains(body, `"invoiced_status":"none"`) {
		t.Fatalf("SO after invoice delete: status %d body %s", status, body)
	}
}
