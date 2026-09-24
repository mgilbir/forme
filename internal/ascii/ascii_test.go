package ascii

import (
	"math/rand"
	"strings"
	"testing"
)

// The characters Unicode's case mapping folds onto ASCII, or out of it, and
// that ASCII's must leave alone.
const (
	kelvin   = "\u212A" // KELVIN SIGN, which Unicode lowercases to "k"
	longS    = "ſ"      // LATIN SMALL LETTER LONG S, which Unicode folds to "s"
	dottedI  = "İ"      // LATIN CAPITAL LETTER I WITH DOT ABOVE: "i" + U+0307
	dotlessI = "ı"      // LATIN SMALL LETTER DOTLESS I, which uppercases to "I"
	angstrom = "\u212B" // ANGSTROM SIGN, which lowercases to "å"
)

func TestEqualFoldIsASCIIs(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"", "", true},
		{"auto", "AUTO", true},
		{"Font-Face", "font-face", true},
		{"@MEDIA", "@media", true},
		{"a", "b", false},
		{"auto", "aut", false},
		// Unicode's folding makes each of these a match, and none is one.
		{kelvin + "eyframes", "keyframes", false},
		{longS + "olid", "solid", false},
		{"@MED" + dottedI + "A", "@media", false},
		{"@med" + dotlessI + "a", "@media", false},
		// The dotted I matches itself, and the letters round it fold.
		{"@MED" + dottedI + "A", "@med" + dottedI + "a", true},
		{angstrom, "å", false},
		// Bytes outside ASCII match only themselves.
		{"é", "é", true},
		{"É", "é", false},
		{"\xff", "\xff", true},
		// @ and [ are one below A and one above Z; ` and { the same around a–z.
		{"@", "`", false},
		{"[", "{", false},
	} {
		if got := EqualFold(tc.a, tc.b); got != tc.want {
			t.Errorf("EqualFold(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
		if got := EqualFold(tc.b, tc.a); got != tc.want {
			t.Errorf("EqualFold(%q, %q) = %v, want %v", tc.b, tc.a, got, tc.want)
		}
	}
}

func TestLowerAndUpperTouchOnlyASCIILetters(t *testing.T) {
	for _, tc := range []struct{ in, lower, upper string }{
		{"", "", ""},
		{"Hello, World", "hello, world", "HELLO, WORLD"},
		{"@[`{", "@[`{", "@[`{"},
		{kelvin + "Hz", kelvin + "hz", kelvin + "HZ"},
		{longS + "pan", longS + "pan", longS + "PAN"},
		{"D" + dottedI + "V", "d" + dottedI + "v", "D" + dottedI + "V"},
		{"d" + dotlessI + "v", "d" + dotlessI + "v", "D" + dotlessI + "V"},
		{"É\xffa", "É\xffa", "É\xffA"},
	} {
		if got := Lower(tc.in); got != tc.lower {
			t.Errorf("Lower(%q) = %q, want %q", tc.in, got, tc.lower)
		}
		if got := Upper(tc.in); got != tc.upper {
			t.Errorf("Upper(%q) = %q, want %q", tc.in, got, tc.upper)
		}
	}
	// A string with nothing to map comes back without a copy, which is what
	// makes it fit to be asked of every keyword a stylesheet holds.
	if n := testing.AllocsPerRun(100, func() { sink = Lower("already-lower") }); n != 0 {
		t.Errorf("Lower of a lower-case string allocated %v times", n)
	}
	if n := testing.AllocsPerRun(100, func() { sink = Upper("ALREADY-UPPER") }); n != 0 {
		t.Errorf("Upper of an upper-case string allocated %v times", n)
	}
}

var sink string

// TestTheFoldsAgree holds EqualFold, Lower and Upper to one another over
// random strings drawn from an alphabet that holds the letters around each
// boundary and the characters Unicode folds onto ASCII.
func TestTheFoldsAgree(t *testing.T) {
	alphabet := []string{"a", "A", "z", "Z", "@", "[", "`", "{", "k", "K", "s", "S", "i", "I",
		kelvin, longS, dottedI, dotlessI, "\xff", "é"}
	r := rand.New(rand.NewSource(1))
	word := func() string {
		var b strings.Builder
		for n := r.Intn(4); n > 0; n-- {
			b.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		return b.String()
	}
	for i := 0; i < 20000; i++ {
		a, b := word(), word()
		want := Lower(a) == Lower(b)
		if got := EqualFold(a, b); got != want {
			t.Fatalf("EqualFold(%q, %q) = %v, but their Lower forms are %q and %q", a, b, got, Lower(a), Lower(b))
		}
		if got := Upper(a) == Upper(b); got != want {
			t.Fatalf("%q and %q are equal under Lower %v and under Upper %v", a, b, want, got)
		}
		if len(Lower(a)) != len(a) || len(Upper(a)) != len(a) {
			t.Fatalf("%q changed length under a fold", a)
		}
	}
}

func TestPrefixSuffixAndIndexFold(t *testing.T) {
	for _, tc := range []struct {
		s, prefix string
		want      bool
	}{
		{"<!DOCTYPE html>", "<!doctype", true},
		{"<!DoCtYpE html>", "<!doctype", true},
		{"<!doctype html>", "<!doctype", true},
		{"<!docty", "<!doctype", false},
		{"<div>", "<!doctype", false},
		{"", "", true},
		{kelvin + "eyframes", "k", false},
	} {
		if got := HasPrefixFold(tc.s, tc.prefix); got != tc.want {
			t.Errorf("HasPrefixFold(%q, %q) = %v, want %v", tc.s, tc.prefix, got, tc.want)
		}
	}
	for _, tc := range []struct {
		s, suffix string
		want      bool
	}{
		{"image/png;BASE64", "base64", true},
		{"base6", "base64", false},
		{"10" + kelvin, "k", false},
		{"", "", true},
	} {
		if got := HasSuffixFold(tc.s, tc.suffix); got != tc.want {
			t.Errorf("HasSuffixFold(%q, %q) = %v, want %v", tc.s, tc.suffix, got, tc.want)
		}
	}
	for _, tc := range []struct {
		s, sub string
		want   int
	}{
		{"abc</STYLE>", "</style", 3},
		{"abc</style>", "</style", 3},
		{"</styl", "</style", -1},
		{"xx</Style></style>", "</style", 2},
		{"nothing", "</style", -1},
		// The needle's own case must not matter either.
		{"abc</style>", "</STYLE", 3},
		{"@" + longS + "upports @supports", "@supports", 1 + len(longS) + len("upports ")},
		{"x", "", 0},
	} {
		if got := IndexFold(tc.s, tc.sub); got != tc.want {
			t.Errorf("IndexFold(%q, %q) = %d, want %d", tc.s, tc.sub, got, tc.want)
		}
		if got := ContainsFold(tc.s, tc.sub); got != (tc.want >= 0) {
			t.Errorf("ContainsFold(%q, %q) = %v, want %v", tc.s, tc.sub, got, tc.want >= 0)
		}
	}
}
