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
	opts, optionsRefused := checkOptions(opts)

	// The sheet is settled before the document is styled, because a media
	// query is a question about it: "@media print" and "@media (min-width:
	// 200mm)" both decide which rules the cascade ever sees.
	built := BuildFor(in, opts.Page)
	rec := NewRecorder(in.Policy)
	for _, why := range optionsRefused {
		rec.ReportDetail(Finding{Rule: RuleInvalidCSS, Message: why})
	}
	for _, f := range built.Findings {
		rec.ReportDetail(f)
	}
	// The sheet the document settled on, checked the same way. An @page rule
	// writes into the same geometry the caller does and had nothing checking
	// it: "@page { margin: 100mm }" on A5 left a content box of negative width,
	// which came out as a scale of -1.60 and a finding saying the text would be
	// set at minus nineteen points.
	var pageRefused []string
	built.Page, pageRefused = checkPage(built.Page, "the @page rule's")
	for _, why := range pageRefused {
		rec.ReportDetail(Finding{Rule: RuleInvalidCSS, Message: why})
	}
	// Build kept its own recorder and this one replays its findings, which
	// carries everything except the two answers that are not findings. A build
	// whose list overflowed has a verdict its list no longer explains — the
	// finding that refused it may be one of the ones the bound dropped — so
	// both are taken across rather than re-derived from what survived.
	buildRefused, buildTruncated := built.Failed, built.Truncated

	// built.Page rather than opts.Page: an @page rule in the document may have
	// changed the margins, and laying out in the space the caller asked for
	// would ignore what the document said about its own.
	avail := built.Page.Content()
	// built.Fonts rather than in.Fonts: the document's own @font-face rules
	// have been loaded onto the caller's library by now, and laying out with
	// the library alone would set the page in the wrong faces.
	root := Layout(built.Root, avail, built.Fonts, rec)

	natural := naturalSize(root)

	scale := fitScale(natural, avail, opts.AllowScaleUp)
	checkScale(rec, scale, opts.MinScale)
	checkFontSizes(rec, root, scale, opts.MinFontSizePt)

	ops := PaintReporting(root, rec)
	checkPageOverflow(rec, ops, avail, scale)

	return Composed{
		Ops: ops, Root: root, Scale: scale, NaturalSize: natural,
		Findings:  rec.Findings(),
		Refused:   rec.Failed() || buildRefused,
		Truncated: rec.Truncated() || buildTruncated,
	}
}

// naturalSize is what the content needed before any scaling: the far edge of
// everything the document put on the page.
//
// It is measured over the whole fragment tree and not, as it was, over the
// root's border box alone. The root's own width is the page's by construction —
// a block-level box resolves an over-constrained width by widening its right
// margin — so reading it reported every document as needing exactly the space
// it was given, and horizontal scale-to-fit could not fire at all. A table two
// thousand pixels wide on a six-hundred-pixel page was drawn at its full width
// with no finding of any kind, which is the commonest way a document meant for
// the screen fails on paper.
//
// Vertical overflow was never affected, because the root's height does grow
// with its content. That is why the defect survived: the axis that reaches the
// page bound in ordinary use worked, and the other one had never been measured.
//
// The border box rather than the margin box, for the reason the root's case
// gives: the margin invented to make the arithmetic add up is not space the
// content needed. The far edges include a box's own left and top margins, since
// those move it.
//
// What an ancestor clips does not count. A box with "overflow: hidden" holding
// something twice its width shows what fits and no more, so scaling the page
// down to make room for the part nobody can see would shrink a document to fit
// something it deliberately hid. Layout has already resolved every clip onto
// the fragments by the time this runs, so the answer here is the same one the
// painter uses rather than a second reading of the same properties.
func naturalSize(root *Fragment) Size {
	if root == nil {
		return Size{}
	}
	var w, h style.Unit
	take := func(r Rect, c Clip) {
		if c.Active {
			r = c.Rect.Intersect(r)
		}
		if r.Empty() {
			return
		}
		if r.Right() > w {
			w = r.Right()
		}
		if r.Bottom() > h {
			h = r.Bottom()
		}
	}
	var walk func(f *Fragment)
	walk = func(f *Fragment) {
		take(f.BorderRect, f.clipSelf)
		content := f.ContentRect()
		for _, line := range f.Lines {
			// A line box is stated relative to the block's content box, and how
			// far the text on it reached is stated relative to the line box.
			// The second matters because a line box is the width it was given
			// rather than the width its content took: a run that cannot be
			// broken — "white-space: nowrap", one long word — reaches past it,
			// and that reach is content needing more room like any other.
			r := line.Rect
			for _, run := range line.Runs {
				if line.Anticlockwise {
					// Along the line is *up* the page here, so a run past the
					// line box's length reaches above it rather than below, and
					// nothing above the page's top edge is a thing scaling can
					// fix. See the painter, which measures this one back from
					// the line box's foot.
					continue
				}
				end := run.X.Add(run.Offset.X).Add(run.Width)
				switch {
				case line.Sideways && end > r.H:
					r.H = end
				case !line.Sideways && end > r.W:
					r.W = end
				}
			}
			r.X, r.Y = content.X.Add(r.X), content.Y.Add(r.Y)
			take(r, f.clipContent)
			// An inline box's own fragments hang from the line rather than from
			// the block's children — one per line it was broken across — so the
			// walk below never reaches them.
			for _, box := range line.Boxes {
				take(box.BorderRect, f.clipContent)
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return Size{W: w, H: h}
}

// checkOptions replaces anything in Options this engine cannot lay out against,
// and says what it replaced.
//
// None of these was checked. "MinScale: 2" is a floor above every scale there
// is and refused every document; a negative one turned the guard off silently,
// which is the same shape of mistake as a zero cap; a page one point wide left
// no content box at all. They are the caller's numbers rather than the
// document's, and a caller has no other way to be told — Compose returns no
// error — so they are reported as refused input like any other.
func checkOptions(opts Options) (Options, []string) {
	var why []string
	// The zero value is A4, which is what the field's own documentation says
	// and is the ordinary case. A *negative* or half-stated one is a mistake.
	switch {
	case opts.Page.Width == 0 && opts.Page.Height == 0 &&
		opts.Page.Margin == (Edges{}):
		opts.Page = A4
	case opts.Page.Width <= 0 || opts.Page.Height <= 0:
		why = append(why, fmt.Sprintf(
			"the page passed to Compose is %.1f x %.1f px, which is not a sheet; A4 was used",
			opts.Page.Width.Px(), opts.Page.Height.Px()))
		opts.Page = A4
	}
	var pageWhy []string
	opts.Page, pageWhy = checkPage(opts.Page, "the page passed to Compose's")
	why = append(why, pageWhy...)

	switch {
	case opts.MinScale == 0:
		opts.MinScale = 0.5
	case opts.MinScale < 0:
		why = append(why, fmt.Sprintf(
			"the minimum scale passed to Compose is %v, which no scale can be below, "+
				"so the guard would never fire; %v was used", opts.MinScale, 0.5))
		opts.MinScale = 0.5
	case opts.MinScale > 1:
		why = append(why, fmt.Sprintf(
			"the minimum scale passed to Compose is %v, which no scale can reach, "+
				"so every document would be refused; 1 was used", opts.MinScale))
		opts.MinScale = 1
	}
	switch {
	case opts.MinFontSizePt == 0:
		opts.MinFontSizePt = 6
	case opts.MinFontSizePt < 0:
		why = append(why, fmt.Sprintf(
			"the minimum font size passed to Compose is %vpt, which no size is below, "+
				"so the guard would never fire; %vpt was used", opts.MinFontSizePt, 6.0))
		opts.MinFontSizePt = 6
	}
	return opts, why
}

// checkPage replaces margins that leave no sheet to lay out on, and says what
// it replaced. whose names where the geometry came from, since the same numbers
// reach here from a caller and from an @page rule.
//
// A negative margin is not a smaller margin: it puts the content box outside
// the paper, where nothing is printed. Margins wider than the sheet are the
// same thing said with two numbers, and left alone they made the content box
// negative — which came out of the scale-to-fit arithmetic as a scale of minus
// one and a half.
func checkPage(page PageSize, whose string) (PageSize, []string) {
	var why []string
	for _, side := range []struct {
		name string
		at   *style.Unit
	}{
		{"top", &page.Margin.Top}, {"right", &page.Margin.Right},
		{"bottom", &page.Margin.Bottom}, {"left", &page.Margin.Left},
	} {
		if *side.at < 0 {
			why = append(why, fmt.Sprintf("%s %s margin is %.1f px; a margin outside "+
				"the paper prints nothing, so it was read as none",
				whose, side.name, side.at.Px()))
			*side.at = 0
		}
	}
	if h := page.Margin.Horizontal(); page.Width > 0 && h >= page.Width {
		why = append(why, fmt.Sprintf("%s left and right margins come to %.1f px on a "+
			"sheet %.1f px wide, which leaves nothing to print in; they were dropped",
			whose, h.Px(), page.Width.Px()))
		page.Margin.Left, page.Margin.Right = 0, 0
	}
	if v := page.Margin.Vertical(); page.Height > 0 && v >= page.Height {
		why = append(why, fmt.Sprintf("%s top and bottom margins come to %.1f px on a "+
			"sheet %.1f px tall, which leaves nothing to print in; they were dropped",
			whose, v.Px(), page.Height.Px()))
		page.Margin.Top, page.Margin.Bottom = 0, 0
	}
	return page, why
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
// its natural size times the factor — so this is one multiplication per run,
// computed before anything is emitted, with no iteration and no possibility of a
// later pass invalidating it. That exactness is the whole reason §5 chose
// geometric scaling.
//
// Per *run* and not per block. A block container's own font size is the size
// its text inherits, and any inline box inside it may set another: a paragraph
// at 20px holding a two-pixel span was checked at twenty and drawn at two. The
// runs are what is drawn, and each carries the size it will be drawn at.
func checkFontSizes(rec *Recorder, root *Fragment, scale, floorPt float64) {
	if root == nil {
		return
	}
	seen := map[style.Unit]bool{}
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		for _, line := range f.Lines {
			for _, run := range line.Runs {
				if run.Text == "" || seen[run.Size] {
					continue
				}
				seen[run.Size] = true
				effective := run.Size.Mul(scale).Pt()
				if effective >= floorPt {
					continue
				}
				rec.ReportDetail(Finding{
					Rule: RuleMinFontSize,
					Message: fmt.Sprintf(
						"text would be set at %.2fpt, below the floor of %.2fpt"+
							" (%.2fpt before the page scaling of %.0f%%)",
						effective, floorPt, run.Size.Pt(), scale*100),
					Path: PathOf(boxElement(run.Box)),
				})
			}
		}
		// A list item's marker is text a box draws that is on no line of its
		// own, so it is asked about separately and at its own size.
		if m := f.Marker; m != nil && m.Text != "" && m.Image == nil && !seen[m.Size] {
			seen[m.Size] = true
			if effective := m.Size.Mul(scale).Pt(); effective < floorPt {
				rec.ReportDetail(Finding{
					Rule: RuleMinFontSize,
					Message: fmt.Sprintf(
						"a list marker would be set at %.2fpt, below the floor of %.2fpt"+
							" (%.2fpt before the page scaling of %.0f%%)",
						effective, floorPt, m.Size.Pt(), scale*100),
					Path: PathOf(boxElement(f.Box)),
				})
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
//
// Every operation that puts a rectangle of ink on the page is checked, and not
// only the fills. An <img> is a rectangle layout placed exactly as a <div> with
// a background is, and checking one and not the other meant the same box off
// the same page was refused or silent depending on whether what filled it was a
// colour or a picture. Text is the one thing still not checked, for the reason
// FillRect.Overhang gives: a glyph's ascender is ink no layout decision placed,
// and refusing a document over two pixels of it is what that flag exists to
// stop.
func checkPageOverflow(rec *Recorder, ops []Op, avail Size, scale float64) {
	page := Rect{W: avail.W.Div(scale), H: avail.H.Div(scale)}
	// How far a rectangle reaches outside the page on its worst side. Measuring
	// this rather than the far corner is what makes the report readable for a
	// box at a negative coordinate: such a box "reaches" less far right than
	// one inside the page, and picking the worst by its right and bottom edges
	// alone named an innocent box and quoted its far corner as the problem.
	excess := func(r Rect) style.Unit {
		out := style.Unit(0)
		for _, d := range [...]style.Unit{style.Unit(0).Sub(r.X), style.Unit(0).Sub(r.Y),
			r.Right().Sub(page.W), r.Bottom().Sub(page.H)} {
			if d > out {
				out = d
			}
		}
		return out
	}

	var worst Rect
	var worstBy style.Unit
	var found bool
	consider := func(r Rect) {
		if r.Empty() || page.Contains(r) {
			return
		}
		if by := excess(r); !found || by > worstBy {
			worst, worstBy, found = r, by, true
		}
	}

	for _, op := range ops {
		switch o := op.(type) {
		case FillRect:
			if o.Overhang {
				// A text decoration, and an inline box's background and border,
				// are skipped for the reason FillRect.Overhang gives: this
				// guard is about boxes the scale was computed from, and none of
				// those is one.
				continue
			}
			consider(o.Rect)
		case DrawImage:
			r := o.Rect
			if o.Clip.Active {
				// What is drawn is what the clip admits, which is also what the
				// scale was computed from.
				r = o.Clip.Rect.Intersect(r)
			}
			consider(r)
		case TileImage:
			// The clip is the area painted; nothing is drawn outside it,
			// including the part of a tile that reaches past it.
			consider(o.Clip)
		}
	}
	if !found {
		return
	}
	rec.ReportDetail(Finding{
		Rule: RuleOverflowPage,
		Message: fmt.Sprintf(
			"content occupies %.1f,%.1f to %.1f,%.1f px after scaling, reaching %.1f px "+
				"outside the page's %.1f x %.1f; the scale-to-fit calculation did not account for it",
			worst.X.Px(), worst.Y.Px(), worst.Right().Px(), worst.Bottom().Px(),
			worstBy.Px(), page.W.Px(), page.H.Px()),
	})
}
