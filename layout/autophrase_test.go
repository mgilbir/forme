package layout

import (
	"strings"
	"testing"
)

// CSS Text 4 §5.2's "word-break: auto-phrase", of which this engine does one
// half.
//
// The value has two effects and they are separable. It allows a line to end
// only at a phrase boundary, which needs a morphological analysis of Japanese
// and is not implemented. And it *suppresses hyphenation*, which needs nothing:
// a word divided at a phrase boundary should not also be divided inside itself,
// so a hyphen becomes a break of last resort rather than an opportunity like
// any other.
//
// The suite writes both halves. word-break-auto-phrase-006 is three boxes at
// "width: min-content" — one with soft hyphens, one hyphenated automatically,
// one with hyphens off — and asks for all three to be the same size. -008 sets
// "width: 0" and asks for the word to be hyphenated anyway, because the
// alternative is a line that overflows.

const phraseCSS = `body { margin: 0 }
	div { font-family: Courier; font-size: 20px; line-height: 20px;
	      hyphens: manual; width: min-content }`

// softWord is "consideration" with a soft hyphen at each of the three places
// the word divides: thirteen letters, and four pieces.
const softWord = "con\u00ADsid\u00ADera\u00ADtion"

// phraseWidth is the width a box at "width: min-content" comes to around a
// word, which is the width of its widest unbreakable piece.
func phraseWidth(t *testing.T, css, body string) float64 {
	t.Helper()
	root := layoutOf(t, 400, `<div id="d" style="`+css+`">`+body+`</div>`, phraseCSS)
	return find(t, root, "d").BorderRect.W.Px()
}

// TestAutoPhraseSuppressesHyphenation.
func TestAutoPhraseSuppressesHyphenation(t *testing.T) {
	whole := phraseWidth(t, "", "consideration")
	if whole != 13*12 {
		t.Fatalf("thirteen Courier characters shrink-wrap to %gpx, want 156; the "+
			"fixture cannot say what it means to say", whole)
	}
	if got := phraseWidth(t, "", softWord); got >= whole {
		t.Fatalf("with soft hyphens and no word-break the word shrink-wraps to "+
			"%gpx, want less than %g — it is divisible, so its narrowest form is "+
			"one of its pieces", got, whole)
	}
	if got := phraseWidth(t, "word-break: auto-phrase", softWord); got != whole {
		t.Errorf("under auto-phrase the same word shrink-wraps to %gpx, want %g — "+
			"the hyphens are given up, so the word is one unbreakable run",
			got, whole)
	}
}

// TestAutoPhraseGivesUpWhereTheWordWouldOverflow.
//
// §5.2's suppression is not a prohibition. A word that fits nowhere else is
// still divided, because the alternative is a line that overflows — which is
// what -008 asks for by name.
func TestAutoPhraseGivesUpWhereTheWordWouldOverflow(t *testing.T) {
	lines := func(css string) int {
		root := layoutOf(t, 400,
			`<div id="d" style="width: 40px;`+css+`">`+softWord+`</div>`, phraseCSS)
		return len(find(t, root, "d").Lines)
	}
	plain := lines("")
	if plain < 4 {
		t.Fatalf("with no word-break the word broke into %d lines, want at least 4 "+
			"— forty pixels holds three Courier characters", plain)
	}
	if got := lines("word-break: auto-phrase"); got != plain {
		t.Errorf("under auto-phrase the word broke into %d lines and without it %d; "+
			"a word that fits nowhere else is divided either way", got, plain)
	}
}

// TestAutoPhraseGivesUpAsLateAsItCan.
//
// Giving up is not giving up on the line. Where a word has to be divided the
// division goes as far along as it fits: "consideration" in a hundred pixels is
// "con-sid-" and not "con-", because the second hyphen is no more of a
// concession than the first and the line has room for it.
//
// It is the case that says the two markers have to be kept apart. A hyphen
// earlier in the line is not "anywhere else to end" — it is the same
// concession, one word-piece sooner — so preferring it would divide the word
// *and* leave the line short.
func TestAutoPhraseGivesUpAsLateAsItCan(t *testing.T) {
	root := layoutOf(t, 400,
		`<div id="d" style="width: 100px; word-break: auto-phrase">`+softWord+`</div>`,
		phraseCSS)
	lines := find(t, root, "d").Lines
	if len(lines) < 2 {
		t.Fatalf("the word came to %d lines in a hundred pixels, want at least 2 — "+
			"it is a hundred and fifty-six pixels long", len(lines))
	}
	var first string
	for _, r := range lines[0].Runs {
		first += r.Text
	}
	// The soft hyphens are still in the text — they mark the opportunities and
	// draw nothing — so what is compared is the letters and the hyphen that was
	// actually printed.
	letters := strings.Map(func(r rune) rune {
		if r == 0x00AD {
			return -1
		}
		return r
	}, first)
	if !strings.HasPrefix(letters, "consid") || len(letters) != len("consid")+1 {
		t.Errorf("the first line reads %q, want the word divided after its second "+
			"piece — a hyphen earlier in the line is the same concession one "+
			"piece sooner, not a way of avoiding it", letters)
	}
}

// TestAutoPhrasePrefersAnyOtherOpportunityToAHyphen.
//
// The two markers are kept apart because they are not ordered by position: an
// ordinary opportunity earlier in the line beats a hyphen later in it, which is
// the whole of what suppressing hyphenation means. A greedy fill that took the
// last opportunity that fitted would hyphenate here and fill the line better.
func TestAutoPhrasePrefersAnyOtherOpportunityToAHyphen(t *testing.T) {
	first := func(css string) string {
		root := layoutOf(t, 400,
			`<div id="d" style="width: 100px;`+css+`">ab `+softWord+`</div>`,
			phraseCSS)
		var out []string
		for _, r := range find(t, root, "d").Lines[0].Runs {
			out = append(out, r.Text)
		}
		return strings.TrimSpace(strings.Join(out, ""))
	}
	if got := first(""); got == "ab" {
		t.Fatalf("with no word-break the first line is %q; the fixture needs a "+
			"hyphen that fits on it", got)
	}
	if got := first("word-break: auto-phrase"); got != "ab" {
		t.Errorf("under auto-phrase the first line is %q, want \"ab\" — the space "+
			"is an opportunity and the hyphens are given up, however much better "+
			"they would fill the line", got)
	}
}

// TestAutoPhraseFallsBackToAHyphenFromEverywhereALineRewinds.
//
// A line rewinds from more than one place, and a target it can reach from one
// it has to reach from all of them. The second place is an item that begins no
// opportunity of its own — "there is none between a span and the text after it"
// — where a line that cannot hold it goes back to the last opportunity it had.
//
// With the hyphens kept apart from the ordinary opportunities, a line whose only
// opportunities are hyphens has nothing recorded there, and without the
// fall-back it does not go back at all: it overflows instead of giving up on the
// suppression, which is the one thing §5.2 says it must not do.
func TestAutoPhraseFallsBackToAHyphenFromEverywhereALineRewinds(t *testing.T) {
	root := layoutOf(t, 400,
		`<div id="d" style="width: 180px; word-break: auto-phrase">`+
			softWord+`<span>x</span>yyyy</div>`, phraseCSS)
	d := find(t, root, "d")
	if len(d.Lines) < 2 {
		t.Fatalf("the content came to %d lines in a hundred and eighty pixels, "+
			"want at least 2 — it is two hundred and sixteen pixels long",
			len(d.Lines))
	}
	var first string
	for _, r := range d.Lines[0].Runs {
		first += r.Text
	}
	if got := len(strings.Map(func(r rune) rune {
		if r == 0x00AD {
			return -1
		}
		return r
	}, first)); got > 14 {
		t.Errorf("the first line reads %q — %d characters in a hundred and eighty "+
			"pixels, which holds fifteen. It did not go back to the hyphen it had, "+
			"so it overflowed rather than giving up on the suppression", first, got)
	}
}

// TestAutoPhraseIsReportedOnlyWhereItsOtherHalfWouldShow.
//
// The half that is missing is about phrases, and a paragraph with no Japanese
// in it has none. Reporting it there is a finding about a page that is right,
// in the one channel that says what a page is missing.
func TestAutoPhraseIsReportedOnlyWhereItsOtherHalfWouldShow(t *testing.T) {
	said := func(body string) string {
		for _, f := range findingsOf(t,
			`<div style="word-break: auto-phrase">`+body+`</div>`, "") {
			if f.Property == "word-break" {
				return f.Message
			}
		}
		return ""
	}
	if got := said("consideration"); got != "" {
		t.Errorf("a paragraph of English under auto-phrase was reported as %q; "+
			"there are no phrases in it for the missing half to be about", got)
	}
	if got := said("\u65e5\u672c\u8a9e\u306e\u6587"); !strings.Contains(got, "auto-phrase") {
		t.Errorf("a paragraph of Japanese under auto-phrase was reported as %q, "+
			"which does not name the value", got)
	}
	// And the mixture: one ideograph is enough, because one phrase is.
	if got := said("english \u6587 english"); !strings.Contains(got, "auto-phrase") {
		t.Errorf("a paragraph with one ideograph in it was reported as %q", got)
	}
}

// TestAutoPhraseSuppressesOnlyTheHyphens.
//
// "auto-phrase" is on the box and every item in it carries the flag, so a
// suppression that did not ask what the opportunity *is* would suppress every
// one of them. An ideograph boundary, a <wbr> and a U+200B ZERO WIDTH SPACE are
// opportunities the script or the author put there, and none of them is a
// hyphen; a minimum width that ran past them is a box wider than the narrowest
// thing it can hold.
func TestAutoPhraseSuppressesOnlyTheHyphens(t *testing.T) {
	for _, c := range []struct{ what, body string }{
		{"a space", "ab cdef"},
		{"an ideograph boundary", "abcd\u65e5\u672c"},
		{"a wbr", "ab<wbr>cdef"},
		{"a zero width space", "ab\u200bcdef"},
	} {
		plain := phraseWidth(t, "", c.body)
		phrase := phraseWidth(t, "word-break: auto-phrase", c.body)
		if plain != phrase {
			t.Errorf("%s: the narrowest width is %gpx under auto-phrase and %gpx "+
				"without it; the value gives up hyphens and nothing else",
				c.what, phrase, plain)
		}
	}
	// And the hyphens really are given up, so this is not passing because
	// nothing is suppressed at all.
	if phraseWidth(t, "word-break: auto-phrase", softWord) ==
		phraseWidth(t, "", softWord) {
		t.Error("a word divided only by soft hyphens has the same narrowest width " +
			"either way, so the check above says nothing")
	}
}
