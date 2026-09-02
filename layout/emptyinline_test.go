package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// An empty inline box is still on the line.
//
// §10.8.1 says it by name: "empty inline elements generate empty inline boxes,
// but these boxes still have margins, padding, borders and a line height, and
// thus influence these calculations just like elements with content". This
// engine did it for a box with an inset — insetItems has the two-hundred-pixel
// span in its own note — and not for one with nothing at all, so a
// "line-height: 5" span with no content and no border made no difference to the
// line it was on.
//
// §9.4.2 is the other half of the same sentence and it is where the limit is: a
// line box holding nothing *but* boxes like this "must be treated as a
// zero-height line box", and "as not existing for any other purpose". So the
// leading counts where there is something to count it beside, and a block whose
// whole content is empty inline boxes has no line at all.
//
// empty-inline-001 and -003 are the suite's two halves and they disagree on
// purpose.

// redBlockHeight is the height of the one red background a fixture paints.
func redBlockHeight(t *testing.T, htmlSrc, cssSrc string) style.Unit {
	t.Helper()
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if f, ok := op.(FillRect); ok && f.Color.R > 0.5 {
			return f.Rect.H
		}
	}
	return 0
}

const emptyInlineCSS = `div { line-height: 1; background: red } span { line-height: 5 }`

func TestAnEmptyInlineBoxRaisesTheLine(t *testing.T) {
	got := redBlockHeight(t, `<div><span></span>X</div>`, emptyInlineCSS)
	want := redBlockHeight(t, `<div><span>Y</span>X</div>`, emptyInlineCSS)
	if want == 0 {
		t.Fatal("the fixture with content painted nothing")
	}
	if got != want {
		t.Errorf("an empty span makes a %v line and one holding a letter makes "+
			"a %v line; §10.8.1 counts the empty box \"just like elements with "+
			"content\"", got, want)
	}
}

func TestALineOfNothingButEmptyInlineBoxesDoesNotExist(t *testing.T) {
	// §9.4.2. The block has no line, so no height and no background — which is
	// what empty-inline-001 draws.
	if got := redBlockHeight(t, `<div><span></span></div>`, emptyInlineCSS); got != 0 {
		t.Errorf("a block holding nothing but an empty span is %v tall, want 0: "+
			"§9.4.2 treats such a line as not existing", got)
	}
	if got := redBlockHeight(t, `<div><span></span><span></span></div>`, emptyInlineCSS); got != 0 {
		t.Errorf("two of them make a %v line, want 0", got)
	}
}

func TestAnEmptyBoxWithAnInsetWasAlreadyCounted(t *testing.T) {
	// The case insetItems has always handled, kept here so that the rule reads
	// as one rule: a border is content for §9.4.2 and the line exists.
	got := redBlockHeight(t, `<div><span style="border-left:1px solid"></span></div>`,
		emptyInlineCSS)
	if got == 0 {
		t.Errorf("a span with a border makes no line at all; §9.4.2 counts " +
			"\"inline elements with non-zero margins, padding, or borders\"")
	}
}

func TestAnEmptyBoxAgreesWithANonEmptyOne(t *testing.T) {
	// The change is that the two answer the same. A <b> is set in a bold face
	// whose ascent and descent differ from the block's, so its half-leading puts
	// its box a fraction off the block's own — and it does that whether or not
	// there is a letter inside it.
	empty := redBlockHeight(t, `<div>un<b></b>ken</div>`, `div { line-height: 2; background: red }`)
	full := redBlockHeight(t, `<div>un<b>x</b>ken</div>`, `div { line-height: 2; background: red }`)
	if empty != full {
		t.Errorf("an empty <b> makes a %v line and one holding a letter makes a "+
			"%v line; the box is the same box either way", empty, full)
	}
}
