// Package ascii compares and case-maps the syntax of CSS and HTML, which folds
// ASCII and nothing else.
//
// CSS Syntax 3 §2.1 and the Infra standard define "ASCII case-insensitive":
// two strings match when they are equal after A–Z are mapped to a–z, and no
// other code point is touched. Every keyword, property name, at-rule name,
// pseudo-class, unit, function name, media feature and named colour in CSS is
// compared that way, and so are HTML's element and attribute names and the
// attribute values HTML lists as case-insensitive.
//
// Go's strings.EqualFold, ToLower and ToUpper are Unicode's case mapping, and
// they answer more than that. U+212A KELVIN SIGN folds to "k" and U+017F LATIN
// SMALL LETTER LONG S to "s", so "\u212Aeyframes" was @keyframes and "ſolid"
// was solid; U+0130 LATIN CAPITAL LETTER I WITH DOT ABOVE lowercases to "i"
// followed by U+0307, which is a string one byte longer than the one it came
// from, so an offset found in the lowered copy was not an offset into the text.
// None of those is the keyword it matched, and no browser reads any of them as
// one.
//
// The functions here fold A–Z and leave every other byte as it is, so what
// they return is always the same length as what they were given and a byte
// that is not ASCII is never equal to one that is. They do not decode UTF-8,
// and do not need to: a byte of a multi-byte sequence is never in A–Z.
//
// Text that is case-mapped for display — text-transform, small caps — is not
// syntax and does not come here. It reads the pinned Unicode tables in package
// paragraph. cmd/asciicase_test.go holds the engine to this split.
package ascii

// LowerByte maps an ASCII capital to its small letter and returns every other
// byte unchanged.
func LowerByte(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// UpperByte maps an ASCII small letter to its capital and returns every other
// byte unchanged.
func UpperByte(c byte) byte {
	if 'a' <= c && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

// Lower is s with A–Z mapped to a–z. It returns s itself, without copying, when
// there is nothing to map, which is the common case for a keyword already
// written in lower case.
func Lower(s string) string {
	i := 0
	for i < len(s) && (s[i] < 'A' || s[i] > 'Z') {
		i++
	}
	if i == len(s) {
		return s
	}
	b := []byte(s)
	for ; i < len(b); i++ {
		b[i] = LowerByte(b[i])
	}
	return string(b)
}

// Upper is s with a–z mapped to A–Z, returning s itself when there is nothing
// to map.
func Upper(s string) string {
	i := 0
	for i < len(s) && (s[i] < 'a' || s[i] > 'z') {
		i++
	}
	if i == len(s) {
		return s
	}
	b := []byte(s)
	for ; i < len(b); i++ {
		b[i] = UpperByte(b[i])
	}
	return string(b)
}

// EqualFold reports whether a and b are ASCII case-insensitively equal. It
// copies neither.
func EqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] && LowerByte(a[i]) != LowerByte(b[i]) {
			return false
		}
	}
	return true
}

// HasPrefixFold reports whether s begins with prefix, ignoring ASCII case.
func HasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && EqualFold(s[:len(prefix)], prefix)
}

// HasSuffixFold reports whether s ends with suffix, ignoring ASCII case.
func HasSuffixFold(s, suffix string) bool {
	return len(s) >= len(suffix) && EqualFold(s[len(s)-len(suffix):], suffix)
}

// IndexFold is the index of the first instance of sub in s, ignoring ASCII
// case, or -1.
//
// The scan is anchored on sub's first byte, so the comparison of the rest runs
// only where it can succeed. That keeps the ordinary search — a short needle in
// a long document — linear in the document; it is not a guarantee against a
// needle built to defeat it, which no caller here takes from a document.
func IndexFold(s, sub string) int {
	if sub == "" {
		return 0
	}
	first := LowerByte(sub[0])
	for i := 0; i+len(sub) <= len(s); i++ {
		if LowerByte(s[i]) == first && EqualFold(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

// ContainsFold reports whether sub is within s, ignoring ASCII case.
func ContainsFold(s, sub string) bool { return IndexFold(s, sub) >= 0 }
