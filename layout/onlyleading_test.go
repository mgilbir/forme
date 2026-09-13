package layout

import (
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/style"
)

// A space the line edge removes is not what keeps a block's line alive.
//
// CSS 2.1 §9.4.2: a line box holding "no text, no preserved white space, no
// inline elements with non-zero margins, padding, or borders" is a zero-height
// line box, and this engine goes one further for a whole block — a block with no
// real content has no line at all rather than an empty one, which is what
// onlyLeading is for.
//
// It was asked of the items *before* the line edge trimmed them, so a space that
// the edge removes counted as real content. On its own that changed nothing,
// because a leading collapsible space is collapsed away before it becomes an
// item. It took a zero width space in front of it — which separates, so the
// space after it is not the first thing on the line and does not collapse — and
// an empty inline box to give the surviving line its height:
//
//	"​ "                          no line, and a div nought high
//	"<span>​</span><span> </span>" a line 21.8px high, holding nothing
//
// The div was a line high for having two spans in it, and the page showed
// nothing either way.
//
// Found by FuzzBoundaryLines. It is the same rule as the bidi control that made
// a line look occupied, one step further along: there the line was wrongly
// non-empty, here it was wrongly *kept*.
func TestASpaceTheLineEdgeRemovesDoesNotKeepTheLine(t *testing.T) {
	const zwsp = "​"
	for _, tc := range []struct{ what, markup string }{
		{"a zero width space then a space, in two boxes",
			`<span>` + zwsp + `</span><span> </span>`},
		{"the same text written plainly", zwsp + " "},
		{"a span holding only a space", `<span> </span>`},
		{"an empty span", `<span></span>`},
	} {
		if got, h := linesAndHeight(t, tc.markup); got != 0 || h != 0 {
			t.Errorf("%s: %q set %d lines and a block %v high; there is nothing "+
				"on the page, so there is no line", tc.what, tc.markup, got, h)
		}
	}

	// §9.4.2's exception, which must keep its line: an inline box with a
	// non-zero border is named in the rule, so a line holding one is not empty
	// however little else is on it.
	for _, tc := range []struct{ what, markup string }{
		{"a bordered span holding a space",
			`<span style="border-left:1px solid red"> </span>`},
		{"a bordered span before the space",
			`<span style="border-left:1px solid red">` + zwsp + `</span><span> </span>`},
		{"ordinary text", "a"},
		{"text with spaces round it", " a "},
	} {
		if got, h := linesAndHeight(t, tc.markup); got == 0 || h == 0 {
			t.Errorf("%s: %q set %d lines and a block %v high; it puts something "+
				"on the page and must have a line", tc.what, tc.markup, got, h)
		}
	}
}

// linesAndHeight is how many lines a div sets and how tall it comes out.
func linesAndHeight(t *testing.T, markup string) (int, style.Unit) {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}
	built := Build(Input{HTML: `<div id="d">` + markup + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#d { font-family: T; font-size: 16px }`}}})
	if built.Root == nil {
		return 0, 0
	}
	w, _ := style.FromPx(200)
	h, _ := style.FromPx(1000000)
	frag := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	f := findLaidOut(frag, "d")
	if f == nil {
		return 0, 0
	}
	return len(f.Lines), f.BorderRect.H
}

// findLaidOut is the fragment the element with this id produced.
func findLaidOut(f *Fragment, id string) *Fragment {
	if f == nil {
		return nil
	}
	if f.Box != nil && f.Box.Element != nil {
		if got, _ := f.Box.Element.Attr("id"); got == id {
			return f
		}
	}
	for _, c := range f.Children {
		if hit := findLaidOut(c, id); hit != nil {
			return hit
		}
	}
	return nil
}
