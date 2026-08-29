package layout

import "testing"

// A shown word separator at the edge of a line, and the inline box that hid it.
//
// word-space-transform makes a break opportunity visible, and §4.1.2's first
// rule says a line does not begin with the space it is made of: the suite's
// word-space-transform-010 writes "<wbr>あ<wbr>い<wbr>" fifteen ways and every
// one has to draw "あ　い" flush against its padding at both ends. The rule is
// about *collapsible* white space, and U+3000 is not — it is one of §4.1's
// other space separators — so the piece has to be marked as what it is rather
// than recognised from the character.
//
// The other half is an inline box's own edge. trimLineEdge skips one to reach
// the space in front of it, because a padding is not content and a space before
// it still ends the line; the intrinsic measurement stopped at it instead, and a
// float was as wide as a space more than the line it held. Neither half moves a
// reftest on its own and the two together move three: 010, 011 and 012.

// borderWidths is the widths of a fixture's horizontal border bands, which for a
// floated box with a border is how wide the float came out.
func borderWidths(t *testing.T, htmlSrc, cssSrc string) []string {
	t.Helper()
	var out []string
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if f, ok := op.(FillRect); ok && f.Rect.H.Px() == 3 {
			out = append(out, fmtPx(f.Rect.W))
		}
	}
	if len(out) == 0 {
		t.Fatalf("the fixture drew no border bands: %s", htmlSrc)
	}
	return out
}

const separatorCSS = `div { border: solid blue; float: left; clear: both }
	.show { word-space-transform: ideographic-space }
	.pad { padding: 0 1em }`

// sameWidth compares a document that writes its separators as marks against one
// that writes the same picture as characters. It is the reftest's own question
// and it needs no particular face: both halves are set in whatever face the test
// runs against.
func sameWidth(t *testing.T, name, marks, characters string) {
	t.Helper()
	got := borderWidths(t, marks, separatorCSS)
	want := borderWidths(t, characters, separatorCSS)
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("%s: %s came out %v and %s came out %v", name, marks, got,
			characters, want)
	}
}

func TestAShownSeparatorDoesNotBeginOrEndALine(t *testing.T) {
	sameWidth(t, "at the edges of the block",
		`<div class=show><wbr>a<wbr>b<wbr></div>`,
		`<div>a　b</div>`)
}

func TestAShownSeparatorInsideAnInlineBoxDoesNotEither(t *testing.T) {
	// The same three marks with the padding moved onto a span. The line begins
	// at the span's padding, which is not content, so the separator after it is
	// still at the beginning of the line — and the one before the closing
	// padding is still at the end.
	sameWidth(t, "at the edges of an inline box",
		`<div class=show><span class=pad><wbr>a<wbr>b<wbr></span></div>`,
		`<div><span class=pad>a　b</span></div>`)
}

func TestAZeroWidthSpaceIsTheSameMark(t *testing.T) {
	// U+200B and <wbr> are the two virtual word separators and the property
	// expands both. A document that writes one must measure as one that writes
	// the other.
	sameWidth(t, "written as characters",
		`<div class=show>&#x200b;a&#x200b;b&#x200b;</div>`,
		`<div class=show><wbr>a<wbr>b<wbr></div>`)
}

func TestAnInlineBoxsEdgeDoesNotEndTheLinesTrailingSpace(t *testing.T) {
	// The half that has nothing to do with the property: an ordinary collapsible
	// space before a span's closing padding is removed at the end of the line,
	// so the float is as wide as the text and the two paddings. It came out one
	// space wider.
	got := borderWidths(t, `<div><span class=pad>ab </span></div>`, separatorCSS)
	want := borderWidths(t, `<div><span class=pad>ab</span></div>`, separatorCSS)
	if got[0] != want[0] {
		t.Errorf("a float holding \"ab \" in a padded span came out %v and one "+
			"holding \"ab\" came out %v; §4.1.2 removes the space at the end of "+
			"the line and the padding after it is not content", got, want)
	}
}

func TestOnlyTheSeparatorsPieceIsMarked(t *testing.T) {
	ideographic := wordSpaceTransform{Separator: "　"}
	space := wordSpaceTransform{Separator: " "}
	pieces := []piece{
		{Text: "a"},
		{Text: "　", Space: true},
		{Text: "b"},
		{Text: " ", Space: true, Collapsible: true, TrimAtEnd: true},
		// Another of §4.1's other space separators, which the author wrote and
		// the property said nothing about. It is a space and it is not
		// collapsible, and marking it would let a line edge take away a
		// character somebody typed.
		{Text: "\u2003", Space: true},
		// A separator's character sitting in a piece that is not a space at all
		// — which cannot happen, and is here so that the test is about the two
		// conditions rather than about one of them.
		{Text: "　"},
	}
	got := collapsibleSeparators(append([]piece(nil), pieces...), ideographic)
	if !got[1].Collapsible || !got[1].TrimAtEnd {
		t.Errorf("the separator's piece was left as %+v; it is what §4.1.2's "+
			"first rule has to be able to take away", got[1])
	}
	if got[0].Collapsible || got[2].Collapsible || got[4].Collapsible ||
		got[5].Collapsible {
		t.Errorf("a piece that is not the separator was marked: %+v", got)
	}
	// "space" needs nothing: an ordinary space is already both.
	same := collapsibleSeparators(append([]piece(nil), pieces...), space)
	for i := range same {
		if same[i] != pieces[i] {
			t.Errorf("piece %d changed under \"space\": %+v became %+v", i,
				pieces[i], same[i])
		}
	}
	// And with the property off, nothing at all.
	off := collapsibleSeparators(append([]piece(nil), pieces...), wordSpaceTransform{})
	for i := range off {
		if off[i] != pieces[i] {
			t.Errorf("piece %d changed with the property off: %+v became %+v", i,
				pieces[i], off[i])
		}
	}
}
