package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/httpapi"
)

// TestDocumentPDFEndpoints creates one of each printable document and fetches
// its PDF over HTTP, which also proves each document type's header, address,
// and line queries against the real schema.
func TestDocumentPDFEndpoints(t *testing.T) {
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
	// Our own company (the issuer) with an address...
	exec(`INSERT INTO organizations (name, tax_id, is_self) VALUES ('Tadmor Trading', 'TT-1', true)`)
	exec(`INSERT INTO addresses (organization_id, line1, city, country_code)
	      SELECT id, '5 Oasis Rd', 'Palmyra', 'US' FROM organizations WHERE is_self`)
	// ...a customer and a supplier, each with an address.
	exec(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='1100') FROM o`)
	exec(`INSERT INTO addresses (organization_id, line1, city, country_code)
	      SELECT organization_id, '1 Main St', 'Springfield', 'US' FROM customers`)
	exec(`WITH o AS (INSERT INTO organizations (name) VALUES ('Beta') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='2000') FROM o`)
	exec(`INSERT INTO addresses (organization_id, line1, city, country_code)
	      SELECT organization_id, '2 Werkstrasse', 'Hamburg', 'DE' FROM suppliers`)

	// One of each document, with a line.
	exec(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
	      VALUES ('INV/2026 01', (SELECT id FROM customers LIMIT 1), '2026-06-15', 'USD')`)
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      SELECT id, 1, 'Service', 10, 5, (SELECT id FROM accounts WHERE code='4000') FROM sales_invoices`)
	exec(`INSERT INTO purchase_bills (bill_number, supplier_id, bill_date, currency_code)
	      VALUES ('BILL-7', (SELECT id FROM suppliers LIMIT 1), '2026-06-16', 'USD')`)
	exec(`INSERT INTO purchase_bill_lines (bill_id, line_no, description, quantity, unit_cost, expense_account_id)
	      SELECT id, 1, 'Parts', 4, 25, (SELECT id FROM accounts WHERE code='6000') FROM purchase_bills`)
	exec(`INSERT INTO sales_credit_notes (credit_note_number, customer_id, credit_note_date, currency_code)
	      VALUES ('CN-1', (SELECT id FROM customers LIMIT 1), '2026-06-17', 'USD')`)
	exec(`INSERT INTO sales_credit_note_lines (credit_note_id, line_no, description, quantity, unit_price, revenue_account_id)
	      SELECT id, 1, 'Returned goods', 1, 50, (SELECT id FROM accounts WHERE code='4000') FROM sales_credit_notes`)
	exec(`INSERT INTO purchase_credit_notes (credit_note_number, supplier_id, credit_note_date, currency_code)
	      VALUES ('SCN-1', (SELECT id FROM suppliers LIMIT 1), '2026-06-18', 'USD')`)
	exec(`INSERT INTO purchase_credit_note_lines (credit_note_id, line_no, description, quantity, unit_cost, expense_account_id)
	      SELECT id, 1, 'Damaged parts', 1, 25, (SELECT id FROM accounts WHERE code='6000') FROM purchase_credit_notes`)
	exec(`INSERT INTO sales_orders (order_number, customer_id, order_date, expected_ship_date, currency_code)
	      VALUES ('SO-1', (SELECT id FROM customers LIMIT 1), '2026-06-19', '2026-06-30', 'USD')`)
	exec(`INSERT INTO sales_order_lines (order_id, line_no, description, quantity, unit_price)
	      SELECT id, 1, 'Widget', 3, 20 FROM sales_orders`)
	exec(`INSERT INTO purchase_orders (order_number, supplier_id, order_date, currency_code)
	      VALUES ('PO-1', (SELECT id FROM suppliers LIMIT 1), '2026-06-20', 'USD')`)
	exec(`INSERT INTO purchase_order_lines (order_id, line_no, description, quantity, unit_cost)
	      SELECT id, 1, 'Widget', 3, 15 FROM purchase_orders`)

	scan := func(sql string) int {
		t.Helper()
		var id int
		if err := pool.QueryRow(ctx, sql).Scan(&id); err != nil {
			t.Fatalf("scan: %v\nsql: %s", err, sql)
		}
		return id
	}

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	cases := []struct {
		name     string
		path     string // API collection the document lives under
		id       int
		filename string
	}{
		// The slash and space in the invoice number must not reach the filename.
		{"invoice", "sales-invoices", scan(`SELECT id FROM sales_invoices`), "invoice-INV-2026-01.pdf"},
		{"bill", "purchase-bills", scan(`SELECT id FROM purchase_bills`), "bill-BILL-7.pdf"},
		{"credit note", "sales-credit-notes", scan(`SELECT id FROM sales_credit_notes`), "credit-note-CN-1.pdf"},
		{"supplier credit", "purchase-credit-notes", scan(`SELECT id FROM purchase_credit_notes`), "supplier-credit-SCN-1.pdf"},
		{"sales order", "sales-orders", scan(`SELECT id FROM sales_orders`), "sales-order-SO-1.pdf"},
		{"purchase order", "purchase-orders", scan(`SELECT id FROM purchase_orders`), "purchase-order-PO-1.pdf"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/"+c.path+"/"+itoa(c.id)+"/pdf", nil)
			req.AddCookie(&http.Cookie{Name: "tadmor_session", Value: testToken})
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET pdf: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, body: %s", resp.StatusCode, body)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
				t.Errorf("Content-Type = %q", ct)
			}
			if cd := resp.Header.Get("Content-Disposition"); cd != `inline; filename="`+c.filename+`"` {
				t.Errorf("Content-Disposition = %q, want filename %q", cd, c.filename)
			}
			if !strings.HasPrefix(string(body), "%PDF-") {
				t.Errorf("body is not a PDF: %.20q", body)
			}

			if status, _ := get(t, srv.URL+"/api/"+c.path+"/999999/pdf"); status != http.StatusNotFound {
				t.Errorf("missing document: status = %d, want 404", status)
			}
		})
	}
}
