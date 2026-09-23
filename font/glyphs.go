package font

import "strings"

// GlyphNameToRune maps a glyph name to a Unicode code point for the common
// cases: the uniXXXX/uXXXX conventions, the names of Adobe's Glyph List, and
// the ASCII and Latin-1 ranges, where the standard Latin encodings are
// identity. shape reads it to know which character each code of a simple
// font's encoding stands for.
func GlyphNameToRune(name string, code byte) (rune, bool) {
	if strings.HasPrefix(name, "uni") && len(name) == 7 {
		if v, ok := parseHexN(name[3:]); ok {
			return rune(v), true
		}
	}
	if strings.HasPrefix(name, "u") && len(name) >= 5 && len(name) <= 7 {
		if v, ok := parseHexN(name[1:]); ok {
			return rune(v), true
		}
	}
	// The named glyphs of the standard Latin encodings, from Adobe's Glyph
	// List. This is consulted before the identity rule below because the two
	// disagree exactly where it matters: WinAnsiEncoding puts the curly quotes,
	// the dashes, the bullet, the ellipsis and the euro between 0x80 and 0x9F,
	// where nothing about the code implies the character.
	if r, ok := glyphNameRunes[name]; ok {
		return r, true
	}
	// ASCII and Latin-1 high range: the standard Latin encodings are identity
	// there, which covers a font naming a glyph the list above does not.
	if (code >= 0x20 && code <= 0x7E) || code >= 0xA0 {
		return rune(code), true
	}
	return 0, false
}
func parseHexN(s string) (int, bool) {
	v := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			v = v<<4 | int(c-'0')
		case c >= 'A' && c <= 'F':
			v = v<<4 | int(c-'A'+10)
		case c >= 'a' && c <= 'f':
			v = v<<4 | int(c-'a'+10)
		default:
			return 0, false
		}
	}
	return v, true
}
