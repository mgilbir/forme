package layout

import (
	"strings"
	"testing"
)

// A word is not a box, and automatic hyphenation is about words.
//
// "high<span>way</span>" is one word written in two, and hyphens-span-002
// writes it seven ways — around the whole word, around each half, with an empty
// span between the halves — and asks for one answer from all of them.
// hyphens-out-of-flow-002 does the same with an absolutely positioned element
// between the letters, which is not in the text at all.
//
// So the points cannot be worked out where the pieces are built, which happens
// one text node at a time and cannot see the node after. They are gathered in a
// pass of its own over the inline subtree — see hyphenwords.go — and this is
// what that pass is for.

// hyphLines is the lines of a document with "hyphens: auto" on a box six
// characters wide, which is where "highway" divides.
func hyphLines(t *testing.T, inner string, extra string) []string {
	t.Helper()
	return lineTextsOf(t, layoutOf(t, 600, `<div id="d" lang="en">`+inner+`</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 72px; hyphens: auto }`+extra), "d")
}

func joined(lines []string) string { return strings.Join(lines, "|") }

// TestAWordIsDividedWhereTheDictionarySays.
func TestAWordIsDividedWhereTheDictionarySays(t *testing.T) {
	if got := joined(hyphLines(t, "highway", "")); got != "high-|way" {
		t.Errorf("\"highway\" set as %q, want \"high-|way\"", got)
	}
	// And every point is offered, not only the first: "hyphenation" is
	// hy-phen-ation, and six characters holds "hy-" and then "phen-".
	if got := joined(hyphLines(t, "hyphenation", "")); got != "hy-|phen-|ation" {
		t.Errorf("\"hyphenation\" set as %q, want \"hy-|phen-|ation\"", got)
	}
}

// TestAWordWrittenInSeveralBoxesIsStillOneWord is hyphens-span-002's own list.
func TestAWordWrittenInSeveralBoxesIsStillOneWord(t *testing.T) {
	for _, inner := range []string{
		"highway",
		"<span>high</span>way",
		"high<span></span>way",
		"high<span>way</span>",
		"<span>high</span><span>way</span>",
		"<span>highwa</span>y",
		"<span>hi</span>ghway",
		"<b>h</b><i>i</i><b>g</b><i>h</i>way",
	} {
		if got := joined(hyphLines(t, inner, "")); got != "high-|way" {
			t.Errorf("%q set as %q, want \"high-|way\" — it is the same word however "+
				"many boxes it is written in", inner, got)
		}
	}
}

// TestAnOutOfFlowBoxDoesNotDivideAWord is hyphens-out-of-flow-002. An overlay
// hung off the middle of a word is not in the word: nothing of it sits between
// the letters either side.
func TestAnOutOfFlowBoxDoesNotDivideAWord(t *testing.T) {
	for _, css := range []string{"position:absolute", "position:fixed", "float:left"} {
		inner := `high<span style="` + css + `">x</span>way`
		if got := joined(hyphLines(t, inner, "")); got != "high-|way" {
			t.Errorf("with %q between the letters the word set as %q, want \"high-|way\"",
				css, got)
		}
	}
}

// TestAPictureDividesAWord is the containment. An atomic inline is a thing on
// the line and not a letter, so the letters either side of it are not one word
// — and a dictionary asked about "highway" would answer about text that is not
// there.
//
// The split is "highwa" and "y" rather than "high" and "way", and that is what
// makes it a test: neither half hyphenates on its own, and the whole word's
// point is at 4, inside the first half. So a hyphen after "high" can only come
// from the two halves having been read as one word.
func TestAPictureDividesAWord(t *testing.T) {
	//
	// Five characters wide, so that a hyphen after "high" would fit and the six
	// letters before the box would not: whether the two halves are one word is
	// the difference between an overflowing line and a broken one.
	inner := `highwa<span class="ib"></span>y`
	got := joined(hyphLines(t, inner,
		` #d { width: 60px } .ib { display: inline-block; width: 1px; height: 1px }`))
	if strings.Contains(got, "-") {
		t.Errorf("a word divided by an inline-block was hyphenated: %q", got)
	}
}

// TestABlockEndsTheWord, for the same reason and one further out: nothing spans
// two inline formatting contexts, and a word that appeared to would be divided
// by letters in a different paragraph.
func TestABlockEndsTheWord(t *testing.T) {
	// The same split as above and for the same reason: the point is at 4, which
	// is inside the first block's own text, so a hyphen there can only come
	// from the two blocks having been read as one word.
	root := layoutOf(t, 600,
		`<div id="d" lang="en">highwa</div><div id="q" lang="en">y</div>`,
		`#d, #q { font-family: Courier; font-size: 20px; width: 60px; hyphens: auto }`)
	if got := joined(lineTextsOf(t, root, "d")); got != "highwa" {
		t.Errorf("the first block set as %q, want \"highwa\"; the block after it is "+
			"not part of the word", got)
	}
}

// TestLineBreakAnywhereTurnsHyphenationOff. §5.3 says it in three words —
// "Hyphenation is not applied" — and the reason is visible: the value already
// offers a break between every pair of characters, so what a hyphenation point
// would add is not an opportunity but a *hyphen*, printed at a break the value
// would have taken anyway.
//
// line-break-anywhere-002 is a column one character wide with a green rectangle
// behind it, and a hyphen is a character sticking out of the column.
func TestLineBreakAnywhereTurnsHyphenationOff(t *testing.T) {
	// One character wide, which is the reftest's own shape: every character
	// takes a line of its own, and a line that ends at a hyphenation point
	// prints a hyphen beside it — outside the column, where the reftest's green
	// rectangle is not.
	got := joined(hyphLines(t, "hyphenation",
		` #d { width: 12px; line-break: anywhere }`))
	if strings.Contains(got, "-") {
		t.Errorf("line-break: anywhere printed a hyphen: %q", got)
	}
	// And the same document without the value does print them, so this is a
	// difference the value makes rather than a width that hides one.
	if got := joined(hyphLines(t, "hyphenation", ` #d { width: 12px }`)); !strings.Contains(got, "-") {
		t.Errorf("without line-break: anywhere the column set %q and printed no "+
			"hyphen, so the test above measures nothing", got)
	}
}

// TestABoxThatDidNotAskIsNotDivided, and it ends the word rather than joining
// it: a span that asked for its own letters not to be divided has asked for the
// word it is half of not to be divided there either.
func TestABoxThatDidNotAskIsNotDivided(t *testing.T) {
	for _, inner := range []string{
		`<span style="hyphens:none">high</span>way`,
		`high<span style="hyphens:none">way</span>`,
		`<span style="hyphens:manual">high</span>way`,
	} {
		if got := joined(hyphLines(t, inner, "")); strings.Contains(got, "-") {
			t.Errorf("%q was hyphenated as %q", inner, got)
		}
	}
}

// TestASoftHyphenIsStillAnOpportunity. §6.1 does not make the dictionary's
// points replace the author's mark: both are places the word may break.
func TestASoftHyphenIsStillAnOpportunity(t *testing.T) {
	// "supercali" has no automatic point in the first six characters that would
	// beat the mark, and the mark is where the line ends.
	got := joined(hyphLines(t, "cat&shy;alogue", ""))
	if !strings.HasPrefix(got, "cat­-|") {
		t.Errorf("a soft hyphen in an auto-hyphenated word set %q; the mark is still "+
			"an opportunity", got)
	}
}
