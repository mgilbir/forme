package layout

import (
	"strings"
	"testing"
)

// splitInline's recursive arm: an inline holding an inline that holds a block.
//
// §9.2.1.1 lifts a block out of the inline it was written in, and splitInline
// does it. One of its arms handles a child that is itself an inline with a block
// somewhere inside — it splits that child too, keeps the pieces in the current
// piece, and lets the blocks out to the top.
//
// No document reaches it, and the reason is in fixup: it recurses into children
// before settling a box, so by the time a parent is split its inline children
// have already been flattened and hold no blocks. splitBlockInInline's own
// comment says the two orders "come out the same either way". Counted rather
// than assumed — the whole arm is four blocks and all four were at nought across
// this package's tests and all 6253 reftest documents, including the nested
// fixture TestNestedInlinesSplitToo, which produces its answer by the other
// route entirely.
//
// It is kept and tested rather than removed, and the difference is the same one
// as for the intrinsic-sizing text arm. Without it such a child falls to the
// default case and is put in the current piece with its block still inside it —
// a block-level box left in an inline formatting context, which is the one thing
// the whole rule exists to prevent, and nothing would report it.
//
// So the arm is exercised directly, on a tree built by hand because the builder
// cannot produce one.

// nestedInlineWithABlock is
//
//	outer: "a", inner: "b", <block>"c"</block>, "d", "e"
//
// with the block two inlines deep, which is the shape splitInline's recursive
// arm is for and the shape fixup never hands it.
func nestedInlineWithABlock() (outer, inner, block *Box) {
	text := func(s string) *Box {
		return &Box{Outer: OuterInline, Inner: InnerText, Text: s}
	}
	block = &Box{Outer: OuterBlock, Inner: InnerFlow}
	block.Children = []*Box{text("c")}

	inner = &Box{Outer: OuterInline, Inner: InnerFlow}
	inner.Children = []*Box{text("b"), block, text("d")}
	block.Parent = inner

	outer = &Box{Outer: OuterInline, Inner: InnerFlow}
	outer.Children = []*Box{text("a"), inner, text("e")}
	inner.Parent = outer
	return outer, inner, block
}

// shapeOf renders a split result as one line per box, so an ordering is readable
// when it is wrong.
func shapeOf(out []*Box) string {
	var b strings.Builder
	var walk func(x *Box, depth int)
	walk = func(x *Box, depth int) {
		b.WriteString(strings.Repeat("  ", depth))
		switch {
		case x.IsText():
			b.WriteString("text " + x.Text)
		case x.Outer == OuterBlock:
			b.WriteString("block")
		default:
			b.WriteString("inline")
		}
		b.WriteString("\n")
		for _, c := range x.Children {
			walk(c, depth+1)
		}
	}
	for _, x := range out {
		walk(x, 0)
	}
	return b.String()
}

func TestANestedInlineHoldingABlockIsSplitToo(t *testing.T) {
	outer, _, _ := nestedInlineWithABlock()
	got := shapeOf(splitInline(outer))
	want := `inline
  text a
  inline
    text b
block
  text c
inline
  inline
    text d
  text e
`
	if got != want {
		t.Errorf("the nested split gave:\n%s\nwant:\n%s", got, want)
	}
}

// The block must come out at the top, not stay inside a piece.
//
// Stated on its own because it is the whole point of the rule and the thing the
// default case would silently get wrong: a block left inside an inline piece is
// a block-level box in an inline formatting context, which nothing downstream
// knows how to place.
func TestTheNestedBlockLeavesEveryInline(t *testing.T) {
	outer, _, block := nestedInlineWithABlock()
	out := splitInline(outer)

	atTop := false
	for _, x := range out {
		if x == block {
			atTop = true
		}
		var walk func(*Box) bool
		walk = func(y *Box) bool {
			for _, c := range y.Children {
				if c == block || walk(c) {
					return true
				}
			}
			return false
		}
		if x != block && walk(x) {
			t.Errorf("the block is still inside an inline piece; §9.2.1.1 " +
				"lifts it out of every inline it was written in, and a block " +
				"in an inline formatting context has nowhere to be placed")
		}
	}
	if !atTop {
		t.Errorf("the block is not among the %d boxes the split returned", len(out))
	}
}

// splitFrom is outermost first, and both ends of it are read.
//
// Two consumers depend on the order from opposite directions, which is what
// makes it worth pinning rather than noting:
//
//   - the painter walks it outermost first, because an opacity group declared on
//     the outer inline encloses one declared on the inner, and a nested group
//     built the other way round composites in the wrong order;
//   - decorationsFor takes splitFrom[len-1] as the innermost, because that one
//     answers for the whole chain — its own walk goes on up through the boxes
//     the block would otherwise have reached.
//
// Appending rather than prepending at the recursive arm would reverse it and
// satisfy neither.
func TestTheNestedBlockNamesItsInlinesOutermostFirst(t *testing.T) {
	outer, inner, block := nestedInlineWithABlock()
	splitInline(outer)

	if len(block.splitFrom) != 2 {
		t.Fatalf("the block names %d inlines, want 2: it was written inside "+
			"both, and each still moves and decorates it", len(block.splitFrom))
	}
	if block.splitFrom[0] != outer {
		t.Errorf("the first inline named is not the outer one; the painter " +
			"walks this outermost first, so an opacity group on the outer " +
			"inline must enclose one on the inner")
	}
	if block.splitFrom[1] != inner {
		t.Errorf("the last inline named is not the inner one; decorationsFor " +
			"takes the last as the innermost, and it answers for the whole chain")
	}
}
