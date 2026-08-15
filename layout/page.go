package layout

import (
	"fmt"

	"github.com/mgilbir/forme/style"
)

// The sheet a document is laid out onto.
//
// Page geometry is layout's and not a backend's: a point is a typographic unit
// rather than a PDF one, and how big the sheet is and how much of it the margins
// take is what the engine lays out against. What a *particular* backend then
// does about a page that did not fit — scale it, complain, refuse — travels with
// that backend.

// PageSize is the sheet a document is laid out onto.
type PageSize struct {
	// Width and Height are the whole sheet.
	Width, Height style.Unit
	// Margin is the space left around the content.
	Margin Edges
}

// PageSizePt builds a page size from a width and height in points, which is how
// paper is conventionally measured.
func PageSizePt(w, h float64) PageSize {
	return PageSize{Width: ptToUnit(w), Height: ptToUnit(h)}
}

// WithMarginPt returns the page with a uniform margin in points.
func (p PageSize) WithMarginPt(m float64) PageSize {
	u := ptToUnit(m)
	p.Margin = Edges{Top: u, Right: u, Bottom: u, Left: u}
	return p
}

// The paper sizes a document generator is actually asked for.
var (
	A4     = PageSizePt(595.276, 841.89).WithMarginPt(56.7) // 20mm
	A5     = PageSizePt(419.528, 595.276).WithMarginPt(42.5)
	Letter = PageSizePt(612, 792).WithMarginPt(54) // 0.75in
	Legal  = PageSizePt(612, 1008).WithMarginPt(54)
)

// ptToUnit converts points to layout units: a point is 1/72 inch and a CSS
// pixel is 1/96, so a point is 4/3 of a pixel.
func ptToUnit(pt float64) style.Unit {
	u, _ := style.FromPx(pt * 96 / 72)
	return u
}

// Content is the area a document is laid out in: the sheet minus its margins.
func (p PageSize) Content() Size {
	return Size{
		W: p.Width.Sub(p.Margin.Horizontal()),
		H: p.Height.Sub(p.Margin.Vertical()),
	}
}

// Options is what a caller can say about the sheet and about how hard the
// engine may try to make a document fit on it.
type Options struct {
	// Page is the sheet. The zero value is A4 with a 20mm margin.
	//
	// The faces are not here. They are on Input, because a document brings its
	// own with @font-face and the set it is laid out in is the caller's library
	// with the document's faces over it — see Input.Fonts and Built.Fonts.
	Page PageSize
	// MinScale is the floor §6.1 puts under scale-to-fit. A document that had
	// to be shrunk past it is refused rather than produced illegibly. Zero uses
	// the default of 0.5.
	MinScale float64
	// MinFontSizePt is the floor under an effective font size, in points. Zero
	// uses the default of 6.
	MinFontSizePt float64
	// AllowScaleUp lets an underfull page be enlarged to fill the sheet. It is
	// off by default because it is surprising and it degrades images.
	AllowScaleUp bool
}

// Composed is a document laid out and painted, ready for a backend.
type Composed struct {
	// Ops is the display list, in paint order.
	Ops []Op
	// Root is the fragment tree the display list was painted from.
	Root *Fragment
	// Scale is the factor of §5: 1 when the content fitted, less when it had to
	// be shrunk. It is reported because a caller may want to refuse a document
	// that only fitted by being made small.
	Scale float64
	// NaturalSize is what the content needed at its natural size, before any
	// scaling. It is what a caller adjusting a template needs to know.
	NaturalSize Size
	// Findings is everything the guardrails raised, in a deterministic order.
	Findings []Finding
	// Refused is a rule having fired at Error severity. A backend that sees it
	// should produce nothing: the caller was told not to render, rather than
	// left to decide.
	//
	// It is not a summary of Findings and cannot be recomputed from them. A
	// rule counts the moment it fires, before the list deduplicates and before
	// the bound below cuts it — so a document refused by its six-hundredth
	// finding is refused with that finding nowhere in the list. This field is
	// the authority; the list is the explanation, when there is room for one.
	Refused bool

	// Truncated is the bound having stopped findings being recorded, so what
	// Findings holds is some of them rather than all of them.
	//
	// A backend that reports findings has to say so. Presenting a cut list as a
	// complete one is how "three problems" becomes what a reader believes about
	// a document with four hundred.
	Truncated bool
}

// Compose is everything between a document and a backend: build the box tree,
// lay it out on the sheet, decide the scale, check that what came out is worth
// producing, and paint it.
//
// It stops one step short of a document, and that step is the only one that
// knows what a document is. A backend takes Ops and writes them — into a PDF,
// into a raster, into a test — and everything above this line is the same
// whichever it is.
func Compose(in Input, opts Options) Composed {
	if opts.Page.Width == 0 || opts.Page.Height == 0 {
		opts.Page = A4
	}
	if opts.MinScale == 0 {
		opts.MinScale = 0.5
	}
	if opts.MinFontSizePt == 0 {
		opts.MinFontSizePt = 6
	}

	built := Build(in)
	rec := NewRecorder(in.Policy)
	for _, f := range built.Findings {
		rec.ReportDetail(f)
	}
	// Build kept its own recorder and this one replays its findings, which
	// carries everything except the two answers that are not findings. A build
	// whose list overflowed has a verdict its list no longer explains — the
	// finding that refused it may be one of the ones the bound dropped — so
	// both are taken across rather than re-derived from what survived.
	buildRefused, buildTruncated := built.Failed, built.Truncated

	avail := opts.Page.Content()
	// built.Fonts rather than in.Fonts: the document's own @font-face rules
	// have been loaded onto the caller's library by now, and laying out with
	// the library alone would set the page in the wrong faces.
	root := Layout(built.Root, avail, built.Fonts, rec)

	// The natural size is the far edge of the root's border box, not its margin
	// box, and the difference is not cosmetic. A block-level box resolves an
	// over-constrained width by widening its right margin, so the root's margin
	// box is *always* exactly the page width — measuring that would report every
	// document as needing precisely the space it was given, and scale-to-fit
	// would never fire. The border box's far edges include the root's own left
	// and top margins, since those move it, and exclude the one that was
	// invented to make the arithmetic add up.
	natural := Size{}
	if root != nil {
		natural = Size{W: root.BorderRect.Right(), H: root.BorderRect.Bottom()}
	}

	scale := fitScale(natural, avail, opts.AllowScaleUp)
	checkScale(rec, scale, opts.MinScale)
	checkFontSizes(rec, root, scale, opts.MinFontSizePt)

	ops := Paint(root)
	checkPageOverflow(rec, ops, avail, scale)

	return Composed{
		Ops: ops, Root: root, Scale: scale, NaturalSize: natural,
		Findings:  rec.Findings(),
		Refused:   rec.Failed() || buildRefused,
		Truncated: rec.Truncated() || buildTruncated,
	}
}

// fitScale is §5's factor: one number, applied to everything.
//
// The proposal argues this at length and the argument decides the whole shape of
// the engine. Laying out again at a smaller size would reflow the text, which
// moves the line breaks, which changes the height — non-monotonically, since a
// smaller font can produce a *taller* block by breaking differently. Scaling the
// finished layout geometrically leaves every proportion as the author designed
// it, needs one pass, and makes the size of every element exactly its natural
// size times this number, so a threshold check is a multiplication rather than
// an iteration.
func fitScale(natural, avail Size, allowUp bool) float64 {
	s := 1.0
	if natural.W > 0 && natural.W > avail.W {
		s = min(s, avail.W.Px()/natural.W.Px())
	}
	if natural.H > 0 && natural.H > avail.H {
		s = min(s, avail.H.Px()/natural.H.Px())
	}
	if allowUp && natural.W > 0 && natural.H > 0 {
		up := min(avail.W.Px()/natural.W.Px(), avail.H.Px()/natural.H.Px())
		if up > s {
			s = up
		}
	}
	return s
}

// checkScale is the min-scale guardrail of §6.1.
//
// It is the blunt one and probably the most useful: if the content had to be
// shrunk past half to fit, the document is wrong, and no per-element threshold
// is needed to say so.
func checkScale(rec *Recorder, scale, floor float64) {
	if scale >= floor {
		return
	}
	rec.ReportDetail(Finding{
		Rule: RuleMinScale,
		Message: fmt.Sprintf(
			"the content had to be scaled to %.0f%% to fit the page, past the floor of %.0f%%",
			scale*100, floor*100),
	})
}

// checkFontSizes is the min-font-size guardrail of §6.1.
//
// Because the scale is geometric, the effective size of every element is exactly
// its natural size times the factor — so this is one multiplication per box,
// computed before anything is emitted, with no iteration and no possibility of a
// later pass invalidating it. That exactness is the whole reason §5 chose
// geometric scaling.
func checkFontSizes(rec *Recorder, root *Fragment, scale, floorPt float64) {
	if root == nil {
		return
	}
	seen := map[style.Unit]bool{}
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f.Box != nil && len(f.Lines) > 0 {
			size := f.Box.FontSize
			if !seen[size] {
				seen[size] = true
				effective := size.Mul(scale).Pt()
				if effective < floorPt {
					rec.ReportDetail(Finding{
						Rule: RuleMinFontSize,
						Message: fmt.Sprintf(
							"text would be set at %.2fpt, below the floor of %.2fpt"+
								" (%.2fpt before the page scaling of %.0f%%)",
							effective, floorPt, size.Pt(), scale*100),
						Path: PathOf(f.Box.Element),
					})
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
}

// checkPageOverflow is the overflow-page guardrail of §6.2.
//
// It should never fire. The scale of §5 is computed so that everything fits, so
// this is a self-check on that computation as much as a guardrail on the
// document — which is exactly why it is worth having. A threshold that verifies
// an earlier calculation catches the case where the calculation was wrong, and
// that is a class of fault no amount of checking the document can reach.
//
// Content that overflows its own *box* is the other guardrail's business; this
// is only about leaving the page.
func checkPageOverflow(rec *Recorder, ops []Op, avail Size, scale float64) {
	page := Rect{W: avail.W.Div(scale), H: avail.H.Div(scale)}
	var worst Rect
	var found bool

	for _, op := range ops {
		r, ok := op.(FillRect)
		if !ok || r.Rect.Empty() || r.Overhang {
			// A text decoration, and an inline box's background and border, are
			// skipped for the reason FillRect.Overhang gives: this guard is about
			// boxes the scale was computed from, and none of those is one.
			continue
		}
		if page.Contains(r.Rect) {
			continue
		}
		if !found || r.Rect.Right() > worst.Right() || r.Rect.Bottom() > worst.Bottom() {
			worst, found = r.Rect, true
		}
	}
	if !found {
		return
	}
	rec.ReportDetail(Finding{
		Rule: RuleOverflowPage,
		Message: fmt.Sprintf(
			"content reaches %.1f x %.1f px after scaling, outside the page's %.1f x %.1f; "+
				"the scale-to-fit calculation did not account for it",
			worst.Right().Px(), worst.Bottom().Px(), page.W.Px(), page.H.Px()),
	})
}
