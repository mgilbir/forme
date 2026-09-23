package paragraph

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// TestTheCaseIsTheReleasesAndNotTheToolchains.
//
// Three case pairs Unicode 16 added — Cyrillic tje, Latin s with a diagonal
// stroke, and a Garay letter, which is outside the Basic Multilingual Plane —
// under every transform that reaches the simple mapping. With the fallback
// taken from package unicode, which is 15.0.0 in Go 1.26, each came back as it
// went in.
func TestTheCaseIsTheReleasesAndNotTheToolchains(t *testing.T) {
	for _, tc := range []struct {
		in   string
		kind TextTransform
		lang Language
		want string
	}{
		{"ᲊ", TransformUppercase, "", "Ᲊ"},
		{"Ᲊ", TransformLowercase, "", "ᲊ"},
		{"ꟍ", TransformUppercase, "", "Ꟍ"},
		{"Ꟍ", TransformLowercase, "", "ꟍ"},
		{"\U00010D70", TransformUppercase, "", "\U00010D50"},
		{"\U00010D50", TransformLowercase, "", "\U00010D70"},
		// Beside a character with a full mapping, which takes the per-character
		// path rather than the whole-string one.
		{"ßᲊ", TransformUppercase, "", "SSᲉ"},
		// Beside one a language condition is about, which is a third path.
		{"IᲉ", TransformLowercase, "tr", "ıᲊ"},
	} {
		if got, _ := TransformText(tc.in, tc.kind, WordClosed, tc.lang); got != tc.want {
			t.Errorf("%q under %v (lang %q) became %q, want %q", tc.in, tc.kind, tc.lang, got, tc.want)
		}
	}
	// Titlecase is asked of the function and not through "capitalize", which
	// reaches it only for a character isWordRune calls a letter — and that is
	// package unicode's answer too, so a letter Unicode 16 added begins no word
	// there yet.
	if got := simpleTitle(0x1C8A); got != 0x1C89 {
		t.Errorf("the titlecase of U+1C8A is U+%04X, want U+1C89", got)
	}
}

// TestGoIsASubsetOfTheRelease is the other half. Unicode's case pair stability
// policy says a pair, once made, is never unmade, so every simple mapping the
// toolchain's older release knows must be one this table has too — and a
// difference is either a table read wrongly or a mapping read from the wrong
// field. It also counts the pairs Go does not know, which is what the change
// to generated tables was for.
func TestGoIsASubsetOfTheRelease(t *testing.T) {
	added := 0
	for r := rune(0); r <= unicode.MaxRune; r++ {
		for _, m := range []struct {
			name      string
			go_, ours func(rune) rune
		}{
			{"upper", unicode.ToUpper, simpleUpper},
			{"lower", unicode.ToLower, simpleLower},
			{"title", unicode.ToTitle, simpleTitle},
		} {
			g, o := m.go_(r), m.ours(r)
			switch {
			case g != r && o != g:
				t.Errorf("%s of U+%04X: Go (Unicode %s) says U+%04X and the table says U+%04X",
					m.name, r, unicode.Version, g, o)
			case g == r && o != r:
				added++
			}
		}
	}
	if unicode.Version < "16" && added == 0 {
		t.Errorf("the table knows no mapping Go %s does not; it is not from a later release",
			unicode.Version)
	}
	t.Logf("%d simple mappings the table has and Unicode %s does not", added, unicode.Version)
}

// TestTheSimpleTablesAreWhatTheLookupAssumes: sorted, since the lookup halves;
// every entry found; and the ASCII entries exactly the letters the byte loop in
// simpleCased moves without looking.
func TestTheSimpleTablesAreWhatTheLookupAssumes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		table  []simpleCase
		lo, hi byte
	}{
		{"simpleUppercase", simpleUppercase[:], 'a', 'z'},
		{"simpleLowercase", simpleLowercase[:], 'A', 'Z'},
		{"simpleTitlecase", simpleTitlecase[:], 'a', 'z'},
	} {
		ascii := 0
		for i, e := range tc.table {
			if i > 0 && tc.table[i-1].r >= e.r {
				t.Fatalf("%s is not sorted at %d", tc.name, i)
			}
			if e.to == e.r {
				t.Errorf("%s maps U+%04X to itself", tc.name, e.r)
			}
			if got := lookupSimpleCase(e.r, tc.table); got != e.to {
				t.Errorf("%s: U+%04X came back U+%04X", tc.name, e.r, got)
			}
			if e.r < utf8.RuneSelf {
				ascii++
				if byte(e.r) < tc.lo || byte(e.r) > tc.hi || e.to != e.r^0x20 {
					t.Errorf("%s maps ASCII U+%04X to U+%04X, which the byte loop "+
						"would not", tc.name, e.r, e.to)
				}
			}
		}
		if ascii != 26 {
			t.Errorf("%s has %d ASCII entries; the byte loop moves 26", tc.name, ascii)
		}
	}
}

// TestTheWholeStringPathAgreesWithTheRuneOne. upperString and lowerString skip
// ASCII without a search and return text with nothing to map as it came, and
// neither shortcut may change an answer.
func TestTheWholeStringPathAgreesWithTheRuneOne(t *testing.T) {
	reference := func(s string, f func(rune) rune) string {
		var b strings.Builder
		for _, r := range s {
			b.WriteRune(f(r))
		}
		return b.String()
	}
	var chars []string
	for _, e := range simpleUppercase {
		chars = append(chars, string(e.r))
	}
	for _, e := range simpleLowercase {
		chars = append(chars, string(e.r))
	}
	chars = append(chars, "", "a", "Z", "1", " ", "中", "́", "\xff")
	for _, c := range chars {
		for _, in := range []string{c, "a" + c, c + "Q", "x" + c + "中" + c, "ÉCOLE " + c} {
			if got, want := upperString(in), reference(in, simpleUpper); got != want {
				t.Fatalf("upperString(%q) = %q, want %q", in, got, want)
			}
			if got, want := lowerString(in), reference(in, simpleLower); got != want {
				t.Fatalf("lowerString(%q) = %q, want %q", in, got, want)
			}
		}
	}
}
