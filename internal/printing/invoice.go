// Package printing renders documents as PDFs for sending outside the system.
// It reads with the same Querier convention as reporting and lays pages out
// with the internal/pdf writer.
package printing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"tadmor/internal/pdf"
	"tadmor/internal/reporting"
)

// A4 in points.
const (
	pageW = 595.28
	pageH = 841.89

	marginL = 54.0
	marginR = 54.0
	rightX  = pageW - marginR
	topY    = pageH - 54
	bottomY = 72.0 // content stops here; the footer sits below
)

// Table column geometry: description is left-aligned from descX and truncated
// to fit; the numeric columns are right-aligned at their x.
const (
	numX    = marginL
	descX   = marginL + 26
	qtyX    = 360.0
	priceX  = 432.0
	taxX    = 472.0
	amountX = rightX
	descMax = qtyX - 60 - descX // keep clear of the Qty column
)

// partyBlock is one side of the invoice: an organization and its address.
type partyBlock struct {
	name    string
	legal   *string
	taxID   *string
	address []string
}

// invoiceData is everything the layout needs, fetched up front.
type invoiceData struct {
	number   string
	date     string
	dueDate  *string
	currency string
	status   string
	subtotal string
	taxTotal string
	total    string
	applied  string
	balance  string
	ref      *string
	memo     *string
	customer partyBlock
	seller   *partyBlock // nil until an organization is marked is_self
	lines    []reporting.SalesInvoiceLine
}

// SalesInvoicePDF renders the invoice as a PDF, returning the bytes and the
// invoice number (for the download filename). It returns
// reporting.ErrNotFound when the invoice does not exist.
func SalesInvoicePDF(ctx context.Context, q reporting.Querier, invoiceID int) ([]byte, string, error) {
	d, err := fetchInvoice(ctx, q, invoiceID)
	if err != nil {
		return nil, "", err
	}
	return renderInvoice(d), d.number, nil
}

func fetchInvoice(ctx context.Context, q reporting.Querier, invoiceID int) (invoiceData, error) {
	var d invoiceData
	err := q.QueryRow(ctx,
		`SELECT si.invoice_number, si.invoice_date::text, si.due_date::text, si.currency_code,
		        si.status,
		        si.subtotal::numeric(19,4)::text, si.tax_total::numeric(19,4)::text, si.total::numeric(19,4)::text,
		        b.amount_applied::numeric(19,4)::text, b.balance::numeric(19,4)::text,
		        si.reference, si.memo,
		        o.name, o.legal_name, o.tax_id
		 FROM sales_invoices si
		 JOIN customers c     ON c.id = si.customer_id
		 JOIN organizations o ON o.id = c.organization_id
		 JOIN sales_invoice_balances b ON b.invoice_id = si.id
		 WHERE si.id = $1`, invoiceID).Scan(
		&d.number, &d.date, &d.dueDate, &d.currency, &d.status,
		&d.subtotal, &d.taxTotal, &d.total, &d.applied, &d.balance,
		&d.ref, &d.memo,
		&d.customer.name, &d.customer.legal, &d.customer.taxID)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, reporting.ErrNotFound
	}
	if err != nil {
		return d, err
	}

	// Billing address: the invoice's explicit one, else the customer
	// organization's first. Its absence is not an error.
	var line1, line2, city, region, postal, country *string
	err = q.QueryRow(ctx,
		`SELECT a.line1, a.line2, a.city, a.region, a.postal_code, co.name
		 FROM sales_invoices si
		 JOIN customers c ON c.id = si.customer_id
		 JOIN addresses a ON a.id = COALESCE(si.billing_address_id,
		     (SELECT id FROM addresses WHERE organization_id = c.organization_id ORDER BY id LIMIT 1))
		 LEFT JOIN countries co ON co.code = a.country_code
		 WHERE si.id = $1`, invoiceID).Scan(&line1, &line2, &city, &region, &postal, &country)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return d, err
	}
	d.customer.address = addressLines(line1, line2, city, region, postal, country)

	// The issuer: whichever organization is flagged as our own company.
	var s partyBlock
	err = q.QueryRow(ctx,
		`SELECT o.name, o.legal_name, o.tax_id,
		        a.line1, a.line2, a.city, a.region, a.postal_code, co.name
		 FROM organizations o
		 LEFT JOIN LATERAL (
		     SELECT * FROM addresses WHERE organization_id = o.id ORDER BY id LIMIT 1
		 ) a ON true
		 LEFT JOIN countries co ON co.code = a.country_code
		 WHERE o.is_self`).Scan(
		&s.name, &s.legal, &s.taxID, &line1, &line2, &city, &region, &postal, &country)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// No self organization configured; the PDF omits the seller block.
	case err != nil:
		return d, err
	default:
		s.address = addressLines(line1, line2, city, region, postal, country)
		d.seller = &s
	}

	lines, err := reporting.SalesInvoiceLines(ctx, q, invoiceID)
	if err != nil {
		return d, err
	}
	d.lines = lines
	return d, nil
}

// addressLines formats a postal address as display lines, skipping blanks.
func addressLines(line1, line2, city, region, postal, country *string) []string {
	var out []string
	for _, l := range []*string{line1, line2} {
		if l != nil && *l != "" {
			out = append(out, *l)
		}
	}
	cityLine := ""
	if city != nil && *city != "" {
		cityLine = *city
	}
	if region != nil && *region != "" {
		cityLine += ", " + *region
	}
	if postal != nil && *postal != "" {
		cityLine += " " + *postal
	}
	if cityLine != "" {
		out = append(out, strings.TrimPrefix(cityLine, ", "))
	}
	if country != nil && *country != "" {
		out = append(out, *country)
	}
	return out
}

const (
	gray     = 0.45 // secondary text
	ruleGray = 0.75 // table rules
)

// renderInvoice lays the document out. y decreases as content flows down;
// long line tables continue onto extra pages with the header row repeated.
func renderInvoice(d invoiceData) []byte {
	doc := pdf.New()
	p := doc.AddPage(pageW, pageH)
	pageCount := 1

	// --- Document title and header meta (first page only) ---
	title := "INVOICE"
	switch d.status {
	case "draft":
		title = "DRAFT INVOICE"
	case "void":
		title = "VOID INVOICE"
	}
	p.Text(pdf.HelveticaBold, 20, rightX-pdf.Width(pdf.HelveticaBold, 20, title), topY-6, title)

	metaY := topY - 36
	meta := func(label, value string) {
		p.TextGray(pdf.Helvetica, 9, rightX-140, metaY, gray, label)
		p.Text(pdf.Helvetica, 9, rightX-pdf.Width(pdf.Helvetica, 9, value), metaY, value)
		metaY -= 13
	}
	meta("Invoice no.", d.number)
	meta("Invoice date", d.date)
	if d.dueDate != nil {
		meta("Due date", *d.dueDate)
	}
	meta("Currency", d.currency)

	// --- Seller (issuer) block, top left ---
	y := topY - 6
	if d.seller != nil {
		y = partyText(p, marginL, y, 11, *d.seller)
	}

	// --- Bill-to block ---
	if y > metaY {
		y = metaY
	}
	y -= 28
	p.TextGray(pdf.HelveticaBold, 8, marginL, y, gray, "BILL TO")
	y -= 14
	y = partyText(p, marginL, y, 10, d.customer)

	// --- Lines table ---
	y -= 24
	y = tableHeader(p, y)
	for _, l := range d.lines {
		if y < bottomY+20 {
			p = doc.AddPage(pageW, pageH)
			pageCount++
			y = tableHeader(p, topY)
		}
		p.TextGray(pdf.Helvetica, 9, numX, y, gray, fmt.Sprintf("%d", l.LineNo))
		p.Text(pdf.Helvetica, 9, descX, y, truncate(pdf.Helvetica, 9, descMax, l.Description))
		amounts := []struct {
			x float64
			s string
		}{
			{qtyX, formatQty(l.Quantity)},
			{priceX, formatAmount(l.UnitPrice)},
			{taxX, formatQty(l.TaxRate)},
			{amountX, formatAmount(l.LineSubtotal)},
		}
		for _, a := range amounts {
			p.Text(pdf.Helvetica, 9, a.x-pdf.Width(pdf.Helvetica, 9, a.s), y, a.s)
		}
		y -= 6
		p.Line(marginL, y, rightX, y, 0.4, 0.9)
		y -= 12
	}

	// --- Totals ---
	if y < bottomY+110 {
		p = doc.AddPage(pageW, pageH)
		pageCount++
		y = topY
	}
	y -= 8
	totalsX := 400.0
	total := func(label, value string, f pdf.Font) {
		p.Text(f, 9, totalsX, y, label)
		p.Text(f, 9, rightX-pdf.Width(f, 9, value), y, value)
		y -= 14
	}
	total("Subtotal", formatAmount(d.subtotal), pdf.Helvetica)
	total("Tax", formatAmount(d.taxTotal), pdf.Helvetica)
	p.Line(totalsX, y+9, rightX, y+9, 0.8, 0.2)
	y -= 2
	total("Total", d.currency+" "+formatAmount(d.total), pdf.HelveticaBold)
	if formatAmount(d.applied) != "0.00" {
		total("Amount paid", formatAmount(d.applied), pdf.Helvetica)
		total("Balance due", d.currency+" "+formatAmount(d.balance), pdf.HelveticaBold)
	}

	// --- Reference and memo, bottom left of the totals ---
	noteY := y - 14
	if d.ref != nil && *d.ref != "" {
		p.TextGray(pdf.Helvetica, 9, marginL, noteY, gray, "Reference: "+*d.ref)
		noteY -= 13
	}
	if d.memo != nil && *d.memo != "" {
		for _, line := range wrap(pdf.Helvetica, 9, rightX-marginL, *d.memo) {
			p.TextGray(pdf.Helvetica, 9, marginL, noteY, gray, line)
			noteY -= 13
		}
	}

	// --- Footers, now that the page count is known ---
	for i, page := range doc.Pages() {
		footer := fmt.Sprintf("Invoice %s  ·  Page %d of %d", d.number, i+1, pageCount)
		page.TextGray(pdf.Helvetica, 8, (pageW-pdf.Width(pdf.Helvetica, 8, footer))/2, 40, gray, footer)
	}

	return doc.Bytes()
}

// partyText draws an organization block (name, legal name, tax id, address)
// and returns the y below it.
func partyText(p *pdf.Page, x, y, nameSize float64, b partyBlock) float64 {
	p.Text(pdf.HelveticaBold, nameSize, x, y, b.name)
	y -= 13
	if b.legal != nil && *b.legal != "" && *b.legal != b.name {
		p.TextGray(pdf.Helvetica, 9, x, y, gray, *b.legal)
		y -= 12
	}
	for _, line := range b.address {
		p.TextGray(pdf.Helvetica, 9, x, y, gray, line)
		y -= 12
	}
	if b.taxID != nil && *b.taxID != "" {
		p.TextGray(pdf.Helvetica, 9, x, y, gray, "Tax ID: "+*b.taxID)
		y -= 12
	}
	return y
}

// tableHeader draws the column headings and returns the y of the first row.
func tableHeader(p *pdf.Page, y float64) float64 {
	h := func(x float64, label string, rightAlign bool) {
		if rightAlign {
			x -= pdf.Width(pdf.HelveticaBold, 8, label)
		}
		p.TextGray(pdf.HelveticaBold, 8, x, y, gray, label)
	}
	h(numX, "#", false)
	h(descX, "DESCRIPTION", false)
	h(qtyX, "QTY", true)
	h(priceX, "UNIT PRICE", true)
	h(taxX, "TAX %", true)
	h(amountX, "AMOUNT", true)
	y -= 6
	p.Line(marginL, y, rightX, y, 0.8, 0.2)
	return y - 14
}

// truncate shortens s with an ellipsis until it fits in max points.
func truncate(f pdf.Font, size, max float64, s string) string {
	if pdf.Width(f, size, s) <= max {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && pdf.Width(f, size, string(r)+"…") > max {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// wrap greedily breaks s into lines no wider than max points.
func wrap(f pdf.Font, size, max float64, s string) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if pdf.Width(f, size, candidate) > max && line != "" {
			lines = append(lines, line)
			line = word
			continue
		}
		line = candidate
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// formatAmount renders a decimal string like "1234.5000" as "1,234.50". The
// fraction keeps at least two digits, more only when they are significant.
// It works on the string itself so values never pass through floating point.
func formatAmount(s string) string {
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	intPart, frac, _ := strings.Cut(s, ".")
	frac = strings.TrimRight(frac, "0")
	for len(frac) < 2 {
		frac += "0"
	}
	var grouped strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(c)
	}
	return sign + grouped.String() + "." + frac
}

// formatQty renders a decimal string with all insignificant fraction digits
// removed: "2.0000" -> "2", "2.5000" -> "2.5".
func formatQty(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
