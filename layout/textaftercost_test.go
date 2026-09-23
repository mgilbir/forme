package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// thaiBetweenBoxes is a block holding n × ("ก", an empty inline box), the
// shape of "<p lang=th>ก<span style='padding:0 1px'></span>ก…", and the text
// boxes in it.
func thaiBetweenBoxes(n int, tail string) (texts []*Box) {
	block := &Box{Outer: OuterBlock, Style: style.Initial()}
	for i := 0; i < n; i++ {
		text := &Box{Outer: OuterInline, Inner: InnerText, Text: "ก", Parent: block}
		span := &Box{Outer: OuterInline, Style: style.Initial(), Parent: block}
		block.Children = append(block.Children, text, span)
		texts = append(texts, text)
	}
	if tail != "" {
		block.Children = append(block.Children,
			&Box{Outer: OuterInline, Inner: InnerText, Text: tail, Parent: block})
	}
	return texts
}

// TestTextAfterStopsWhenItIsFull is the text a box ends before, which the
// dictionary breaking reads eighty bytes of after a Thai character.
//
// The walk stopped when it had n bytes. It cuts a text at a character boundary,
// though, so with two bytes left and a three-byte character next it took
// nothing, the count stayed short of n, and the walk went on to the end of the
// paragraph: laying out a thousand "ก" each followed by an empty padded span
// took 60 ms, and four thousand took 708. And what it went on to find could
// still be appended — an ASCII letter fits in two bytes — so the text it
// returned skipped the character that did not fit and carried on after it.
func TestTextAfterStopsWhenItIsFull(t *testing.T) {
	l := &layouter{}
	// Twenty-six "ก" are 78 bytes, the twenty-seventh does not fit in eighty,
	// and the "ab" after all of them would.
	texts := thaiBetweenBoxes(28, "ab")
	got := l.textAfter(texts[0], 80)
	if want := strings.Repeat("ก", 26); got != want {
		t.Errorf("the text after the first box is %q, want %q: what follows a box is "+
			"a prefix of what is written after it", got, want)
	}

	measure := func(n int) func() {
		texts := thaiBetweenBoxes(n, "")
		return func() {
			l := &layouter{}
			for _, b := range texts {
				l.textAfter(b, 80)
			}
		}
	}
	lo, hi, ratio := layoutScaling(measure(1000), measure(4000))
	if ratio > 8 {
		t.Errorf("the text after each of 1000 boxes took %v and of 4000 took %v, a "+
			"factor of %.1f: linear is four, and a walk to the end of the paragraph "+
			"per box is sixteen", lo, hi, ratio)
	}
}
