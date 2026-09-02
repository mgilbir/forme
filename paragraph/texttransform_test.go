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
		got, _ := TransformText(tc.in, tc.kind, false, "")
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
			if got, _ := TransformText(in, tc.kind, false, ""); got != tc.want {
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
				if got := fullCased(in, m.table, m.simple, m.whole, nil); got != want {
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

// The two values that remap characters rather than changing case, and the
// grammar that lets them combine with one that does.

// TestTheValueIsASetAndNotAChoice. CSS Text 3 §2.1.1 writes the property as
//
//	none | [ capitalize | uppercase | lowercase ] || full-width || full-size-kana
//
// so one case change and either remapping may appear together, in any order —
// and two case changes may not, and an unknown keyword makes the whole
// declaration invalid rather than the keyword it appeared in.
func TestTheValueIsASetAndNotAChoice(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  TextTransform
	}{
		{"none", TransformNone},
		{"uppercase", TransformUppercase},
		{"lowercase", TransformLowercase},
		{"capitalize", TransformCapitalize},
		{"full-width", TransformFullWidth},
		{"full-size-kana", TransformFullSizeKana},

		{"uppercase full-width", TransformUppercase | TransformFullWidth},
		{"full-width uppercase", TransformUppercase | TransformFullWidth},
		{"full-width full-size-kana lowercase",
			TransformLowercase | TransformFullWidth | TransformFullSizeKana},
		{"full-width full-size-kana", TransformFullWidth | TransformFullSizeKana},

		// Keywords are case-insensitive and the whitespace between them is not
		// part of them.
		{"  UPPERCASE   Full-Width ", TransformUppercase | TransformFullWidth},

		// Invalid, and an invalid declaration is dropped whole.
		{"uppercase lowercase", TransformNone},
		{"capitalize uppercase", TransformNone},
		{"uppercase uppercase", TransformNone},
		{"full-width full-width", TransformNone},
		{"none uppercase", TransformNone},
		{"uppercase wibble", TransformNone},
		{"wibble", TransformNone},
		{"fullwidth", TransformNone},
		{"", TransformNone},
	} {
		if got := TransformOf(tc.value); got != tc.want {
			t.Errorf("text-transform:%q read as %05b, want %05b", tc.value, got, tc.want)
		}
	}
}

// TestTheTransformsAreAppliedInTheSpecifiedOrder, which is not the order the
// keywords were written in: the case change first, then full-width, then
// full-size-kana.
//
// Every row is one of the suite's own — text-transform-multiple-001 states the
// declaration and the text it must produce, side by side — so this is the
// specification's example rather than one invented to match the code. The
// separators are what show the order: a space becomes U+3000 IDEOGRAPHIC SPACE
// only after the case change, and "Hiragana:" keeps its colon as a fullwidth
// one.
func TestTheTransformsAreAppliedInTheSpecifiedOrder(t *testing.T) {
	for _, tc := range []struct {
		value, in, want string
	}{
		{"uppercase full-width", "HELLO Transformed world",
			"ＨＥＬＬＯ　ＴＲＡＮＳＦＯＲＭＥＤ　ＷＯＲＬＤ"},
		{"lowercase full-width", "HELLO Transformed world",
			"ｈｅｌｌｏ　ｔｒａｎｓｆｏｒｍｅｄ　ｗｏｒｌｄ"},
		{"capitalize full-width", "HELLO Transformed world",
			"ＨＥＬＬＯ　Ｔｒａｎｓｆｏｒｍｅｄ　Ｗｏｒｌｄ"},
		{"uppercase full-size-kana", "Katakana: ァィゥェォヵㇰヶㇱㇲッㇳㇴ",
			"KATAKANA: アイウエオカクケシスツトヌ"},
		{"full-width full-size-kana lowercase", "Hiragana: ぁぃぅぇぉゕゖっゃゅょゎ",
			"ｈｉｒａｇａｎａ：　あいうえおかけつやゆよわ"},

		// Not the suite's, and the only row that can tell the order at all.
		//
		// Case and width commute for nearly everything: a fullwidth Latin letter
		// has the same case as the letter it came from, so uppercasing before or
		// after gives the same page. They part company exactly where a case
		// mapping is not one character for one — ß uppercases to "SS", and SS has
		// a fullwidth form where ß has none. Case first gives "ＳＳ"; width first
		// leaves the ß alone and then uppercases it to a narrow "SS".
		//
		// Seventeen characters of Unicode discriminate the two orders and this is
		// the first of them. Nothing discriminates full-width from full-size-kana,
		// or either of those from a case change — both were checked over every
		// character of Unicode, and both commute — so the specification's order is
		// what the code follows and this row is all of it that a test can hold.
		{"uppercase full-width", "straße", "ＳＴＲＡＳＳＥ"},
	} {
		got, _ := TransformText(tc.in, TransformOf(tc.value), false, "")
		if got != tc.want {
			t.Errorf("text-transform:%s on %q gave\n\t%q, want\n\t%q",
				tc.value, tc.in, got, tc.want)
		}
	}
}

// TestARemappingLeavesEverythingElseAlone is the containment half, and it is the
// one that matters most for full-width: nearly half its table is ASCII, so it is
// the one transform that touches the characters every document is made of.
func TestARemappingLeavesEverythingElseAlone(t *testing.T) {
	for _, tc := range []struct {
		kind     TextTransform
		in, want string
		why      string
	}{
		{TransformFullWidth, "6月", "６月", "the suite's own fixture"},
		{TransformFullWidth, "日本語", "日本語", "already wide, and not in the table"},
		{TransformFullSizeKana, "Hello, world", "Hello, world", "no kana at all"},
		{TransformFullSizeKana, "あいうえお", "あいうえお", "kana, none of it small"},
		{TransformFullSizeKana, "6月", "6月", "full-size-kana is not full-width"},
		{TransformFullWidth, "ぁ", "ぁ", "full-width is not full-size-kana"},
	} {
		if got, _ := TransformText(tc.in, tc.kind, false, ""); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.why, tc.in, got, tc.want)
		}
	}
}

// TestRemappingTwiceChangesNothingMore.
//
// Neither table maps a character to one it also maps *from*, so applying either
// twice is applying it once. That is a property of the data rather than of the
// code, and it is worth pinning because it is what says the two tables are
// mappings into a set of forms rather than a walk that could go round: were a
// fullwidth character also a key, "full-width full-width" and a document that
// nested two transformed elements would disagree.
func TestRemappingTwiceChangesNothingMore(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table []widthPair
	}{
		{"fullWidthForms", fullWidthForms[:]},
		{"fullSizeKana", fullSizeKana[:]},
	} {
		keys := map[rune]bool{}
		for _, p := range tc.table {
			keys[p.from] = true
		}
		for _, p := range tc.table {
			if keys[p.to] {
				t.Errorf("%s maps %#04X to %#04X, which the table also transforms",
					tc.name, p.from, p.to)
			}
			if p.from == p.to {
				t.Errorf("%s maps %#04X to itself", tc.name, p.from)
			}
		}
	}
}

// TestEveryWidthTableEntryIsFound is TestEveryTableEntryIsFound for the other
// pair of tables and the other binary search — the same defects, the same way of
// catching them.
func TestEveryWidthTableEntryIsFound(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table []widthPair
	}{
		{"fullWidthForms", fullWidthForms[:]},
		{"fullSizeKana", fullSizeKana[:]},
	} {
		if len(tc.table) == 0 {
			t.Errorf("%s is empty", tc.name)
		}
		in := map[rune]bool{}
		for i, p := range tc.table {
			if i > 0 && tc.table[i-1].from >= p.from {
				t.Fatalf("%s is not sorted at %d: %#04X after %#04X",
					tc.name, i, p.from, tc.table[i-1].from)
			}
			in[p.from] = true
			if to, ok := lookupWidth(p.from, tc.table); !ok || to != p.to {
				t.Errorf("%s: %#04X is in the table and came back (%#04X, %v)",
					tc.name, p.from, to, ok)
			}
		}
		for r := range in {
			for _, n := range []rune{r - 1, r + 1} {
				if in[n] {
					continue
				}
				if to, ok := lookupWidth(n, tc.table); ok {
					t.Errorf("%s: %#04X is not in the table and came back %#04X",
						tc.name, n, to)
				}
			}
		}
	}
}

// TestTheFullWidthTableCoversASCII is the one assertion here that is about what
// the table is *for*. The value exists to set Latin text as though it were
// ideographic, so every printable ASCII character must have a form — and the
// space's is U+3000 and not U+FF00, which is the row a hand-written table gets
// wrong and the row the whitespace processing in layout/box.go depends on.
func TestTheFullWidthTableCoversASCII(t *testing.T) {
	for r := rune(0x20); r <= 0x7E; r++ {
		to, ok := lookupWidth(r, fullWidthForms[:])
		if !ok {
			t.Errorf("%#04X %q has no fullwidth form", r, r)
			continue
		}
		want := r + 0xFEE0
		if r == ' ' {
			want = 0x3000
		}
		if to != want {
			t.Errorf("%#04X %q maps to %#04X, want %#04X", r, r, to, want)
		}
	}
}
