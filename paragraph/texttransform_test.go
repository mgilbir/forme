package paragraph

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// The full case mappings — the ones that turn one character into more than one.
//
// Go's strings.ToUpper is one character to one character, so it leaves ß alone
// and produces "STRAßE". These tests are about the table that fixes that, and
// about the two things a table consulted on a fast path can get wrong: missing a
// character it holds, and changing one it does not.

// TestACharacterWhoseCaseIsNotOneCharacter is the case the table exists for, and
// every row is one Unicode states unconditionally.
func TestACharacterWhoseCaseIsNotOneCharacter(t *testing.T) {
	for _, tc := range []struct {
		in   string
		kind TextTransform
		want string
		what string
	}{
		{"straße", TransformUppercase, "STRASSE", "the sharp s, which is why anyone notices"},
		{"ﬁne", TransformUppercase, "FINE", "a Latin ligature"},
		{"ﬅop", TransformUppercase, "STOP", "another one"},
		{"և", TransformUppercase, "ԵՒ", "an Armenian ligature"},
		{"\u01F0", TransformUppercase, "J\u030C", "a letter whose upper case is a letter and a mark"},
		// Greek is nearly all of the table. Unicode 17 spells this one out as
		// iota, dialytika, tonos rather than composing the first two, and the
		// escapes are here because a combining mark in a source file attaches
		// itself to whatever precedes it and is invisible in a diff.
		{"\u0390", TransformUppercase, "\u0399\u0308\u0301", "Greek dialytika and tonos"},
		{"ᾀ", TransformUppercase, "ἈΙ", "an iota subscript, which becomes a capital iota"},
		{"\u0130", TransformLowercase, "i\u0307", "the only character whose lower case is longer"},

		// Titlecase is a third mapping rather than a variation on the other two,
		// and this is the row that shows it: "ß" titlecases to "Ss" and
		// uppercases to "SS".
		{"ßtraße", TransformCapitalize, "Sstraße", "titlecase is not uppercase"},
		{"ﬁne wine", TransformCapitalize, "Fine Wine", "a ligature starting a word"},
	} {
		got, _ := TransformText(tc.in, tc.kind, false)
		if got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.in, got, tc.want)
		}
	}
}

// TestTextWithNoFullMappingIsUnchangedByTheTable is the containment half. Almost
// every document contains none of these characters, and for those the tables must
// be invisible — not merely produce the same answer, but take the same path.
func TestTextWithNoFullMappingIsUnchangedByTheTable(t *testing.T) {
	for _, in := range []string{
		"", "hello world", "HELLO WORLD", "Ünicode Ölaf", "καλημέρα", "ΚΑΛΗΜΕΡΑ",
		"日本語", "a1!", " \t\n",
	} {
		for _, tc := range []struct {
			kind TextTransform
			want string
		}{
			{TransformUppercase, strings.ToUpper(in)},
			{TransformLowercase, strings.ToLower(in)},
		} {
			if got, _ := TransformText(in, tc.kind, false); got != tc.want {
				t.Errorf("%q became %q, want %q — no character of it has a full mapping",
					in, got, tc.want)
			}
		}
	}
}

// TestTheFastPathAgreesWithTheSlowOne.
//
// fullCased hands text with no full-mapped character straight to strings.ToUpper
// and rebuilds the rest a character at a time, and the two halves must not be
// able to disagree. The reference here is the obvious implementation — look in
// the table, fall back to Go — written out so that the optimisation has
// something to be measured against rather than only itself.
//
// The corpus is every character in the tables in turn, each with ASCII, a
// multi-byte character with no mapping, and another table character on either
// side of it. The prefix cases are the ones that matter: text[:i] takes the
// whole-string path and everything after it takes the loop, so a boundary that
// is off by a character loses one or maps it twice.
func TestTheFastPathAgreesWithTheSlowOne(t *testing.T) {
	reference := func(text string, table []fullCase, simple func(rune) rune) string {
		var out strings.Builder
		for _, r := range text {
			if s, ok := lookupFullCase(r, table); ok {
				out.WriteString(s)
			} else {
				out.WriteRune(simple(r))
			}
		}
		return out.String()
	}
	for _, m := range []struct {
		name   string
		table  []fullCase
		simple func(rune) rune
		whole  func(string) string
	}{
		{"uppercase", fullUppercase[:], unicode.ToUpper, strings.ToUpper},
		{"lowercase", fullLowercase[:], unicode.ToLower, strings.ToLower},
	} {
		for _, e := range m.table {
			c := string(e.r)
			for _, in := range []string{
				c, "a" + c, c + "a", "a" + c + "b", "über" + c, c + "Ω" + c,
				strings.Repeat(c, 3), "  " + c + "  ",
			} {
				want := reference(in, m.table, m.simple)
				if got := fullCased(in, m.table, m.simple, m.whole); got != want {
					t.Fatalf("%s %q: got %q, want %q", m.name, in, got, want)
				}
			}
		}
	}
}

// TestEveryTableEntryIsFound. The tables are searched by halving, which needs
// them sorted and finds nothing quietly when they are not — so this asks for
// every entry by name rather than trusting the generator, and asks for the
// characters either side of each to be absent.
func TestEveryTableEntryIsFound(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table []fullCase
	}{
		{"fullLowercase", fullLowercase[:]},
		{"fullTitlecase", fullTitlecase[:]},
		{"fullUppercase", fullUppercase[:]},
	} {
		if len(tc.table) == 0 {
			t.Errorf("%s is empty", tc.name)
		}
		in := map[rune]bool{}
		for i, e := range tc.table {
			if i > 0 && tc.table[i-1].r >= e.r {
				t.Fatalf("%s is not sorted at %d: %#04X after %#04X",
					tc.name, i, e.r, tc.table[i-1].r)
			}
			if e.s == "" {
				t.Errorf("%s: %#04X maps to nothing", tc.name, e.r)
			}
			in[e.r] = true
			got, ok := lookupFullCase(e.r, tc.table)
			if !ok || got != e.s {
				t.Errorf("%s: %#04X is in the table and came back (%q, %v)",
					tc.name, e.r, got, ok)
			}
		}
		// Neighbours, which is where a search that is off by one lands.
		for r := range in {
			for _, n := range []rune{r - 1, r + 1} {
				if in[n] {
					continue
				}
				if s, ok := lookupFullCase(n, tc.table); ok {
					t.Errorf("%s: %#04X is not in the table and came back %q",
						tc.name, n, s)
				}
			}
		}
		// And nothing ASCII, which firstFullCase skips without decoding.
		for r := rune(0); r < utf8.RuneSelf; r++ {
			if _, ok := lookupFullCase(r, tc.table); ok {
				t.Errorf("%s holds %#04X, and firstFullCase passes over every "+
					"character below U+0080 without looking", tc.name, r)
			}
		}
	}
}

// TestAFullMappingIsLongerThanWhatItReplaces is the property the whole table
// turns on, and the reason the transform cannot be done in place or measured
// from the source text.
func TestAFullMappingIsLongerThanWhatItReplaces(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table []fullCase
	}{
		{"fullLowercase", fullLowercase[:]},
		{"fullTitlecase", fullTitlecase[:]},
		{"fullUppercase", fullUppercase[:]},
	} {
		for _, e := range tc.table {
			if utf8.RuneCountInString(e.s) < 2 {
				t.Errorf("%s: %#04X maps to %q, which is one character — the simple "+
					"mapping already says so and the table is the override",
					tc.name, e.r, e.s)
			}
		}
	}
}
