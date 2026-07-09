package printing

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
	"testing"

	"tadmor/internal/pdf"
)

func TestFormatAmount(t *testing.T) {
	for in, want := range map[string]string{
		"0.0000":        "0.00",
		"1234.5000":     "1,234.50",
		"-1234567.8900": "-1,234,567.89",
		"10.1250":       "10.125",
		"999.0000":      "999.00",
		"1000":          "1,000.00",
	} {
		if got := formatAmount(in); got != want {
			t.Errorf("formatAmount(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatQty(t *testing.T) {
	for in, want := range map[string]string{
		"2.0000": "2", "2.5000": "2.5", "10": "10", "0.2500": "0.25",
	} {
		if got := formatQty(in); got != want {
			t.Errorf("formatQty(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncateAndWrap(t *testing.T) {
	long := strings.Repeat("wide text ", 30)
	short := truncate(pdf.Helvetica, 9, 100, long)
	if !strings.HasSuffix(short, "…") || pdf.Width(pdf.Helvetica, 9, short) > 100 {
		t.Errorf("truncate produced %q (width %v)", short, pdf.Width(pdf.Helvetica, 9, short))
	}
	if got := truncate(pdf.Helvetica, 9, 100, "fits"); got != "fits" {
		t.Errorf("truncate mangled a fitting string: %q", got)
	}
	for _, line := range wrap(pdf.Helvetica, 9, 120, long) {
		if pdf.Width(pdf.Helvetica, 9, line) > 120 {
			t.Errorf("wrapped line too wide: %q", line)
		}
	}
}

// extractText decompresses every content stream and returns the concatenation,
// so tests can assert on the strings drawn into the document.
func extractText(t *testing.T, out []byte) string {
	t.Helper()
	var all strings.Builder
	for rest := out; ; {
		start := bytes.Index(rest, []byte(">>\nstream\n"))
		if start < 0 {
			break
		}
		rest = rest[start+len(">>\nstream\n"):]
		end := bytes.Index(rest, []byte("\nendstream"))
		zr, err := zlib.NewReader(bytes.NewReader(rest[:end]))
		if err != nil {
			t.Fatalf("content stream is not zlib: %v", err)
		}
		content, err := io.ReadAll(zr)
		if err != nil {
			t.Fatalf("decompress: %v", err)
		}
		all.Write(content)
		rest = rest[end:]
	}
	return all.String()
}

func strp(s string) *string { return &s }

func TestRenderInvoice(t *testing.T) {
	d := docData{
		kind:     "Invoice",
		number:   "INV-42",
		status:   "posted",
		currency: "USD",
		meta: []metaItem{
			{"Invoice no.", "INV-42"},
			{"Invoice date", "2026-07-01"},
			{"Due date", "2026-07-31"},
			{"Currency", "USD"},
		},
		partyLabel: "BILL TO",
		party: partyBlock{
			name:    "Acme Corp",
			legal:   strp("Acme Corporation Ltd"),
			taxID:   strp("US12-3456789"),
			address: []string{"1 Main St", "Springfield, IL 62701", "United States"},
		},
		seller: &partyBlock{
			name:    "Tadmor Trading",
			address: []string{"5 Oasis Rd", "Palmyra"},
		},
		unitLabel:    "UNIT PRICE",
		subtotal:     "1500.0000",
		taxTotal:     "123.7500",
		total:        "1623.7500",
		applied:      "600.0000",
		balance:      "1023.7500",
		appliedLabel: "Amount paid",
		balanceLabel: "Balance due",
		ref:          strp("PO-777"),
		memo:         strp("Thank you for your business."),
	}
	for i := 1; i <= 3; i++ {
		d.lines = append(d.lines, docLine{
			no: i, desc: fmt.Sprintf("Item %d", i),
			qty: "2.0000", unit: "250.0000", taxRate: "8.2500", subtotal: "500.0000",
		})
	}

	out := renderDoc(d)
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("output is not a PDF")
	}
	text := extractText(t, out)
	for _, want := range []string{
		"INVOICE", "INV-42", "2026-07-31",
		"Tadmor Trading", "Acme Corp", "US12-3456789",
		"UNIT PRICE", "BILL TO",
		"Item 3", "1,500.00", "USD 1,623.75", "Amount paid", "USD 1,023.75",
		"Reference: PO-777", "Thank you for your business.",
		"Page 1 of 1",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered invoice is missing %q", want)
		}
	}

	// A draft is labelled as such, and unpaid invoices show no payment rows.
	d.status = "draft"
	d.applied = "0.0000"
	text = extractText(t, renderDoc(d))
	if !strings.Contains(text, "DRAFT INVOICE") {
		t.Error("draft invoice not labelled DRAFT")
	}
	if strings.Contains(text, "Amount paid") {
		t.Error("unpaid invoice shows an Amount paid row")
	}

	// Enough lines to spill onto a second page repeats the table header.
	for i := 4; i <= 60; i++ {
		d.lines = append(d.lines, docLine{
			no: i, desc: fmt.Sprintf("Item %d", i),
			qty: "1.0000", unit: "10.0000", taxRate: "0.0000", subtotal: "10.0000",
		})
	}
	text = extractText(t, renderDoc(d))
	if !strings.Contains(text, "Page 2 of") {
		t.Error("long invoice did not paginate")
	}
	if strings.Count(text, "DESCRIPTION") < 2 {
		t.Errorf("table header should repeat on the next page, found %d", strings.Count(text, "DESCRIPTION"))
	}
}

// TestRenderOrder covers what orders do differently: no application rows even
// with the fields unset, the purchasing unit label, and the cancelled title.
func TestRenderOrder(t *testing.T) {
	d := docData{
		kind:     "Purchase Order",
		number:   "PO-7",
		status:   "open",
		currency: "EUR",
		meta: []metaItem{
			{"Order no.", "PO-7"},
			{"Order date", "2026-07-01"},
			{"Expected receipt", "2026-07-20"},
			{"Currency", "EUR"},
		},
		partyLabel: "SUPPLIER",
		party:      partyBlock{name: "Beta GmbH"},
		unitLabel:  "UNIT COST",
		subtotal:   "100.0000",
		taxTotal:   "0.0000",
		total:      "100.0000",
		lines: []docLine{
			{no: 1, desc: "Widget", qty: "4.0000", unit: "25.0000", taxRate: "0.0000", subtotal: "100.0000"},
		},
	}

	text := extractText(t, renderDoc(d))
	for _, want := range []string{
		"PURCHASE ORDER", "PO-7", "Expected receipt", "2026-07-20",
		"SUPPLIER", "Beta GmbH", "UNIT COST", "EUR 100.00",
		"Purchase Order PO-7", // footer
	} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered order is missing %q", want)
		}
	}
	if strings.Contains(text, "Amount paid") || strings.Contains(text, "Balance due") {
		t.Error("order shows application rows")
	}
	if strings.Contains(text, "OPEN") {
		t.Error("open order title should carry no status prefix")
	}

	d.status = "cancelled"
	if text := extractText(t, renderDoc(d)); !strings.Contains(text, "CANCELLED PURCHASE ORDER") {
		t.Error("cancelled order not labelled CANCELLED")
	}
}

// TestRenderCreditNote checks the credit-note application labels.
func TestRenderCreditNote(t *testing.T) {
	d := docData{
		kind:         "Credit Note",
		number:       "CN-1",
		status:       "posted",
		currency:     "USD",
		meta:         []metaItem{{"Credit note no.", "CN-1"}},
		partyLabel:   "CREDIT TO",
		party:        partyBlock{name: "Acme Corp"},
		unitLabel:    "UNIT PRICE",
		subtotal:     "50.0000",
		taxTotal:     "0.0000",
		total:        "50.0000",
		applied:      "20.0000",
		balance:      "30.0000",
		appliedLabel: "Amount applied",
		balanceLabel: "Unapplied",
		lines: []docLine{
			{no: 1, desc: "Returned goods", qty: "1.0000", unit: "50.0000", taxRate: "0.0000", subtotal: "50.0000"},
		},
	}

	text := extractText(t, renderDoc(d))
	for _, want := range []string{
		"CREDIT NOTE", "CREDIT TO", "Amount applied", "Unapplied", "USD 30.00",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered credit note is missing %q", want)
		}
	}
}
