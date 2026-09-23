package layout

import (
	"math/rand"
	"slices"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// Two walks up the inline boxes around a run, each asked for every run and each
// answered from scratch: which translucent box dims the run, when shaping asks
// whether two runs may share a glyph, and which boxes paint around it, when a
// line's backgrounds and borders are laid out. A paragraph of spans nested d
// deep with a word in each paid d walks of up to d boxes for both. Each is
// answered from the box's parent now, and is held here to the plain walk.

// translucentByWalking is the translucency of b, walked in full.
func translucentByWalking(b *Box) *Box {
	for cur := b; cur != nil && cur.Outer == OuterInline; cur = cur.Parent {
		if cur.Replaced != nil || isAtomicInline(cur) {
			return nil
		}
		if cur.IsText() {
			continue
		}
		if groupsItsPaint(cur) {
			return cur
		}
	}
	return nil
}

// paintedByWalking is paintedInlines(b), walked in full, outermost first.
func paintedByWalking(l *layouter, b *Box) []*Box {
	var out []*Box
	for cur := b; cur != nil && cur.Outer == OuterInline; cur = cur.Parent {
		if cur.Replaced != nil || isAtomicInline(cur) {
			break
		}
		if cur.IsText() {
			continue
		}
		if l.inlinePaints(cur) || cur.Position.positioned() {
			out = append(out, cur)
		}
	}
	slices.Reverse(out)
	return out
}

// randomInlineTree is a block holding a random tree of inline boxes: text,
// atomic inlines, replaced ones, a block among them, translucent boxes and boxes
// that paint or are positioned. Whether a box paints is set in l's memo of it,
// which is what paintedInlines asks.
func randomInlineTree(r *rand.Rand, l *layouter) []*Box {
	half := style.Initial().With("opacity", "0.5")
	root := &Box{Outer: OuterBlock, Style: style.Initial()}
	all := []*Box{root}
	var grow func(parent *Box, depth int)
	grow = func(parent *Box, depth int) {
		for k := r.Intn(4); k > 0 && depth < 7; k-- {
			b := &Box{Outer: OuterInline, Style: style.Initial(), Parent: parent}
			switch r.Intn(12) {
			case 0:
				b.Inner = InnerText
			case 1:
				b.Inner = InnerFlowRoot
			case 2:
				b.Replaced = &ReplacedContent{}
			case 3:
				b.Outer = OuterBlock
			case 4:
				b.Position = PositionRelative
			}
			if r.Intn(4) == 0 {
				b.Style = half
			}
			l.inlineDraws[b] = r.Intn(3) == 0
			all = append(all, b)
			if !b.IsText() {
				grow(b, depth+1)
			}
		}
	}
	grow(root, 0)
	r.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	return all
}

func TestTheInlineChainsAreWhatWalkingSays(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	translucent, painted := 0, 0
	for i := 0; i < 3000; i++ {
		l := &layouter{inlineDraws: map[*Box]bool{}, inlineChains: map[*Box][]*Box{}}
		tr := translucency{}
		for _, b := range randomInlineTree(r, l) {
			if got, want := tr.inline(b), translucentByWalking(b); got != want {
				t.Fatalf("tree %d: the translucent box around a box is %p, and walking "+
					"says %p", i, got, want)
			} else if want != nil {
				translucent++
			}
			if got, want := l.paintedInlines(b), paintedByWalking(l, b); !slices.Equal(got, want) {
				t.Fatalf("tree %d: the painted boxes around a box are %v, and walking "+
					"says %v", i, got, want)
			} else if len(want) > 1 {
				painted++
			}
		}
	}
	if translucent == 0 || painted == 0 {
		t.Fatalf("%d boxes were inside a translucent one and %d inside two that paint; "+
			"the trees are not exercising the walks", translucent, painted)
	}
}

// TestTheInlineChainsAreLinearInTheNesting asks both questions of every box of
// a chain of nested spans, each holding a word. The chain is built rather than
// parsed, because the parser stops nesting at 256 and the curve wants room.
func TestTheInlineChainsAreLinearInTheNesting(t *testing.T) {
	chain := func(depth int) (*layouter, []*Box) {
		l := &layouter{inlineDraws: map[*Box]bool{}, inlineChains: map[*Box][]*Box{}}
		parent := &Box{Outer: OuterBlock, Style: style.Initial()}
		var texts []*Box
		for i := 0; i < depth; i++ {
			span := &Box{Outer: OuterInline, Style: style.Initial(), Parent: parent}
			// Every tenth one paints, so that the chains are not all empty and
			// what is shared between them is something.
			l.inlineDraws[span] = i%10 == 0
			text := &Box{Outer: OuterInline, Inner: InnerText, Style: style.Initial(),
				Parent: span}
			texts = append(texts, text)
			parent = span
		}
		return l, texts
	}
	for _, q := range []struct {
		what string
		ask  func(l *layouter, texts []*Box)
	}{
		{"the translucent box around each word", func(_ *layouter, texts []*Box) {
			tr := translucency{}
			for _, b := range texts {
				tr.inline(b)
			}
		}},
		{"the painted boxes around each word", func(l *layouter, texts []*Box) {
			for _, b := range texts {
				l.paintedInlines(b)
			}
		}},
	} {
		t.Run(q.what, func(t *testing.T) {
			measure := func(depth int) func() {
				return func() {
					l, texts := chain(depth)
					q.ask(l, texts)
				}
			}
			c := costtest.Time(t, q.what, measure(500), measure(2000))
			if c.Ratio > 8 {
				t.Errorf("%s in a chain of 500 spans took %v and of 2000 took %v, a "+
					"factor of %.1f: linear is four and a walk per word is sixteen",
					q.what, c.Small, c.Large, c.Ratio)
			}
		})
	}
}
