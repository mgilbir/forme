package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// "unicode-bidi: plaintext" hands the direction to the text, and a paragraph
// with no strong character has nothing to hand over.
//
// UAX #9's P3 says "otherwise, set it to zero", which is the right answer for a
// paragraph nobody knows anything else about. Here there is something else:
// css-writing-modes gives such a paragraph the direction of the paragraph
// before it, and of the containing block where there is none — which is what a
// plain text editor does and is what the value is named after.
//
// bidi-lines-002 writes it out. Five lines of a right-to-left block, three of
// them nothing but "!", and its reference puts each of those three on the side
// the line before it was on: the first against the block's own direction, and
// the others against whichever line last had a letter in it.

// bangXs is where each "!" of a plaintext block is drawn, line by line.
func bangXs(t *testing.T, htmlSrc string) []style.Unit {
	t.Helper()
	return bangXsIn(t, htmlSrc, "rtl")
}

func bangXsIn(t *testing.T, htmlSrc, dir string) []style.Unit {
	t.Helper()
	css := `div { direction: ` + dir + `; unicode-bidi: plaintext;
		text-align: start; width: 20em; margin: 0 }`
	var out []style.Unit
	for _, op := range paintOf(t, htmlSrc, css) {
		if v, ok := op.(DrawText); ok && strings.Contains(v.Text, "!") {
			out = append(out, v.At.X)
		}
	}
	return out
}

func TestAPlaintextParagraphWithNothingStrongTakesTheBlocks(t *testing.T) {
	// One line, nothing but a neutral. The block is right-to-left, so the line
	// is: there is no paragraph before it to ask.
	got := bangXsIn(t, `<div>!</div>`, "rtl")
	ltr := bangXsIn(t, `<div>!</div>`, "ltr")
	if len(got) != 1 || len(ltr) != 1 {
		t.Fatalf("the fixtures drew %d and %d exclamation marks, want one each",
			len(got), len(ltr))
	}
	if got[0] <= ltr[0] {
		t.Errorf("in a right-to-left block the lone \"!\" is at x=%v and in a "+
			"left-to-right one at x=%v; with no paragraph before it, the block "+
			"decides", got[0], ltr[0])
	}
}

func TestAPlaintextParagraphTakesThePreviousOne(t *testing.T) {
	// Three lines: a Latin one, then two with nothing but "!". Both of the
	// latter follow a left-to-right paragraph and go to the left, against the
	// block's own right-to-left direction.
	got := bangXs(t, `<div>! Hello<br>!<br>!</div>`)
	if len(got) != 3 {
		t.Fatalf("the fixture drew %d exclamation marks, want three", len(got))
	}
	if got[1] != got[0] || got[2] != got[0] {
		t.Errorf("the three are at %v, %v and %v; the two that follow the Latin "+
			"line take its direction", got[0], got[1], got[2])
	}
}

func TestTheCarryFollowsTheLastStrongParagraph(t *testing.T) {
	// A Latin line, a bare one, an Arabic line, a bare one. The two bare lines
	// go opposite ways, each following the line above it.
	got := bangXs(t, `<div>! Hello<br>!<br>! سلام<br>!</div>`)
	if len(got) != 4 {
		t.Fatalf("the fixture drew %d exclamation marks, want four", len(got))
	}
	if got[1] != got[0] {
		t.Errorf("the second \"!\" is at x=%v and the Latin line's at x=%v", got[1], got[0])
	}
	if got[3] != got[2] {
		t.Errorf("the fourth \"!\" is at x=%v and the Arabic line's at x=%v", got[3], got[2])
	}
	if got[0] == got[2] {
		t.Errorf("the Latin and Arabic lines both put their \"!\" at x=%v; the "+
			"fixture is not telling the two directions apart", got[0])
	}
}

func TestAStrongParagraphIsUnaffected(t *testing.T) {
	// The carry must not override a paragraph that has a strong character of
	// its own: an Arabic line after a Latin one is still right-to-left.
	got := bangXs(t, `<div>! Hello<br>! سلام</div>`)
	if len(got) != 2 {
		t.Fatalf("the fixture drew %d exclamation marks, want two", len(got))
	}
	if got[1] <= got[0] {
		t.Errorf("the Arabic line's \"!\" is at x=%v and the Latin line's at "+
			"x=%v; P2 found a strong character and it decides", got[1], got[0])
	}
}
