package layout

import (
	"strings"
	"testing"
)

// A phrase boundary that falls where an inline box does, CSS Text 4 §2.2.
//
// Every test here asks *which box* the invented separator went into, not
// whether one exists. That is the whole of the rule — a U+3000 in the wrong box
// is a U+3000 in the right place on the page, drawn in the wrong background —
// and a test that only counted separators would pass for all three answers.

const edgePhraseCSS = `body, div, b, u, em { margin: 0; padding: 0 }
	div { word-space-transform: ideographic-space auto-phrase }`

// separatorBoxes returns, for every ideographic space in a document, the name of
// the element whose box holds it — whether the document wrote it or the property
// invented it.
func separatorBoxes(t *testing.T, htmlSrc, cssSrc string) []string {
	t.Helper()
	got := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}})
	if got.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	var out []string
	var walk func(*Box)
	walk = func(b *Box) {
		for _, c := range b.Children {
			if c.IsText() && strings.TrimLeft(c.Text, "　") == "" {
				name := "(anonymous)"
				if b.Element != nil {
					name = b.Element.Name
				}
				out = append(out, name)
			}
			walk(c)
		}
	}
	walk(got.Root)
	return out
}

// inventedSeparators is the separators the property added, which is the ones a
// document with "auto-phrase" has and the same document without it does not.
//
// It is a difference and not a count because a document may write an
// ideographic space of its own, and one of the tests below turns on exactly
// that: a separator already at the boundary is not doubled, and a helper that
// counted every U+3000 would call the author's own the engine's.
func inventedSeparators(t *testing.T, htmlSrc string) []string {
	t.Helper()
	plain := map[string]int{}
	for _, name := range separatorBoxes(t, htmlSrc,
		`body, div, b, u, em { margin: 0; padding: 0 }
		 div { word-space-transform: ideographic-space }`) {
		plain[name]++
	}
	var out []string
	for _, name := range separatorBoxes(t, htmlSrc, edgePhraseCSS) {
		if plain[name] > 0 {
			plain[name]--
			continue
		}
		out = append(out, name)
	}
	return out
}

// paragraphText is every character of a document's text boxes, in order, which
// is what a reader would copy off the page.
func paragraphText(t *testing.T, htmlSrc, cssSrc string) string {
	t.Helper()
	got := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}})
	if got.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	var b strings.Builder
	var walk func(*Box)
	walk = func(box *Box) {
		for _, c := range box.Children {
			if c.IsText() {
				b.WriteString(c.Text)
			}
			walk(c)
		}
	}
	walk(got.Root)
	return b.String()
}

// TestASeparatorGoesInTheOutermostBoxAtTheBoundary is word-space-transform-019
// written out, and the assertion is its own: "the virtual word separator must be
// inserted in the outermost element that participates in this inline box
// boundary".
//
// The boundary between へ and 行 is where <u> ends and <em> begins, so it is
// inside neither node and inside no node at all. It belongs to the <b> that
// holds them both — the separator takes the <b>'s background and neither of the
// other two, which is what the suite's reference draws by writing the character
// there by hand.
func TestASeparatorGoesInTheOutermostBoxAtTheBoundary(t *testing.T) {
	const doc = `<div lang=ja>東京<b><u>へ</u><em>行きましょ</em></b>う。</div>`
	got := inventedSeparators(t, doc)
	if len(got) != 1 {
		t.Fatalf("%d separators were invented, want 1 — the sentence has one "+
			"phrase boundary in it: %v", len(got), got)
	}
	if got[0] != "b" {
		t.Errorf("the separator went into <%s>; it belongs to the outermost "+
			"element at the boundary, which is the <b> holding both the <u> "+
			"that ends there and the <em> that begins there", got[0])
	}
	// And it is one character in the right place in the text, not two or none.
	if want := "東京へ　行きましょう。"; paragraphText(t, doc, edgePhraseCSS) != want {
		t.Errorf("the document reads %q, want %q",
			paragraphText(t, doc, edgePhraseCSS), want)
	}
}

// TestABoundaryInsideOneNodeIsStillDoneWhereItWas is the control for the path
// that already existed: a boundary with no box edge at it is inserted into the
// node's own text by the white space processing, and this change must not have
// moved it or done it twice.
func TestABoundaryInsideOneNodeIsStillDoneWhereItWas(t *testing.T) {
	const doc = `<div lang=ja>東京へ行きましょう。</div>`
	if want := "東京へ　行きましょう。"; paragraphText(t, doc, edgePhraseCSS) != want {
		t.Errorf("the document reads %q, want %q", paragraphText(t, doc, edgePhraseCSS), want)
	}
	if got := inventedSeparators(t, doc); len(got) != 0 {
		t.Errorf("%v separators were invented as boxes of their own; a boundary "+
			"inside a node goes into that node's text", got)
	}
}

// TestNothingIsInventedWithoutTheProperty. The initial value invents nothing,
// and a document that asks only for the separators it wrote gets only those.
func TestNothingIsInventedWithoutTheProperty(t *testing.T) {
	const doc = `<div lang=ja>東京<b><u>へ</u><em>行きましょ</em></b>う。</div>`
	for _, css := range []string{
		`body, div { margin: 0 }`,
		`body, div { margin: 0 } div { word-space-transform: ideographic-space }`,
	} {
		if got := separatorBoxes(t, doc, css); len(got) != 0 {
			t.Errorf("%q invented %v; only \"auto-phrase\" asks for separators the "+
				"document did not write", css, got)
		}
	}
}

// TestASeparatorAlreadyThereIsNotDoubled. §2.2 invents one only where the
// boundary has none, and the document may have written it as the very character
// that would be invented.
func TestASeparatorAlreadyThereIsNotDoubled(t *testing.T) {
	const doc = `<div lang=ja>東京<b><u>へ</u>　<em>行きましょ</em></b>う。</div>`
	if got := inventedSeparators(t, doc); len(got) != 0 {
		t.Errorf("%v were invented beside a separator the document already wrote", got)
	}
	if want := "東京へ　行きましょう。"; paragraphText(t, doc, edgePhraseCSS) != want {
		t.Errorf("the document reads %q, want %q", paragraphText(t, doc, edgePhraseCSS), want)
	}
}

// TestAPictureIsNotPartOfAPhrase. The model reads a stretch of text, and
// something that is not a character interrupts it: a picture between two halves
// of a sentence is not a character the phrase runs through, and joining the two
// halves across it would ask the model about a sentence nobody wrote.
func TestAPictureIsNotPartOfAPhrase(t *testing.T) {
	const doc = `<div lang=ja>東京<b><u>へ</u><img src="x.png"><em>行きましょ</em></b>う。</div>`
	if got := inventedSeparators(t, doc); len(got) != 0 {
		t.Errorf("%v were invented across a picture; it stands between the two "+
			"halves as surely as a full stop does", got)
	}
}

// TestAForcedBreakIsNotPartOfAPhrase. A <br> is a line the author ended, and a
// phrase does not continue across one — the same answer endsAWord gives for a
// word, and for the same reason.
func TestAForcedBreakIsNotPartOfAPhrase(t *testing.T) {
	const doc = `<div lang=ja>東京<b><u>へ</u><br><em>行きましょ</em></b>う。</div>`
	if got := inventedSeparators(t, doc); len(got) != 0 {
		t.Errorf("%v were invented across a forced break; the author ended the "+
			"line there and the sentence with it", got)
	}
}

// TestAnInlineBlockIsNotPartOfAPhrase. It is an atomic inline: one object on
// the line, whatever text is inside it, and the phrase either side of it is a
// different phrase.
func TestAnInlineBlockIsNotPartOfAPhrase(t *testing.T) {
	const doc = `<div lang=ja>東京<b><u>へ</u><span style="display:inline-block">x</span><em>行きましょ</em></b>う。</div>`
	if got := inventedSeparators(t, doc); len(got) != 0 {
		t.Errorf("%v were invented across an inline-block", got)
	}
}

// TestTheSeparatorIsTheOneTheValueNames. The property's other half chooses the
// character, and "space" asks for U+0020 where "ideographic-space" asks for
// U+3000. A separator invented at a box edge takes the same one as a separator
// invented inside a node, because there is one value and it is read once.
func TestTheSeparatorIsTheOneTheValueNames(t *testing.T) {
	const doc = `<div lang=ja>東京<b><u>へ</u><em>行きましょ</em></b>う。</div>`
	got := paragraphText(t, doc, `body, div, b, u, em { margin: 0; padding: 0 }
		div { word-space-transform: space auto-phrase }`)
	if want := "東京へ 行きましょう。"; got != want {
		t.Errorf("the document reads %q, want %q — \"space\" asks for U+0020", got, want)
	}
}

// TestTwoBoundariesInOneBoxBothLandWhereTheyBelong.
//
// "私は東京へ行きましょう。" has two phrase boundaries, and this document puts
// both of them at a box edge inside the same <b>. They are found by walking the
// children in order and put in afterwards — so the second insertion's index was
// taken before the first went in, and going front to back would move the second
// separator in front of the box it belongs after. Both come out in the right
// place or the sentence reads "私は　　東京へ行きましょう。".
func TestTwoBoundariesInOneBoxBothLandWhereTheyBelong(t *testing.T) {
	const doc = `<div lang=ja><b><u>私は</u><em>東京へ</em><i>行きましょう。</i></b></div>`
	got := inventedSeparators(t, doc)
	if len(got) != 2 {
		t.Fatalf("%d separators were invented, want 2: %v", len(got), got)
	}
	for i, name := range got {
		if name != "b" {
			t.Errorf("separator %d went into <%s>, want the <b> holding all three", i, name)
		}
	}
	if want := "私は　東京へ　行きましょう。"; paragraphText(t, doc, edgePhraseCSS) != want {
		t.Errorf("the document reads %q, want %q",
			paragraphText(t, doc, edgePhraseCSS), want)
	}
}
