package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"tadmor/internal/dbtest"
	"tadmor/internal/httpapi"
)

func TestPostSalesInvoiceEndpoint(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()

	// Start from a clean ledger (safe: the advisory lock serializes DB tests).
	dbtest.Reset(ctx, t, pool)

	// A draft invoice the endpoint can post. Committed so the handler's own
	// transaction sees it.
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

	var invID int
	if err := pool.QueryRow(ctx,
		`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
		 VALUES ('INV-1', (SELECT id FROM customers LIMIT 1), '2026-06-15', 'USD') RETURNING id`).Scan(&invID); err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      VALUES ($1, 1, 'Service', 10, 5, (SELECT id FROM accounts WHERE code='4000'))`, invID)

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer srv.Close()

	invURL := srv.URL + "/sales-invoices/" + strconv.Itoa(invID) + "/post"

	// First post succeeds and returns a journal-entry id.
	status, body := post(t, invURL)
	if status != http.StatusOK {
		t.Fatalf("first post: status = %d, want 200 (body: %s)", status, body)
	}
	var ok struct {
		JournalEntryID int `json:"journal_entry_id"`
	}
	if err := json.Unmarshal([]byte(body), &ok); err != nil {
		t.Fatalf("decode body %q: %v", body, err)
	}
	if ok.JournalEntryID <= 0 {
		t.Fatalf("expected a journal_entry_id, got %d", ok.JournalEntryID)
	}

	// Posting again conflicts (no longer draft).
	if status, body := post(t, invURL); status != http.StatusConflict {
		t.Fatalf("second post: status = %d, want 409 (body: %s)", status, body)
	}

	// Unknown invoice is a 404.
	if status, body := post(t, srv.URL+"/sales-invoices/999999/post"); status != http.StatusNotFound {
		t.Fatalf("missing invoice: status = %d, want 404 (body: %s)", status, body)
	}

	// Non-numeric id is a 400.
	if status, body := post(t, srv.URL+"/sales-invoices/abc/post"); status != http.StatusBadRequest {
		t.Fatalf("bad id: status = %d, want 400 (body: %s)", status, body)
	}

	// The committed posting balanced the ledger.
	var sumBalance string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(balance),0)::text FROM trial_balance`).Scan(&sumBalance); err != nil {
		t.Fatalf("sum trial balance: %v", err)
	}
	if sumBalance != "0.0000" {
		t.Fatalf("ledger does not net to zero after posting: %s", sumBalance)
	}
}

func TestPostStockMovementReceiptEndpoint(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("setup exec: %v\nsql: %s", err, sql)
		}
	}
	exec(`INSERT INTO fiscal_years (name, start_date, end_date) VALUES ('FY2026','2026-01-01','2026-12-31')`)
	exec(`INSERT INTO accounting_periods (fiscal_year_id, name, start_date, end_date)
	      SELECT id,'2026-06','2026-06-01','2026-06-30' FROM fiscal_years WHERE name='FY2026'`)
	exec(`INSERT INTO products (sku, name, track_inventory, inventory_account_id, cogs_account_id)
	      VALUES ('P-INV','Widget',true,(SELECT id FROM accounts WHERE code='1200'),(SELECT id FROM accounts WHERE code='5000'))`)
	exec(`INSERT INTO warehouses (code, name) VALUES ('MAIN','Main')`)

	var movID, grni int
	if err := pool.QueryRow(ctx,
		`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, quantity, unit_cost)
		 VALUES ((SELECT id FROM products WHERE sku='P-INV'), (SELECT id FROM warehouses WHERE code='MAIN'), 'receipt', 10, 7)
		 RETURNING id`).Scan(&movID); err != nil {
		t.Fatalf("create movement: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM accounts WHERE code='2150'`).Scan(&grni); err != nil {
		t.Fatalf("grni account: %v", err)
	}

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer srv.Close()
	url := srv.URL + "/stock-movements/" + strconv.Itoa(movID) + "/post"

	// Missing credit_account_id -> 422 (a receipt needs a clearing account).
	if status, body := postJSON(t, url, `{"currency":"USD"}`); status != http.StatusUnprocessableEntity {
		t.Fatalf("receipt without credit account: status = %d, want 422 (body: %s)", status, body)
	}

	// With the GRNI account it posts.
	status, body := postJSON(t, url, `{"currency":"USD","credit_account_id":`+strconv.Itoa(grni)+`}`)
	if status != http.StatusOK {
		t.Fatalf("receipt post: status = %d, want 200 (body: %s)", status, body)
	}
	var ok struct {
		JournalEntryID int `json:"journal_entry_id"`
	}
	if err := json.Unmarshal([]byte(body), &ok); err != nil || ok.JournalEntryID <= 0 {
		t.Fatalf("decode %q: %v / id=%d", body, err, ok.JournalEntryID)
	}

	var inv string
	if err := pool.QueryRow(ctx, `SELECT balance::text FROM trial_balance WHERE code='1200'`).Scan(&inv); err != nil {
		t.Fatalf("inventory balance: %v", err)
	}
	if inv != "70.0000" {
		t.Fatalf("inventory balance = %s, want 70.0000", inv)
	}
}

func TestUnpostSalesInvoiceEndpoint(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

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
	      SELECT o.id,(SELECT id FROM accounts WHERE code='1100') FROM o`)
	var invID int
	if err := pool.QueryRow(ctx,
		`INSERT INTO sales_invoices (invoice_number, customer_id, invoice_date, currency_code)
		 VALUES ('INV-1', (SELECT id FROM customers LIMIT 1), '2026-06-15', 'USD') RETURNING id`).Scan(&invID); err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	exec(`INSERT INTO sales_invoice_lines (invoice_id, line_no, description, quantity, unit_price, revenue_account_id)
	      VALUES ($1, 1, 'Service', 10, 5, (SELECT id FROM accounts WHERE code='4000'))`, invID)

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer srv.Close()
	base := srv.URL + "/sales-invoices/" + strconv.Itoa(invID)

	if status, body := post(t, base+"/post"); status != http.StatusOK {
		t.Fatalf("post: status = %d (body: %s)", status, body)
	}

	// Unpost returns the reversing journal entry id.
	status, body := post(t, base+"/unpost")
	if status != http.StatusOK {
		t.Fatalf("unpost: status = %d, want 200 (body: %s)", status, body)
	}
	var got struct {
		ReversalEntryID int `json:"reversal_entry_id"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.ReversalEntryID <= 0 {
		t.Fatalf("decode %q: %v / id=%d", body, err, got.ReversalEntryID)
	}

	// Invoice is draft again, and a second unpost conflicts.
	var ds string
	if err := pool.QueryRow(ctx, `SELECT status FROM sales_invoices WHERE id=$1`, invID).Scan(&ds); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if ds != "draft" {
		t.Fatalf("invoice status = %s, want draft", ds)
	}
	if status, body := post(t, base+"/unpost"); status != http.StatusConflict {
		t.Fatalf("second unpost: status = %d, want 409 (body: %s)", status, body)
	}
}

// post issues a POST with an empty JSON body and returns the status and body.
func post(t *testing.T, url string) (int, string) {
	t.Helper()
	return postJSON(t, url, "")
}

// postJSON issues a POST with the given JSON body and returns the status and body.
func postJSON(t *testing.T, url, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	resp, err := http.Post(url, "application/json", r)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
