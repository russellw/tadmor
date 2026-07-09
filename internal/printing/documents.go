// Package printing renders documents as PDFs for sending outside the system.
// It reads with the same Querier convention as reporting and lays pages out
// with the internal/pdf writer. Every document type shares one layout
// (render.go); a docSpec supplies the labels and queries that differ.
package printing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"tadmor/internal/reporting"
)

// docSpec describes how to fetch and label one document type. headerSQL and
// addressSQL are both keyed on the document id ($1). headerSQL must return
// exactly the columns fetchDoc scans — number, date, second date, currency,
// status, subtotal, tax total, total, applied, balance, reference, memo, and
// the counterparty organization's name, legal name, and tax id — with NULLs
// where a concept (due date, applications) does not apply.
type docSpec struct {
	kind         string // "Invoice", "Bill", ... also the title, footer, and filename base
	numberLabel  string
	dateLabel    string
	dueLabel     string // label for the second date, when the document has one
	unitLabel    string
	partyLabel   string
	appliedLabel string // "" when the document has no application concept
	balanceLabel string
	headerSQL    string
	addressSQL   string
	// recipientSQL returns one row, one column: the counterparty
	// organization's email (nullable), keyed on the document id ($1).
	recipientSQL string
	lines        func(context.Context, reporting.Querier, int) ([]docLine, error)
}

// SalesInvoicePDF renders the invoice as a PDF, returning the bytes and the
// document number (for the download filename). It returns
// reporting.ErrNotFound when the invoice does not exist; likewise the other
// document renderers below.
func SalesInvoicePDF(ctx context.Context, q reporting.Querier, id int) ([]byte, string, error) {
	return docPDF(ctx, q, salesInvoiceSpec, id)
}

// PurchaseBillPDF renders the bill as a PDF.
func PurchaseBillPDF(ctx context.Context, q reporting.Querier, id int) ([]byte, string, error) {
	return docPDF(ctx, q, purchaseBillSpec, id)
}

// SalesCreditNotePDF renders the sales credit note as a PDF.
func SalesCreditNotePDF(ctx context.Context, q reporting.Querier, id int) ([]byte, string, error) {
	return docPDF(ctx, q, salesCreditNoteSpec, id)
}

// PurchaseCreditNotePDF renders the purchase (supplier) credit note as a PDF.
func PurchaseCreditNotePDF(ctx context.Context, q reporting.Querier, id int) ([]byte, string, error) {
	return docPDF(ctx, q, purchaseCreditNoteSpec, id)
}

// SalesOrderPDF renders the sales order as a PDF.
func SalesOrderPDF(ctx context.Context, q reporting.Querier, id int) ([]byte, string, error) {
	return docPDF(ctx, q, salesOrderSpec, id)
}

// PurchaseOrderPDF renders the purchase order as a PDF.
func PurchaseOrderPDF(ctx context.Context, q reporting.Querier, id int) ([]byte, string, error) {
	return docPDF(ctx, q, purchaseOrderSpec, id)
}

func docPDF(ctx context.Context, q reporting.Querier, spec docSpec, id int) ([]byte, string, error) {
	d, err := fetchDoc(ctx, q, spec, id)
	if err != nil {
		return nil, "", err
	}
	return renderDoc(d), d.number, nil
}

// Recipient resolvers, one per document type, mirroring the PDF renderers.
// Each returns the counterparty organization's email, "" when the organization
// has none on file, and reporting.ErrNotFound when the document does not exist.

func SalesInvoiceRecipient(ctx context.Context, q reporting.Querier, id int) (string, error) {
	return docRecipient(ctx, q, salesInvoiceSpec, id)
}

func PurchaseBillRecipient(ctx context.Context, q reporting.Querier, id int) (string, error) {
	return docRecipient(ctx, q, purchaseBillSpec, id)
}

func SalesCreditNoteRecipient(ctx context.Context, q reporting.Querier, id int) (string, error) {
	return docRecipient(ctx, q, salesCreditNoteSpec, id)
}

func PurchaseCreditNoteRecipient(ctx context.Context, q reporting.Querier, id int) (string, error) {
	return docRecipient(ctx, q, purchaseCreditNoteSpec, id)
}

func SalesOrderRecipient(ctx context.Context, q reporting.Querier, id int) (string, error) {
	return docRecipient(ctx, q, salesOrderSpec, id)
}

func PurchaseOrderRecipient(ctx context.Context, q reporting.Querier, id int) (string, error) {
	return docRecipient(ctx, q, purchaseOrderSpec, id)
}

func docRecipient(ctx context.Context, q reporting.Querier, spec docSpec, id int) (string, error) {
	var email *string
	err := q.QueryRow(ctx, spec.recipientSQL, id).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", reporting.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if email == nil {
		return "", nil
	}
	return *email, nil
}

func fetchDoc(ctx context.Context, q reporting.Querier, spec docSpec, id int) (docData, error) {
	d := docData{
		kind:         spec.kind,
		unitLabel:    spec.unitLabel,
		partyLabel:   spec.partyLabel,
		appliedLabel: spec.appliedLabel,
		balanceLabel: spec.balanceLabel,
	}
	var date string
	var due, applied, balance *string
	err := q.QueryRow(ctx, spec.headerSQL, id).Scan(
		&d.number, &date, &due, &d.currency, &d.status,
		&d.subtotal, &d.taxTotal, &d.total, &applied, &balance,
		&d.ref, &d.memo,
		&d.party.name, &d.party.legal, &d.party.taxID)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, reporting.ErrNotFound
	}
	if err != nil {
		return d, err
	}
	d.meta = []metaItem{{spec.numberLabel, d.number}, {spec.dateLabel, date}}
	if due != nil {
		d.meta = append(d.meta, metaItem{spec.dueLabel, *due})
	}
	d.meta = append(d.meta, metaItem{"Currency", d.currency})
	if applied != nil && balance != nil {
		d.applied, d.balance = *applied, *balance
	}

	// Counterparty address; its absence is not an error.
	var line1, line2, city, region, postal, country *string
	err = q.QueryRow(ctx, spec.addressSQL, id).Scan(&line1, &line2, &city, &region, &postal, &country)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return d, err
	}
	d.party.address = addressLines(line1, line2, city, region, postal, country)

	if d.seller, err = fetchSelf(ctx, q); err != nil {
		return d, err
	}
	if d.lines, err = spec.lines(ctx, q, id); err != nil {
		return d, err
	}
	return d, nil
}

// fetchSelf loads the issuer: whichever organization is flagged as our own
// company. nil (not an error) when none is configured.
func fetchSelf(ctx context.Context, q reporting.Querier) (*partyBlock, error) {
	var s partyBlock
	var line1, line2, city, region, postal, country *string
	err := q.QueryRow(ctx,
		`SELECT o.name, o.legal_name, o.tax_id,
		        a.line1, a.line2, a.city, a.region, a.postal_code, co.name
		 FROM organizations o
		 LEFT JOIN LATERAL (
		     SELECT * FROM addresses WHERE organization_id = o.id ORDER BY id LIMIT 1
		 ) a ON true
		 LEFT JOIN countries co ON co.code = a.country_code
		 WHERE o.is_self`).Scan(
		&s.name, &s.legal, &s.taxID, &line1, &line2, &city, &region, &postal, &country)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.address = addressLines(line1, line2, city, region, postal, country)
	return &s, nil
}

// salesLines adapts a sales-side line query (invoices and sales credit notes
// share the row shape) to the generic table rows.
func salesLines(fetch func(context.Context, reporting.Querier, int) ([]reporting.SalesInvoiceLine, error),
) func(context.Context, reporting.Querier, int) ([]docLine, error) {
	return func(ctx context.Context, q reporting.Querier, id int) ([]docLine, error) {
		lines, err := fetch(ctx, q, id)
		if err != nil {
			return nil, err
		}
		out := make([]docLine, len(lines))
		for i, l := range lines {
			out[i] = docLine{l.LineNo, l.Description, l.Quantity, l.UnitPrice, l.TaxRate, l.LineSubtotal}
		}
		return out, nil
	}
}

// purchaseLines is the purchasing-side mirror of salesLines.
func purchaseLines(fetch func(context.Context, reporting.Querier, int) ([]reporting.PurchaseBillLine, error),
) func(context.Context, reporting.Querier, int) ([]docLine, error) {
	return func(ctx context.Context, q reporting.Querier, id int) ([]docLine, error) {
		lines, err := fetch(ctx, q, id)
		if err != nil {
			return nil, err
		}
		out := make([]docLine, len(lines))
		for i, l := range lines {
			out[i] = docLine{l.LineNo, l.Description, l.Quantity, l.UnitCost, l.TaxRate, l.LineSubtotal}
		}
		return out, nil
	}
}

func salesOrderDocLines(ctx context.Context, q reporting.Querier, id int) ([]docLine, error) {
	lines, err := reporting.SalesOrderLines(ctx, q, id)
	if err != nil {
		return nil, err
	}
	out := make([]docLine, len(lines))
	for i, l := range lines {
		out[i] = docLine{l.LineNo, l.Description, l.Quantity, l.UnitPrice, l.TaxRate, l.LineSubtotal}
	}
	return out, nil
}

func purchaseOrderDocLines(ctx context.Context, q reporting.Querier, id int) ([]docLine, error) {
	lines, err := reporting.PurchaseOrderLines(ctx, q, id)
	if err != nil {
		return nil, err
	}
	out := make([]docLine, len(lines))
	for i, l := range lines {
		out[i] = docLine{l.LineNo, l.Description, l.Quantity, l.UnitCost, l.TaxRate, l.LineSubtotal}
	}
	return out, nil
}

var salesInvoiceSpec = docSpec{
	kind:         "Invoice",
	numberLabel:  "Invoice no.",
	dateLabel:    "Invoice date",
	dueLabel:     "Due date",
	unitLabel:    "UNIT PRICE",
	partyLabel:   "BILL TO",
	appliedLabel: "Amount paid",
	balanceLabel: "Balance due",
	headerSQL: `
		SELECT si.invoice_number, si.invoice_date::text, si.due_date::text, si.currency_code, si.status,
		       si.subtotal::numeric(19,4)::text, si.tax_total::numeric(19,4)::text, si.total::numeric(19,4)::text,
		       b.amount_applied::numeric(19,4)::text, b.balance::numeric(19,4)::text,
		       si.reference, si.memo, o.name, o.legal_name, o.tax_id
		FROM sales_invoices si
		JOIN customers c     ON c.id = si.customer_id
		JOIN organizations o ON o.id = c.organization_id
		JOIN sales_invoice_balances b ON b.invoice_id = si.id
		WHERE si.id = $1`,
	// Billing address: the invoice's explicit one, else the customer
	// organization's first.
	addressSQL: `
		SELECT a.line1, a.line2, a.city, a.region, a.postal_code, co.name
		FROM sales_invoices si
		JOIN customers c ON c.id = si.customer_id
		JOIN addresses a ON a.id = COALESCE(si.billing_address_id,
		    (SELECT id FROM addresses WHERE organization_id = c.organization_id ORDER BY id LIMIT 1))
		LEFT JOIN countries co ON co.code = a.country_code
		WHERE si.id = $1`,
	recipientSQL: `
		SELECT o.email
		FROM sales_invoices si
		JOIN customers c     ON c.id = si.customer_id
		JOIN organizations o ON o.id = c.organization_id
		WHERE si.id = $1`,
	lines: salesLines(reporting.SalesInvoiceLines),
}

var purchaseBillSpec = docSpec{
	kind:         "Bill",
	numberLabel:  "Bill no.",
	dateLabel:    "Bill date",
	dueLabel:     "Due date",
	unitLabel:    "UNIT COST",
	partyLabel:   "SUPPLIER",
	appliedLabel: "Amount paid",
	balanceLabel: "Balance due",
	headerSQL: `
		SELECT pb.bill_number, pb.bill_date::text, pb.due_date::text, pb.currency_code, pb.status,
		       pb.subtotal::numeric(19,4)::text, pb.tax_total::numeric(19,4)::text, pb.total::numeric(19,4)::text,
		       b.amount_applied::numeric(19,4)::text, b.balance::numeric(19,4)::text,
		       pb.reference, pb.memo, o.name, o.legal_name, o.tax_id
		FROM purchase_bills pb
		JOIN suppliers s     ON s.id = pb.supplier_id
		JOIN organizations o ON o.id = s.organization_id
		JOIN purchase_bill_balances b ON b.bill_id = pb.id
		WHERE pb.id = $1`,
	addressSQL: `
		SELECT a.line1, a.line2, a.city, a.region, a.postal_code, co.name
		FROM purchase_bills pb
		JOIN suppliers s ON s.id = pb.supplier_id
		JOIN addresses a ON a.organization_id = s.organization_id
		LEFT JOIN countries co ON co.code = a.country_code
		WHERE pb.id = $1
		ORDER BY a.id LIMIT 1`,
	recipientSQL: `
		SELECT o.email
		FROM purchase_bills pb
		JOIN suppliers s     ON s.id = pb.supplier_id
		JOIN organizations o ON o.id = s.organization_id
		WHERE pb.id = $1`,
	lines: purchaseLines(reporting.PurchaseBillLines),
}

var salesCreditNoteSpec = docSpec{
	kind:         "Credit Note",
	numberLabel:  "Credit note no.",
	dateLabel:    "Credit note date",
	unitLabel:    "UNIT PRICE",
	partyLabel:   "CREDIT TO",
	appliedLabel: "Amount applied",
	balanceLabel: "Unapplied",
	headerSQL: `
		SELECT cn.credit_note_number, cn.credit_note_date::text, NULL::text, cn.currency_code, cn.status,
		       cn.subtotal::numeric(19,4)::text, cn.tax_total::numeric(19,4)::text, cn.total::numeric(19,4)::text,
		       b.amount_applied::numeric(19,4)::text, b.balance::numeric(19,4)::text,
		       cn.reference, cn.memo, o.name, o.legal_name, o.tax_id
		FROM sales_credit_notes cn
		JOIN customers c     ON c.id = cn.customer_id
		JOIN organizations o ON o.id = c.organization_id
		JOIN sales_credit_note_balances b ON b.credit_note_id = cn.id
		WHERE cn.id = $1`,
	addressSQL: `
		SELECT a.line1, a.line2, a.city, a.region, a.postal_code, co.name
		FROM sales_credit_notes cn
		JOIN customers c ON c.id = cn.customer_id
		JOIN addresses a ON a.organization_id = c.organization_id
		LEFT JOIN countries co ON co.code = a.country_code
		WHERE cn.id = $1
		ORDER BY a.id LIMIT 1`,
	recipientSQL: `
		SELECT o.email
		FROM sales_credit_notes cn
		JOIN customers c     ON c.id = cn.customer_id
		JOIN organizations o ON o.id = c.organization_id
		WHERE cn.id = $1`,
	lines: salesLines(reporting.SalesCreditNoteLines),
}

var purchaseCreditNoteSpec = docSpec{
	kind:         "Supplier Credit",
	numberLabel:  "Credit note no.",
	dateLabel:    "Credit note date",
	unitLabel:    "UNIT COST",
	partyLabel:   "SUPPLIER",
	appliedLabel: "Amount applied",
	balanceLabel: "Unapplied",
	headerSQL: `
		SELECT cn.credit_note_number, cn.credit_note_date::text, NULL::text, cn.currency_code, cn.status,
		       cn.subtotal::numeric(19,4)::text, cn.tax_total::numeric(19,4)::text, cn.total::numeric(19,4)::text,
		       b.amount_applied::numeric(19,4)::text, b.balance::numeric(19,4)::text,
		       cn.reference, cn.memo, o.name, o.legal_name, o.tax_id
		FROM purchase_credit_notes cn
		JOIN suppliers s     ON s.id = cn.supplier_id
		JOIN organizations o ON o.id = s.organization_id
		JOIN purchase_credit_note_balances b ON b.credit_note_id = cn.id
		WHERE cn.id = $1`,
	addressSQL: `
		SELECT a.line1, a.line2, a.city, a.region, a.postal_code, co.name
		FROM purchase_credit_notes cn
		JOIN suppliers s ON s.id = cn.supplier_id
		JOIN addresses a ON a.organization_id = s.organization_id
		LEFT JOIN countries co ON co.code = a.country_code
		WHERE cn.id = $1
		ORDER BY a.id LIMIT 1`,
	recipientSQL: `
		SELECT o.email
		FROM purchase_credit_notes cn
		JOIN suppliers s     ON s.id = cn.supplier_id
		JOIN organizations o ON o.id = s.organization_id
		WHERE cn.id = $1`,
	lines: purchaseLines(reporting.PurchaseCreditNoteLines),
}

var salesOrderSpec = docSpec{
	kind:        "Sales Order",
	numberLabel: "Order no.",
	dateLabel:   "Order date",
	dueLabel:    "Expected ship",
	unitLabel:   "UNIT PRICE",
	partyLabel:  "CUSTOMER",
	headerSQL: `
		SELECT so.order_number, so.order_date::text, so.expected_ship_date::text, so.currency_code, so.status,
		       so.subtotal::numeric(19,4)::text, so.tax_total::numeric(19,4)::text, so.total::numeric(19,4)::text,
		       NULL::text, NULL::text,
		       so.reference, so.memo, o.name, o.legal_name, o.tax_id
		FROM sales_orders so
		JOIN customers c     ON c.id = so.customer_id
		JOIN organizations o ON o.id = c.organization_id
		WHERE so.id = $1`,
	addressSQL: `
		SELECT a.line1, a.line2, a.city, a.region, a.postal_code, co.name
		FROM sales_orders so
		JOIN customers c ON c.id = so.customer_id
		JOIN addresses a ON a.organization_id = c.organization_id
		LEFT JOIN countries co ON co.code = a.country_code
		WHERE so.id = $1
		ORDER BY a.id LIMIT 1`,
	recipientSQL: `
		SELECT o.email
		FROM sales_orders so
		JOIN customers c     ON c.id = so.customer_id
		JOIN organizations o ON o.id = c.organization_id
		WHERE so.id = $1`,
	lines: salesOrderDocLines,
}

var purchaseOrderSpec = docSpec{
	kind:        "Purchase Order",
	numberLabel: "Order no.",
	dateLabel:   "Order date",
	dueLabel:    "Expected receipt",
	unitLabel:   "UNIT COST",
	partyLabel:  "SUPPLIER",
	headerSQL: `
		SELECT po.order_number, po.order_date::text, po.expected_receipt_date::text, po.currency_code, po.status,
		       po.subtotal::numeric(19,4)::text, po.tax_total::numeric(19,4)::text, po.total::numeric(19,4)::text,
		       NULL::text, NULL::text,
		       po.reference, po.memo, o.name, o.legal_name, o.tax_id
		FROM purchase_orders po
		JOIN suppliers s     ON s.id = po.supplier_id
		JOIN organizations o ON o.id = s.organization_id
		WHERE po.id = $1`,
	addressSQL: `
		SELECT a.line1, a.line2, a.city, a.region, a.postal_code, co.name
		FROM purchase_orders po
		JOIN suppliers s ON s.id = po.supplier_id
		JOIN addresses a ON a.organization_id = s.organization_id
		LEFT JOIN countries co ON co.code = a.country_code
		WHERE po.id = $1
		ORDER BY a.id LIMIT 1`,
	recipientSQL: `
		SELECT o.email
		FROM purchase_orders po
		JOIN suppliers s     ON s.id = po.supplier_id
		JOIN organizations o ON o.id = s.organization_id
		WHERE po.id = $1`,
	lines: purchaseOrderDocLines,
}
