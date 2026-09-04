package layout

import (
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/style"
)

// The box tree: the fourth of §3's stages, turning a styled document into the
// boxes layout will position.
//
// It is a separate stage because the document tree and the box tree are not the
// same shape, and the places they differ are where layout goes wrong if the
// distinction is skipped. An element can produce no box, or several. A run of
// text between two block children needs a box of its own that no element
// generated. And whether a box is block-level or inline-level is a property of
// the *box*, decided by the cascade, not of the tag that produced it.
//
// # Why this is in package render rather than its own
//
// §3 sketches each stage as a package. What it actually requires is that each
// stage have "a data structure at its boundary" so §7's oracle can attach, and
// Box is that. Splitting the stages into packages would mean moving the finding
// vocabulary below them all — the box stage reports, so it cannot sit above the
// package that defines a finding — and that is a rearrangement to make on
// evidence rather than in advance.

// Outer is how a box participates in its parent's formatting context: as a
// block, as something in a line, or not at all.
type Outer uint8

const (
	// OuterNone is "display: none". The element and everything inside it
	// produce no boxes, which is different from producing an invisible one:
	// nothing inside is laid out, measured or painted.
	OuterNone Outer = iota
	// OuterBlock takes a line of its own.
	OuterBlock
	// OuterInline sits in a line with its siblings.
	OuterInline
)

func (o Outer) String() string {
	switch o {
	case OuterBlock:
		return "block"
	case OuterInline:
		return "inline"
	}
	return "none"
}

// Inner is what formatting context a box establishes for its children.
type Inner uint8

const (
	// InnerFlow is ordinary block-and-inline layout.
	InnerFlow Inner = iota
	// InnerFlowRoot is the same, but as an independent formatting context —
	// what an inline-block establishes, and what stops margins collapsing
	// through it.
	InnerFlowRoot
	// InnerFlex, InnerTable and the table-internal contexts are named here and
	// laid out later; naming them now is what lets the box tree be built once.
	InnerFlex
	InnerTable
	InnerTableRowGroup
	InnerTableRow
	InnerTableCell
	InnerTableCaption
	InnerTableColumnGroup
	InnerTableColumn
	// InnerText is a run of text, which has no children and no context.
	InnerText
)

func (i Inner) String() string {
	switch i {
	case InnerFlowRoot:
		return "flow-root"
	case InnerFlex:
		return "flex"
	case InnerTable:
		return "table"
	case InnerTableRowGroup:
		return "table-row-group"
	case InnerTableRow:
		return "table-row"
	case InnerTableCell:
		return "table-cell"
	case InnerTableCaption:
		return "table-caption"
	case InnerTableColumnGroup:
		return "table-column-group"
	case InnerTableColumn:
		return "table-column"
	case InnerText:
		return "text"
	}
	return "flow"
}

// Box is a node of the box tree.
type Box struct {
	Outer Outer
	Inner Inner

	// Element is the element that generated this box, or nil when nothing did.
	// An anonymous box has no element and no style of its own — it inherits
	// everything, which is what the specification means by anonymous.
	Element *html.Node

	// Style is the computed style of the generating element. An anonymous box
	// carries the style it inherits from, so that a consumer never has to walk
	// up looking for one.
	Style style.ComputedStyle

	// Text is the content of a text box, after white-space processing. It is
	// empty for every other kind.
	Text string

	// FontSize is the computed font-size, resolved here rather than in the
	// cascade because it is the one property whose value depends on its own
	// parent's computed value: "font-size: 2em" means twice the parent's size,
	// so it can only be resolved walking downwards.
	//
	// Only an element that *declared* one resolves; an element that inherited
	// its font-size takes its parent's number unchanged. See fontSizeOf for why
	// that distinction is the whole of the property's correctness.
	//
	// Every font-relative length in the box — its margins, its padding, its
	// line height — is measured against this, so it has to exist before layout
	// asks anything about geometry.
	FontSize style.Unit

	// ListItem marks a box that generates a marker — a bullet or a number.
	ListItem bool

	// MarkerImage is the picture list-style-image named, once it has loaded.
	//
	// nil is not "no image was asked for": it is "no image is being drawn",
	// which §12.6.2 makes the same thing. The property takes effect only while
	// the image is *available*, so a url that does not load leaves this nil and
	// the marker falls back to list-style-type — which is why the type is still
	// cascaded and still read for a box that names an image.
	MarkerImage *ReplacedContent

	// ListValue is what a numbered marker counts to, taken from the "list-item"
	// counter rather than from the item's position among its siblings.
	//
	// ListNumbered says whether it means anything, because zero is a value a
	// list can legitimately be at — <ol start="0"> — and not a way of saying
	// there is no counter. Reading the zero as "unset" numbered that list from
	// one, which is the same shape of fault as a sentinel colliding with a real
	// value elsewhere in this engine.
	//
	// The two differ whenever a document says so, and documents do: <ol start>,
	// <li value>, a "counter-reset: list-item" on the list, an item that is not
	// the first child, or a list whose items are not siblings at all. Counting
	// siblings gets every one of those wrong, and gets them wrong quietly — the
	// list is numbered, just not with the numbers the author asked for.
	//
	// Where there is no counter — a "display: list-item" that no rule increments
	// — the position among the parent's list items is written here instead, by
	// the block walk, which is the only place that knows it. ListNumbered stays
	// false there, and says which of the two answered. Keeping the fallback in
	// the same field is what lets everything that draws a marker read one number:
	// it used to travel as an argument, and three of the five call sites had
	// nothing to pass and passed zero.
	ListValue    int
	ListNumbered bool

	// Replaced is the content of a replaced element — the decoded image an
	// <img> names — or nil for every other box.
	//
	// It is nil rather than empty when the content could not be loaded, and
	// that is the whole of what CSS means by an element being replaced: an
	// image that did not arrive makes the element an ordinary inline box
	// holding its alt text, not a replaced one holding nothing. Layout
	// therefore asks whether this is nil rather than asking what the element's
	// tag is, and an <img> with a broken src goes down exactly the same path as
	// a <span>.
	Replaced *ReplacedContent

	// Control is what makes a box a form control, or nil for every other box.
	//
	// It carries only what CSS cannot say — an intrinsic size in characters and
	// in lines — for the reason ReplacedContent gives about itself: being a
	// control changes where a box's auto width and height come from and nothing
	// else about it. See control.go, and note what it deliberately is not: no
	// PDF form field is produced from it and nothing about it is interactive.
	Control *Control

	// BackgroundImages is the pictures this box's background-image named, by the
	// reference the stylesheet wrote.
	//
	// It is a map rather than a slice because the layers are read later, by
	// layout, and matching a slice to them by index would depend on the loading
	// pass and the reading pass agreeing about a value they parse separately —
	// which is the sort of coupling that survives every test and breaks on the
	// document that repeats one file in two layers.
	//
	// A reference that failed to load is absent rather than present and nil, so
	// a layer naming it paints nothing. That is the same answer a broken <img>
	// gets and for the same reason: the picture is missing, not the box.
	BackgroundImages map[string]*ReplacedContent

	// InsideMarker is the list item whose "list-style-position: inside" marker
	// this box draws, where that is not the item itself.
	//
	// §12.5.1 puts an inside marker "as the first inline box in the principal
	// block box, before the element's content", and an item whose content is
	// block-level has no inline box for it to be the first of. The anonymous
	// box rule is what settles it: the marker is inline content of the item's
	// own, so it belongs in the anonymous block that holds the item's leading
	// inline content — and where the item has none, in an anonymous block of
	// its own before the first block child. See wrapInlines.
	InsideMarker *Box

	// TableWrapper marks the anonymous box §17.4 puts around a table to hold it
	// and its captions.
	//
	// It is a flag rather than an Inner of its own because the wrapper really is
	// an ordinary flow root — block layout, margin collapsing, floats and
	// positioning all treat it as one, and that is the point of it. The one thing
	// that is not ordinary is its width, which is the table's rather than its
	// containing block's, and that is the single question this answers.
	TableWrapper bool

	// FirstLine is the ::first-line style of the element this box came from, or
	// nil where no rule selects one. It is carried here rather than looked up in
	// layout because the pseudo styles belong to the cascade's result, which the
	// box builder holds and the layouter does not.
	FirstLine style.ComputedStyle

	// Pseudo is "before" or "after" when this box is a generated one, and empty
	// otherwise. It is here because a pseudo-element carries its originating
	// element in Element like any other box of that element, so the tree alone
	// cannot say which side of the children a generated box was on.
	Pseudo string

	// Float and Clear are CSS 2.1 §9.5. They live on the box rather than being
	// read out of Style at layout time for the same reason Outer and Inner do:
	// whether a box is in the normal flow changes what the box tree itself is
	// allowed to do to it — an anonymous block box is generated around a run of
	// *in-flow* inline content, and a float in that run is not part of the run.
	Float FloatSide
	Clear ClearSide

	// Position is CSS 2.1 §9.3's scheme, and it lives here for the same reason
	// Float does and one more. An absolutely positioned box is out of flow, so
	// the anonymous-box rules have to know about it before layout runs; and
	// §9.7 blockifies it exactly as it blockifies a float, which is a
	// computed-value rule and so belongs to the stage that computes the box.
	Position PositionScheme

	// ZIndex and ZAuto are §9.9's stacking level. They are read here rather than
	// out of Style at paint time because painting order is decided by a
	// traversal that has to sort boxes against each other, and a sort key that
	// has to be re-parsed at every comparison is a sort that gets written once
	// and then quietly avoided.
	ZIndex int
	ZAuto  bool

	// Order is the box's index in document order, which is the tie-break
	// Appendix E uses between two positioned boxes at the same stacking level.
	//
	// It is recorded rather than derived because painting does not walk the tree
	// in document order any more: an absolutely positioned box is placed after
	// the flow has been laid out and hangs from whichever fragment could hold
	// it, so its position among its siblings has been lost by the time anything
	// needs to sort it. Two overlapping cards written one after the other would
	// otherwise stack in whichever order the placement pass happened to reach
	// them, which is stable, invisible and wrong.
	Order int

	// noLeadInset and noTrailInset mark a piece of an inline box that was split
	// by a block inside it as one whose own margin, border and padding does not
	// begin or does not end here.
	//
	// §8.6's slice model: an inline box broken across a block — or across a line
	// — carries its left inset on its first piece and its right on its last, and
	// nothing on the joins. They are set on the pieces splitInline makes rather
	// than derived at layout time because only the split knows which piece is
	// which; by the time inline layout sees them they are ordinary siblings.
	//
	// Both false is a box that was never split, which is every box in a document
	// without a block inside an inline.
	noLeadInset, noTrailInset bool

	// afterTheFirstLine marks an anonymous block that is not where its parent's
	// first formatted line falls.
	//
	// §16.1 indents "the first line of a block container", and §5.12.1 says the
	// first formatted line of an element may sit inside a block-level descendant
	// — so a block container whose inline content is broken up by block children
	// has exactly one first line, in the first of the boxes it was broken into,
	// and the anonymous blocks after it are continuations of the same flow. They
	// inherit text-indent like everything else and must not act on it, or a
	// paragraph interrupted by a figure comes back indented as though it began
	// again.
	afterTheFirstLine bool
	// splitFrom is the inline boxes a block-level box was lifted out of by
	// §9.2.1.1, outermost first, and is empty for every other box.
	//
	// The inline is broken *around* the block, so the block is not its child in
	// the box tree and nothing about it is found by walking up. Two things about
	// it still reach the block, because they are about the elements rather than
	// the boxes: a relative offset declared on the inline moves everything it
	// contains, and a text-decoration declared on it is drawn across everything
	// it contains. Both were lost, and the second one silently — the block came
	// out at the right place with no underline on it, which reads as a document
	// that did not ask for one.
	splitFrom []*Box

	// fontSizeKnown says a cascade decided this box's FontSize, so a zero there
	// is a zero the document asked for rather than a field nobody filled in.
	//
	// "font-size: 0" is a real declaration and a common one — it is how a
	// stylesheet removes the white space between inline-blocks, and the suite's
	// table-vertical-align-baseline-008 writes it — and ensureFontSize used to
	// read it as absent and put sixteen pixels back. That is invisible in the
	// text, which has no glyphs to draw at either size, and visible in the
	// *strut*: every line in such a box came out the height of a font nobody
	// asked for.
	//
	// It is a flag rather than a pointer or a sentinel because the zero value of
	// a Box has to stay usable: a fragment tree assembled by hand in a test has
	// no cascade behind it, and that is the case ensureFontSize exists for.
	//
	// It travels with FontSize wherever one box takes another's — the anonymous
	// block CSS Display §2.1 wraps a run of inline children in, the anonymous
	// table box §17.2.1 inserts, §17.4's table wrapper, and the text an <img>'s
	// alt attribute becomes — because the two are one fact and copying half of
	// it is how a zero becomes sixteen. Only the first is observable today:
	// nothing asks ensureFontSize about the other three, so a dropped flag
	// answers the same. They carry it because a rule applied in three places out
	// of four is the drift this field exists to prevent.
	fontSizeKnown bool

	// staticInline records that this box was inline-level before §9.7 blockified
	// it for being out of flow.
	//
	// It exists for one question, and the question is asked in the specification's
	// own words. §10.3.7 resolves an "auto" left on an absolutely positioned box
	// against "a hypothetical box that would have been the first box of the
	// element if its 'position' property had been 'static'" — and with a static
	// position there is no blockification, so the hypothetical box of an
	// abspos <span> is on the line where it was written and that of an abspos
	// <div> is a block starting at the containing block's content edge. Reading
	// the used display instead makes every such box inline-level, because §9.7
	// has already turned every one of them into a block.
	staticInline bool

	Children []*Box
	Parent   *Box
	// ContentImage is the reference a picture in generated content names —
	// "content: url(x.png)" — for a box that stands for that picture and
	// nothing else.
	//
	// It is a reference rather than the picture because nothing in this walk
	// reads a file. The loader that fetches every other picture in the document
	// fetches this one too, under the same caps, the same cache and the same
	// policy; a second path to a file would be a second policy, and the second
	// one is always the one that is missing a check.
	ContentImage string
}

// outOfFlow reports whether a box takes no space among its siblings, which is
// true of a float and of an absolutely positioned box.
//
// The two are different mechanisms and the box tree treats them identically,
// because everything the box tree does with either is a consequence of the one
// property they share: a box that is not in the flow does not end a run of
// inline content, does not force an anonymous block around its neighbours, and
// does not split the inline box it was written inside.
func (b *Box) outOfFlow() bool {
	return b.Float != FloatNone || b.Position.outOfFlow()
}

// Anonymous reports whether a box was generated by the engine rather than by an
// element.
func (b *Box) Anonymous() bool { return b.Element == nil && b.Inner != InnerText }

// IsText reports whether a box is a run of text.
func (b *Box) IsText() bool { return b.Inner == InnerText }

// maxBoxes bounds the tree. A document is untrusted, and anonymous box
// generation can produce more boxes than there were elements, so the html
// package's node cap does not bound this on its own.
//
// It is a variable rather than a constant only so that a test can lower it.
// Building a million boxes to watch a cap fire would take longer than the rest
// of the suite put together, and a bound that is never seen to fire is one
// nobody knows works — which was found the hard way: the first test for this
// built five thousand boxes, nowhere near the cap, and passed just as happily
// with the cap removed.
var maxBoxes = 1 << 20

// BuildBoxes turns a styled document into a box tree.
//
// The root box is the one the document element generated. It is nil when the
// document produces no boxes at all, which is what "html { display: none }"
// means and is not an error.
func BuildBoxes(doc *html.Node, styled style.Styled, rec *Recorder) *Box {
	b := &boxBuilder{
		styles: styled.Styles, pseudo: styled.Pseudo, rec: rec,
		ownFontSize: styled.OwnFontSize, ownPseudoFontSize: styled.OwnPseudoFontSize,
		// Counters are settled before any box exists, because a counter's value
		// depends on what came *before* an element in the document and the box
		// walk cannot answer that while descending.
		counters: computeCounters(doc, styled.Styles, styled.Pseudo),
	}
	root := documentElementOf(doc)
	if root == nil {
		return nil
	}
	b.documentElement = root
	// The root's font-size is resolved against the initial value, since there
	// is no parent to take one from, and it then becomes what every "rem" in
	// the document means. CSS Values §5.1.1 says it in as many words: an em on
	// the root element refers to the property's *initial* value.
	//
	// The build below is handed that same initial value rather than the answer,
	// and the distinction is the whole of this. Every element resolves its own
	// declared font-size against its parent's, and the root is an element like
	// any other — so handing it 32 as a parent made "font-size: 2em" resolve a
	// second time, to 64, while every "rem" in the document went on meaning 32.
	// A document whose root said 2em came out at twice the size of one whose
	// root said 32px, which is numbers-units-021.
	b.rootFontSize = defaultFontSize
	b.rootFontSize = b.fontSizeOf(root, defaultFontSize)

	box := b.build(root, nil, defaultFontSize)
	if box == nil {
		return nil
	}
	// Anonymous boxes are added after the whole tree exists, because the rule
	// that generates them is about a box's *set* of children — a block child
	// anywhere makes every inline run a candidate — and that set is not known
	// while the children are still being made.
	b.fixup(box)
	// §17.4's wrapper is a second pass because it has to see tables that the
	// first pass generated as well as the ones the author wrote, and it may
	// replace the root itself: "html { display: table }" is legal.
	return b.wrapTables(box)
}

func documentElementOf(doc *html.Node) *html.Node {
	if doc == nil {
		return nil
	}
	if doc.Type == html.ElementNode {
		return doc
	}
	for _, c := range doc.Children {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

// defaultFontSize is the initial value of font-size, "medium", in layout units.
// It is what the root resolves a relative size against and what a document that
// sets no size gets.
var defaultFontSize = mustPx(16)

func mustPx(px float64) style.Unit {
	u, _ := style.FromPx(px)
	return u
}

type boxBuilder struct {
	styles map[*html.Node]style.ComputedStyle
	pseudo map[style.PseudoKey]style.ComputedStyle
	// ownFontSize and ownPseudoFontSize say which elements declared a font-size
	// of their own. See fontSizeOf.
	ownFontSize map[*html.Node]bool
	// resolvedFontSize remembers what each element's declared font-size came to,
	// so that the root — the one element asked twice — cannot answer differently
	// the second time. See fontSizeOf.
	resolvedFontSize  map[*html.Node]style.Unit
	ownPseudoFontSize map[style.PseudoKey]bool
	rec               *Recorder
	// counters holds what each element and each pseudo-element sees. See
	// counter.go for why it is a walk of its own rather than something this
	// builder can answer on the way down, and for why the two are separate.
	counters     counterSnapshots
	rootFontSize style.Unit
	count        int
	// documentElement is the root, which §2.7 blockifies and so exempts from
	// "display: contents". See contentsIsHonoured.
	documentElement *html.Node
	// reportedPhraseSeparators says the finding below has been made once. It is
	// once per document rather than once per box for the reason
	// layouter.reportWordBreak is: the value is inherited, so a page that
	// declares it once and has a hundred text nodes has one gap and not a
	// hundred.
	reportedPhraseSeparators bool
	// afterWord says the last character emitted was part of a word, which is what
	// "text-transform: capitalize" needs to know and what a text node cannot
	// answer on its own: in "<b>e</b>xample" the "x" does not begin a word. It is
	// carried on the builder because the walk visits text in document order, and
	// it is reset around a block-level box because a block starts a new line of
	// text whatever preceded it.
	afterWord bool
	// boundary is the text built so far, as much of it as §4.1.1's segment
	// break rules need: the last rune written and the last one a reader would
	// see. It is carried for the reason afterWord is — the walk visits text in
	// document order and a text node cannot see the one before it — and it is
	// what makes "aa&#x200b;<span></span>\nbb" transform like
	// "aa&#x200b;\nbb", which css-text-4 requires in as many words:
	// "intervening inline box boundaries must be ignored".
	//
	// It is reset where afterWord is, and for the same reason: a block begins
	// its text afresh, and so does the text after a <br>.
	boundary textBoundary
	// stopped records that the box cap was reached, so it is reported once
	// rather than per box.
	stopped bool
}

// build makes the box or boxes for one node, or nil for none.
func (b *boxBuilder) build(n *html.Node, inherited style.ComputedStyle, fontSize style.Unit) *Box {
	switch n.Type {
	case html.TextNode:
		return b.textBox(n, inherited, fontSize)
	case html.ElementNode:
		return b.elementBox(n, fontSize)
	}
	return nil
}

// fontSizeOf resolves an element's computed font-size against its parent's.
//
// A value this engine cannot resolve leaves the size at the parent's, which is
// what inheriting would have given — the safe answer, since a size of zero would
// make every em-based margin in the subtree vanish.
//
// # Why an inherited size is not resolved again
//
// Only an element that *declared* a font-size resolves one. CSS makes the
// computed value of font-size an absolute length, so inheritance passes a
// number — and the cascade now stores one, so for almost every document this
// call resolves "28px" to 28px and the guard changes nothing.
//
// It still matters for the one value the cascade could not resolve, which it
// leaves as the author wrote it. A descendant of an unresolvable
// "font-size: 2em" inherits that string, and re-resolving it against the parent
// would double the size at every level: a paragraph four elements inside such a
// wrapper came out at 256px.
//
// That was a real bug, found by the table work, and back then it applied to
// every relative font-size in every document. The two halves of a reftest
// nested their content to different depths, so the compounding moved one and not
// the other and the difference finally showed. It had been invisible until then
// because it moves every part of a document equally.
func (b *boxBuilder) fontSizeOf(n *html.Node, parent style.Unit) style.Unit {
	if !b.ownFontSize[n] {
		return parent
	}
	// Resolved once per element, and remembered.
	//
	// Every element is asked once except the root, which is asked twice: to
	// establish what "rem" means before the tree is built, and again as an
	// element of that tree. The two must agree, and they cannot be made to
	// agree by argument alone — the second call sees a rootFontSize the first
	// one was computing, so "font-size: 2rem" on the root resolved against 16
	// the first time and against its own answer the second, and the root came
	// out twice the size the rest of the document was measuring against.
	if got, ok := b.resolvedFontSize[n]; ok {
		return got
	}
	cs, ok := b.styles[n]
	if !ok {
		return parent
	}
	vals, _ := css.ParseComponentValues(cs["font-size"])
	size, unsupported, ok := style.ResolveFontSize(vals, parent, b.rootFontSize)
	if !ok {
		if unsupported {
			b.rec.ReportDetail(Finding{
				Rule:     RuleUnsupportedValue,
				Source:   AtHTML(n.Offset),
				Message:  "the font-size " + quoteValue(cs["font-size"]) + " could not be resolved; the inherited size was kept",
				Path:     PathOf(n),
				Property: "font-size",
			})
		}
		return parent
	}
	if b.resolvedFontSize == nil {
		b.resolvedFontSize = map[*html.Node]style.Unit{}
	}
	b.resolvedFontSize[n] = size
	return size
}

// quoteValue renders a value for a diagnostic without letting a hostile
// stylesheet put control characters into a caller's log.
func quoteValue(s string) string {
	const max = 40
	var out strings.Builder
	out.WriteByte('"')
	for i, r := range s {
		if i >= max {
			out.WriteString("...")
			break
		}
		if r < 0x20 || r == 0x7F {
			out.WriteByte('?')
			continue
		}
		out.WriteRune(r)
	}
	out.WriteByte('"')
	return out.String()
}

func (b *boxBuilder) elementBox(n *html.Node, parentFontSize style.Unit) *Box {
	cs := b.styles[n]
	outer, inner, listItem := displayOf(cs)
	if outer == OuterNone {
		// Nothing inside is laid out either, which is the difference between
		// display:none and visibility:hidden.
		return nil
	}
	if !b.room(n) {
		return nil
	}
	order := b.count

	fontSize := b.fontSizeOf(n, parentFontSize)
	float := floatOf(cs)
	position := positionOf(cs)
	if position.outOfFlow() {
		// §9.7's first row: a box that is absolutely positioned does not float.
		// The two mechanisms both remove a box from the flow and they cannot
		// both do it — an implementation that honoured the float as well would
		// shift the box to an edge it was never asked to go to, and then resolve
		// its offsets against a containing block chosen for the float.
		float = FloatNone
	}
	if isLayoutInternalDisplay(inner) && replacesItsOwnContent(n) {
		// A replaced element cannot be a table cell, a row, or any of the other
		// boxes that exist only inside a table. Its content is not a formatting
		// context this engine — or any engine — can put a table's structure
		// through, so the declaration is dropped and the element is laid out as
		// what it is: an inline replaced box, which the table fixup then wraps in
		// an anonymous cell along with whatever inline content sits beside it.
		//
		// The visible difference is not academic. Honouring it made an <img
		// style="display: table-cell"> a cell, and the cell took the column's
		// width and the row's height and stretched the picture to fill them —
		// 15 by 15 pixels of swatch drawn 63 wide and 21 tall. The suite's
		// table-anonymous-objects-211 puts a row of them beside a row of real
		// cells and asks for the two to match.
		outer, inner = OuterInline, InnerFlow
	}
	staticInline := outer == OuterInline
	outer, inner = outOfFlowDisplay(outer, inner, float, position)
	if outer == OuterBlock && inner == InnerFlow && overflowIsScrollable(cs) {
		// CSS 2.1 §9.4.1: a block box whose overflow is anything but visible
		// establishes a block formatting context. That is not a painting
		// detail — it is what makes "overflow: hidden" the idiom for containing
		// a float, because §10.6.7 gives a formatting-context root a height
		// that includes the floats inside it. An engine that treated overflow
		// as purely visual would leave the float sticking out of the box the
		// author wrapped around it precisely to stop that.
		inner = InnerFlowRoot
	}
	z, zAuto := zIndexOf(cs)
	box := &Box{
		Outer: outer, Inner: inner, Element: n, Style: cs,
		ListItem: listItem, FontSize: fontSize, fontSizeKnown: true,
		Float: float, Clear: clearOf(cs),
		Position: position, ZIndex: z, ZAuto: zAuto, Order: order,
		staticInline: staticInline,
	}
	box.FirstLine = b.pseudo[style.PseudoKey{Node: n, Name: "first-line"}]
	box.ListValue, box.ListNumbered = b.listValueOf(n, listItem)
	box.Control = b.controlFor(n)
	if (outer != OuterInline && !box.outOfFlow()) || endsAWord(n) {
		// A block-level box begins its text afresh, so a word cannot run into it
		// from the paragraph before. Without this, "<p>hi</p><p>there</p>" under
		// "capitalize" would leave the second paragraph's "t" lower-case, since
		// the "i" before it is a word character.
		//
		// An out-of-flow box is not one of them, however §9.7 has blockified it.
		// It is not in the text at all: nothing of it sits between the letters
		// either side, and a word does not end because an author hung an overlay
		// off the middle of it. text-transform-capitalize-033 is exactly that —
		// "p<span style='position: absolute'></span>ass" — and it came out
		// "PAss".
		//
		// A <br> is inline and does the same thing for the same reason: it is a
		// line break the author wrote, so what follows it begins a line and
		// begins a word. "i ask<br/>questions" under "capitalize" is "I Ask" and
		// "Questions", which is what text-transform-cap-003 asks for by writing
		// its expectation out in full.
		b.afterWord = false
		// And the boundary with it, for the reason above rather than for an
		// observable one: a segment break at the start or the end of a block is
		// at the edge of a line, and §4.1.2 removes the space it would become
		// whether or not the exceptions here got to it first. A planted defect
		// dropping this leaves every test passing and the suite unmoved. It
		// stays because the alternative is a boundary that is wrong and
		// invisible, which is the pair this whole vocabulary exists to avoid.
		b.boundary = textBoundary{}
	}
	if box.outOfFlow() {
		// And an out-of-flow box's *content* is not in the text either side of
		// it. That is the same reasoning as the exception just above, applied
		// one level in: the paragraph above keeps its boundary across the box,
		// so the text inside must not be allowed to overwrite it — and must not
		// read it either, because what an overlay begins with does not follow
		// the paragraph it was hung off.
		//
		// seg-break-transformation-019 is exactly that shape, four times over:
		// an absolutely positioned, a fixed and two floated <aside>s written
		// between a zero-width space and the segment break it suppresses.
		saved := b.boundary
		b.boundary = textBoundary{}
		defer func() { b.boundary = saved }()
	}

	// ::before and ::after bracket the element's own children rather than
	// replacing them, which is why they are added here and not by the caller.
	if before := b.generated(n, "before", fontSize); before != nil {
		before.Parent = box
		box.Children = append(box.Children, before)
	}
	// A control that shows a value rather than markup — a text field, a submit
	// button — has that value here as an ordinary text box. It comes before the
	// element's own children because an <input> is void and has none, and
	// because a <select>'s children are its options rather than its label.
	b.controlContent(box, n, cs, fontSize)
	// <wbr> under word-space-transform, which is the element becoming the
	// character HTML says it is rendered as.
	//
	// The property expands a *virtual word separator*, and there are two of
	// them: U+200B, which a document writes in its text, and this, which it
	// writes as an element. Giving the element a zero width space of its own
	// puts the two on one path — the same collapsing, the same transform, the
	// same measuring, the same breaking — rather than teaching every stage that
	// an element can be a space.
	//
	// It is done here, before the children, because the element is void and has
	// none. With the property at its initial value nothing is added and the
	// element stays what it was: a break opportunity that marks no boundary in
	// the text, which is what the flattening makes of an empty one.
	if strings.EqualFold(n.Name, "wbr") {
		if wst := b.wordSpaceTransformFor(cs); wst.Transforms() {
			sep := &html.Node{Type: html.TextNode, Text: "\u200b", Offset: n.Offset}
			if t := b.textBox(sep, cs, fontSize); t != nil {
				t.Parent = box
				box.Children = append(box.Children, t)
			}
		}
	}
	b.appendChildren(box, n, cs, fontSize)
	if after := b.generated(n, "after", fontSize); after != nil {
		after.Parent = box
		box.Children = append(box.Children, after)
	}
	if outer != OuterInline && !box.outOfFlow() {
		// And a block-level box ends its text: the word does not continue into
		// whatever comes after it. An out-of-flow one still does not, for the
		// reason above.
		b.afterWord = false
		// And the boundary with it, for the reason above rather than for an
		// observable one: a segment break at the start or the end of a block is
		// at the edge of a line, and §4.1.2 removes the space it would become
		// whether or not the exceptions here got to it first. A planted defect
		// dropping this leaves every test passing and the suite unmoved. It
		// stays because the alternative is a boundary that is wrong and
		// invisible, which is the pair this whole vocabulary exists to avoid.
		b.boundary = textBoundary{}
	}
	return box
}

// appendChildren builds an element's children into a box, following the ones
// that stand for their own contents rather than for a box.
//
// It is a function rather than a loop in elementBox because "display: contents"
// makes it recursive: such an element's children belong to *this* box, and one
// of them may be another such element.
func (b *boxBuilder) appendChildren(box *Box, n *html.Node,
	inherited style.ComputedStyle, fontSize style.Unit) {

	for _, child := range n.Children {
		if controlSkipsChild(box, child) {
			continue
		}
		if child.Type == html.ElementNode && b.replacedByItsContents(child) {
			b.appendContents(box, child, fontSize)
			continue
		}
		if c := b.build(child, inherited, fontSize); c != nil {
			c.Parent = box
			box.Children = append(box.Children, c)
		}
	}
}

// appendContents is css-display-3 §3.1's "display: contents": the element
// generates no box of its own, and for the purposes of box generation stands in
// the tree as the boxes its children and pseudo-elements make.
//
// Everything else about the element is unchanged, and that is the half worth
// saying: its computed style is still what its children inherit, its font-size
// is still what an em inside it means, and its ::before and ::after still
// generate boxes — the specification says so in as many words, because "no box"
// is a statement about this element and not about anything it contains.
//
// The children are appended to the enclosing box, so the element leaves no trace
// in the tree at all. That is the whole visible difference from what this used
// to do, which was to treat the value as "inline": an inline box takes part in
// layout, so its own margins, padding, borders and background were drawn and its
// boundary broke shaping and letter-spacing, all of which the author asked
// against.
func (b *boxBuilder) appendContents(box *Box, n *html.Node, parentFontSize style.Unit) {
	if !b.room(n) {
		return
	}
	cs := b.styles[n]
	fontSize := b.fontSizeOf(n, parentFontSize)
	add := func(c *Box) {
		if c == nil {
			return
		}
		c.Parent = box
		box.Children = append(box.Children, c)
	}
	add(b.generated(n, "before", fontSize))
	b.appendChildren(box, n, cs, fontSize)
	add(b.generated(n, "after", fontSize))
}

// replacedByItsContents reports whether an element is one that "display:
// contents" removes from box generation.
//
// The value does not apply to every element, and css-display-3's appendix on
// unusual elements is why: an element whose layout is not decided by CSS box
// generation has no contents to be replaced by. A replaced element's content is
// a picture or a document and a form control's is a widget the engine draws —
// neither is a subtree of boxes, so "the boxes its children make" names nothing
// and the declaration cannot be honoured. Those keep the box they had and the
// finding that says the value was not applied.
//
// The root element is the other exception, and is not one this engine chose:
// §2.7 blockifies the root, so "display: contents" on <html> computes to
// "block" and the element has a box like any other. It is left with its old
// treatment and its finding rather than blockified here, because blockifying
// the root is a rule about every display value and not about this one.
func (b *boxBuilder) replacedByItsContents(n *html.Node) bool {
	return contentsIsHonoured(n, b.styles[n], b.documentElement)
}

// contentsIsHonoured is the predicate above with the state it needs passed in,
// so that the guardrail in pipeline.go can ask the same question the box tree
// asks rather than a second copy of it.
func contentsIsHonoured(n *html.Node, cs style.ComputedStyle, root *html.Node) bool {
	if n == nil || cs == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(cs["display"]), "contents") {
		return false
	}
	if n == root {
		return false
	}
	return !replacesItsOwnContent(n) && controlKindOf(n) == controlNone
}

// endsAWord reports whether an element ends the word before it, without being
// block-level.
//
// A <br> is the one: it is a line break the author wrote, and a word does not
// continue across a line the author ended. The <wbr> beside it deliberately is
// not — it offers a break opportunity and marks no boundary in the text, so
// "sur<wbr/>name" is one word and "capitalize" gives it one capital.
func endsAWord(n *html.Node) bool {
	return n != nil && n.Type == html.ElementNode && strings.EqualFold(n.Name, "br")
}

// fontSizeOfStyle resolves a font-size from a computed style that belongs to no
// element of its own, which is what a pseudo-element has.
//
// own says whether a rule set the pseudo-element's font-size, for the reason
// fontSizeOf gives: a pseudo-element inherits from its originating element, and
// re-resolving a size it inherited would compound it.
func (b *boxBuilder) fontSizeOfStyle(cs style.ComputedStyle, parent style.Unit, own bool) style.Unit {
	if !own {
		return parent
	}
	vals, _ := css.ParseComponentValues(cs["font-size"])
	size, _, ok := style.ResolveFontSize(vals, parent, b.rootFontSize)
	if !ok {
		return parent
	}
	return size
}

// room reports whether another box may be made.
func (b *boxBuilder) room(n *html.Node) bool { return b.roomAt(n.Offset) }

// roomAt is room for a box no element generated, which has no node to be
// reported against — every box §17.2.1 inserts is one.
func (b *boxBuilder) roomAt(offset int) bool {
	if b.count >= maxBoxes {
		if !b.stopped {
			b.stopped = true
			b.rec.Report(RuleLimit, AtHTML(offset),
				"the document produces more boxes than this engine will build; "+
					"the rest of it was not laid out")
		}
		return false
	}
	b.count++
	return true
}

// textBox makes a text box, applying Phase I of white-space processing.
//
// Only Phase I: the rules that span a box boundary and the rules that need a
// line are applied later, and whitespace.go says where and why.
//
// Text whose content collapses to nothing produces no box. That is not an
// optimisation: an empty text box between two blocks would make the parent a
// block container with inline content, and so generate an anonymous block that
// occupies a line.
func (b *boxBuilder) textBox(n *html.Node, inherited style.ComputedStyle, fontSize style.Unit) *Box {
	wst := b.wordSpaceTransformFor(inherited)
	kind := transformOf(inherited["text-transform"])
	before := b.boundary
	// Whether a run of white space open at the end of the node before continues
	// into this one is a question only "full-width" has to ask here. See
	// Boundary.Collapsed: the flattening collapses such a run across the
	// boundary and keeps the break opportunity the rule's parenthesis asks for,
	// which is the better answer and is the one every other document gets. What
	// it cannot do is collapse a space that is no longer white space — and
	// "full-width" maps U+0020 to U+3000 IDEOGRAPHIC SPACE, which is not
	// collapsible at all, before the flattening ever sees it.
	//
	// So the run is closed here only where leaving it open would freeze it, and
	// asking any wider costs fifty-seven documents: a space removed rather than
	// collapsed takes its break opportunity with it, and the sixteen
	// border-top-width pairs and the seventeen content-counter ones wrap
	// somewhere else without it.
	if !transformFreezesSpace(kind) {
		before.Collapsed = false
	}
	text := collapseWhitespaceAfter(n.Text, inherited["white-space-collapse"], wst,
		before, writingSystemAt(n))
	b.reportPhraseSeparators(n, text, wst)
	// Whether the run of white space this node ends with is still open, asked
	// of the collapsed text and *before* the transform below rewrites it. See
	// Boundary.Collapsed: "full-width" turns the space into a U+3000, and by
	// then there is nothing left for the question to be about.
	endsCollapsed := endsCollapsedSpace(text, inherited["white-space-collapse"], wst)
	// text-transform, applied here so that the text every later stage measures,
	// breaks, draws and writes into the PDF is the text that will appear.
	// texttransform.go works through why it cannot wait until paint time.
	//
	// It runs after the white-space processing rather than before it, and the
	// order is load-bearing for "full-width": that value maps U+0020 to U+3000
	// IDEOGRAPHIC SPACE, which is not collapsible, so transforming first would
	// turn a run of spaces into a run of spaces nothing may collapse. It is also
	// what lets "capitalize" see the word boundaries the reader will.
	text, b.afterWord = transformText(text, kind, b.afterWord, languageAt(n))
	// After the transform rather than before it, because what the next node
	// follows is the text that will be on the page: "full-width" turns a space
	// into U+3000, which nothing collapses, and the rules below are about the
	// character a reader sees.
	b.boundary = boundaryAfter(b.boundary, text)
	if text != "" {
		// Guarded for the reason BoundaryAfter is: a node that collapsed to
		// nothing is not between the characters either side of it, so it leaves
		// the run it found exactly as open as it was.
		b.boundary.Collapsed = endsCollapsed
	}
	if text == "" {
		return nil
	}
	if !b.room(n) {
		return nil
	}
	return &Box{
		Outer: OuterInline, Inner: InnerText,
		Style: inherited, Text: text, FontSize: fontSize, fontSizeKnown: true,
	}
}

// displayOf reads the display property into the outer/inner pair.
//
// CSS now defines display as two values — how a box behaves in its parent, and
// what context it makes for its children — and the single keywords are
// shorthands for pairs. Modelling it as the pair is what makes "inline-block"
// stop being a special case: it is simply inline outside and flow-root inside.
func displayOf(cs style.ComputedStyle) (Outer, Inner, bool) {
	value := strings.ToLower(strings.TrimSpace(cs["display"]))

	// The two-value syntax, "inline flow-root" and friends.
	if outer, inner, ok := twoValueDisplay(value); ok {
		return outer, inner, false
	}

	switch value {
	case "none":
		return OuterNone, InnerFlow, false
	case "-webkit-box":
		// The legacy flexible box, of which this engine implements exactly the
		// part CSS Overflow 4's compatibility section needs: a block that
		// "-webkit-line-clamp" can be written on. Its own layout — the old
		// flexbox — is not implemented, and treating it as a block is what every
		// engine does for the vertical, single-column case the clamp is used in.
		return OuterBlock, InnerFlow, false
	case "block", "flow-root":
		inner := InnerFlow
		if value == "flow-root" {
			inner = InnerFlowRoot
		}
		return OuterBlock, inner, false
	case "inline":
		return OuterInline, InnerFlow, false
	case "inline-block":
		return OuterInline, InnerFlowRoot, false
	case "list-item":
		return OuterBlock, InnerFlow, true
	case "flex":
		return OuterBlock, InnerFlex, false
	case "inline-flex":
		return OuterInline, InnerFlex, false
	case "table":
		return OuterBlock, InnerTable, false
	case "inline-table":
		return OuterInline, InnerTable, false
	case "table-row-group":
		return OuterBlock, InnerTableRowGroup, false
	case "table-header-group":
		return OuterBlock, InnerTableRowGroup, false
	case "table-footer-group":
		return OuterBlock, InnerTableRowGroup, false
	case "table-row":
		return OuterBlock, InnerTableRow, false
	case "table-cell":
		return OuterBlock, InnerTableCell, false
	case "table-caption":
		return OuterBlock, InnerTableCaption, false
	case "table-column-group":
		return OuterBlock, InnerTableColumnGroup, false
	case "table-column":
		return OuterBlock, InnerTableColumn, false
	case "contents":
		// "display: contents" replaces the element with its children. It is not
		// implemented, and the closest available answer — treating it as inline
		// — is wrong in a way that shows: the element's own box would take part
		// in layout when the author asked for it not to. Treated as inline and
		// reported by the caller.
		return OuterInline, InnerFlow, false
	}
	// An unrecognised value: the initial one, which is what the cascade would
	// have used had the declaration been invalid.
	return OuterInline, InnerFlow, false
}

// isLayoutInternalDisplay reports the display types that exist only inside a
// table — the ones css-display-3 calls layout-internal.
//
// The caption is one of them. It is not inside the table's border box, but it is
// still a box §17.4's wrapper generates and holds, and it is no more available
// to a replaced element than a cell is.
func isLayoutInternalDisplay(inner Inner) bool {
	switch inner {
	case InnerTableRowGroup, InnerTableRow, InnerTableCell,
		InnerTableColumn, InnerTableColumnGroup, InnerTableCaption:
		return true
	}
	return false
}

// replacesItsOwnContent reports whether an element's content comes from
// somewhere other than its children.
//
// It is asked at build time, where Box.Replaced is not filled in yet — that
// happens in a later pass, once there is a resolver — so the question has to be
// put to the element rather than to the box. The two elements here are the two
// that pass sets: an <img> and an <object>. A form control is replaced in the
// CSS sense too, and is deliberately not included, because this engine does not
// model one as replaced anywhere else either; whatever "display: table-cell" on
// a <select> should do, it should not be decided here alone.
func replacesItsOwnContent(n *html.Node) bool {
	if n == nil {
		return false
	}
	switch strings.ToLower(n.Name) {
	case "img", "object":
		return true
	case "svg", "math":
		// A foreign root is a replaced element: it has a box, and its content is
		// not HTML but a picture the element carries with it. The parser keeps
		// that content as source rather than parsing it — see html.Node.Foreign
		// — and layout reads it exactly as it reads an SVG an <img> points at.
		return true
	}
	return false
}

// floatOf and clearOf read the two out-of-flow properties.
//
// An unrecognised value gives the initial one, which is what the cascade would
// have produced had the declaration been thrown out. "inline-start" and
// "inline-end" are CSS Logical rather than CSS 2.1 and are not read here: they
// need the writing mode, and answering them as "left" would be right for a
// left-to-right document and silently wrong for the documents they exist for.
func floatOf(cs style.ComputedStyle) FloatSide {
	switch strings.ToLower(strings.TrimSpace(cs["float"])) {
	case "left":
		return FloatLeft
	case "right":
		return FloatRight
	}
	return FloatNone
}

func clearOf(cs style.ComputedStyle) ClearSide {
	switch strings.ToLower(strings.TrimSpace(cs["clear"])) {
	case "left":
		return ClearLeft
	case "right":
		return ClearRight
	case "both":
		return ClearBoth
	}
	return ClearNone
}

// outOfFlowDisplay applies CSS 2.1 §9.7: a floated or absolutely positioned
// box's display is blockified.
//
// The table in §9.7 is what makes "float: left" work on a <span> without the
// author also writing "display: block", and it makes "position: absolute" do the
// same — which matters more than it sounds, because the everyday abspos idiom is
// on a <span> or an <a>. It is a computed-value rule rather than a layout one,
// and doing it here rather than in layout is what keeps the rest of the engine
// from having to ask "is this inline box actually inline" at every step — the
// box tree's invariant that an inline box takes part in a line is only true if
// an out-of-flow box has already stopped being inline.
//
// Both also establish a block formatting context of their own, which is the half
// of §9.7 that is easy to forget and expensive to omit: without it the margins
// of a float's first child would collapse out through its top edge, so a floated
// <div> holding a <p> would be positioned an em above where the author put it,
// and the floats inside it would escape into the surrounding context.
func outOfFlowDisplay(outer Outer, inner Inner, float FloatSide, position PositionScheme) (Outer, Inner) {
	if outer == OuterNone || (float == FloatNone && !position.outOfFlow()) {
		return outer, inner
	}
	switch inner {
	case InnerFlow, InnerFlowRoot:
		return OuterBlock, InnerFlowRoot
	case InnerTable, InnerFlex:
		// The two-value forms whose inner half survives blockification: an
		// inline-table floats as a table, an inline-flex as a flex container.
		return OuterBlock, inner
	}
	// The table-internal displays. §9.7 turns each into its block-level
	// equivalent, and the honest one for a lone floated cell or row is a block.
	return OuterBlock, InnerFlowRoot
}

// overflowIsScrollable reports whether either axis of overflow is something
// other than visible.
func overflowIsScrollable(cs style.ComputedStyle) bool {
	for _, axis := range [2]string{"overflow-x", "overflow-y"} {
		switch strings.ToLower(strings.TrimSpace(cs[axis])) {
		case "", "visible":
		default:
			return true
		}
	}
	return false
}

// twoValueDisplay reads the "outer inner" form.
func twoValueDisplay(value string) (Outer, Inner, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 {
		return 0, 0, false
	}
	var outer Outer
	switch parts[0] {
	case "block":
		outer = OuterBlock
	case "inline":
		outer = OuterInline
	default:
		return 0, 0, false
	}
	var inner Inner
	switch parts[1] {
	case "flow":
		inner = InnerFlow
	case "flow-root":
		inner = InnerFlowRoot
	case "flex":
		inner = InnerFlex
	case "table":
		inner = InnerTable
	default:
		return 0, 0, false
	}
	return outer, inner, true
}

// fixup adds the anonymous boxes the specification requires, depth first so a
// parent sees children that are already settled.
// The table repair runs first, and before the two flow rules rather than after
// them, because it is the rule that decides *which* boxes are in a flow at all.
// A stray cell inside a <span> becomes an inline-table there, and an
// inline-table is an atomic inline that does not split the span it is in; run
// the split first and the cell is lifted out as a block, taking the table it
// should have grown with it.
func (b *boxBuilder) fixup(box *Box) {
	for _, c := range box.Children {
		b.fixup(c)
	}
	box.Children = b.fixupTables(box)
	box.Children = b.splitBlockInInline(box)
	box.Children = b.wrapInlines(box)
}

// splitBlockInInline breaks an inline box apart around any block-level box
// inside it, per CSS 2.1 §9.2.1.1.
//
// "<span>before<div>block</div>after</span>" does not put a block inside an
// inline. The span is *split*: an inline piece holding "before", then the block,
// then a second inline piece holding "after", all three siblings of whatever the
// span was a child of. Both pieces keep the span's style, so a border on it is
// drawn twice — which is exactly what a browser does and looks odd until you
// know why.
//
// Without this the block would sit inside an inline formatting context, where
// nothing in block layout knows how to place it. It is common markup — an <a>
// wrapping a card of block content is the everyday case — so leaving it
// unhandled is not a corner.
func (b *boxBuilder) splitBlockInInline(parent *Box) []*Box {
	// There is deliberately no early return for an inline parent. One was
	// written here first, on the reasoning that inside an inline formatting
	// context there is nothing to promote a block *to* — and removing it changes
	// no output, because fixup runs depth first: a block promoted to be a direct
	// child of an inline is then lifted again by that inline's own parent, and
	// the sequence comes out the same either way.
	var out []*Box
	for _, child := range parent.Children {
		// Only an inline box that is *not* itself a block container splits. An
		// inline-block is inline outside and a block container inside, so a
		// block within it is where the author put it and lifting it out would
		// empty the very box that was meant to hold it.
		if child.Outer != OuterInline || laysOutOwnChildren(child) || !containsBlock(child) {
			out = append(out, child)
			continue
		}
		for _, piece := range splitInline(child) {
			piece.Parent = parent
			out = append(out, piece)
		}
	}
	return out
}

// isBlockContainer reports whether a box lays its children out as blocks and
// lines rather than taking part in someone else's line.
//
// It is the distinction the two rules above both turn on. A <div> is one. An
// inline-block is one despite being inline on the outside — that is the whole of
// what flow-root means. An ordinary <span> is not, and neither is a flex
// container or a table, which have rules of their own.
func isBlockContainer(b *Box) bool {
	switch b.Inner {
	case InnerFlowRoot, InnerTableCell, InnerTableCaption:
		// A cell and a caption are block containers that happen to live in a
		// table: their children are blocks and lines like any other, which is
		// why the anonymous block rule has to reach inside them.
		return true
	case InnerFlow:
		return b.Outer == OuterBlock
	}
	return false
}

// laysOutOwnChildren reports whether a box is opaque to the two flow rules
// above: whatever is inside it belongs to it, and neither splitting nor
// wrapping may reach past it.
//
// It differs from isBlockContainer at exactly the boxes that are not block
// containers and are not part of anyone else's inline formatting context
// either — an inline-table, an inline-flex. Those hold block-level boxes
// legitimately, and §9.2.1.1's split must not fire on one: an inline-table in
// the middle of a sentence would otherwise break the sentence in two and be
// lifted out as a block, which is the opposite of what "inline" asked for.
func laysOutOwnChildren(b *Box) bool {
	switch b.Inner {
	case InnerTable, InnerFlex:
		return true
	}
	return isBlockContainer(b)
}

// containsBlock reports whether an inline box has a block-level box anywhere
// inside it.
func containsBlock(b *Box) bool {
	for _, c := range b.Children {
		if c.outOfFlow() {
			// An out-of-flow box inside an inline box does not split it.
			// §9.2.1.1 is about a *block-level box in the normal flow* appearing
			// inside an inline formatting context, and neither a float nor an
			// absolutely positioned box is in the flow: they are placed beside
			// the line boxes rather than interrupting them. Splitting the inline
			// around one would draw the inline's border twice and put a line
			// break in the middle of a sentence — which is what the everyday
			// "position: absolute" tooltip inside a sentence would do.
			continue
		}
		if c.Outer == OuterBlock {
			return true
		}
		// The search stops at a nested block container. A block inside an
		// inline-block belongs to that box and is not something to be lifted
		// past it.
		if c.Outer == OuterInline && !laysOutOwnChildren(c) && containsBlock(c) {
			return true
		}
	}
	return false
}

// splitInline returns the sequence an inline box becomes once the blocks inside
// it have been lifted out: inline pieces and blocks, alternating.
//
// An inline piece with nothing in it is dropped. "<span><div>x</div></span>" is
// one block and no inline pieces at all, and keeping two empty spans either side
// would give the line boxes they generate a height the author never asked for.
//
// That last is a simplification and it has a cost worth stating. §9.4.2 makes an
// empty inline box with a non-zero horizontal margin, border or padding into a
// line box that is *not* zero-height, so a span whose only child is a block does
// keep its own padding-right somewhere — on an empty trailing piece. Dropping
// the piece drops the padding with it. The tests that show it are
// visuren/emptyspan-1 and -4 and normal-flow/block-in-inline-empty-001 and -004;
// keeping such a piece means deciding whether it is empty *after* its lengths
// are resolved, which is a layouter's question and not a box builder's.
//
// The insets that do survive go on the right pieces, which is §8.6's slice
// model: the left inset on the first piece and the right on the last.
func splitInline(b *Box) []*Box {
	var out []*Box

	piece := clonePiece(b)
	flush := func() {
		if len(piece.Children) == 0 {
			return
		}
		// Every piece after the first begins in the middle of the box, and
		// every piece is assumed to end in the middle until the split turns out
		// to be over. The last one is corrected below.
		piece.noLeadInset = len(out) > 0
		piece.noTrailInset = true
		out = append(out, piece)
		piece = clonePiece(b)
	}

	for _, child := range b.Children {
		switch {
		case child.outOfFlow():
			// Out of flow, so it does not interrupt the inline it sits in. It
			// stays with the piece so that the inline formatting context which
			// has to flow around it is the one it was written in — and, for an
			// absolutely positioned box, so that the place it was written is
			// still where its static position is taken from.
			child.Parent = piece
			piece.Children = append(piece.Children, child)

		case child.Outer == OuterBlock:
			flush()
			// The block leaves the inline, and what the inline was doing to it
			// has to leave with it. §9.2.1.1 breaks the inline *around* the
			// block, which does not stop the inline's own relative offset from
			// moving it: "left: 2em" on a span moves everything the span
			// contains, and the block it was broken around is still something
			// it contains. See splitFrom.
			child.splitFrom = append([]*Box{b}, child.splitFrom...)
			out = append(out, child)

		case child.Outer == OuterInline && !laysOutOwnChildren(child) && containsBlock(child):
			// A nested inline that itself holds a block: split it too, and its
			// inline pieces belong to this one's current piece.
			for _, inner := range splitInline(child) {
				if inner.Outer == OuterBlock {
					flush()
					// The inner split already named the inline it came out of;
					// this one is outside that, so it goes in front.
					inner.splitFrom = append([]*Box{b}, inner.splitFrom...)
					out = append(out, inner)
					continue
				}
				inner.Parent = piece
				piece.Children = append(piece.Children, inner)
			}

		default:
			child.Parent = piece
			piece.Children = append(piece.Children, child)
		}
	}
	flush()
	// The split is over, so whichever piece came last really does end the box.
	if n := len(out); n > 0 && out[n-1].Outer != OuterBlock {
		out[n-1].noTrailInset = false
	}
	// A box that begins or ends with a block still has a fragment there, and its
	// own left or right inset belongs to that fragment. Keeping it is what makes
	// "<span style='padding-right: 10px'><div>x</div></span>" reserve the ten
	// pixels the specification says it does. It is two boxes at most, whatever
	// the box contains.
	//
	// A box with no inset to carry keeps neither, and that is not an
	// optimisation. §9.4.2 says a line box holding nothing but an inline box
	// with *zero* margins, borders and padding "must be treated as not
	// existing", and a piece that exists is a piece an anonymous block gets
	// wrapped around — which would stand between two blocks and stop their
	// margins collapsing. Three tests in box-display say so, and they are the
	// reason this asks the question rather than always keeping the piece.
	if len(out) > 0 && mayInsetHorizontally(b.Style) {
		if out[0].Outer == OuterBlock {
			lead := clonePiece(b)
			lead.noTrailInset = true
			out = append([]*Box{lead}, out...)
		}
		if out[len(out)-1].Outer == OuterBlock {
			trail := clonePiece(b)
			trail.noLeadInset = true
			out = append(out, trail)
		}
	}
	return out
}

// mayInsetHorizontally reports whether a style could give an inline box a
// non-zero margin, border or padding on the horizontal axis.
//
// It is a syntactic question asked without a layouter, so it cannot resolve a
// length and does not try: anything that is not written as a zero counts as
// possibly non-zero. Being wrong that way keeps a fragment that turns out to
// need nothing, which costs a margin collapse; being wrong the other way would
// drop room the author asked for. The first is the direction to be wrong in,
// and the common cases — the property absent, or a reset stylesheet's
// "margin: 0" — are both answered exactly.
func mayInsetHorizontally(cs style.ComputedStyle) bool {
	for _, side := range [2]string{"left", "right"} {
		if !isZeroLength(cs["margin-"+side]) || !isZeroLength(cs["padding-"+side]) {
			return true
		}
		if !noBorder(cs["border-"+side+"-style"]) && !isZeroLength(cs["border-"+side+"-width"]) {
			return true
		}
	}
	return false
}

// isZeroLength reports whether a declaration is absent or written as a zero.
//
// "auto" is a zero here because that is what it computes to for the horizontal
// margin of an inline box: §10.3.1 gives an inline box's auto margins the value
// zero rather than sharing out the space, which is what makes an inline box
// uncentreable.
func isZeroLength(v string) bool {
	switch s := strings.ToLower(strings.TrimSpace(v)); s {
	case "", "0", "auto":
		return true
	default:
		n, ok := parseNumber(strings.TrimRight(s, "%abcdefghijklmnopqrstuvwxyz"))
		return ok && n == 0
	}
}

// clonePiece makes an empty copy of an inline box, keeping everything that
// decides how it is styled and measured.
//
// fontSizeKnown travels with FontSize because the two are one fact, and no
// document can tell: nothing asks ensureFontSize about an inline piece, so a
// clone that lost the flag would answer the same today. It is copied because a
// clone of a box the cascade sized is a box the cascade sized, and the day a
// piece reaches ensureFontSize a dropped flag would put sixteen pixels into a
// box that asked for none.
func clonePiece(b *Box) *Box {
	return &Box{
		Outer: b.Outer, Inner: b.Inner,
		Element: b.Element, Style: b.Style,
		FontSize: b.FontSize, fontSizeKnown: b.fontSizeKnown,
		ListItem: b.ListItem, Replaced: b.Replaced,
		Float: b.Float, Clear: b.Clear,
		Position: b.Position, ZIndex: b.ZIndex, ZAuto: b.ZAuto,
		Order: b.Order,
	}
}

// wrapInlines is the anonymous block box rule of CSS Display §2.1: when a block
// container has any block-level child, every run of inline-level children is
// wrapped in an anonymous block box.
//
// Without it a block container would have two kinds of child and layout would
// have to handle a mixture at every step. With it, a block container's children
// are either all block-level or all inline-level, which is the invariant the
// whole of block layout is written against.
func (b *boxBuilder) wrapInlines(parent *Box) []*Box {
	if len(parent.Children) == 0 {
		return parent.Children
	}
	// The rule applies to *block containers*, and an inline box is not one —
	// which matters more than it sounds. Wrapping an inline box's children in
	// anonymous blocks would leave a block inside an inline formatting context
	// by another name, and it would do it *before* splitBlockInInline could see
	// the inline content to split, so the pieces would come out empty and the
	// styling of the split element would vanish.
	//
	// An inline-block is a block container despite being inline outside, which
	// is the whole of what flow-root means here.
	if !isBlockContainer(parent) {
		return parent.Children
	}

	// An out-of-flow box does not count as a block-level child here, and does
	// not end a run of inline content either. It neither needs an anonymous
	// block around it nor forces one around the text beside it — and the text
	// beside it is exactly what has to stay in one inline formatting context,
	// because that context is what shortens its line boxes to make room for a
	// float. Wrapping the text on each side of a float in its own anonymous
	// block would give the float two separate contexts to flow around and
	// neither would be the one it was placed in.
	//
	// For an absolutely positioned box the consequence is different and just as
	// necessary: its static position is where it was written among the words,
	// and an anonymous block boundary drawn through that run would put it at the
	// start of a box that the author's sentence does not contain.
	hasBlock := false
	for _, c := range parent.Children {
		if c.Outer == OuterBlock && !c.outOfFlow() {
			hasBlock = true
			break
		}
	}
	if !hasBlock {
		// All inline: the parent is an inline formatting context and no
		// anonymous box is needed.
		return parent.Children
	}

	var out []*Box
	var run []*Box
	seenInFlow := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		// A run that is nothing but collapsible white space generates no
		// anonymous box. Otherwise "<div>\n<p>a</p>\n</div>" — whose newlines
		// are text nodes — would gain two anonymous blocks and two blank lines
		// that the author did not write and cannot see in the markup.
		//
		// An out-of-flow box in such a run is content and stays, but it does
		// not make the run into an inline formatting context: it is block-level
		// once §9.7 has blockified it, and nothing is left beside it for a line
		// box to hold. So it joins the parent's block-level children where it
		// was written rather than being sealed inside an anonymous block.
		//
		// That distinction is worth more than it looks, and it is why the
		// out-of-flow test was moved out of allWhitespace. An anonymous block
		// around a lone float *commits the pending margin* — it is an in-flow
		// box, so the margin above it is separated from the margin below —
		// which silently defeats the rule in layout.go that a float between two
		// paragraphs leaves their spacing alone. Every document in the suite
		// writes a newline between its elements, so the anonymous block was
		// there in every one of them and the rule almost never fired.
		if !hasInFlowContent(run) {
			for _, c := range run {
				if c.outOfFlow() {
					out = append(out, c)
				}
			}
			run = nil
			return
		}
		// An anonymous box inherits and has nothing else. Handing it the
		// parent's whole computed style would hand it the parent's margins,
		// padding, borders and background too — so the anonymous block around a
		// paragraph of text inside <body> would be indented by body's own
		// margin, and the gap it left would look like a deliberate one.
		anon := &Box{
			Outer: OuterBlock, Inner: InnerFlow,
			Style: style.Inherited(parent.Style), Parent: parent,
			FontSize: parent.FontSize, fontSizeKnown: parent.fontSizeKnown,
			Children: run,
			// §5.12.1's first *formatted* line is the parent's, wherever it
			// ends up: a block child splits the parent's inline content into
			// anonymous blocks, and the line the pseudo-element styles is the
			// first line of the first of them. afterTheFirstLine below is what
			// keeps it to that one.
			FirstLine: parent.FirstLine,
			// Anything in flow before this one means the parent's first line has
			// already happened, whether it was another anonymous block or a
			// block-level child of the parent's own.
			afterTheFirstLine: seenInFlow,
		}
		seenInFlow = true
		for _, c := range run {
			c.Parent = anon
		}
		out = append(out, anon)
		run = nil
	}

	for _, c := range parent.Children {
		if c.Outer == OuterBlock && !c.outOfFlow() {
			flush()
			out = append(out, c)
			seenInFlow = true
			continue
		}
		run = append(run, c)
	}
	flush()
	return b.houseInsideMarker(parent, out)
}

// houseInsideMarker gives an inside marker a line to be the first thing on,
// where the item's own content is block-level and offers none.
//
// §12.5.1 makes the marker "the first inline box in the principal block box,
// before the element's content", and the anonymous box rule has just cut that
// content into blocks. So the marker goes where the item's *first* inline
// content went: into the anonymous block holding it, or — where the item begins
// with a block, as "<li><ol>...</ol></li>" does — into an anonymous block of its
// own in front of it. list-style-position-023 and -024 are that second case, and
// their reference writes it out as "<div>1. <div>1. ...".
//
// Without this the item drew its marker and nothing else. The layout took the
// inline path on the strength of the marker alone, and every block child of the
// item — the whole of a nested list — was never laid out at all.
func (b *boxBuilder) houseInsideMarker(parent *Box, children []*Box) []*Box {
	if !markerInside(parent) {
		return children
	}
	for _, c := range children {
		// An out-of-flow box is not the item's content and does not get to
		// decide where the marker goes. No document can tell: wrapInlines only
		// ever emits an out-of-flow box straight into the parent's children,
		// never wrapped, so one met here is never the anonymous block the test
		// below is looking for and stopping at it would give the same answer.
		// It is the correct reading rather than a live branch, and a planted
		// defect that drops it moves nothing.
		if c.Outer != OuterBlock || c.outOfFlow() {
			continue
		}
		if c.Anonymous() && c.Inner == InnerFlow {
			// The item's own leading inline content, already wrapped. The
			// marker is the first inline box of it.
			c.InsideMarker = parent
			return children
		}
		break
	}
	// A block first, so the marker needs a block of its own. It is empty and it
	// is still a line: an inside marker makes a line box wherever it is, which
	// is what gives an item with no content at all its height.
	offset := 0
	if parent.Element != nil {
		offset = parent.Element.Offset
	}
	if !b.roomAt(offset) {
		return children
	}
	anon := &Box{
		Outer: OuterBlock, Inner: InnerFlow,
		Style: style.Inherited(parent.Style), Parent: parent,
		FontSize: parent.FontSize, fontSizeKnown: parent.fontSizeKnown,
		FirstLine: parent.FirstLine, InsideMarker: parent,
	}
	return append([]*Box{anon}, children...)
}

// hasInFlowContent reports whether a run of inline-level boxes holds anything
// that would put a line box on the page.
//
// The qualification about white space is the whole of it. CSS Display generates
// no anonymous block around white space that *collapses*, which is why an
// indented "<div>\n<p>a</p>\n</div>" gains no blank lines. White space that is
// preserved is content — a blank line inside a <pre> is a line the author wrote
// — so a run of it generates its box like any other text.
//
// An out-of-flow box is not in-flow content by definition: it is taken out of
// the flow, it generates no line box, and the run around it is empty without
// it. The caller keeps it and drops the run.
func hasInFlowContent(run []*Box) bool {
	for _, c := range run {
		if c.outOfFlow() {
			continue
		}
		if !c.IsText() {
			return true
		}
		if !whiteSpaceOf(c.Style["white-space-collapse"]).Collapse {
			return true
		}
		if strings.TrimSpace(c.Text) != "" {
			return true
		}
	}
	return false
}

// listValueOf reads the "list-item" counter for a list item.
//
// It is resolved at build time because that is when the counters are known: they
// depend on what came *before* an element in the document, which the layout walk
// cannot answer while descending. See counter.go.
func (b *boxBuilder) listValueOf(n *html.Node, listItem bool) (int, bool) {
	if !listItem {
		return 0, false
	}
	if vals := b.counters.elements[n]["list-item"]; len(vals) > 0 {
		return vals[len(vals)-1], true
	}
	return 0, false
}

// languageAt is the language in force at a node: the nearest lang attribute at
// or above it.
//
// It is read here rather than resolved through the cascade because it is not a
// CSS property — it is an HTML attribute, and the cascade carries no entry for
// it. The walk is the same one :lang() does in style/match.go, and is up the
// tree rather than down because a document usually declares its language once,
// on <html>.
//
// A text node has no attributes of its own, so the walk starts at its parent by
// construction: the loop below skips anything that is not an element.
func languageAt(n *html.Node) paragraph.Language {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Type != html.ElementNode {
			continue
		}
		if v, ok := cur.Attr("lang"); ok && v != "" {
			return paragraph.LanguageOf(v)
		}
	}
	return ""
}

// hyphenationAt is the key of the pattern table a node's words are divided
// with.
//
// The tag whole, as orthographyAt reads it and for the same reason: "zh-Latn" is
// romanised Chinese and divides between its syllables where "zh" is Han and does
// not. See paragraph.HyphenationOf, which is a different question from
// languageAt's and must not be answered with it.
func hyphenationAt(n *html.Node) paragraph.Language {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Type != html.ElementNode {
			continue
		}
		if v, ok := cur.Attr("lang"); ok && v != "" {
			return paragraph.HyphenationOf(v)
		}
	}
	return ""
}

// orthographyAt is the hyphenation orthography in force at a node.
//
// The tag whole, as writingSystemAt reads it and for the same reason: what
// decides is the script, and "zh-Latn" is romanised Chinese where "zh" is not.
// See paragraph.OrthographyOf.
func orthographyAt(n *html.Node) paragraph.Orthography {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Type != html.ElementNode {
			continue
		}
		if v, ok := cur.Attr("lang"); ok && v != "" {
			return paragraph.OrthographyOf(v)
		}
	}
	return paragraph.OrthographyPlain
}

// writingSystemAt is the writing system in force at a node, which is a
// different question from the language and is asked of the same attribute.
//
// languageAt cuts a tag down to its primary subtag, because that is what a
// casing tailoring is keyed on. The writing system is keyed on the *script*, so
// this reads the tag whole: "ain-Kana" is Ainu written in katakana and is
// typeset as Japanese, and "ja-Latn" is Japanese romanised and is not. See
// paragraph.WritingSystemOf.
func writingSystemAt(n *html.Node) paragraph.WritingSystem {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Type != html.ElementNode {
			continue
		}
		if v, ok := cur.Attr("lang"); ok && v != "" {
			return paragraph.WritingSystemOf(v)
		}
	}
	return paragraph.WritingSystemOther
}

// wordSpaceTransformFor reads word-space-transform off a computed style.
func (b *boxBuilder) wordSpaceTransformFor(cs style.ComputedStyle) paragraph.WordSpaceTransform {
	wst, _ := wordSpaceTransformOf(cs["word-space-transform"])
	return wst
}

// reportPhraseSeparators says that "auto-phrase" found no phrases in a node's
// text because there is no model for the language it is in.
//
// It is a question about the text and the content language rather than about
// the declaration, which is why it is here and not where the value is read.
// §2.2 answers two thirds of it for the UA — "if the content language is
// unknown, or if the user agent does not support detecting phrase boundaries for
// that language, there are no virtual expandable separators" — so a document
// that declares no language gets no separators and gets that *right*. What is
// left is a language whose phrases another UA would find and this one cannot,
// which today is Chinese. See paragraph.PhrasesUnfound.
func (b *boxBuilder) reportPhraseSeparators(n *html.Node, text string,
	wst paragraph.WordSpaceTransform) {

	if b.reportedPhraseSeparators || !wst.Invents() {
		return
	}
	if !paragraph.PhrasesUnfound(text, writingSystemAt(n)) {
		return
	}
	b.reportedPhraseSeparators = true
	b.rec.ReportDetail(Finding{
		Rule: RuleUnsupportedValue,
		Message: "\"auto-phrase\" in word-space-transform found no boundaries: " +
			"inventing a word separator where the document marked none needs a " +
			"phrase model for the language, and there is none here for this one, " +
			"so only the marks the document did write are expanded",
		Property: "word-space-transform",
		Path:     PathOf(n),
	})
}

// wordSpaceTransformValue is the same read without the node, for a caller that
// has a Box rather than the style it was built from.
func wordSpaceTransformValue(s map[string]string) paragraph.WordSpaceTransform {
	wst, _ := wordSpaceTransformOf(s["word-space-transform"])
	return wst
}
