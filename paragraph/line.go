package paragraph

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Point is a position.
type Point struct{ X, Y style.Unit }

// DecorationKind is one of the three lines §16.3.1 defines.
type DecorationKind uint8

const (
	DecorationUnderline DecorationKind = iota
	DecorationOverline
	DecorationLineThrough
)

// Decoration is one line to draw, together with the box that asked for it.
type Decoration struct {
	Kind DecorationKind
	// By is the box whose declaration produced the line, held opaquely. It is
	// kept rather than a resolved colour because the colour is not known until
	// painting — and because "currentcolor", which is text-decoration-color's
	// initial value, means the declaring box's own colour rather than the colour
	// of whatever text the line happens to cross. See itemRef.
	By Ref
	// Shift is how far the declaring box's own baseline sits below the line's,
	// which is §10.8.1's vertical-align applied to *that* box and not to the run
	// the decoration is attached to.
	//
	// §16.3.1 is the reason the two are different numbers: a decoration is drawn
	// across the whole of the box that declared it "without paying any attention
	// to" what it crosses, so a div with an overline over three spans at three
	// vertical-aligns rules one straight line and not three stepped ones. The
	// field is filled in when a run is placed on a line, since only then is the
	// line's own geometry known, and it stays zero for the decorations of every
	// document that does not use the property.
	Shift style.Unit
}

// Frame is what the walk over an inline subtree carries down: enough to
// resolve a relatively positioned inline box's own offset, and the offset
// already accumulated from the inline boxes around it.
//
// The accumulation is what makes nesting work. "<span style=...><em style=...>"
// offsets the em by the sum of the two, because §9.4.3 moves a box together with
// everything inside it, and the inside of an inline box is a run of text that
// has been flattened out of the tree by the time anything could walk it again.
type Frame struct {
	Containing style.Unit
	CbHeight   style.Unit
	CbDefinite bool
	Offset     Point
	// Measuring says the walk is being made to find an intrinsic width rather
	// than to lay a line out, so nothing that has a layout of its own is given
	// one.
	//
	// It is not an optimisation. Laying an inline-block out during a
	// measurement produces a fragment that is then discarded — and the
	// absolutely positioned boxes found inside it would have been recorded
	// against it, so every one of them would be placed twice, once against a
	// rectangle that no longer exists. That is the same fault settle() has to
	// undo when a subtree is laid out again, and here it is cheaper not to
	// create it.
	Measuring bool
	// Strut is the block container's own line metrics, which an atomic inline
	// needs to resolve vertical-align. It is the containing block's rather than
	// the box's own, because that is what the property is measured against: a
	// "text-top" is the top of the *parent's* content area.
	Strut Strut
	// Valign is §10.8.1's vertical-align, accumulated over the inline boxes the
	// walk is currently inside.
	//
	// It travels on the frame for the same reason offset does: the flattening
	// destroys the boxes, and a run of text has to carry out of it whatever its
	// ancestors declared. A text box cannot be asked — vertical-align is not
	// inherited, so the anonymous box holding a <span>'s words has the initial
	// value whatever the span said.
	Valign VAlignState
	// Bidi collects the context's text in logical order as the walk flattens it,
	// so that the bidirectional algorithm has a paragraph to run over. It is nil
	// while measuring: an intrinsic width is a sum over the items and does not
	// depend on the order they are set in.
	Bidi *BidiBuilder
}

// inlineItem is one Piece of inline content before it has been put on a line.
// Ref is something on a line that the line itself does not own: a box out
// of flow, or the fragment an atomic inline was already laid out into.
//
// It is opaque on purpose, and the purpose is a seam. Breaking a paragraph into
// lines, ordering the runs on one and stacking them to a height are questions
// about text, measured widths and Unicode — none of which needs a box tree. What
// that half needs to know about a float is that one was met here and must be
// handed back; what it needs to know about an atomic inline is how far it
// reaches. Both are answerable without naming what the thing is, and holding it
// opaque is what keeps the answering code from quietly growing a dependency on
// the tree it is supposed to be independent of.
//
// Nothing in that half dereferences one. Whatever built the items put it in and
// is the only thing that takes it out again, in the layer that knows what it is.
type Ref any

type Item struct {
	Text string
	// Box is the box this item's content came from, held opaquely: it carries
	// the colour and the decorations painting needs, and it is what tells two
	// items of the same inline box apart. Neither is a question the breaking
	// asks. See itemRef.
	Box   Ref
	Face  *shape.Face
	Size  style.Unit
	Width style.Unit
	// BreakBefore marks an item that may begin a line, which is what a break
	// opportunity is once the text has been cut into pieces.
	BreakBefore bool
	// Space marks white Space of any kind: it ends the word before it and does
	// not join the word after it.
	//
	// "Of any kind" is §4.1.2's sense — white Space, other Space separators and
	// preserved tabs — rather than Phase I's, which is only U+0020, U+0009 and
	// the segment breaks. The two differ over the ideographic Space and its
	// relatives, which hang at the end of a line and are never collapsed.
	Space bool
	// Collapsible marks white space that §4.1.2 removes when it lands at
	// either end of a line.
	//
	// It is not the same question as space, and conflating the two is what made
	// "<pre>   x</pre>" lose its indentation: the leading run is white space,
	// so it was dropped at the start of the line, but it is *preserved* white
	// space and dropping it removes something the author wrote.
	Collapsible bool
	// TrimAtEnd marks white space that §4.1.2's third rule removes outright when
	// it lands at the end of a line, rather than hanging past it.
	//
	// It is the collapsible spaces, and one character more: "any trailing U+1680
	// OGHAM SPACE MARK whose white-space property is normal, nowrap, or
	// pre-line". The ogham space mark is not collapsible — it is a *visible*
	// space, a stemline in an ogham face, and collapsing a run of them would
	// shorten a line of ogham the way collapsing a run of hyphens would shorten a
	// rule — so it needs the removal without the collapsing, and that is why this
	// is a second flag rather than a use of the first.
	TrimAtEnd bool
	// Hangs marks preserved white space that sits past the end of the line
	// rather than moving to the next one.
	//
	// §4.1.2 Hangs whatever white space its third rule left at the end of a
	// line, so it is not counted when the line is measured for alignment and
	// never causes a break of its own. Two values are named as not doing it:
	// break-spaces, which is the whole difference between it and pre-wrap, and
	// pre, which the rule does not list — a line under pre ends only where the
	// author ended it, and the rule is about what happens at a wrap.
	Hangs bool
	// HangsHard says the hang is unconditional: the sequence never takes
	// room, whether or not there is room for it. §4.1.2 gives that answer
	// for normal, nowrap and pre-line, and the conditional one for pre-wrap
	// before a forced break — where the sequence does take room, and gives
	// it up only when it would overflow. The two differ nowhere on the page
	// and differ in both intrinsic widths.
	HangsHard bool
	// BreakWord is overflow-wrap's last-resort break, carried per item because
	// the property is the box's and a line holds items from several boxes.
	BreakWord bool
	// Anywhere is the value of overflow-wrap that also lowers the min-content
	// width, so a shrink-to-fit box narrows to its widest character rather than
	// to its widest word. §5.5 says break-word's opportunities "are not
	// considered when calculating min-content intrinsic sizes" and Anywhere's
	// are, which is the whole difference between the two values.
	Anywhere bool
	// Tab marks one preserved Tab. Its advance is not a property of the text —
	// it is the distance to the next Tab stop, so it is resolved when the Tab
	// has a place on a line and not before.
	Tab bool
	// TabStop is the distance between two tab stops, from tab-size.
	TabStop style.Unit
	// TabFloor is §4.1.2's 0.5ch threshold: a tab whose shift would be shorter
	// than this advances to the tab stop after the nearest one instead.
	TabFloor style.Unit
	// Forced marks a break the author asked for — a <br>, or a newline in
	// preserved white space. It ends the line wherever it falls, which is the
	// difference between a break opportunity and an instruction.
	Forced bool
	// NoWrap marks text that may not break at its spaces, so a line takes it
	// whole or overflows.
	NoWrap bool
	// Inset marks an item that is an inline box's own horizontal margin, border
	// and padding rather than anything of its content: §8.3, §8.4 and §8.5 make
	// all three apply to a non-replaced inline box on the horizontal axis, and
	// what they do there is push the content along. See insetItems.
	Inset bool
	// InsetLead distinguishes the two: the item before the box's content from
	// the item after it, in *logical* order.
	//
	// Which of them carries the box's left inset and which its right is not the
	// same question, and on a right-to-left line it is the other answer — see
	// insetSides for §8.6. The flag says where the item sits in the content, and
	// the width says what it holds.
	InsetLead bool
	// InsetLevel is the embedding level the box's own edges sit at, and
	// insetLevelKnown says insetSides worked one out.
	//
	// An inset carries no characters, so the algorithm gives it no level of its
	// own, and the two obvious guesses are both wrong somewhere: the level of the
	// neighbouring item glues the box's edge to whatever run happens to abut it,
	// and the paragraph's base level detaches it from its own content. What the
	// edge of an inline box sits at is the *lowest* level anything inside it
	// reached — an embedding inside the box only raises the level of what is
	// inside, and the box's own boundary is outside all of them.
	//
	// The flag is separate because zero is a real level, the left-to-right one,
	// and a box with no content on the line at all has to stay distinguishable
	// from a box whose content is left-to-right.
	InsetLevel      int
	InsetLevelKnown bool
	// Float is a Float met in this run of inline content. It carries no text of
	// its own: it is a marker saying "a Float belongs here", because where a
	// Float appears among the words decides which line box it is placed against,
	// and that position is lost once the items are on lines.
	Float Ref
	// Offset is the relative displacement of the inline boxes this item is
	// inside, which travels with the item because the flattening loses the boxes
	// themselves.
	Offset Point
	// AtomicBox marks an item that is a box on the line rather than a run of
	// text: a replaced element or an inline-block. It is set whether or not the
	// box was laid out, because an intrinsic-width measurement needs to know
	// there is one without producing a fragment for it.
	AtomicBox Ref
	// Atomic is that box's fragment, already laid out. It is nil while
	// measuring.
	//
	// Being laid out already is what makes the item Atomic: its size comes from
	// its own content and its own declarations, so nothing about the line can
	// change it. All the line decides is where it goes.
	Atomic Ref
	// Leads reports that this item is a run of text whose own inline box takes
	// part in §10.8.1's stacking, and above and below are how far it reaches from
	// the baseline.
	//
	// The flag is separate from the two lengths rather than derived from them
	// because zero is a legitimate answer: "line-height: 0" on a <span> gives a
	// run that reaches nowhere at all, and reading that as "this item has no
	// metrics" would let a taller strut win a line the span was supposed to
	// collapse. Every other kind of item — a float marker, an absolutely
	// positioned one, an inline box's own inset — leaves it clear.
	Leads        bool
	Above, Below style.Unit
	// Ascent and Descent are how far the item reaches above and below the
	// baseline, measured over its *margin* box.
	//
	// The two differ by which of §10.8.1's rules gave them. A replaced element's
	// baseline is its bottom margin edge, so it is all Ascent — which is why a
	// picture sits on the line of type rather than in the middle of it, and why
	// a line holding one is as tall as the image plus whatever descender space
	// the surrounding text still wants. An inline-block's baseline is the
	// baseline of its *last line box*, so a box of two paragraphs hangs below
	// the line by the depth of its second one — unless it has no line boxes at
	// all or clips its overflow, when it too is all Ascent.
	Ascent, Descent style.Unit
	// Valign is §10.8.1's vertical-align, as the walk accumulated it over the
	// inline boxes this item sits inside.
	Valign VAlignState
	// Decorations are the lines ruled across this item, and spacing is what
	// letter-spacing and word-spacing added to its width. Both travel with the
	// item because the flattening loses the boxes they were read from.
	Decorations []Decoration
	Spacing     TextSpacing
	// Abs is an absolutely positioned box met in this run, and it is a marker for
	// the same reason and a different consequence. A float met among the words
	// changes where the words go; an absolutely positioned one does not change
	// anything at all, but its *static position* — where it would have been — is
	// what §10.3.7 falls back on, and that is exactly the information the
	// flattening destroys.
	Abs Ref
	// BidiPara, bidiStart and bidiEnd say where this item's text sits in the
	// inline formatting context's bidi paragraphs, which is what the algorithm
	// resolves levels over.
	//
	// BidiPara counts from one so that zero means "contributes no characters",
	// which is what a float or an absolutely positioned box is: out of flow, and
	// taking no part in the ordering. Numbering from zero would have made a
	// forgotten field on any of the several places that build an item read as a
	// claim to be the first character of the first paragraph.
	BidiPara           int
	BidiStart, BidiEnd int
	// Para is the resolved paragraph, filled in once the algorithm has run, and
	// level is this item's embedding level. Both are zero-valued in a document
	// that needs no reordering, which is what tells the line builder there is
	// nothing to do.
	Para  *bidiParagraph
	Level int
}

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
	for _, item := range runs {
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

// CursorAdvanced reports whether a line took at least one item, or at least one
// byte of one, from where it began.
//
// The cursor is the pair (item, byte into that item), so "forwards" is the
// lexicographic order on the two and not a comparison of either alone: a line
// that ends inside the item it started in has the same index and a greater
// offset, and a line that ends in a later item may have any offset at all,
// including a smaller one — which is why the offset is only consulted when the
// index has not moved.
func CursorAdvanced(wasI, wasByte, i, iByte int) bool {
	return i > wasI || (i == wasI && iByte > wasByte)
}

// MaxLineFits bounds how many times one line box may be broken again because
// the band it was broken against turned out to be the wrong one.
//
// Each round is a strictly narrower band or a strictly lower line, so the
// sequence cannot return to a state it has left — but a narrower band can make
// a line taller (a word wraps onto it) as easily as shorter, and a taller line
// meets a different set of floats, so there is no argument that it settles at
// all. Two extra attempts is what the suite's deepest case needs; past that the
// line keeps the break it has, which is a line that is slightly wrong rather
// than a render that does not finish.
//
// A variable so that a test can lower it and watch the bound decide, on the
// model of maxRelayouts.
var MaxLineFits = 2

// MidLineBox is an out-of-flow box met after a line had already begun, together
// with how much of that line had been filled when it was reached.
//
// The same record serves floats and absolutely positioned boxes because both
// need the one number and neither needs anything else — but they need it for
// opposite reasons, which is why the two are told apart rather than merged. For
// a float it says whether there is still room beside the line; for an absolutely
// positioned box it *is* the static position, and there is no question of room
// because the box takes none.
type MidLineBox struct {
	// Box is the out-of-flow Box itself, held opaquely: the breaking records
	// that one was reached and hands it straight back, and the caller that put
	// it among the items is the one that knows what to do with it. See itemRef.
	Box Ref
	// Used is how much of the line's width had been filled when the box was
	// reached, measured from the line's own left edge.
	Used style.Unit
	// Abs distinguishes the two kinds.
	Abs bool
}

// State is what the flattening carries from one inline box to the next.
//
// Both fields are about a rule that spans a box boundary, which is why they
// travel rather than being recomputed per box: neither can be answered by
// looking at one text node.
type State struct {
	// BreakOpportunity says the content before this point ended at one. In
	// "foo <em>bar</em>" the space and the word are in different text boxes, so
	// an engine that started each box afresh would find no opportunity between
	// them and set the whole phrase as one unbreakable word.
	BreakOpportunity bool
	// AfterCollapsibleSpace says the last thing emitted was a collapsible
	// space, so §4.1.1's fourth rule collapses the next one into it —
	// "provided both spaces are within the same inline formatting context",
	// which is exactly the span this state covers.
	//
	// It starts true, because the beginning of the context is the beginning of
	// its first line and §4.1.2 removes the collapsible space there.
	AfterCollapsibleSpace bool
}

// StartOfContext is the state an inline formatting context begins in.
func StartOfContext() State { return State{AfterCollapsibleSpace: true} }

// Measure returns the advance width of a string in a face, memoized.
//
// It is the face's own advance and nothing else, which is what the three callers
// that use it want: a tab stop is a multiple of the space advance, "ch" is the
// advance of a zero, and a list marker is set without the text's spacing. Text
// that is laid out on a line goes through measureSpaced instead.
func (br *Breaker) Measure(face *shape.Face, text string, size style.Unit) style.Unit {
	return br.MeasureSpaced(face, text, size, TextSpacing{})
}

// MeasureSpaced is the advance of a run as it will be set, with letter-spacing
// and word-spacing in it.
//
// Measuring is the inner loop of line breaking, and the same words recur
// constantly in a document — every "the" in a page measures the same. The key
// includes the face and the size because both scale the answer, and the spacing
// because it changes it: two boxes at the same size in the same face with
// different letter-spacing must not share an entry. Leaving it out of the key is
// the same memoization bug lengthKey.zeroAdvance records for the "ch" unit, and
// it produces a wrong page only in a document that uses two values.
func (br *Breaker) MeasureSpaced(face *shape.Face, text string, size style.Unit,
	sp TextSpacing) style.Unit {

	if text == "" {
		return 0
	}
	key := measureKey{face: face, text: text, size: size, spacing: sp}
	if got, ok := br.measured[key]; ok {
		return got
	}
	// Measure returns the advance in the units the size was given in, so a size
	// in CSS pixels gives an advance in CSS pixels.
	w, _ := style.FromPx(face.Measure(text, size.Px()))
	w = w.Add(SpacingAdvance(text, sp))
	br.measured[key] = w
	return w
}

type measureKey struct {
	face    *shape.Face
	text    string
	size    style.Unit
	spacing TextSpacing
}

// BlockEllipsis is what a clamped block puts at the end of its last line.
//
// CSS Overflow 4's "block-ellipsis: auto" is "a UA-defined value", and the
// horizontal ellipsis is the one every engine uses and the one the suite's
// references write.
const BlockEllipsis = "\u2026"

// PositiveInteger reads a whole number above zero, which is the only form of
// either clamp property this engine acts on.
func PositiveInteger(value string) (int, bool) {
	s := strings.TrimSpace(value)
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
		if n > maxClampLines {
			return maxClampLines, true
		}
	}
	return n, n > 0
}

// maxClampLines bounds the count an untrusted stylesheet can state.
//
// The number is only ever compared against a line count, so a large one clamps
// nothing — but it is parsed from a document and multiplied by nothing, and a
// bound costs one line. It is far above any block a reader would call clamped.
const maxClampLines = 1 << 20

// MaxBalanceLines bounds how many lines this engine will balance.
//
// Balancing costs a binary search over the width, and each probe breaks the
// whole paragraph again — so a page of running prose set to "text-wrap: balance"
// would be laid out sixteen times over. CSS Text §5.1 allows the bound in as
// many words ("UAs may disable balancing when the number of lines exceeds some
// threshold"), and balancing is a display effect: it is what a headline of two
// or three lines is for, and nobody can see it in a paragraph of thirty.
//
// It is a variable so that a test can lower it far enough to watch it decide
// something. A bound that has only ever been observed not to trip is one nobody
// knows works.
var MaxBalanceLines = 6

// BalanceWidth is CSS Text §5.1's "text-wrap-style: balance", computed as the
// narrowest width that still fits the text in the same number of lines.
//
//	balance: Line breaks are chosen to balance the remaining (empty) space in
//	each line box, if a better balance than block-progression-first filling is
//	possible.
//
// The specification gives no algorithm, and the one below is the one every
// implementation uses, because its two statements turn out to be the same: a
// greedy break at the narrowest width that still makes N lines is the greedy
// break whose longest line is as short as it can be, which is exactly "the
// remaining space is as even as it can be made". "The quickest brown fox jumped
// over the lazy dog" in thirty-five characters greedily fills the first line to
// thirty-three and leaves twelve on the second; the narrowest width that still
// takes two lines is twenty-four, and there it reads "The quickest brown fox /
// jumped over the lazy dog", which is what the suite's text-wrap-balance-003
// draws with an explicit <br>.
//
// The search needs the count to fall as the width grows, and it does: a wider
// line takes at least what a narrower one took.
//
// Returns MaxUnit — no cap at all — when the box does not balance, when it is
// one line already, or when it is longer than this engine will balance.
func (br *Breaker) BalanceWidth(items []Item, width, indent style.Unit) style.Unit {
	full := br.countLines(items, width, indent, MaxBalanceLines+1)
	if full < 2 || full > MaxBalanceLines {
		return style.MaxUnit
	}
	// One unit is the finest distinction the geometry can hold, so the search
	// stops when the bracket is that wide and there is nothing left to choose
	// between.
	lo, hi := style.Unit(1), width
	for hi.Sub(lo) > 1 {
		mid := lo.Add(hi.Sub(lo).Div(2))
		if br.countLines(items, mid, indent, full+1) <= full {
			hi = mid
			continue
		}
		lo = mid
	}
	return hi
}

// capAt is the balanced width for a line beginning at an item.
func capAt(caps []style.Unit, i int) style.Unit {
	if i < 0 || i >= len(caps) {
		return style.MaxUnit
	}
	return caps[i]
}

// BalanceClampedWidth is §5.1's balancing where CSS Overflow 4's clamp has
// already cut the block off: the narrowest width that still shows everything the
// full width showed.
//
// The suite states the rule as a picture rather than as prose, and both of its
// halves are in the diagrams. line-clamp-002 balances "1 2 3 4 5 6 7 8 9 0 1 2"
// into two lines of thirteen characters where the second carries a four-
// character ellipsis — so the ellipsis is part of what is being evened out, not
// something added afterwards. And line-clamp-003 shows *more* text balanced than
// unbalanced: three lines of "1 2 3", "4 5 6", "7 8 9…" against an unbalanced
// "1 2 3 4 5", "6 7 8 9", "…", because the narrower measure lets the last line
// hold something beside the mark.
//
// So the search is over how far into the content the clamped layout reaches,
// and the answer is the narrowest width that reaches as far as the full width
// did. Reaching *further* is fine and is what the third line above does.
func (br *Breaker) BalanceClampedWidth(items []Item,
	width, indent, ellipsis style.Unit, maxLines int) style.Unit {

	wantI, wantByte := br.clampedReach(items, width, indent, ellipsis, maxLines)
	lo, hi := style.Unit(1), width
	for hi.Sub(lo) > 1 {
		mid := lo.Add(hi.Sub(lo).Div(2))
		i, iByte := br.clampedReach(items, mid, indent, ellipsis, maxLines)
		if i > wantI || (i == wantI && iByte >= wantByte) {
			hi = mid
			continue
		}
		lo = mid
	}
	return hi
}

// clampedReach is how far into the items a clamped block gets: the cursor after
// the last line it shows.
//
// The last line is the one the ellipsis sits on, so it is broken in a narrower
// measure than the rest — and it is the one line that does not overflow. A word
// too long for its line is set anyway everywhere else in this engine, because
// the alternative is losing it; here the alternative is exactly what the clamp
// asks for, since what does not fit beside the mark is what the mark stands for.
// "unbreakable" against nine characters less an ellipsis shows nothing at all,
// which is what the suite's line-clamp-003 draws.
func (br *Breaker) clampedReach(items []Item,
	width, indent, ellipsis style.Unit, maxLines int) (int, int) {

	i, iByte := 0, 0
	for n := 0; n < maxLines; n++ {
		for iByte == 0 && i < len(items) && items[i].Float != nil {
			i++
		}
		if i >= len(items) {
			break
		}
		room := width
		if n == 0 {
			room = room.Sub(indent)
		}
		last := n == maxLines-1
		if last {
			room = room.Sub(ellipsis)
		}
		wasI, wasByte := i, iByte
		runs, next, nextByte, _, _ := br.BreakOneLine(items, i, iByte, room, 0)
		if last {
			var used style.Unit
			for _, r := range runs {
				used = used.Add(r.Width)
			}
			if used > room {
				// The breaker only overflows when a single unit left it no
				// choice, so a line wider than its room is one unit that did not
				// fit — and on the clamped line that unit is not shown.
				break
			}
		}
		i, iByte = next, nextByte
		if !CursorAdvanced(wasI, wasByte, i, iByte) {
			break
		}
	}
	return i, iByte
}

// BalanceWidthInBands is §5.1's balancing where a float has shortened some of
// the lines: the same search, over the widths the lines actually had.
//
// The bands come from laying the box out once, which is the only way to know
// them — a float inside the box is placed as the lines are built, and what
// shortens a line is decided by the lines above it. They are the *greedy*
// layout's bands and the balanced one may differ slightly, since a line that
// changes height meets a different set of floats; the difference is a line's
// worth of a float's edge, and browsers make the same approximation.
func (br *Breaker) BalanceWidthInBands(items []Item, bands []style.Unit,
	width, indent style.Unit) style.Unit {

	full := br.countLinesInBands(items, bands, width, indent, MaxBalanceLines+1)
	if full < 2 || full > MaxBalanceLines {
		return style.MaxUnit
	}
	lo, hi := style.Unit(1), width
	for hi.Sub(lo) > 1 {
		mid := lo.Add(hi.Sub(lo).Div(2))
		if br.countLinesInBands(items, bands, mid, indent, full+1) <= full {
			hi = mid
			continue
		}
		lo = mid
	}
	return hi
}

// countLinesInBands is countLines with a width per line rather than one for all
// of them.
//
// A line's room is the narrower of the band it is in and the width being
// probed — the cap chooses break points inside the room the floats leave, it
// does not widen a line past them.
func (br *Breaker) countLinesInBands(items []Item, bands []style.Unit,
	cap, indent style.Unit, limit int) int {

	n := 0
	iByte := 0
	for i := 0; i < len(items); {
		for iByte == 0 && i < len(items) && items[i].Float != nil {
			i++
		}
		if i >= len(items) {
			break
		}
		room := style.Min(bandAt(bands, n), cap)
		if n == 0 {
			room = room.Sub(indent)
		}
		wasI, wasByte := i, iByte
		runs, next, nextByte, _, forced := br.BreakOneLine(items, i, iByte, room, 0)
		if len(runs) > 0 || forced {
			n++
		}
		if n >= limit {
			return n
		}
		i, iByte = next, nextByte
		if !CursorAdvanced(wasI, wasByte, i, iByte) {
			break
		}
	}
	return n
}

// MaxBalancePasses bounds how many times a balanced box beside a float is laid
// out.
//
// Each pass measures the widths the last one produced and balances in them, and
// the two agree after one round for every box whose floats are all above its
// text. Where a float sits part-way down, moving the lines can move the float
// and the answer chases itself; three attempts is where that stops. It is a
// variable so that a test can lower it and watch the bound decide something.
var MaxBalancePasses = 3

// SameUnits reports whether two runs of measurements are the same.
func SameUnits(a, b []style.Unit) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// LineCap is the width this line may be broken at.
//
// Two sources, and the per-line one wins where it has an answer: the scored
// search names a width for each line, and the width search names one for the
// whole box. A line past the end of either is not capped by it.
func LineCap(perItem, perLine []style.Unit, item, line int) style.Unit {
	if line >= 0 && line < len(perLine) {
		return perLine[line]
	}
	if len(perLine) > 0 {
		return style.MaxUnit
	}
	return capAt(perItem, item)
}

// bandAt is the width of the nth line, or of the last one recorded once the
// probe runs past what was measured.
//
// A probe that makes more lines than the layout did is asking about lines that
// were never laid out, and the band below the last float is the best answer
// there is: it is what every line after it had.
func bandAt(bands []style.Unit, n int) style.Unit {
	if len(bands) == 0 {
		return style.MaxUnit
	}
	if n >= len(bands) {
		return bands[len(bands)-1]
	}
	return bands[n]
}

// maxScoredItems bounds the paragraph the scored search will look at.
//
// The search is quadratic in the break opportunities — every position is a state
// and every state enumerates the lines that can start at it — so a long
// paragraph is left to the width search, which is linear and gives the same
// answer wherever the lines all have the same room. Six lines of ordinary prose
// is well under this; a paragraph that is not is one nobody can see the
// balancing of anyway.
var maxScoredItems = 400

// BalanceScoredCaps is §5.1's balancing as a choice between break sets rather
// than as a narrower measure to fill greedily in.
//
// The two are the same question when every line has the same room, and they part
// when a float shortens some of them. The suite's text-wrap-balance-float-001 is
// the case: three lines with sixteen and a half characters of room, sixteen and
// a half, and twenty-three and a half. Filling greedily in a narrower measure can
// reach two arrangements there and no others — thirteen, twelve and eight
// characters, or thirteen, sixteen and four — while the reference is nine, seven
// and seventeen. What that one minimises is the sum of the squares of the space
// left over: 188.75 against 272.75 and 392.75.
//
// So that is what is minimised, over break sets that make the same number of
// lines. The count is a constraint rather than part of the score because
// balancing may not cost a line — a paragraph that grew one to look tidier would
// be balancing at the expense of the thing being balanced.
//
// The square is what makes it *balance* rather than merely fit: it charges one
// line left half empty far more than two lines left a quarter empty each, which
// is the difference a reader sees.
//
// Returns the width to break each line at, or nil where there is nothing to
// choose or too much to choose between.
func (br *Breaker) BalanceScoredCaps(items []Item, bands []style.Unit,
	indent style.Unit, lines int) []style.Unit {

	if lines < 2 || lines > MaxBalanceLines || len(items) > maxScoredItems {
		return nil
	}

	type state struct {
		i, iByte, n int
	}
	type answer struct {
		score      float64
		used       style.Unit
		next       state
		ok, walked bool
	}
	memo := map[state]answer{}

	room := func(n int) style.Unit {
		r := bandAt(bands, n)
		if n == 0 {
			r = r.Sub(indent)
		}
		return r
	}

	var best func(st state) answer
	best = func(st state) answer {
		if got, ok := memo[st]; ok {
			return got
		}
		// Guard against a cycle: a state is marked as being worked on, and a
		// candidate that leads back to it is refused rather than followed. The
		// enumeration below only ever moves the cursor forward, so this cannot
		// fire — it is here because the alternative to refusing is a hang on an
		// untrusted document.
		memo[st] = answer{}
		if st.i >= len(items) {
			out := answer{ok: st.n == lines}
			memo[st] = out
			return out
		}
		if st.n >= lines {
			memo[st] = answer{}
			return answer{}
		}

		out := answer{}
		r := room(st.n)
		for w := r; w >= 0; {
			runs, next, nextByte, _, _ := br.BreakOneLine(items, st.i, st.iByte, w, 0)
			if !CursorAdvanced(st.i, st.iByte, next, nextByte) {
				break
			}
			var used style.Unit
			for _, run := range runs {
				used = used.Add(run.Width)
			}
			rest := best(state{next, nextByte, st.n + 1})
			if rest.ok {
				slack := float64(r.Sub(used).Px())
				score := slack*slack + rest.score
				if !out.ok || score <= out.score {
					out = answer{score: score, used: used, next: state{next, nextByte, st.n + 1}, ok: true}
				}
			}
			// The next candidate is the widest line strictly shorter than this
			// one, which is this one measured a layout unit narrower. The
			// minimum is what keeps it strictly shorter: a line holding a single
			// unit too wide for it is set anyway, so its used width is *more*
			// than the width it was asked for, and stepping down from that would
			// step back up.
			shorter := style.Min(used, w).Sub(1)
			if shorter >= w || shorter < 0 {
				break
			}
			w = shorter
		}
		memo[st] = out
		return out
	}

	first := best(state{0, 0, 0})
	if !first.ok {
		return nil
	}
	caps := make([]style.Unit, 0, lines)
	for st, cur := (state{0, 0, 0}), first; cur.ok && len(caps) < lines; {
		caps = append(caps, cur.used)
		st = cur.next
		if st.i >= len(items) {
			break
		}
		cur = memo[st]
	}
	if len(caps) == 0 {
		return nil
	}
	return caps
}

// countLines is how many lines the greedy breaker makes of these items in a
// given width.
//
// It stops counting at limit, because the caller only ever needs to know whether
// the count is above a number: a two-character probe width over a page of text
// would otherwise break every word in it to answer a question already settled.
//
// Floats are not consulted. Balancing chooses break points within the room the
// content has, and what that room is on each line is decided by the real loop
// against the real bands; a count that placed floats would have to place them
// once per probe and roll them back once per probe.
func (br *Breaker) countLines(items []Item, width, indent style.Unit, limit int) int {
	n := 0
	iByte := 0
	for i := 0; i < len(items); {
		for iByte == 0 && i < len(items) && items[i].Float != nil {
			i++
		}
		if i >= len(items) {
			break
		}
		room := width
		if n == 0 {
			room = width.Sub(indent)
		}
		wasI, wasByte := i, iByte
		runs, next, nextByte, _, forced := br.BreakOneLine(items, i, iByte, room, 0)
		if len(runs) > 0 || forced {
			n++
		}
		if n >= limit {
			return n
		}
		i, iByte = next, nextByte
		if !CursorAdvanced(wasI, wasByte, i, iByte) {
			// The same forward-progress guard the real loop carries. A probe
			// width of one unit is narrower than any glyph, and a breaker that
			// cannot fit even one would otherwise be asked for ever.
			break
		}
	}
	return n
}

// BreakOneLine fills a single line, greedily, and says where the next one
// starts.
//
// Greedy is what browsers do: a line takes what fits and the next one starts
// after it. The alternative — minimising raggedness across a paragraph, which is
// what TeX does — produces better-looking text and needs the whole paragraph
// before any line is settled, which is a different shape of engine.
//
// One line at a time rather than the whole paragraph, because width is no longer
// a property of the paragraph: with a float beside it, each line has its own.
//
// forced reports that the line ended at a break the author wrote, which is what
// makes an empty line real — "a<br><br>b" leaves a blank line, and an engine
// that dropped empty lines would close the gap up.
//
// lineX is where the line box starts within the block's content box, which a
// float beside it makes something other than zero. It is needed because a tab
// stop is measured from the block's edge and not from the line's.
//
// The returned items carry their resolved widths: a tab's is not known until it
// has a place, so an item on a line is not always the item that came in.
func (br *Breaker) BreakOneLine(items []Item, from, fromByte int, width, lineX style.Unit) (
	line []Item, next, nextByte int, outOfFlow []MidLineBox, forced bool) {

	var used style.Unit
	// content says the line holds something a reader would see. It is not the
	// same as the line being non-empty: an inline box's own margin, border and
	// padding take room without being content, and §4.1.2's rule about the
	// *beginning of a line* is about the text. "<span style='margin-left: 5px'>
	// x</span>" sets one space in from five pixels, not from nine.
	content := false
	// A break opportunity that fell on an inline box's leading edge is a break
	// before the box, and the box's margin travels to the next line with the
	// word it pushes along. That decision cannot be taken when the margin is
	// reached: the margin is an item of its own, the word after it is not an
	// opportunity of its own, and it is the *pair* that has to fit. So where the
	// opportunity was is remembered and the line rewinds to it if the word turns
	// out not to fit.
	//
	// Rewinding rather than looking ahead is what keeps this linear: each item is
	// still visited once, and the position is a single index rather than a scan.
	insetAt, insetLine, insetFlow := -1, 0, 0
	// The most recent point at which this line could have ended, kept for
	// break-spaces. §3's break-spaces value puts the soft wrap opportunity
	// *after* every preserved space and nowhere else, so a space belongs to the
	// unit that precedes it: "X XX X" in four characters is "X " and "XX X",
	// because the run "XX " — word and the space that follows it, with no
	// opportunity between them — is three characters wide and does not fit after
	// the first two. Greedy filling has to measure that whole run and not just
	// its first item, and this marker is how: the space is placed if it fits, and
	// sends the line back to the last opportunity if it does not.
	//
	// It is a marker rather than a lookahead for the reason insetAt is: each item
	// is visited once on the way forward and the rewind is a single index, so a
	// paragraph costs one pass however many times a line has to be given back.
	// Progress is guaranteed because the marker is only set once the line holds
	// content, so it is always past the item the line started at.
	oppAt, oppLine, oppFlow := -1, 0, 0
	// Where the white space that ends this line begins.
	//
	// §4.1.2's third and fourth rules are both about white space "at the end of a
	// line": the collapsible part of it is removed and what remains hangs. Neither
	// can happen to white space the line breaks *inside*, and breaking inside it
	// is what a greedy fill does on its own — the run is wider than the room left,
	// so the first opportunity in it ends the line and the rest goes to the next
	// one. What that produces is a line of nothing but spaces, which is precisely
	// the thing the two rules exist to prevent.
	//
	// So the run is found before the fill starts and the fill is told not to break
	// in it. It reaches from the last item that is not white space to the end of
	// the line's material — the next forced break, or the last item there is — and
	// an inline box's own inset is passed over rather than ending it, since a
	// margin is not content and a span wrapped around the spaces must not make
	// them breakable again.
	end := len(items)
	for k := from; k < len(items); k++ {
		if items[k].Forced {
			end = k
			break
		}
	}
	tailFrom := end
	for tailFrom > from && isLineTailSpace(items[tailFrom-1]) {
		tailFrom--
	}
	i := from
	for ; i < len(items); i++ {
		item := items[i]
		if i == from && fromByte > 0 {
			// The line begins part-way through an item, because the line before
			// it ended inside this word. The cursor is an index *and* an offset
			// rather than a rewritten items slice: the caller re-runs this over
			// several band widths, so anything written back would be seen by the
			// next attempt and the split would compound.
			_, item = br.SplitItem(item, fromByte)
		}

		if item.Float != nil {
			// Recorded with how far along the line it was reached, which is what
			// decides whether it goes beside this line or below it.
			outOfFlow = append(outOfFlow, MidLineBox{Box: item.Float, Used: used})
			continue
		}

		if item.Abs != nil {
			// Recorded and otherwise ignored. It consumes no width, so the words
			// on the line are placed exactly as they would have been had the box
			// not been written at all — which is what "out of flow" means and is
			// the assertion a test can make that a float cannot.
			outOfFlow = append(outOfFlow, MidLineBox{Box: item.Abs, Used: used, Abs: true})
			continue
		}

		if item.Forced {
			// An instruction rather than an opportunity: the line ends here
			// whatever room is left, and an empty one still occupies its height.
			return trimLineEdge(line), i + 1, 0, outOfFlow, true
		}

		// §4.1.2's first rule: a sequence of collapsible spaces at the
		// beginning of a line is removed. It is the space the break happened
		// at, or the one the author left after a tag, and keeping it would
		// indent every line after the first.
		//
		// Only a *collapsible* one. Preserved white space at the start of a
		// line is content — it is what makes "<pre>    indented</pre>" indent,
		// and it is the whole of the pre-wrap leading-space rule.
		//
		// The test is on the line's *content* rather than on the line being
		// empty, because an inline box's margin is not content — but no
		// document in the suite can tell the two apart, and that is worth
		// recording rather than leaving as an implied claim. Reaching the
		// difference needs a collapsible space that survives collection sitting
		// immediately after an inline box's leading margin, on a line that
		// begins there. The two requirements contradict each other: a line
		// begins at a margin only when a space preceded the box, and a space
		// before the box is exactly what makes §4.1.1 collapse away the one
		// after its opening tag. Planting "len(line) == 0" here moves nothing —
		// 2973 clean passes either way — so this branch is the correct reading
		// of the rule and has no test, which is a different thing from being
		// covered.
		if item.Collapsible && !content {
			continue
		}

		if item.Tab {
			// The distance to the next tab stop, plus whatever letter-spacing adds
			// after the character — a tab is a character like any other for that
			// purpose, and leaving it out would put the run after a tab a spacing
			// to the left of where it is drawn.
			item.Width = TabAdvance(lineX.Add(used), item.TabStop, item.TabFloor).
				Add(item.Spacing.Letter)
		}

		// A hanging space never causes a break: it sits past the line's end
		// rather than moving to the next one. Without this, "XX    XX" under
		// pre-wrap would push the second word down a line for spaces that take
		// no room on the page at all.
		if !item.NoWrap && !item.Hangs && i < tailFrom && item.BreakBefore &&
			len(line) > 0 && used.Add(item.Width) > width {
			return trimLineEdge(line), i, 0, outOfFlow, false
		}

		// The rewind. The item does not begin a break opportunity of its own,
		// but an inline box opened just before it did, and the pair is what does
		// not fit — so the line ends where the box began and the box's leading
		// margin goes with it.
		if !item.NoWrap && !item.Hangs && i < tailFrom && !item.BreakBefore && !item.Inset &&
			insetAt >= 0 && used.Add(item.Width) > width {
			return trimLineEdge(line[:insetLine]), insetAt, 0, outOfFlow[:insetFlow], false
		}

		// The rewind to the last opportunity. Something that does not fit and
		// cannot begin a line of its own is the tail of the unit before it, so a
		// line that cannot hold it cannot hold that unit either and ends at the
		// last opportunity instead. Where there is no such opportunity it stays
		// and the line overflows.
		//
		// It was written for break-spaces, whose preserved space is exactly that
		// — data, never dropped to make a line fit — and it was restricted to
		// spaces, which was too narrow. "xy <span>ab</span>cdefgh" in seventy-two
		// pixels put everything on one line and let it overflow: "cdefgh" begins
		// no opportunity, because there is none between a span and the text after
		// it, so nothing sent the line back to the space it had.
		//
		// Atomic inlines are still excluded, and that is measured rather than
		// reasoned. Letting an inline-block or an image rewind costs thirty-two
		// reftests, so the suite says the behaviour they have is the right one
		// and this is not the change that should alter it; extending the rule to
		// text alone moves nothing on the suite either way and fixes the case
		// above, which makes it a strict improvement rather than a trade.
		if (item.Space || item.AtomicBox == nil) && !item.Collapsible &&
			!item.Hangs && i < tailFrom && !item.NoWrap && !item.Inset &&
			!item.BreakBefore && oppAt >= 0 && used.Add(item.Width) > width {
			return trimLineEdge(line[:oppLine]), oppAt, 0, outOfFlow[:oppFlow], false
		}

		// A single item wider than the line has nowhere to go. It is placed and
		// overflows — breaking inside a word would be worse, since a word split
		// at an arbitrary point reads as a different word — and it is reported,
		// because the part past the edge is simply not drawn and nothing else
		// about the page says so.
		// overflow-wrap, CSS Text §5.5: the last resort.
		//
		// Its opportunities exist only "if there are no otherwise-acceptable
		// break points in the line", and this is the place that knows: every
		// rewind above has been tried and none applied, so ending the line here
		// is the only way not to overflow.
		//
		// The condition is about the *line* and not about one over-wide item,
		// which is the correction that made this do anything at all. A first
		// draft fired only where a single item was wider than the whole line,
		// and the suite's own fixture is not that shape: "XXXX XX" in four
		// characters cuts into pieces that each fit, and it is the run of them
		// that does not. Requiring no rewind target is what keeps it a last
		// resort — a line with a space in it breaks at the space.
		//
		// That last requirement is the correct reading of the rule and has no
		// test, which is a different thing from being covered. It cannot have
		// one: the two rewinds above return before this is reached, and the
		// branch before them takes any item that begins an opportunity of its
		// own, so what is left to arrive here holding a rewind target is an
		// atomic inline or a collapsible space — and breakInsideWord can cut
		// neither, so the branch declines and the line goes on to the same place
		// it would have reached anyway. Instrumented to count the case, no
		// document in the suite reaches it: zero hits over all 5177 reftests.
		// Dropping the conjunct therefore moves nothing, which is why it is
		// recorded here rather than left as an implied claim.
		if item.BreakWord && !item.NoWrap && !item.Hangs && i < tailFrom && !item.Inset && !item.Tab &&
			insetAt < 0 && oppAt < 0 && used.Add(item.Width) > width {
			// The offset is into items[i]. It is only the cursor's offset away
			// from that when this *is* the item the cursor pointed at: a line
			// that began at a float and reached its first text later is at
			// i > from, where the item is whole.
			base := 0
			if i == from {
				base = fromByte
			}
			// As much of the item as the room left will hold. This is what
			// keeps the fill greedy: a line with "ab" on it and "cdefgh" next
			// takes "cd" as well rather than stopping at the two characters it
			// already has.
			if head, at, ok := br.breakInsideWord(item, width.Sub(used)); ok {
				line = append(line, head)
				return trimLineEdge(line), i, base + at, outOfFlow, false
			}
			// Nothing of it fits — a single character too wide for the room
			// left, or a space, which has one cluster and cannot be cut. The
			// line ends in front of it and it begins the next one, which is
			// where a preserved space under break-spaces has to go: the value
			// exists so that spaces are data, and dropping one to tidy the line
			// would lose it.
			//
			// Only where the line holds something already. Otherwise there is
			// nothing to end and the item would begin this line again for ever;
			// it is placed, it overflows, and it is reported below.
			if content {
				return trimLineEdge(line), i, base, outOfFlow, false
			}
		}

		if item.Width > width && !content && !item.Space && !item.NoWrap && !item.Inset {
			// An inset is not text and has no text to name in the report. A
			// margin wider than the line is also not the fault the report is
			// about — nothing is clipped, the content is simply pushed past the
			// edge, and the box the author wrote is the box that was drawn.
			br.report.ReportOverflow(item, width)
		}
		// Recorded before the switch below, because that is where content becomes
		// true: an opportunity at the very start of a line is not one the line can
		// be sent back to.
		if item.BreakBefore && content {
			oppAt, oppLine, oppFlow = i, len(line), len(outOfFlow)
		}

		switch {
		case item.Inset && item.BreakBefore && content && insetAt < 0:
			// The line could have ended here. Remember enough to come back.
			insetAt, insetLine, insetFlow = i, len(line), len(outOfFlow)
		case !item.Inset:
			// Something that is not a margin has been placed, so the break
			// before the last box is no longer the one to rewind to: there is a
			// nearer opportunity, or none, and either way this one is spent.
			insetAt = -1
			content = true
		}
		line = append(line, item)
		used = used.Add(item.Width)
	}
	return trimLineEdge(line), i, 0, outOfFlow, false
}

func FmtPx(u style.Unit) string {
	return strconvFormat(u.Px()) + "px"
}

// trimLineEdge is §4.1.2's third rule: a sequence of collapsible spaces at the
// end of a line is removed, "as well as any trailing U+1680 OGHAM SPACE MARK
// whose white-space property is normal, nowrap, or pre-line". Both are
// trimAtEnd, which is why the scan reads that rather than collapsible.
//
// CSS Text removes it because it is the space the break happened at: leaving it
// would make a right-aligned line hang, and a centred one sit off-centre by half
// a space.
//
// Preserved white space is *not* removed, and the difference is deliberate: it
// hangs instead, so it stays in the runs, which is what a reader copying text
// out of the page gets. Removing it would silently drop characters the author
// wrote from the document's text, which is the same class of fault as the
// missing spaces that once made "A heading" extract as "Aheading".
//
// What the hanging affects is the break decision — hangs is what stops a
// trailing space pushing the next word down a line — and the alignment, which
// alignedWidth discounts it from so that a centred line does not sit half a
// space off centre. Justification is the one consumer §4.1.2 names that this
// engine still does not have, and textalign.go reports it rather than setting
// justified text ragged in silence.
// An inline box's own margin, border and padding is not text and does not stop
// the rule reaching the space before it: "<span>word </span>" ends the line with
// a space whether or not the span has a margin, and the span's margin is still
// its margin once the space has gone. So the scan looks past an inset and the
// inset is kept.
func trimLineEdge(line []Item) []Item {
	end := len(line)
	for end > 0 && (line[end-1].TrimAtEnd || line[end-1].Inset) {
		end--
	}
	if end == len(line) {
		return line
	}
	// Cutting the capacity keeps the append below from writing over the items
	// after end, which are still the caller's.
	out := line[:end:end]
	for _, item := range line[end:] {
		if item.Inset {
			out = append(out, item)
		}
	}
	return out
}

// isLineTailSpace reports whether an item can be part of the white space that
// ends a line: the space itself, an inline box's own inset, and a box that is
// out of flow.
//
// The last two are there so that a span wrapped around the spaces, or an
// absolutely positioned box written among them, does not break the run in two
// and make the half before it breakable again. Neither is content — §4.1.2's
// rules are about the text — and neither takes the line anywhere.
func isLineTailSpace(item Item) bool {
	if item.Inset || item.Abs != nil {
		return true
	}
	// White space that the end of a line does something to: the third rule
	// removes it, or the fourth hangs it. break-spaces is the value where
	// neither happens — its spaces are data, they take room, and §3 puts an
	// opportunity after every one of them — so a line may end inside a run of
	// them and this must not say otherwise.
	return item.Space && (item.Hangs || item.TrimAtEnd)
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

// ParseNumber reads a bare number, which line-height accepts as a multiplier.
func ParseNumber(s string) (float64, bool) {
	var v float64
	var seenDigit, seenDot bool
	frac := 0.1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			seenDigit = true
			if seenDot {
				v += float64(c-'0') * frac
				frac /= 10
			} else {
				v = v*10 + float64(c-'0')
			}
		case c == '.' && !seenDot:
			seenDot = true
		default:
			return 0, false
		}
	}
	return v, seenDigit
}

// DescribeRune names a character for a diagnostic, by code point as well as by
// its shape — the shape is what the author recognises and the code point is what
// they can search for, and a character with no glyph often cannot be shown at
// all in whatever is reading the report.
func DescribeRune(r rune) string {
	out := "U+" + strings.ToUpper(hex(uint32(r)))
	if unicode.IsPrint(r) {
		out += " (" + string(r) + ")"
	}
	return out
}

func hex(v uint32) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0000"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{digits[v&0xf]}, b...)
		v >>= 4
	}
	for len(b) < 4 {
		b = append([]byte{'0'}, b...)
	}
	return string(b)
}

// UnsupportedScript names why a rune cannot be laid out, or reports false.
//
// The right-to-left scripts used to be here, and are not any more: the
// bidirectional algorithm is applied per paragraph and its reordering per line —
// see bidi.go — so Hebrew, Arabic, Syriac and Thaana are set in the order they
// are read. Shaping them was already forme's, and still is.
func UnsupportedScript(r rune) (string, bool) {
	switch {
	// Scripts with no spaces between words, which need a dictionary to know
	// where a line may break.
	case r >= 0x0E00 && r <= 0x0E7F, // Thai
		r >= 0x0E80 && r <= 0x0EFF, // Lao
		r >= 0x1780 && r <= 0x17FF, // Khmer
		r >= 0x1000 && r <= 0x109F: // Myanmar
		return "this script writes no spaces between words, so finding a line " +
			"break needs a dictionary, which is not implemented; the text would " +
			"run on as one unbreakable word", true
	}
	return "", false
}

// strconvFormat renders a length for a diagnostic, to a tenth of a pixel — more
// precision than that is noise in a message a person reads.
func strconvFormat(v float64) string {
	return strconv.FormatFloat(float64(int(v*10+0.5))/10, 'f', -1, 64)
}

// MissesVisible reports whether a face cannot set some character of text that
// would have put ink on the page.
//
// It is one predicate rather than two because it has two callers that must not
// disagree: the guardrail that reports a missing glyph, and the fallback that
// goes looking for a face which has it. They did disagree, briefly, and the
// result was the fallback substituting a whole different font for a paragraph
// whose only "missing" character was a no-break space — changing every metric on
// the page to fix nothing. Asking the same question twice in two ways is how
// that happens, so it is asked once.
//
// Shaping the whole run first is what keeps it cheap: the answer is almost
// always no, and only then is it worth walking the characters.
func MissesVisible(face *shape.Face, text string) bool {
	if _, missing := face.ShapeGlyphs(text); missing == 0 {
		return false
	}
	for _, r := range text {
		if r == '\n' || r == '\t' || MarksNoPaper(r) {
			continue
		}
		if _, missing := face.ShapeGlyphs(string(r)); missing > 0 {
			return true
		}
	}
	return false
}

// MarksNoPaper reports whether a character is a space by definition.
//
// A face that cannot encode one of these is not a problem to report. The encoder
// substitutes a space for anything it cannot represent, and for a character that
// was never going to put ink down that substitution is either exactly right — a
// no-break space *is* a space, differing only in whether a line may break at it,
// which is settled long before the face is asked — or wrong by a fraction of an
// em, as for the fixed-width spaces whose whole purpose is to be a particular
// width.
//
// The distinction matters because the substitution is not harmless in general. A
// Hebrew letter the face cannot encode also becomes a space, so the word does
// not appear as a row of boxes — it is simply absent, from the page and from the
// text extracted out of it. That is worth an error. A no-break space becoming a
// space is not, and reporting it was the most common finding this engine
// produced: 154 documents in the reftest suite raised it for U+00A0 alone.
//
// # Why the format characters are not listed here
//
// They were, and the list could not be observed. A planted defect that deleted
// the whole format-character branch — soft hyphen, the zero-width spaces, the
// bidi embeddings and isolates, the byte order mark — broke nothing, and the
// reason is that shaping already answers "not missing" for every one of them,
// on both kinds of face. A simple face encodes through WinAnsi and drops them;
// a composite face shapes them to no glyph and no advance, which is what they
// are for. Measured on Ahem: every one reports missing=0, and so does the
// no-break space, because a composite face has a real glyph for it.
//
// So only the space separators are here, and only the simple faces need them —
// which is to say the fourteen standard PDF faces, which is what a document gets
// unless a caller supplies something else.
func MarksNoPaper(r rune) bool {
	switch {
	case r == 0x00A0, // no-break space
		r == 0x1680,                // ogham space mark
		r >= 0x2000 && r <= 0x200A, // en quad through hair space
		r == 0x202F,                // narrow no-break space
		r == 0x205F,                // medium mathematical space
		r == 0x3000:                // ideographic space
		return true
	}
	return false
}

// SplitItem cuts one text item in two at a byte offset, re-measuring both.
//
// The bidi range goes with the text. An item carries the span of the paragraph
// buffer its characters occupy, and the two halves occupy the two halves of it —
// which is what keeps the resolved levels attached to the right characters after
// a word has been broken across a line.
//
// Re-measuring rather than apportioning the original width is the point. A face
// may kern or ligate across the cut, so the two pieces do not in general add up
// to the whole, and the number that has to be right is the one used to place the
// text that is actually drawn.
func (br *Breaker) SplitItem(item Item, at int) (head, tail Item) {
	head, tail = item, item
	head.Text, tail.Text = item.Text[:at], item.Text[at:]
	// at is an offset into the string, and the bidi range counts runes: the
	// paragraph the levels were resolved over is a []rune, and bidiStart is a
	// position in it. Adding the byte offset to it is right for Latin and wrong
	// for everything that needs the algorithm at all — a Hebrew letter is two
	// bytes, so a word cut in half moved the range twice as far as the text, and
	// the tail then read its level from a position two characters past its own.
	//
	// What that looks like is "אבגדהו12" in an RTL block narrow enough to cut the
	// word: the "12" belongs to the left of the letters and was drawn to the
	// right of them, on the line the tail begins, while the same text unbroken
	// orders correctly.
	runesBefore := utf8.RuneCountInString(item.Text[:at])
	head.BidiEnd = item.BidiStart + runesBefore
	tail.BidiStart = item.BidiStart + runesBefore
	head.Width = br.MeasureSpaced(item.Face, head.Text, item.Size, item.Spacing)
	tail.Width = br.MeasureSpaced(item.Face, tail.Text, item.Size, item.Spacing)
	// The tail begins a line, so it takes no opportunity from what was in front
	// of the head — there is nothing in front of it any more.
	//
	// Nothing reads it: a tail is always the first item of the line it starts, and
	// there an opportunity is neither recorded (that wants content before it) nor
	// acted on (the break in front of one wants a line with something on it). It
	// is cleared because leaving it would make the field state something untrue
	// about where the item now sits, not because a document can tell.
	tail.BreakBefore = false
	return head, tail
}

// breakInsideWord is overflow-wrap's last resort: the largest prefix of an item
// that fits, cut where a grapheme cluster ends.
//
// The cut is at a cluster boundary and not at a character, for the reason
// break-all's is — CSS Text §2 puts a soft wrap opportunity between typographic
// character units, and a cut inside one separates a letter from its accent. It
// is the same rule and the same table; only when it applies differs.
//
// The prefix is found by bisection over the boundaries rather than by measuring
// the text a cluster at a time. Widths do not add up: a face may kern or ligate
// across the join, so a running total is not the width of the prefix it claims
// to be. Bisection measures each candidate whole, which is exact for the one
// that is chosen, and costs a logarithmic number of measurements rather than
// one per character — which matters, because the input is untrusted and this is
// reached precisely for the longest words in a document.
//
// It reports false when there is nothing to gain: an item with one cluster, or
// one whose first cluster already overflows. Both leave the word to overflow and
// be reported, which is right — a line cannot hold less than one character, and
// breaking one off to leave the rest overflowing anyway would only lose a
// character off the end.
func (br *Breaker) breakInsideWord(item Item, width style.Unit) (head Item, at int, ok bool) {
	if !item.BreakWord || item.Face == nil || width <= 0 || item.Text == "" {
		return Item{}, 0, false
	}
	bounds := segment.Boundaries(nil, item.Text)
	if len(bounds) == 0 {
		return Item{}, 0, false // one cluster: nothing to cut
	}

	// The largest boundary whose prefix fits. Bisection needs the predicate to
	// be monotone, and it is for any face whose advances are non-negative: a
	// longer prefix is never narrower. A face with a negative advance would make
	// this pick a cut that is merely *a* fitting one rather than the longest,
	// which is a worse line and not a wrong page.
	lo, hi := 0, len(bounds) // lo is known to fit (the empty prefix), hi is not known
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if mid > len(bounds) {
			break
		}
		w := br.MeasureSpaced(item.Face, item.Text[:bounds[mid-1]], item.Size, item.Spacing)
		if w <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return Item{}, 0, false // not even one cluster fits
	}
	at = bounds[lo-1]
	head, _ = br.SplitItem(item, at)
	return head, at, true
}
