package layout

import (
	"strings"
	"testing"
)

// Which element decides at a soft wrap opportunity between two boxes.
//
// CSS Text §5.1: "for soft wrap opportunities defined by the boundary between
// two characters, the white-space property on the nearest common ancestor of the
// two characters controls breaking". An opportunity inside one box is that box's
// to allow or refuse and needs no rule; one that crosses a boundary belongs to
// neither side of it, and this engine was asking the box the *later* character
// happened to be in.
//
// So a zero width space written in a wrapping div, between two spans that say
// "white-space: pre", offered a break that the second span refused — and the
// span had nothing to say about a boundary outside itself. Both characters went
// on one line in a box one character wide.

// wrappedLines returns what each line of a box reads.
//
// Courier at 20px is 12px a character, so a box 12px wide holds exactly one and
// the count of lines is the whole assertion.
func wrappedLines(t *testing.T, htmlSrc string) []string {
	t.Helper()
	root := layoutOf(t, 600, htmlSrc, noDefaults+`div, span { font-family: Courier; font-size: 20px }`)
	var out []string
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if got, _ := f.Box.Element.Attr("id"); got == "d" {
				for _, ln := range f.Lines {
					var b strings.Builder
					for _, r := range ln.Runs {
						b.WriteString(r.Text)
					}
					out = append(out, b.String())
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// TestABoundaryOpportunityIsTheAncestorsToAllow is the bug, and it is the
// suite's line-breaking-009 with the numbers spelled out.
func TestABoundaryOpportunityIsTheAncestorsToAllow(t *testing.T) {
	for _, tc := range []struct{ what, html string }{
		// The zero width space is in the div and the characters are in the
		// spans, so the nearest common ancestor of the pair around the boundary
		// is the div — which wraps.
		{"between two preserved spans",
			`<div id="d" style="width:12px">` +
				`<span style="white-space:pre">X</span>&#x200B;` +
				`<span style="white-space:pre">X</span></div>`},
		// line-breaking-011: only the second character is in a span, which
		// makes no difference — the ancestor is the same div either way.
		{"with only the second in a span",
			`<div id="d" style="width:12px">X&#x200B;` +
				`<span style="white-space:pre">X</span></div>`},
		// line-breaking-032: §5.1 puts an opportunity around an atomic inline
		// as readily as between two characters, and the same element decides
		// it. An inline-block stands in for the picture the suite uses.
		{"after an atomic inline",
			`<div id="d" style="width:12px">` +
				`<span style="white-space:pre"><span style="display:inline-block; width:12px; height:12px"></span></span>` +
				`<span style="white-space:pre">X</span></div>`},
	} {
		got := wrappedLines(t, tc.html)
		if len(got) != 2 {
			t.Errorf("%s: the content took %d lines (%q), want 2 — the boundary is "+
				"the div's, and the div wraps", tc.what, len(got), got)
		}
	}
}

// TestTheBoundaryIsAskedBeforeAnyOpportunityHasBeenFound.
//
// §5.1's sentence is about the boundary, not about an opportunity that has
// already reached it — and the two are not the same, because the opportunities
// are not all found in one place. word-break: break-all makes one at exactly
// this edge, and it is made after the boundary's owner has been settled.
//
// The suite writes it as break-boundary-2-chars-001: "abc<span>xyz</span>def"
// under break-all, with the span set to "white-space: pre". Every character of
// the div's text goes on a line of its own and the span's three go together, so
// there is a break at each edge of the span — offered by break-all and allowed
// by the div, over a span that would have refused both.
func TestTheBoundaryIsAskedBeforeAnyOpportunityHasBeenFound(t *testing.T) {
	got := wrappedLines(t, `<div id="d" style="width:12px; word-break:break-all">ab`+
		`<span style="white-space:pre">xyz</span>cd</div>`)
	want := []string{"a", "b", "xyz", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("the content took %d lines (%q), want %d: %q", len(got), got, len(want), want)
	}
	for i := range want {
		if strings.TrimSpace(got[i]) != want[i] {
			t.Errorf("line %d reads %q, want %q — the span's own text is unbreakable "+
				"and the boundaries around it are the div's", i, got[i], want[i])
		}
	}
}

// TestTheAncestorCanRefuseWhatTheBoxWouldAllow is the same rule read the other
// way, and it is the half that says this is about the ancestor rather than about
// "pre never wraps".
//
// The div says nowrap and the span says normal. The opportunity is still the
// div's, and the div refuses it — where reading the later character's box would
// have taken it.
func TestTheAncestorCanRefuseWhatTheBoxWouldAllow(t *testing.T) {
	got := wrappedLines(t, `<div id="d" style="width:12px; white-space:nowrap">X&#x200B;`+
		`<span style="white-space:normal">X</span></div>`)
	if len(got) != 1 {
		t.Errorf("the content took %d lines (%q), want 1: the boundary belongs to "+
			"the div, which does not wrap", len(got), got)
	}
}

// TestAnOpportunityInsideABoxIsStillItsOwn is the containment case, and the one
// that matters most: almost every opportunity in almost every document is inside
// a single box, and for those the ancestor rule must not be reached at all.
//
// A space inside a "white-space: pre" span is that span's, and pre does not
// wrap. Getting this wrong would break every preformatted block in the corpus.
func TestAnOpportunityInsideABoxIsStillItsOwn(t *testing.T) {
	got := wrappedLines(t, `<div id="d" style="width:12px">`+
		`<span style="white-space:pre">X X</span></div>`)
	if len(got) != 1 {
		t.Errorf("the span's own space broke the line into %d (%q); the boundary is "+
			"inside one box and pre does not wrap", len(got), got)
	}
	// And a wrapping span in the same shape does break, so the fixture is
	// telling the two apart rather than never breaking at all.
	if got := wrappedLines(t, `<div id="d" style="width:12px">`+
		`<span style="white-space:normal">X X</span></div>`); len(got) != 2 {
		t.Errorf("a wrapping span's own space gave %d lines (%q), want 2", len(got), got)
	}

	// The two rules in one document, which is what says the ancestor's answer
	// is used at the boundary and *only* there. The zero width space in front
	// of the span is the div's and breaks; the ordinary space inside the span
	// is the span's and does not. Three lines would mean the boundary's answer
	// had been handed to the span's own opportunities as well.
	got = wrappedLines(t, `<div id="d" style="width:12px">Y&#x200B;`+
		`<span style="white-space:pre">X X</span></div>`)
	if len(got) != 2 || strings.TrimSpace(got[1]) != "X X" {
		t.Errorf("the content took %d lines (%q), want 2 with the span's space kept "+
			"on the second — the boundary in front of the span wraps and the space "+
			"inside it does not", len(got), got)
	}
}

// TestAPlainDocumentIsUnchanged. Every box in it has the same white-space, so
// the ancestor's answer is the box's answer and nothing may move.
func TestAPlainDocumentIsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		what, html string
		want       int
	}{
		{"a zero width space", `<div id="d" style="width:12px">X&#x200B;X</div>`, 2},
		{"an ordinary space", `<div id="d" style="width:12px">X X</div>`, 2},
		{"across a span", `<div id="d" style="width:12px">X <span>X</span></div>`, 2},
		{"nothing to break at", `<div id="d" style="width:12px">XX</div>`, 1},
	} {
		if got := wrappedLines(t, tc.html); len(got) != tc.want {
			t.Errorf("%s: %d lines (%q), want %d", tc.what, len(got), got, tc.want)
		}
	}
}

// TestTheCommonAncestorOfTwoBoxes, asked directly, because the walk has a case
// no document reaches: two boxes in different trees have none, and the answer
// has to be "none" rather than whichever root the loop stopped at.
func TestTheCommonAncestorOfTwoBoxes(t *testing.T) {
	built := Build(Input{HTML: `<div id="a"><span id="b">x</span><span id="c">y</span></div>`})
	if built.Root == nil {
		t.Fatal("no boxes")
	}
	a := findBox(t, built.Root, "a")
	b := findBox(t, built.Root, "b")
	c := findBox(t, built.Root, "c")
	if got := nearestCommonAncestor(b, c); got != a {
		t.Errorf("the common ancestor of two sibling spans is %v, want the div", got)
	}
	// A box is its own ancestor, which is what makes an opportunity inside one
	// box answer with that box.
	if got := nearestCommonAncestor(b, b); got != b {
		t.Errorf("a box's common ancestor with itself is %v, want itself", got)
	}
	if got := nearestCommonAncestor(a, b); got != a {
		t.Errorf("the common ancestor of a box and its child is %v, want the parent", got)
	}

	other := Build(Input{HTML: `<div id="a">z</div>`})
	if other.Root == nil {
		t.Fatal("no boxes in the second document")
	}
	if got := nearestCommonAncestor(b, findBox(t, other.Root, "a")); got != nil {
		t.Errorf("two boxes in different trees have a common ancestor %v", got)
	}
	if got := nearestCommonAncestor(nil, b); got != nil {
		t.Errorf("a nil box has a common ancestor %v", got)
	}
}
