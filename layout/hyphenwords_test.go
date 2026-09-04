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

// hyphenate-limit-chars, css-text §6.2: the author overriding the dictionary
// about how much of a word has to stay on each side of the break.
//
// Three numbers — how long a word must be before it may be divided, how many of
// its letters stay on the line, how many go to the next — and "auto" for any of
// them leaves that one to the engine. "auto" is not a number chosen here: it is
// the hyphenmins the language's own pattern file states, which for American
// English is two and three.

// limitLines sets "example" — which the patterns divide as ex-am-ple — one
// character wide, so that every point it keeps shows as a line of its own.
func limitLines(t *testing.T, value string) string {
	t.Helper()
	return strings.Join(lineTextsOf(t, layoutOf(t, 600,
		`<div id="d" lang="en">example</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 12px; hyphens: auto;
		      hyphenate-limit-chars: `+value+` }`), "d"), "|")
}

// TestHyphenateLimitCharsDecidesWhichPointsSurvive is the reftest's own table,
// row for row: nine declarations over the same word, and the reference writes
// out what each must produce.
func TestHyphenateLimitCharsDecidesWhichPointsSurvive(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"auto", "ex-|am-|ple"},
		// The word is seven letters and may not be divided under eight.
		{"8", "example"},
		{"auto 2 2", "ex-|am-|ple"},
		// Three letters before: the point after "ex" goes.
		{"auto 3 2", "exam-|ple"},
		{"auto 4 2", "exam-|ple"},
		// Five before: neither point has that many in front of it.
		{"auto 5 2", "example"},
		{"auto 2 3", "ex-|am-|ple"},
		// Four after: "ple" is three, so the point in front of it goes.
		{"auto 2 4", "ex-|ample"},
		// Both at once, and nothing is left.
		{"auto 3 4", "example"},
	} {
		if got := limitLines(t, tc.value); got != tc.want {
			t.Errorf("hyphenate-limit-chars: %s set %q, want %q", tc.value, got, tc.want)
		}
	}
}

// TestTwoValuesLimitBothEnds. The property's second value is the letters before
// the break *and* the letters after it — the third takes the second's value when
// it is not written — and reading it as the first end alone is a mistake with no
// symptom until a word is short enough to notice.
func TestTwoValuesLimitBothEnds(t *testing.T) {
	// Four before and four after: the point after "exam" leaves "ple", which is
	// three. Taken as four-before-and-auto-after it would survive, because the
	// language wants three.
	if got := limitLines(t, "auto 4"); got != "example" {
		t.Errorf("\"auto 4\" set %q, want \"example\": the second value limits both ends", got)
	}
	// And with the third written out, the same declaration keeps the point.
	if got := limitLines(t, "auto 4 3"); got != "exam-|ple" {
		t.Errorf("\"auto 4 3\" set %q, want \"exam-|ple\"", got)
	}
}

// TestAValueThisCannotReadLeavesEveryLimitAtAuto. An invalid declaration is
// dropped, and what stands is the property's initial value — which is what a
// browser does with one and what the engine does with every other property.
func TestAValueThisCannotReadLeavesEveryLimitAtAuto(t *testing.T) {
	for _, value := range []string{
		"quips", "auto auto auto auto", "-1", "2 3 4 5", "1.5",
		// The whole declaration goes, not the part that parsed: read as far as
		// it went, this would be "three letters before" and would change the
		// page.
		"auto 3 quips",
	} {
		if got := limitLines(t, value); got != "ex-|am-|ple" {
			t.Errorf("hyphenate-limit-chars: %s set %q, want the auto answer "+
				"\"ex-|am-|ple\"", value, got)
		}
	}
}

// A word the author has divided takes their divisions and no others.
//
// §6.3.1: "Automatic hyphenation opportunities elsewhere within a word must be
// ignored if the word contains a conditional hyphen (&shy; or U+00AD SOFT
// HYPHEN), in favor of the conditional hyphen(s)."
//
// The mark neither ends the word nor joins it. Ending it — which is what this
// did, because a soft hyphen is not a letter — hyphenated each half on its own,
// so a word with one division in it came out with four. The suite writes it as
// hyphens-auto-control: "fragilistic&shy;expiali" in three widths, whose own
// comment lists the opportunities as "frag[A]ilis[A]tic[C]ex[A]pi[A]ali" and
// asks for the [C] and none of the [A]s.
func TestAWordWithASoftHyphenInItTakesNoOtherDivision(t *testing.T) {
	// The mark stays in the run — it is what says a line may end there, and
	// "hyphens: none" has to be able to see it again — and draws nothing. What
	// is compared here is what a reader sees.
	joined := func(lines []string) string {
		return strings.Map(func(r rune) rune {
			if isDefaultIgnorable(r) {
				return -1
			}
			return r
		}, joined(lines))
	}
	// "highway" divides as "high-way" and the box holds six characters, so a
	// dictionary point after "high" is what the first row proves is there.
	if got := joined(hyphLines(t, "highway", "")); got != "high-|way" {
		t.Fatalf("the fixture cannot say what it means to say: %q", got)
	}
	// With a division of the author's own, the dictionary's is not offered —
	// so the line ends where they put it and nowhere else.
	if got := joined(hyphLines(t, "high&shy;way", "")); got != "high-|way" {
		t.Errorf("\"high&shy;way\" set as %q, want \"high-|way\"", got)
	}
	if got := joined(hyphLines(t, "hi&shy;ghway", "")); got != "hi-|ghway" {
		t.Errorf("\"hi&shy;ghway\" set as %q, want \"hi-|ghway\" — the "+
			"dictionary's point after \"high\" was offered as well", got)
	}
	// And the mark joins the halves into one word rather than making two. Each
	// half of "hy&shy;phenation" would divide on its own — "phen-ation" is a
	// point the dictionary has — and the word as a whole must not.
	if got := joined(hyphLines(t, "hy&shy;phenation", "")); got != "hy-|phenation" {
		t.Errorf("\"hy&shy;phenation\" set as %q, want \"hy-|phenation\" — the "+
			"half after the mark was hyphenated as a word of its own", got)
	}
	// A mark in one box and the letters in another is the same word, which is
	// the whole reason this is a pass over the subtree.
	if got := joined(hyphLines(t, "hy<span>&shy;</span>phenation", "")); got != "hy-|phenation" {
		t.Errorf("with the mark in a span of its own: %q", got)
	}
	// And a word with no mark in it is untouched by any of this, even next to
	// one that has.
	if got := joined(hyphLines(t, "a&shy;b highway", "")); !strings.HasSuffix(got, "high-|way") {
		t.Errorf("%q — a marked word turned the dictionary off for the next one", got)
	}
	// An out-of-flow box's own text is a word of its own and its mark is its
	// own too: the two formatting contexts are not part of each other's words,
	// which is what hyphens-out-of-flow-002 is about from the other side.
	if got := joined(hyphLines(t, `high<div style="position:absolute">a&shy;b</div>way`, "")); got != "high-|way" {
		t.Errorf("with a marked word inside an absolutely positioned box: %q, "+
			"want \"high-|way\" — the mark was carried out of the box", got)
	}
	// And it does not reach in either. The word around the box has a mark; the
	// word inside it does not, and divides.
	root := layoutOf(t, 600,
		`<div id="d" lang="en">hi&shy;gh<div id="abs">highway</div>way</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 72px; hyphens: auto }
		 #abs { position: absolute; width: 72px }`)
	if got := joined(lineTextsOf(t, root, "abs")); got != "high-|way" {
		t.Errorf("inside the box: %q, want \"high-|way\" — the mark outside it "+
			"turned its own dictionary off", got)
	}
}
