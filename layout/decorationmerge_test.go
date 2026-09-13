package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A box that declares a decoration and inherits one as well.
//
// §16.3.1 makes the two independent: a decoration is drawn across everything the
// declaring box contains, and a descendant declaring one of its own adds to it
// rather than replacing it. So an <em> with an overline inside an underlined
// paragraph carries both lines, in two colours, because each takes the colour of
// the box that declared it.
//
// decorationsFor has three arms for the join — nothing of its own, nothing above
// it, and both — and only the first two had ever run. The third was at 0% across
// every unit test and all 6253 reftest documents: the suite's decoration tests
// declare one line per document and the corpora do not nest them.
//
// Losing it is a line off the page. Whichever of the two the broken arm dropped,
// the result is a document that draws one of the lines the author asked for and
// looks deliberate.

func TestABoxDrawsItsOwnDecorationAndTheOneAboveIt(t *testing.T) {
	// "ab" then "cdef" in Courier at 20px: 12px a character, so 24px and 48px.
	root := layoutOf(t, 600,
		`<div id="p">ab<em id="e">cdef</em></div>`,
		noDefaults+decoCSS+` #p { text-decoration: underline }
		 #e { text-decoration: overline; color: #ff0000 }`)
	ops := Paint(root)
	red := style.RGBA{R: 255, A: 1}

	// The paragraph's underline is black and reaches across both runs, the em
	// included — that is the inherited half, and the half a merge that kept only
	// "own" would drop over the em.
	under := bands(ops, black)
	if len(under) != 2 {
		t.Fatalf("the paragraph's underline painted %d black bands, want one "+
			"per run: it is declared on the div and drawn across everything the "+
			"div contains, the em included", len(under))
	}
	if total := under[0].W.Add(under[1].W).Px(); total != 72 {
		t.Errorf("the underline is %gpx wide altogether, want 72 — the whole "+
			"text. An underline that stopped at the em would be 24", total)
	}

	// The em's own overline is red and covers the em alone — the declared half,
	// and the half a merge that kept only "above" would drop.
	over := bands(ops, red)
	if len(over) != 1 {
		t.Fatalf("the em's overline painted %d red bands, want 1: it is declared "+
			"on the em and drawn across what the em contains", len(over))
	}
	if w := over[0].W.Px(); w != 48 {
		t.Errorf("the overline is %gpx wide, want 48 — the em's four characters", w)
	}

	// And they are two different lines rather than one drawn twice: the
	// paragraph's sits under the text and the em's over it.
	base := baselineOfFirstRun(t, root, "p")
	if over[0].Y >= base {
		t.Errorf("the overline's top is at %v and the baseline at %v; an "+
			"overline is drawn above the text", over[0].Y, base)
	}
	for i, r := range under {
		if r.Y <= base {
			t.Errorf("underline band %d has its top at %v and the baseline is "+
				"at %v; an underline is drawn below the text", i, r.Y, base)
		}
	}
}

// Three deep, so the join is a join and not a swap.
//
// With two decorations an arm that returned the wrong one is caught, but an arm
// that returned *one* of them is not distinguishable from an arm that dropped
// the other. A third line, declared a third level down in a third colour, says
// the list grows: the innermost box carries all three.
func TestDecorationsAccumulateDownTheTree(t *testing.T) {
	root := layoutOf(t, 600,
		`<div id="p"><span id="s"><em id="e">abcd</em></span></div>`,
		noDefaults+decoCSS+` #p { text-decoration: underline }
		 #s { text-decoration: overline; color: #ff0000 }
		 #e { text-decoration: line-through; color: #0000ff }`)
	ops := Paint(root)

	for _, c := range []struct {
		what   string
		colour style.RGBA
	}{
		{"the div's underline", black},
		{"the span's overline", style.RGBA{R: 255, A: 1}},
		{"the em's line-through", style.RGBA{B: 255, A: 1}},
	} {
		got := bands(ops, c.colour)
		if len(got) != 1 {
			t.Errorf("%s painted %d bands, want 1; three boxes each declare a "+
				"line and the text carries all three", c.what, len(got))
			continue
		}
		if w := got[0].W.Px(); w != 48 {
			t.Errorf("%s is %gpx wide, want 48 — the four characters all three "+
				"boxes contain", c.what, w)
		}
	}

	// All three at once, which is the assertion the loop above cannot make: a
	// painter that drew each line but only ever one of them would satisfy every
	// iteration separately.
	n := len(bands(ops, black)) + len(bands(ops, style.RGBA{R: 255, A: 1})) +
		len(bands(ops, style.RGBA{B: 255, A: 1}))
	if n != 3 {
		t.Errorf("%d decoration bands were painted in all, want 3", n)
	}
}
