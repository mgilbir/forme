package paragraph

import (
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// How tall a line turns out.
//
// CSS 2.1 §10.8: the line box is the distance between the uppermost box top and
// the lowermost box bottom, which read as a requirement says it contains
// everything on it. §10.8.1's vertical-align moves the boxes first, and its
// aligned subtrees — "top" and "bottom" — are placed against a line box whose
// height they can change, which reads as circular and is not.

// VAlign is what vertical-align asks of an inline-level box.
//
// The set is CSS 2.1 §10.8.1's, less the two that are not a choice of position:
// "inherit" is the cascade's business and a length or percentage is carried as
// a displacement from the baseline rather than as a mode of its own, because
// that is exactly what it is — "vertical-align: 4px" is baseline alignment with
// the baseline moved.
//
// It is read for an ordinary inline box as well as for an atomic one, which it
// was not: a <sup> used to be set at the smaller size the user-agent stylesheet
// gives it and on the same baseline as its surroundings. The two share every
// line of the arithmetic — §10.8.1 aligns "inline-level boxes" and says nothing
// about which kind — and the only difference left is where the two extents come
// from. See itemExtents.
type VAlign uint8

const (
	VAlignBaseline VAlign = iota
	VAlignTop
	VAlignBottom
	VAlignMiddle
	VAlignTextTop
	VAlignTextBottom
)

// Strut is the block's own contribution to every line box it makes.
//
// CSS 2.1 §10.8 gives each line box an imaginary zero-width inline box of the
// block's font and line-height, and that box takes part in the alignment
// whether or not there is any text on the line. It is why a line holding
// nothing but an image is still as tall as the image *plus* the descender space
// the type would have wanted, and why an empty <p> occupies a line.
type Strut struct {
	// Height and Baseline are the line-Height and where the Baseline sits in it.
	Height, Baseline style.Unit
	// Ascent and Descent are the font's own extents at the block's size, which
	// are what "text-top" and "text-bottom" name. They are not the same as the
	// two above: those include the half-leading.
	Ascent, Descent style.Unit
	// XHeight is what "middle" is measured against.
	XHeight style.Unit
}

// VAlignState is where §10.8.1's alignment has got to at one point of the walk
// over an inline subtree.
//
// The two halves answer different questions and cannot be one field, which is
// the whole reason this is a struct. align and raise say where the box sits
// against the baseline of *what it is inside*; subtree and lineAlign say which
// aligned subtree it belongs to and where that subtree sits against the *line
// box*. Only "top" and "bottom" ask the second question, and §10.8.1 defines
// them alone in terms of a subtree:
//
//	The aligned subtree of an inline element contains that element and the
//	aligned subtrees of all children inline elements whose computed
//	vertical-align value is not top or bottom.
//
// So a "middle" inside a "top" is still part of the top's subtree, placed within
// it by its own rule, and the whole of it then moves to the top of the line
// together. Getting that wrong is visible rather than academic: a
// "vertical-align: top" span holding text in two sizes had each run's own top
// put at the top of the line, which pulls the smaller one up out of the words it
// belongs with.
type VAlignState struct {
	// Align and raise place the box against its parent's baseline. A keyword
	// other than "baseline" is a position rather than a displacement, so it
	// replaces what it is inside instead of adding to it; a length, a
	// percentage, "sub" and "super" accumulate, which is what makes nested
	// superscripts rise twice.
	Align VAlign
	Raise style.Unit
	// LineAlign is vAlignTop or vAlignBottom when the box is inside an aligned
	// subtree placed against the line box, and vAlignBaseline when it is not.
	// subtree is the box that asked for it, which is what groups the items of
	// one subtree together.
	LineAlign VAlign
	Subtree   alignSubtree
}

// alignSubtree identifies one of §10.8.1's aligned subtrees.
//
// The stacking only ever asks whether two items belong to the same subtree, so
// this is compared for identity and never read. What declared the subtree is the
// business of whatever resolved the vertical-align — see itemRef for why the
// stacking is kept from knowing.
type alignSubtree any

// Aligned reports whether vertical-align moved anything at all, which is false
// for the great majority of the inline content of the great majority of
// documents.
func (v VAlignState) Aligned() bool {
	return v.Align != VAlignBaseline || v.Raise != 0 || v.LineAlign != VAlignBaseline
}

// itemExtents is how far an inline-level item's own box reaches above and below
// its own baseline, before vertical-align has moved it.
//
// The two kinds of item answer it differently and that is the whole of what
// distinguishes them here. An atomic inline's box is its margin box, measured
// when it was laid out. A run of text is §10.8's inline box: "the height of the
// inline box encloses all glyphs and their half-leading on each side and is thus
// exactly 'line-height'", which is the pair the leading gave it.
//
// Anything else on a line — an inset, a float marker, the record of an
// absolutely positioned box — is not an inline-level box at all and takes no
// part in §10.8.1's stacking.
//
// # What is missing, and what it cost to add
//
// An inline box that produces no items at all is not represented here, so its
// leading does not reach the line. §10.8 says it should: the height of an inline
// box "encloses all glyphs and their half-leading on each side and is thus
// exactly line-height", and one enclosing no glyphs still keeps the leading. The
// suite's empty-inline-003 is a <span> with "line-height: 5" and nothing in it,
// beside an "X", and the line it sits on should be five times the height it is.
//
// It was tried. An item marked Empty, carrying the box's leading and no width,
// emitted where the recursion produced nothing, together with §9.4.2's rule that
// a line of nothing but those is no line at all — which is needed, or an empty
// <span> alone in a <div> grows a line box it must not have. That produced the
// right answer for all four shapes of the case and cost *nineteen* other tests,
// most of them about white-space collapsing.
//
// The cause is worth recording because it is not about heights. An item in the
// stream is walked by everything downstream, and a zero-width item at the end of
// a line is not TrimAtEnd and not Inset — so it stops the trailing-space trimming
// in breaking.go that tests exactly those two flags. Doing this properly means
// auditing every walk of the item stream for what an item that stands for
// nothing should do, which is a larger change than the one test it is worth.
//
// One case of it *is* done, and it is the case that objection does not cover.
// An inline box with a margin, a border or padding already emits a pair of
// items for its own edges, and those are Inset — every walk of the stream
// already knows what to do with them — so they carry the box's leading and lead
// the line. It costs nothing and gains margin-padding-clear/margin-right-114.
// See layout/flatten.go's insetItems. What is still not done is the box with no
// edges at all, which is the shape this paragraph is about.
func itemExtents(item Item) (ascent, descent style.Unit, ok bool) {
	if item.Atomic != nil {
		return item.Ascent, item.Descent, true
	}
	if item.Leads {
		return item.Above, item.Below, true
	}
	return 0, 0, false
}

// StackLine gives a line its height and its baseline, once what is on it is
// known.
//
// CSS 2.1 §10.8 builds a line box by aligning everything on it against the
// baseline and then taking the distance from the highest top to the lowest
// bottom. The block's own "strut" — an imaginary zero-width Piece of its font
// at its line-height — always takes part, which is what gives a line of text its
// height whether or not there is text on it, and what leaves the familiar gap
// under an image that sits alone on a line: the strut still wants its
// descender.
//
// What is implemented is §10.8.1's alignment of every inline-level box on the
// line — a run of text as much as an atomic inline, since the section names
// neither and the arithmetic is the same for both once each has said how far it
// reaches above and below its own baseline. See itemExtents.
//
// Where it is not exact is which box a keyword alignment is measured against.
// §10.8.1 names the *parent*; this engine has flattened the tree by now and uses
// the block's own strut, so a "text-top" inside an intervening box whose font
// metrics differ from the block's names the block's content area rather than
// that box's.
func StackLine(runs []Item, s Strut) LineStack {
	ls := LineStack{Strut: s, Baseline: s.Baseline}
	// What the strut wants below the baseline. It can be *negative*, which is
	// the case that makes this a maximum rather than a floor: "line-height: 0"
	// gives the strut a half-leading of minus half the font's own height, so
	// its descent is below the baseline by a negative amount and an image on
	// the line has to be able to overrule it. Taking the strut's descent
	// unconditionally would make such a line shorter than the picture on it.
	descent := s.Height.Sub(s.Baseline)

	// First pass: everything aligned against the baseline, which is what
	// decides where the baseline is. What belongs to an aligned subtree is
	// gathered instead, because it is placed against a line box that does not
	// exist yet.
	//
	// Stacking the text runs and not only the atomic inlines was a fault fixed
	// rather than a simplification kept: a <span> set larger than the paragraph
	// around it grew nothing, so its line box stayed the strut's height and its
	// baseline sat where the smaller type wanted it.
	// §9.4.2's zero-height line: one holding nothing but the leading of inline
	// boxes that put nothing on it. Their leading is counted where there is
	// something to count it beside — §10.8.1's "just like elements with
	// content" — and not where there is not. empty-inline-001 and -003 are the
	// two halves and they disagree on purpose.
	content := false
	for _, item := range runs {
		if item.LeadingOnly {
			continue
		}
		if _, _, ok := itemExtents(item); ok {
			content = true
			break
		}
	}
	for _, item := range runs {
		if item.LeadingOnly && !content {
			continue
		}
		a, d, ok := itemExtents(item)
		if !ok {
			continue
		}
		a, d = alignedExtents(item.Valign, a, d, s)
		if item.Valign.LineAlign != VAlignBaseline {
			ls.gather(item.Valign, a, d)
			continue
		}
		if a > ls.Baseline {
			ls.Baseline = a
		}
		if d > descent {
			descent = d
		}
	}

	// Second pass: the subtrees that align against the line box itself. §10.8.1
	// defines them in terms of a line box whose height they can change, which
	// reads as circular and is not: a subtree taller than the line grows it on
	// the side away from its own edge, and one that fits changes nothing.
	height := ls.Baseline.Add(descent)
	for i := range ls.groups {
		g := &ls.groups[i]
		h := g.Ascent.Add(g.Descent)
		switch g.lineAlign {
		case VAlignTop:
			// Its top is the line's top, so anything it needs it takes from
			// below the baseline.
			if h > height {
				descent = descent.Add(h.Sub(height))
				height = h
			}
		case VAlignBottom:
			// Its bottom is the line's bottom, so it takes from above.
			if h > height {
				ls.Baseline = ls.Baseline.Add(h.Sub(height))
				height = h
			}
		}
	}
	ls.Height = ls.Baseline.Add(descent)

	// Where each subtree's own baseline ended up, which is what places the boxes
	// in it. It is a third pass because the height above is not settled until
	// every subtree has had its say, and a "top" subtree that grew the line
	// moves a "bottom" one.
	for i := range ls.groups {
		g := &ls.groups[i]
		if g.lineAlign == VAlignBottom {
			g.Baseline = ls.Height.Sub(g.Descent)
			continue
		}
		g.Baseline = g.Ascent
	}
	return ls
}

// LineStack is a finished line box: its height and baseline, and where each
// aligned subtree on it ended up.
type LineStack struct {
	Strut            Strut
	Height, Baseline style.Unit
	// groups is one entry per aligned subtree placed against the line box. There
	// is one for each "vertical-align: top" or "bottom" box with content on the
	// line, which is none at all in almost every document — so the lookups below
	// scan a slice rather than consult a map, and an ordinary line never
	// allocates.
	groups []alignGroup
}

// alignGroup is one of §10.8.1's aligned subtrees, as it appears on one line.
type alignGroup struct {
	subtree   alignSubtree
	lineAlign VAlign
	// Ascent and Descent are the subtree's extents: the highest top and the
	// lowest bottom of the boxes in it, measured from the subtree's own
	// baseline.
	Ascent, Descent style.Unit
	// Baseline is where that Baseline sits, from the top of the line box.
	Baseline style.Unit
}

// gather adds one box's extents to its subtree's.
func (ls *LineStack) gather(v VAlignState, ascent, descent style.Unit) {
	for i := range ls.groups {
		if ls.groups[i].subtree != v.Subtree {
			continue
		}
		if ascent > ls.groups[i].Ascent {
			ls.groups[i].Ascent = ascent
		}
		if descent > ls.groups[i].Descent {
			ls.groups[i].Descent = descent
		}
		return
	}
	ls.groups = append(ls.groups, alignGroup{
		subtree: v.Subtree, lineAlign: v.LineAlign, Ascent: ascent, Descent: descent,
	})
}

// baselineFor is where the baseline a box is placed against sits, from the top
// of the line box: the line's own, or its aligned subtree's.
func (ls *LineStack) baselineFor(v VAlignState) style.Unit {
	if v.LineAlign == VAlignBaseline {
		return ls.Baseline
	}
	for i := range ls.groups {
		if ls.groups[i].subtree == v.Subtree {
			return ls.groups[i].Baseline
		}
	}
	return ls.Baseline
}

// Shift is how far a box's own baseline sits below the line's, once
// vertical-align has placed it.
//
// It is the one number painting needs: a run's glyphs sit on its own baseline,
// and the line has only one of its own.
func (ls *LineStack) Shift(v VAlignState, ascent, descent style.Unit) style.Unit {
	a, _ := alignedExtents(v, ascent, descent, ls.Strut)
	// a is how far the box reaches above the baseline it is aligned against, so
	// the box's own baseline is that much below that box's top — and ascent is
	// how far its own baseline is below its own top.
	return ls.baselineFor(v).Sub(a).Add(ascent).Sub(ls.Baseline)
}

// alignedExtents is how far a box reaching ascent above and descent below its
// own baseline reaches above and below the baseline it is aligned against, once
// its vertical-align has been applied.
func alignedExtents(v VAlignState, ascent, descent style.Unit, s Strut) (style.Unit, style.Unit) {
	h := ascent.Add(descent)
	switch v.Align {
	case VAlignTextTop:
		// The top of the box against the top of the parent's content area,
		// which is the font's own ascent above the baseline rather than the
		// line box's top: the half-leading is not part of it.
		return s.Ascent, h.Sub(s.Ascent)
	case VAlignTextBottom:
		return h.Sub(s.Descent), s.Descent
	case VAlignMiddle:
		// The box's own midpoint against the baseline raised by half the
		// parent's x-height.
		half := h.Div(2)
		return half.Add(s.XHeight.Div(2)), half.Sub(s.XHeight.Div(2))
	}
	// Baseline, with whatever "sub", "super" or a length displaced it by.
	return ascent.Add(v.Raise), descent.Sub(v.Raise)
}

// AtomicTop is where an atomic inline's margin box goes within its line box.
func (ls *LineStack) AtomicTop(item Item) style.Unit {
	return ls.Baseline.
		Add(ls.Shift(item.Valign, item.Ascent, item.Descent)).
		Sub(item.Ascent)
}

// NormalLineHeightFallbackFactor is what "normal" means for a face that does
// not say.
//
// The font's own answer is ascent + descent + line gap, and forme reports all
// three now — but only for a face that has the tables to state them. The
// fourteen standard PDF faces have no hhea and no OS/2 at all: their metrics
// come from AFM data, which carries no line gap, so Descriptor reports zero with
// the bit clear. Zero and silence are different answers and this is the one
// place in the engine where reading them as the same would be invisible: a page
// spaced by a number the font never gave.
//
// 1.2 for those, because CSS 2.1 §10.8.1 recommends between 1.0 and 1.2 and a
// value inside the range beats one derived from a term that is missing.
const NormalLineHeightFallbackFactor = 1.2

// lineExtents is how far a face's type reaches above and below its baseline,
// for the purpose of laying lines out.
//
// It is one function because the two places that ask have to agree. "line-height:
// normal" is these two plus the line gap, and an inline box's *content area* —
// what its background paints and what "text-top" and "text-bottom" name — is
// these two exactly. When they come from different formulas the two disagree,
// and the suite catches it directly: inline-formatting-context-002 draws a black
// inline box beside a float of the same text and asks for the two to be the same
// height. Ours were 14.4 and 19.2.
//
// # Where the numbers come from, and the case that forced this
//
// hhea's ascender and descender, for any face that has an hhea to state them.
// The standard fourteen PDF faces have neither hhea nor OS/2: their metrics come
// from AFM data, whose Ascender and Descender are *typographic* — about the top
// of a "d" and the bottom of a "p" — rather than the box the face wants a line
// laid out in. Times comes to 0.900em that way and Courier to 0.786em, which is
// tighter than any browser sets the same text: a browser reads usWinAscent and
// usWinDescent from a real Times, which come to about 1.15em.
//
// So for a face that states no line gap — the one thing that says there is no
// hhea and no OS/2 behind these numbers — the glyph bounding box is used
// instead. It is the AFM's own answer to the same question, it is the nearest
// thing the file has to usWin*, and it puts Times at 1.116em and Courier at
// 1.055em: inside §10.8.1's recommended range, and close to what a browser
// produces for the same document.
//
// The previous shape of this was a 1.2 factor for "normal" alone, which put the
// line height in range and left the content area at 0.900em. That is the pair
// the suite disagreed with — and simply dropping the factor, so both came to
// 0.900em, made them agree at a line height no browser would produce and lost
// inline-formatting-context-015, whose reference is a 30px cell that two lines
// of text have to fill.
func LineMetrics(face *shape.Face) (top, bottom, upem float64, ok bool) {
	upem = float64(face.UnitsPerEm())
	if upem <= 0 {
		return 0, 0, 0, false
	}
	d := face.Descriptor()
	top, bottom = float64(d.Ascent), float64(d.Descent)
	if !d.Has(shape.MetricLineGap) {
		top, bottom = float64(d.BBox[3]), float64(d.BBox[1])
	}
	if top-bottom <= 0 {
		return 0, 0, 0, false
	}
	return top, bottom, upem, true
}
