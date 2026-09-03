package paragraph

import (
	"strings"
	"testing"
)

// word-break: auto-phrase, CSS Text §5.2, and what "withheld" means.
//
// The value allows a line to end at a phrase boundary and suppresses the
// implicit opportunities inside a phrase — which is keep-all with the phrase
// boundaries let back in. The difference from keep-all is what happens to a
// phrase that does not fit: keep-all's suppression is a prohibition and this
// one is a preference. The suite says so by name, in
// word-break-auto-phrase-009's assert: "auto-phrase's must give up on
// suppressing wrapping opportunities when that would lead to overflow."
//
// So an opportunity inside a phrase is *demoted* rather than removed, and these
// tests are written over three symbols rather than two: "|" is an opportunity
// the line takes, "·" is one it reaches for only when it has no other, and
// nothing between two characters is no opportunity at all.

// autoPhrase is read through the parser rather than built here, for the reason
// keepAll is: the value is what a document writes.
func autoPhrase(t *testing.T) WordBreak {
	t.Helper()
	wb, unhandled := WordBreakOf("auto-phrase")
	if unhandled != "" {
		t.Fatalf("auto-phrase was reported as unhandled: %q", unhandled)
	}
	if !wb.AutoPhrase {
		t.Fatal("auto-phrase read as some other value")
	}
	return wb
}

// ranked renders the pieces with a bar at each opportunity the line takes and a
// middle dot at each one it gives up.
func ranked(t *testing.T, text string, wb WordBreak, w WritingSystem) string {
	t.Helper()
	pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true},
		wb, LineBreak{}, Hyphens{}, w)
	var b strings.Builder
	for _, p := range pieces {
		switch {
		case p.BreakBefore && p.LastResort:
			b.WriteString("·")
		case p.BreakBefore:
			b.WriteString("|")
		case p.LastResort:
			t.Errorf("%q: a piece is a last resort without being an opportunity", text)
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

func TestAutoPhraseKeepsAPhraseWholeUntilThatWouldOverflow(t *testing.T) {
	wb := autoPhrase(t)
	for _, tc := range []struct {
		text, want, what string
	}{
		{"東京へ行きましょう。", "東·京·へ|行·き·ま·し·ょ·う。",
			"the boundary the suite's word-break-auto-phrase-001 asks for"},
		{"楽しいドライブ。", "楽·し·い|ド·ラ·イ·ブ。",
			"the phrase the overflow and intrinsic tests are built on"},
		{"楽しいドライブ、楽しいドライブ。", "楽·し·い|ド·ラ·イ·ブ、|楽·し·い|ド·ラ·イ·ブ。",
			"an opportunity a prohibition moved keeps its rank where it lands"},
	} {
		if got := ranked(t, tc.text, wb, WritingSystemJapanese); got != tc.want {
			t.Errorf("%s\n%q\n got %s\nwant %s", tc.what, tc.text, got, tc.want)
		}
	}
}

// The third row above is the one that had to be got right twice, and it is
// worth its own statement. "ドライブ、楽しい" has an opportunity after "ブ" that
// is inside a phrase, so auto-phrase would give it up — and UAX #14 will not
// let a line begin with the comma, so the opportunity moves past it, and where
// it lands is exactly where the model says the next phrase starts. Ranking it
// before the move suppressed it: the comma's opportunity vanished, the line
// broke at the phrase before it, and word-break-auto-phrase-wbr-nobr-002 set
// three lines where it asks for two.
func TestAnOpportunityAProhibitionMovedIsRankedWhereItLands(t *testing.T) {
	const text = "ドライブ、楽しい"
	got := ranked(t, text, autoPhrase(t), WritingSystemJapanese)
	if !strings.Contains(got, "、|楽") {
		t.Errorf("%q: %s — the opportunity after the comma was given up, and it "+
			"is where the next phrase begins", text, got)
	}
}

// §5.2's fallback: the value has effect only where the UA has a model for the
// content language, and otherwise it is "normal". Two of the suite's own
// documents are exactly these two rows.
func TestAutoPhraseIsNormalWhereThereIsNoModelForTheText(t *testing.T) {
	wb := autoPhrase(t)
	for _, tc := range []struct {
		text string
		w    WritingSystem
		what string
	}{
		{"東京へ行きましょう。", WritingSystemOther,
			"content with no language tag — word-break-auto-phrase-fallback-002"},
		{"กรุงเทพคือสวยงาม", WritingSystemJapanese,
			"Thai tagged as Japanese — word-break-auto-phrase-fallback-001"},
	} {
		normal := ranked(t, tc.text, WordBreak{}, tc.w)
		got := ranked(t, tc.text, wb, tc.w)
		if got != normal {
			t.Errorf("%s\n%q\n got %s\nwant %s (which is what normal gives)",
				tc.what, tc.text, got, normal)
		}
		if strings.Contains(got, "·") {
			t.Errorf("%s: %q gave up an opportunity with no model to justify it",
				tc.what, tc.text)
		}
	}
}

// The three classes §5.2 names by their UAX #14 letters, which the suite tests
// one document each: "the UA must not insert a virtual word boundary between a
// typographic letter unit and an adjacent typographic character unit from the
// [UAX14] GL / WJ / ZWJ line breaking class".
//
// What is checked is the boundary either side of the character, and of either
// rank. It is not "the same pieces keep-all gives", which was written first and
// is a different claim: keep-all forbids the opportunities inside a word and
// this value gives them up, so "東京" comes out whole under one and demoted
// under the other. That difference is the value working. A break beside the
// word joiner would be the value failing, and there is none of either kind.
func TestAutoPhraseOffersNothingBesideGlueAWordJoinerOrAZeroWidthJoiner(t *testing.T) {
	wb := autoPhrase(t)
	for _, tc := range []struct {
		text, glue, what string
	}{
		{"東京\u00a0へ\u00a0行きましょう。", "\u00a0", "GL, a no-break space — auto-phrase-003"},
		{"東京\u2060へ\u2060行きましょう。", "\u2060", "WJ, a word joiner — auto-phrase-004"},
		{"東京\u200dへ\u200d行きましょう。", "\u200d", "ZWJ — auto-phrase-005"},
	} {
		got := ranked(t, tc.text, wb, WritingSystemJapanese)
		for _, beside := range []string{"|" + tc.glue, "·" + tc.glue, tc.glue + "|", tc.glue + "·"} {
			if strings.Contains(got, beside) {
				t.Errorf("%s\n%q\n got %s — there is a boundary beside the glue",
					tc.what, tc.text, got)
			}
		}
	}
}

// "UAs must not suppress wrapping opportunities introduced by wbr or ZWSP" —
// word-break-auto-phrase-007, and the reason the suppression lives where it
// does. A zero width space sets the opportunity outright rather than offering
// one to the rules, so nothing in this value can reach it.
func TestAutoPhraseDoesNotSuppressAZeroWidthSpace(t *testing.T) {
	const text = "ドラ​イブ"
	got := ranked(t, text, autoPhrase(t), WritingSystemJapanese)
	// The zero width space is a piece of its own and takes no room, so what the
	// opportunity is in front of is the character after it.
	if !strings.Contains(got, "|イ") {
		t.Errorf("%q: %s — the author's own opportunity was given up", text, got)
	}
}
