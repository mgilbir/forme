package layout

import (
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Block layout: the fifth of §3's stages, turning boxes into fragments with a
// resolved position and size in absolute page coordinates.
//
// What is here is the block formatting context of CSS 2.1 §9.4.1 and §10 — the
// widths, the box model, margin collapsing, and where the out-of-flow boxes of
// §9.5 go. Inline layout is next door in inline.go, which is a division by
// formatting context rather than by convenience: a block container's children
// are all block-level or all inline-level, never a mixture, so the two walks
// never interleave. Floats are the one thing that crosses the line, because a
// float can be written among either and its geometry is read by both.
//
// # Why margin collapsing is here and not later
//
// It is the part of block layout that cannot be added afterwards. Collapsing
// changes where every subsequent box goes, so an engine that positioned boxes
// first and collapsed margins second would have to move everything it had
// placed — and the rules about *which* margins collapse depend on borders and
// padding that are themselves being resolved. Doing it in the same pass is what
// keeps it a single traversal.

// Fragment is a box with a resolved geometry.
//
// A box produces one fragment here. It will produce more than one when content
// can break — a paragraph across two columns, an inline across two lines — which
// is why this is a fragment rather than simply a laid-out box, and why the type
// is named for the thing that can be plural.
type Fragment struct {
	// Box is what generated this fragment.
	Box *Box

	// BorderRect is the border box in absolute page coordinates: the area a
	// background paints and a border draws on.
	BorderRect Rect

	// Margin, Border and Padding are the resolved edges. They are kept rather
	// than folded into rectangles because painting needs them separately — a
	// border is drawn on its own width — and because the guardrails ask about
	// them by name.
	Margin, Border, Padding Edges

	// Outline is the used width of CSS 2.1 §18.4's outline: the ring drawn just
	// outside the border edge.
	//
	// It is here and not read from the style at paint time because it is a
	// length, and a length in ems needs the box's font size to become a number.
	// Zero when there is no outline to draw, which is the ordinary case and is
	// what keeps the paint pass cheap.
	Outline style.Unit

	Children []*Fragment

	// Lines is the inline content of a block container, in the same coordinates
	// as its children.
	//
	// A fragment has children or lines, with one exception: a float is out of
	// flow, so a block whose in-flow content is entirely inline can still have
	// floated children beside those lines. The anonymous box rule deliberately
	// does not wrap a float, because wrapping it would put it in a different
	// formatting context from the text meant to run around it.
	Lines []LineFragment

	// Marker is the bullet or number a list item generates, nil otherwise. It
	// is on the fragment rather than in the box tree because its text depends
	// on the item's position among its siblings, which is not a property of the
	// box.
	Marker *Marker

	// collapsed is the grid lines of a table using §17.6.2's collapsing border
	// model, in coordinates relative to this fragment's border box.
	//
	// They hang off the table rather than off the cells because that is whose
	// borders they are: a collapsed border is centred on a grid line and belongs
	// half to the cell on each side, so neither cell can draw it.
	collapsed []collapsedBand
	// inCollapsedGrid marks a fragment whose own border is part of that grid —
	// the table and every cell in it — and so must not be painted here.
	//
	// It is separate from the list because a table with no borders anywhere still
	// must not fall back to painting the one it declared: that border took part
	// in the conflict resolution like every other candidate, and losing is not
	// the same as being absent.
	inCollapsedGrid bool
	// contentH is the border-box height this fragment's content came to, before
	// a declared height was allowed to raise it.
	//
	// It is recorded for one caller and one rule. §17.5.3 makes a declared height
	// on a table cell a *minimum* — the cell box is at least that tall — and then
	// aligns the cell's *content* within whatever height its row ended up with.
	// Those are two different heights, and reading the second off the box gave a
	// cell with "height: 1in" no slack at all: its content already filled it, so
	// "vertical-align: bottom" had nothing to move and the text stayed at the top
	// of a box four times its size.
	contentH style.Unit

	// background is this box's background images, resolved against its geometry
	// and in painting order — CSS lists layers front to back, so the last one
	// written is the first one painted.
	background []bgPaint
	// bgColorRect is where the background *colour* goes, which is the painting
	// area of the bottom layer and so is the border box by default rather than
	// the padding box. An empty rectangle means no colour is painted.
	bgColorRect Rect
	// bgBands are the rectangles this box's background is painted in, instead of
	// over its whole box. An empty slice means the ordinary single rectangle.
	//
	// §17.5.1's separated borders model is the one thing that needs it. A row,
	// column, row group or column group there is a box drawn *behind the cells
	// it holds* — and the border-spacing between those cells is not part of it,
	// because the space between cells is the table's own background showing
	// through. The suite says so by drawing the expected column as one solid
	// block and then knocking white stripes out of it at every gap.
	//
	// Bands rather than a smaller box, because the area is not a rectangle: a
	// column crosses every row and the gaps between them, and a row crosses
	// every column and the gaps between those. And bands that clip rather than
	// bands that are painted, because a background image is positioned against
	// the whole box and only *shown* through the cells — an image tiled per band
	// would start afresh in each one, which is a different picture.
	bgBands []Rect
	// bgSuppressed marks the box whose background became the canvas's, so that
	// it is not also painted over its own smaller box. See §2.11.2: the element
	// the background was taken from is left with the initial values.
	bgSuppressed bool

	// canvas, canvasLayers and canvasColor are set on the root fragment only.
	//
	// The canvas is the page area the document was laid out into, which is what
	// a fixed-attachment layer is positioned against and what the propagated
	// root background is painted over. canvasColor is the box the colour is
	// taken from, which is the root or its <body> and is nil when neither
	// declared one.
	canvas       Rect
	canvasLayers []bgPaint
	canvasColor  *Box

	// clipSelf and clipContent are CSS 2.1 §11.1: the area this box's own
	// background and border may paint, and the area everything inside it may.
	//
	// They are two values because the two rules differ. "overflow" clips a
	// box's *contents* to its padding box and leaves the box itself alone, so a
	// wide border on an "overflow: hidden" element is still drawn; "clip" cuts
	// the element's own rendered content as well. See visualeffects.go.
	//
	// The zero value clips nothing, which is what a fragment built by hand — or
	// by a Layout that ran before this stage existed — gets. That is the right
	// default for a display list: an unresolved clip must not silently hide a
	// box, because a box that is not drawn is far harder to notice than one
	// that is drawn too large.
	clipSelf, clipContent Clip

	// Offset is CSS 2.1 §9.4.3's relative displacement: how far the box is drawn
	// from where the flow put it.
	//
	// It is carried rather than folded into BorderRect because folding it in
	// during layout would be the classic way to get relative positioning wrong.
	// A relatively positioned box still *occupies* its original space, so every
	// question layout asks afterwards — where the next sibling goes, how tall
	// the parent is, which band a line box has — must be answered against the
	// unoffset position. Applying the offset in absolutise, which visits each
	// fragment once and already translates each subtree by its parent's origin,
	// moves the box and everything inside it and moves nothing else.
	Offset Point
}

// ContentRect is the area the children were laid out in.
func (f *Fragment) ContentRect() Rect {
	return f.BorderRect.Inset(f.Border.Add(f.Padding))
}

// PaddingRect is the border box minus its border.
func (f *Fragment) PaddingRect() Rect { return f.BorderRect.Inset(f.Border) }

// MarginRect is the border box plus its margin, which is the space the box
// actually occupies in its parent's flow.
func (f *Fragment) MarginRect() Rect { return f.BorderRect.Outset(f.Margin) }

// Layout positions a box tree inside an available width, returning the root
// fragment.
//
// avail is the content box of the page: what §11 calls the page box minus its
// margins. Only the width constrains layout — the height is what comes out, and
// whether it fits is §5's scale-to-fit question, asked afterwards.
//
// set supplies the faces; a nil one uses the fourteen standard faces, which need
// no embedding and cover Latin.
func Layout(root *Box, avail Size, set FontSet, rec *Recorder) *Fragment {
	if root == nil {
		return nil
	}
	l := &layouter{
		rec: rec, avail: avail,
		lengths:          map[lengthKey]style.Length{},
		fonts:            map[fontKey]resolvedFont{},
		textFaces:        map[*Box]*shape.Face{},
		reportedScripts:  map[string]bool{},
		reportedGlyphs:   map[string]bool{},
		reportedOverflow: map[string]bool{},
		decorations:      map[*Box][]textDecoration{},
		backgrounds:      map[*Box][]backgroundLayer{},
		reportedOnce:     map[string]bool{},
		inlineDraws:      map[*Box]bool{},
		inlineChains:     map[*Box][]*Box{},
		inlineOffsets:    map[*Box]Point{},
		inlineAligns:     map[*Box]vAlignState{},
		intrinsic:        map[*Box]intrinsicWidths{},
		grids:            map[*Box]*tableGrid{},
		tableDemands:     map[*Box][]tableColumnDemand{},
		collapsed:        map[*Box]*collapsedGrid{},
		positioned:       map[*Box]*Fragment{},
		fontSet:          set,
		rootFontSize:     root.FontSize,
		root:             root,
	}
	// The breaker reports through the layouter, so it is made once the layouter
	// exists rather than in the literal above.
	l.br = newBreaker(l)
	if l.rootFontSize == 0 {
		l.rootFontSize = defaultFontSize
	}
	if l.fontSet == nil {
		l.fontSet = StandardFonts()
	}
	// The root box establishes the outermost block formatting context, so no
	// float in the document can escape the page. The context handed in here is
	// therefore a placeholder that nothing will ever be put in — block() makes a
	// fresh one for any box that establishes a context, and the root is one.
	// The initial containing block is the page's content box: this engine has
	// one page whose size is settled before layout runs, so unlike a viewport it
	// has a definite height, and a percentage resolved against it is a number
	// rather than an indefinite value that computes to auto.
	page := Rect{W: avail.W, H: avail.H}
	if root.Position.outOfFlow() {
		// The root taken out of the flow is the one box the deferring walk below
		// cannot reach: it is where layout starts, so nothing walks *to* it and
		// there is no parent to record a static position against. It is placed
		// here instead, through the same function every other out-of-flow box
		// goes through, against a stand-in for the initial containing block —
		// which is the page, and which is also the containing block §10.1 gives
		// it, since it has no ancestor at all and so certainly no positioned one.
		//
		// Its static position is the page's origin for the same reason: there is
		// no flow it was taken out of, so the place it "would have been" is where
		// layout would have started.
		//
		// This used to be laid out in the flow with its offsets dropped and a
		// warning raised. The warning was honest and the page was still wrong:
		// "html { position: absolute; left: 100px }" put the document at the top
		// left corner. abspos-containing-block-initial-004a, -004b, -004c, -004d
		// and -009b are five documents that do exactly that.
		icb := &Fragment{BorderRect: page}
		l.layoutAbsolute(absCandidate{box: root, parent: icb}, page)
		l.placeAbsolutes(page)
		if len(icb.Children) == 0 {
			return nil
		}
		frag := icb.Children[0]
		l.resolveBackgrounds(frag, page)
		l.resolveClips(frag)
		return frag
	}
	frag, m := l.block(root, avail.W,
		flow{ctx: &floatContext{}, cbHeight: avail.H, cbDefinite: true})

	// Layout runs in coordinates relative to each parent's content box, because
	// a box's position is not known until its margins have finished collapsing
	// with its descendants' — and that is only settled after its subtree is
	// laid out. Absolute coordinates are then one pass over the finished tree.
	//
	// The alternative, translating each subtree as it is placed, costs a walk
	// per level; this costs one walk in total.
	// A floated root element has no containing block to be shifted within
	// except the page itself, and no siblings to make room for — so all that is
	// left of §9.5 for it is which edge it goes against. Its width already
	// shrank to fit, and leaving the position alone would honour half of the
	// declaration and quietly drop the visible half.
	if root.Float == FloatRight {
		frag.BorderRect.X = avail.W.Sub(frag.Margin.Right).Sub(frag.BorderRect.W)
	}

	absolutise(frag, 0, m.top.value())

	// Everything out of flow is placed now, against a tree that is already in
	// page coordinates. That ordering is the whole design of position.go: an
	// absolutely positioned box resolves against an *ancestor's* box, which does
	// not exist as a rectangle until this point, and it may do so because
	// nothing in the flow depends on it in return.
	l.placeAbsolutes(page)

	// Backgrounds last, because a background image is placed against a rectangle
	// rather than taking part in deciding one: a percentage position is of a box
	// whose width is settled by its children, and a fixed-attachment layer is
	// placed against the page. Both are known only now, and neither changes
	// anything the walk above computed.
	l.resolveBackgrounds(frag, page)

	// Clipping last of all, because §11.1's rectangles are final ones: a
	// padding box in page coordinates, for boxes that were positioned after
	// everything else. It changes no geometry — §11.1 is about painting and
	// never about layout, and a clipped box still occupies every inch of the
	// space it did — so nothing computed above depends on it.
	l.resolveClips(frag)
	return frag
}

type layouter struct {
	rec   *Recorder
	avail Size

	// lengths memoizes parsing a computed value into a Length.
	//
	// The cascade stores computed values as text, so laying out a document
	// re-parses "1em" once per box per property — a thousand elements with ten
	// box properties each is ten thousand tokenizer runs of a string that is
	// almost always one of a handful. The key includes the font size because
	// that is what an em resolves against.
	lengths map[lengthKey]style.Length
	// textFaces memoizes the face a text box is actually set in, which is not
	// the family's face when the family cannot cover the text. See faceForText.
	textFaces map[*Box]*shape.Face
	// restrictedFamilies memoizes whether a font-family list can resolve to a
	// face carrying a unicode-range, keyed by the list as written. It is what
	// keeps the per-cluster family walk off every document that has no such
	// descriptor, which is almost all of them.
	restrictedFamilies map[string]bool
	// br is the half of inline layout that is about text rather than boxes, and
	// it owns the memo of measured runs. See breaker.
	br *breaker
	// fonts memoizes the face a style resolves to, which is the other thing
	// inline layout asks for most.
	fonts map[fontKey]resolvedFont
	// intrinsic memoizes the two content-based widths of a box, which are what
	// a float with an auto width is sized by.
	intrinsic map[*Box]intrinsicWidths
	// grids and tableDemands memoize the two expensive answers about a table:
	// where its cells sit in the grid, and what each column asks for. Both are
	// wanted once while the table's width is being resolved and again while it
	// is laid out, so a table nested n deep would cost 2^n without them.
	grids        map[*Box]*tableGrid
	tableDemands map[*Box][]tableColumnDemand
	// tablePercentBase is what a percentage width on a table resolves against.
	//
	// §17.4 makes it the *wrapper's* containing block rather than the wrapper,
	// which is the one thing about a table's width that cannot be answered where
	// the table is laid out: by then the containing block is the wrapper, and the
	// wrapper is as wide as the table. So the wrapper records it on the way past.
	tablePercentBase map[*Box]style.Unit
	// collapsed memoizes §17.6.2's resolved grid lines, for the same reason and
	// with one more: every cell asks for it while its own border is resolved, so
	// without the memo a table would re-run the conflict resolution once per
	// cell.
	collapsed map[*Box]*collapsedGrid
	// deferred holds the out-of-flow boxes met during the walk, to be placed
	// once the tree is in absolute coordinates. See position.go for why they
	// can wait and floats cannot.
	deferred []absCandidate
	// positioned maps each positioned box to its fragment, which is how an
	// absolutely positioned box finds the containing block §10.1 gives it. It is
	// a map rather than a walk up the fragment tree because a fragment does not
	// know its parent — layout builds downwards — and giving it one would add a
	// pointer to every fragment to answer a question a handful of boxes ask.
	positioned map[*Box]*Fragment
	// fontSet is where faces come from.
	fontSet FontSet
	// rootFontSize is the font-size of the root element, which is what "rem"
	// resolves against. It is settled once, before the walk: the point of rem is
	// that it does not compound as elements nest, so reading it from the box in
	// hand — as this did — makes it a synonym for em.
	rootFontSize style.Unit
	// root is the box layout began at, kept because §8.3.1's "margins of the
	// root element's box do not collapse" is the one rule in the collapsing
	// model that is about *which* box rather than about what the box declares.
	// Identity is the right question to ask and an element name is not: this
	// package is handed hand-built trees by its own tests, and a tree whose top
	// box is a <div> still has a root.
	root *Box
	// relayouts counts the subtrees that had to be laid out a second time
	// because the position predicted for them turned out to be wrong. It is
	// bounded: see maxRelayouts.
	relayouts int
	// reportedScripts and reportedGlyphs suppress repeating a complaint that is
	// about a script or a character rather than about a place.
	reportedScripts map[string]bool
	// reportedWordBreak is the same for the word-break values read as normal.
	reportedWordBreak map[string]bool
	// reportedAutospace is the same for the text-autospace values not applied.
	reportedAutospace map[string]bool
	// reportedHyphens is the same for the hyphens values read as manual.
	reportedHyphens map[string]bool
	// reportedHanging is the same for the hanging-punctuation values not applied.
	reportedHanging map[string]bool
	// reportedLineBreak is the same again for line-break.
	reportedLineBreak map[string]bool
	// reportedTextJustify is the same again for text-justify, and is reported
	// only where a line is being justified — see reportTextJustify.
	reportedTextJustify map[string]bool
	reportedGlyphs      map[string]bool
	// reportedOverflow suppresses repeating one run's overflow per line.
	reportedOverflow map[string]bool
	// decorations memoizes the text decorations drawn across each box, which are
	// built from the box's parent's — see decorationsFor.
	decorations map[*Box][]textDecoration
	// backgrounds memoizes the seven background properties read into layers,
	// which is one tokenizer run per property per box and is asked for twice
	// when a box's background is propagated to the canvas.
	backgrounds map[*Box][]backgroundLayer
	// reportedOnce suppresses repeating a finding about a *value* — a
	// stylesheet rule rather than a box — so a gradient on four hundred
	// elements is one thing to be told. Keyed by whatever tells two of them
	// apart. See reportOnce.
	reportedOnce map[string]bool
	// inlineDraws memoizes whether an inline box has a background or a border to
	// paint, and inlineChains the chain of such boxes above another box. Both are
	// asked once per item per line, which is the hottest loop in the engine, and
	// both answer "nothing" for almost every box in an ordinary document.
	inlineDraws  map[*Box]bool
	inlineChains map[*Box][]*Box
	// inlineFragments are the fragments a *positioned* inline box produced, in
	// line order. §10.1 forms the containing block of an absolutely positioned
	// descendant from the first and last of them — see inlineContainingBlock.
	inlineFragments map[*Box][]*Fragment
	// inlineOffsets is §9.4.3's accumulated displacement at each inline box that
	// has one, which is what its background and border are drawn at. It is
	// recorded by the walk that computes it because nothing downstream can:
	// collectInline flattens the boxes away, and the offset an inline box's own
	// fragment needs is its own rather than that of the item that reached it.
	//
	// Only a box a relative position actually moved is recorded, so the map stays
	// empty in a document with no positioned inline in it.
	inlineOffsets map[*Box]Point
	// inlineAligns is §10.8.1's accumulated vertical-align at each inline box
	// that has one, recorded for the same reason and read by the same code: an
	// inline box's own background and border move with the box, and the box is
	// gone by the time the fragment for them is made.
	//
	// It is the box's own accumulation rather than an item's, which is the whole
	// reason it is a map and not a field on the item. An item carries the sum
	// down to the innermost box it sits in; a fragment for a box halfway up that
	// chain needs the sum down to *itself*.
	//
	// Only a box that vertical-align actually moved is recorded, so the map stays
	// empty in a document that never uses the property.
	inlineAligns map[*Box]vAlignState
	// inlineDecorations counts the fragments made for those backgrounds and
	// borders, which is what maxInlineDecorations bounds, and inlineDecorCapped
	// says the bound has already been reported.
	inlineDecorations int
	inlineDecorCapped bool
	// clamps is the stack of line-clamp containers being laid out, innermost
	// last. It is on the layouter because CSS Overflow 4 counts *descendant*
	// line boxes, which no one block container can see. See clamp.go.
	clamps []*lineClamp
}

type lengthKey struct {
	value    string
	fontSize style.Unit
	// metrics is part of the key because "ex", "ch" and "ic" resolve against the
	// face, so two boxes at the same size in different fonts do not share an
	// answer. Leaving it out would have made the first font to parse "40ch"
	// decide it for every other — a memoization bug, which is the kind that
	// produces a wrong page only when a document uses two faces.
	metrics faceMetrics
}

// faceMetrics are the measurements the font-relative units need from whatever
// face will set a box: the x-height for "ex", the advance of "0" for "ch" and
// the advance of "水" for "ic".
//
// Each carries a "known" bit beside it because zero is an answer for none of
// them and a fallback for all three, and the fallbacks differ: half an em, no
// answer at all, and one em. A face that states no x-height and a face that
// states zero are different things, and four bugs in this engine have come from
// reading a zero as an answer.
//
// It is a value, and comparable, because it is part of the memoization key.
type faceMetrics struct {
	xHeight      style.Unit
	xHeightKnown bool
	zeroAdvance  style.Unit
	zeroKnown    bool
	icAdvance    style.Unit
	icKnown      bool
}

// metricsFor gathers whichever of the three a value asks for, and only those.
//
// The face is resolved only for a value that needs it. Asking for one has side
// effects — fontFor reports a fallback when the requested family is missing — so
// resolving it for every length would make a box that merely declares an
// unavailable font report it without ever setting any text. It is also a
// measurement per property, on the hot path of layout.
func (l *layouter) metricsFor(b *Box, raw string) faceMetrics {
	var m faceMetrics
	if usesUnit(raw, 'c', 'h') {
		m.zeroAdvance, m.zeroKnown = l.zeroAdvance(b)
	}
	if usesUnit(raw, 'e', 'x') {
		m.xHeight, m.xHeightKnown = l.xHeightOf(b)
	}
	if usesUnit(raw, 'i', 'c') {
		m.icAdvance, m.icKnown = l.icAdvance(b)
	}
	return m
}

// collapsed is what a box contributes to its parent's flow once its own margins
// have collapsed with its descendants'.
//
// through marks a box with nothing to separate its own two margins — no border,
// no padding, no content, no height — so they collapse into one another and the
// box's neighbours end up adjoining each other through it. An empty <div>
// between two paragraphs is the everyday case, and an engine that misses it puts
// a gap where the author sees none.
type collapsed struct {
	top, bottom marginRun
	through     bool
	// topAlone is the run on the box's *top* edge before a box that collapses
	// through merged its two edges into one. It is the part of that run which
	// still sits above the border edge when §9.5.2 puts a clearance under it,
	// and it is meaningless — and left zero — for a box that does not collapse
	// through, because then the two edges never merged.
	topAlone marginRun
}

// marginRun is a set of adjoining margins, carried as the two numbers §8.3.1's
// rule needs rather than as the single value it produces.
//
// The rule is written over the whole set at once:
//
//	the maximum of the absolute values of the negative adjoining margins is
//	deducted from the maximum of the positive adjoining margins
//
// and folding it two at a time gives a different answer, because the maximum of
// a set is not recoverable from a value that has already had a negative taken
// off it. 16px, then 96px, then -96px is 96 - 96 = 0 over the set; folded from
// the right it is collapse(16, collapse(96, -96)) = collapse(16, 0) = 16.
//
// That is not a corner. It is the shape of every test that puts a negative
// margin under an empty box: margin-bottom-103 and -104 in the suite lay a
// paragraph's own em of bottom margin, an empty box's 50%, and a negative inch
// end to end, and expect them to come to nothing — the two-at-a-time answer
// leaves the em standing and the black rule an em low.
type marginRun struct {
	pos, neg style.Unit
	// cleared records that a box whose own margins collapsed through had a
	// clearance put under them, and that this run is what §9.5.2 calls the
	// margin resulting from collapsing that box's margins with its following
	// siblings'. The sentence that needs it is the one after:
	//
	//	that resulting margin does not collapse with the bottom margin of the
	//	parent block
	//
	// so a run carrying this flag is committed inside the parent rather than
	// hoisted out through its bottom edge, and the parent is that much taller.
	// It is on the run rather than on the box because the rule is about where
	// the run ends up, and the run outlives the box: it goes on collecting the
	// margins of every following sibling that collapses through.
	cleared bool
}

// marginOf is the run one margin makes on its own.
func marginOf(u style.Unit) marginRun {
	if u < 0 {
		return marginRun{neg: u}
	}
	return marginRun{pos: u}
}

// merge is the union of two sets of adjoining margins. It is associative and
// commutative, which is what folding a run of them requires and what taking the
// value between steps loses.
func (m marginRun) merge(o marginRun) marginRun {
	return marginRun{
		pos:     style.Max(m.pos, o.pos),
		neg:     style.Min(m.neg, o.neg),
		cleared: m.cleared || o.cleared,
	}
}

// add merges in one more margin.
func (m marginRun) add(u style.Unit) marginRun { return m.merge(marginOf(u)) }

// value is the width the collapsed margin has on the page.
func (m marginRun) value() style.Unit { return m.pos.Add(m.neg) }

// absolutise turns positions relative to each parent's content box into absolute
// page coordinates, in one pass over the finished tree.
//
// It is also where §9.4.3's relative offset is applied, and the two belong
// together: the offset is a translation of a whole subtree, which is precisely
// what this walk already performs at every level. Adding it here means a
// relatively positioned box carries its descendants with it for free and moves
// nothing else at all, because by the time this runs every position in the flow
// has already been decided against the unoffset geometry.
func absolutise(f *Fragment, x, y style.Unit) {
	if f == nil {
		return
	}
	f.BorderRect.X = f.BorderRect.X.Add(x).Add(f.Offset.X)
	f.BorderRect.Y = f.BorderRect.Y.Add(y).Add(f.Offset.Y)
	// The bands are in the same coordinates the border box was, so they take the
	// same translation. See Fragment.bgBands.
	for i := range f.bgBands {
		f.bgBands[i].X = f.bgBands[i].X.Add(x).Add(f.Offset.X)
		f.bgBands[i].Y = f.bgBands[i].Y.Add(y).Add(f.Offset.Y)
	}

	content := f.ContentRect()
	// The inline boxes' own backgrounds and borders, which hang off the line
	// boxes rather than off the children: they are in the same coordinates the
	// lines are, so they take the same translation. Their §9.4.3 offset is folded
	// in here for the reason the walk applies every other one here — it moves the
	// box and nothing that was measured against it.
	for i := range f.Lines {
		for _, ib := range f.Lines[i].Boxes {
			ib.BorderRect.X = ib.BorderRect.X.Add(content.X).Add(ib.Offset.X)
			ib.BorderRect.Y = ib.BorderRect.Y.Add(content.Y).Add(ib.Offset.Y)
		}
	}
	for _, c := range f.Children {
		absolutise(c, content.X, content.Y)
	}
}

// block lays out a block-level box at a position, inside a containing width,
// and returns its fragment.
//
// The position given is the top left of the *border box*: the caller has
// already decided where the margin puts it, because deciding that is what
// margin collapsing does and only the caller can see both sides of a collapse.
// at is where the box's own border box sits inside the formatting context: the
// containing block's content left edge, and the flow position the caller has
// reached. The caller cannot supply the box's own left edge because that depends
// on margins which may be auto, and auto margins are resolved here.
func (l *layouter) block(b *Box, containing style.Unit, at flow) (*Fragment, collapsed) {
	return l.blockIn(b, containing, at, nil)
}

// blockIn is block layout with the option of having the box's width, margins and
// height decided by the caller instead of by its own declarations.
//
// The only caller that supplies them is the absolute placement of position.go,
// whose §10.3.7 constraint resolves a width against a containing block this walk
// cannot see. Routing it through the same function rather than giving it a
// layout of its own is deliberate: margin collapsing, floats, line breaking,
// list markers and the height rules are identical for an absolutely positioned
// box, and a second implementation of them would agree with this one on the day
// it was written and on no day after.
func (l *layouter) blockIn(b *Box, containing style.Unit, at flow,
	forced *forcedGeometry) (*Fragment, collapsed) {

	// The two values this engine understands and does not act on for a box of
	// this shape. Both are asked once per box here rather than by a pass of their
	// own, because this is the one function every block-level box goes through.
	l.checkTableBoxSizing(b)
	l.checkIntrinsicSizing(b)

	margin := l.edges(b, "margin", containing)
	if b.Inner == InnerTable {
		// §17.4 moved the margins to the wrapper. Reading them here as well
		// would apply them twice: the wrapper would be indented and the table
		// would be indented again inside it.
		margin = Edges{}
	}
	border := l.borderWidths(b)
	padding := l.paddingOf(b, containing)

	// A replaced box is sized from its content rather than from its containing
	// block, on both axes at once, and the answer is then handed to the
	// ordinary block arithmetic as though the author had declared it. That is
	// exactly what CSS 2.1 §10.3.4 and §10.6.5 say to do — the margin rules for
	// a block-level replaced element are the same ones as for any other block,
	// applied to a width that came from somewhere else.
	var replaced *Size
	if b.Replaced != nil {
		s := l.replacedSize(b, containing, at.cbHeight, at.cbDefinite)
		replaced = &s
	}

	width := l.resolveWidth(b, margin, border, padding, containing, &margin, replaced)
	declaredHeight, hasHeight := l.explicitHeight(b, containing, at.cbHeight, at.cbDefinite)
	if replaced != nil {
		declaredHeight, hasHeight = replaced.H, true
	}
	if forced != nil {
		margin, width = forced.margin, forced.width
		if forced.hasHeight {
			declaredHeight, hasHeight = forced.height, true
		}
	}

	// A box that establishes a block formatting context seals both its edges.
	// CSS 2.1 §8.3.1 puts it the other way round — the margins of such a box do
	// not collapse with its in-flow children — but the effect is the same as a
	// border of zero width that a margin cannot cross, and expressing it here
	// keeps the collapsing rules in one place. Leaving it out is what would make
	// a floated <div> holding a <p> sit an em above where the author put it.
	//
	// The root element is sealed for a second reason and by the same mechanism.
	// §8.3.1 says flatly that "margins of the root element's box do not
	// collapse", and it is the one entry in the collapsing model that is about
	// which box this is rather than about what it declares — so it is asked by
	// identity. Without it "html { margin-top: 1em }" over a body whose first
	// child also has one draws a single em rather than two, which is
	// margin-collapse-020 exactly: the green bar lands an em high and the red
	// the reference covers is left showing.
	sealed := establishesBFC(b) || b == l.root

	// A margin collapses through an edge only when nothing sits on that edge to
	// stop it. A border or a padding of even one unit is something.
	topOpen := border.Top == 0 && padding.Top == 0 && !sealed
	// The bottom edge also needs the height to be the content's own: a declared
	// height is a floor the margin cannot reach across.
	bottomOpen := border.Bottom == 0 && padding.Bottom == 0 && !hasHeight && !sealed

	// Whether the box's *own* two margins are adjoining to each other, which is a
	// different question from whether either of them is adjoining a child's, and
	// §8.3.1 gives it a different answer. Its list of adjoining pairs asks for
	// 'auto' where a parent's bottom margin meets its last child's:
	//
	//   bottom margin of a last in-flow child and bottom margin of its parent if
	//   the parent has 'auto' computed height
	//
	// and for 'auto' *or zero* where a box's own two meet:
	//
	//   top and bottom margins of a box that does not establish a new block
	//   formatting context and that has zero computed 'min-height', zero or
	//   'auto' computed 'height', and no in-flow children
	//
	// Reading one condition for both is the zero-versus-unset mistake in its
	// usual shape, and it is not a corner: "height: 0" is how the suite writes a
	// box that exists to contribute a margin and nothing else, and treating it as
	// a floor leaves that margin uncollapsed on both sides. The suite's
	// margin-collapse-016 and -019 put such a box between two others and check
	// where the second one lands; with the height read as a barrier it lands two
	// ems low, and the red the reference covers is left showing.
	ownMarginsAdjoin := border.Vertical() == 0 && padding.Vertical() == 0 && !sealed &&
		(!hasHeight || declaredHeight == 0)

	frag := &Fragment{
		Box:     b,
		Margin:  margin,
		Border:  border,
		Padding: padding,
		Outline: l.outlineWidth(b),
		BorderRect: Rect{
			// Relative to the parent's content box; absolutise fixes it later.
			X: margin.Left,
			W: width.Add(padding.Horizontal()).Add(border.Horizontal()),
		},
	}
	if b.Position == PositionRelative {
		frag.Offset = l.relativeOffset(b, containing, at.cbHeight, at.cbDefinite)
	}
	// And the offsets of any inline this block was broken out of, which move it
	// exactly as they move the rest of what that inline contains. See splitFrom.
	for _, from := range b.splitFrom {
		if from.Position != PositionRelative {
			continue
		}
		d := l.relativeOffset(from, containing, at.cbHeight, at.cbDefinite)
		frag.Offset.X = frag.Offset.X.Add(d.X)
		frag.Offset.Y = frag.Offset.Y.Add(d.Y)
	}
	if b.Position.positioned() {
		// Recorded even for a box that is only relatively positioned, because
		// §10.1 makes any positioned ancestor a containing block — that is the
		// entire reason the "position: relative with no offsets" wrapper is an
		// idiom rather than a no-op.
		l.positioned[b] = frag
	}

	// Where this box's children are laid out, in the coordinates of the
	// formatting context they belong to.
	//
	// The root box gets a context of its own even though it does not seal its
	// margins: a float has to be contained by the page whatever else is true,
	// and the root's relationship to the canvas — which is what its margin
	// behaviour is about — is a separate question this does not touch.
	//
	// The containing block a child resolves a percentage offset against is this
	// box's content box, and whether its height is *definite* is a separate
	// question from what the height turns out to be: a box sized by its content
	// has no height to take a percentage of while its children are still being
	// laid out, and CSS 2.1 makes such a percentage compute to auto rather than
	// to the number the content later happens to produce.
	// The height a percentage inside this box resolves against, which is this
	// box's *used* height and not the one it declared. §10.5 resolves a
	// percentage against "the height of the containing block", and §10.7's
	// minimum and maximum are applied before there is one: a box at
	// "height: 200px; max-height: 100px" is a hundred pixels tall, so a child at
	// "height: 100%" is a hundred and not two.
	//
	// Only where the height is definite at all. A maximum on its own does not
	// make one — the box is still as tall as its content, and CSS 2.1 makes a
	// percentage against that compute to auto — so the clamp is applied to a
	// declared height and not to the absence of one.
	//
	// No document can tell, and that is worth recording rather than leaving as
	// an implied claim. cbDefinite travels beside cbHeight and is false here
	// either way, and Length.Resolve refuses a percentage without it — so a
	// number put in cbHeight for a box with no height is never read. The guard
	// is what makes the *value* honest: clamping a height that does not exist
	// would hand down a number the box is not, and the day a reader takes
	// cbHeight without asking whether it is definite, this is what stops it
	// being wrong.
	childHeight := declaredHeight
	if hasHeight {
		childHeight = l.clampHeight(b, declaredHeight, containing, at.cbHeight, at.cbDefinite)
	}
	inner := flow{
		ctx:        at.ctx,
		x:          at.x.Add(margin.Left).Add(border.Left).Add(padding.Left),
		y:          at.y.Add(border.Top).Add(padding.Top),
		cbHeight:   childHeight,
		cbDefinite: hasHeight,
		carriedTop: at.carriedTop,
	}
	own := at.ctx
	if sealed || b.Parent == nil {
		own = &floatContext{}
		inner = flow{ctx: own, cbHeight: childHeight, cbDefinite: hasHeight}
	}

	contentHeight, hoistTop, hoistBottom, placedAnything :=
		l.clampedChildren(b, frag, width, topOpen, bottomOpen, inner)

	// What the content itself came to, which a declared height may raise the box
	// above but does not change. The float rule below applies to it either way:
	// a float inside a box that contains its own is part of what that box holds,
	// whether or not a height was declared. See Fragment.contentH.
	natural := contentHeight
	if own != at.ctx {
		natural = style.Max(natural, own.bottom())
	}

	if hasHeight {
		if b.Inner == InnerTable || b.Inner == InnerTableCell {
			// §17.5.3: a declared height on a table or a cell is a *minimum*.
			// Content is never cut off to honour it, which is the opposite of
			// what a declared height does to an ordinary block — and is why a
			// table with "height: 1px" is as tall as its rows rather than a
			// hairline with its rows spilling out.
			contentHeight = style.Max(contentHeight, declaredHeight)
		} else {
			contentHeight = declaredHeight
		}
	} else if own != at.ctx {
		// CSS 2.1 §10.6.7: the auto height of a box that establishes a block
		// formatting context reaches the bottom of the floats inside it. This is
		// the entire reason "overflow: hidden" is the idiom for containing a
		// float, and an ordinary block deliberately does not do it — a float in
		// a plain <div> hangs out of the bottom, which looks like a bug and is
		// the specified behaviour.
		contentHeight = style.Max(contentHeight, own.bottom())
	}
	minHeight, hasMinHeight := l.verticalLength(b, "min-height", at.cbHeight, at.cbDefinite)
	// What the content came to, before a minimum or a maximum is applied to it.
	// Comparing the two is how the box learns that it was made taller than what
	// is inside it, which decides whether its last child's bottom margin still
	// reaches its bottom edge. See below.
	contentNeeded := contentHeight
	if b.Replaced == nil {
		// A replaced box's height has already been through §10.4's constraint
		// table, which resolves the two axes together to keep the picture's
		// shape. Clamping it again here would undo that on one axis and leave
		// the other, which is how an image comes out squashed rather than
		// merely small.
		contentHeight = l.clampHeight(b, contentHeight, containing, at.cbHeight, at.cbDefinite)
	}

	frag.BorderRect.H = contentHeight.Add(padding.Vertical()).Add(border.Vertical())
	frag.contentH = natural.Add(padding.Vertical()).Add(border.Vertical())

	out := collapsed{top: marginOf(margin.Top), bottom: marginOf(margin.Bottom)}
	if topOpen {
		out.top = out.top.merge(hoistTop)
	}
	// §8.3.1 makes two margins adjoining only where nothing separates them, and
	// a minimum that raised this box above its own content is something: the last
	// child's bottom margin edge is where the content ended, and the box's bottom
	// edge is lower down, so the two do not meet and do not collapse.
	//
	// The specification's list asks only for an "'auto' computed height" on this
	// pair, and a min-height leaves the computed height auto — read to the letter
	// the margin escapes however tall the minimum made the box. That reading is
	// what this did, and it is wrong on the suite by a clear margin: min-height
	// on a parent is how an author reserves a band, and a child's bottom margin
	// escaping through it moves everything below the parent down by a distance
	// the author put *inside* it. margin-collapse-min-height-001 is 550px of it,
	// and the same section's sentence about a non-zero min-height stopping a
	// child's bottom margin collapsing with its parent's says which way the rule
	// is meant to fall, even though its own precondition is the narrower case
	// where the parent's *top* margin is in the collapse too.
	//
	// Only a minimum that actually bound counts. Where the content is taller than
	// the minimum the margin reaches the edge exactly as it did before, and where
	// a maximum cut the box down the child is overflowing rather than being held
	// off the edge — hence the comparison is against what the content needed and
	// not against whether a minimum was declared.
	raisedByMinimum := hasMinHeight && contentHeight > contentNeeded
	if bottomOpen && !raisedByMinimum {
		out.bottom = out.bottom.merge(hoistBottom)
	}

	// A box collapses through when there is nothing at all between its two
	// margins: no border, no padding, no height that keeps them apart, no height
	// its content needed, and no minimum keeping it open.
	if ownMarginsAdjoin && !placedAnything && frag.BorderRect.H == 0 &&
		!(hasMinHeight && minHeight > 0) {
		both := out.top.merge(out.bottom)
		return frag, collapsed{top: both, bottom: both, through: true, topAlone: out.top}
	}
	return frag, out
}

// children lays out a box's children and reports what came of them: the content
// height, the margins hoisted out through each open edge, and whether anything
// with a size was actually placed.
//
// The walk keeps a *pending* margin rather than adding gaps as it goes. Every
// margin it meets joins that one run, and the run is only committed to the flow
// when something with a size arrives — which is what lets an empty box between
// two paragraphs disappear entirely instead of separating them.
//
// The run is a marginRun and not a number, because §8.3.1's rule is over the
// whole set of adjoining margins at once and folding it two at a time gives a
// different answer. See marginRun.
func (l *layouter) children(b *Box, parent *Fragment, width style.Unit,
	topOpen, bottomOpen bool, origin flow) (height style.Unit,
	hoistTop, hoistBottom marginRun, placed bool) {

	if b.Inner == InnerTable {
		// A table's children are not a flow at all: they are the grid, and §17.5
		// places them from the columns and rows rather than by stacking them.
		// The two hoisted margins are zero because a table seals both its edges,
		// and something was placed because a table occupies its own height
		// whether or not any cell has content in it.
		return l.tableContent(b, parent, width, origin), marginRun{}, marginRun{}, true
	}
	if len(b.Children) == 0 && !markerInside(b) {
		// An inside marker is content the box did not have to be given: §12.5.1
		// makes it the first inline box in the principal block box, so a list item
		// with nothing in it still has a line to put it on. Without the exception
		// the empty item is zero-tall and paints no background, which is what a
		// dozen of the suite's "does this property apply to a list item" tests are
		// built to show.
		return 0, marginRun{}, marginRun{}, false
	}
	// A block container's in-flow children are either all block-level or all
	// inline-level — the anonymous box rule guarantees it — so this is a two-way
	// choice rather than a mixture. Floats are the reason this is a scan rather
	// than a look at the first child: a float is block-level and out of flow, so
	// "<div><span class=float></span>text</div>" has a block-level child *and*
	// an inline formatting context, and deciding from the first child would lose
	// the text entirely.
	if hasInlineChild(b) || markerInside(b) {
		// Inline content: lines of text, which have a height of their own. An
		// inside marker is inline content of the item's own, which is why it can
		// answer here for a box that has no inline child — or no child at all.
		h := l.inlineContent(b, parent, width, origin)
		// Whether anything was placed is whether a *line* came of it, and not
		// whether there were inline children to try. §8.3.1's self-collapsing
		// box "does not contain a line box", and a box holding nothing but
		// collapsible white space contains none: the space is removed at both
		// edges of the line it would have been on, and no line is made.
		//
		// Answering "yes, there were children" instead is the difference between
		// <div><span style="position:absolute"></span></div> written on one line
		// and the same thing written on three. The markup in a document is
		// indented, so the second is what documents actually contain — and it
		// stopped the box collapsing through, which moved everything after it
		// down by the margin that should have collapsed away. The suite has four
		// of them in margin-collapse-113 to -116, where a box holding one
		// absolutely positioned or floated child is written across lines and the
		// bands below it come out an em low.
		return h, marginRun{}, marginRun{}, len(parent.Lines) > 0
	}

	var y style.Unit
	var pending marginRun
	hoisted := false
	// The floats that existed before this box began. Anything added after it is
	// inside this box's own subtree and moves when this box moves — which is
	// what decides whether a margin can carry a cleared child past a float. See
	// where the hypothetical position is worked out.
	outsideFloats := origin.ctx.mark()
	// listIndex counts the list items among the children, which is what a
	// numbered marker is numbered by. It counts only list items, so a heading
	// between two items does not advance the numbering.
	listIndex := 0

	for _, child := range b.Children {
		if child.Outer != OuterBlock {
			continue
		}
		if l.clampReached() {
			// Past the clamp point, so this child and everything after it is
			// "fragmented away and neither rendered nor measured" — which is
			// why the walk stops here rather than skipping the child: an
			// out-of-flow box after the clamp point is discarded with the rest,
			// and a list item after it does not advance the numbering of the
			// items that are shown.
			break
		}
		if child.ListItem {
			listIndex++
			if !child.ListNumbered {
				// No "list-item" counter answered for this box, so its position
				// among its parent's list items is the number. This is the only
				// place in the engine that knows it, and it is written onto the box
				// rather than passed along because the marker is worked out in
				// several places and at several depths — inside the box's own first
				// line, beside its border box, and again for a floated item met
				// among the words — and three of those had no index to pass.
				//
				// Idempotent, which matters: a box may be laid out twice when its
				// width has to be guessed and then read, and the second pass
				// recomputes the same count.
				child.ListValue = listIndex
			}
		}

		if child.outOfFlow() {
			// A float's margin box goes at the flow position, and its margins
			// collapse with nothing — so the pending margin is committed for it
			// without being consumed, and the box after it still collapses with
			// the box before it as though the float were not there. That is what
			// makes a float between two paragraphs leave their spacing alone.
			//
			// The exception is a parent whose top edge is still open and has
			// hoisted nothing: the pending margin is on its way out of the
			// parent altogether, so the float sits at the parent's content top.
			offset := pending.value()
			if topOpen && !hoisted {
				offset = 0
			}
			if child.Position.outOfFlow() {
				// The same position, kept rather than used: this is the *static*
				// position of §10.6.4, where the box's margin box would have
				// started had it been in the flow, and it is the answer §10.3.7
				// falls back on when neither offset on an axis was given. It can
				// only be recorded here, because nothing after the walk knows
				// where in the flow the box was written.
				// The hypothetical box is block-level and so fills the parent's
				// content width: its left margin edge is at the parent's content
				// left edge and its right margin edge at the parent's content
				// right edge, which is what makes both static positions nought.
				l.deferAbsolute(child, parent, 0, y.Add(offset), 0, listIndex)
				continue
			}
			parent.Children = append(parent.Children,
				l.floatChild(child, width, origin, y.Add(offset), style.MaxUnit, 0, listIndex))
			continue
		}

		// Where the child's border box is predicted to land, so that the floats
		// inside it can be placed while it is being laid out.
		//
		// It has to be a prediction: the child's own collapsed top margin is not
		// known until its subtree has been walked, because a descendant's margin
		// can escape through its top edge. The prediction uses the child's own
		// margin and is therefore exact unless a descendant's is larger — and
		// what happens when it is not exact is settled below rather than left to
		// be approximately right.
		est := y
		if !topOpen || hoisted {
			est = y.Add(pending.add(l.ownTopMargin(child, width)).value())
		}
		est = est.Add(l.clearanceAt(child, origin, est))

		// §9.5's other rule, and the one an engine can omit without the page
		// looking broken: a box that establishes a block formatting context may
		// not *overlap* a float, so it is put beside the float and narrowed, or
		// dropped below it when it cannot be narrowed enough. Predicted here for
		// the same reason the position is, and re-asked at the settled position
		// below.
		estDrop, estGeom := l.avoidFloats(child, width, origin, est, 0, false)
		est = est.Add(estDrop)

		mark, consulted := origin.ctx.mark(), origin.ctx.consulted
		absMark := len(l.deferred)
		cf, cm := l.blockIn(child, width, origin.at(est), estGeom)
		// Whether the *subtree* read the float geometry, captured before the
		// clearance query below adds a read of its own.
		//
		// Over-attributing that read would not move anything — the second
		// layout happens at the corrected position and produces what the
		// translation would have — so this is about cost rather than about
		// geometry, and a planted defect that sets it to true is invisible in
		// the output by design. What it would cost is a redundant layout of
		// every cleared subtree in the document, and a spurious "stopped short"
		// once those exhausted the budget.
		subtreeRead := origin.ctx.consulted != consulted

		// What was pending before this child's own top margin joined it. It is
		// needed because clearance separates the two: see the hoist below.
		before := pending
		pending = pending.merge(cm.top)

		// Whether the parent's top edge is still open with nothing committed
		// inside it, so that a margin met here is on its way out of the parent
		// rather than into it.
		escapes := topOpen && !hoisted

		// The position §9.5.2 measures clearance against: where the box would
		// have gone had it not cleared anything. With the parent's edge open and
		// nothing placed, that is the parent's content top, because the margin
		// would have left through the edge and taken the parent with it — it
		// moves the two together and neither apart.
		//
		// A box whose own margins collapse through is the exception §9.5.2 makes
		// in its own parenthesis, "including the case where the element's
		// margins collapse through, in which case its bottom margin is also
		// included": such a box is measured with its whole run whether the
		// parent's edge is open or not. Leaving it out is not academic — it is
		// the difference between clearing a float and not clearing it at all,
		// and no-clearance-adjoining-opposite-float and
		// no-clearance-due-to-large-margin-after-left-right in the suite are
		// exactly that case, a 150px and a 185px top margin that carry a cleared
		// empty box past the float so that nothing is drawn at all.
		// §9.5.2 measures clearance against where the box would have gone had it
		// not cleared anything — with 'clear' none, so with its top margin
		// collapsing and leaving through the parent's open edge exactly as any
		// other margin would. The whole run counts: the margin moves the parent
		// and the box together, and the floats it is being measured against move
		// with neither.
		// A float placed inside this box is adjoining in the sense the suite's
		// adjoining-float-before-clearance means: the margin that would carry
		// the cleared box past it would move the float too, because both are
		// inside the box the margin is leaving through. Its own assert says what
		// follows — "if the clearance candidate would pull a float down with it
		// (due to margin collapsing) if there were no clearance, clearance needs
		// to be inserted to separate the two [...] No matter how large the
		// margin is, it should still be just below the float."
		//
		// So against such a float the margin buys nothing and the hypothetical
		// position is measured without it. Against a float that was already
		// there before this box started, the margin moves this box relative to
		// it and the run counts in full.
		adjoining := origin.ctx.mark() > outsideFloats
		hypothetical := y.Add(pending.value())
		if escapes && adjoining && !cm.through {
			hypothetical = y
		} else if escapes {
			hypothetical = hypothetical.Sub(origin.carriedTop)
		}
		// Measured against the floats that were there before this child was laid
		// out. Its own are in the context by now and are not floats it clears:
		// see clearanceOver.
		clearance := l.clearanceOver(child, origin, hypothetical, mark)

		// Where the border edge lands, which is not the same question.
		//
		// With clearance, §9.5.2's first rule puts the edge level with the
		// bottom of the float and the margin is spent getting there — that is
		// what clearanceAt returns, measured from this box's own offset, so the
		// edge is that offset plus it.
		//
		// Without clearance the margin is doing the moving. Where it goes
		// depends on whether the parent's edge is open: through it, in which
		// case the parent moves and the box stays at the offset the flow has
		// reached — adding the run here as well would count it twice — or into
		// the parent, in which case it is this box that moves.
		edge := hypothetical.Add(clearance)
		if clearance == 0 && escapes {
			// No clearance, and the margin leaves through the parent's open
			// edge: it moves the parent, and this box stays where the flow had
			// reached. Adding the run here as well would count it twice.
			edge = y
		}

		if cm.through && clearance == 0 {
			// Nothing separates this box's own margins, so it contributes no
			// height and its two margins join the run. It still gets a
			// position, because it still exists — and a relatively positioned
			// one is a containing block, so where it sits is visible even
			// though nothing of it is drawn.
			//
			// The run needs nothing added to it here. A box that collapses
			// through reports the *same* run as both its top and its bottom —
			// see the return above, which merges the two and hands back one
			// value twice — and the top was merged into pending before this
			// branch was reached. Merging the bottom as well was here, and a
			// plant that removed it changed no test in the suite and no test in
			// this package, because it was merging a run into itself.
			at := y.Add(pending.value())
			if escapes {
				// The run leaves through the parent's open edge, so it moves the
				// parent and this box stays where the flow had reached. Adding
				// it here as well counts it twice — which is the same rule the
				// branch below states for a box that does not collapse through,
				// and it was stated there and not here.
				//
				// It is invisible until something asks where the box is. An
				// empty "position: relative" wrapper round an absolutely
				// positioned box is exactly that question, and the box came out
				// one whole collapsed margin below the content it was meant to
				// cover.
				at = y
			}
			cf = l.settle(child, width, origin, cf, est, at, mark, absMark, subtreeRead)
			cf.BorderRect.Y = at
			if child.ListItem {
				cf.Marker = l.markerFor(child, cf, origin)
			}
			parent.Children = append(parent.Children, cf)
			continue
		}

		if escapes {
			// The parent's top edge is open, so everything collected so far
			// belongs outside the parent rather than inside it.
			//
			// Except this child's own top margin, when the child has clearance.
			// CSS 2.1 §8.3.1 lists what makes two margins adjoining and puts
			// clearance beside a border and a padding in the list of things that
			// separate them, so a cleared box's top margin is not adjoining its
			// parent's and cannot escape through it. What can still escape is
			// whatever was pending before this child — those margins belong to
			// earlier children that collapsed through, and a later box's
			// clearance says nothing about them.
			//
			// Leaving it out is not a rounding. A cleared box whose own margin
			// escapes takes its parent with it: the parent is placed by that
			// margin and the child is placed against the float, so the two move
			// apart by exactly the margin. The suite's clear-on-parent-with-
			// margins pairs a "clear: left" box with a "margin-top: -1000px"
			// child and lands the wrapper eight hundred pixels above the page.
			hoistTop = pending
			if clearance != 0 || estDrop != 0 {
				hoistTop = before
			}
			pending = marginRun{}
			hoisted = true
		}

		at := edge

		if cm.through {
			// A box whose own margins collapse through, with a clearance under
			// them. §9.5.2:
			//
			//	If the top and bottom margins of an element with clearance are
			//	adjoining, its margins collapse with the adjoining margins of
			//	following siblings but that resulting margin does not collapse
			//	with the bottom margin of the parent block.
			//
			// The run is therefore neither lost nor laid twice. The part of it
			// on the box's top edge is already above the border edge, with the
			// clearance under it; the rest goes on collecting following
			// siblings' margins below the edge, which is why the flow carries on
			// from the edge less that part rather than from the edge itself.
			//
			// margin-collapse-clear-012 works the arithmetic out in a comment in
			// its own source and is the reason this is expressed as a
			// subtraction: a 40px top margin, an 80px bottom margin, a following
			// sibling with 140px under it and a 100px float to clear give a
			// parent 100 + (140 - 40) = 200px tall.
			//
			// The second half of the sentence is why the parent is that tall at
			// all rather than 100: the run is committed inside it instead of
			// leaving through its bottom edge, which is what marginRun.cleared
			// carries to the end of the walk.
			cf = l.settle(child, width, origin, cf, est, at, mark, absMark, subtreeRead)
			cf.BorderRect.Y = at
			if child.ListItem {
				cf.Marker = l.markerFor(child, cf, origin)
			}
			parent.Children = append(parent.Children, cf)
			y = at.Sub(cm.topAlone.value())
			pending = cm.bottom
			pending.cleared = true
			// The clearance is a real separation, so the parent can no longer
			// collapse through itself and swallow it.
			placed = true
			continue
		}

		// The float-avoidance question again, now that the position is settled.
		// A prediction that turned out wrong moved the box into a different
		// band, and a box that had to be narrowed to fit one band is the wrong
		// width for another — so this counts as having read the geometry and
		// forces the second layout rather than the translation.
		atDrop, atGeom := l.avoidFloats(child, width, origin, at, 0, false)
		at = at.Add(atDrop)
		if !sameForced(estGeom, atGeom) {
			subtreeRead = true
		}
		cf = l.settleIn(child, width, origin, cf, est, at, mark, absMark, subtreeRead, atGeom)
		// The same question again, now that the box has a height. See
		// fitBesideFloats: the two answers differ exactly when a float begins
		// below the box's top and inside its height, which is the case the band
		// at a single y cannot see.
		at, cf = l.fitBesideFloats(child, width, origin, cf, at, atGeom, mark, absMark)
		parent.Children = append(parent.Children, cf)

		y = at
		cf.BorderRect.Y = y
		if child.ListItem {
			// After the box has its position, not before. An item with no line
			// of its own takes its marker's place from where a line *would*
			// have started, and that question is asked against the floats at
			// this y — see firstLineStart.
			cf.Marker = l.markerFor(child, cf, origin)
		}
		y = y.Add(cf.BorderRect.H)
		pending = cm.bottom
		placed = true
	}

	// The top edge is served first, and the order is the rule rather than a
	// detail of the code.
	//
	// When every child collapsed through, nothing was ever committed and the
	// whole run belongs outside — and it leaves through the *top*, because that
	// is the edge it is adjoining: the first child's top margin is adjoining the
	// parent's, and the run reached the bottom only by collapsing through every
	// child on the way. CSS 2.2 §8.3.1 says what follows, in the sentence it
	// added over 2.1:
	//
	//   If the top margin of a box with non-zero computed 'min-height' and
	//   'auto' computed 'height' collapses with the bottom margin of its last
	//   in-flow child, then the child's bottom margin does not collapse with the
	//   parent's bottom margin.
	//
	// which is exactly this box: a min-height is what keeps the parent from
	// collapsing through itself, and so the only reason the question can be
	// asked at all. With the bottom served first the run left through the bottom
	// and the top got nothing, so the parent stayed where it was and the margin
	// appeared under it — margin-collapse-min-height-002 in the suite, where the
	// white square lands half a square high and the red the reference covers is
	// left showing.
	//
	// Nothing else can reach this: both edges open with nothing placed means no
	// border, no padding, no declared height and no content, so the box's height
	// is its min-height, and a min-height of zero would have made it collapse
	// through before this line was reached.
	if topOpen && !hoisted {
		hoistTop = hoistTop.merge(pending)
		pending = marginRun{}
	}
	if bottomOpen && !pending.cleared {
		// The bottom edge is open too, so the trailing margin belongs outside —
		// unless §9.5.2 has already said it does not. A run that collected the
		// margins of a box with clearance "does not collapse with the bottom
		// margin of the parent block", so it stays inside and the parent grows
		// by it. See marginRun.cleared.
		hoistBottom = pending
		pending = marginRun{}
	}
	return y.Add(pending.value()), hoistTop, hoistBottom, placed
}

// hasInlineChild reports whether a box has any in-flow inline-level child, which
// is what makes it an inline formatting context.
func hasInlineChild(b *Box) bool {
	for _, c := range b.Children {
		if c.Outer == OuterInline {
			return true
		}
	}
	return false
}

// at moves the flow position without changing the containing block.
func (f flow) at(y style.Unit) flow {
	return flow{
		ctx: f.ctx, x: f.x, y: f.y.Add(y),
		cbHeight: f.cbHeight, cbDefinite: f.cbDefinite,
	}
}

// ownTopMargin is a box's own declared top margin, before anything collapses
// into it.
func (l *layouter) ownTopMargin(b *Box, containing style.Unit) style.Unit {
	v, _ := l.lengthOf(b, "margin-top", containing)
	return v
}

// clearanceAt returns how far down a box has to move to clear the floats it
// asked to clear, given where it would otherwise have sat.
//
// CSS 2.1 §9.5.2 computes clearance as a difference rather than as a position,
// and the distinction is the whole rule: a box that was already below the floats
// gets no clearance at all, so "clear: left" on a paragraph well down the page
// changes nothing. An implementation that simply moved the box to the float
// bottom would push it *up* in that case.
func (l *layouter) clearanceAt(b *Box, origin flow, at style.Unit) style.Unit {
	return l.clearanceOver(b, origin, at, -1)
}

// clearanceOver is clearanceAt measured against only the floats the context held
// before this box was laid out. See floatContext.clearanceOver for why the bound
// is needed and what it costs to leave it out.
func (l *layouter) clearanceOver(b *Box, origin flow, at style.Unit, floats int) style.Unit {
	if b.Clear == ClearNone {
		return 0
	}
	want := origin.ctx.clearanceOver(b.Clear, floats)
	have := origin.y.Add(at)
	if want > have {
		return want.Sub(have)
	}
	return 0
}

// floatChild lays a float out and places it in the formatting context.
//
// room is how much of the current line is still free and drop is how far down
// the next line begins. The two only mean anything for a float met part-way
// along a line; a float met between blocks passes MaxUnit and zero, because
// there is no line for it to be squeezed off the end of.
//
// The float is laid out *before* it is placed, and that order is not an
// accident: a float establishes its own formatting context, so nothing outside
// it can reach inside, and its size therefore does not depend on where it ends
// up. Placing first would need a size that is not known yet.
func (l *layouter) floatChild(b *Box, width style.Unit, origin flow,
	top, room, drop style.Unit, index int) *Fragment {

	cf := outOfClamp(l, func() *Fragment {
		f, _ := l.block(b, width, origin.at(top))
		return f
	})
	if b.ListItem {
		cf.Marker = l.markerFor(b, cf, origin)
	}

	box := cf.MarginRect()
	size := Size{W: box.W, H: box.H}

	// §9.5.1 rules 5 and 6 in one line: no higher than the flow has reached, and
	// no higher than the floats this box was told to clear.
	at := origin.y.Add(top)
	if size.W > room {
		// Rule 9, in the form browsers apply it: a float met after a line has
		// begun goes beside the rest of that line when it fits, and below the
		// line when it does not. This engine does not re-break the text already
		// placed on the line, so what it gives up is the case where a wide float
		// should have pushed earlier words of the same line down with it.
		at = at.Add(drop)
	}
	if c := origin.ctx.clearance(b.Clear); c > at {
		at = c
	}

	rect := origin.ctx.place(size, b.Float, at, origin.x, origin.x.Add(width))

	// Back out of the formatting context's coordinates into the parent's content
	// box, which is what every fragment position in this engine is relative to.
	cf.BorderRect.X = rect.X.Sub(origin.x).Add(cf.Margin.Left)
	cf.BorderRect.Y = rect.Y.Sub(origin.y).Add(cf.Margin.Top)
	return cf
}

// maxRelayouts bounds how many subtrees one render may lay out twice.
//
// It is a variable rather than a constant so that a test can lower it far enough
// to watch the bound fire. A cap that has only ever been observed not to trip is
// one nobody knows works, which this repository has learned before: the first
// test for the box cap built five thousand boxes, nowhere near the limit, and
// passed just as happily with the cap removed.
var maxRelayouts = 4096

// settle corrects a child that was laid out at a predicted position.
//
// The prediction is described where it is made. When it was right, which is the
// overwhelmingly common case, this does nothing at all. When it was wrong there
// are two repairs, and which one applies is decided by whether the subtree ever
// *read* the float geometry:
//
//   - If it only added floats, every one of them is wrong by the same constant,
//     and moving them is exactly equivalent to having laid the subtree out in
//     the right place. Nothing else in the subtree depends on the offset.
//   - If it read the geometry — a line shortened, a float placed beside another,
//     a clearance computed — then the answer it got was against the wrong band,
//     and no translation repairs that. The subtree is laid out again from the
//     corrected position, which is now exact because the collapsed top margin
//     that the prediction was missing is known.
//
// The second repair is bounded, because a corrected parent re-lays its children
// and each of those may correct again. Past the bound the cheap repair is used
// and the render says it stopped short, which is a page that is slightly wrong
// and says so rather than one that took unbounded time to be right.
func (l *layouter) settle(child *Box, width style.Unit, origin flow, cf *Fragment,
	predicted, actual style.Unit, mark, absMark int, read bool) *Fragment {
	return l.settleIn(child, width, origin, cf, predicted, actual, mark, absMark, read, nil)
}

// settleIn is settle with the geometry §9.5 forced on a box that had to avoid a
// float, so that the second layout is done against the band the box ended in
// rather than against the one it was predicted to be in.
func (l *layouter) settleIn(child *Box, width style.Unit, origin flow, cf *Fragment,
	predicted, actual style.Unit, mark, absMark int, read bool,
	forced *forcedGeometry) *Fragment {

	delta := actual.Sub(predicted)
	if delta == 0 || origin.ctx.mark() == mark && !read {
		return cf
	}
	if !read || l.relayouts >= maxRelayouts {
		if l.relayouts >= maxRelayouts {
			l.rec.Report(RuleLimit, AtHTML(offsetOf(child)),
				"too many boxes had to be laid out twice to settle where the floats "+
					"around them are; the rest were placed against the position they "+
					"were predicted to have")
		}
		origin.ctx.shift(mark, delta)
		return cf
	}

	l.relayouts++
	origin.ctx.truncate(mark)
	// The out-of-flow boxes the discarded layout found are discarded with it.
	// Without this they would be placed twice — once against a fragment that is
	// about to be thrown away — and the page would carry a ghost of every
	// absolutely positioned box inside a subtree that had to be laid out again.
	l.deferred = l.deferred[:absMark]
	corrected := origin.at(actual)
	corrected.carriedTop = delta
	again, _ := l.blockIn(child, width, corrected, forced)
	return again
}

// maxFloatFits bounds how many times one box may be re-fitted against the floats
// around it.
//
// Each round moves the box strictly down or narrows it, so the sequence cannot
// cycle through the same state twice — but narrowing changes the height, a new
// height meets a different set of floats, and there is no argument that says the
// pair settles quickly. Three rounds is what the suite's deepest case needs
// (a box that drops, is narrowed by the band it lands in, and grows tall enough
// to meet the next float); past that the box keeps the position it has, which is
// a page that is slightly wrong rather than one that takes unbounded time.
//
// It is a variable so that a test can lower it and watch the bound decide,
// on the model of maxRelayouts.
var maxFloatFits = 3

// fitBesideFloats re-asks §9.5's non-overlap question with the height the box
// turned out to have, and lays the box out again where the answer has changed.
//
// The prediction made before the box existed could only ask about the band at
// its top edge. That is the right answer for a box no taller than the float
// beside it and the wrong one for every box that reaches past the float's
// bottom, or past the top of a float that begins lower down — and the second is
// a case the suite tests by name (floats-wrap-top-below-bfc-*), because it is
// the one where a float slides silently through a box that was supposed to move
// out of its way.
//
// The rewind is the one settleIn does: the floats the discarded layout placed
// are dropped, the out-of-flow boxes it deferred are dropped with them, and the
// box is laid out again from the corrected position. Unlike settleIn this cannot
// take the cheap translation, because a box whose band changed is a box whose
// width may have changed, and no translation repairs that.
func (l *layouter) fitBesideFloats(child *Box, width style.Unit, origin flow,
	cf *Fragment, at style.Unit, geom *forcedGeometry, mark, absMark int) (style.Unit, *Fragment) {

	if !avoidsFloats(child) || len(origin.ctx.boxes) == 0 {
		return at, cf
	}
	for i := 0; i < maxFloatFits; i++ {
		drop, next := l.avoidFloats(child, width, origin, at, cf.BorderRect.H, true)
		if drop == 0 && sameForced(geom, next) {
			return at, cf
		}
		if l.relayouts >= maxRelayouts {
			l.rec.Report(RuleLimit, AtHTML(offsetOf(child)),
				"too many boxes had to be laid out twice to settle where the floats "+
					"around them are; the rest were placed against the position they "+
					"were predicted to have")
			return at, cf
		}
		l.relayouts++
		at, geom = at.Add(drop), next
		origin.ctx.truncate(mark)
		l.deferred = l.deferred[:absMark]
		cf, _ = l.blockIn(child, width, origin.at(at), geom)
	}
	return at, cf
}

// sameForced reports whether two forced geometries would lay a box out the same
// way, so that a box whose band did not change is not laid out twice.
func sameForced(a, b *forcedGeometry) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func offsetOf(b *Box) int {
	if b == nil || b.Element == nil {
		return -1
	}
	return b.Element.Offset
}

// resolveWidth computes a block-level box's content width by the constraint of
// CSS 2.1 §10.3.3: the margins, borders, padding and width across the box add up
// to the containing block's width.
//
// It may adjust the margins, which is why they are passed by pointer: "auto" on
// both sides is what centres a box, and it can only be resolved once the width
// is known.
// replaced, when not nil, is the width and height CSS 2.1 §10.3.2 gave a
// replaced box. It stands in for a declared width, and it is *not* clamped
// again: §10.4's constraint table has already applied the minimum and maximum
// on both axes together, and a second independent clamp would break the ratio
// it was careful to keep.
func (l *layouter) resolveWidth(b *Box, margin, border, padding Edges,
	containing style.Unit, out *Edges, replaced *Size) style.Unit {

	fixed := border.Horizontal().Add(padding.Horizontal())
	available := containing.Sub(fixed)

	clamp := func(v style.Unit) style.Unit {
		if replaced != nil {
			return v
		}
		return l.clampWidth(b, v, containing)
	}

	declared, hasWidth := l.explicitWidth(b, containing)
	if replaced != nil {
		declared, hasWidth = replaced.W, true
	}
	marginLeftAuto := l.isAuto(b, "margin-left")
	marginRightAuto := l.isAuto(b, "margin-right")

	switch {
	case b.Inner == InnerTable:
		// §17.5.2 decides a table's width by its own algorithm, and then §10.3.3
		// resolves the margins against it exactly as though it had been
		// declared. Routing it through the declared-width branch below is what
		// gives a table "margin: 0 auto" — the margins are on the wrapper, so
		// this arrives here only for a table whose author put a width on it.
		declared, hasWidth = l.tableUsedWidth(b, available), true
	case b.TableWrapper && !hasWidth:
		// §17.4: the wrapper is as wide as the table's border box and no wider,
		// which is what makes "margin: 0 auto" centre a table rather than
		// centring nothing inside a full-width box.
		if w, ok := l.wrapperWidthForPercentTable(b, containing); ok {
			declared, hasWidth = w, true
			break
		}
		declared, hasWidth = l.shrinkToFit(b, maxZero(available.Sub(margin.Horizontal()))), true
	}

	if b.Float != FloatNone {
		// CSS 2.1 §10.3.5, the rules for a floated box, and both halves matter.
		// An auto margin is zero rather than a share of the slack — a float is
		// not centred by "margin: auto", which surprises people — and an auto
		// width shrinks to fit rather than filling the line, which is the whole
		// visible difference between a float and a block. A float that filled
		// its containing block would leave no room beside it, so nothing would
		// ever flow around one, and the feature would look implemented and do
		// nothing.
		if marginLeftAuto {
			out.Left = 0
		}
		if marginRightAuto {
			out.Right = 0
		}
		if hasWidth {
			return clamp(declared)
		}
		return clamp(l.shrinkToFit(b, maxZero(available.Sub(out.Horizontal()))))
	}

	if !hasWidth {
		// §3.1's fit-content, which is the one keyword that needs the space
		// available and so cannot be answered by explicitWidth. Here it can:
		// the containing block is resolved, the border and padding are off it,
		// and what is left is what the box has to fit into.
		//
		// It is asked after the float branch above rather than before, because a
		// float already shrinks to fit and the branch has its own idea of what
		// the space is — the band it landed in, less the margins it just
		// resolved. Answering here as well would be the same number by a second
		// route, and the two would drift.
		if v, ok := l.fitContentWidth(b, maxZero(available.Sub(margin.Horizontal()))); ok {
			declared, hasWidth = v, true
		}
	}

	if !hasWidth {
		// An auto width fills whatever the margins leave, which is why a plain
		// <div> is as wide as its parent. An auto margin against an auto width
		// is zero — there is nothing left over to distribute.
		if marginLeftAuto {
			out.Left = 0
		}
		if marginRightAuto {
			out.Right = 0
		}
		width := available.Sub(out.Horizontal())
		return clamp(maxZero(width))
	}

	width := clamp(declared)
	slack := available.Sub(width).Sub(margin.Horizontal())

	// §10.3.3's first sentence, which is easy to read past because it is about
	// what the *rules below* see rather than about a result:
	//
	//	If 'width' is not 'auto' and [the total] is larger than the width of
	//	the containing block, then any 'auto' values for 'margin-left' or
	//	'margin-right' are, for the following rules, treated as zero.
	//
	// So a box already too wide for its parent does not get pulled further out
	// by an auto margin taking a negative share of a slack there is none of. It
	// starts at the containing block's edge and overflows the other one — which
	// is what a reader sees in every browser and what
	// block-non-replaced-width-002 draws.
	//
	// Treated as zero rather than solved for: an auto margin is a request for
	// the leftovers, and there are none.
	//
	// Two parts of this cannot be told apart from the alternatives, and both are
	// arithmetic rather than judgement. "Larger than" is a strict comparison and
	// an exact fit gives an auto margin nothing by either reading, so "<= 0"
	// answers identically. And clearing the *right* one changes no number: a
	// right auto margin has a declared value of zero, so the over-constrained
	// branch below gives it "margin.Right + slack" where the auto branch gave it
	// "slack", and those are the same. The left one is the whole of what moves —
	// clearing only the right leaves it taking the negative share and the box
	// starts outside its parent.
	//
	// Both are written as §10.3.3 writes them, which names the pair and says
	// "larger than", rather than trimmed to what a test can see.
	if slack < 0 {
		marginLeftAuto, marginRightAuto = false, false
	}

	switch {
	case marginLeftAuto && marginRightAuto:
		// Centred. An odd number of units puts the extra on the right, which is
		// arbitrary but has to be decided somewhere, and deciding it here keeps
		// it reproducible.
		half := slack.Div(2)
		out.Left, out.Right = half, slack.Sub(half)
	case marginLeftAuto:
		out.Left = slack
	case marginRightAuto:
		out.Right = slack
	default:
		// Over-constrained. The specification resolves it by ignoring one of the
		// two margins, and which one is decided by the *containing block's*
		// direction rather than the box's own: the margin at the end of the
		// inline direction is the one that gives way, so the box stays where its
		// starting margin put it.
		//
		// The containing block's and not this box's, because a box that declares
		// "direction: rtl" is stating which way its own contents run, not which
		// side of its parent it hangs from.
		if b.Parent != nil && isRTL(b.Parent) {
			out.Left = margin.Left.Add(slack)
			break
		}
		out.Right = margin.Right.Add(slack)
	}
	return width
}

func maxZero(u style.Unit) style.Unit {
	if u < 0 {
		return 0
	}
	return u
}

// clampWidth and clampHeight apply the min and max properties, with the CSS
// rule that a minimum wins over a maximum.
//
// v is a *content* size and so is the answer, but the limits are declared values
// and mean whatever box-sizing says they mean. So under "border-box" the clamp
// happens in the declared space — the inset is added, the limits applied, the
// inset taken off again — rather than comparing a content width against a
// border-box minimum. boxsizing.go works through what the naive order produces.
func (l *layouter) clampWidth(b *Box, v, containing style.Unit) style.Unit {
	inset, _ := l.sizingInset(b, containing)
	lo := style.Unit(0)
	// An intrinsic keyword names a content width and box-sizing does not touch
	// it — css-sizing-3 §3.3 — so it is put into the declared space the clamp
	// works in by adding the inset back, which is the same arithmetic the
	// answer below undoes. A declared length is already in that space.
	if v, ok := l.keywordLimit(b, "min-width"); ok {
		lo = v.Add(inset)
	} else if min, ok := l.lengthOf(b, "min-width", containing); ok {
		lo = min
	}
	hi := style.MaxUnit
	if v, ok := l.keywordLimit(b, "max-width"); ok {
		hi = v.Add(inset)
	} else if max, ok := l.lengthOf(b, "max-width", containing); ok {
		hi = max
	}
	return maxZero(style.Clamp(v.Add(inset), lo, hi).Sub(inset))
}

// clampHeight is clampWidth's vertical half, and the two differ in what a
// percentage limit is a percentage *of*.
//
// containing is still the containing block's width, because that is what a
// percentage padding resolves against on both axes and the inset is what
// box-sizing needs. But §10.7 measures a percentage "min-height" or "max-height"
// against the containing block's *height*, and adds the clause the horizontal
// axis has no counterpart for: if that height is not stated explicitly, the
// percentage is treated as zero for a minimum and as "none" for a maximum,
// rather than resolved against anything.
//
// Passing the width for both was a real fault rather than a simplification.
// "min-height: 50%" in a 600px-wide containing block came out as 300px whatever
// the block's height was, which is the mistake explicitHeight's comment warns
// against, made one function away from the warning.
func (l *layouter) clampHeight(b *Box, v, containing, cbHeight style.Unit, cbDefinite bool) style.Unit {
	_, inset := l.sizingInset(b, containing)
	lo := style.Unit(0)
	if min, ok := l.verticalLength(b, "min-height", cbHeight, cbDefinite); ok {
		lo = min
	}
	hi := style.MaxUnit
	if max, ok := l.verticalLength(b, "max-height", cbHeight, cbDefinite); ok {
		hi = max
	}
	return maxZero(style.Clamp(v.Add(inset), lo, hi).Sub(inset))
}

// explicitWidth resolves a declared width to a *content* width.
//
// Under "box-sizing: border-box" the declaration named the border box, so the
// padding and the border come out of it. A declared width smaller than its own
// padding gives a content width of zero rather than a negative one — the box
// still exists and is still as wide as its padding, which is what every browser
// produces and what the specification's "the content box cannot be negative"
// amounts to.
func (l *layouter) explicitWidth(b *Box, containing style.Unit) (style.Unit, bool) {
	// An intrinsic keyword is already a content width and box-sizing does not
	// touch it, so it returns before the inset below rather than through it.
	if v, ok := l.keywordWidth(b); ok {
		return v, true
	}
	v, ok := l.lengthOf(b, "width", containing)
	if !ok {
		// A control's auto width is its intrinsic one — cols characters, size
		// characters — which is a content width and so skips the inset below for
		// the same reason a keyword does. It is asked *after* the declaration
		// rather than before, so that any width an author wrote wins over it,
		// which is what "auto" means and what a browser does.
		return l.controlIntrinsicWidth(b)
	}
	inset, _ := l.sizingInset(b, containing)
	return maxZero(v.Sub(inset)), true
}

// explicitHeight resolves a declared height to a *content* height.
//
// containing is the containing block's width, which is what a percentage padding
// resolves against and so what box-sizing's inset is computed from. The height a
// *percentage height* is a percentage of is a different number and is passed
// separately, together with whether it is definite — because §10.5's rule is
// conditional and the condition is the whole of the difference between this and
// explicitWidth:
//
//	The percentage is calculated with respect to the height of the generated
//	box's containing block. If the height of the containing block is not
//	specified explicitly (i.e., it depends on content height), and this element
//	is not absolutely positioned, the value computes to 'auto'.
//
// Refusing every percentage was the previous reading, and it was half of the
// rule. The condition is not "a containing block's height is never known" — the
// initial containing block is the page, whose height is settled before layout
// runs, and a block that declares its own height passes a definite one down to
// its children. So "html, body, div { height: 100% }" is a chain of definite
// heights from the page, and it is the idiom the CSS 2.1 reftests use to put
// something at the *bottom* of the page: 42 of them draw their expected picture
// with a background positioned against a full-height box, and every one drew it
// against a box as tall as its text instead.
//
// What stays refused is the case the rule is actually about, and it stays
// refused for the reason the old comment gave: a box being sized by its content
// has no height yet, and resolving against the width instead would produce a
// plausible number that is not the one CSS asks for.
func (l *layouter) explicitHeight(b *Box, containing, cbHeight style.Unit, cbDefinite bool) (style.Unit, bool) {
	l.ensureFontSize(b)
	v, ok := l.verticalLength(b, "height", cbHeight, cbDefinite)
	if !ok {
		// A control's auto height is rows line boxes, which is a used height and
		// not a minimum: a textarea holding twenty lines is still two lines tall
		// and the rest is clipped, exactly as a browser scrolls it away.
		return l.controlIntrinsicHeight(b)
	}
	_, inset := l.sizingInset(b, containing)
	return maxZero(v.Sub(inset)), true
}

// lengthOf resolves a property to a length, with percentages against a basis.
func (l *layouter) lengthOf(b *Box, property string, basis style.Unit) (style.Unit, bool) {
	length, ok := l.parseLength(b, property)
	if !ok {
		return 0, false
	}
	return length.Resolve(basis, true)
}

// isAuto reports whether a property is the auto keyword.
func (l *layouter) isAuto(b *Box, property string) bool {
	length, ok := l.parseLength(b, property)
	return ok && length.Kind == style.LengthAuto
}

// parseLength reads one of a box's computed values, memoized.
func (l *layouter) parseLength(b *Box, property string) (style.Length, bool) {
	raw := strings.TrimSpace(b.Style[property])
	if raw == "" {
		return style.Length{}, false
	}
	m := l.metricsFor(b, raw)
	key := lengthKey{value: raw, fontSize: b.FontSize, metrics: m}
	if got, ok := l.lengths[key]; ok {
		return got, true
	}

	vals, _ := css.ParseComponentValues(raw)
	length, _, ok := style.ParseLength(vals, l.lengthContext(b, m))
	if !ok {
		return style.Length{}, false
	}
	l.lengths[key] = length
	return length, true
}

// lengthContext is what the font- and viewport-relative units resolve against.
func (l *layouter) lengthContext(b *Box, m faceMetrics) style.LengthContext {
	return style.LengthContext{
		FontSize:         b.FontSize,
		RootFontSize:     l.rootFontSize,
		ViewportWidth:    l.avail.W,
		ViewportHeight:   l.avail.H,
		ViewportKnown:    true,
		ZeroAdvance:      m.zeroAdvance,
		FontMetricsKnown: m.zeroKnown,
		XHeight:          m.xHeight,
		XHeightKnown:     m.xHeightKnown,
		IcAdvance:        m.icAdvance,
		IcAdvanceKnown:   m.icKnown,
	}
}

// lengthOfValues parses component values that are part of a larger declaration
// — one term of a background position, one component of a background size —
// which parseLength cannot do because it reads a whole property by name.
//
// It is not memoized: the values it is given are already the result of a parse,
// and the boxes that reach it are the ones with a background image on them.
func (l *layouter) lengthOfValues(b *Box, vals []css.ComponentValue) (style.Length, bool) {
	l.ensureFontSize(b)
	var m faceMetrics
	for _, v := range vals {
		if !v.IsToken() || v.Token.Kind != css.Dimension {
			continue
		}
		switch {
		case strings.EqualFold(v.Token.Unit, "ch"):
			m.zeroAdvance, m.zeroKnown = l.zeroAdvance(b)
		case strings.EqualFold(v.Token.Unit, "ex"):
			m.xHeight, m.xHeightKnown = l.xHeightOf(b)
		case strings.EqualFold(v.Token.Unit, "ic"):
			m.icAdvance, m.icKnown = l.icAdvance(b)
		}
	}
	length, _, ok := style.ParseLength(vals, l.lengthContext(b, m))
	return length, ok
}

// usesUnit reports whether a value might carry a two-letter unit.
//
// It is a cheap over-approximation: a false positive costs one face lookup that
// the parse then does not use, and a false negative would silently resolve the
// unit against no font at all. The unit is always preceded by a digit, which is
// what keeps "inherit" and a family name containing the letters from matching.
func usesUnit(raw string, a, b byte) bool {
	upper := func(c byte) byte {
		if c >= 'a' && c <= 'z' {
			return c - 32
		}
		return c
	}
	a, b = upper(a), upper(b)
	for i := 1; i+1 < len(raw); i++ {
		if upper(raw[i]) == a && upper(raw[i+1]) == b &&
			raw[i-1] >= '0' && raw[i-1] <= '9' {
			return true
		}
	}
	return false
}

// xHeightOf is the height of a lowercase x in a box's own font.
//
// It reports false when the face does not state one, so that "ex" falls back to
// half an em rather than to zero — a zero would collapse the box the author was
// sizing, and a face stating no x-height is the common case for the fourteen
// standard faces this engine uses by default.
func (l *layouter) xHeightOf(b *Box) (style.Unit, bool) {
	face, ok := l.fontFor(b)
	if !ok {
		return 0, false
	}
	d := face.Descriptor()
	upem := float64(face.UnitsPerEm())
	// The Declared check cannot be observed today and is kept deliberately. A
	// face that states no x-height reports zero, so the positive test below
	// already sends it to the fallback, and a planted defect removing the
	// Declared check changes nothing. Relying on that is the trap, though: it
	// works only while "not stated" and "stated as zero" happen to be the same
	// bytes, and telling those apart is the entire reason Declared exists. Four
	// bugs in this engine have come from reading a zero as an answer.
	if upem <= 0 || !d.Has(shape.MetricXHeight) || d.XHeight <= 0 {
		return 0, false
	}
	return b.FontSize.Mul(float64(d.XHeight) / upem), true
}

// zeroAdvance is the width of "0" in a box's own font, which is what "ch" means.
//
// It reports false when no face could be found, so that a "ch" length is
// unresolvable — and therefore reported — rather than silently zero, which would
// collapse the box the author was trying to size.
func (l *layouter) zeroAdvance(b *Box) (style.Unit, bool) {
	face, ok := l.fontFor(b)
	if !ok {
		return 0, false
	}
	return l.br.Measure(face, "0", b.FontSize), true
}

// waterIdeograph is CSS Values §5.1.4's own choice of character for "ic":
// U+6C34, which is the same glyph in every CJK script and is full-width in every
// face that has it.
const waterIdeograph = "\u6c34"

// icAdvance is the advance of "水" in a box's own font, which is what "ic" means.
//
// It reports false when the face has no glyph for it, and that is the ordinary
// case rather than the exception: a Latin face has no ideographs at all, and
// measuring the notdef box it would put there instead is measuring nothing the
// author asked about. §5.1.4 has an answer for it — one em — which the resolver
// applies, and which is what makes "width: 4ic" on a Latin element mean four
// ems rather than four boxes of tofu.
//
// It asks the box's *own* face and does not fall back to another. A fallback
// face is chosen per run of text, from the text; there is no text here, and a
// box that declares a Latin family has said what it wants its ideographic
// advance measured in even when the answer is "you have not got one".
//
// The measurement cannot be told from the fallback by any document, and that is
// worth writing down rather than leaving to be rediscovered. A water ideograph
// is a full-width character, so every face that has one makes it exactly one em
// — Noto Sans JP does — and one em is what §5.1.4 says to assume when there is
// no face to ask. Made never to be consulted, every test here still passes and
// the reftest suite does not move: 5567 clean passes either way.
//
// It stays measured because the coincidence is a property of the faces that
// happen to be loadable and not of the unit: a condensed CJK face sets its
// ideographs narrower than the em, and on the day one is loaded the fallback
// would size every "ic" box wrong with nothing to say so. What pins the decision
// meanwhile is TestIcIsTheFacesOwnAdvanceWhenItHasTheIdeograph, which asks this
// function rather than a page.
func (l *layouter) icAdvance(b *Box) (style.Unit, bool) {
	face, ok := l.fontFor(b)
	if !ok {
		return 0, false
	}
	if missesVisible(face, waterIdeograph) {
		return 0, false
	}
	return l.br.Measure(face, waterIdeograph, b.FontSize), true
}

// ensureFontSize gives a box a font size where nothing decided one, so that an
// "em" in one of its declarations resolves against something.
//
// It is for the box tree a caller assembled itself, which has no cascade behind
// it. A box the builder made always carries a size, and a zero one there is a
// zero the document asked for: "font-size: 0" is how a stylesheet removes the
// white space between inline-blocks, and reading it as absent put sixteen pixels
// of strut on every line of such a box.
func (l *layouter) ensureFontSize(b *Box) {
	if !b.fontSizeKnown && b.FontSize == 0 {
		b.FontSize = defaultFontSize
		b.fontSizeKnown = true
	}
}

// edges resolves the four sides of a margin or padding.
//
// Percentages on *both* axes resolve against the containing block's width, which
// looks wrong and is not: it is what makes a percentage padding produce a box
// with a constant aspect ratio, and resolving the vertical ones against the
// height would need a height that is usually not known yet.
func (l *layouter) edges(b *Box, prefix string, containing style.Unit) Edges {
	side := func(name string) style.Unit {
		v, ok := l.lengthOf(b, prefix+"-"+name, containing)
		if !ok {
			return 0
		}
		if prefix == "padding" && v < 0 {
			// A negative padding is not a thing; the declaration is invalid and
			// the initial value stands.
			return 0
		}
		return v
	}
	return Edges{
		Top:    side("top"),
		Right:  side("right"),
		Bottom: side("bottom"),
		Left:   side("left"),
	}
}

// borderWidths resolves the four border widths, which are zero unless a style is
// set — "border-width: 5px" with no "border-style" draws nothing and occupies
// nothing, which is a rule that surprises people and is in the specification for
// a reason: it lets a width be declared once and switched on per side.
func (l *layouter) borderWidths(b *Box) Edges {
	if e, ok := l.collapsedBorderWidths(b); ok {
		// A table or a cell in §17.6.2's model does not have the border it
		// declared: it has half of whichever border won each of the grid lines
		// it touches, which may have come from any of six boxes.
		return e
	}
	side := func(name string) style.Unit {
		if noBorder(b.Style["border-"+name+"-style"]) {
			return 0
		}
		v, ok := l.lengthOf(b, "border-"+name+"-width", 0)
		if !ok {
			// The keyword widths. They are the only place a border width is not
			// a length, and "medium" is the initial value.
			return keywordBorderWidth(b.Style["border-"+name+"-width"])
		}
		return maxZero(v)
	}
	return Edges{
		Top:    side("top"),
		Right:  side("right"),
		Bottom: side("bottom"),
		Left:   side("left"),
	}
}

// outlineWidth is the used width of §18.4's outline, and zero when none is drawn.
//
// The same three-way answer as a border width — a style of none means nothing is
// drawn whatever the width says, a length is used as given, and the keywords are
// what is left — and it is deliberately the same function shape, because the two
// properties differ in where they are painted rather than in how they are read.
//
// The colour is checked here rather than at paint time because this is where a
// finding can be raised. "invert" is CSS 2.1's initial value and asks for the
// pixels underneath to be inverted, which a display list of fills cannot express
// without reading back what it has drawn. Approximating it with a colour would
// put an outline of the wrong colour on the page and say nothing, which is the
// failure this engine reports everywhere else rather than commits.
func (l *layouter) outlineWidth(b *Box) style.Unit {
	if b == nil || noBorder(b.Style["outline-style"]) {
		return 0
	}
	w, ok := l.lengthOf(b, "outline-width", 0)
	if !ok {
		w = keywordBorderWidth(b.Style["outline-width"])
	}
	w = maxZero(w)
	if w == 0 {
		return 0
	}
	if strings.EqualFold(strings.TrimSpace(b.Style["outline-color"]), "invert") {
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(offsetOf(b)),
			Message: "\"outline-color: invert\" asks for the colours under the outline " +
				"to be inverted, which this engine cannot do because it draws a list " +
				"of fills and never reads back what it has drawn; no outline was drawn",
			Path:     PathOf(b.Element),
			Property: "outline-color",
		})
		return 0
	}
	return w
}

// collapsedBorderWidths is the used border of a table or of one of its cells
// under §17.6.2, or false for every other box.
//
// It is asked here rather than by the table code alone because a cell goes
// through ordinary block layout — blockIn resolves its own border from its own
// style like any other box — and a second answer given only to the table
// algorithm would leave the cell's fragment carrying a border box that disagreed
// with where the table put it.
func (l *layouter) collapsedBorderWidths(b *Box) (Edges, bool) {
	switch b.Inner {
	case InnerTable:
		if !borderCollapses(b) {
			return Edges{}, false
		}
		return l.collapsedGridFor(b).table, true
	case InnerTableCell:
		table := tableAncestorOf(b)
		if table == nil || !borderCollapses(table) {
			return Edges{}, false
		}
		e, ok := l.collapsedGridFor(table).cells[b]
		return e, ok
	}
	return Edges{}, false
}

// tableAncestorOf finds the table a cell belongs to.
//
// §17.2.1 guarantees the chain — a cell is in a row, a row is in a row group or
// in a table — so this is two or three steps. The bound is there because the box
// tree is built from an untrusted document and a walk with no end is the one
// mistake a cycle in it could turn into a hang.
func tableAncestorOf(cell *Box) *Box {
	b := cell.Parent
	for i := 0; b != nil && i < 4; i++ {
		if b.Inner == InnerTable {
			return b
		}
		b = b.Parent
	}
	return nil
}

// paddingOf is a box's padding, which a table using §17.6.2 does not have.
//
// "in this model, a table does not have padding (but does have borders and
// margins)". It is not a rounding: the grid lines start at the table's border,
// so a padding would put a gap between the table's own border and the first
// line's other half — two borders that are meant to be one.
func (l *layouter) paddingOf(b *Box, containing style.Unit) Edges {
	if b.Inner == InnerTable && borderCollapses(b) {
		return Edges{}
	}
	return l.edges(b, "padding", containing)
}

func noBorder(styleValue string) bool {
	switch strings.ToLower(strings.TrimSpace(styleValue)) {
	case "", "none", "hidden":
		return true
	}
	return false
}

// keywordBorderWidth is the thin/medium/thick scale, which the specification
// leaves to the engine beyond requiring thin <= medium <= thick. These are the
// values every browser uses.
func keywordBorderWidth(value string) style.Unit {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "thin":
		return mustPx(1)
	case "thick":
		return mustPx(5)
	case "medium":
		return mustPx(3)
	}
	return 0
}
