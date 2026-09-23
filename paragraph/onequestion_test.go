package paragraph

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Questions about text that two functions used to answer two ways. Each test
// writes the expected characters down, because a test that only asked the two
// former answers to agree would pass when both were wrong; where two paths still
// exist, it asks them the same input as well.

// TestTheContextAfterAWindowOfWideCharactersIsKept is audit C117. The window is
// 128 bytes; it is backed up to a character boundary and then to a cluster
// boundary, and it was backed up to the last ASCII byte instead — nothing, for
// any run of Arabic, Hebrew or CJK longer than the window.
func TestTheContextAfterAWindowOfWideCharactersIsKept(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
	}{
		// 128 bytes is 64 Arabic letters exactly; the last cluster of the window
		// is given up because the text may continue it.
		{"Arabic", strings.Repeat("ب", 100), strings.Repeat("ب", 63)},
		// 128 bytes is 42 ideographs and two bytes of a 43rd.
		{"CJK", strings.Repeat("漢", 100), strings.Repeat("漢", 41)},
		{"Hebrew", strings.Repeat("א", 100), strings.Repeat("א", 63)},
		// And the text that was always right, which the fix must not move.
		{"Latin", strings.Repeat("a", 200), strings.Repeat("a", 127)},
		{"short", "بب", "بب"},
	} {
		if got := ContextAfter(tc.text); got != tc.want {
			t.Errorf("%s: ContextAfter kept %d bytes (%q…), want %d", tc.name,
				len(got), firstN(got, 12), len(tc.want))
		}
		if got := ContextAfter(tc.text); !utf8.ValidString(got) || !strings.HasPrefix(tc.text, got) {
			t.Errorf("%s: %q is not a prefix of the text made of whole characters", tc.name, got)
		}
	}
	// End to end, as §5.4 is about it: the head of a long Arabic word cut for a
	// line is shaped with the rest of the word after it.
	br := NewBreaker(nil)
	item := Item{Text: strings.Repeat("ب", 180)}
	if head := br.SplitHead(item, len("ب")*20); head.PostContext == "" {
		t.Error("the head of a 180-letter Arabic word is shaped with nothing after it")
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// TestALineEndLooksThroughTheSameThingsItTrims is audit C118. The white space
// that ends a line (isLineTailSpace) and the white space removed from it
// (trimLineEdge) disagreed about a bidi control: the trim looked through one
// and the break decision stopped at it, so the space in front of a control
// counted towards the width and pushed the control onto a line of its own.
func TestALineEndLooksThroughTheSameThingsItTrims(t *testing.T) {
	face := courier(t)
	word := Item{Text: "aaaa", Face: face, Size: u(size20), Width: u(48)}
	space := Item{Text: " ", Face: face, Size: u(size20), Width: u(12),
		Space: true, Collapsible: true, TrimAtEnd: true}
	for _, control := range []string{"\u202c", "\u2069", "\u200f"} {
		ctl := Item{Text: control, Face: face, Size: u(size20), BreakBefore: true}
		items := []Item{word, space, ctl}
		// Room for the word and not for the word and its space: the one line
		// the text makes, with the space trimmed off it.
		line, next, _, _, _, _ := NewBreaker(nil).BreakOneLine(items, 0, 0, u(50), 0)
		if next != len(items) {
			t.Errorf("%U after a trailing space: the line ended at item %d of %d, "+
				"so the control went to a line of its own", []rune(control)[0], next, len(items))
		}
		if got := lineText(line); got != "aaaa"+control {
			t.Errorf("%U: the line reads %q, want the word and the control", []rune(control)[0], got)
		}
	}
	// And the question itself, asked of every kind of item both places see: an
	// item the trim looks through, leaving the space in front of it removed, is
	// one the break decision counts as the line's trailing white space.
	for _, it := range []Item{
		{Text: "\u202C"}, {Text: "\u2066\u2069"}, {Inset: true}, {Text: "a"},
		{Text: "\u200B"}, {Text: "b\u202C"},
	} {
		trimmed := trimLineEdge([]Item{word, space, it})
		looksThrough := len(trimmed) == 2 && trimmed[0].Text == "aaaa"
		if looksThrough != isLineTailSpace(it) {
			t.Errorf("%q: the trim looks through it %v, and the break decision counts "+
				"it as line-end white space %v", it.Text, looksThrough, isLineTailSpace(it))
		}
	}
}

// TestAHyphenMayEndALineInFrontOfANoBreakSpace is audit C175: UAX #14's LB12a
// is "[^SP BA HY] × GL", which exempts a hyphen and a soft hyphen. The gate
// that withholds a hyphen's opportunity where white space follows asked
// unicode.IsSpace, which holds the three no-break spaces.
func TestAHyphenMayEndALineInFrontOfANoBreakSpace(t *testing.T) {
	ws := WhiteSpace{Collapse: true, Wrap: true}
	for _, tc := range []struct{ text, want, what string }{
		{"ab-\u00a0cd", "ab-|\u00a0cd", "a no-break space"},
		{"ab-\u2007cd", "ab-|\u2007cd", "a figure space"},
		{"ab-\u202fcd", "ab-|\u202fcd", "a narrow no-break space"},
		{"ab\u00ad\u00a0cd", "ab\u00ad|\u00a0cd", "a soft hyphen before a no-break space"},
		// White space still withholds it: the break is after the space.
		{"ab- cd", "ab- |cd", "a space"},
		{"ab-\u2003cd", "ab-\u2003|cd", "an em space, which is class BA"},
		{"ab-\u3000cd", "ab-\u3000|cd", "an ideographic space, likewise"},
	} {
		pieces, _ := SplitAtBreaks(tc.text, ws, WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
		var parts []string
		for _, p := range pieces {
			if p.BreakBefore && len(parts) > 0 {
				parts = append(parts, "|")
			}
			parts = append(parts, p.Text)
		}
		if got := strings.Join(parts, ""); got != tc.want {
			t.Errorf("%s: %q breaks as %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestGeorgianIsNotUppercasedWhateverTheLanguage is audit C120: the per-
// character path the Turkish i sends the rest of the text down did not leave
// Mkhedruli alone, where the whole-string path and the Greek path did.
func TestGeorgianIsNotUppercasedWhateverTheLanguage(t *testing.T) {
	for _, tc := range []struct {
		text string
		lang Language
		want string
	}{
		{"i ა α", "tr", "İ ა Α"},
		{"i ა α", "az", "İ ა Α"},
		{"i ა α", "", "I ა Α"},
		{"i ა α", "el", "I ა Α"},
		{"i\u0307 ა", "lt", "I ა"},
	} {
		if got, _ := TransformText(tc.text, TransformUppercase, WordClosed, tc.lang); got != tc.want {
			t.Errorf("%q uppercased in %q is %q, want %q", tc.text, tc.lang, got, tc.want)
		}
	}
	// The three paths asked the same character: whole string, per character
	// (from a Turkish i in front of it, which is what sends the rest down that
	// path) and Greek.
	for _, r := range []rune{'ა', 'ჰ', 'ჿ', 'ß', 'a'} {
		plain, _ := TransformText(string(r), TransformUppercase, WordClosed, "")
		turkish, _ := TransformText("i"+string(r), TransformUppercase, WordClosed, "tr")
		greek, _ := TransformText(string(r), TransformUppercase, WordClosed, "el")
		if turkish != "İ"+plain || greek != plain {
			t.Errorf("%q uppercases to %q alone, %q after a Turkish i and %q in Greek",
				r, plain, turkish, greek)
		}
	}
}

// TestCapitalizeHonoursTheLanguageAsUppercaseDoes is audit C121: CSS Text §2.1
// makes all three case changes language-sensitive, and capitalize consulted
// only the titlecase table.
func TestCapitalizeHonoursTheLanguageAsUppercaseDoes(t *testing.T) {
	for _, tc := range []struct {
		text string
		lang Language
		want string
	}{
		{"istanbul izmir", "tr", "İstanbul İzmir"},
		{"istanbul izmir", "az", "İstanbul İzmir"},
		{"istanbul izmir", "", "Istanbul Izmir"},
		// Lithuanian removes the dot above a titlecased i: SpecialCasing's
		// titlecase field for it is empty.
		{"i\u0307x", "lt", "Ix"},
		{"i\u0307x", "", "I\u0307x"},
		// A combining mark belongs to the letter before it and does not end the
		// word (UAX #29's WB4): a decomposed résumé has one capital.
		{"re\u0301sume\u0301 x", "", "Re\u0301sume\u0301 X"},
	} {
		if got, _ := TransformText(tc.text, TransformCapitalize, WordClosed, tc.lang); got != tc.want {
			t.Errorf("%q capitalised in %q is %q, want %q", tc.text, tc.lang, got, tc.want)
		}
	}
}

// TestAFinalSigmaIsDecidedAcrossTextNodes is audit C176: Final_Sigma looks both
// ways, and each way can be in another text node. The look back is carried in;
// the look forward is left open and settled by the node after.
func TestAFinalSigmaIsDecidedAcrossTextNodes(t *testing.T) {
	lower := TransformLowercase
	// One node, the context complete: as it always was.
	if got, _ := TransformText("ΟΔΟΣ ΑΚΙ", lower, WordClosed, ""); got != "οδος ακι" {
		t.Errorf("one node: %q", got)
	}
	// The sigma ends its node: final for now, and said so.
	out, _, ctx, open := TransformTextIn("ΟΔΟΣ", lower, WordClosed, "", CaseContext{})
	if out != "οδος" || open != len("οδο") || !ctx.CasedBefore {
		t.Fatalf("ΟΔΟΣ: %q, open at %d, context %+v; want οδος open at %d and cased",
			out, open, ctx, len("οδο"))
	}
	for _, tc := range []struct {
		next, want string
		decided    bool
	}{
		{"ΑΚΙ", "οδοσ", true},     // a cased letter follows: not final
		{"'ΑΚΙ", "οδοσ", true},    // through a case-ignorable apostrophe
		{",", "οδος", true},       // not cased: final
		{".", "οδος", false},      // a full stop is case-ignorable (MidNumLet)
		{" ΑΚΙ", "οδος", true},    // a space ends the word
		{"\u0301", "οδος", false}, // all case-ignorable: still open
		{"", "οδος", false},       // nothing at all: still open
		{"\u200bΑ", "οδοσ", true}, // a zero width space is case-ignorable (Cf)
		{"1", "οδος", true},       // a digit is not cased
		{"ά", "οδοσ", true},       // lower case is cased too
		{"ΑΚΙ ΣΑΣ", "οδοσ", true},
	} {
		isCased, decided := CasedAhead(tc.next)
		got := out
		if decided && isCased {
			got = UnfinalSigma(out, open)
		}
		if got != tc.want || decided != tc.decided {
			t.Errorf("followed by %q: %q (decided %v), want %q (decided %v)",
				tc.next, got, decided, tc.want, tc.decided)
		}
	}
	// The look back: a node that begins with a sigma after a cased letter in
	// the node before is final; after nothing cased, it is not.
	if got, _, _, open := TransformTextIn("Σ", lower, WordClosed, "", CaseContext{CasedBefore: true}); got != "ς" || open != 0 {
		t.Errorf("Σ after a cased letter: %q, open at %d", got, open)
	}
	if got, _, _, open := TransformTextIn("Σ", lower, WordClosed, "", CaseContext{}); got != "σ" || open != -1 {
		t.Errorf("Σ with nothing before: %q, open at %d", got, open)
	}
	// The context a node leaves is its own last character that is not case-
	// ignorable, and what it was given where it has none.
	for _, tc := range []struct {
		text string
		in   bool
		want bool
	}{
		{"α", false, true}, {"1", true, false}, {"α'", false, true},
		{"\u0301", true, true}, {"\u0301", false, false}, {"", true, true},
	} {
		if _, _, ctx, _ := TransformTextIn(tc.text, TransformNone, WordClosed, "", CaseContext{CasedBefore: tc.in}); ctx.CasedBefore != tc.want {
			t.Errorf("%q after cased=%v leaves cased=%v", tc.text, tc.in, ctx.CasedBefore)
		}
	}
	// And a correction at an offset that is not a ς changes nothing.
	if got := UnfinalSigma("οδος", 0); got != "οδος" {
		t.Errorf("UnfinalSigma at a non-sigma rewrote the text: %q", got)
	}
}

// TestAPhraseIsScoredAcrossABoxBoundary is audit C122. The model reads three
// characters either side of a boundary, and each box was scored alone: the
// boundary at its first character was never scored, and the ones near its
// edges were scored without their neighbours.
func TestAPhraseIsScoredAcrossABoxBoundary(t *testing.T) {
	const whole = "日本語を勉強します"
	wb := WordBreakOf("auto-phrase")
	ws := WhiteSpace{Collapse: true, Wrap: true}
	w := WritingSystemJapanese
	// The whole sentence, split in one box: the opportunity between 勉 and 強
	// is inside a phrase, so it is a last resort.
	pieces, _ := SplitAtBreaks(whole, ws, wb, LineBreak{}, Hyphens{}, w)
	rankAt := func(pieces []Piece, text string) (offered, lastResort bool) {
		for _, p := range pieces {
			if strings.HasPrefix(p.Text, text) {
				return p.BreakBefore, p.LastResort
			}
		}
		t.Fatalf("no piece begins %q in %v", text, pieces)
		return
	}
	if offered, last := rankAt(pieces, "強"); !offered || !last {
		t.Fatalf("whole: before 強 offered %v last resort %v; want a withheld opportunity "+
			"— the fixture has lost its premise", offered, last)
	}
	// The same sentence written as two boxes, with the context the layout
	// carries across the boundary.
	first, trailing := SplitAtBreaks("日本語を勉", ws, wb, LineBreak{}, Hyphens{}, w)
	_ = first
	if trailing.PhraseTail != "語を勉" {
		t.Errorf("the first box leaves %q for the phrase model, want %q", trailing.PhraseTail, "語を勉")
	}
	second, _ := SplitAtBreaksAfter("強します", ws, wb, LineBreak{}, Hyphens{}, w, Carried{
		Offered: trailing.Offered, Deferred: trailing.Deferred, Held: trailing.Held,
		Taken: trailing.Taken, Prev: '勉', PhraseBefore: trailing.PhraseTail,
	})
	if offered, last := rankAt(second, "強"); !offered || !last {
		t.Errorf("split before 強: offered %v last resort %v; the same boundary as in the "+
			"whole sentence, and ranked the same", offered, last)
	}
	// And every boundary of the sentence, split at every character, answers as
	// the whole sentence does — and of a second string, which is not Japanese
	// anyone would write and is here because it has a boundary whose reading
	// changes with the characters after a cut, which the first does not.
	for _, whole := range []string{whole, "私ん東ノ。強語常"} {
		scores := PhraseBreaks(whole, w)
		for cut := len("日"); cut < len(whole); cut += len("日") {
			got := PhraseBreaksBetween(lastRunes(whole[:cut], PhraseContext), whole[cut:],
				"", w)
			for at, boundary := range scores {
				if at < cut {
					continue
				}
				if g, ok := got[at-cut]; !ok || g != boundary {
					t.Errorf("cut at %d: the boundary at %d scores %v (%v), whole %v",
						cut, at, g, ok, boundary)
				}
			}
			head := PhraseBreaksBetween("", whole[:cut], FirstRunes(whole[cut:], PhraseContext), w)
			for at, boundary := range scores {
				if at >= cut {
					continue
				}
				if g, ok := head[at]; !ok || g != boundary {
					t.Errorf("%q cut at %d: the boundary at %d in the head scores %v (%v), whole %v",
						whole, cut, at, g, ok, boundary)
				}
			}
		}
	}
}

// TestThePhraseScriptsAreUnicodesOwn is audit C174: the Japanese model's script
// test and the "has phrases" test are the generated script tables, not lists of
// ranges and not UAX #14's ID class.
func TestThePhraseScriptsAreUnicodesOwn(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want bool
		what string
	}{
		{'漢', true, "a Han ideograph"},
		{0x2B740, true, "extension D"},
		{0x30000, true, "extension G, which the typed ranges stopped short of"},
		{0x31350, true, "extension H"},
		{'あ', true, "hiragana"},
		{'ア', true, "katakana"},
		{0x31F0, true, "a katakana phonetic extension"},
		{'、', true, "an ideographic comma, which the model has weights for"},
		{'한', false, "Hangul"},
		{'😀', false, "an emoji, which is class ID"},
		{'a', false, "Latin"},
	} {
		if got := inJapaneseScript(tc.r); got != tc.want {
			t.Errorf("%s (U+%04X): in the model's script %v, want %v", tc.what, tc.r, got, tc.want)
		}
	}
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"漢字", true}, {"かな", true}, {"한국어", false}, {"😀", false}, {"、。", false},
		{string(rune(0x30000)), true},
	} {
		if got := NeedsPhraseBreaking(tc.text); got != tc.want {
			t.Errorf("NeedsPhraseBreaking(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
	if PhrasesUnfound("😀 한국어", WritingSystemChinese) {
		t.Error("a zh box of emoji and Hangul is said to have phrases no model can find")
	}
}
