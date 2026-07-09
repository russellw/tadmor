package printing

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
	"testing"

	"tadmor/internal/pdf"
	"tadmor/internal/reporting"
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
	d := invoiceData{
		number:   "INV-42",
		date:     "2026-07-01",
		dueDate:  strp("2026-07-31"),
		currency: "USD",
		status:   "posted",
		subtotal: "1500.0000",
		taxTotal: "123.7500",
		total:    "1623.7500",
		applied:  "600.0000",
		balance:  "1023.7500",
		ref:      strp("PO-777"),
		memo:     strp("Thank you for your business."),
		customer: partyBlock{
			name:    "Acme Corp",
			legal:   strp("Acme Corporation Ltd"),
			taxID:   strp("US12-3456789"),
			address: []string{"1 Main St", "Springfield, IL 62701", "United States"},
		},
		seller: &partyBlock{
			name:    "Tadmor Trading",
			address: []string{"5 Oasis Rd", "Palmyra"},
		},
	}
	for i := 1; i <= 3; i++ {
		d.lines = append(d.lines, reporting.SalesInvoiceLine{
			LineNo: i, Description: fmt.Sprintf("Item %d", i),
			Quantity: "2.0000", UnitPrice: "250.0000", TaxRate: "8.2500",
			LineSubtotal: "500.0000", TaxAmount: "41.2500", LineTotal: "541.2500",
		})
	}

	out := renderInvoice(d)
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("output is not a PDF")
	}
	text := extractText(t, out)
	for _, want := range []string{
		"INVOICE", "INV-42", "2026-07-31",
		"Tadmor Trading", "Acme Corp", "US12-3456789",
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
	text = extractText(t, renderInvoice(d))
	if !strings.Contains(text, "DRAFT INVOICE") {
		t.Error("draft invoice not labelled DRAFT")
	}
	if strings.Contains(text, "Amount paid") {
		t.Error("unpaid invoice shows an Amount paid row")
	}

	// Enough lines to spill onto a second page repeats the table header.
	for i := 4; i <= 60; i++ {
		d.lines = append(d.lines, reporting.SalesInvoiceLine{
			LineNo: i, Description: fmt.Sprintf("Item %d", i),
			Quantity: "1.0000", UnitPrice: "10.0000", TaxRate: "0.0000",
			LineSubtotal: "10.0000", TaxAmount: "0.0000", LineTotal: "10.0000",
		})
	}
	text = extractText(t, renderInvoice(d))
	if !strings.Contains(text, "Page 2 of") {
		t.Error("long invoice did not paginate")
	}
	if strings.Count(text, "DESCRIPTION") < 2 {
		t.Errorf("table header should repeat on the next page, found %d", strings.Count(text, "DESCRIPTION"))
	}
}
