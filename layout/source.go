package layout

import "github.com/mgilbir/forme/html"

// Which box painted each operation.
//
// The display list is enough to draw a page and not enough to say what is on
// it. A backend that writes a structured document — a tagged PDF, whose
// structure tree has the page's marked content for leaves; an accessibility
// export; text extracted with its headings and its tables — has to tie each
// mark back to the element it is the rendering of: this run of text is that
// <h2>'s, this picture is that <img alt=…>, these fills are that <td>'s border
// and not its words. Once the fragments are painted that connection is gone,
// and finding it again from geometry is the second layout engine the list
// exists to make unnecessary. So every operation carries it.
//
// It is a pointer to a box, the box tree Composed.Root's fragments were laid
// out from, and not a copy of what a backend might want from the element:
// what that is depends on the backend, and from a box it can reach all of it —
// the element and its attributes through Box.Element, the element's ancestors
// through html.Node.Parent, the boxes around it through Box.Parent, and which
// pseudo-element a generated box is through Box.Pseudo.
//
// What it is not is an order. The list is in paint order, which Appendix E,
// z-index and floats rearrange; the order a document is read in is the
// element tree's, and a backend derives it from the boxes this names.

// PaintedBy is the box that painted an operation, and what of the box the
// operation is.
//
// Every operation the painter emits has one, a group's included: Box is never
// nil and Part is never PartNone in a list that leaves Paint. The zero PaintedBy
// is what an operation built anywhere else has, and says only that nothing
// attributed it.
//
// It is not Source, which is where in the input a finding came from.
type PaintedBy struct {
	// Box is the box. For a run of text it is the text box the run came from,
	// which has no element of its own: its Parent is the inline or block box
	// of the element the text is in — see Element. For a list marker it is
	// the list item, for the canvas the root or <body> whose background became
	// the page's, and for a collapsed border's band the box whose border won
	// that stretch of grid line, which its colour and style were read from.
	Box *Box
	// Part is what of Box the operation is.
	Part Part

	// element is the element where it is not Box's to find: a Link of an <a>
	// with "display: contents", which has no box of its own. See Element.
	element *html.Node
	// node is the text node the operation draws, for the document's own
	// text. See Node.
	node *html.Node
}

// Node is the text node of the document an operation draws: the html.Node of
// type TextNode a run of text, or a formula's operator drawn as glyphs, was
// taken from. It is nil for everything else — text the engine generated
// (::before and ::after content, a list marker, a form control's value, an
// image's alternative text, a radical's sign), and every operation that is
// not the text itself: a shadow, an emphasis mark, a decoration line, a Link.
//
// It is what places a run among its element's children. Element says a run
// belongs to a <p>, and not where in the <p>; and paint order cannot say it
// either, since a child that makes a stacking context — transformed,
// positioned, translucent — is painted after the text on both sides of it.
// The node's place among its parent's Children is the run's place in the
// document. A text node broken across lines, or into runs of different
// faces, is several operations with one node.
//
// A formula's stretched operator takes the first text node its token holds,
// which is the whole of its text in every document but one that splits an
// operator's characters across comments or CDATA.
func (s PaintedBy) Node() *html.Node { return s.node }

// Element is the element the operation belongs to, which is an element of
// the document's tree and not of the box tree: Box's own element; for a run of
// the document's text, the element the text is in; for any other box no
// element generated — an anonymous block, generated text — that of the
// nearest box around it that one did. It is nil for the zero PaintedBy.
//
// The two trees differ at an element with "display: contents", which
// generates no box: "<span style='display: contents' lang=fr>bonjour</span>"
// puts the run's box straight under the paragraph's, and walking the boxes
// would declare "bonjour" in the paragraph's language. So a run of the
// document's text is resolved through its text node's parent, and a Link of
// an <a> with "display: contents" names the <a> itself.
//
// A generated box (::before, ::after) carries its originating element, so the
// text in one leads to that element; a backend that has to tell generated
// text from written text finds Box.Pseudo on the boxes from Box outwards.
func (s PaintedBy) Element() *html.Node {
	if s.element != nil {
		return s.element
	}
	for b := s.Box; b != nil; b = b.Parent {
		if b.Element != nil {
			return b.Element
		}
		for n := b.textNode; n != nil; n = n.Parent {
			if n.Type == html.ElementNode {
				return n
			}
		}
	}
	return nil
}

// Part is what of a box an operation is: what the document says, or how the
// box is dressed.
//
// The distinction is the one a structured document draws between content and
// artifact. A heading's words are the heading; its background, its border and
// the line under it are how it looks, and a reader that reads the page aloud
// reads the first and not the rest. So a backend needs it per operation, and
// cannot have it from the kind of operation: a FillRect is a background, a
// border, an underline, a shadow, and — for a picture of one colour — the
// picture.
type Part uint8

const (
	// PartNone is no part: the zero PaintedBy, which no operation the painter
	// emits has.
	PartNone Part = iota
	// PartContent is what the box has to say: a run of its text, its picture,
	// what an SVG or a formula draws. A Link is the content of the <a>.
	PartContent
	// PartMarker is a list item's marker, its bullet or its number, and the
	// picture list-style-image put in its place. It is generated rather than
	// written, which is why it is not PartContent: a reader that reads the
	// list reads it, and one that copies the item's text may not want it.
	PartMarker
	// PartBackground is the box's background: its colour, its images and
	// gradients. The page's background is the root's or <body>'s, which is
	// where it came from.
	PartBackground
	// PartBorder is the box's border, and for a table in the collapsing
	// model its stretches of the grid lines.
	PartBorder
	// PartOutline is the box's outline.
	PartOutline
	// PartDecoration is a line text-decoration draws — attributed to the box
	// that declared it, which is the box whose colour, thickness and position
	// it takes — or a run's emphasis marks, attributed to the run's box.
	PartDecoration
	// PartShadow is a shadow: a run's text-shadow, attributed to the run's
	// box, or what a drop-shadow() filter casts, attributed to the filtered
	// box.
	PartShadow
	// PartGroup is an operation that holds others: a ClipPath, a FilterGroup
	// or a TransformGroup. Box is the box it was made for — whose rounded
	// corners, filter or transform it is — and each operation inside it is
	// attributed in its own right.
	PartGroup
)

func (p Part) String() string {
	switch p {
	case PartNone:
		return "none"
	case PartContent:
		return "content"
	case PartMarker:
		return "marker"
	case PartBackground:
		return "background"
	case PartBorder:
		return "border"
	case PartOutline:
		return "outline"
	case PartDecoration:
		return "decoration"
	case PartShadow:
		return "shadow"
	case PartGroup:
		return "group"
	}
	return "unknown"
}

// OpPaintedBy is an operation's PaintedBy, for a backend that has an Op and not one
// of the kinds.
func OpPaintedBy(op Op) PaintedBy {
	switch v := op.(type) {
	case FillRect:
		return v.PaintedBy
	case DrawText:
		return v.PaintedBy
	case DrawTextShadow:
		return v.PaintedBy
	case DrawEmphasisMark:
		return v.PaintedBy
	case DrawGlyphs:
		return v.PaintedBy
	case DrawImage:
		return v.PaintedBy
	case TileImage:
		return v.PaintedBy
	case FillGradient:
		return v.PaintedBy
	case FillPath:
		return v.PaintedBy
	case ClipPath:
		return v.PaintedBy
	case FilterGroup:
		return v.PaintedBy
	case TransformGroup:
		return v.PaintedBy
	case Link:
		return v.PaintedBy
	}
	return PaintedBy{}
}

// attributed is op with s as its PaintedBy, where it has none yet: an operation
// a step inside the one that painted it already attributed — a decoration's
// line to the box that declared it — keeps what that step said. A group's
// Part is PartGroup whatever s says, since what a group is to its box is a
// group, and what is inside it says the rest.
func attributed(op Op, s PaintedBy) Op {
	switch v := op.(type) {
	case FillRect:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case DrawText:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case DrawTextShadow:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case DrawEmphasisMark:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case DrawGlyphs:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case DrawImage:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case TileImage:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case FillGradient:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case FillPath:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case Link:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = s
		}
		return v
	case ClipPath:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = PaintedBy{Box: s.Box, Part: PartGroup}
		}
		v.Ops = attributedAll(v.Ops, s)
		return v
	case FilterGroup:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = PaintedBy{Box: s.Box, Part: PartGroup}
		}
		v.Ops = attributedAll(v.Ops, s)
		return v
	case TransformGroup:
		if v.PaintedBy.Box == nil {
			v.PaintedBy = PaintedBy{Box: s.Box, Part: PartGroup}
		}
		v.Ops = attributedAll(v.Ops, s)
		return v
	}
	return op
}

// attributedAll is attributed for what a group holds: the operations a step
// put inside a group it made are the step's as much as those beside it — a
// rounded border's dashes cut to the curve, a tiling clipped to a rounded
// corner. The list is copied before it is written to, and only then, since a
// group's operations may be shared with a copy of the group that is not this
// one; most groups hold nothing left to attribute and are not copied at all.
func attributedAll(ops []Op, s PaintedBy) []Op {
	var out []Op
	for i, op := range ops {
		a := attributed(op, s)
		if out == nil && !sameAttribution(a, op) {
			out = append([]Op(nil), ops...)
		}
		if out != nil {
			out[i] = a
		}
	}
	if out == nil {
		return ops
	}
	return out
}

// sameAttribution reports whether attributing an operation left it as it was:
// its own PaintedBy, and the list of a group, which attributedAll returns
// unchanged when there was nothing in it to attribute.
func sameAttribution(a, b Op) bool {
	if OpPaintedBy(a) != OpPaintedBy(b) {
		return false
	}
	switch v := a.(type) {
	case ClipPath:
		return sameList(v.Ops, b.(ClipPath).Ops)
	case FilterGroup:
		return sameList(v.Ops, b.(FilterGroup).Ops)
	case TransformGroup:
		return sameList(v.Ops, b.(TransformGroup).Ops)
	}
	return true
}

// sameList reports whether two lists are one, the same backing array and
// length.
func sameList(a, b []Op) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// from paints a step and attributes to s every operation the step left in the
// list that nothing inside it attributed.
//
// It is a wrapper around the step, as clipping is, and for clipping's reason:
// what a step paints goes through decompositions — a dashed border is a dozen
// fills, a tiling is bands — and through passes that copy and rebuild
// operations, and teaching each of them which box it is drawing for would be
// a second copy of every one. What is read afterwards is what the step left,
// and an operation the step moved or rebuilt was attributed before it was.
//
// The steps it wraps are the painter's leaves — a box's background, its
// border, its content, one run of text — and none of them holds another, so
// no operation is read by two of these and the cost is one pass over the list.
//
// While the step runs s is p.src as well, which a step that makes many
// operations writes into them as it makes them: an operation is an interface
// value, and attributing one afterwards copies it, which for the runs and
// border bands of a long document is an allocation for each. An operation that
// arrives attributed is left as it is.
func (p *painter) from(s PaintedBy, paint func()) {
	at, outer := len(p.ops), p.src
	p.src = s
	paint()
	p.src = outer
	if s.Box == nil {
		return
	}
	for i := at; i < len(p.ops); i++ {
		switch p.ops[i].(type) {
		case ClipPath, FilterGroup, TransformGroup:
		default:
			if OpPaintedBy(p.ops[i]).Box != nil {
				continue
			}
		}
		p.ops[i] = attributed(p.ops[i], s)
	}
}
