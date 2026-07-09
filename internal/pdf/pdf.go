// Package pdf is a minimal PDF writer, sufficient for printable business
// documents. It stays inside the standard library by construction: pages are
// laid out with the PDF "standard 14" Helvetica fonts, which every conforming
// reader provides built in, so no font program has to be embedded — only the
// advance widths (helvetica_metrics.go) are needed, for text measurement.
//
// Coordinates are PDF-native: points (1/72 inch), origin at the bottom-left
// of the page, y increasing upward. Text is WinAnsi (CP1252): runes outside
// that repertoire render as '?'.
package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strconv"
)

// Font selects one of the built-in fonts.
type Font int

const (
	Helvetica Font = iota
	HelveticaBold
)

// fontRes returns the font's resource name and width table.
func (f Font) res() string {
	if f == HelveticaBold {
		return "/F2"
	}
	return "/F1"
}

func (f Font) widths() *[256]int {
	if f == HelveticaBold {
		return &helveticaBoldWidths
	}
	return &helveticaWidths
}

// Width returns the rendered width, in points, of s at the given font size.
func Width(f Font, size float64, s string) float64 {
	w := f.widths()
	units := 0
	for _, b := range encodeWinAnsi(s) {
		units += w[b]
	}
	return float64(units) * size / 1000
}

// Doc is a document being assembled. The zero value is not usable; call New.
type Doc struct {
	pages []*Page
}

// New returns an empty document.
func New() *Doc {
	return &Doc{}
}

// AddPage appends a page of the given size in points and returns it.
// A4 is 595.28 x 841.89.
func (d *Doc) AddPage(width, height float64) *Page {
	p := &Page{width: width, height: height}
	d.pages = append(d.pages, p)
	return p
}

// Pages returns the document's pages in order, for post-layout passes (such
// as footers) that need the final page count.
func (d *Doc) Pages() []*Page {
	return d.pages
}

// Page accumulates drawing operations for one page.
type Page struct {
	width, height float64
	content       bytes.Buffer
}

// Text draws s in black with its baseline starting at (x, y).
func (p *Page) Text(f Font, size, x, y float64, s string) {
	p.TextGray(f, size, x, y, 0, s)
}

// TextGray draws s in a gray level (0 black .. 1 white).
func (p *Page) TextGray(f Font, size, x, y, gray float64, s string) {
	fmt.Fprintf(&p.content, "BT %s %s Tf %s g %s %s Td ",
		f.res(), num(size), num(gray), num(x), num(y))
	p.content.Write(escapeString(encodeWinAnsi(s)))
	p.content.WriteString(" Tj ET\n")
}

// Line strokes a straight line of the given width and gray level.
func (p *Page) Line(x1, y1, x2, y2, width, gray float64) {
	fmt.Fprintf(&p.content, "%s w %s G %s %s m %s %s l S\n",
		num(width), num(gray), num(x1), num(y1), num(x2), num(y2))
}

// FillRect fills a rectangle (x, y is its bottom-left corner) with a gray level.
func (p *Page) FillRect(x, y, w, h, gray float64) {
	fmt.Fprintf(&p.content, "%s g %s %s %s %s re f\n",
		num(gray), num(x), num(y), num(w), num(h))
}

// num formats a coordinate compactly (PDF has no use for trailing zeros).
func num(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// encodeWinAnsi converts a UTF-8 string to WinAnsi (CP1252) bytes,
// substituting '?' for anything unrepresentable.
func encodeWinAnsi(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 0x80:
			out = append(out, byte(r))
		case r >= 0xA0 && r <= 0xFF: // Latin-1 block coincides with WinAnsi
			out = append(out, byte(r))
		default:
			if b, ok := winAnsiSpecials[r]; ok {
				out = append(out, b)
			} else {
				out = append(out, '?')
			}
		}
	}
	return out
}

// winAnsiSpecials maps the runes WinAnsi places in the 0x80–0x9F block
// (where Unicode and CP1252 diverge).
var winAnsiSpecials = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86,
	'‡': 0x87, 'ˆ': 0x88, '‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C,
	'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95,
	'–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B,
	'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// escapeString wraps bytes as a PDF literal string, escaping the delimiters.
func escapeString(b []byte) []byte {
	out := make([]byte, 0, len(b)+2)
	out = append(out, '(')
	for _, c := range b {
		switch c {
		case '(', ')', '\\':
			out = append(out, '\\', c)
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		default:
			out = append(out, c)
		}
	}
	return append(out, ')')
}

// Bytes serializes the document.
//
// Object layout: 1 catalog, 2 page tree, 3/4 the two fonts, then one page
// object and one (flate-compressed) content stream per page.
func (d *Doc) Bytes() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n") // binary marker comment

	offsets := make([]int, 0, 4+2*len(d.pages))
	obj := func(body string) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}

	kids := ""
	for i := range d.pages {
		kids += fmt.Sprintf("%d 0 R ", 5+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, len(d.pages)))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")

	for i, p := range d.pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] "+
			"/Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>",
			num(p.width), num(p.height), 6+2*i))

		var compressed bytes.Buffer
		zw := zlib.NewWriter(&compressed)
		_, _ = zw.Write(p.content.Bytes()) // writes to a bytes.Buffer cannot fail
		_ = zw.Close()

		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n",
			len(offsets), compressed.Len())
		buf.Write(compressed.Bytes())
		buf.WriteString("\nendstream\nendobj\n")
	}

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets)+1, xref)
	return buf.Bytes()
}
