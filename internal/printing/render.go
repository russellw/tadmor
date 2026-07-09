package printing

import (
	"fmt"
	"strings"

	"tadmor/internal/pdf"
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

const (
	gray     = 0.45 // secondary text
	ruleGray = 0.75 // table rules
)

// partyBlock is one side of the document: an organization and its address.
type partyBlock struct {
	name    string
	legal   *string
	taxID   *string
	address []string
}

// metaItem is one label/value row in the top-right header block.
type metaItem struct {
	label, value string
}

// docLine is one table row: the fields every document's lines share.
type docLine struct {
	no       int
	desc     string
	qty      string
	unit     string
	taxRate  string
	subtotal string
}

// docData is everything the layout needs to render one document, fetched up
// front. appliedLabel empty means the document has no application concept
// (orders), so the applied/balance rows never render.
type docData struct {
	kind     string // "Invoice", "Bill", ... used in the title and footer
	number   string
	status   string
	currency string
	meta     []metaItem // top-right label/value rows

	partyLabel string // heading over the counterparty block
	party      partyBlock
	seller     *partyBlock // nil until an organization is marked is_self

	unitLabel string // per-unit column heading: "UNIT PRICE" or "UNIT COST"

	subtotal string
	taxTotal string
	total    string
	applied  string
	balance  string

	appliedLabel string
	balanceLabel string

	ref  *string
	memo *string

	lines []docLine
}

// title prefixes the document name with its lifecycle state when that state
// changes what the paper means.
func (d docData) title() string {
	t := strings.ToUpper(d.kind)
	switch d.status {
	case "draft":
		return "DRAFT " + t
	case "void":
		return "VOID " + t
	case "cancelled":
		return "CANCELLED " + t
	}
	return t
}

// renderDoc lays the document out. y decreases as content flows down; long
// line tables continue onto extra pages with the header row repeated.
func renderDoc(d docData) []byte {
	doc := pdf.New()
	p := doc.AddPage(pageW, pageH)
	pageCount := 1

	// --- Document title and header meta (first page only) ---
	title := d.title()
	p.Text(pdf.HelveticaBold, 20, rightX-pdf.Width(pdf.HelveticaBold, 20, title), topY-6, title)

	metaY := topY - 36
	for _, m := range d.meta {
		p.TextGray(pdf.Helvetica, 9, rightX-140, metaY, gray, m.label)
		p.Text(pdf.Helvetica, 9, rightX-pdf.Width(pdf.Helvetica, 9, m.value), metaY, m.value)
		metaY -= 13
	}

	// --- Issuer block, top left ---
	y := topY - 6
	if d.seller != nil {
		y = partyText(p, marginL, y, 11, *d.seller)
	}

	// --- Counterparty block ---
	if y > metaY {
		y = metaY
	}
	y -= 28
	p.TextGray(pdf.HelveticaBold, 8, marginL, y, gray, d.partyLabel)
	y -= 14
	y = partyText(p, marginL, y, 10, d.party)

	// --- Lines table ---
	y -= 24
	y = tableHeader(p, y, d.unitLabel)
	for _, l := range d.lines {
		if y < bottomY+20 {
			p = doc.AddPage(pageW, pageH)
			pageCount++
			y = tableHeader(p, topY, d.unitLabel)
		}
		p.TextGray(pdf.Helvetica, 9, numX, y, gray, fmt.Sprintf("%d", l.no))
		p.Text(pdf.Helvetica, 9, descX, y, truncate(pdf.Helvetica, 9, descMax, l.desc))
		amounts := []struct {
			x float64
			s string
		}{
			{qtyX, formatQty(l.qty)},
			{priceX, formatAmount(l.unit)},
			{taxX, formatQty(l.taxRate)},
			{amountX, formatAmount(l.subtotal)},
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
	if d.appliedLabel != "" && formatAmount(d.applied) != "0.00" {
		total(d.appliedLabel, formatAmount(d.applied), pdf.Helvetica)
		total(d.balanceLabel, d.currency+" "+formatAmount(d.balance), pdf.HelveticaBold)
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
		footer := fmt.Sprintf("%s %s  ·  Page %d of %d", d.kind, d.number, i+1, pageCount)
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
func tableHeader(p *pdf.Page, y float64, unitLabel string) float64 {
	h := func(x float64, label string, rightAlign bool) {
		if rightAlign {
			x -= pdf.Width(pdf.HelveticaBold, 8, label)
		}
		p.TextGray(pdf.HelveticaBold, 8, x, y, gray, label)
	}
	h(numX, "#", false)
	h(descX, "DESCRIPTION", false)
	h(qtyX, "QTY", true)
	h(priceX, unitLabel, true)
	h(taxX, "TAX %", true)
	h(amountX, "AMOUNT", true)
	y -= 6
	p.Line(marginL, y, rightX, y, 0.8, 0.2)
	return y - 14
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
