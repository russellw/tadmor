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
	"tadmor/internal/mailer"
)

// captureMailer records the last message instead of sending it, standing in
// for a configured transport in tests.
type captureMailer struct{ last *mailer.Message }

func (c *captureMailer) Send(_ context.Context, m mailer.Message) error {
	c.last = &m
	return nil
}

// TestEmailDocumentEndpoint covers both sides of the inert design: with the
// default (no-op) mailer the endpoint reports 501 and sends nothing, and with a
// mailer wired in it renders the document and hands it over as a PDF attachment.
func TestEmailDocumentEndpoint(t *testing.T) {
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
	exec(`INSERT INTO organizations (name, tax_id, is_self) VALUES ('Tadmor Trading', 'TT-1', true)`)
	exec(`INSERT INTO addresses (organization_id, line1, city, country_code)
	      SELECT id, '5 Oasis Rd', 'Palmyra', 'US' FROM organizations WHERE is_self`)
	exec(`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
	      INSERT INTO customers (organization_id, ar_account_id)
	      SELECT o.id, (SELECT id FROM accounts WHERE code='1100') FROM o`)
	exec(`INSERT INTO addresses (organization_id, line1, city, country_code)
	      SELECT organization_id, '1 Main St', 'Springfield', 'US' FROM customers`)
	exec(`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
	      VALUES ('INV/2026 01', (SELECT id FROM customers LIMIT 1), '2026-06-15', 'USD')`)
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      SELECT id, 1, 'Service', 10, 5, (SELECT id FROM accounts WHERE code='4000') FROM sales_invoices`)

	var invoiceID int
	if err := pool.QueryRow(ctx, `SELECT id FROM sales_invoices`).Scan(&invoiceID); err != nil {
		t.Fatalf("scan invoice id: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Default server: the no-op mailer makes the endpoint report "not configured".
	t.Run("inert without mailer", func(t *testing.T) {
		srv := httptest.NewServer(httpapi.NewServer(pool, log).Handler(nil))
		defer srv.Close()
		status, body := postJSON(t, srv.URL+"/api/sales-invoices/"+itoa(invoiceID)+"/email", `{"to":["acme@example.com"]}`)
		if status != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501; body: %s", status, body)
		}
	})

	// With a mailer wired in the same request sends and carries the PDF.
	t.Run("sends with mailer", func(t *testing.T) {
		cap := &captureMailer{}
		srv := httptest.NewServer(httpapi.NewServer(pool, log, httpapi.WithMailer(cap)).Handler(nil))
		defer srv.Close()

		status, body := postJSON(t, srv.URL+"/api/sales-invoices/"+itoa(invoiceID)+"/email", `{"to":["acme@example.com"]}`)
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", status, body)
		}
		if cap.last == nil {
			t.Fatal("mailer received no message")
		}
		if len(cap.last.To) != 1 || cap.last.To[0] != "acme@example.com" {
			t.Errorf("To = %v", cap.last.To)
		}
		if !strings.Contains(cap.last.Subject, "INV/2026 01") {
			t.Errorf("Subject = %q", cap.last.Subject)
		}
		if len(cap.last.Attachments) != 1 {
			t.Fatalf("attachments = %d, want 1", len(cap.last.Attachments))
		}
		a := cap.last.Attachments[0]
		if a.ContentType != "application/pdf" || a.Filename != "invoice-INV-2026-01.pdf" {
			t.Errorf("attachment = %q (%s)", a.Filename, a.ContentType)
		}
		if !strings.HasPrefix(string(a.Data), "%PDF-") {
			t.Errorf("attachment is not a PDF: %.20q", a.Data)
		}

		// A missing document still 404s before any send.
		cap.last = nil
		if status, _ := post(t, srv.URL+"/api/sales-invoices/999999/email"); status != http.StatusNotFound {
			t.Errorf("missing document: status = %d, want 404", status)
		}
		if cap.last != nil {
			t.Error("mailer was called for a missing document")
		}
	})
}
