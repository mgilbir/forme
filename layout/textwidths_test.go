package layout

import (
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/style"
)

// A text box measured on its own is as wide as its text.
//
// measureWidths dispatches on what the box is, and one of its arms is a text
// box. No document reaches that arm: the block-children loop below it skips
// everything that is not block-level, hasInlineChild routes any box with inline
// children to inlineWidths before it, and grid and flex items are blockified
// before they are measured — so a *text* box is never what contentWidths is
// handed. Counted rather than assumed: a counter on the arm stayed at nought
// across the whole of this package's tests and all 6253 documents of the reftest
// corpus.
//
// It is kept and tested rather than removed, and the difference matters. Without
// the arm a text box falls through to the block loop, which walks its children —
// it has none — and answers zero. A box sized to zero is not a wrong number in
// the way a crash is a wrong answer; it is a float that is a sliver, or a table
// column with nothing in it, and nothing would say why. The arm is the answer to
// a question nobody asks today and the wrong answer is silent, which is the pair
// that earns a test rather than a deletion.
//
// So the arm is exercised directly here. It is also the answer to a worry
// recorded when the dictionary lookahead landed — that intrinsic sizing and the
// fill might disagree about where a word divides, because textWidths gets one
// box with no neighbours. They cannot disagree, because nothing asks it.
func TestATextBoxMeasuresItsText(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}

	built := Build(Input{HTML: `<div id="d">abcd abcd</div>`,
		CSS: []Stylesheet{{Source: noDefaults +
			`#d { font-family: Courier; font-size: 16px }`}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	text := aTextBoxIn(built.Root)
	if text == nil {
		t.Fatal("the document produced no text box")
	}

	// The run's own state, built the way Layout builds it, so the measurement
	// goes through the memo and the font resolution a real one would.
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	l := newLayouter(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	got := l.contentWidths(text)

	// Courier is 600/1000, so a character at 16px is 9.6: "abcd abcd" laid out
	// on one line is nine of them and the widest unbreakable run is four.
	//
	// Against the numbers rather than against each other, because the fault this
	// is here for is both of them coming out nought.
	if got.max <= 0 || got.min <= 0 {
		t.Fatalf("a text box measured %v wide at most and %v at least; the arm "+
			"that measures one is gone and the block loop answered for a box "+
			"with no children", got.max, got.min)
	}
	if got.min >= got.max {
		t.Errorf("a text box of two words measured %v at least and %v at most; "+
			"the minimum is the widest word and the maximum is the line",
			got.min, got.max)
	}

	// And how far it agrees with the path documents actually take. The div holds
	// this text and nothing else, so the block measuring its inline content is
	// measuring the same characters — inlineWidths from one side, textWidths
	// from the other.
	//
	// The minimum agrees exactly. The maximum is a sixty-fourth of a pixel
	// apart: 5528 against 5529, which is the tiling difference between summing
	// separately quantized runs and quantizing the line. The two arms do not go
	// through the same walk — inlineWidths collects through collectInline with
	// a bidi builder and Measuring set, and this one calls itemsFor straight —
	// and one of those roundings happens once more than the other.
	//
	// It is pinned rather than fixed, and the distinction is the point. Nothing
	// reaches this arm, so nothing can see the difference; a "fix" would be a
	// change to an unreachable path chosen without a document to check it
	// against. What the bound is for is the day somebody routes a text box
	// through contentWidths and finds the two answers a unit apart — better to
	// have it written down and measured than rediscovered as a float a
	// sixty-fourth too narrow.
	block := l.contentWidths(built.Root.Children[0])
	if got.min != block.min {
		t.Errorf("the text box's minimum is %v and the block's is %v; the two "+
			"arms are looking at the same characters and this one has always "+
			"agreed", got.min, block.min)
	}
	if d := block.max.Sub(got.max); d < 0 || d > 1 {
		t.Errorf("the text box's maximum is %v and the block's is %v, %v apart; "+
			"they differ by one sixty-fourth of a pixel and no more — a larger "+
			"gap is a second measurement going wrong rather than a rounding",
			got.max, block.max, d)
	}
}

// aTextBoxIn is the first text box in a tree, in document order.
func aTextBoxIn(b *Box) *Box {
	if b == nil {
		return nil
	}
	if b.IsText() {
		return b
	}
	for _, c := range b.Children {
		if hit := aTextBoxIn(c); hit != nil {
			return hit
		}
	}
	return nil
}
