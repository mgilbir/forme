package paragraph

import (
	"unicode/utf8"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// What a line is made of.
//
// An Item is one run of a paragraph as the breaking sees it: some text, the face
// and size it is set in, the width that comes to, and the flags §4.1.2 and §5
// need to decide where a line may end. It is a value — laying a paragraph out
// does not change it, which is what lets the balancer break the same items at a
// dozen candidate widths and keep the answer it likes.

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
	// Autospace is §8.1's ideograph spacing, already inside Width and sitting at
	// this run's far edge.
	//
	// It is kept apart so that a line that ends here can leave it out. The
	// spacing goes *between* two characters, and two characters on different
	// lines are not adjacent — so a gap the line break falls on is not a gap at
	// all, exactly as the letter-spacing after a line's last character is not
	// applied. Without it a run whose far edge carries an eighth of an em is an
	// eighth of an em too wide to end a line it fits on.
	Autospace style.Unit
	// EdgeLetterSpacing is the letter-spacing at this run's far edge — the
	// *logical* far edge, so the right of a left-to-right run and the left of a
	// right-to-left one. It is already inside Width.
	//
	// It is not Spacing.Letter. §8.2's gap between two characters in different
	// elements is the innermost element containing both of them, so the spacing
	// after a run's last character can be a value neither the run nor its box
	// declared: see layout/letterspacingboundary.go, which resolves it and folds
	// it into Width.
	//
	// It is recorded because the width alone cannot be taken apart afterwards,
	// and the gap has to be found again: it goes *between* two characters, so it
	// belongs to neither of the boxes they are in, and both where an inline
	// box's own edge is drawn and how far its background reaches depend on
	// knowing which part of a run's advance is the gap.
	//
	// Zero for anything that is not a run of spaced text, which includes a run
	// §8.2's cursive tracking took the spacing off entirely.
	EdgeLetterSpacing style.Unit

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
	// HangStart and HangEnd mark the character CSS Text §8.4 hangs outside the
	// line: an opening bracket or quote in the margin before the first formatted
	// line, and a closing one past the end of the last.
	//
	// They are two flags rather than one because they are two different things
	// to do. The end hang is white space's hang — it is on the line, it takes no
	// room, and the alignment discounts it — and carries Hangs as well so that
	// every stage which already knows about that does the right thing with no
	// second rule. The start hang is not a hang at the end of anything: what it
	// does is move the line's own beginning, which is where a negative
	// text-indent does its work and is where layout applies it.
	HangStart, HangEnd bool
	// MayHangEnd marks an item that is a single stop or comma which §8.4's
	// allow-end would hang, if hanging it is what lets the line hold it.
	//
	// It is a candidate rather than a decision, and the decision belongs to the
	// fill: the value hangs the character "only if it does not otherwise fit",
	// which is a question about a line and cannot be answered before the line is
	// known. The fill sets HangEnd and Hangs on its own copy when it takes it.
	//
	// The character is cut into an item of its own before the fill for the reason
	// hangingpunctuation.go gives for the other two values: a hang is a width,
	// and a width is a property of an item.
	MayHangEnd bool
	// PreContext and PostContext are the text either side of this run, where the
	// boundary between it and its neighbour does not break shaping.
	//
	// CSS Text §8.1: the boundary between two inline elements does not break
	// shaping, so "\u0639<span>\u0639</span>\u0639" is one joined Arabic word set
	// as three runs. A cursive letter takes its shape from its neighbours, and
	// without them each run is shaped alone and every letter comes out isolated —
	// which for a reader of the script is the difference between a word and three
	// letters standing apart.
	//
	// Empty for every run of every document not written in a joining script, and
	// the layer that fills them in does not even ask unless the face carries the
	// positional forms. See layout's linkShapingContext.
	PreContext, PostContext string
	// Hyphen is how much wider the line becomes if it ends after this item: the
	// width of the hyphen a soft hyphen asks to have printed. Zero means this is
	// not a hyphenation point, which is every item in almost every document.
	//
	// A width rather than a flag because this half of the engine has no faces to
	// ask. Which character is printed and how wide it is are questions about the
	// font, answered where the items are built; what the line breaking needs is
	// the number, and it needs it *before* it decides, since a line that can hold
	// nine characters and a hyphen cannot hold ten and a hyphen.
	Hyphen style.Unit
	// HyphenText is the character to print, carried with the width so that the
	// item the line breaking appends is one this package can build.
	//
	// It is U+2010 HYPHEN where the face has that glyph and U+002D HYPHEN-MINUS
	// where it does not, which is what CSS Text §6.1 allows and what the suite's
	// own fixtures expect — hyphens-manual-011 names two references, one for
	// each, because the two are different glyphs in some faces.
	HyphenText string
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
	// Edged says this inset item's own side of the box carries a non-zero
	// margin, border or padding — which is not the same as its Width being
	// non-zero, because they can cancel.
	//
	// §9.4.2 asks the first question and not the second: a line box is
	// zero-height only where it holds "no inline elements with non-zero
	// margins, padding, or borders", so a "border-left: 100px" against a
	// "margin-left: -100px" measures nothing and keeps its line. The pair
	// abspos/static-inside-inline-002 and -003 turns on exactly this, and on
	// which *side* of the box carries it: -002 puts the border on the leading
	// edge, where the fragment before an absolutely positioned box has it, and
	// -003 on the trailing edge, where that fragment does not.
	Edged bool
	// InsetLead distinguishes the two: the item before the box's content from
	// the item after it, in *logical* order.
	//
	// Which of them carries the box's left inset and which its right is not the
	// same question, and on a right-to-left line it is the other answer — see
	// insetSides for §8.6. The flag says where the item sits in the content, and
	// the width says what it holds.
	InsetLead bool
	// InsetLeft and InsetRight are the box's own horizontal margin, border and
	// padding, as the two *physical* values.
	//
	// Width carries one of them — which one is §8.6's question, answered by
	// insetSides once the levels are known — and that is the right number for
	// placing the item. It is the wrong number for asking whether there is room
	// between two runs, because which physical side faces a boundary depends on
	// which way the text runs and not on which end of the box the edge is. So
	// both are kept, and shapingcontext.go picks the one that is actually there.
	InsetLeft, InsetRight style.Unit
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
	// Offset is the relative displacement of the inline boxes the box was
	// written inside, carried for the same reason Item.Offset is: the
	// flattening loses the boxes themselves, and a float is placed long after
	// the walk that knew which inlines it was in.
	Offset Point
	// Abs distinguishes the two kinds.
	Abs bool
	// AfterContent says something was already on the line when the box was
	// reached — which is not the same as Used being non-zero, and the
	// difference is the whole reason it is here.
	//
	// §10.6.4's hypothetical box for an absolutely positioned box is
	// block-level, so it would have split the inline content it was written
	// among, and its top edge is the bottom of the line holding what precedes
	// it. What precedes it may take no width at all: an inline box's own edge
	// is content in §9.4.2's sense — the line it is on is not a zero-height one
	// — and a border cancelled by a negative margin measures nothing.
	// abspos/static-inside-inline-002 is exactly that, a "border-left: 100px"
	// against a "margin-left: -100px".
	AfterContent bool
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
	// AfterAtomic says the last thing emitted was an atomic inline — a
	// picture, an inline-block — so the opportunity after it is the one CSS
	// Text §5.1 grants around one, and is subject to that section's exception.
	//
	// It is distinguished from any other opportunity because the exception is:
	// a word joiner beside a picture holds it, and a word joiner beside a space
	// does not, so the two opportunities cannot be told apart by their own flag.
	AfterAtomic bool
	// AfterBinding says the last character emitted was one that holds on to an
	// atomic inline following it — see BindsToAtomicInline. It travels for the
	// same reason the rest of this does: "a&#8288;<span>b</span>" puts the word
	// joiner and the box in different text nodes.
	AfterBinding bool
	// AfterDeferred says the opportunity being carried is one an *ideograph*
	// offered rather than one a space did, and it is the difference between two
	// prohibitions that read alike.
	//
	// UAX #14's "a line may not begin with this character" is applied inside a
	// run only to the opportunities SplitAtBreaks defers — the ones between two
	// ideographs — and not to the one after a space: "AA )BB" breaks after the
	// space and always has. An opportunity crossing a box boundary has to be
	// held to the same rule as one that did not cross it, or "中中<span>〜</span>文"
	// and "中中〜文" answer differently about the same text.
	AfterDeferred bool
	// AfterLetterUnit says the last character emitted was a typographic letter
	// unit that is not itself an ideograph, which is what decides whether an
	// ideograph beginning the next box may be broken away from it. It travels
	// for the reason the rest of this does: the two characters of that boundary
	// are in different text nodes, and neither box can see both.
	AfterLetterUnit bool
	// AfterBox is the box the character before this point came from, held
	// opaquely, and it is here for a rule about *which element* decides.
	//
	// CSS Text §5.1: "for soft wrap opportunities defined by the boundary
	// between two characters, the white-space property on the nearest common
	// ancestor of the two characters controls breaking". An opportunity inside
	// one box is that box's to allow or refuse and needs nothing; one that
	// crosses a boundary belongs to neither side of it.
	//
	// Without this the answer was taken from the box the *later* character is
	// in, so a zero width space in a wrapping div between two "white-space: pre"
	// spans offered a break that the second span refused — and the span had
	// nothing to say about a boundary outside itself.
	//
	// The caller resolves it, because the ancestry is the caller's tree.
	AfterBox Ref
}

// StartOfContext is the state an inline formatting context begins in.
func StartOfContext() State { return State{AfterCollapsibleSpace: true} }

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
	// CSS Text §5.4, shaping across an intra-word break:
	//
	//	When shaping scripts such as Arabic are allowed to break within words
	//	due to hyphenation or [...] the characters must still be shaped as if
	//	the word were not broken.
	//
	// A cut is the one thing that makes a boundary where the text has none, so
	// each half is told what the other is. Without it "عائلة" broken by
	// overflow-wrap comes out as two words: the letter before the cut takes its
	// final form and the one after it takes its initial, and a reader of Arabic
	// sees a word that is not there. The suite says so twice, in
	// overflow-wrap-shaping-001 and -002, and writes the expectation as
	// presentation forms so that there is nothing to argue about.
	//
	// The context either side of the whole item goes on the outside of each
	// half, because that is where it was: what preceded the item still precedes
	// the head, and what followed it still follows the tail.
	head.PostContext = tail.Text + item.PostContext
	tail.PreContext = item.PreContext + head.Text
	head.Width = br.MeasureSpacedInContext(item.Face, head.Text, item.Size, item.Spacing,
		head.PreContext, head.PostContext)
	tail.Width = br.MeasureSpacedInContext(item.Face, tail.Text, item.Size, item.Spacing,
		tail.PreContext, tail.PostContext)
	// §8.1's gap sits at the item's far edge, so it goes with the tail — the
	// head's far edge is the cut, which is a boundary the gap was never at. The
	// measurements above do not include it, since it is not in the text.
	head.Autospace = 0
	tail.Autospace = item.Autospace
	tail.Width = tail.Width.Add(item.Autospace)
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
