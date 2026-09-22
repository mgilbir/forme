package paragraph

import (
	"strings"
	"unicode/utf8"
)

// The simple case mappings, read from the release the rest of this package's
// tables are from.
//
// They were Go's: unicode.ToUpper and strings.ToUpper, which answer from the
// release the toolchain shipped — Unicode 15.0.0 for Go 1.26 — beside tables
// generated from 17.0.0. Every case pair Unicode added in between was a
// character "text-transform: uppercase" left as it was: "ᲊ" and "ꟍ" have had
// capitals since Unicode 16, and came back lower case. casingtable.go now holds
// every simple mapping of the release its header names, and these are the
// functions that read it; see cmd/gencasing.
//
// They are the fallback the full mappings in fullCased and capitalizeWords
// take, and nothing else here: a mapping that is more than one character, or
// that depends on the language or the characters around it, is decided before
// a character reaches one of these.

// simpleUpper is a character's simple uppercase mapping, or the character.
func simpleUpper(r rune) rune { return lookupSimpleCase(r, simpleUppercase[:]) }

// simpleLower is a character's simple lowercase mapping, or the character.
func simpleLower(r rune) rune { return lookupSimpleCase(r, simpleLowercase[:]) }

// simpleTitle is a character's simple titlecase mapping, or the character.
func simpleTitle(r rune) rune { return lookupSimpleCase(r, simpleTitlecase[:]) }

// upperString and lowerString map every character of a string by its simple
// mapping. They are the whole-string halves of fullCased, which is what
// strings.ToUpper and strings.ToLower were.
func upperString(s string) string { return simpleCased(s, simpleUppercase[:], 'a', 'z') }
func lowerString(s string) string { return simpleCased(s, simpleLowercase[:], 'A', 'Z') }

// simpleCased maps a string through one of the simple tables.
//
// Text with nothing to map is returned as it came, and ASCII — which is most of
// what this sees — is mapped a byte at a time without a search: lo..hi is the
// range of ASCII letters the table moves, and it moves each of them by 32.
// simplecasing_test.go holds the table to that.
func simpleCased(s string, table []simpleCase, lo, hi byte) string {
	i := firstSimpleCased(s, table, lo, hi)
	if i < 0 {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	out.WriteString(s[:i])
	for j := i; j < len(s); {
		if c := s[j]; c < utf8.RuneSelf {
			if lo <= c && c <= hi {
				c ^= 0x20
			}
			out.WriteByte(c)
			j++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[j:])
		out.WriteRune(lookupSimpleCase(r, table))
		j += size
	}
	return out.String()
}

// firstSimpleCased is the byte offset of the first character of s the table
// moves, or -1.
func firstSimpleCased(s string, table []simpleCase, lo, hi byte) int {
	for j := 0; j < len(s); {
		if c := s[j]; c < utf8.RuneSelf {
			if lo <= c && c <= hi {
				return j
			}
			j++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[j:])
		// A byte that is not UTF-8 is rebuilt as U+FFFD, which is what
		// strings.ToUpper did with one and what the loop above writes, so the
		// text is not returned as it came.
		if lookupSimpleCase(r, table) != r || size == 1 {
			return j
		}
		j += size
	}
	return -1
}

// lookupSimpleCase searches one of the generated tables, which are sorted, and
// answers the character itself where the table has nothing: a character with no
// mapping in the release is one that has no case.
func lookupSimpleCase(r rune, table []simpleCase) rune {
	i, j := 0, len(table)
	for i < j {
		h := int(uint(i+j) >> 1)
		if table[h].r < r {
			i = h + 1
		} else {
			j = h
		}
	}
	if i < len(table) && table[i].r == r {
		return table[i].to
	}
	return r
}
