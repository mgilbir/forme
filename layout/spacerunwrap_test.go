package layout

import (
	"strings"
	"testing"
)

// Which box decides whether a line may end after a run of spaces.
//
// CSS Text §5.1 has two rules, one for each kind of opportunity. One "created by
// characters that disappear at the line break (e.g. U+0020 SPACE)" is governed
// by "the box directly containing that character"; one "defined by the boundary
// between two characters or atomic inlines" by "the nearest common ancestor of
// the two characters". §4.1.1 adds the fact that makes a run of spaces across a
// box boundary interesting: the later spaces collapse into the first, and each
// "is invisible, but retains its soft wrap opportunity, if any". So the run's
// end has the kept space's opportunity and the collapsed ones' at one place, and
// a line may end there where any of those boxes lets it.
//
// The text is Courier at 20px, so a character is 12px and the box is twelve
// characters wide: "1111 2222222222" is fifteen and cannot be set on one line
// unless nothing lets it break.

// spaceRunCSS is the stylesheet for these tests: Courier, a twelve-character
// box, and a class for each value of white-space.
const spaceRunCSS = `#d { font-family: Courier; font-size: 20px }
.n { white-space: nowrap } .w { white-space: normal } .p { white-space: pre }
.b { white-space: break-spaces } .ib { display: inline-block; width: 60px }
.m { margin-left: 6px }`

// spaceRunLines is the lines the markup sets in the twelve-character box.
func spaceRunLines(t *testing.T, markup string) string {
	t.Helper()
	return strings.Join(linesOfMarkupCSS(t, markup, 12*12, spaceRunCSS), "|")
}

// TestTheKeptSpaceMayEndTheLineBeforeANowrapBox is the defect: "1111 <nobr>
// 2222</nobr>" keeps the paragraph's space and collapses the nobr's, and the
// line never broke after "1111" — the only box asked was the nobr, about the
// space that collapsed.
//
// Every arrangement of the two spaces and the two values is here, because the
// rule has two halves and each is the other's regression: the kept space and
// the collapsed one each decide for themselves, and an answer that consults only
// one of them gets half the table wrong.
func TestTheKeptSpaceMayEndTheLineBeforeANowrapBox(t *testing.T) {
	const broken, whole = "1111|2222222222", "1111 2222222222"
	for _, tc := range []struct{ markup, want string }{
		// The defect, spelled as the issue spells it and as a class.
		{`1111 <nobr> 2222222222</nobr>`, broken},
		{`1111 <span class="n"> 2222222222</span>`, broken},
		// The collapsed space alone in a box of its own, and then the word in
		// the paragraph, or in a second nowrap box.
		{`1111 <nobr> </nobr>2222222222`, broken},
		{`1111 <nobr> </nobr><nobr>2222222222</nobr>`, broken},
		// Nested: two nowrap boxes, each with a space that collapses.
		{`1111 <span class="n"> <span class="n"> 2222222222</span></span>`, broken},
		// A wrapping box inside the nowrap one, whose space collapses too.
		{`1111 <span class="n"> <span class="w">2222222222</span></span>`, broken},
		{`1111 <span class="w"> <span class="n">2222222222</span></span>`, broken},
		// The space outside and nothing to collapse: this always worked.
		{`1111 <nobr>2222222222</nobr>`, broken},

		// The mirror: the kept space is the nowrap box's, and it says no.
		{`1111<nobr> 2222222222</nobr>`, whole},
		{`<nobr>1111 </nobr>2222222222`, whole},
		{`<span class="n">1111 <span class="w">2222222222</span></span>`, whole},
		// A nowrap box around the whole run says no whatever is inside it.
		{`<span class="n">1111 <nobr> 2222222222</nobr></span>`, whole},
		// And the collapsed space is a wrapping box's, which says yes: the last
		// row of white-space-wrap-after-nowrap-001, and its neighbours.
		{`<nobr>1111 </nobr> 2222222222`, broken},
		{`<span class="n">1111 </span><span class="w"> 2222222222</span>`, broken},
		{`<span class="n">1111 <span class="w"> 2222222222</span></span>`, broken},

		// Preserved spaces do not collapse, so there is no run to share: pre
		// keeps both spaces and may not break after either, and break-spaces
		// breaks after its own.
		{`1111 <span class="p"> 2222222222</span>`, "1111  2222222222"},
		{`1111 <span class="b"> 2222222222</span>`, "1111  |2222222222"},
	} {
		if got := spaceRunLines(t, tc.markup); got != tc.want {
			t.Errorf("%s set %q, want %q", tc.markup, got, tc.want)
		}
	}
}

// TestAMarginEdgeKeepsTheOpportunityItsWhiteSpaceAllows. §5.1 moves an
// opportunity in front of a box's first character to the box's margin edge, and
// that moves where the line ends and not whether it may. The margin's item took
// whatever was carried to it and asked no one, so a nowrap box broke in front of
// any inline box with a margin inside it — and, the other way, a space that a
// nowrap box keeps was taken in front of a margin outside it.
func TestAMarginEdgeKeepsTheOpportunityItsWhiteSpaceAllows(t *testing.T) {
	for _, tc := range []struct{ markup, want string }{
		{`<nobr>1111 <span class="m">2222222222</span></nobr>`, "1111 2222222222"},
		{`<nobr>1111-<span class="m">2222222222</span></nobr>`, "1111-2222222222"},
		{`<nobr>1111 </nobr><span class="m">2222222222</span>`, "1111 2222222222"},
		// Where the space may end the line, the margin still takes it.
		{`1111 <nobr> <span class="m">2222222222</span></nobr>`, "1111|2222222222"},
		{`1111 <span class="m">2222222222</span>`, "1111|2222222222"},
		{`<span class="n">1111 </span> <span class="m">2222222222</span>`, "1111|2222222222"},
	} {
		if got := spaceRunLines(t, tc.markup); got != tc.want {
			t.Errorf("%s set %q, want %q", tc.markup, got, tc.want)
		}
	}
}

// TestAPictureAfterACollapsedSpaceKeepsTheKeptSpacesOpportunity. In front of an
// atomic inline there are two opportunities: the picture's own, which §5.1
// gives to the boundary and so to the nearest common ancestor, and the spaces',
// which it gives to the boxes the spaces are in. "1111 <nobr> <img></nobr>"
// refused the first — the ancestor of the nobr's collapsed space and the
// picture is the nobr — and never asked about the second.
//
// The picture is five characters wide and the word after it seven, so the two
// fill a line exactly and "1111 " cannot join them. The lines are given as their
// text, and the picture's line has only the word's.
func TestAPictureAfterACollapsedSpaceKeepsTheKeptSpacesOpportunity(t *testing.T) {
	const ib = `<span class="ib"></span>`
	for _, tc := range []struct{ markup, want string }{
		{`1111 <nobr> ` + ib + `xxxxxxx</nobr>`, "1111|xxxxxxx"},
		{`1111 <nobr>` + ib + `xxxxxxx</nobr>`, "1111|xxxxxxx"},
		// The nowrap box around both the space and the picture refuses both.
		{`<nobr>1111 ` + ib + `xxxxxxx</nobr>`, "1111 xxxxxxx"},
		{`<nobr>1111 <nobr> ` + ib + `xxxxxxx</nobr></nobr>`, "1111 xxxxxxx"},
	} {
		if got := spaceRunLines(t, tc.markup); got != tc.want {
			t.Errorf("%s set %q, want %q", tc.markup, got, tc.want)
		}
	}
}

// TestTheMinimumWidthKnowsTheKeptSpacesOpportunity. The min-content width reads
// the same items and has to agree with the lines: "1111 <nobr> 2222</nobr>" may
// break after the first word, so its minimum is the longer word and not both.
func TestTheMinimumWidthKnowsTheKeptSpacesOpportunity(t *testing.T) {
	for _, tc := range []struct {
		markup string
		chars  float64
	}{
		{`1111 <nobr> 2222222222</nobr>`, 10},
		{`1111 <nobr> </nobr>2222222222`, 10},
		{`<nobr>1111 </nobr>2222222222`, 15},
		{`<nobr>1111 <span class="m">2222222222</span></nobr>`, 15 + 0.5},
	} {
		root := layoutOf(t, 600, `<div id="mc">`+tc.markup+`</div>`,
			noDefaults+`#mc { float: left; width: min-content; font-family: Courier; font-size: 20px }
			.m { margin-left: 6px }`)
		if got := find(t, root, "mc").BorderRect.W.Px(); got != tc.chars*12 {
			t.Errorf("%s is %gpx at min-content, want %gpx", tc.markup, got, tc.chars*12)
		}
	}
}
