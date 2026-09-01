package layout

import "testing"

// text-autospace inside the elements whose content is preformatted.
//
// CSS Text 4's own default style sheet turns the ideograph spacing off in them,
// and the reason is what they are for: the spacing is a typesetter's, and these
// elements hold text quoted exactly. An eighth of an em between a letter and an
// ideograph inside a fragment of program source changes what the reader is
// being shown.
//
// The suite's text-autospace-preformatted-001 is the five of them beside two
// paragraphs that do take the spacing, and its reference draws the gap as a
// margin so that the two can be told apart.

// spacedTo is how far the last run of a box reaches, which is where the gaps
// this rule is about show up.
func spacedTo(t *testing.T, markup string) float64 {
	t.Helper()
	root := layoutOf(t, 600, `<div id="d">`+markup+`</div>`,
		`body { margin: 0 }
		 #d, span, code, kbd, samp, pre, tt {
		   font-family: Courier; font-size: 16px; line-height: 1 }`)
	var far float64
	for _, op := range Paint(root) {
		if o, ok := op.(DrawText); ok && o.At.X.Px() > far {
			far = o.At.X.Px()
		}
	}
	if far == 0 {
		t.Fatalf("nothing was drawn for %q", markup)
	}
	return far
}

func TestAutospaceIsOffInsideThePreformattedElements(t *testing.T) {
	const text = `A水Z`
	spaced := spacedTo(t, `<span>`+text+`</span>`)
	off := spacedTo(t, `<span style="text-autospace:no-autospace">`+text+`</span>`)
	if spaced <= off {
		t.Fatalf("the spacing moves the last run from %g to %g, so this fixture "+
			"cannot tell the two apart", off, spaced)
	}
	for _, name := range []string{"code", "kbd", "samp", "pre", "tt"} {
		got := spacedTo(t, "<"+name+">"+text+"</"+name+">")
		if got != off {
			t.Errorf("<%s> put its last run at %g, want %g — the same place as "+
				"an explicit no-autospace, and not the %g a paragraph gets",
				name, got, off, spaced)
		}
	}
}

// TestAutospaceIsStillOnEverywhereElse, because a rule written against the
// wrong selector would turn it off for the whole document and every one of the
// rows above would still pass.
func TestAutospaceIsStillOnEverywhereElse(t *testing.T) {
	const text = `A水Z`
	off := spacedTo(t, `<span style="text-autospace:no-autospace">`+text+`</span>`)
	for _, markup := range []string{
		`<span>` + text + `</span>`,
		`<p>` + text + `</p>`,
		`<em>` + text + `</em>`,
		`<b>` + text + `</b>`,
		// And inside a preformatted element, which the rule sets on the element
		// and not on its subtree — but text-autospace inherits, so a child does
		// take it. This row is the inheritance and not the selector.
		`<div style="text-autospace:normal">` + text + `</div>`,
	} {
		if got := spacedTo(t, markup); got <= off {
			t.Errorf("%s put its last run at %g, no further than the %g of a "+
				"box with the spacing turned off", markup, got, off)
		}
	}
}
