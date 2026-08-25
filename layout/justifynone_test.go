package layout

import (
	"fmt"
	"strings"
	"testing"
)

// text-justify on an inline box.
//
// CSS Text 3 §7.3 applies the property to "block containers and inline boxes",
// and "none" means "justification is disabled: there are no justification
// opportunities within the text". The block's own value was read and an inline's
// was not, so a span written to hold two words at their natural distance had the
// line's slack poured into it like any other gap.
//
// css-text/text-justify/text-justify-006 is the fixture: four letters and three
// spaces, the middle space inside a "text-justify: none" span, against a
// reference that writes the answer out in preserved white space.

// justifiedGaps is the width of each space run on the first line of #p.
func justifiedGaps(t *testing.T, htmlSrc, cssSrc string) string {
	t.Helper()
	root := layoutOf(t, 600, htmlSrc, cssSrc)
	var parts []string
	for _, r := range find(t, root, "p").Lines[0].Runs {
		if strings.Trim(r.Text, " ") == "" && r.Text != "" {
			parts = append(parts, fmt.Sprintf("%g", r.Width.Px()))
		}
	}
	return strings.Join(parts, ",")
}

// The block is 220px and Courier at 20px is 12px a character, so "X X X X" is
// 84px of content and there are 136px to spread. The <b> is a second line's
// worth of filler, without which the line is a last line and is not justified.
const justifySrc = `<div id="p">X <span id="s">X X</span> X <b>###########</b></div>`

const justifyCSS = `#p { font-family: Courier; font-size: 20px; width: 220px;
	text-align: justify } b { font-weight: normal }`

// TestASpanThatRefusesJustificationKeepsItsSpaces is the bug.
func TestASpanThatRefusesJustificationKeepsItsSpaces(t *testing.T) {
	got := justifiedGaps(t, justifySrc, justifyCSS+` #s { text-justify: none }`)
	// The two spaces outside the span share all 136px; the one inside keeps the
	// 12px the font gave it.
	if got != "80,12,80" {
		t.Errorf("the spaces are %s wide, want 80,12,80 — the space inside the "+
			"\"text-justify: none\" span is not an opportunity", got)
	}
}

// TestWithoutTheDeclarationEverySpaceStretches is the containment argument and
// the other half of the same fixture: the property has to be what makes the
// difference, not the markup.
func TestWithoutTheDeclarationEverySpaceStretches(t *testing.T) {
	// Three equal shares of the 136px, to the layout unit: the remainder is
	// spread a unit at a time over the leading gaps rather than dropped, so the
	// three are not exactly equal and their total is exact.
	got := justifiedGaps(t, justifySrc, justifyCSS)
	if !strings.HasPrefix(got, "57.3") || len(strings.Split(got, ",")) != 3 {
		t.Errorf("the spaces are %s wide, want three shares of about 57.33 (36 of "+
			"the font's own and 136 spread over three gaps)", got)
	}
}

// TestTheBlocksOwnValueStillTurnsJustificationOff. Reading the property per item
// must not replace reading it on the block: "text-justify: none" there leaves an
// ordinary start-aligned line, which is a different thing from a justified line
// with no opportunities on it.
func TestTheBlocksOwnValueStillTurnsJustificationOff(t *testing.T) {
	got := justifiedGaps(t, justifySrc, justifyCSS+` #p { text-justify: none }`)
	if got != "12,12,12" {
		t.Errorf("the spaces are %s wide, want 12,12,12", got)
	}
}

// TestAnInheritedNoneReachesTheSpacesInside. The property is inherited, so a
// span inside the one that declared it refuses justification too — which is what
// makes reading the item's own computed value the whole of the lookup.
func TestAnInheritedNoneReachesTheSpacesInside(t *testing.T) {
	got := justifiedGaps(t,
		`<div id="p">X <span id="s">X <em>X X</em></span> X <b>###########</b></div>`,
		justifyCSS+` #s { text-justify: none } em { font-style: normal }`)
	// Four spaces now, two of them inside the span. The 136px is shared by the
	// two outside it.
	if got != "68,12,12,68" {
		t.Errorf("the spaces are %s wide, want 68,12,12,68; the <em> inside the "+
			"span inherits \"none\" and its space is not an opportunity either", got)
	}
}
