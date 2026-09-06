package layout

import "testing"

// TestComposeReturnsTheSheetItLaidOutOn.
//
// The ops are measured from the corner of the content box, so the margins are
// how a backend gets from the corner of the paper to the origin — and the
// document may have changed both. An @page rule sets the size and the margins,
// and a caller with only the Options it passed in has the sheet it *asked* for
// rather than the one that was used.
func TestComposeReturnsTheSheetItLaidOutOn(t *testing.T) {
	asked := PageSizePt(595, 842).WithMarginPt(20) // A4, 20pt
	out := Compose(Input{
		HTML: `<p>x</p>`,
		CSS:  []Stylesheet{{Source: `@page { size: 200pt 300pt; margin: 10pt }`}},
	}, Options{Page: asked})

	if out.Page.Width == asked.Width && out.Page.Height == asked.Height {
		t.Errorf("Compose returned the sheet it was asked for (%v x %v); the "+
			"document's @page rule set another one",
			out.Page.Width.Pt(), out.Page.Height.Pt())
	}
	// Within a layout unit, which is what a length rounds to.
	near := func(got, want float64) bool { return got-want < 0.01 && want-got < 0.01 }
	if got := out.Page.Width.Pt(); !near(got, 200) {
		t.Errorf("the page is %gpt wide, want 200 — what @page said", got)
	}
	if got := out.Page.Height.Pt(); !near(got, 300) {
		t.Errorf("the page is %gpt tall, want 300 — what @page said", got)
	}
	if got := out.Page.Margin.Left.Pt(); !near(got, 10) {
		t.Errorf("the left margin is %gpt, want 10 — the offset a backend "+
			"translates the ops by", got)
	}
	// And it is the geometry the ops were actually laid out in: the content box
	// is 180pt wide, so a box of 100% width is 180pt and not 555.
	if out.Page.Content().W != out.Page.Width.Sub(out.Page.Margin.Horizontal()) {
		t.Error("the returned page's content box is not its own width less its margins")
	}
}

// TestComposeReturnsTheCallersSheetWhereTheDocumentSaysNothing is the control:
// almost every document has no @page rule, and the answer then is what was
// asked for.
func TestComposeReturnsTheCallersSheetWhereTheDocumentSaysNothing(t *testing.T) {
	asked := PageSizePt(400, 500).WithMarginPt(15)
	out := Compose(Input{HTML: `<p>x</p>`}, Options{Page: asked})
	if out.Page != asked {
		t.Errorf("Compose returned %+v for a document that says nothing about its "+
			"own page, want %+v", out.Page, asked)
	}
}
