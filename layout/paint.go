package layout

import (
	"fmt"
	"image"
	"math"
	"sort"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// The display list: the stage after layout, and the one that has no PDF in it
// at all.
//
// Keeping this apart from the stage that writes a content stream is what makes
// this package testable without a backend: the reftest comparison reads display
// lists, and so does every test here. The display list is where a rasterizer
// attaches, and it separates "did we lay this out correctly" from "did we emit
// correct PDF" — two failure modes that are miserable to debug together, because
// each can produce a page that looks exactly like the other's symptom.
//
// It is also why the coordinates here are still CSS's: origin at the top left,
// y increasing downwards, lengths in layout units. The flip to PDF's bottom-left
// origin and the conversion to points happen once, in the backend, and a coordinate
// system that changed halfway through would make every sign error plausible.

// Op is one primitive of the display list.
//
// The set is deliberately small. Anything a backend cannot draw directly is
// something this stage should have decomposed — a border is four filled bands
// rather than a "border" primitive, because a backend that had to understand
// border-collapse would be a second layout engine.
//
// There are five: FillRect, DrawText, DrawImage and TileImage, which put ink on
// the page, and Link, which puts none and says where a hyperlink is. A backend
// that switches over them must have a case for each, and one that only draws
// may skip Link.
type Op interface{ isOp() }

// FillRect paints a rectangle in a solid colour.
type FillRect struct {
	Rect  Rect
	Color style.RGBA
	// Overhang marks a fill whose position no layout decision accounted for: a
	// text decoration, and the background and border of an inline box.
	//
	// It exists for the overflow-page guardrail, which is about *boxes* leaving
	// the page and reads the display list to find them. Text is not checked by it
	// at all — a glyph whose ascender reaches above the page top produces a
	// DrawText, which the guard skips — so an overline over the same letters must
	// be skipped too. Without this, "line-height: 0.3" on the first line of a page
	// puts the overline a few pixels above the top edge, the guard fires at Error
	// severity, and no document is produced at all: an overhang of two pixels
	// turned into a refusal, from a rule whose whole purpose is to catch a wrong
	// scale calculation.
	//
	// An inline box's decoration is the same case and reaches it by the same
	// route. §10.6.1 gives the box a content area the height of its *font* rather
	// than of the line it sits on, and §8.4 and §8.5 keep its vertical border and
	// padding out of layout entirely — so a "line-height: 0.5" span, or one with
	// ten pixels of padding, puts ink above the first line of a page that nothing
	// in the flow ever measured. The scale-to-fit calculation cannot have
	// accounted for it, so a guard checking that calculation must not read it.
	Overhang bool
}

// DrawText draws a run of text with the origin of its baseline at At.
//
// The position is the baseline rather than the top of the line box, because that
// is what a text-drawing backend takes and because converting between them needs
// the face's metrics — which this stage has and the backend may not.
type DrawText struct {
	At   Point
	Text string
	// RTL says the run reads right to left.
	//
	// The text is in *logical* order — the order it is written and read, which
	// is what a reader copying it out of the page expects and what the string
	// here has to be for the text of the document to survive. Which way the
	// glyphs go is a separate fact, and it is one the backend has to be told
	// rather than one it can work out: a run of punctuation between two Hebrew
	// words is right-to-left because of its neighbours, and by the time the run
	// reaches a backend the neighbours are gone.
	RTL bool
	// Sideways says the run is set down the page rather than across it: each
	// glyph is turned ninety degrees clockwise from the way it stands in the
	// font, and the advance from one to the next goes downwards from At.
	//
	// It is the one thing a quarter turn does not do to a display list for
	// free. Every rectangle in the list — a background, a border, a clip —
	// comes out of a ninety degree rotation as another rectangle, so the turn
	// is already expressed in the coordinates by the time a backend sees them;
	// glyphs are the exception, because a glyph is a shape and not a rectangle
	// and the only thing that can turn one is whatever draws it.
	//
	// See layout/writingmode.go, which is where the rest of the argument is.
	Sideways bool
	// Anticlockwise says the turn went the other way: each glyph is turned
	// ninety degrees *anticlockwise* from the way it stands in the font, and
	// the advance from one to the next goes upwards from At.
	//
	// It goes with Sideways rather than instead of it, the way Upright does,
	// and for the same reason: the run still runs along the page's vertical
	// axis and a backend that ignored this would still put the glyphs in the
	// right column. What it would get wrong is which way up they stand and
	// which end of the column the first one is at.
	//
	// "writing-mode: sideways-lr" is the only thing that asks for it. CSS
	// Writing Modes §3.1 gives that mode a left-to-right block flow like
	// vertical-lr's and the *other* quarter turn, which is the one thing about
	// it a permutation of the four sides cannot hide from a backend.
	Anticlockwise bool
	// Upright says the glyphs are *not* turned: each stands the way it does in
	// the font and the pen moves one em down to the next one, whatever the
	// face's horizontal advance for it is. It is what "text-orientation:
	// upright" asks for, and it goes with Sideways rather than instead of it —
	// the run still runs down the page, and only the glyphs on it are different.
	//
	// The em is the advance because CSS Writing Modes §4.4 says to synthesize
	// the vertical metrics a face does not state, and the em box is the
	// synthesis. It is not an approximation of a number the font has: for the
	// faces this engine reads, there is no such number.
	Upright bool
	Face    *shape.Face
	Size    style.Unit
	Color   style.RGBA
	// PreContext and PostContext are the text either side of this run, where the
	// boundary between it and its neighbour did not break shaping.
	//
	// A backend shapes this run from its text and its face, and in a cursive
	// script a letter's shape comes from its neighbours — so a run drawn without
	// them comes out in isolated forms, and at a different width from the one
	// layout measured and placed the next run at. CSS Text §8.1 is why a run is
	// not always a whole word; layout's shapingcontext.go is where the boundary
	// is decided.
	//
	// They are context and not content: nothing of them is drawn, and nothing of
	// them belongs to the text a reader extracts from the page.
	PreContext, PostContext string
	// MergePre and MergePost say that side may contribute glyphs and not only
	// forms. See paragraph.Item.MergePre.
	MergePre, MergePost string
	// ContextKerns says the neighbours above are set in this run's own face, so
	// a pair that spans the boundary is this font's pair. It is false where font
	// fallback put the neighbour in another face: a character is the same
	// character whichever font sets it and still decides this run's joined
	// shapes, but a kerning pair is one font's statement about two of its own
	// glyphs and does not cross a font change.
	ContextKerns bool
	// CharSpacing is letter-spacing: an extra advance after every typographic
	// character unit.
	//
	// A unit and not a character, which is a distinction the backend has to
	// make. CSS Text §2's typographic character unit is a grapheme cluster: a
	// Thai letter carries its vowel sign and its tone mark, a Khmer consonant
	// carries the vowel that follows it, and a pen that added this after every
	// glyph would move a mark off the letter it is drawn on. The rule is that
	// it falls after the last glyph of each cluster — see the comparison's
	// spacingAfterGlyph, which is the reference reading of it.
	//
	// It is a property of the drawing rather than of the position because layout
	// already spent it — the run's width includes it, and the run after this one
	// is placed accordingly — so a backend that ignored it would draw the glyphs
	// bunched at the left of a gap the right size.
	CharSpacing style.Unit

	// Features is what the document turned off: a font's own rules that a CSS
	// property or a CSS Text rule has overruled.
	//
	// It is on the run because a backend shapes the run for itself, and a
	// backend that shaped it with the font's rules intact would draw a ligature
	// the engine measured as two letters — a run measured to one width and
	// painted at another, which is the failure this package has a comment about
	// wherever it could happen. See shape.Features.
	Features shape.Features

	// Clip is §11.1's clipping, when something cuts this run.
	//
	// It is set only when the clip really does cut the run: a run wholly inside
	// its clip carries none, and one wholly outside is not emitted at all. That
	// is not an optimisation — it is what keeps the display list of a document
	// whose text merely happens to sit inside an "overflow: hidden" box
	// identical to the list of one that does not, which is what the reftest
	// comparison needs to be able to say the two look the same.
	Clip Clip
}

// DrawImage paints a decoded image to fill a rectangle.
//
// The image is stretched to the rectangle, and the rectangle is where the
// picture goes rather than where the box is. For "object-fit: fill", which is
// the initial value, the two are the same and it is the element's content box:
// inside its padding, inside its border. For every other value objectfit.go has
// already worked out a smaller or larger rectangle of the picture's own shape,
// and set Clip when that rectangle reaches outside the box.
type DrawImage struct {
	Rect  Rect
	Image image.Image
	// Key identifies the source bytes. Two elements naming one file carry the
	// same key, which is what lets the backend embed the picture once — a
	// document with a logo in a header repeated on every row would otherwise
	// carry it as many times as it is drawn.
	Key string
	// Clip is §11.1's clipping, when something clips this picture.
	//
	// It cannot be folded into Rect the way a fill's is, because Rect is where
	// the picture is *stretched to*: narrowing it would squeeze the whole
	// image into the visible strip rather than cutting the part that is not.
	// A backend must intersect its own clipping path with this.
	Clip Clip
}

// TileImage paints a picture repeatedly across a rectangle.
//
// It is one operation for a whole tiling rather than one per tile, and that is a
// decision about safety rather than about tidiness. The number of tiles is
// (area / tile size), and a stylesheet chooses both: "background-size: 0.001px"
// with "repeat" over an A4 page is four hundred billion placements. An engine
// that emitted one operation each would allocate until it died on a document an
// attacker wrote, and no cap on the *document* bounds it, because nothing in the
// document says the number.
//
// So the tiling leaves layout as a description — where the first tile is, how
// far apart they are, and what area they may be drawn into — and the count
// appears nowhere. A backend expands it with the mechanism it has: PDF has
// tiling patterns, which are exactly this value. The count is still checked
// against maxBackgroundTiles before this is built, because whatever expands it
// is entitled not to be handed four hundred billion cells.
type TileImage struct {
	// Clip is the area painted. Nothing is drawn outside it, including the part
	// of a tile that reaches past it.
	Clip Rect
	// Tile is the first tile: its position and its size.
	Tile Rect
	// StepX and StepY are the distance to the next tile on each axis, and are
	// always greater than zero. An axis that does not repeat has a step of the
	// tile's own size and a Clip no wider than one tile, so the neighbouring
	// cells fall outside — which is what keeps a backend from needing a case for
	// "does not repeat".
	StepX, StepY style.Unit

	Image image.Image
	// Key identifies the source bytes, so a backend embeds one picture once.
	Key string
}

// Tiles is how many tiles touch the clip on each axis, which is what a consumer
// that expands them needs before it starts.
func (t TileImage) Tiles() (cols, rows int) {
	if t.StepX <= 0 || t.StepY <= 0 || t.Clip.Empty() || t.Tile.Empty() {
		return 0, 0
	}
	return tileSpan(t.Clip.X, t.Clip.Right(), t.Tile.X, t.Tile.W, t.StepX),
		tileSpan(t.Clip.Y, t.Clip.Bottom(), t.Tile.Y, t.Tile.H, t.StepY)
}

// tileSpan counts the tiles on one axis that overlap [clipLo, clipHi).
//
// A tile at tileLo + k·step covers [that, that + size), so it overlaps when
// k·step < clipHi − tileLo and k·step > clipLo − tileLo − size. The two bounds
// are what the floor and the ceiling below are: the index of the first tile that
// reaches into the clip, and of the last one that starts before it ends.
func tileSpan(clipLo, clipHi, tileLo, size, step style.Unit) int {
	lo := math.Floor(clipLo.Sub(tileLo).Sub(size).Px()/step.Px()) + 1
	hi := math.Ceil(clipHi.Sub(tileLo).Px()/step.Px()) - 1
	if hi < lo {
		return 0
	}
	n := hi - lo + 1
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(n)
}

// Link is a hyperlink: the areas of the page that follow it, and where to.
//
// It draws nothing. It is in the display list because a backend that makes a
// link annotation needs the areas, and the areas are layout's geometry: the
// alternative is a backend finding the <a>'s boxes again in the fragment tree,
// which is the second layout engine this list exists to make unnecessary. See
// link.go for which elements are links.
//
// There is one Link per <a>, however many places it is on the page, and it is
// at the place in the list where the first of its areas was painted. An area
// on a line is painted just before that line's text, and the area of a box
// with the box's background — so a link in the flow is in document order, and
// one Appendix E moves out of the flow is where its box is painted.
type Link struct {
	// Rects are the border boxes of the <a>'s fragments: one for each line an
	// inline <a> is broken across, one for a block-level or atomic <a>, and one
	// for each image, inline-block, float, absolutely positioned box or lifted
	// block that an inline <a> holds and its line fragments do not enclose. They
	// are in the order they were painted, and they may overlap.
	//
	// Each is cut to whatever clips the box it came from — an "overflow:
	// hidden" ancestor, a "clip" — exactly as a fill there would be, so no part
	// of a link reaches outside what is visible of it. An area clipped away
	// entirely is not here, and a link with no area left is not in the list at
	// all: a reader cannot click what they cannot see. An area whose box is
	// "visibility: hidden" is not here either, since §11.2's box is not drawn
	// and a browser does not send a click to it; opacity is no such thing, and
	// a transparent link is still a link.
	//
	// Where two links' areas overlap, the one later in the list is on top, as
	// a later mark is on top of an earlier one, and a browser gives a click to
	// what is on top. A link inside another in the flow — XHTML can nest them,
	// and HTML can through a table — is the later of the two, so a backend
	// that lets the last annotation win gives the click to the innermost link,
	// which is the one a browser follows.
	Rects []Rect
	// Href is where the link goes, as the URL standard reads it and otherwise
	// as the document wrote it.
	//
	// It is an http, https or mailto URL, or a reference with no scheme — a
	// relative one, which is relative to the document and is left for the
	// backend to resolve against the document's address, since this engine is
	// never told it; or a fragment, "#x", into the document itself. Every other
	// scheme is refused before it gets here, and is reported. See link.go.
	Href string

	// of is the <a> the areas belong to, while the paint gathers them into one
	// Link. It is nil in every Link that leaves Paint.
	of *hyperlink
}

func (FillRect) isOp()  {}
func (DrawText) isOp()  {}
func (DrawImage) isOp() {}
func (TileImage) isOp() {}
func (Link) isOp()      {}

// Paint turns a fragment tree into a display list, in painting order.
//
// The order is CSS 2.1 Appendix E. It used to be tree order, with a note saying
// that this would stop being true the moment positioning arrived, and it has:
// tree order is painting order only while nothing is out of flow and nothing
// asks to be painted somewhere else in the stack. Both are now possible, so what
// is here is the real algorithm.
//
// # What a stacking context is, and what it is not
//
// §9.9 and Appendix E divide the tree into *stacking contexts*, each of which is
// painted as an atomic unit in the eight steps of §E.2. The root element makes
// one. So does
// a positioned box with a z-index that is not auto — and only that, which is the
// distinction the whole scheme turns on and the one that reads as a technicality
// until it bites. A positioned box with "z-index: auto" is painted as a unit,
// at the same level as one with "z-index: 0", but it does *not* make a stacking
// context: its own positioned descendants are hoisted out and sorted against its
// siblings rather than against each other. So a descendant with "z-index: -1"
// paints behind an ancestor whose z-index is auto and in front of one whose
// z-index is 0, from the same markup. Collapsing auto onto 0 gives a page where
// that descendant is simply not visible, which looks like a missing box rather
// than like a stacking bug.
//
// # What each step of Appendix E is for
//
// The eight steps exist to make three guarantees that tree order alone does not.
// Backgrounds of every ordinary block in a subtree are painted before any of
// the text in it, so a later sibling's background cannot cover an earlier one's
// words. Floats are a layer of their own between the two, so a floated image
// sits over the block backgrounds it overlaps and under the text that runs
// around it. And everything positioned is painted after everything that is not,
// which is what makes "position: relative" with no offsets at all a way to lift
// a box above its neighbours — a fact that looks like an accident of the
// specification and is relied on constantly.
//
// # What is not done
//
// A transform creates a stacking context and is not implemented, so it does not
// appear here. Opacity does and is: see dimming, which works out what fraction
// of each fragment's own marks reaches the page and which box asked for it.
// Every step of §E.2 is present, reduced to the primitives this engine emits.
func Paint(root *Fragment) []Op { return PaintReporting(root, nil) }

// PaintReporting is Paint, with the findings the painting itself raises.
//
// There is one kind: an opacity group whose marks could not be folded into. It
// cannot be raised in layout, where every other finding about a box is raised,
// because the question is about the marks the box turned out to paint and
// nothing before the paint knows them — so the recorder comes here instead of
// the finding going there. See opacity.go.
//
// A nil recorder is the plain Paint, and is what the package's own tests use:
// they read the display list and none of them asks what was reported. The path
// that matters is Compose's, and TestAGroupThatOverlapsItselfIsReported goes
// through it.
func PaintReporting(root *Fragment, rec *Recorder) []Op {
	if root == nil {
		return nil
	}
	if rec == nil {
		// A recorder nobody reads, for the reason newLayouter makes one: the
		// work budget it carries bounds the paint whether or not anyone is
		// told what it cut.
		rec = NewRecorder(nil)
	}
	p := &painter{colors: map[string]style.RGBA{}, rec: rec}
	p.dimming(root, 1, nil)
	p.findInlineLevels(root)
	p.canvasBackground(root)
	p.stackingContext(root)
	p.settleGroups()
	for _, b := range p.order {
		p.groups[b].report(rec)
	}
	return gatherLinks(p.ops)
}

// dimming works out, before anything is painted, how much of each fragment's
// own paint reaches the page and which boxes decided it.
//
// CSS Color 4 applies opacity to every element, and what it dims is the
// element and all of its descendants — the element tree's, not the fragment
// tree's. The two differ in exactly one kind of box: a non-atomic inline box,
// which has no fragment among Children at all. Its text is runs on its block's
// lines, its background is a fragment per line on LineFragment.Boxes, and an
// inline-block, a float or an absolutely positioned box written inside it
// hangs from the block's fragment as though the span were not there. A walk of
// fragments alone therefore never met the span's opacity, and "<span
// style='opacity: 0'>secret</span>" printed the secret at full strength (audit
// C32). So every fragment, run and inline fragment is dimmed by the inline
// boxes between it and the fragment it hangs from as well — see inlineDim.
//
// owners is outermost first and is carried down rather than looked up, because
// a block that was lifted out of an inline is no longer inside it — §9.2.1.1
// made it a sibling of the inline's two halves — and the opacity the inline
// asked for still covers it. That is the whole of what splitFrom is for here,
// and it is the case opacity-affects-block-in-inline draws.
func (p *painter) dimming(f *Fragment, a float64, owners *opacityOwner) {
	if f == nil || f.Box == nil {
		return
	}
	p.dimFragment(f, dim{a: a, owners: owners})
}

// dim is how much of one mark's paint reaches the page, and the boxes that
// asked for it, innermost first. The zero value is a mark nothing dims: an
// alpha below one always has a box that asked for it.
type dim struct {
	a      float64
	owners *opacityOwner
}

// dimmed reports whether any box around the mark asked for opacity.
func (d dim) dimmed() bool { return d.owners != nil }

// with is d with one more box's opacity inside it.
func (p *painter) with(d dim, b *Box) dim {
	if !groupsItsPaint(b) {
		return d
	}
	if !d.dimmed() {
		d.a = 1
	}
	return dim{a: d.a * opacityOf(b.Style), owners: p.groupFor(b, d.owners)}
}

// dimFragment records a fragment's dimming, given what the fragment it hangs
// from was dimmed by, and carries on into everything painted from it.
func (p *painter) dimFragment(f *Fragment, around dim) {
	d := around
	for _, from := range f.Box.splitFrom {
		d = p.with(d, from)
	}
	d = p.with(d, f.Box)
	p.setDim(f, d)
	// The lines, in tree order after the box itself and before its children,
	// which is the order the groups are reported in. A run and an inline
	// fragment are dimmed by the inline boxes between them and this block as
	// well as by the block.
	for _, line := range f.Lines {
		for _, box := range line.Boxes {
			if box.Box != nil {
				p.setDim(box, p.inlineDim(box.Box, d))
			}
		}
		for i := range line.Runs {
			if b := line.Runs[i].Box; b != nil {
				p.inlineDim(b, d)
			}
		}
	}
	for _, c := range f.Children {
		if c == nil || c.Box == nil {
			continue
		}
		// The inline boxes the child was written inside, which have no fragment
		// of their own on the way down to it: an inline-block, a float or an
		// absolutely positioned box inside a translucent <span> is part of the
		// span's group.
		p.dimFragment(c, p.inlineDim(c.Box.Parent, d))
	}
}

// setDim records a fragment's dimming, when there is any.
func (p *painter) setDim(f *Fragment, d dim) {
	if !d.dimmed() {
		return
	}
	if p.alpha == nil {
		p.alpha, p.owners = map[*Fragment]float64{}, map[*Fragment]*opacityOwner{}
	}
	p.alpha[f], p.owners[f] = d.a, d.owners
}

// inlineDim is the dimming of what is painted inside box b, given that the
// block container whose lines b is on is dimmed by base: base, with the
// opacity of every non-atomic inline box from b outwards folded in.
//
// The walk stops at the first box that is not one — the block container, or an
// atomic inline, each of which has a fragment and was dimmed as one. A text box
// is walked through rather than counted: it carries its parent element's whole
// computed style, opacity included, and counting it would dim a translucent
// span's words twice.
//
// It is memoized per box, because every run on every line asks it and a run
// inside two hundred nested spans would otherwise walk all of them. The answer
// depends on base, so the memo keeps the base it was computed from and is not
// trusted for any other — which no layout this engine produces asks for, since
// an inline box is on exactly one block's lines.
func (p *painter) inlineDim(b *Box, base dim) dim {
	var chain []*Box
	d := base
	for cur := b; cur != nil && cur.Outer == OuterInline; cur = cur.Parent {
		if cur.Replaced != nil || isAtomicInline(cur) {
			break
		}
		if cur.IsText() {
			continue
		}
		if m, ok := p.inlineDims[cur]; ok && m.base == base {
			d = m.dim
			break
		}
		chain = append(chain, cur)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		d = p.with(d, chain[i])
		if p.inlineDims == nil {
			p.inlineDims = map[*Box]memoDim{}
		}
		p.inlineDims[chain[i]] = memoDim{base: base, dim: d}
	}
	return d
}

// memoDim is one entry of inlineDims: what is inside an inline box is dimmed
// by dim, when its block is dimmed by base.
type memoDim struct{ base, dim dim }

// dimOf is what a fragment's own marks are dimmed by.
func (p *painter) dimOf(f *Fragment) dim {
	o := p.owners[f]
	if o == nil {
		return dim{}
	}
	return dim{a: p.alpha[f], owners: o}
}

// opacityOwner is one box that asked for opacity, and the chain of the ones
// outside it.
//
// A list rather than a slice, and the reason is worth the type. Two siblings
// under one group are handed the same chain to extend; extending a *slice*
// appends to it twice, and if it happens to have spare capacity the second
// append overwrites the first's entry — so the elder sibling's marks are
// credited to the younger and the elder is never reported. Whether the slice has
// spare capacity depends on how the runtime grew it three levels up, which is to
// say the bug is invisible until a document is nested one box deeper. Copying
// the slice at every level would fix it and would leave the next reader free to
// stop copying; a list cannot be extended in place at all.
type opacityOwner struct {
	box *Box
	up  *opacityOwner
}

// groupFor records a box that asked for opacity, once, and puts it at the head
// of the chain covering what is inside it.
//
// Once per *element*, not per box. CSS Color 4's group is the element and
// everything inside it, and §9.2.1.1 makes several boxes of one inline
// element: the pieces of a <span> broken around a block inside it are copies of
// the span, each in its own anonymous block, and the block itself names the
// original.
// Keyed by box, one translucent span was three groups — three accounts of what
// it painted, each checked for overlap without the other two, and three
// reports about one declaration.
func (p *painter) groupFor(b *Box, owners *opacityOwner) *opacityOwner {
	b = p.groupBox(b)
	if p.groups == nil {
		p.groups = map[*Box]*group{}
	}
	if _, seen := p.groups[b]; !seen {
		g := &group{box: b, alpha: opacityOf(b.Style)}
		if owners != nil {
			parent := p.groups[owners.box]
			parent.children = append(parent.children, g)
		}
		p.groups[b] = g
		p.order = append(p.order, b)
	}
	return &opacityOwner{box: b, up: owners}
}

// groupKey is what makes two boxes one group: the element they came from, and
// which of its pseudo-elements, if either.
type groupKey struct {
	el     *html.Node
	pseudo string
}

// groupBox is the box that stands for b's element in the groups: the first box
// of that element met, or b itself for a box no element generated.
func (p *painter) groupBox(b *Box) *Box {
	if b.Element == nil || b.IsText() {
		return b
	}
	k := groupKey{b.Element, b.Pseudo}
	if first, ok := p.elementGroups[k]; ok {
		return first
	}
	if p.elementGroups == nil {
		p.elementGroups = map[groupKey]*Box{}
	}
	p.elementGroups[k] = b
	return b
}

// grouped paints a fragment's own marks and folds into them the opacity of
// every box around it that asked for one.
func (p *painter) grouped(f *Fragment, paint func()) { p.as(p.dimOf(f), paint) }

// as paints marks and folds a dimming into them, crediting them to the
// innermost box that asked for it.
//
// Every mark goes through here exactly once: a fragment's own through grouped,
// and a run of text or an inline box's fragment with what inlineDim found for
// it. Nothing here nests — a block's lines are not painted inside the block's
// own call — because a mark folded twice would carry the block's alpha twice
// and be held by two groups.
func (p *painter) as(d dim, paint func()) {
	if !d.dimmed() {
		paint()
		return
	}
	at := len(p.ops)
	paint()
	ops, marks := dimOps(p.ops, at, d.a)
	p.ops = ops
	// To the innermost group only. The groups around it read these through
	// their children when they are settled; appending to every one of them
	// held each mark once per level of nested opacity (audit C21).
	g := p.groups[d.owners.box]
	g.marks = append(g.marks, marks...)
}

// canvasBackground paints the page's own background, before anything else.
//
// It is CSS 2.1 §14.2 and css-backgrounds-3 §2.11: the root element's background
// becomes the canvas's and covers the whole canvas, and when the root declares
// none it is taken from <body> instead. The consequence is the one authors rely
// on without knowing they do — "body { background: silver }" makes the *page*
// silver rather than a rectangle the height of the text — and it is the reason
// the element the background came from is not painted again here.
//
// The images are positioned against the root element's box even when the values
// came from <body>, which is what the specification says in as many words and is
// decided in layout; what is left here is the order.
func (p *painter) canvasBackground(root *Fragment) {
	if root == nil || root.canvas.Empty() {
		return
	}
	if b := root.canvasColor; b != nil {
		if c, ok := p.color(b, "background-color"); ok && c.A > 0 {
			p.emit(FillRect{Rect: root.canvas, Color: c})
		}
	}
	p.backgroundImages(root.canvasLayers, root.canvasColor)
}

// backgroundImages emits the operations of each resolved layer. who is the box
// the background is the background of, which a finding about a layer names.
//
// The layers arrive in painting order, so this is a loop and not a decision. All
// the arithmetic — the tile, the step, the clip — happened in layout, where a
// finding could be raised about it. A picture is one TileImage however many
// tiles it is; a solid or banded layer is rectangles, and tiling says how many.
func (p *painter) backgroundImages(layers []bgPaint, who *Box) {
	for _, l := range layers {
		if l.Clip.Empty() || l.Tile.Empty() {
			continue
		}
		if l.Solid != nil {
			p.tiling(l, []bgBand{{Rect: Rect{W: l.Tile.W, H: l.Tile.H}, Color: *l.Solid}}, who)
			continue
		}
		if len(l.Bands) > 0 {
			p.tiling(l, l.Bands, who)
			continue
		}
		if l.Image == nil {
			continue
		}
		p.emit(TileImage{
			Clip: l.Clip, Tile: l.Tile,
			StepX: l.StepX, StepY: l.StepY,
			Image: l.Image, Key: l.Key,
		})
	}
}

// emit appends operations a fragment paints for itself, charged to the
// document's work budget as its own content: a background, a border edge, a
// run of text. What a single declaration can multiply — a tiling, the dashes of
// a border — is charged before it is expanded, by the code that expands it, and
// appended directly.
func (p *painter) emit(ops ...Op) {
	if len(ops) == 0 {
		return
	}
	if !p.rec.chargeMark(int64(len(ops))*costOp, "the marks past that point") {
		return
	}
	p.ops = append(p.ops, ops...)
}

type painter struct {
	ops []Op
	// rec is the render's recorder, which carries the document's work budget
	// every operation is charged to. It is never nil. See emit.
	rec *Recorder
	// colors memoizes parsing a computed colour, which is asked for once per
	// box per property and is almost always one of a handful of values.
	colors map[string]style.RGBA

	// alpha is the fraction of a fragment's own marks that reaches the page,
	// and owners are the boxes that asked for it. Both are worked out before
	// anything is painted, because the paint walk sorts the tree into Appendix
	// E's layers and a fragment is painted from a flat list that no longer says
	// what it was inside. Only the fragments that are dimmed are in either map.
	alpha  map[*Fragment]float64
	owners map[*Fragment]*opacityOwner
	// groups is what each of those boxes painted, in the order the boxes were
	// met, so that the report about a group is in document order.
	groups map[*Box]*group
	order  []*Box
	// elementGroups is the box each element's group is kept under. See
	// groupFor.
	elementGroups map[groupKey]*Box
	// inlineDims memoizes inlineDim, per inline box.
	inlineDims map[*Box]memoDim
	// inlineLinks memoizes linkAbove, per inline box, and linkSteps counts the
	// boxes it has walked, which is what a test bounds.
	inlineLinks map[*Box]*hyperlink
	linkSteps   int

	// levels is the inline level of each element that has one, innerLevels
	// memoizes innerLevelOf per box, and lineLevels is, per block, the
	// outermost levels on its lines. See inlinestacking.go.
	levels      map[levelKey]*inlineLevel
	innerLevels map[*Box]*inlineLevel
	lineLevels  map[*Fragment][]*inlineLevel
	// orderPrefixes memoizes orderPrefix, per box.
	orderPrefixes map[*Box][]orderStep
	// joinRefused says the work budget refused the joining of an inline box's
	// outline pieces once, so every outline after it is drawn a ring per
	// piece rather than some joined and some not. See joinedOutline.
	joinRefused bool
}

// stackLevel is one positioned box waiting to be painted, with what decides
// where in the order it goes.
type stackLevel struct {
	// frag is the box, or level the inline box, whichever this entry is: a
	// non-atomic inline box has no fragment to be sorted by, and is sorted as
	// the level that holds what it paints. See inlinestacking.go.
	frag  *Fragment
	level *inlineLevel
	// z is the z-index, with auto counted as zero. §E.2 step 7 paints
	// "z-index: auto" and "z-index: 0" together in tree order, so for the
	// purpose of *ordering* the two really are the same number — they differ
	// only in whether the box becomes a context of its own, which is asked
	// separately.
	z int
	// key is the box's position in order-modified document order, which is
	// what breaks a tie between equal z-indexes. See orderKey.
	key []orderStep
}

// layers is what one stacking context's subtree contributes, split into
// Appendix E's steps.
type layers struct {
	// blocks are the in-flow, non-positioned, non-floating descendants whose
	// backgrounds and borders are §E.2 step 4.
	blocks []*Fragment
	// floats are the non-positioned floating descendants of §E.2 step 5, each painted
	// whole rather than as a background here and text later — §E.2 says a float
	// is painted as though it created a stacking context, so its own text goes
	// with its own background rather than joining the parent's text layer.
	floats []*Fragment
	// content are the fragments with inline content — line boxes and list
	// markers — which is §E.2 step 6, together with the atomic inlines that sit
	// on those lines.
	//
	// The two are one list rather than two because §E.2 paints an inline-block
	// "atomically, as if it created a new stacking context", *in the line box it
	// sits in* and so in tree order among the text around it. Two lists painted
	// one after the other would put every inline-block over every run of words,
	// or under every one, and neither is the order.
	content []contentItem
	// tables are the tables using §17.6.2's collapsing border model, whose grid
	// lines are drawn after every background in the table and not with the
	// table's own. §17.5.1 paints a table in six layers — the table, the column
	// groups, the columns, the row groups, the rows and the cells — and a border
	// centred on a grid line runs under the edge of a row and of two cells, so a
	// background painted after it would rub it out.
	tables []*Fragment
	// positioned are §E.2 steps 3, 7 and 8, which are one list sorted by z rather
	// than three: the steps differ only in the sign of the number.
	positioned []stackLevel
	// levels is the inline levels already in positioned, since a level is met
	// once for each block its marks are on and each fragment it holds.
	levels map[*inlineLevel]bool
}

// contentItem is one entry of the content layer: either a fragment whose lines
// and marker are painted, or an atomic inline painted whole.
type contentItem struct {
	frag *Fragment
	// atomic marks an inline-level box that §E.2 paints as a unit — an
	// inline-block, an inline-table, or an inline replaced element. Its
	// background travels with its text rather than joining the block backgrounds
	// of step 4, which is the difference the "z-ordering of inline-block" tests
	// are written about: a later sibling block's background is painted *under*
	// an inline-block that overlaps it, not over it.
	atomic bool
	// scope, when it is set, makes the entry the marks on frag's lines that
	// are that inline level's own, and nothing else of frag. See
	// inlinestacking.go. marks are those marks, which the pre-pass listed.
	scope *inlineLevel
	marks []lineMark
}

// atomicInline reports whether a fragment is an inline-level box that §E.2
// paints atomically.
//
// It is the box's *used* outer display, which for an inline-block is inline and
// for a floated or absolutely positioned box has already been blockified by
// §9.7 — so a float never reaches here and neither does an abspos box, and both
// are dealt with by the branches above the caller.
func atomicInline(f *Fragment) bool {
	return f.Box != nil && f.Box.Outer == OuterInline
}

// stackingContext paints a fragment and everything under it, in the order of
// Appendix E §E.2.
func (p *painter) stackingContext(f *Fragment) {
	if f.Box == nil {
		return
	}
	lv := &layers{}
	p.gather(f, lv, true, true)

	// Step 1: the context root's own background and border.
	p.decorations(f)

	sortLevels(lv.positioned)
	at := 0

	// Step 3: the stacking contexts with a negative z-index, most negative
	// first. They go behind the in-flow content of this context but in front of
	// its root's own background, which is the one thing a negative z-index
	// cannot get behind — and is why "z-index: -1" on a child does not hide it
	// under its own parent's background.
	for at < len(lv.positioned) && lv.positioned[at].z < 0 {
		p.stackLevel(lv.positioned[at])
		at++
	}

	// Steps 4, 5 and 6: block backgrounds, then floats, then inline content.
	for _, g := range lv.blocks {
		p.decorations(g)
	}
	for _, g := range lv.tables {
		p.paintCollapsed(g)
	}
	for _, g := range lv.floats {
		p.unit(g)
	}
	for _, g := range lv.content {
		p.contentItem(g)
	}

	// Steps 7 and 8: everything positioned, in z order. The two steps are one
	// loop because the sort has already put the zeroes before the positives and
	// there is nothing between them.
	for ; at < len(lv.positioned); at++ {
		p.stackLevel(lv.positioned[at])
	}

	// Step 10: the outlines of everything in this context.
	p.outlines(f)
}

// stackLevel paints one positioned box, as a context of its own or as a unit.
//
// The choice is §9.9's: a z-index that is not auto makes a stacking context, and
// everything inside it — including its positioned descendants, however extreme
// their z-index — is sealed within it. A z-index of auto does not, so the box is
// painted as a unit and its positioned descendants have already been hoisted
// into the enclosing context by gather.
func (p *painter) stackLevel(s stackLevel) {
	if s.level != nil {
		p.paintLevel(s.level)
		return
	}
	if !sealsItsDescendants(s.frag.Box) {
		p.unit(s.frag)
		return
	}
	p.stackingContext(s.frag)
}

// sealsItsDescendants reports whether a positioned box is a stacking context of
// its own, so that everything inside it is painted within it however extreme a
// z-index a descendant asks for.
//
// §9.9.1: a z-index that is not auto makes one. And CSS 2.2 added the second
// half, in the changes appendix the suite links from fixed-pos-stacking-001:
//
//	If the box has 'position: fixed' or if it is the root, it also establishes
//	a new stacking context.
//
// Without it a "z-index: -1" inside a fixed box was hoisted into the context
// around it and painted *under* the page's background — which is exactly what
// that test draws, in red, to be covered.
//
// The root is the other half of the sentence and needs nothing here: the paint
// begins by making a stacking context of it, so it never reaches this.
//
// "Not auto" is the *used* value, which is auto wherever z-index does not
// apply — see usedZIndex. A static block with "opacity: 0.5; z-index: -1" is
// sealed by its opacity and not by the number.
func sealsItsDescendants(b *Box) bool {
	_, auto := usedZIndex(b)
	return !auto || b.Position == PositionFixed || groupsItsPaint(b)
}

// # Who stacks where
//
// Every painter asks the next four questions of a box, and they are answered
// here once so that no step of the paint has a reading of its own. Each of
// the defects this replaced was one step asking a nearby question instead:
// the level read the z-index of boxes it does not apply to (audit C97), the
// gather sorted a flex item as a plain block because it only knew about
// positioning (C98), and the outline pass walked a different tree from the
// one the rest of the paint walks (C95, C96).

// zIndexApplies reports whether z-index means anything on a box.
//
// CSS 2.1 §9.9.1 gives it to positioned boxes, and css-flexbox §4.3 and
// css-grid §9.5 to flex and grid items, positioned or not:
//
//	Flex items paint exactly the same as inline blocks [CSS2], except that
//	order-modified document order is used in place of raw document order,
//	and z-index values other than auto create a stacking context even if
//	position is static.
//
// On every other box the declaration is ignored — the computed value is kept,
// but the used value is auto.
func zIndexApplies(b *Box) bool {
	return b.Position.positioned() || isFlexOrGridItem(b)
}

// isFlexOrGridItem reports whether a box is an item of a flex or grid
// container: an in-flow child of one. An absolutely positioned child is not an
// item (css-flexbox §4.1), and a float in one is not floating — unfloatItems
// cleared it when the box tree was built.
func isFlexOrGridItem(b *Box) bool {
	up := b.Parent
	return up != nil && (up.Inner == InnerFlex || up.Inner == InnerGrid) &&
		!b.Position.outOfFlow()
}

// usedZIndex is a box's z-index as painting uses it, and whether that is auto.
func usedZIndex(b *Box) (z int, auto bool) {
	if b.ZAuto || !zIndexApplies(b) {
		return 0, true
	}
	return b.ZIndex, false
}

// stacksAsLevel reports whether a box is painted among the stacking levels of
// §E.2 steps 3, 7 and 8 rather than in the layer its display would put it in:
// every positioned box and every stacking context.
//
// A stacking context that is not positioned is one of two things this engine
// implements: a box with an opacity below one, which CSS Color 4 paints
// at the stacking order a positioned element with "z-index: 0" would have, and
// a flex or grid item with a z-index. The first stacks at zero whatever its z-index says, because z-index
// does not apply to it; the second stacks at its number.
//
// It is asked of a box, and a non-atomic inline box is one too: what such a
// box paints is gathered into an inline level and sorted as one entry. A block
// §9.2.1.1 lifted out of one is not a level of its own on that account; it is
// part of the inline's level, which is how it comes to be painted where the
// inline is. See inlinestacking.go.
func stacksAsLevel(b *Box) bool {
	if b.Position.positioned() || groupsItsPaint(b) {
		return true
	}
	_, auto := usedZIndex(b)
	return !auto
}

// paintsAtomically reports whether a fragment that is not a stacking level is
// painted whole, in tree order among the text of the content layer, rather
// than split between the block backgrounds of step 4 and the text of step 6.
//
// An inline-level box is (§E.2), and so is a flex or grid item — the sentence
// quoted at zIndexApplies. Painting an item's background with the block
// backgrounds put a later item's background over an earlier one's text, and a
// later sibling block's background over the whole row.
func paintsAtomically(f *Fragment) bool {
	return atomicInline(f) || isFlexOrGridItem(f.Box)
}

// opensAContext reports whether a fragment is painted by a stackingContext
// call of its own, which is what everything inside it is sealed in — its
// outlines included.
func opensAContext(f *Fragment) bool {
	return stacksAsLevel(f.Box) && sealsItsDescendants(f.Box)
}

// unit paints a fragment and its non-positioned content as one indivisible
// group, in the same block-float-text layering a stacking context uses for its
// own content.
//
// It is what a float is painted by (§E.2 step 5) and what a positioned box with
// "z-index: auto" is painted by (step 7): both are atomic with respect to their
// surroundings and neither seals its positioned descendants in.
func (p *painter) unit(f *Fragment) {
	lv := &layers{}
	p.gather(f, lv, true, false)

	p.decorations(f)
	for _, g := range lv.blocks {
		p.decorations(g)
	}
	for _, g := range lv.tables {
		p.paintCollapsed(g)
	}
	for _, g := range lv.floats {
		p.unit(g)
	}
	for _, g := range lv.content {
		p.contentItem(g)
	}
}

// gather walks a subtree and sorts what it finds into Appendix E's layers.
//
// root says whether f itself is the thing being painted, whose own background is
// step 1 rather than step 4. collect says whether positioned descendants belong
// to this walk's layers: they do when it is collecting for a stacking context,
// and they do not when it is collecting for a unit, because a unit's positioned
// descendants were hoisted into the enclosing context before it was painted.
func (p *painter) gather(f *Fragment, lv *layers, root, collect bool) {
	if f.Box == nil {
		return
	}
	if !root {
		lv.blocks = append(lv.blocks, f)
	}
	if len(f.collapsed) > 0 {
		// Collected even when f is the root of this walk, because its grid lines
		// go after the backgrounds of everything inside it rather than with its
		// own.
		lv.tables = append(lv.tables, f)
	}
	if len(f.Lines) > 0 || f.Marker != nil || f.Box.Replaced != nil {
		lv.content = append(lv.content, contentItem{frag: f})
	}
	if collect {
		// The inline levels on f's lines: a positioned or translucent span is
		// painted at its level in this context, not with f's text.
		for _, l := range p.lineLevels[f] {
			p.addLevel(lv, l)
		}
	}
	for _, c := range f.Children {
		if c.Box == nil {
			continue
		}
		p.gatherChild(c, lv, collect)
	}
}

// gatherChild sorts one child of a gathered fragment into the layers.
func (p *painter) gatherChild(c *Fragment, lv *layers, collect bool) {
	if l := p.levelHolding(c); l != nil {
		// Written inside an inline level, or lifted out of one, so the level
		// paints it wherever the level is painted. What this context sorts is
		// the outermost level around it.
		if collect {
			p.addLevel(lv, l.top)
		}
		return
	}
	p.gatherOwn(c, lv, collect)
}

// gatherOwn sorts a fragment into the layers of the context or level that
// paints it.
func (p *painter) gatherOwn(c *Fragment, lv *layers, collect bool) {
	if stacksAsLevel(c.Box) {
		if !collect {
			// Already hoisted; painting it here as well would draw it twice.
			return
		}
		z, _ := usedZIndex(c.Box)
		lv.positioned = append(lv.positioned, stackLevel{
			frag: c, z: z, key: p.orderKey(c.Box),
		})
		if !sealsItsDescendants(c.Box) {
			// Not a stacking context, so the positioned boxes inside it
			// belong to this one. Without this hoist a "z-index: 5" inside a
			// plain "position: relative" wrapper would be trapped under
			// everything the wrapper is under, which is the bug that makes
			// authors write z-indexes in the thousands.
			p.hoist(c, lv)
		}
		return
	}
	if c.Box.Float != FloatNone {
		lv.floats = append(lv.floats, c)
		if collect {
			// A float is atomic for its own content and transparent for its
			// positioned descendants: §E.2 step 5 says so in as many words,
			// and it is what stops a float trapping a positioned box behind
			// the text of the paragraph beside it.
			p.hoist(c, lv)
		}
		return
	}
	if paintsAtomically(c) {
		// §E.2's step 4 is over the "non-inline-level" descendants, so an
		// inline-block's background and border are not there: they belong
		// with the line the box sits on, and the box is painted whole and in
		// tree order among the words. A flex or grid item is painted the
		// same way, by the flexbox and grid specifications' own sentence. It
		// is transparent for its positioned descendants for the same reason
		// a float is — they are hoisted into the enclosing context rather
		// than sealed inside a box that never became a stacking context.
		lv.content = append(lv.content, contentItem{frag: c, atomic: true})
		if collect {
			p.hoist(c, lv)
		}
		return
	}
	p.gather(c, lv, false, collect)
}

// contentItem paints one entry of the content layer.
func (p *painter) contentItem(it contentItem) {
	switch {
	case it.atomic:
		p.unit(it.frag)
	case it.scope != nil:
		p.levelMarks(it.frag, it.marks, it.scope)
	default:
		p.content(it.frag)
	}
}

// hoist finds the positioned boxes inside a subtree that is painted as a unit,
// so that they take their place in the enclosing stacking context instead: the
// fragments, and the inline levels on the lines of f and of everything under
// it.
func (p *painter) hoist(f *Fragment, lv *layers) {
	for _, l := range p.lineLevels[f] {
		p.addLevel(lv, l)
	}
	for _, c := range f.Children {
		if c.Box == nil {
			continue
		}
		// The same tests gather makes, and for the same reason: a box gather
		// will skip as "already hoisted" has to actually be hoisted here, or it
		// is painted nowhere at all.
		if l := p.levelHolding(c); l != nil {
			p.addLevel(lv, l.top)
			continue
		}
		if stacksAsLevel(c.Box) {
			z, _ := usedZIndex(c.Box)
			lv.positioned = append(lv.positioned, stackLevel{
				frag: c, z: z, key: p.orderKey(c.Box),
			})
			if !sealsItsDescendants(c.Box) {
				p.hoist(c, lv)
			}
			continue
		}
		p.hoist(c, lv)
	}
}

// sortLevels orders the positioned boxes by z-index and then by tree order.
//
// Ascending, so the most negative is painted first and the largest last, which
// is the whole of what a z-index means. The tie-break is tree order and not
// something arbitrary: two boxes at the same level are stacked back to front in
// the order they were written, which is the rule that makes overlapping cards in
// a list read correctly without any of them naming a number. It is the order
// the flex and grid containers above them lay their items out in, which is the
// order they were written in wherever none of them says otherwise: see
// orderKey.
func sortLevels(levels []stackLevel) {
	sort.SliceStable(levels, func(i, j int) bool {
		if levels[i].z != levels[j].z {
			return levels[i].z < levels[j].z
		}
		return compareOrder(levels[i].key, levels[j].key) < 0
	})
}

// clipping applies a clip to everything a painting step produced.
//
// It is a wrapper around the step rather than an argument threaded into it, for
// the reason inlineDecorations gives about its own flag: the decompositions
// underneath — a dashed border is a dozen fills, a 3-D one is two tones, a
// tiling is an area and a step — would each have to learn about clipping, and a
// second copy of any of them is what those functions exist to prevent. One
// place applies the clip, to whatever the shared code produced, and a painting
// step that forgot to clip is not expressible: the three steps that emit
// anything all go through here.
func (p *painter) clipping(c Clip, paint func()) {
	if c.blocks() {
		// Nothing can be painted through it, so nothing is built and then
		// thrown away. This is the case "clip: rect(0, 0, 0, 0)" and a
		// zero-sized "overflow: hidden" box are, and it is common enough in a
		// hostile document to be worth not doing the work for.
		return
	}
	at := len(p.ops)
	paint()
	if !c.Active {
		return
	}
	p.ops = clipOps(p.ops, at, c)
}

// clipOps narrows every operation from index at onwards, dropping the ones that
// no longer mark anything.
func clipOps(ops []Op, at int, c Clip) []Op {
	kept := ops[:at]
	for _, op := range ops[at:] {
		switch v := op.(type) {
		case FillRect:
			// Exact: a rectangle cut by a rectangle is a rectangle. No clip
			// travels with it, which is what keeps the overflow-page guardrail
			// and the reftest comparison from needing to know about clipping at
			// all.
			v.Rect = v.Rect.Intersect(c.Rect)
			if v.Rect.Empty() {
				continue
			}
			kept = append(kept, v)

		case TileImage:
			// The area a tiling may paint is already one of its fields, and
			// this is the same statement narrowed. The tile positions are not
			// touched, so a tiling cut in half still lines up with the one
			// beside it.
			v.Clip = v.Clip.Intersect(c.Rect)
			if v.Clip.Empty() {
				continue
			}
			kept = append(kept, v)

		case DrawImage:
			if c.Rect.Intersect(v.Rect).Empty() {
				continue
			}
			v.Clip = v.Clip.meet(c)
			if c.Rect.Contains(v.Rect) {
				// Wholly visible, so the clip is not worth carrying: it would
				// make an unclipped picture and a clipped one compare as
				// different marks when they put the same ink on the page.
				v.Clip = Clip{}
			}
			kept = append(kept, v)

		case Link:
			// Exact, as a fill's is: each area cut by the clip, and one cut
			// away entirely is gone. A new slice, since the areas may be
			// shared with an op nothing here is narrowing.
			rects := make([]Rect, 0, len(v.Rects))
			for _, r := range v.Rects {
				if r = r.Intersect(c.Rect); !r.Empty() {
					rects = append(rects, r)
				}
			}
			if len(rects) == 0 {
				continue
			}
			v.Rects = rects
			kept = append(kept, v)

		case DrawText:
			ink := textInk(v)
			if ink.Empty() {
				// Nothing measurable to place: a run with no face, which the
				// engine does not produce and a caller building a display list
				// by hand might. Keeping it is the safe direction — the other
				// one drops every such run through any clip at all, since an
				// empty rectangle meets nothing.
				kept = append(kept, v)
				continue
			}
			if c.hides(textInkReserved(v)) {
				// Every glyph is outside the clip. This is the case the whole
				// feature exists for and the only one that can be settled
				// exactly without cutting a letter in half.
				//
				// Asked of a *wider* rectangle than the clip question below,
				// because the two want to be wrong in opposite directions. This
				// one throws the run away, so being wrong here loses text off
				// the page and nothing downstream can put it back; it is asked
				// of every pixel the face could reach. The one below only
				// records that a clip cuts the run, so being wrong there costs
				// nothing on paper — but it does make this run a different mark
				// from the same run drawn whole, so it is asked of where the
				// letters actually sit.
				continue
			}
			if !c.admits(ink) {
				v.Clip = v.Clip.meet(c)
			}
			kept = append(kept, v)

		default:
			kept = append(kept, op)
		}
	}
	return kept
}

// textInk is where a run of text puts ink.
//
// The glyphs the run is actually made of, which the face reads out of its own
// tables — the glyph headers of a real font, Adobe's published boxes for the
// standard fourteen. It is the rectangle for every question about what a reader
// can *see*: whether a run is cut, whether it is buried under something opaque.
// Those must not be answered yes about a run nobody sees cut or hidden, and the
// face's own ascent and descent answer yes far too often. They describe the
// face — the room a line of it needs, tallest accent and deepest tail included
// — and almost no run uses all of it. A four-em ellipsis is three dots on the
// baseline; asked of Courier's descent it appears to hang ten times as far
// below the line as it does, and a box ending under the baseline appears to cut
// it.
//
// The face's numbers remain the fallback for a font that cannot say, which is a
// CFF-flavoured one: its glyph extents are in the charstrings and reading them
// means interpreting them. That fallback errs large, which for these questions
// is the direction that calls two documents different rather than the same.
//
// It is deliberately not the rectangle for the question of whether to keep a run
// at all; textInkReserved is, and says why.
func textInk(v DrawText) Rect {
	above, below := v.Size, v.Size.Mul(0.3)
	if v.Face != nil {
		if a, b, ok := v.Face.InkExtent(v.Text, v.Size.Px()); ok {
			above, _ = style.FromPx(a)
			below, _ = style.FromPx(b)
		} else if upem := float64(v.Face.UnitsPerEm()); upem > 0 {
			d := v.Face.Descriptor()
			above = v.Size.Mul(float64(d.Ascent) / upem)
			below = v.Size.Mul(-float64(d.Descent) / upem)
		}
	}
	return textInkAt(v, above, below)
}

// textInkReserved is every pixel the run could reach: the box inline layout set
// aside for it.
//
// The same extents lineMetrics gives the line box — the face's line gap when it
// declares one, and the box enclosing all its glyphs when it does not — so a run
// is bounded here by exactly the rectangle layout reserved, which is the promise
// textInk used to make and had stopped keeping once layout moved to the glyph
// box for gap-less faces.
//
// It is wider than textInk for such a face, since the glyph box has to hold an
// accented capital and a bracket that ordinary text never reaches. That width is
// the point: this answers the one question whose wrong answer is unrecoverable,
// which is whether to drop the run from the page.
func textInkReserved(v DrawText) Rect {
	above, below := v.Size, v.Size.Mul(0.3)
	if v.Face != nil {
		if top, bottom, upem, ok := lineMetrics(v.Face); ok {
			above = v.Size.Mul(top / upem)
			below = v.Size.Mul(-bottom / upem)
		}
	}
	return textInkAt(v, above, below)
}

// textInkAt is the rectangle both of them return, given how far the run's ink
// reaches above and below its baseline.
//
// It is built in the run's own axes and then placed, which is what makes a
// sideways run's ink a rectangle standing on the page rather than lying across
// it: "above the baseline" is towards the right once the page has been turned
// clockwise, and the run's advance goes down it. See placeRun.
func textInkAt(v DrawText, above, below style.Unit) Rect {
	var width style.Unit
	if v.Face != nil {
		w, _ := style.FromPx(v.Face.Measure(v.Text, v.Size.Px()))
		// The characters letter-spacing goes after, and not every rune: a run
		// of zero-width formatting characters is not a run of typographic
		// character units, and counting them makes a word's ink reach a
		// tracking-width past the page for each one. See paragraph.SpacedUnits.
		width = w.Add(v.CharSpacing.Mul(float64(spacedUnits(v.Text))))
	}
	if v.Upright {
		// An upright run is not the face's advances at all: it is one em per
		// character along the line, and one em across it centred on the
		// baseline. Both are the metrics CSS Writing Modes §4.4 has the UA
		// synthesize where a face states none, so this is the run's real extent
		// and not an estimate of it. See DrawText.Upright.
		width = v.Size.Mul(float64(uprightUnits(v.Text))).
			Add(v.CharSpacing.Mul(float64(spacedUnits(v.Text))))
		above, below = v.Size.Div(2), v.Size.Div(2)
	}
	return placeRun(Rect{
		Y: style.Unit(0).Sub(above),
		W: width, H: above.Add(below),
	}, v.At, turnOfRun(v))
}

// decorations paints a box's own background and border, which is what §E.2 steps
// 1 and 4 both consist of.
// The clip is the box's own — §11.1.1 clips a box's *contents* and not the box,
// so an "overflow: hidden" element with a wide border still draws all of it.
// What can cut a box's own background is §11.1.2's "clip", which is in clipSelf
// and not in clipContent.
func (p *painter) decorations(f *Fragment) {
	if f.Box == nil {
		return
	}
	p.grouped(f, func() { p.decorationsIn(f) })
	// The fragment's area as a link's, where it is one: every fragment that
	// is not a line's comes through here exactly once, which is what makes
	// this the place. The clip is the one its background has.
	p.linkArea(f, f.clipSelf, p.linkOf(f.Box))
}

func (p *painter) decorationsIn(f *Fragment) {
	if len(f.bgBands) > 0 {
		// The background is shown through the bands and nothing else — see
		// Fragment.bgBands — while the border, if the box has one at all, is one
		// border round one box and is drawn once.
		//
		// The two guards paintDecorations opens with are asked here as well, and
		// they have to be: a row group with "visibility: hidden" is laid out and
		// not drawn like any other box, and the banded path would otherwise be
		// the one place in the painter where that is not true.
		if isHidden(f.Box) {
			return
		}
		if !f.bgSuppressed {
			for _, band := range f.bgBands {
				p.clipping(f.clipSelf.with(band), func() { p.paintBackground(f) })
			}
		}
		p.clipping(f.clipSelf, func() { p.borders(f) })
		return
	}
	p.clipping(f.clipSelf, func() { p.paintDecorations(f) })
}

func (p *painter) paintDecorations(f *Fragment) {
	if isHidden(f.Box) {
		// §11.2: the box is laid out and not drawn. It has already taken its
		// space — every position on this page was computed with it in — so this
		// is the only place the property has any effect, and it is asked per box
		// rather than per subtree because a descendant may set "visibility:
		// visible" and reappear.
		return
	}
	if f.bgSuppressed {
		// This box's background became the canvas's, and was painted over the
		// whole page before anything else. Painting it again over its own box
		// would double a translucent colour and would put a "no-repeat" image
		// down twice in two places.
		p.borders(f)
		return
	}
	p.paintBackground(f)
	p.borders(f)
}

// paintBackground is §E.2 steps 1 and 4 for one box: "the background color of
// the element, then the background image".
//
// The background paints over the *border* box by default, under the border
// rather than up to it — which is what background-clip's initial value of
// border-box means, and is why a dashed border shows the background through its
// gaps rather than the page. It stops at the border box and never reaches the
// margin, which is the space that is meant to show through.
func (p *painter) paintBackground(f *Fragment) {
	if bg, ok := p.color(f.Box, "background-color"); ok && bg.A > 0 {
		if rect := f.bgColorRect; !rect.Empty() {
			p.emit(FillRect{Rect: rect, Color: bg})
		}
	}
	p.backgroundImages(f.background, f.Box)
}

// content paints the inline-level marks a fragment carries: its list marker and
// its line boxes, which are §E.2 step 6.
//
// The marker is here rather than with the decorations because it is text. A
// marker painted with the backgrounds would be covered by the background of any
// box painted after it in step 4, which for an "outside" marker — one that sits
// in the margin, outside its own list item — is a real overlap rather than a
// theoretical one.
// The clip is the box's *content* clip, so its own "overflow" cuts the text and
// pictures inside it to its padding box. That is §11.1.1 in one line, and it is
// the half of clipping every author uses.
//
// The lines are painted outside the fragment's own grouped call rather than
// inside it, because what is on them is not all dimmed by what dims the block:
// a run inside a translucent <span> is dimmed by the span as well, and each run
// and inline fragment folds its own dimming in lines. See painter.as.
func (p *painter) content(f *Fragment) {
	if f.clipContent.blocks() {
		return
	}
	p.grouped(f, func() { p.clipping(f.clipContent, func() { p.paintContent(f) }) })
	p.lines(f)
}

func (p *painter) paintContent(f *Fragment) {
	// A replaced element's content is painted here rather than with the
	// backgrounds, and for the same reason the marker is: it is content. §E.2
	// paints every block background in a stacking context before any of its
	// content, so an image drawn with the backgrounds would be covered by the
	// background of any box painted after it — which for an image overlapping
	// its next sibling is a real overlap rather than a theoretical one.
	hidden := isHidden(f.Box)
	if r := f.Box.Replaced; r.Paints() && !hidden {
		if box := f.ContentRect(); !box.Empty() {
			// Where the content goes inside that box, which is object-fit's
			// question and not the box's. The clip comes back active only when
			// the content reaches outside — "cover" always does, and "none"
			// does when the picture is larger than the box it was put in.
			fit, _ := objectFitOf(f.Box.Style.Get("object-fit"))
			rect, clip := fitContent(box, naturalSizeOf(r), fit, objectPositionOf(f.Box))
			p.clipping(clip, func() {
				// Content that is one colour is a fill, not a picture stretched
				// over the box. The two paint the same pixels and only one of
				// them says on the page what the document said in its source —
				// see the note on ReplacedContent.Solid.
				if r.SVG != nil {
					// A picture with geometry in it: each rectangle placed
					// through the viewport transform and clipped to the box.
					// See svg.go.
					p.emit(r.SVG.paint(rect)...)
				} else if r.Solid != nil {
					p.emit(FillRect{Rect: rect, Color: *r.Solid})
				} else {
					p.emit(DrawImage{Rect: rect, Image: r.Image, Key: r.Key})
				}
			})
		}
	}
	if m := f.Marker; m != nil && m.Image != nil && m.Image.Image != nil && !hidden {
		// §12.6.2: the image *replaces* the marker the type would have made, so
		// the text below is not drawn as well. It is still on the Marker, which
		// is what a caller extracting the page's text reads.
		rect := Rect{
			X: f.BorderRect.X.Add(m.ImageRect.X), Y: f.BorderRect.Y.Add(m.ImageRect.Y),
			W: m.ImageRect.W, H: m.ImageRect.H,
		}
		if !rect.Empty() {
			p.emit(DrawImage{
				Rect: rect, Image: m.Image.Image, Key: m.Image.Key,
			})
		}
	} else if m := f.Marker; m != nil && m.Face != nil && !hidden {
		at := Point{X: f.BorderRect.X.Add(m.At.X), Y: f.BorderRect.Y.Add(m.At.Y)}
		if len(m.pieces) == 0 {
			p.emit(DrawText{At: at, Text: m.Text, Face: m.Face, Size: m.Size, Color: m.Color})
			return
		}
		// A marker whose text does not run one way, drawn a stretch at a time
		// in the order markerPieces put them in.
		ops := make([]Op, 0, len(m.pieces))
		for _, pc := range m.pieces {
			ops = append(ops, DrawText{
				At:   Point{X: at.X.Add(pc.x), Y: at.Y},
				Text: pc.text, RTL: pc.rtl,
				Face: m.Face, Size: m.Size, Color: m.Color,
			})
		}
		p.emit(ops...)
	}
}

// inlineDecorations paints one fragment of an inline box: the same background
// and border every other box gets, marked as an overhang.
//
// The marking is done over the operations rather than passed into the painting,
// because the decomposition it would have to be threaded through is border.go's
// — a dashed border is a dozen fills and a 3-D one is two tones — and a second
// copy of that decomposition is exactly what border.go exists to prevent. What
// is here is one flag applied to whatever the shared code produced.
//
// clip is the content clip of the block whose line the fragment is on: an
// inline box clips nothing of its own, and what cuts it is what cuts the words
// beside it. See resolveClips.
func (p *painter) inlineDecorations(f *Fragment, clip Clip) {
	if f.Box == nil {
		return
	}
	at := len(p.ops)
	p.grouped(f, func() { p.clipping(clip, func() { p.decorationsIn(f) }) })
	for i := at; i < len(p.ops); i++ {
		r, ok := p.ops[i].(FillRect)
		if !ok {
			continue
		}
		r.Overhang = true
		p.ops[i] = r
	}
}

// borders paints the four edges as filled bands.
//
// Four rectangles rather than a stroked outline, because a stroke is centred on
// its path and a CSS border is not: it lies entirely inside the border box. A
// stroked border would be half a width out on every side, which is invisible at
// one pixel and obvious at ten.
//
// The corners are mitred by giving the top and bottom bands the full width and
// the sides only what is left, which is right for a solid border of one colour
// and is where a proper implementation of border-style would start.
func (p *painter) borders(f *Fragment) {
	if f.inCollapsedGrid {
		// §17.6.2: this box's declared border is one of the candidates the grid
		// lines were resolved from, and it was either drawn as part of one of
		// them or beaten by another box's. Drawing it here as well would put a
		// losing candidate on the page after the winner — and would draw it at
		// its full width over a line that is meant to be shared, which is the
		// separated model showing through.
		return
	}
	r := f.BorderRect
	e := f.Border

	// The sides take what the top and bottom leave, which mitres the corners
	// well enough for a border of one colour and is where a per-corner
	// implementation would start.
	inner := Rect{
		Y: r.Y.Add(e.Top),
		H: r.H.Sub(e.Top).Sub(e.Bottom),
	}
	if inner.H < 0 {
		inner.H = 0
	}

	edges := [4]struct {
		width style.Unit
		band  Rect
		name  string
		side  side
	}{
		{e.Top, Rect{r.X, r.Y, r.W, e.Top}, "top", sideTop},
		{e.Right, Rect{r.Right().Sub(e.Right), inner.Y, e.Right, inner.H}, "right", sideRight},
		{e.Bottom, Rect{r.X, r.Bottom().Sub(e.Bottom), r.W, e.Bottom}, "bottom", sideBottom},
		{e.Left, Rect{r.X, inner.Y, e.Left, inner.H}, "left", sideLeft},
	}
	for _, edge := range edges {
		if edge.width <= 0 {
			continue
		}
		colour, ok := p.color(f.Box, "border-"+edge.name+"-color")
		if !ok || colour.A == 0 {
			continue
		}
		kind := parseBorderStyle(f.Box.Style.Get("border-" + edge.name + "-style"))
		p.paintEdge(edge.band, kind, colour, edge.side, edge.width)
	}
}

// outlines paints CSS 2.1 §18.4's outlines: §E.2's step 10 for one stacking
// context.
//
// It is a step of its own because an outline is drawn *outside* its box, so it
// lies over whatever is beside the box, and painting it with the box's own
// border would put a later sibling's background on top of it. It is the last
// step of *each* context and not one pass over the whole page, which is what
// the step says —
//
//	Finally, implementations that do not draw outlines in steps above must
//	draw outlines from this stacking context at this stage.
//
// — so an outline inside a "z-index: -1" box is under the content of the
// context around that box, exactly as the box is.
//
// The walk is the fragment tree in document order, stopping at every
// descendant that opens a context of its own, whose outlines are its own
// step 10. It is the same test the gather makes; a separate reading of which
// boxes seal was how the outline pass came to walk a different tree from the
// paint (audit C95, C96). An inline level that seals is such a context too,
// and so are the pieces of its inline boxes and what it holds: see
// levelOutlines.
//
// Each ring is clipped by what clips its box. For a fragment that is clipSelf,
// which is every clip its containing block chain passes to it with its own
// "clip" met in: §11.1.1 clips a box's contents, and an outline is part of the
// rendering of the box it rings, so an outline inside an "overflow: hidden"
// box stops at that box's padding edge like everything else in it. For an
// inline box's fragment it is the content clip of the block whose line it is
// on, which is what cuts the box's background and its words.
func (p *painter) outlines(root *Fragment) {
	if root != nil {
		p.outlineWalk(root)
	}
}

// outlineWalk outlines a fragment that is wholly inside the context being
// outlined, and everything under it that is in the same context.
func (p *painter) outlineWalk(f *Fragment) {
	if f.Box == nil {
		return
	}
	p.grouped(f, func() { p.clipping(f.clipSelf, func() { p.outline(f) }) })
	p.lineOutlines(f, nil)
	for _, c := range f.Children {
		if c == nil || c.Box == nil || opensAContext(c) {
			continue
		}
		if l := p.levelHolding(c); l != nil && (l.seals() || l.up != nil) {
			// Held by an inline level whose context is not this one.
			continue
		}
		p.outlineWalk(c)
	}
}

// lineOutlines outlines the inline boxes on a block's lines whose outline
// context is want: nil for the block's own context, or the sealing level being
// outlined.
//
// The pieces of one inline box are outlined together, as one shape, where the
// order they are met in is the order of the box's first piece. See
// joinedOutline. Every piece of one box on one block's lines has the same
// clip, which is the block's, and the same dimming, which is inlineDim's for
// that box, so one grouped call covers them.
func (p *painter) lineOutlines(f *Fragment, want *inlineLevel) {
	var boxes []*Fragment
	for _, line := range f.Lines {
		boxes = append(boxes, line.Boxes...)
	}
	p.outlinePieces(f, boxes, want)
}

// outlinePieces outlines the inline box fragments of boxes, which are on f's
// lines, whose outline context is want: each box's pieces as one, in the order
// of each box's first piece.
func (p *painter) outlinePieces(f *Fragment, boxes []*Fragment, want *inlineLevel) {
	var order []*Box
	var pieces map[*Box][]*Fragment
	for _, box := range boxes {
		if box == nil || box.Box == nil || box.Outline <= 0 ||
			p.outlineContext(box.Box) != want {
			continue
		}
		if pieces == nil {
			pieces = map[*Box][]*Fragment{}
		}
		if _, seen := pieces[box.Box]; !seen {
			order = append(order, box.Box)
		}
		pieces[box.Box] = append(pieces[box.Box], box)
	}
	for _, b := range order {
		ps := pieces[b]
		p.grouped(ps[0], func() { p.clipping(f.clipContent, func() { p.joinedOutline(ps) }) })
	}
}

// outline paints one box's ring.
func (p *painter) outline(f *Fragment) { p.joinedOutline([]*Fragment{f}) }

// joinedOutline paints the outline of one box, given its fragments: one ring
// for a box that is one rectangle, and one shape for an inline box broken
// across lines.
//
// A ring is four bands, like the border and for the same reason — a stroked
// path is centred on itself and a CSS outline is not — but the arithmetic is
// the simpler one: an outline has a single width, so the two horizontal bands
// run the full width of the ring and the vertical ones fill what is between
// them.
//
// # One outline for a box broken across lines
//
// CSS UI 4 §5:
//
//	Outlines may be non-rectangular. For example, if the element is broken
//	across several lines, the outline should be an outline or minimum set of
//	outlines that encloses all the element's boxes. Each part of the outline
//	should be fully connected rather than open on some sides.
//
// A ring round each piece encloses them all, and where two pieces' rings do
// not meet it is the minimum. Where they do — the pieces on two lines set
// close enough that the outlines overlap, which at "line-height: normal" is
// every line — the rings cross: each piece's ring runs through the inside of
// the other. The shape that encloses the two is the union of the rings'
// outer rectangles less the union of the pieces themselves, and that is what
// is drawn: piece j's bands, less every earlier piece's outer rectangle (the
// earlier piece painted that part already, or it is inside the earlier
// piece) and every later piece's border box (it is inside that piece). Each
// point of the union is painted by exactly one piece, the first whose outer
// rectangle holds it, so a translucent outline is not darker where two meet.
// Pieces whose rings do not meet are cut by nothing, and draw exactly the
// rings they drew before.
//
// The cut is exact for a solid outline, which is a set of rectangles. For the
// other styles each remaining part of a band is drawn as a band of its own,
// with its own dashes, its own thirds of a double line and its own 3-D tones,
// which is the nearest the display list's rectangles come to a styled
// polygon: the pattern restarts where a band was cut.
//
// Every band is Overhang. The outline is by definition outside the box, so no
// layout decision accounted for its position, and the overflow-page guardrail
// must not read a two-pixel ring as a box leaving the paper.
func (p *painter) joinedOutline(pieces []*Fragment) {
	first := pieces[0]
	w := first.Outline
	if w <= 0 || first.Box == nil || isHidden(first.Box) {
		return
	}
	colour, ok := p.color(first.Box, "outline-color")
	if !ok || colour.A == 0 {
		// "invert", or a colour that did not parse. The finding was raised in
		// layout, where there was a recorder to raise it with.
		return
	}
	kind := parseBorderStyle(first.Box.Style.Get("outline-style"))

	// paintEdge is the border's, and a border's fills are not Overhang because
	// layout accounted for every one of them. These are marked afterwards rather
	// than by threading a flag through paintEdge, paintDashes and paint3D — the
	// flag would be a property of the caller pretending to be a property of the
	// edge, and every border call site would have to pass false.
	at := len(p.ops)
	defer func() {
		for i := at; i < len(p.ops); i++ {
			if r, ok := p.ops[i].(FillRect); ok {
				r.Overhang = true
				p.ops[i] = r
			}
		}
	}()

	n := len(pieces)
	if n == 1 {
		r := first.BorderRect
		o := Rect{X: r.X.Sub(w), Y: r.Y.Sub(w), W: r.W.Add(w).Add(w), H: r.H.Add(w).Add(w)}
		for _, band := range ringBands(o, r, w) {
			p.paintEdge(band.band, kind, colour, band.side, w)
		}
		return
	}
	inner := make([]Rect, n)
	outer := make([]Rect, n)
	var tallest style.Unit
	for i, f := range pieces {
		r := f.BorderRect
		inner[i] = r
		outer[i] = Rect{X: r.X.Sub(w), Y: r.Y.Sub(w), W: r.W.Add(w).Add(w), H: r.H.Add(w).Add(w)}
		tallest = style.Max(tallest, outer[i].H)
	}
	// The pieces by the top of their outer rectangle, so that the ones that
	// can meet piece j — whose tops lie within the tallest ring above j's
	// bottom — are a window of this and not the whole list. Pieces on lines
	// are met top to bottom, so this is almost always already sorted.
	byTop := make([]int, n)
	for i := range byTop {
		byTop[i] = i
	}
	sort.SliceStable(byTop, func(a, b int) bool { return outer[byTop[a]].Y < outer[byTop[b]].Y })

	for j := range pieces {
		o, r := outer[j], inner[j]
		from := sort.Search(n, func(k int) bool { return outer[byTop[k]].Y > o.Y.Sub(tallest) })
		for _, band := range ringBands(o, r, w) {
			parts := []Rect{band.band}
			for k := from; k < n && outer[byTop[k]].Y < o.Bottom() && !p.joinRefused; k++ {
				i := byTop[k]
				if i == j {
					continue
				}
				cut := inner[i]
				if i < j {
					cut = outer[i]
				}
				// What a join costs is a comparison per piece near this one
				// per part of the band left, which a document controls: a box
				// broken across a thousand lines set on top of one another is
				// a thousand pieces each meeting every other. It is charged,
				// and past what the budget pays for the rest of the page's
				// outlines are drawn a ring per piece, as they were before
				// they were joined. A comparison weighs what one mark read
				// by an opacity group's overlap check weighs, which is the
				// same work: a rectangle held against a rectangle.
				if !p.rec.charge(int64(len(parts)+1)*costMarkCompared,
					"the joining of outlines broken across lines past that point, drawn a ring per piece") {
					p.joinRefused = true
					parts = []Rect{band.band}
					break
				}
				parts = cutRects(parts, cut)
			}
			for _, part := range parts {
				p.paintEdge(part, kind, colour, band.side, w)
			}
		}
	}
}

// ringBand is one of the four bands of an outline ring.
type ringBand struct {
	band Rect
	side side
}

// ringBands is the ring of width w between a box's border edge r and the
// outer rectangle o: the top and bottom bands the full width of o, and the
// sides between them.
func ringBands(o, r Rect, w style.Unit) [4]ringBand {
	return [4]ringBand{
		{Rect{o.X, o.Y, o.W, w}, sideTop},
		{Rect{r.Right(), r.Y, w, r.H}, sideRight},
		{Rect{o.X, r.Bottom(), o.W, w}, sideBottom},
		{Rect{o.X, r.Y, w, r.H}, sideLeft},
	}
}

// cutRects is rects with c taken out of each: what is above and below c the
// width of the rectangle, and what is left and right of it between those.
func cutRects(rects []Rect, c Rect) []Rect {
	out := rects[:0:0]
	for _, r := range rects {
		if r.Intersect(c).Empty() {
			out = append(out, r)
			continue
		}
		top, bottom := style.Max(r.Y, c.Y), style.Min(r.Bottom(), c.Bottom())
		for _, piece := range [4]Rect{
			{r.X, r.Y, r.W, top.Sub(r.Y)},
			{r.X, bottom, r.W, r.Bottom().Sub(bottom)},
			{r.X, top, c.X.Sub(r.X), bottom.Sub(top)},
			{c.Right(), top, r.Right().Sub(c.Right()), bottom.Sub(top)},
		} {
			if !piece.Empty() {
				out = append(out, piece)
			}
		}
	}
	return out
}

// lines paints the text of a block container: the marks on its lines that are
// the block's own. A mark inside a positioned or translucent inline box is
// painted by that box's level, where the stacking context around it sorts the
// level, and not here — see levelMarks and inlinestacking.go.
func (p *painter) lines(f *Fragment) {
	if len(f.Lines) == 0 {
		return
	}
	content, around := f.ContentRect(), p.dimOf(f)
	for li := range f.Lines {
		line := &f.Lines[li]
		// The links on the line first, at the place of the text they are
		// around. They are the block's own whatever inline level the <a> is
		// in, because they draw nothing and so have no place in a stacking
		// order to be sorted into — what they need is the clip, and the
		// block's content clip is the one every mark on its lines has.
		for _, lf := range line.links {
			p.linkArea(lf, f.clipContent, lf.Box.link)
		}
		// §E.2's inline layer, in the order it gives: for each line box, the
		// background and border of the inline boxes on it, then the text. They
		// are in tree order among themselves, so an inner box's background is
		// painted over the box it is inside.
		//
		// One deviation, which is the whole of what is not exact here: the
		// specification interleaves each inline box's own text with its
		// decoration, so text belonging to a box painted *earlier* in tree order
		// goes under a later box's background. Two inline boxes on a line only
		// overlap where a negative margin makes them, and painting all the
		// decorations of a line first is what keeps this a loop rather than a
		// second traversal of the tree.
		for _, box := range line.Boxes {
			if box == nil || p.innerLevelOf(box.Box) != nil {
				continue
			}
			p.inlineDecorations(box, f.clipContent)
		}
		for ri := range line.Runs {
			if p.innerLevelOf(line.Runs[ri].Box) != nil {
				continue
			}
			p.lineRun(f, content, around, line, ri)
		}
	}
}

// levelMarks paints the marks an inline level has on one block's lines,
// which the pre-pass listed in the order lines meets them: line by line, the
// inline boxes' decorations and then the runs. The level's own box is left
// out, because it is the root of the level's stacking context and was painted
// as its step 1.
//
// The marks are listed rather than found by a walk of the block's lines,
// because a paragraph of a thousand relatively positioned words is a thousand
// levels on one block, and a walk per level would be a million steps.
func (p *painter) levelMarks(f *Fragment, marks []lineMark, l *inlineLevel) {
	content, around := f.ContentRect(), p.dimOf(f)
	for _, m := range marks {
		line := &f.Lines[m.line]
		if m.box >= 0 {
			if box := line.Boxes[m.box]; !l.owns(box.Box) {
				p.inlineDecorations(box, f.clipContent)
			}
			continue
		}
		p.lineRun(f, content, around, line, m.run)
	}
}

// lineRun paints one run of a block's line. content is the block's content
// box and around its own dimming, which every run on its lines starts from.
func (p *painter) lineRun(f *Fragment, content Rect, around dim, line *LineFragment, ri int) {
	run := line.Runs[ri]
	// Where the baseline is, in whichever direction the line stacks its text
	// across. On a horizontal line it is a distance down from the top of the
	// line box; on a sideways one the line box has been turned with the rest
	// of the block, and its block-start edge — the edge the baseline is
	// measured from, and the edge half-leading is split above — is its right
	// one.
	baseline := content.Y.Add(line.Rect.Y).Add(line.Baseline)
	switch {
	case line.Anticlockwise:
		// The other turn puts the glyphs' up to the left, so the ascent is on
		// the left of the line box and the baseline is measured rightwards
		// from its near edge rather than back from its far one.
		baseline = content.X.Add(line.Rect.X).Add(line.Baseline)
	case line.Sideways:
		baseline = content.X.Add(line.Rect.X).Add(line.Rect.W).Sub(line.Baseline)
	}
	// Spaces are drawn, not skipped, and the reason is text extraction rather
	// than ink. A space glyph marks no paper, so skipping it looks like a free
	// optimisation — but then the words either side are separate text
	// operations with only a position jump between them, and a reader copying
	// the text gets them run together. That was found by reading back a
	// rendered page: "A heading" came out as "Aheading".
	//
	// A preserved tab is the one character that cannot be drawn as itself. No
	// face has a glyph for U+0009, so setting it emits .notdef — a box where
	// white space should be, which is the tofu this engine has a whole
	// guardrail about. Its advance is already spent: line breaking resolved it
	// against the tab stops and gave the next run its position, so what is
	// left to draw is white space, and a space is the character that draws it.
	if isHidden(run.Box) {
		// A run belongs to the inline box it came from, which may be visible
		// inside a hidden block or hidden inside a visible one. Asking per run
		// rather than per fragment is what makes "visibility: visible" on a
		// <span> inside a hidden paragraph show that span and nothing else.
		return
	}
	colour, ok := p.color(run.Box, "color")
	if !ok {
		colour = style.RGBA{A: 1}
	}
	// Where the run starts, from two offsets: how far along the line it is,
	// and how far off the line's baseline it sits.
	//
	// The second is the run's own baseline: the line's, displaced by §10.8.1's
	// vertical-align, and then by §9.4.3's relative positioning. The two are
	// added rather than chosen between, and in that order — a raised <sup>
	// that is also relatively positioned moves twice.
	along := run.X.Add(run.Offset.X)
	across := run.Shift.Add(run.Offset.Y)
	at := Point{
		X: content.X.Add(line.Rect.X).Add(along),
		Y: baseline.Add(across),
	}
	switch {
	case line.Anticlockwise:
		// The same quarter turn the other way: along the line is *up* the
		// page, so the offset is measured back from the line box's foot, and
		// off the baseline is towards the right, because that is where "down"
		// points once a page has been turned anticlockwise. See
		// layout/writingmode.go.
		at = Point{
			X: baseline.Add(across),
			Y: content.Y.Add(line.Rect.Y).Add(line.Rect.H).Sub(along),
		}
	case line.Sideways:
		// The same two offsets, a quarter turn round: along the line is down
		// the page, and off the baseline is back towards the left, because
		// that is the way "up" points once a page has been turned clockwise.
		// See layout/writingmode.go.
		at = Point{
			X: baseline.Sub(across),
			Y: content.Y.Add(line.Rect.Y).Add(along),
		}
	}
	// Each run is its own call, for the reason painter.as gives: what dims it
	// is the block's opacity and every translucent inline box it is inside,
	// and the next run on the line may be inside none.
	p.as(p.inlineDim(run.Box, around), func() {
		p.clipping(f.clipContent, func() { p.paintRun(run, at, colour, turnOfLine(*line)) })
	})
}

// paintRun paints one run of text at its pen position, with the lines ruled
// across it.
func (p *painter) paintRun(run TextRun, at Point, colour style.RGBA, turn runTurn) {
	if _, isControl := controlOf(run.Text); isControl {
		// CSS Text 3 requires a control character to be visible, and no
		// face has a glyph for one — so the mark is synthesized here
		// rather than asked for. The advance was spent by layout and is
		// not changed: the box goes inside it.
		//
		// No DrawText goes with it. Emitting one would put .notdef on
		// the page beside the box, and would put the control character
		// itself into the text extracted from the page, where it is
		// exactly the thing a reader does not want back.
		p.emit(controlBox(at, run.Width, run.Size, colour, turn)...)
		return
	}
	// The two lines that sit clear of the letters are drawn first, so the
	// text is over them where they touch; the line-through is drawn after,
	// because it goes across the letters rather than under them. That is
	// the order every renderer uses, and it only matters where a
	// decoration's colour differs from the text's — which is precisely the
	// case §16.3.1 exists to describe.
	p.decorate(run, at, turn, false)
	p.emit(DrawText{
		At:            at,
		Sideways:      turn.sideways,
		Anticlockwise: turn.anticlockwise,
		Upright:       run.Upright,
		Features:      run.Features,
		Text:          drawableText(run.Text),
		PreContext:    run.PreContext,
		PostContext:   run.PostContext,
		MergePre:      run.MergePre,
		MergePost:     run.MergePost,
		ContextKerns:  run.ContextKerns,
		RTL:           run.RTL,
		Face:          run.Face,
		Size:          run.Size,
		Color:         colour,
		CharSpacing:   run.LetterSpacing,
	})
	p.decorate(run, at, turn, true)
}

// decorate paints the lines ruled across one run.
//
// over selects the pass: the line-through, which goes on top of the letters, or
// the underline and overline, which go under them.
//
// The colour is the *declaring* box's rather than the run's, which is the whole
// subtlety of §16.3.1 and is why a decoration carries the box that produced it.
// A decoration with no colour of its own resolves "currentcolor" against that
// box too, so "p { text-decoration: underline; color: black } em { color: red }"
// rules a black line under red words.
//
// So is the *height*, and for the same reason. §16.3.1 draws a decoration across
// the whole of the box that declared it "without paying any attention to" the
// descendants it crosses, so the band is placed at the declaring box's baseline
// rather than at the run's: three spans at three vertical-aligns under one
// overlining div are ruled by one straight line. at.Y is the run's own baseline
// and carries the run's own shift, which is undone here and the declaring box's
// put in its place.
func (p *painter) decorate(run TextRun, at Point, turn runTurn, over bool) {
	if len(run.Decorations) == 0 || run.Width <= 0 {
		return
	}
	for _, d := range run.Decorations {
		if (d.Kind == decorationLineThrough) != over {
			continue
		}
		colour, ok := p.color(heldBox(d.By), "text-decoration-color")
		if !ok || colour.A == 0 {
			continue
		}
		// The band in the run's own axes, from an origin at the start of its
		// baseline: along the line for its length, off the baseline for where
		// the rule sits. Both are the same numbers a horizontal run has always
		// used — a decoration is measured from the baseline it crosses, which
		// is a fact about the text and not about the page — so placeRun is all
		// that separates one that runs across the page from one that runs down.
		//
		// How thick it is and how far off the baseline it sits are the
		// *declaring* box's, for the reason its colour and its height are: a
		// decoration is drawn across what it crosses without paying attention
		// to it, so a thickness set on the paragraph is one weight of line
		// under words at three sizes. Where the box said neither, its own
		// face's answer is carried on the decoration — a face is not something
		// this stage can ask.
		band := decorationBand(d.Kind, 0, run.Width, d.Shift.Sub(run.Shift),
			asDeclared(d))
		if band.Empty() {
			continue
		}
		p.emit(FillRect{
			Rect: placeRun(band, at, turn), Color: colour, Overhang: true,
		})
	}
}

// placeRun puts a rectangle measured in a run's own axes onto the page.
//
// The rectangle's X is a distance along the line from the run's start and its Y
// a distance off the baseline, positive downwards — the axes a horizontal run
// is already in, which is why the horizontal case is a translation and nothing
// else.
//
// The sideways case is the quarter turn: along the line becomes down the page,
// and off the baseline becomes back towards the left. A rectangle's far edge in
// the second axis is its near edge afterwards, which is where the extra
// subtraction of the height comes from. Anticlockwise is the same turn the
// other way round, and the subtraction moves to the other axis with it.
// runTurn is which way a run is set on the page: across it, or down it one way
// or the other.
//
// The two facts travel together because they are one decision, and because two
// bare booleans side by side at a call site are an invitation to pass them in
// the wrong order — which no test would catch for a run that is not turned at
// all, which is almost every run in almost every document.
type runTurn struct{ sideways, anticlockwise bool }

func turnOfLine(l LineFragment) runTurn {
	return runTurn{sideways: l.Sideways, anticlockwise: l.Anticlockwise}
}

func turnOfRun(v DrawText) runTurn {
	return runTurn{sideways: v.Sideways, anticlockwise: v.Anticlockwise}
}

func placeRun(r Rect, at Point, turn runTurn) Rect {
	switch {
	case !turn.sideways:
		return Rect{X: at.X.Add(r.X), Y: at.Y.Add(r.Y), W: r.W, H: r.H}
	case turn.anticlockwise:
		// The other turn, so both axes go the other way: along the line runs up
		// the page and off the baseline runs right. The extra subtraction moves
		// from the first axis to the second for exactly that reason.
		return Rect{X: at.X.Add(r.Y), Y: at.Y.Sub(r.X).Sub(r.W), W: r.H, H: r.W}
	}
	return Rect{X: at.X.Sub(r.Y).Sub(r.H), Y: at.Y.Add(r.X), W: r.H, H: r.W}
}

// drawableText replaces the characters a face cannot set with the white space
// they stand for.
//
// It is only the tab, and only because a tab's whole meaning is its position:
// the advance was decided against the tab stops before this, so nothing about
// the page depends on the character reaching the face — and everything about it
// depends on .notdef not being drawn where an author wrote an indent.
func drawableText(s string) string {
	if strings.IndexByte(s, '\t') < 0 {
		return s
	}
	return strings.ReplaceAll(s, "\t", " ")
}

// color resolves a computed colour property.
//
// "currentcolor" is the one value that is not a colour but a reference to one —
// it means whatever "color" resolved to, which is what makes a border take the
// text's colour without the author repeating it.
func (p *painter) color(b *Box, property string) (style.RGBA, bool) {
	if b == nil {
		return style.RGBA{}, false
	}
	raw := ascii.TrimCSSSpace(b.Style.Get(property))
	if raw == "" {
		return style.RGBA{}, false
	}
	if ascii.EqualFold(raw, "currentcolor") {
		if property == "color" {
			// The cascade resolves this one: CSS Color 4 §7.2 makes
			// "currentcolor" on "color" itself an "inherit", and inheritance is
			// the cascade's. A computed style therefore never carries it here,
			// and what is left is a box whose style a caller assembled by hand.
			// The initial value is the only answer available with no parent to
			// ask, and it is a backstop rather than the rule.
			return style.RGBA{A: 1}, true
		}
		return p.color(b, "color")
	}
	if got, ok := p.colors[raw]; ok {
		return got, got.A >= 0
	}
	c, ok := parseColorValue(raw)
	if !ok {
		return style.RGBA{}, false
	}
	p.colors[raw] = c
	return c, true
}

// parseColorValue reads a computed colour that is not "currentcolor".
//
// It is separate from the painter's memoized lookup because layout asks the same
// question once, of the root and of <body>, when it decides whether either
// declares a background to give the canvas.
func parseColorValue(raw string) (style.RGBA, bool) {
	raw = ascii.TrimCSSSpace(raw)
	if raw == "" || ascii.EqualFold(raw, "currentcolor") {
		return style.RGBA{}, false
	}
	vals, _ := css.ParseComponentValues(raw)
	return style.ParseColor(vals)
}

// ShapedText is the string handed to the shaper for one text run.
//
// The run's own text is in logical order and carries no direction of its own: a
// run of punctuation between two Hebrew words is right-to-left because of
// characters that are in other runs by now. The shaper applies UAX #9 to the
// string it is given, so left to itself it would answer for that string rather
// than for the paragraph the run came out of, and a lone bracket would come out
// facing the wrong way.
//
// So the direction the layout resolved is stated to it, in the one vocabulary a
// string has for saying so: an explicit right-to-left override in front of the
// text. That is exactly what the character means; it is a default-ignorable code
// point, so the shaper drops it before any glyph is chosen; and what comes back
// is the run's glyphs in the order they are drawn, with rule L4's mirroring
// applied.
//
// The override goes here and not into the run's text, because the run's text is
// what a reader copies out of the finished page.
func ShapedText(v DrawText) string {
	if !v.RTL {
		// A left-to-right run needs nothing. Every character in it resolved to
		// an even level, so the shaper's own answer for the string is already
		// this one.
		return v.Text
	}
	return "‮" + v.Text
}

// ShapedGlyphs is the glyphs a backend draws for a run, in the order the pen
// meets them.
//
// It exists so that the three fields a run needs shaping with cannot be used one
// or two at a time. The text has to go through ShapedText for the direction, and
// the context either side has to go with it or a cursive run comes out in
// isolated forms — at a different width from the one layout measured, so the
// run after it is drawn in the wrong place too. A backend calling ShapeGlyphs
// directly gets that wrong silently, which is why the pairing is stated here
// rather than left as something every backend has to remember.
func ShapedGlyphs(v DrawText) ([]shape.Glyph, int) {
	if v.Face == nil {
		return nil, 0
	}
	if !v.ContextKerns {
		// The neighbour is set in another face, so its characters decide this
		// run's joined shapes and its glyphs decide nothing. See
		// shape.ShapeGlyphsAcrossFaces.
		return v.Face.ShapeGlyphsMerged(ShapedText(v), v.PreContext, v.PostContext,
			v.MergePre, v.MergePost, false,
			v.Features)
	}
	return v.Face.ShapeGlyphsMerged(ShapedText(v), v.PreContext, v.PostContext,
		v.MergePre, v.MergePost, true,
		v.Features)
}

// maxLayerMarks bounds the rectangles one background layer is drawn as.
//
// A solid or banded tiling is not a picture: it leaves here as rectangles, one
// per band per tile along any axis whose tiles cannot be merged, and the count
// is (area / tile) — a ratio the stylesheet controls both ends of.
// "background-size: 1px 1px" on a gradient of two stops over a 600 by 800 box
// was 960,000 fills and 149 MB from one declaration, and the tile cap did not
// see it: that cap is per layer and says how many tiles a *backend* will be
// asked to repeat, and the bands, the layers and the elements sharing the rule
// all multiply it.
//
// Sixty-five thousand is a stripe a pixel apart down a box thirty thousand
// pixels long, which is further than any page goes. A layer past it repeats
// more finely than anything a reader can tell apart, and is drawn as what a
// reader sees there — see averageTiling.
//
// A variable so that a test can lower it.
var maxLayerMarks = 1 << 16

// tiling paints a solid or banded layer: bands placed in every tile.
//
// A solid layer is one band the size of its tile. The rectangles are merged
// along an axis where that is exact — the tiles abut on it (the step is the
// tile's own size, which is every repeat but space) and every band spans the
// whole tile across it — so an abutting tiling of one colour is one rectangle
// however many tiles it is, and a stack of horizontal stripes is one rectangle
// per stripe per row. That merge is not a tidiness: it is what makes a page
// written as "linear-gradient(green, green)" produce the same display list as
// the same page written with background-color, which is what a reftest
// comparing the two is asking about.
//
// What is left is counted before a rectangle is made. Past maxLayerMarks, or
// past what the document's work budget will pay for, the layer is drawn as its
// average colour instead, and the box is told.
func (p *painter) tiling(l bgPaint, bands []bgBand, who *Box) {
	mergeX, mergeY := l.StepX == l.Tile.W, l.StepY == l.Tile.H
	for _, b := range bands {
		mergeX = mergeX && b.Rect.X == 0 && b.Rect.W == l.Tile.W
		mergeY = mergeY && b.Rect.Y == 0 && b.Rect.H == l.Tile.H
	}
	nx, ny := 1, 1
	if !mergeX {
		nx = tileSpan(l.Clip.X, l.Clip.Right(), l.Tile.X, l.Tile.W, l.StepX)
	}
	if !mergeY {
		ny = tileSpan(l.Clip.Y, l.Clip.Bottom(), l.Tile.Y, l.Tile.H, l.StepY)
	}
	if nx <= 0 || ny <= 0 {
		return
	}
	marks := int64(nx) * int64(ny) * int64(len(bands))
	if marks > int64(maxLayerMarks) {
		p.averageTiling(l, bands, who, fmt.Sprintf(
			"it would take %d rectangles, past the %d this engine draws one background layer as",
			marks, maxLayerMarks))
		return
	}
	if !p.rec.charge(marks*costOp, "the background tilings past that point, drawn as their average colour") {
		p.averageTiling(l, bands, who, "")
		return
	}
	xs := axisSpans(mergeX, l.Clip.X, l.Clip.Right(), l.Tile.X, l.Tile.W, l.StepX)
	ys := axisSpans(mergeY, l.Clip.Y, l.Clip.Bottom(), l.Tile.Y, l.Tile.H, l.StepY)
	for _, y := range ys {
		for _, x := range xs {
			for _, b := range bands {
				r := Rect{X: x.lo.Add(b.Rect.X), Y: y.lo.Add(b.Rect.Y), W: b.Rect.W, H: b.Rect.H}
				if mergeX {
					r.X, r.W = x.lo, x.hi.Sub(x.lo)
				}
				if mergeY {
					r.Y, r.H = y.lo, y.hi.Sub(y.lo)
				}
				if r = r.Intersect(l.Clip); r.Empty() {
					continue
				}
				// Paid for above, so appended rather than emitted.
				p.ops = append(p.ops, FillRect{Rect: r, Color: b.Color})
			}
		}
	}
}

// averageTiling paints a tiling as the one colour it averages to, over its
// clip, and says so when why is given; a refusal by the work budget has said so
// already.
//
// It is what a reader sees. A pattern repeating every hundredth of a pixel is
// not stripes on any device: a browser rasterises the tile, and a tile smaller
// than a device pixel is resampled into the pixels it falls in, which is the
// average of its colours weighted by how much of the tile each covers. That is
// computed here, with the alpha premultiplied — a band of transparent and a
// band of red average to half-transparent red, not to a darker red — and with
// the gaps a "space" repeat leaves counted as transparent. It is exact only
// where the pattern really is finer than a pixel, which is why the box is told
// whenever it is drawn this way.
func (p *painter) averageTiling(l bgPaint, bands []bgBand, who *Box, why string) {
	period := l.StepX.Px() * l.StepY.Px()
	if period <= 0 {
		return
	}
	var a, r, g, b float64
	for _, band := range bands {
		f := band.Rect.W.Px() * band.Rect.H.Px() / period
		wa := f * band.Color.A
		a += wa
		r += wa * band.Color.R
		g += wa * band.Color.G
		b += wa * band.Color.B
	}
	if why != "" && who != nil {
		p.rec.ReportDetail(Finding{
			Rule:     RuleLimit,
			Source:   AtHTML(offsetOf(who)),
			Message:  "a background layer repeats too finely to draw tile by tile, so it was drawn as its average colour: " + why,
			Path:     PathOf(who.Element),
			Property: "background-image",
		})
	}
	if a <= 0 {
		return
	}
	avg := style.RGBA{R: r / a, G: g / a, B: b / a, A: min(a, 1)}
	p.emit(FillRect{Rect: l.Clip, Color: avg})
}

type span struct{ lo, hi style.Unit }

// axisSpans is the intervals one axis of a tiling covers within its clip: the
// clip itself when the axis merges, and otherwise tile by tile.
func axisSpans(merge bool, clipLo, clipHi, tileLo, size, step style.Unit) []span {
	if size <= 0 || step <= 0 || clipHi <= clipLo {
		return nil
	}
	if merge {
		return []span{{clipLo, clipHi}}
	}
	n := tileSpan(clipLo, clipHi, tileLo, size, step)
	if n <= 0 {
		return nil
	}
	first := math.Floor(clipLo.Sub(tileLo).Sub(size).Px()/step.Px()) + 1
	out := make([]span, 0, n)
	for i := 0; i < n; i++ {
		lo := tileLo.Add(step.Mul(first + float64(i)))
		out = append(out, span{lo, lo.Add(size)})
	}
	return out
}
