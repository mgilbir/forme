package paragraph

import (
	"strings"
	"unicode/utf8"
)

// FoldCase is Unicode's full default case folding of a string: toCasefold in
// §3.13, from CaseFolding.txt's status C and F entries in the release the rest
// of this package's tables are from. Two strings are a default caseless match
// (§3.13, D144) when their foldings are equal, so a map keyed by FoldCase finds
// every spelling of a name that differs only in case.
//
// It is what CSS Fonts 4 §5.1 matches font family names by: "User agents must
// match these names case insensitively, using the "Default Caseless Matching"
// algorithm", "without normalizing the strings involved and without applying
// any language-specific tailorings", with "the case mappings with status field
// "C" or "F"". So:
//
//   - it folds rather than lowercases. "ß" folds to "ss" and meets "SS", which
//     strings.ToLower leaves apart; final "ς" and "Σ" both fold to "σ"; the
//     KELVIN SIGN folds to "k".
//   - it does not normalize. "å" and "a" followed by a combining ring are two
//     names, as §5.1's own note says they are.
//   - it has no Turkic tailoring. "I" folds to "i" and "İ" to "i" followed by
//     U+0307 COMBINING DOT ABOVE, so "İ" does not meet "i", and "ı" folds to
//     nothing but itself.
//
// Text with nothing to fold is returned as it came, and ASCII is folded a byte
// at a time without a search. A byte that is not UTF-8 becomes U+FFFD, as
// everywhere else in this package.
func FoldCase(s string) string {
	i := firstFolded(s)
	if i < 0 {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	out.WriteString(s[:i])
	for j := i; j < len(s); {
		if c := s[j]; c < utf8.RuneSelf {
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			out.WriteByte(c)
			j++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[j:])
		if full, ok := lookupFullCase(r, foldFull[:]); ok {
			out.WriteString(full)
		} else {
			out.WriteRune(lookupSimpleCase(r, foldCommon[:]))
		}
		j += size
	}
	return out.String()
}

// firstFolded is the byte offset of the first character of s that folds to
// something else, or -1.
func firstFolded(s string) int {
	for j := 0; j < len(s); {
		if c := s[j]; c < utf8.RuneSelf {
			if 'A' <= c && c <= 'Z' {
				return j
			}
			j++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[j:])
		if _, ok := lookupFullCase(r, foldFull[:]); ok || size == 1 ||
			lookupSimpleCase(r, foldCommon[:]) != r {
			return j
		}
		j += size
	}
	return -1
}
