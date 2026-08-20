package layout

import (
	"strings"
	"testing"
)

// line-break, CSS Text §5.3.
//
// All four values are implemented: "anywhere" adds opportunities everywhere, and
// the other three tailor where Chinese and Japanese may break — see
// paragraph/linebreakstrictness_test.go for the tailoring itself. What is here
// is what only a document can show: that a prohibition reaches across a box
// boundary, and that none of the four is reported as unimplemented any more.

// TestLineBreakAnywhereBreaksBeforeAPreservedSpace is the value at the point it
// differs from everything else that adds opportunities.
//
// §5.3: "There is a soft wrap opportunity around every typographic character
// unit, including around any punctuation character or preserved white spaces."
// Around, so before as well as after — and before a *space* is the part neither
// break-all nor break-spaces will give, since UAX #14's LB7 keeps a space with
// the word in front of it.
//
// The three-way comparison is the assertion. The same six characters in the same
// four characters of room break in three different places, so an engine that
// read "anywhere" as either of its neighbours would give one of the other two
// answers here.
func TestLineBreakAnywhereBreaksBeforeAPreservedSpace(t *testing.T) {
	const src = `<p id="p">X XX X</p>`

	// break-spaces alone: only after a space, so the first line stops after the
	// first one and the rest fits exactly.
	root := layoutOf(t, 10000, src, widthCSS(4, "white-space: break-spaces"))
	if got := lineTexts(linesOf(t, root, "p")); len(got) != 2 ||
		got[0] != "X " || got[1] != "XX X" {
		t.Errorf("break-spaces gave %q, want [\"X \" \"XX X\"]", got)
	}

	// word-break: break-all adds opportunities inside the word but still none
	// before a space, so the line stops one character short of full: it may end
	// between the two X's and not between the second and the space after them.
	root = layoutOf(t, 10000, src,
		widthCSS(4, "white-space: break-spaces; word-break: break-all"))
	if got := lineTexts(linesOf(t, root, "p")); len(got) != 2 ||
		got[0] != "X X" || got[1] != "X X" {
		t.Errorf("break-all gave %q, want [\"X X\" \"X X\"]", got)
	}

	// line-break: anywhere does allow it, so the line takes all four characters
	// it has room for and the space that follows starts the next one.
	root = layoutOf(t, 10000, src,
		widthCSS(4, "white-space: break-spaces; line-break: anywhere"))
	if got := lineTexts(linesOf(t, root, "p")); len(got) != 2 ||
		got[0] != "X XX" || got[1] != " X" {
		t.Errorf("line-break: anywhere gave %q, want [\"X XX\" \" X\"]", got)
	}
}

// TestLineBreakAnywhereSplitsARunOfPreservedSpaces: "around every typographic
// character unit" reaches inside a run of them too.
//
// Under pre-wrap a run of preserved spaces is one unit — it hangs or wraps
// whole — and that is the thing "anywhere" takes apart.
func TestLineBreakAnywhereSplitsARunOfPreservedSpaces(t *testing.T) {
	pieces, _ := splitAtBreaks("a    b", whiteSpaceOf("preserve"), wordBreak{},
		lineBreak{Anywhere: true}, hyphens{})
	var spaces int
	for _, p := range pieces {
		if p.Space {
			spaces++
			if p.Text != " " {
				t.Errorf("a space piece is %q, want a single space", p.Text)
			}
		}
	}
	if spaces != 4 {
		t.Errorf("four preserved spaces came to %d pieces, want 4", spaces)
	}

	// Without it they are one piece, which is what pre-wrap means.
	pieces, _ = splitAtBreaks("a    b", whiteSpaceOf("preserve"), wordBreak{},
		lineBreak{}, hyphens{})
	for _, p := range pieces {
		if p.Space && p.Text != "    " {
			t.Errorf("pre-wrap gathered the run into %q, want all four", p.Text)
		}
	}
}

// TestLineBreakAnywhereBreaksMidWord: the "or in the middle of words" half,
// which is where it overlaps break-all — and unlike break-all it needs no help
// from overflow-wrap.
func TestLineBreakAnywhereBreaksMidWord(t *testing.T) {
	root := layoutOf(t, 10000, `<p id="p">abcdef</p>`,
		widthCSS(3, "line-break: anywhere"))
	if got := lineTexts(linesOf(t, root, "p")); len(got) != 2 ||
		got[0] != "abc" || got[1] != "def" {
		t.Errorf("line-break: anywhere gave %q, want [\"abc\" \"def\"]", got)
	}
}

// TestTheOtherLineBreakValuesAreReportedOnlyOverCJK is the reporting rule, and
// the quiet half is the one worth pinning.
//
// This file's subject used to be that loose, normal and strict were read as
// auto and reported over CJK text. They are implemented now — see
// paragraph/linebreak.go for §5.3's tailoring — so none of the four is reported
// at all, and what is left to hold is that nothing is said about any of them
// over any text.
//
// The Latin half of the old test is worth keeping for the reason it was written:
// the suite has three tests (pre-wrap-004, -005 and -006) whose whole assertion
// is that "XX    XX" wraps the same under loose as under auto, and a warning
// there would have been a false one.
func TestTheOtherLineBreakValuesAreReportedOnlyOverCJK(t *testing.T) {
	reported := func(text, value string) string {
		t.Helper()
		rec := NewRecorder(nil)
		built := Build(Input{
			HTML: `<div id="p">` + text + `</div>`,
			CSS:  []Stylesheet{{Source: `#p { width: 40px; line-break: ` + value + ` }`}},
		})
		Layout(built.Root, Size{W: picPx(600), H: picPx(10000)}, StandardFonts(), rec)
		for _, f := range rec.Findings() {
			if f.Property == "line-break" {
				return f.Message
			}
		}
		return ""
	}
	// All four values are implemented, so no text makes any of them worth
	// reporting.
	for _, value := range []string{"loose", "normal", "strict", "anywhere"} {
		for _, text := range []string{"XX    XX", "日本語のテキスト"} {
			if got := reported(text, value); got != "" {
				t.Errorf("line-break: %s was reported over %q: %s", value, text, got)
			}
		}
	}
}

// A prohibition that has to cross a box boundary.
//
// UAX #14 says a line may not begin with a closing bracket, a non-starter or a
// hyphen. Inside one run SplitAtBreaks withholds the opportunity in front of
// such a character as it meets it — but an opportunity can arrive from the box
// *before*, where there is no character left to test it against: an ideograph at
// the end of one text node offers a break, and the next node may be a <span>.
//
// It is not a corner. "中中<span>〜</span>文" is what a test that wants to colour
// the character writes, and the suite's whole line-break strictness family is
// that shape — forty fixtures, each with the character under test in an element
// of its own.

// cjkLines is the text of each line of a CJK paragraph narrow enough to wrap.
func cjkLines(t *testing.T, markup, css string) []string {
	t.Helper()
	root := layoutOf(t, 10000, `<p id="p">`+markup+`</p>`,
		noDefaults+`#p { font-family: Courier; font-size: 20px; width: 60px }`+css)
	return lineTexts(linesOf(t, root, "p"))
}

// TestAProhibitionCrossesABoxBoundary. The same text written with and without a
// span must break in the same place, because a <span> is not something a reader
// can see.
func TestAProhibitionCrossesABoxBoundary(t *testing.T) {
	for _, tc := range []struct{ mark, what string }{
		{"々", "an ideographic iteration mark"},
		{"）", "a fullwidth closing parenthesis"},
		{"…", "an ellipsis"},
	} {
		// Five characters to the line, and the mark is the sixth: the break
		// falls exactly where the rule has something to say about it.
		plain := cjkLines(t, "中中中中中"+tc.mark+"文", "")
		spanned := cjkLines(t, "中中中中中<span>"+tc.mark+"</span>文", "")
		if len(plain) != len(spanned) {
			t.Errorf("%s: %d lines written plainly and %d written in a span: %q vs %q",
				tc.what, len(plain), len(spanned), plain, spanned)
			continue
		}
		for i := range plain {
			if strings.TrimSpace(plain[i]) != strings.TrimSpace(spanned[i]) {
				t.Errorf("%s: line %d is %q written plainly and %q in a span",
					tc.what, i, plain[i], spanned[i])
			}
		}
	}
}

// TestTheStrictnessReachesAcrossABoxBoundaryToo, which is what the suite's
// fixtures need: the character whose class the value tailors is in a span.
func TestTheStrictnessReachesAcrossABoxBoundaryToo(t *testing.T) {
	// A small kana. strict forbids a line beginning with one; normal allows it.
	const doc = "中中中中中<span>ぁ</span>文"
	strict := cjkLines(t, doc, `#p { line-break: strict }`)
	normal := cjkLines(t, doc, `#p { line-break: normal }`)
	if len(strict) == 0 || len(normal) == 0 {
		t.Fatalf("no lines: %q %q", strict, normal)
	}
	if strings.HasPrefix(strict[len(strict)-1], "ぁ") {
		t.Errorf("under strict a line begins with a small kana: %q", strict)
	}
	if !strings.HasPrefix(normal[len(normal)-1], "ぁ") {
		t.Errorf("under normal no line begins with the small kana, so the two "+
			"values gave the same answer and this fixture proves nothing: %q", normal)
	}
}

// TestAnOpportunityFromASpaceIsNotWithheld is the containment case, and it is
// the one that had to be got right rather than assumed.
//
// The prohibition is applied inside a run only to the opportunities an ideograph
// defers, and not to the break after a space: "AA )BB" has always broken after
// the space. An opportunity crossing a boundary has to be held to the same rule,
// or the same text answers differently depending on whether the author wrote a
// span. TestOnlyAPicturesOwnOpportunityIsHeld in atomicbreak_test.go is the
// other half of it and was written long before this.
func TestAnOpportunityFromASpaceIsNotWithheld(t *testing.T) {
	for _, tc := range []struct{ mark, what string }{
		{")", "a closing parenthesis"},
		{"⁠", "a word joiner"},
		{"…", "an ellipsis"},
	} {
		plain := cjkLines(t, "AA "+tc.mark+"BB", "")
		spanned := cjkLines(t, "AA <span>"+tc.mark+"BB</span>", "")
		if len(plain) != len(spanned) {
			t.Errorf("%s: %d lines written plainly and %d in a span: %q vs %q",
				tc.what, len(plain), len(spanned), plain, spanned)
		}
	}
}
