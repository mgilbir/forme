package paragraph

import "testing"

// word-space-transform, CSS Text 4: making a break opportunity visible.
//
// A zero width space and a <wbr> mark a place a line may end and put nothing on
// the page. That is what an author of English wants and not what an author of
// Japanese wants: Japanese is written without spaces between words, and a reader
// learning it — a dictionary, a children's book — wants the divisions shown.

// TestTheValueIsReadAsTheCharacterItSets.
func TestTheValueIsReadAsTheCharacterItSets(t *testing.T) {
	for _, tc := range []struct{ value, sep, unhandled string }{
		{"none", "", ""},
		{"", "", ""},
		{"space", " ", ""},
		{"ideographic-space", "　", ""},
		{"SPACE", " ", ""},
		{"  ideographic-space  ", "　", ""},
		// auto-phrase may come on either side of the other word. It is not
		// reported: whether the analysis can be done is a question about the
		// content language and the text, not about the declaration.
		{"auto-phrase", "", ""},
		{"space auto-phrase", " ", ""},
		{"auto-phrase space", " ", ""},
		{"ideographic-space auto-phrase", "　", ""},
		// Not a value of this property: nothing done and nothing reported, for
		// the reason the parser gives.
		{"nonsense", "", ""},
		{"space nonsense", "", ""},
	} {
		got, unhandled := WordSpaceTransformOf(tc.value)
		if got.Separator != tc.sep {
			t.Errorf("%q: the separator is %q, want %q", tc.value, got.Separator, tc.sep)
		}
		if unhandled != tc.unhandled {
			t.Errorf("%q: reported %q unhandled, want %q", tc.value, unhandled, tc.unhandled)
		}
		if got.Transforms() != (tc.sep != "") {
			t.Errorf("%q: Transforms is %v", tc.value, got.Transforms())
		}
	}
}

// wsCollapse is CollapseWhitespace under the ordinary collapsing value.
func wsCollapse(text, value string) string {
	wst, _ := WordSpaceTransformOf(value)
	return CollapseWhitespace(text, "collapse", wst)
}

// TestASeparatorCollapsesWithTheSpacesAroundIt is the rule that makes the
// property's own test 007 come out right, and it is the whole of why a zero
// width space is treated as white space here rather than replaced where it
// stands.
//
// The property happens between §4.1.1's two phases. A separator with a space
// beside it is one place a line may end, not two, so what reaches the page is
// one space — and a run of separators with no space in it is one as well.
func TestASeparatorCollapsesWithTheSpacesAroundIt(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		{"a​b", "a b", "on its own"},
		{"a ​b", "a b", "a space before it"},
		{"a​ b", "a b", "a space after it"},
		{"a ​ b", "a b", "one on each side"},
		{"a​​b", "a b", "two separators"},
		{"a ​ ​ b", "a b", "two, with spaces all through"},
		{"a​", "a ", "at the end"},
		{"​b", " b", "at the start"},
	} {
		if got := wsCollapse(tc.text, "space"); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestEverySeparatorIsItsOwnWhereNothingCollapses. Under pre, pre-wrap and
// break-spaces §4.1.1 collapses nothing, so a separator the author wrote beside
// a space they also wrote is two spaces on the page. The suite's
// word-space-transform-008 is that, and its reference has the doubled spaces
// written out.
func TestEverySeparatorIsItsOwnWhereNothingCollapses(t *testing.T) {
	wst, _ := WordSpaceTransformOf("space")
	for _, tc := range []struct{ text, want, what string }{
		{"a​b", "a b", "on its own"},
		{"a ​b", "a  b", "a space before it"},
		{"a​​b", "a  b", "two separators"},
		{"a ​ ​ b", "a     b", "test 008's own tail"},
	} {
		if got := CollapseWhitespace(tc.text, "preserve", wst); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestASeparatorAgainstAPreservedBreakIsNotExpanded. CSS Text 4 says not to
// expand one immediately before or after a forced break: the mark says a line
// *may* end there and the break says it must, so making it visible would leave a
// space hanging at the end of a line or indenting the start of one.
func TestASeparatorAgainstAPreservedBreakIsNotExpanded(t *testing.T) {
	wst, _ := WordSpaceTransformOf("ideographic-space")
	for _, tc := range []struct{ text, want, what string }{
		{"a​\nb", "a​\nb", "before the break"},
		{"a\n​b", "a\n​b", "after it"},
		{"a​\n​b", "a​\n​b", "on both sides"},
		// And one that is not against a break is expanded, so the rows above
		// are not passing because nothing is.
		{"a​b\nc", "a　b\nc", "elsewhere in the same text"},
	} {
		if got := CollapseWhitespace(tc.text, "preserve", wst); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestASeparatorBesideAPreservedBreakUnderPreLine is the same rule where the
// breaks are preserved and the spaces are not, which is white-space: pre-line
// and is the one value where a run can hold both a separator and a break that
// survives.
//
// What must come out is the break and nothing else. Expanding the separator
// would put a space at the end of the line the break ends — or worse, replace
// the break with a space and close the two lines into one.
func TestASeparatorBesideAPreservedBreakUnderPreLine(t *testing.T) {
	wst, _ := WordSpaceTransformOf("ideographic-space")
	for _, tc := range []struct{ text, want, what string }{
		{"a​\nb", "a\nb", "before the break"},
		{"a\n​b", "a\nb", "after it"},
		{"a ​ \n b", "a\nb", "with spaces around them"},
		// And a separator with no break in its run is expanded, so the rows
		// above are not passing because nothing is.
		{"a​b\nc", "a　b\nc", "elsewhere in the same text"},
	} {
		if got := CollapseWhitespace(tc.text, "preserve-breaks", wst); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestTheIdeographicValueSetsTheWiderSpace, which is the reason the property has
// two values rather than a boolean: a Latin space between two ideographs looks
// like a mistake, and U+3000 is the width of one character.
func TestTheIdeographicValueSetsTheWiderSpace(t *testing.T) {
	if got := wsCollapse("あ​い", "ideographic-space"); got != "あ　い" {
		t.Errorf("got %q, want %q", got, "あ　い")
	}
	if got := wsCollapse("あ​い", "space"); got != "あ い" {
		t.Errorf("got %q, want %q", got, "あ い")
	}
}

// TestNothingHappensAtTheInitialValue is the containment case, and the one that
// matters most: this sits in the collapsing every text node in every document
// goes through, and a zero width space must stay what it was — a mark that takes
// no room and offers a break — for every document that does not ask.
func TestNothingHappensAtTheInitialValue(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"a​b", "a​b"},
		{"a ​ b", "a ​ b"},
		{"the quick brown fox", "the quick brown fox"},
		{"a  b", "a b"},
		{"a\nb", "a b"},
		{"あ​い", "あ​い"},
	} {
		for _, value := range []string{"none", ""} {
			if got := wsCollapse(tc.text, value); got != tc.want {
				t.Errorf("at %q: %q became %q, want %q", value, tc.text, got, tc.want)
			}
		}
	}
	// And the §4.1.1 rule a zero width space already had is untouched: a
	// segment break beside one is removed rather than becoming a space, which
	// is what a hard-wrapped source means by writing one at a line end.
	if got := wsCollapse("a​\nb", "none"); got != "a​b" {
		t.Errorf("got %q, want %q — the segment break beside a zero width space "+
			"is removed", got, "a​b")
	}
}

// The other half of the value: the separators the document did not write.
//
// §2.2 gives the rule in one sentence — "if a word-separator character, other
// space separator, or U+200B ZERO WIDTH SPACE character does not already occur
// at that boundary, then the UA must insert a virtual expandable separator" —
// and the suite gives each of its clauses a document of its own.
func TestASeparatorIsInventedWhereThePhraseBoundaryHasNone(t *testing.T) {
	value, unhandled := WordSpaceTransformOf("ideographic-space auto-phrase")
	if unhandled != "" || !value.Invents() {
		t.Fatalf("the value was read as %+v, reporting %q", value, unhandled)
	}
	// Every row after the first is the same sentence with one of §2.2's
	// characters standing at the boundary the first row proves is there. They
	// are not the suite's own text, and that is deliberate: two of the suite's
	// documents put the separator where this model finds no boundary anyway, so
	// a rule deleted outright would leave them passing.
	for _, tc := range []struct{ text, want, what string }{
		{"東京へ行きましょう。", "東京へ\u3000行きましょう。",
			"word-space-transform-016, the value doing its work"},
		{"東京へ 行きましょう。", "東京へ 行きましょう。",
			"-021, next to a word-separator character"},
		{"東京へ\u3000行きましょう。", "東京へ\u3000行きましょう。",
			"-022, next to an other space separator"},
		{"東京へ\u200b行きましょう。", "東京へ\u200b行きましょう。",
			"-023, next to a zero width space"},
		{"東京\u00a0へ\u00a0行きましょう。", "東京\u00a0へ\u00a0行きましょう。",
			"-024, beside UAX #14's GL class, which is the no-break space"},
		{"東京\u2060へ\u2060行きましょう。", "東京\u2060へ\u2060行きましょう。",
			"-025, beside its WJ class"},
		{"東京\u200dへ\u200d行きましょう。", "東京\u200dへ\u200d行きましょう。",
			"-026, beside its ZWJ class"},
	} {
		if got := InsertPhraseSeparators(tc.text, value, WritingSystemJapanese); got != tc.want {
			t.Errorf("%s\n%q\n got %q\nwant %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// The separators are invented under a preserving value too, which is a second
// path through Phase I and not the same one read twice: "pre", "pre-wrap" and
// "break-spaces" collapse nothing, so the function returns before the run
// gathering the collapsing values need ever begins.
//
// A phrase boundary is not white space, so nothing about it changes with the
// value — which is exactly why forgetting the second path leaves a document
// under "white-space: pre" with the separators it wrote and none of the ones it
// asked to be found.
func TestSeparatorsAreInventedWhereNothingCollapsesEither(t *testing.T) {
	value, _ := WordSpaceTransformOf("ideographic-space auto-phrase")
	for _, whiteSpace := range []string{"preserve", "preserve-breaks", "break-spaces"} {
		got := CollapseWhitespaceAfter("\u6771\u4eac\u3078\u884c\u304d\u307e\u3057\u3087\u3046\u3002",
			whiteSpace, value, Boundary{}, WritingSystemJapanese)
		const want = "\u6771\u4eac\u3078\u3000\u884c\u304d\u307e\u3057\u3087\u3046\u3002"
		if got != want {
			t.Errorf("under white-space-collapse: %s the text came out %q, want %q",
				whiteSpace, got, want)
		}
	}
}

// And the two clauses about the language, which §2.2 answers for the UA: "if
// this value is omitted, or if the content language is unknown, or if the user
// agent does not support detecting phrase boundaries for that language, there
// are no virtual expandable separator". The suite's -027 and -029 are the
// second and the third.
func TestNoSeparatorIsInventedWithoutALanguageToInventItIn(t *testing.T) {
	value, _ := WordSpaceTransformOf("ideographic-space auto-phrase")
	const text = "東京へ行きましょう。"
	if got := InsertPhraseSeparators(text, value, WritingSystemOther); got != text {
		t.Errorf("with no language known: got %q, want the text unchanged", got)
	}
	// And the first clause: the value without auto-phrase in it invents
	// nothing, whatever language the text is in.
	plain, _ := WordSpaceTransformOf("ideographic-space")
	if plain.Invents() {
		t.Error("\"ideographic-space\" alone reports that it invents separators")
	}
	if got := InsertPhraseSeparators(text, plain, WritingSystemJapanese); got != text {
		t.Errorf("without auto-phrase: got %q, want the text unchanged", got)
	}
	// "auto-phrase" with nothing for its boundaries to become is not a value
	// the grammar allows — "[ space | ideographic-space ] && auto-phrase?" —
	// and it inserts nothing rather than inserting the empty string.
	alone, _ := WordSpaceTransformOf("auto-phrase")
	if alone.Invents() {
		t.Error("\"auto-phrase\" alone reports that it invents separators")
	}
	if got := InsertPhraseSeparators(text, alone, WritingSystemJapanese); got != text {
		t.Errorf("with auto-phrase alone: got %q, want the text unchanged", got)
	}
}

// The separator is written into the text rather than carried beside it, so
// everything downstream measures and breaks what the reader will see. The
// suite's word-space-transform-030 says so: "Transform effects, notably
// transforming virtual word separators into spaces, affect line breaking."
func TestAnInventedSeparatorIsAPlaceALineMayEnd(t *testing.T) {
	value, _ := WordSpaceTransformOf("ideographic-space auto-phrase")
	text := InsertPhraseSeparators("東京へ行きましょう。", value, WritingSystemJapanese)
	pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true},
		WordBreak{KeepAll: true}, LineBreak{}, Hyphens{}, WritingSystemJapanese)
	// keep-all demotes every opportunity between two ideographs, so the one a
	// line reaches for is the separator's — which is the pairing §2.2's own
	// example writes, "word-break: keep-all" beside "word-space-transform:
	// ideographic-space". The demoted ones are still there and are not counted:
	// a line takes one only when it has no separator on it at all, which is the
	// case this document is written not to be.
	var breaks int
	for _, p := range pieces {
		if p.BreakBefore && !p.LastResort {
			breaks++
		}
	}
	if breaks != 1 {
		t.Errorf("%q broke in %d places under keep-all, want 1 — the separator "+
			"the value invented", text, breaks)
	}
}
