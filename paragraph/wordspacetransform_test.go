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
		// auto-phrase may come on either side of the other word, and is the
		// half this engine cannot do.
		{"auto-phrase", "", "auto-phrase"},
		{"space auto-phrase", " ", "auto-phrase"},
		{"auto-phrase space", " ", "auto-phrase"},
		{"ideographic-space auto-phrase", "　", "auto-phrase"},
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
