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

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	invURL := srv.URL + "/api/sales-invoices/" + strconv.Itoa(invID) + "/post"

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
	if status, body := post(t, srv.URL+"/api/sales-invoices/999999/post"); status != http.StatusNotFound {
		t.Fatalf("missing invoice: status = %d, want 404 (body: %s)", status, body)
	}

	// Non-numeric id is a 400.
	if status, body := post(t, srv.URL+"/api/sales-invoices/abc/post"); status != http.StatusBadRequest {
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
		`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, movement_date, quantity, unit_cost)
		 VALUES ((SELECT id FROM products WHERE sku='P-INV'), (SELECT id FROM warehouses WHERE code='MAIN'), 'receipt', '2026-06-16', 10, 7)
		 RETURNING id`).Scan(&movID); err != nil {
		t.Fatalf("create movement: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM accounts WHERE code='2150'`).Scan(&grni); err != nil {
		t.Fatalf("grni account: %v", err)
	}

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()
	url := srv.URL + "/api/stock-movements/" + strconv.Itoa(movID) + "/post"

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

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()
	base := srv.URL + "/api/sales-invoices/" + strconv.Itoa(invID)

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

func TestReadEndpoints(t *testing.T) {
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

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	if status, body := post(t, srv.URL+"/api/sales-invoices/"+strconv.Itoa(invID)+"/post"); status != http.StatusOK {
		t.Fatalf("post: status = %d (body: %s)", status, body)
	}

	// GET the single invoice.
	status, body := get(t, srv.URL+"/api/sales-invoices/"+strconv.Itoa(invID))
	if status != http.StatusOK {
		t.Fatalf("get invoice: status = %d (body: %s)", status, body)
	}
	var inv struct {
		Total         string `json:"total"`
		Balance       string `json:"balance"`
		PaymentStatus string `json:"payment_status"`
	}
	if err := json.Unmarshal([]byte(body), &inv); err != nil {
		t.Fatalf("decode invoice %q: %v", body, err)
	}
	if inv.Total != "50.0000" || inv.Balance != "50.0000" || inv.PaymentStatus != "unpaid" {
		t.Errorf("invoice = %+v, want 50.0000/50.0000/unpaid", inv)
	}

	// GET the trial balance (a JSON array).
	if status, body := get(t, srv.URL+"/api/trial-balance"); status != http.StatusOK || !strings.Contains(body, `"code":"1100"`) {
		t.Fatalf("trial-balance: status = %d, body = %s", status, body)
	}

	// Unknown invoice -> 404.
	if status, body := get(t, srv.URL+"/api/sales-invoices/999999"); status != http.StatusNotFound {
		t.Fatalf("missing invoice: status = %d, want 404 (body: %s)", status, body)
	}
}

func TestCreateThenPostSalesInvoiceEndpoint(t *testing.T) {
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
	var custID, revAcct int
	if err := pool.QueryRow(ctx,
		`WITH o AS (INSERT INTO organizations (name) VALUES ('Acme') RETURNING id)
		 INSERT INTO customers (organization_id, ar_account_id)
		 SELECT id, (SELECT id FROM accounts WHERE code='1100') FROM o RETURNING id`).Scan(&custID); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM accounts WHERE code='4000'`).Scan(&revAcct); err != nil {
		t.Fatalf("revenue account: %v", err)
	}

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	body := `{
		"invoice_number": "INV-1",
		"customer_id": ` + strconv.Itoa(custID) + `,
		"invoice_date": "2026-06-15",
		"currency_code": "USD",
		"lines": [
			{"description": "Service", "quantity": "10", "unit_price": "5", "revenue_account_id": ` + strconv.Itoa(revAcct) + `}
		]
	}`

	// Create returns 201 with the new id.
	status, respBody := postJSON(t, srv.URL+"/api/sales-invoices", body)
	if status != http.StatusCreated {
		t.Fatalf("create: status = %d, want 201 (body: %s)", status, respBody)
	}
	var created struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal([]byte(respBody), &created); err != nil || created.ID <= 0 {
		t.Fatalf("decode %q: %v / id=%d", respBody, err, created.ID)
	}
	idStr := strconv.Itoa(created.ID)

	// It is fetchable, draft, with a trigger-computed total of 50.
	if s, b := get(t, srv.URL+"/api/sales-invoices/"+idStr); s != http.StatusOK || !strings.Contains(b, `"total":"50.0000"`) || !strings.Contains(b, `"status":"draft"`) {
		t.Fatalf("get created invoice: status=%d body=%s", s, b)
	}

	// And it can be posted via the existing endpoint.
	if s, b := post(t, srv.URL+"/api/sales-invoices/"+idStr+"/post"); s != http.StatusOK {
		t.Fatalf("post created invoice: status=%d body=%s", s, b)
	}

	// A duplicate invoice number conflicts (409).
	if s, b := postJSON(t, srv.URL+"/api/sales-invoices", body); s != http.StatusConflict {
		t.Fatalf("duplicate create: status = %d, want 409 (body: %s)", s, b)
	}

	// A missing required field is a 400.
	if s, _ := postJSON(t, srv.URL+"/api/sales-invoices", `{"customer_id": `+strconv.Itoa(custID)+`}`); s != http.StatusBadRequest {
		t.Fatalf("invalid create: status = %d, want 400", s)
	}

	// A nonexistent customer fails the foreign key (422).
	bad := `{"invoice_number":"INV-X","customer_id":999999,"invoice_date":"2026-06-15","currency_code":"USD"}`
	if s, _ := postJSON(t, srv.URL+"/api/sales-invoices", bad); s != http.StatusUnprocessableEntity {
		t.Fatalf("bad customer: status = %d, want 422", s)
	}
}

// TestFullFlowOverHTTP builds master data and drives a document through posting
// entirely over HTTP — no SQL beyond the schema reset — proving the API is
// self-sufficient.
func TestFullFlowOverHTTP(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := dbtest.Acquire(ctx, t)
	defer cleanup()
	dbtest.Reset(ctx, t, pool)

	srv := httptest.NewServer(httpapi.NewServer(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler(nil))
	defer srv.Close()

	// Discover seeded account ids by code via the accounts list endpoint.
	_, accountsBody := get(t, srv.URL+"/api/accounts")
	var accounts []struct {
		ID   int    `json:"id"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(accountsBody), &accounts); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	acct := map[string]int{}
	for _, a := range accounts {
		acct[a.Code] = a.ID
	}
	if acct["1100"] == 0 || acct["4000"] == 0 {
		t.Fatalf("seeded accounts missing: %v", acct)
	}

	// createID POSTs a body and returns the new record id.
	createID := func(path, body string) int {
		t.Helper()
		status, respBody := postJSON(t, srv.URL+"/api"+path, body)
		if status != http.StatusCreated {
			t.Fatalf("POST %s: status = %d (body: %s)", path, status, respBody)
		}
		var created struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal([]byte(respBody), &created); err != nil || created.ID <= 0 {
			t.Fatalf("POST %s: decode %q: %v", path, respBody, err)
		}
		return created.ID
	}

	orgID := createID("/organizations", `{"name":"Acme Corp"}`)
	custID := createID("/customers", `{"organization_id":`+itoa(orgID)+`,"ar_account_id":`+itoa(acct["1100"])+`}`)
	prodID := createID("/products", `{"sku":"WIDGET","name":"Widget","unit_price":"5","revenue_account_id":`+itoa(acct["4000"])+`}`)
	createID("/fiscal-years", `{"name":"FY2026","start_date":"2026-01-01","end_date":"2026-12-31"}`)
	createID("/accounting-periods", `{"fiscal_year_id":1,"name":"2026-06","start_date":"2026-06-01","end_date":"2026-06-30"}`)

	// Update the product (PUT full replace) and confirm via GET.
	if status, body := putJSON(t, srv.URL+"/api/products/"+itoa(prodID),
		`{"sku":"WIDGET","name":"Widget v2","unit_price":"6","revenue_account_id":`+itoa(acct["4000"])+`,"is_active":true}`); status != http.StatusNoContent {
		t.Fatalf("PUT product: status = %d (body: %s)", status, body)
	}
	if _, body := get(t, srv.URL+"/api/products/"+itoa(prodID)); !strings.Contains(body, `"name":"Widget v2"`) || !strings.Contains(body, `"unit_price":"6.0000"`) {
		t.Fatalf("product after update: %s", body)
	}

	// Create an invoice referencing the product, then post it.
	invID := createID("/sales-invoices", `{
		"invoice_number":"INV-1","customer_id":`+itoa(custID)+`,"invoice_date":"2026-06-15","currency_code":"USD",
		"lines":[{"product_id":`+itoa(prodID)+`,"description":"Widget","quantity":"10","unit_price":"5","revenue_account_id":`+itoa(acct["4000"])+`}]
	}`)
	if status, body := post(t, srv.URL+"/api/sales-invoices/"+itoa(invID)+"/post"); status != http.StatusOK {
		t.Fatalf("post invoice: status = %d (body: %s)", status, body)
	}

	// The invoice is posted with a balance of 50, and it shows in the trial balance.
	if _, body := get(t, srv.URL+"/api/sales-invoices/"+itoa(invID)); !strings.Contains(body, `"status":"posted"`) || !strings.Contains(body, `"balance":"50.0000"`) {
		t.Fatalf("posted invoice: %s", body)
	}
	if _, body := get(t, srv.URL+"/api/trial-balance"); !strings.Contains(body, `"code":"1100"`) || !strings.Contains(body, `"balance":"50.0000"`) {
		t.Fatalf("trial balance: %s", body)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// get issues a GET and returns the status and body.
func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// putJSON issues a PUT with the given JSON body and returns the status and body.
func putJSON(t *testing.T, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new PUT %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
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
