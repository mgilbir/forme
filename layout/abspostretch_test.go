package layout

import "testing"

// TestAStretchedAbsoluteBoxIsDefiniteForItsChildren is audit C36.
//
// §10.6.4's fifth rule: with "height: auto" and neither "top" nor "bottom" auto,
// the height is what the two offsets leave of the containing block. The content
// is not an input to that, so the height is known before the box is laid out,
// and §10.5 resolves a percentage height inside the box against it. The overlay
// idiom — "inset: 0" round a "height: 100%" panel — is this document.
//
// Every number here is from the equation, against a positioned 400px wrapper
// with 10px of padding so the containing block is its 420px padding box.
func TestAStretchedAbsoluteBoxIsDefiniteForItsChildren(t *testing.T) {
	const wrap = `<div style="position:relative; height:400px; padding:10px">`
	for _, tc := range []struct {
		what, decl string
		outer      float64 // the stretched box's content height
	}{
		// 420 - 20 - 40 = 360.
		{"offsets alone", `top:20px; bottom:40px`, 360},
		// The box's own padding and border are inside the stretch and outside
		// the content box its child resolves against: 420 - 0 - 0 - 2*15 - 2*5.
		{"with padding and border", `top:0; bottom:0; padding:15px; border:5px solid`, 380},
		// A maximum is §10.4's clamp on the solved height, and the child sees
		// the clamped number.
		{"clamped by max-height", `top:0; bottom:0; max-height:100px`, 100},
		// Percentage offsets resolve against the containing block's height:
		// 420 - 42 - 84 = 294.
		{"percentage offsets", `top:10%; bottom:20%`, 294},
	} {
		root := layoutOf(t, 1000, wrap+`<div id="o" style="position:absolute; left:0; width:100px; `+
			tc.decl+`"><div id="c" style="height:50%"></div></div></div>`, noDefaults)
		o, c := find(t, root, "o"), find(t, root, "c")
		px(t, tc.what+": the stretched box's content height", o.ContentRect().H, tc.outer)
		px(t, tc.what+": its child's 50% height", c.BorderRect.H, tc.outer/2)
	}
}

// TestAnAbsoluteBoxAnchoredAtOneEndIsStillSizedByItsContent is the other side of
// the rule: with either offset auto the height is the content's, so a percentage
// inside it has nothing definite to resolve against and behaves as auto.
func TestAnAbsoluteBoxAnchoredAtOneEndIsStillSizedByItsContent(t *testing.T) {
	root := layoutOf(t, 1000, `<div style="position:relative; height:400px">`+
		`<div id="o" style="position:absolute; top:0; left:0; width:100px">`+
		`<div id="c" style="height:50%"><div style="height:30px"></div></div></div></div>`,
		noDefaults)
	px(t, "the box's height", find(t, root, "o").BorderRect.H, 30)
	px(t, "its child's height", find(t, root, "c").BorderRect.H, 30)
}
