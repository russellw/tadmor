package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"testing"
)

func TestWidth(t *testing.T) {
	// Helvetica digits are 556/1000 em; space is 278.
	if got, want := Width(Helvetica, 10, "00"), 11.12; math.Abs(got-want) > 1e-9 {
		t.Errorf("Width(digits) = %v, want %v", got, want)
	}
	if got, want := Width(Helvetica, 1000, " "), 278.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("Width(space) = %v, want %v", got, want)
	}
	if Width(HelveticaBold, 10, "abc") <= Width(Helvetica, 10, "abc") {
		t.Error("bold text should measure wider than regular")
	}
	// The euro sign lives in WinAnsi's CP1252 block and must have a width.
	if Width(Helvetica, 10, "€") == 0 {
		t.Error("euro sign has no width")
	}
}

func TestEncodeWinAnsi(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []byte
	}{
		{"abc", []byte("abc")},
		{"é", []byte{0xE9}},
		{"€–", []byte{0x80, 0x96}},
		{"中", []byte{'?'}}, // unrepresentable
	} {
		if got := encodeWinAnsi(tc.in); !bytes.Equal(got, tc.want) {
			t.Errorf("encodeWinAnsi(%q) = % x, want % x", tc.in, got, tc.want)
		}
	}
}

func TestEscapeString(t *testing.T) {
	if got := escapeString([]byte(`a(b)c\`)); string(got) != `(a\(b\)c\\)` {
		t.Errorf("escapeString = %s", got)
	}
}

// TestBytes checks the document's structural invariants: header, one object
// per expected slot, an xref table whose offsets really point at those
// objects, and a decompressible content stream containing the drawn text.
func TestBytes(t *testing.T) {
	d := New()
	p := d.AddPage(595.28, 841.89)
	p.Text(Helvetica, 10, 72, 800, "hello (world)")
	p.Line(72, 790, 523, 790, 0.5, 0.5)
	d.AddPage(595.28, 841.89).Text(HelveticaBold, 12, 72, 800, "page 2")

	out := d.Bytes()
	if !bytes.HasPrefix(out, []byte("%PDF-1.4\n")) {
		t.Fatal("missing PDF header")
	}
	if !bytes.HasSuffix(out, []byte("%%EOF\n")) {
		t.Fatal("missing EOF marker")
	}

	// 4 fixed objects + (page + content) per page.
	wantObjects := 4 + 2*2
	offsets := regexp.MustCompile(`(?m)^(\d{10}) 00000 n `).FindAllSubmatch(out, -1)
	if len(offsets) != wantObjects {
		t.Fatalf("xref has %d entries, want %d", len(offsets), wantObjects)
	}
	for i, m := range offsets {
		off, _ := strconv.Atoi(string(m[1]))
		want := fmt.Sprintf("%d 0 obj\n", i+1)
		if !bytes.HasPrefix(out[off:], []byte(want)) {
			t.Errorf("xref entry %d points at %q, want %q", i+1, out[off:off+12], want)
		}
	}

	// Decompress the first content stream and check the text made it in.
	start := bytes.Index(out, []byte("stream\n"))
	if start < 0 {
		t.Fatal("no content stream")
	}
	end := bytes.Index(out[start:], []byte("\nendstream"))
	zr, err := zlib.NewReader(bytes.NewReader(out[start+len("stream\n") : start+end]))
	if err != nil {
		t.Fatalf("content stream is not valid zlib: %v", err)
	}
	content, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Contains(content, []byte(`(hello \(world\)) Tj`)) {
		t.Errorf("content stream missing text: %s", content)
	}
	if !bytes.Contains(content, []byte(" l S")) {
		t.Errorf("content stream missing line: %s", content)
	}
}
