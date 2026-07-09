#!/usr/bin/env python3
"""Generate helvetica_metrics.go from the Adobe Core 14 AFM metric files.

The PDF writer uses the PDF "standard 14" fonts, which every conforming
reader ships built in, so no font program is embedded — but laying text out
(right-aligning amounts, truncating long descriptions) still needs the
per-character advance widths. Those come from Adobe's freely redistributable
Core 14 AFM files; this script projects them through WinAnsiEncoding (the
encoding the writer declares) into plain Go arrays.

Usage:
    python3 gen_helvetica_metrics.py Helvetica.afm Helvetica-Bold.afm

The AFM files themselves are not committed (they are ~75 KB each and carry
their own distribution conditions); one canonical source is the Apache PDFBox
repository, e.g.
https://raw.githubusercontent.com/apache/pdfbox/trunk/pdfbox/src/main/resources/org/apache/pdfbox/resources/afm/Helvetica.afm
The generated widths are pure facts (integer advance widths in 1/1000 em)
and are what every PDF producer embeds for these fonts.
"""

import re
import sys

# WinAnsiEncoding: character code -> Adobe glyph name, for the codes that
# differ from or extend ASCII. Codes 0x20..0x7E are ASCII with the two
# PostScript renames noted below; 0x80..0x9F is the CP1252 block;
# 0xA0..0xFF is Latin-1.
WINANSI = {
    0x27: "quotesingle",  # ASCII apostrophe (StandardEncoding calls it quoteright)
    0x60: "grave",        # ASCII backtick (StandardEncoding calls it quoteleft)
    0x80: "Euro",
    0x82: "quotesinglbase", 0x83: "florin", 0x84: "quotedblbase",
    0x85: "ellipsis", 0x86: "dagger", 0x87: "daggerdbl", 0x88: "circumflex",
    0x89: "perthousand", 0x8A: "Scaron", 0x8B: "guilsinglleft", 0x8C: "OE",
    0x8E: "Zcaron",
    0x91: "quoteleft", 0x92: "quoteright", 0x93: "quotedblleft",
    0x94: "quotedblright", 0x95: "bullet", 0x96: "endash", 0x97: "emdash",
    0x98: "tilde", 0x99: "trademark", 0x9A: "scaron", 0x9B: "guilsinglright",
    0x9C: "oe", 0x9E: "zcaron", 0x9F: "Ydieresis",
    0xA0: "space",   # no-break space renders with the space width
    0xA1: "exclamdown", 0xA2: "cent", 0xA3: "sterling", 0xA4: "currency",
    0xA5: "yen", 0xA6: "brokenbar", 0xA7: "section", 0xA8: "dieresis",
    0xA9: "copyright", 0xAA: "ordfeminine", 0xAB: "guillemotleft",
    0xAC: "logicalnot", 0xAD: "hyphen", 0xAE: "registered", 0xAF: "macron",
    0xB0: "degree", 0xB1: "plusminus", 0xB2: "twosuperior",
    0xB3: "threesuperior", 0xB4: "acute", 0xB5: "mu", 0xB6: "paragraph",
    0xB7: "periodcentered", 0xB8: "cedilla", 0xB9: "onesuperior",
    0xBA: "ordmasculine", 0xBB: "guillemotright", 0xBC: "onequarter",
    0xBD: "onehalf", 0xBE: "threequarters", 0xBF: "questiondown",
    0xC0: "Agrave", 0xC1: "Aacute", 0xC2: "Acircumflex", 0xC3: "Atilde",
    0xC4: "Adieresis", 0xC5: "Aring", 0xC6: "AE", 0xC7: "Ccedilla",
    0xC8: "Egrave", 0xC9: "Eacute", 0xCA: "Ecircumflex", 0xCB: "Edieresis",
    0xCC: "Igrave", 0xCD: "Iacute", 0xCE: "Icircumflex", 0xCF: "Idieresis",
    0xD0: "Eth", 0xD1: "Ntilde", 0xD2: "Ograve", 0xD3: "Oacute",
    0xD4: "Ocircumflex", 0xD5: "Otilde", 0xD6: "Odieresis", 0xD7: "multiply",
    0xD8: "Oslash", 0xD9: "Ugrave", 0xDA: "Uacute", 0xDB: "Ucircumflex",
    0xDC: "Udieresis", 0xDD: "Yacute", 0xDE: "Thorn", 0xDF: "germandbls",
    0xE0: "agrave", 0xE1: "aacute", 0xE2: "acircumflex", 0xE3: "atilde",
    0xE4: "adieresis", 0xE5: "aring", 0xE6: "ae", 0xE7: "ccedilla",
    0xE8: "egrave", 0xE9: "eacute", 0xEA: "ecircumflex", 0xEB: "edieresis",
    0xEC: "igrave", 0xED: "iacute", 0xEE: "icircumflex", 0xEF: "idieresis",
    0xF0: "eth", 0xF1: "ntilde", 0xF2: "ograve", 0xF3: "oacute",
    0xF4: "ocircumflex", 0xF5: "otilde", 0xF6: "odieresis", 0xF7: "divide",
    0xF8: "oslash", 0xF9: "ugrave", 0xFA: "uacute", 0xFB: "ucircumflex",
    0xFC: "udieresis", 0xFD: "yacute", 0xFE: "thorn", 0xFF: "ydieresis",
}

ASCII_NAMES = {
    0x20: "space", 0x21: "exclam", 0x22: "quotedbl", 0x23: "numbersign",
    0x24: "dollar", 0x25: "percent", 0x26: "ampersand", 0x28: "parenleft",
    0x29: "parenright", 0x2A: "asterisk", 0x2B: "plus", 0x2C: "comma",
    0x2D: "hyphen", 0x2E: "period", 0x2F: "slash",
    0x3A: "colon", 0x3B: "semicolon", 0x3C: "less", 0x3D: "equal",
    0x3E: "greater", 0x3F: "question", 0x40: "at",
    0x5B: "bracketleft", 0x5C: "backslash", 0x5D: "bracketright",
    0x5E: "asciicircum", 0x5F: "underscore",
    0x7B: "braceleft", 0x7C: "bar", 0x7D: "braceright", 0x7E: "asciitilde",
}
for c in range(0x30, 0x3A):
    ASCII_NAMES[c] = "zero one two three four five six seven eight nine".split()[c - 0x30]
for c in range(0x41, 0x5B):
    ASCII_NAMES[c] = chr(c)
for c in range(0x61, 0x7B):
    ASCII_NAMES[c] = chr(c)


def glyph_name(code: int) -> str | None:
    if code in WINANSI:
        return WINANSI[code]
    return ASCII_NAMES.get(code)


CHAR_RE = re.compile(r"C\s+(-?\d+)\s*;\s*WX\s+(-?\d+)\s*;\s*N\s+(\S+)\s*;")


def afm_widths(path: str) -> tuple[str, list[int]]:
    """Return (FontName, widths[256]) projected through WinAnsiEncoding."""
    by_name: dict[str, int] = {}
    font_name = ""
    with open(path, encoding="latin-1") as f:
        for line in f:
            if line.startswith("FontName "):
                font_name = line.split()[1]
            m = CHAR_RE.match(line)
            if m:
                by_name[m.group(3)] = int(m.group(2))
    widths = [0] * 256
    for code in range(0x20, 0x100):
        name = glyph_name(code)
        if name is not None and name in by_name:
            widths[code] = by_name[name]
    return font_name, widths


def go_array(widths: list[int]) -> str:
    lines = []
    for row in range(0, 256, 16):
        lines.append("\t" + " ".join(f"{w}," for w in widths[row : row + 16]))
    return "\n".join(lines)


def main() -> None:
    if len(sys.argv) != 3:
        sys.exit("usage: gen_helvetica_metrics.py Helvetica.afm Helvetica-Bold.afm")
    regular_name, regular = afm_widths(sys.argv[1])
    bold_name, bold = afm_widths(sys.argv[2])
    if regular_name != "Helvetica" or bold_name != "Helvetica-Bold":
        sys.exit(f"unexpected fonts: {regular_name}, {bold_name}")
    print(f"""// Code generated by gen_helvetica_metrics.py; DO NOT EDIT.
//
// Advance widths (1/1000 em) for the standard-14 Helvetica fonts under
// WinAnsiEncoding, indexed by character code. Derived from Adobe's Core 14
// AFM metric files (see gen_helvetica_metrics.py for provenance).

package pdf

var helveticaWidths = [256]int{{
{go_array(regular)}
}}

var helveticaBoldWidths = [256]int{{
{go_array(bold)}
}}""")


if __name__ == "__main__":
    main()
