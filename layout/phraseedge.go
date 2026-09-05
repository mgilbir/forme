package layout

import (
	"unicode/utf8"

	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/style"
)

// A phrase boundary that falls where an inline box does, CSS Text 4 §2.2.
//
// # What the rest of the engine already does
//
// "word-space-transform: ideographic-space auto-phrase" asks for two things:
// expand the separators the document wrote, and *invent* one at every phrase
// boundary it did not mark. The inventing is done in
// paragraph.InsertPhraseSeparators, over the text of one node, as part of the
// white space processing — which is the right place for a boundary inside a
// node and cannot see one anywhere else.
//
// Japanese is written without word spaces, so where the phrases are is a
// question about the whole sentence rather than about any run of it. The
// suite's word-space-transform-019 writes
//
//	東京<b><u>へ</u><em>行きましょ</em></b>う。
//
// whose one boundary is between へ and 行 — and that is exactly where <u> ends
// and <em> begins, so it is inside neither node and inside no node at all. The
// phrase model finds it in the sentence and finds nothing in any of the four
// pieces the sentence is cut into.
//
// # Where the separator goes
//
// §2.2: "If a phrase boundary is found at the same position as one or more
// inline box boundaries, the virtual word separator must be inserted in the
// outermost element that participates in this inline box boundary." Between
// </u> and <em> that is the <b> holding them both — the separator is drawn with
// the <b>'s background and with neither the <u>'s nor the <em>'s, which is what
// the suite's reference writes out by hand.
//
// It is the same answer §8.1's autospace and §8.2's letter-spacing give for a
// gap at a box edge: the element containing both characters decides, and
// commonAncestor is the walk all three use.
//
// # Why it is a box and not a width
//
// Because it is a character. A virtual word separator becomes a real U+3000: it
// is measured, it is drawn, a line may break at it, and it takes the background
// of the box it is in. The two gaps above are widths added to a run and could
// not do any of that — and appending this one to either neighbour's run would
// paint it in that neighbour's background, which is the one thing the rule
// above is about.
//
// So a text box is made for it, from a text node the document does not have.
// That is not new: an empty <wbr> becomes a zero width space the same way, and
// for the same reason — putting the invented character on the ordinary path
// rather than teaching every stage that a box can be a space.

// phraseSeparatorsAtABoxEdge inserts a separator at every phrase boundary that
// falls between two of a block's text boxes.
//
// It runs once per block-level box, over the inline subtree it holds, because a
// phrase is a question about a paragraph and a block-level box is where one
// ends. It does nothing at all unless something in that subtree asked for
// auto-phrase, which is almost every document.
func (b *boxBuilder) phraseSeparatorsAtABoxEdge(box *Box) {
	for _, seg := range inlineSegments(box) {
		b.separateSegment(seg)
	}
}

// textLeaf is one text box of a segment, with where its text begins in the
// segment's own text.
type textLeaf struct {
	box   *Box
	start int
}

// inlineSegments cuts a block's inline content into the stretches a phrase may
// run through.
//
// A stretch is broken by anything that is not a character: a block-level child,
// a float, an absolutely positioned box, a picture, an inline-block, a form
// control, a list marker, a <br>. Each of those stands between two phrases as
// surely as a full stop does, and running the model across one would join two
// sentences that a reader never sees joined. What a stretch *may* contain is
// inline box edges, which is the whole point — they are exactly what the
// node-by-node reading cannot see past.
func inlineSegments(box *Box) [][]textLeaf {
	var out [][]textLeaf
	var cur []textLeaf
	at := 0
	flush := func() {
		if len(cur) > 1 {
			out = append(out, cur)
		}
		cur, at = nil, 0
	}
	var walk func(*Box)
	walk = func(parent *Box) {
		for _, c := range parent.Children {
			switch {
			case c.IsText():
				cur = append(cur, textLeaf{box: c, start: at})
				at += len(c.Text)
			case plainInlineBox(c):
				walk(c)
			default:
				flush()
			}
		}
	}
	walk(box)
	flush()
	return out
}

// insertion is one separator waiting to be put in, held until the whole segment
// has been read: putting one in moves the children after it, and the offsets
// this walk is working from were taken before any of that.
type insertion struct {
	parent *Box
	at     int
	sep    *Box
}

// separateSegment finds the phrase boundaries of one stretch of inline content
// and inserts a separator at each one that falls between two of its boxes.
func (b *boxBuilder) separateSegment(seg []textLeaf) {
	text := ""
	for _, leaf := range seg {
		text += leaf.box.Text
	}
	var breaks map[int]bool
	var todo []insertion
	for i := 0; i+1 < len(seg); i++ {
		lhs, rhs := seg[i], seg[i+1]
		// The element containing both characters, which decides the property,
		// the language, and where the separator goes. All three are the same
		// element for the same reason.
		host := commonAncestor(lhs.box, rhs.box)
		if host == nil {
			continue
		}
		wst := wordSpaceTransformValue(host.Style)
		if !wst.Invents() {
			continue
		}
		prev, _ := utf8.DecodeLastRuneInString(lhs.box.Text)
		next, _ := utf8.DecodeRuneInString(rhs.box.Text)
		if !paragraph.PhraseSeparatorAt(prev, next) {
			continue
		}
		if breaks == nil {
			// Read once for the segment, and only once something in it has
			// asked: the model is the expensive part and almost no document
			// reaches it.
			breaks = paragraph.PhraseBreaks(text, boxWritingSystem(host))
			if len(breaks) == 0 {
				return
			}
		}
		if !breaks[rhs.start] {
			continue
		}
		sep := b.separatorBox(host, wst)
		if sep == nil {
			continue
		}
		if at, ok := childIndexHolding(host, lhs.box); ok {
			todo = append(todo, insertion{parent: host, at: at + 1, sep: sep})
		}
	}
	// Back to front, so that an index taken before any insertion is still the
	// index it names after the ones after it have gone in.
	for i := len(todo) - 1; i >= 0; i-- {
		put := todo[i]
		put.sep.Parent = put.parent
		kids := make([]*Box, 0, len(put.parent.Children)+1)
		kids = append(kids, put.parent.Children[:put.at]...)
		kids = append(kids, put.sep)
		kids = append(kids, put.parent.Children[put.at:]...)
		put.parent.Children = kids
	}
}

// separatorBox is the text box the invented separator becomes.
//
// It is built from a text node the document does not have, through the ordinary
// textBox path, so that the character is collapsed, transformed, measured,
// broken and drawn exactly as one the document wrote would be. The offset is the
// host element's, which is where a reader looking for it in the source would
// find the boundary.
func (b *boxBuilder) separatorBox(host *Box, wst paragraph.WordSpaceTransform) *Box {
	n := &html.Node{Type: html.TextNode, Text: wst.Separator, Offset: offsetOf(host)}
	return b.textBox(n, host.Style, hostFontSize(host))
}

// hostFontSize is the size the separator is set at, which is the box it goes
// into. A U+3000 is one em wide, so this is the whole of how wide the gap is.
func hostFontSize(host *Box) style.Unit {
	for p := host; p != nil; p = p.Parent {
		if p.fontSizeKnown {
			return p.FontSize
		}
	}
	return host.FontSize
}

// childIndexHolding is where in a box's children the subtree holding a
// descendant begins.
func childIndexHolding(parent, descendant *Box) (int, bool) {
	for p := descendant; p != nil; p = p.Parent {
		if p.Parent != parent {
			continue
		}
		for i, c := range parent.Children {
			if c == p {
				return i, true
			}
		}
	}
	return 0, false
}

// plainInlineBox reports whether a box is an inline box and nothing more: an
// <em> or a <span>, whose edges a phrase may run through.
//
// Two of the tests are asked of the *element* rather than of the box, and have
// to be: Box.Replaced is filled in a later pass once there is a resolver, so at
// build time an <img> is an inline box with no children and is otherwise
// indistinguishable from an empty <span>. See replacesItsOwnContent, whose
// documentation is this same paragraph from the other side, and endsAWord,
// which is the <br>.
func plainInlineBox(b *Box) bool {
	return b.Outer == OuterInline && b.Inner == InnerFlow && !b.outOfFlow() &&
		b.Replaced == nil && b.Control == nil && !b.ListItem && b.MarkerImage == nil &&
		!replacesItsOwnContent(b.Element) && !endsAWord(b.Element)
}
