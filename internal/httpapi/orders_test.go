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

// TestOrderFlowOverHTTP drives a sales order and a purchase order through their
// full lifecycle over the real HTTP surface: create, confirm, fulfil, and read
// the derived fulfilment state back.
func TestOrderFlowOverHTTP(t *testing.T) {
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
	exec(`WITH o AS (INSERT INTO organizations (name) VALUES ('Beta') RETURNING id)
	      INSERT INTO suppliers (organization_id, ap_account_id)
	      SELECT o.id,(SELECT id FROM accounts WHERE code='2000') FROM o`)
	exec(`INSERT INTO products (sku, name, track_inventory, inventory_account_id, cogs_account_id)
	      VALUES ('P-INV','Widget',true,(SELECT id FROM accounts WHERE code='1200'),(SELECT id FROM accounts WHERE code='5000'))`)
	exec(`INSERT INTO warehouses (code, name) VALUES ('MAIN','Main')`)

	scan := func(sql string) int {
		t.Helper()
		var id int
		if err := pool.QueryRow(ctx, sql).Scan(&id); err != nil {
			t.Fatalf("scan: %v\nsql: %s", err, sql)
		}
		return id
	}
	custID := scan(`SELECT id FROM customers LIMIT 1`)
	supID := scan(`SELECT id FROM suppliers LIMIT 1`)
	prodID := scan(`SELECT id FROM products WHERE sku='P-INV'`)
	whID := scan(`SELECT id FROM warehouses WHERE code='MAIN'`)

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

	// --- sales order --------------------------------------------------------

	status, body := postJSON(t, srv.URL+"/api/sales-orders", `{
		"order_number":"SO-1","customer_id":`+strconv.Itoa(custID)+`,"order_date":"2026-06-10","currency_code":"USD",
		"lines":[{"product_id":`+strconv.Itoa(prodID)+`,"description":"Widget","quantity":"10","unit_price":"25"}]}`)
	if status != http.StatusCreated {
		t.Fatalf("create SO: status %d (body %s)", status, body)
	}
	soID := idOf(body)
	soLine := scan(`SELECT id FROM sales_order_lines WHERE order_id = ` + strconv.Itoa(soID))

	// Cannot invoice before confirming.
	if status, body := postJSON(t, srv.URL+"/api/sales-orders/"+strconv.Itoa(soID)+"/invoice",
		`{"invoice_number":"INV-1","invoice_date":"2026-06-11"}`); status != http.StatusConflict {
		t.Fatalf("invoice draft SO: status %d, want 409 (body %s)", status, body)
	}

	if status, body := post(t, srv.URL+"/api/sales-orders/"+strconv.Itoa(soID)+"/confirm"); status != http.StatusOK {
		t.Fatalf("confirm SO: status %d (body %s)", status, body)
	}

	// Invoice 4 of 10.
	status, body = postJSON(t, srv.URL+"/api/sales-orders/"+strconv.Itoa(soID)+"/invoice", `{
		"invoice_number":"INV-1","invoice_date":"2026-06-11",
		"lines":[{"order_line_id":`+strconv.Itoa(soLine)+`,"quantity":"4"}]}`)
	if status != http.StatusCreated {
		t.Fatalf("invoice SO: status %d (body %s)", status, body)
	}
	if status, body := get(t, srv.URL+"/api/sales-orders/"+strconv.Itoa(soID)); status != http.StatusOK ||
		!strings.Contains(body, `"invoiced_status":"partial"`) {
		t.Fatalf("SO after partial invoice: status %d body %s", status, body)
	}

	// Ship everything remaining.
	if status, body := postJSON(t, srv.URL+"/api/sales-orders/"+strconv.Itoa(soID)+"/ship",
		`{"warehouse_id":`+strconv.Itoa(whID)+`}`); status != http.StatusCreated {
		t.Fatalf("ship SO: status %d (body %s)", status, body)
	}
	if status, body := get(t, srv.URL+"/api/sales-orders/"+strconv.Itoa(soID)); status != http.StatusOK ||
		!strings.Contains(body, `"shipped_status":"shipped"`) {
		t.Fatalf("SO after ship: status %d body %s", status, body)
	}

	// --- purchase order -----------------------------------------------------

	status, body = postJSON(t, srv.URL+"/api/purchase-orders", `{
		"order_number":"PO-1","supplier_id":`+strconv.Itoa(supID)+`,"order_date":"2026-06-10","currency_code":"USD",
		"lines":[{"product_id":`+strconv.Itoa(prodID)+`,"description":"Widget","quantity":"10","unit_cost":"7"}]}`)
	if status != http.StatusCreated {
		t.Fatalf("create PO: status %d (body %s)", status, body)
	}
	poID := idOf(body)

	if status, body := post(t, srv.URL+"/api/purchase-orders/"+strconv.Itoa(poID)+"/confirm"); status != http.StatusOK {
		t.Fatalf("confirm PO: status %d (body %s)", status, body)
	}

	// Receive all, into MAIN: one movement for the single stocked line.
	status, body = postJSON(t, srv.URL+"/api/purchase-orders/"+strconv.Itoa(poID)+"/receive",
		`{"warehouse_id":`+strconv.Itoa(whID)+`}`)
	if status != http.StatusCreated {
		t.Fatalf("receive PO: status %d (body %s)", status, body)
	}
	var recv struct {
		MovementIDs []int `json:"movement_ids"`
	}
	if err := json.Unmarshal([]byte(body), &recv); err != nil || len(recv.MovementIDs) != 1 {
		t.Fatalf("receive body %q: %v", body, err)
	}

	// Bill all.
	if status, body := postJSON(t, srv.URL+"/api/purchase-orders/"+strconv.Itoa(poID)+"/bill",
		`{"bill_number":"BILL-1","bill_date":"2026-06-12"}`); status != http.StatusCreated {
		t.Fatalf("bill PO: status %d (body %s)", status, body)
	}
	if status, body := get(t, srv.URL+"/api/purchase-orders/"+strconv.Itoa(poID)); status != http.StatusOK ||
		!strings.Contains(body, `"billed_status":"billed"`) || !strings.Contains(body, `"received_status":"received"`) {
		t.Fatalf("PO after receive+bill: status %d body %s", status, body)
	}
}
