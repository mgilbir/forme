package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// <wbr>, which HTML defines as "a line break opportunity" and nothing else.
//
// It was reaching the layout as an inline box with nothing in it. A box with
// nothing in it produces no items, and no items is no opportunity, so
// "aaaa<wbr>bbbb" was set as one unbreakable word in a box four characters wide
// — and nothing said so, because an empty inline box is not a thing to report.
// Eighteen of the suite's reftests write one.
//
// Every expectation below is arithmetic that can be read: Courier is 600/1000,
// so a character at 20px is 12px and a line of n characters is 12n.

// wbrCSS is a box four characters wide, so a word of eight has to break or
// overflow.
const wbrCSS = `#p { font-family: Courier; font-size: 20px; width: 48px }`

// TestAWbrOffersABreakOpportunity.
func TestAWbrOffersABreakOpportunity(t *testing.T) {
	for _, tc := range []struct {
		markup string
		want   []string
		what   string
	}{
		{`aaaa<wbr>bbbb`, []string{"aaaa", "bbbb"}, "between two halves of a word"},
		{`aaaa<wbr/>bbbb`, []string{"aaaa", "bbbb"}, "written closed"},
		// Across an inline box boundary, which is where an opportunity has to
		// travel on the state rather than inside a run.
		{`aaaa<wbr><span>bbbb</span>`, []string{"aaaa", "bbbb"}, "before an inline box"},
		{`<span>aaaa</span><wbr>bbbb`, []string{"aaaa", "bbbb"}, "after one"},
		{`<span>aaaa<wbr></span>bbbb`, []string{"aaaa", "bbbb"}, "inside one, at its end"},
		// Two of them: the line breaks at the last one that fits.
		{`aa<wbr>aa<wbr>bbbb`, []string{"aaaa", "bbbb"}, "two, and the later one fits"},
	} {
		root := layoutOf(t, 600, `<div id="p">`+tc.markup+`</div>`, wbrCSS)
		got := lineTextsOf(t, root, "p")
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: %q, want %q", tc.what, got, tc.want)
		}
	}
	// The control: the same text with nothing in it does not break, so the rows
	// above are measuring the element rather than the width.
	root := layoutOf(t, 600, `<div id="p">aaaabbbb</div>`, wbrCSS)
	if got := lineTextsOf(t, root, "p"); len(got) != 1 {
		t.Errorf("the control broke into %q; a word with no opportunity in it "+
			"overflows rather than breaking", got)
	}
}

// TestAWbrIsNotAForcedBreak, which is the whole difference between it and the
// <br> beside it in the flattening. <br> ends the line wherever it falls, even
// on a line with room to spare; <wbr> only says a line may end here.
func TestAWbrIsNotAForcedBreak(t *testing.T) {
	root := layoutOf(t, 600, `<div id="p">aa<wbr>bb</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 300px }`)
	got := lineTextsOf(t, root, "p")
	if len(got) != 1 || got[0] != "aabb" {
		t.Errorf("%q, want one line of aabb: there is room, so the opportunity "+
			"is not taken", got)
	}
	// And <br> in the same room does end the line, so this is not passing
	// because the fixture never breaks.
	br := layoutOf(t, 600, `<div id="p">aa<br>bb</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 300px }`)
	if got := lineTextsOf(t, br, "p"); len(got) != 2 {
		t.Errorf("the control set <br> as %q; it ends the line whatever the room", got)
	}
}

// textWidth is how far the text on a line reaches, which is not the line box:
// the box is the width the block was given whatever is set in it. Measuring the
// box instead makes every fixture below agree with every other, which is a
// mistake this file made once and is the reason this helper exists.
func textWidth(t *testing.T, root *Fragment, id string) style.Unit {
	t.Helper()
	lines := find(t, root, id).Lines
	if len(lines) == 0 {
		t.Fatalf("#%s has no lines to measure", id)
	}
	var end style.Unit
	for _, l := range lines {
		for _, r := range l.Runs {
			if x := r.X.Add(r.Width); x > end {
				end = x
			}
		}
	}
	return end
}

// TestAWbrTakesNoRoom. It is an opportunity and not a character, so a line with
// one in it is exactly as wide as the same line without.
func TestAWbrTakesNoRoom(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 300px }`
	with := layoutOf(t, 600, `<div id="p">aaaa<wbr>bbbb</div>`, css)
	without := layoutOf(t, 600, `<div id="p">aaaabbbb</div>`, css)
	a, b := textWidth(t, with, "p"), textWidth(t, without, "p")
	if a != b {
		t.Errorf("the text is %v wide with a <wbr> in it and %v without; eight "+
			"characters of Courier at 20px is 96px either way", a, b)
	}
	// And the number is the arithmetic rather than whatever came out, so a
	// version where both are zero does not pass.
	if want, _ := style.FromPx(96); a != want {
		t.Errorf("the text is %v wide, want %v", a, want)
	}
}

// TestAWbrMarksNoBoundaryInTheText is the reason the opportunity is recorded on
// the state rather than emitted as a zero-width space in the run.
//
// HTML calls <wbr> a line break opportunity and nothing else. "sur<wbr/>name" is
// one word, and text-transform: capitalize gives it one capital — which a space
// in the text, of any width, would not. capitalizeboundary_test.go asserts this
// and was written long before the element offered any opportunity at all; it is
// named here because it is the half of the behaviour this change had to leave
// alone, and did.
func TestAWbrMarksNoBoundaryInTheText(t *testing.T) {
	got := transformed(t, `<span style="text-transform:capitalize">sur<wbr />name</span>`)
	if got != "Surname" {
		t.Errorf("got %q, want %q", got, "Surname")
	}
}

// TestSpacesAroundAWbrStillCollapse is the containment case. §4.1.1's collapsing
// runs over the text, and an element that puts nothing in the text must not come
// between two spaces that would otherwise have become one.
func TestSpacesAroundAWbrStillCollapse(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 300px }`
	for _, tc := range []struct{ markup, what string }{
		{`aa <wbr> bb`, "a space on each side"},
		{`aa <wbr>bb`, "one before"},
		{`aa<wbr> bb`, "one after"},
	} {
		with := layoutOf(t, 600, `<div id="p">`+tc.markup+`</div>`, css)
		without := layoutOf(t, 600, `<div id="p">aa bb</div>`, css)
		a, b := textWidth(t, with, "p"), textWidth(t, without, "p")
		if a != b {
			t.Errorf("%s: the text is %v wide, want %v — the spaces around a "+
				"<wbr> collapse as they would without it", tc.what, a, b)
		}
		// Five characters of Courier at 20px, so a second space that survived
		// would show as 72 rather than 60.
		if want, _ := style.FromPx(60); a != want {
			t.Errorf("%s: the text is %v wide, want %v", tc.what, a, want)
		}
	}
}

// TestADocumentWithNoWbrIsUnchanged. The case added here sits in the walk every
// inline box goes through, and it must not touch a document that has none.
func TestADocumentWithNoWbrIsUnchanged(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 96px }`
	for _, tc := range []struct{ markup, want string }{
		{`the quick brown fox`, "the|quick|brown|fox"},
		{`a <span>b</span> c`, "a b c"},
		{`aaaaaaaaaaaa`, "aaaaaaaaaaaa"},
		{`aa<br>bb`, "aa|bb"},
	} {
		root := layoutOf(t, 600, `<div id="p">`+tc.markup+`</div>`, css)
		if got := strings.Join(lineTextsOf(t, root, "p"), "|"); got != tc.want {
			t.Errorf("%q came out %q, want %q", tc.markup, got, tc.want)
		}
	}
}
