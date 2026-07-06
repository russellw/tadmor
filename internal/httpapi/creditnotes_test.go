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

// TestSalesCreditNoteEndpoints drives a sales credit note over HTTP: create a
// draft, post it, apply it to an open invoice, and read back the applications
// and the invoice's reduced balance.
func TestSalesCreditNoteEndpoints(t *testing.T) {
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

	// A posted invoice for 50 the credit can offset.
	invID := createID("/sales-invoices", `{
		"invoice_number":"INV-1","customer_id":`+strconv.Itoa(custID)+`,"invoice_date":"2026-06-15","currency_code":"USD",
		"lines":[{"description":"Service","quantity":"10","unit_price":"5","revenue_account_id":`+strconv.Itoa(revAcct)+`}]
	}`)
	if s, b := post(t, srv.URL+"/api/sales-invoices/"+strconv.Itoa(invID)+"/post"); s != http.StatusOK {
		t.Fatalf("post invoice: status=%d body=%s", s, b)
	}

	// Create a draft credit note for 20; the trigger computes its total.
	noteID := createID("/sales-credit-notes", `{
		"credit_note_number":"CN-1","customer_id":`+strconv.Itoa(custID)+`,"credit_note_date":"2026-06-16","currency_code":"USD",
		"lines":[{"description":"Returned service","quantity":"4","unit_price":"5","revenue_account_id":`+strconv.Itoa(revAcct)+`}]
	}`)
	noteURL := srv.URL + "/api/sales-credit-notes/" + strconv.Itoa(noteID)
	if s, b := get(t, noteURL); s != http.StatusOK ||
		!strings.Contains(b, `"total":"20.0000"`) || !strings.Contains(b, `"status":"draft"`) ||
		!strings.Contains(b, `"payment_status":"open"`) {
		t.Fatalf("get created credit note: status=%d body=%s", s, b)
	}
	if s, b := get(t, noteURL+"/lines"); s != http.StatusOK || !strings.Contains(b, `"line_total":"20.0000"`) {
		t.Fatalf("get credit note lines: status=%d body=%s", s, b)
	}

	// Applying before posting allocates nothing to worry about here — post
	// first, then apply to the open invoice.
	if s, b := post(t, noteURL+"/post"); s != http.StatusOK {
		t.Fatalf("post credit note: status=%d body=%s", s, b)
	}
	if s, b := post(t, noteURL+"/post"); s != http.StatusConflict {
		t.Fatalf("second post: status=%d, want 409 (body: %s)", s, b)
	}

	status, body := post(t, noteURL+"/apply")
	if status != http.StatusOK || !strings.Contains(body, `"document_id":`+strconv.Itoa(invID)) ||
		!strings.Contains(body, `"amount":"20.0000"`) {
		t.Fatalf("apply credit note: status=%d body=%s", status, body)
	}

	// The applications read back with the invoice number; the invoice balance
	// dropped to 30; the note is fully applied.
	if s, b := get(t, noteURL+"/applications"); s != http.StatusOK ||
		!strings.Contains(b, `"document_number":"INV-1"`) || !strings.Contains(b, `"amount_applied":"20.0000"`) {
		t.Fatalf("get applications: status=%d body=%s", s, b)
	}
	if s, b := get(t, srv.URL+"/api/sales-invoices/"+strconv.Itoa(invID)); s != http.StatusOK ||
		!strings.Contains(b, `"balance":"30.0000"`) || !strings.Contains(b, `"payment_status":"partial"`) {
		t.Fatalf("invoice after credit: status=%d body=%s", s, b)
	}
	if s, b := get(t, noteURL); s != http.StatusOK ||
		!strings.Contains(b, `"balance":"0.0000"`) || !strings.Contains(b, `"payment_status":"applied"`) {
		t.Fatalf("credit note after apply: status=%d body=%s", s, b)
	}

	// Applied notes refuse to unpost (409); the ledger nets to zero throughout.
	if s, b := post(t, noteURL+"/unpost"); s != http.StatusConflict {
		t.Fatalf("unpost applied note: status=%d, want 409 (body: %s)", s, b)
	}
	var sumBalance string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(balance),0)::text FROM trial_balance`).Scan(&sumBalance); err != nil {
		t.Fatalf("sum trial balance: %v", err)
	}
	if sumBalance != "0.0000" {
		t.Fatalf("ledger does not net to zero: %s", sumBalance)
	}
}
