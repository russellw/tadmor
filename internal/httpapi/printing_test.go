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

func TestSalesInvoicePDFEndpoint(t *testing.T) {
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
	// Our own company (the invoice issuer) with an address...
	exec(`INSERT INTO organizations (name, tax_id, is_self) VALUES ('Tadmor Trading', 'TT-1', true)`)
	exec(`INSERT INTO addresses (organization_id, line1, city, country_code)
	      SELECT id, '5 Oasis Rd', 'Palmyra', 'US' FROM organizations WHERE is_self`)
	// ...and a customer with an invoice.
	exec(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='1100') FROM o`)
	var invID int
	if err := pool.QueryRow(ctx,
		`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
		 VALUES ('INV/2026 01', (SELECT id FROM customers LIMIT 1), '2026-06-15', 'USD') RETURNING id`).Scan(&invID); err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      VALUES ($1, 1, 'Service', 10, 5, (SELECT id FROM accounts WHERE code='4000'))`, invID)

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/sales-invoices/"+itoa(invID)+"/pdf", nil)
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
	// The slash and space in the invoice number must not reach the filename.
	if cd := resp.Header.Get("Content-Disposition"); cd != `inline; filename="invoice-INV-2026-01.pdf"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if !strings.HasPrefix(string(body), "%PDF-") {
		t.Errorf("body is not a PDF: %.20q", body)
	}

	if status, _ := get(t, srv.URL+"/api/sales-invoices/999999/pdf"); status != http.StatusNotFound {
		t.Errorf("missing invoice: status = %d, want 404", status)
	}
}
