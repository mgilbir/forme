// Package diag renders an author's text for a message about it.
//
// Every reader in this module reports what it refused in words, and nearly
// every one of those messages quotes the input: a tag name, a property, a
// value, a URL, a font family. The input is untrusted and it is bytes. A Go
// string carries no promise of UTF-8, and HTML's reader leaves the bytes as the
// document had them (see html/encoding.go) — so "<a\x93" produced the message
// "the tag <a\x93 is never closed", which is not text. Every consumer that
// writes a message to a log, a terminal, a JSON report or a PDF annotation has
// to be able to take it as text, and the fuzz targets say so. A control
// character is the other half of the same fault: a message carrying a carriage
// return, an escape sequence or a C1 control can rewrite what a person reading
// the log sees.
//
// So what a message quotes goes through here, and comes out as text:
//
//   - a byte that begins no UTF-8 character is U+FFFD REPLACEMENT CHARACTER,
//     one for each byte, which is what every reader of the document makes of it
//     when it walks the text — the message shows what the engine read; and
//   - a control character, C0, DEL or C1, is "?".
//
// The controls are General_Category Cc, U+0000–U+001F and U+007F–U+009F. That
// set is fixed by Unicode's stability policy, so it is written out rather than
// asked of package unicode, which the engine does not consult for properties
// (cmd/pinnedunicode_test.go).
package diag

import (
	"strings"
	"unicode/utf8"
)

// Text is s fit to be written into a message: every byte that begins no UTF-8
// character read as U+FFFD, and every control character as "?".
//
// It returns s itself, and allocates nothing, when there is nothing to change —
// which is every message an ordinary document produces — so a sink can pass
// every message through it.
func Text(s string) string {
	i := firstUnfit(s)
	if i < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteString(s[:i])
	write(&b, s[i:], -1)
	return b.String()
}

// Cut is Text of s, cut before the first character that begins max bytes or
// more into it, with "..." for what was cut. It is the form for a quotation
// whose length is the document's to choose, so that a message stays a message.
func Cut(s string, max int) string {
	if len(s) <= max && firstUnfit(s) < 0 {
		return s
	}
	var b strings.Builder
	write(&b, s, max)
	return b.String()
}

// Quote is Cut of s in double quotes.
func Quote(s string, max int) string {
	var b strings.Builder
	b.WriteByte('"')
	write(&b, s, max)
	b.WriteByte('"')
	return b.String()
}

// IsText reports whether s is already what Text would make of it.
func IsText(s string) bool { return firstUnfit(s) < 0 }

// write appends Text of s to b, cut as Cut says when max is not negative.
func write(b *strings.Builder, s string, max int) {
	for i, r := range s {
		if max >= 0 && i >= max {
			b.WriteString("...")
			return
		}
		// range yields utf8.RuneError for a byte that begins no character,
		// one byte at a time, and WriteRune writes it as U+FFFD.
		if isControl(r) {
			b.WriteByte('?')
			continue
		}
		b.WriteRune(r)
	}
}

// firstUnfit is the offset of the first byte of s that Text would change, or -1.
func firstUnfit(s string) int {
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c < 0x20 || c == 0x7F {
				return i
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 || isControl(r) {
			return i
		}
		i += size
	}
	return -1
}

// isControl reports whether r is General_Category Cc.
func isControl(r rune) bool {
	return r < 0x20 || r >= 0x7F && r <= 0x9F
}
