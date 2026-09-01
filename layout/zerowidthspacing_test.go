package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §8.2's letter-spacing and the characters it is not added after.
//
// A zero-width formatting character — a bidi control, a joiner, an invisible
// operator — is not a typographic character unit, so no spacing follows it.
// paragraph.SpacedUnits has always said so and the measurement of a *run* has
// always agreed. What did not agree is everything that had to find the spacing
// at the end of a line, because a run made only of those characters answered
// "no spacing here" and was taken to mean "no spacing at all" — so the spacing
// after the letter in front of it stopped hanging and started counting.
//
// The suite writes it as letter-spacing-202, which puts twenty-eight of them on
// each side of two letters.

// zw is the run of formatting characters letter-spacing-202 uses.
const zw = "\u200B\u200C\u200D\uFEFF\u200E\u200F\u2066\u2067\u2068\u202A\u202B\u202C\u202D\u202E\u2069\u2069\u206A\u206B\u206C\u206D\u206B\u206E\u206F\u2060\u2061\u2062\u2063\u2064"

// spacedFloatWidth is the width a float shrink-wraps to around some text set
// with 20px of letter-spacing in a 20px monospace face, where a character is
// 12px wide.
func spacedFloatWidth(t *testing.T, body string) (w, h float64) {
	t.Helper()
	root := layoutOf(t, 400,
		`<div id="d" style="float:left;letter-spacing:20px">`+body+`</div>`,
		`body{margin:0} #d{font: 20px/1 monospace}`)
	r := find(t, root, "d").BorderRect
	return r.W.Px(), r.H.Px()
}

// TestAFloatIsNotWidenedByTheSpacingAfterAZeroWidthCharacter.
func TestAFloatIsNotWidenedByTheSpacingAfterAZeroWidthCharacter(t *testing.T) {
	plain, plainH := spacedFloatWidth(t, "xx")
	if plain != 44 || plainH != 20 {
		t.Fatalf("two letters with 20px of tracking shrink-wrap to %gx%g, want 44x20 "+
			"— 12px a character and one 20px gap between them, with the trailing "+
			"spacing hanging past the end; the fixture cannot say what it means to say",
			plain, plainH)
	}
	for _, c := range []struct{ what, body string }{
		{"in front and behind", zw + "xx" + zw},
		{"between the letters as well", zw + "x" + zw + "x" + zw},
		{"behind only", "xx" + zw},
	} {
		w, h := spacedFloatWidth(t, c.body)
		if w != plain {
			t.Errorf("with formatting characters %s the float is %gpx wide, want %g "+
				"— nothing is drawn for them and no spacing goes after them",
				c.what, w, plain)
		}
		if h != plainH {
			t.Errorf("with formatting characters %s the float is %gpx tall, want %g "+
				"— the letters still fit on one line", c.what, h, plainH)
		}
	}
}

// TestTheInkOfARunSkipsTheSpacingAfterAZeroWidthCharacter.
//
// The other half, and the one that decides whether a run is judged cut or
// buried: how far its ink reaches. Counting a spacing for each formatting
// character put the end of a two-letter run twenty-six trackings past the page.
//
// The run is built here rather than laid out because that is the only way to be
// sure the letters and the formatting characters are in *one* run: where layout
// happens to cut them into several, each piece is measured on its own and the
// mistake has nowhere to show.
func TestTheInkOfARunSkipsTheSpacingAfterAZeroWidthCharacter(t *testing.T) {
	face := ahemFace(t)
	size, _ := style.FromPx(20)
	spacing, _ := style.FromPx(20)
	ink := func(text string) style.Unit {
		return textInk(DrawText{
			Text: text, Face: face, Size: size, CharSpacing: spacing,
			Color: style.RGBA{A: 1},
		}).W
	}
	plain := ink("xx")
	if plain.Px() != 80 {
		t.Fatalf("two Ahem letters at 20px with 20px of tracking ink %gpx, want 80; "+
			"the fixture cannot say what it means to say", plain.Px())
	}
	for _, c := range []struct{ what, text string }{
		{"between them", "x" + zw + "x"},
		{"in front", zw + "xx"},
		{"behind", "xx" + zw},
	} {
		if got := ink(c.text); got != plain {
			t.Errorf("with formatting characters %s the run inks %gpx, want %g — "+
				"a character nothing is drawn for carries no tracking",
				c.what, got.Px(), plain.Px())
		}
	}
}

// TestABlockGlyphRunIsRebuiltWithoutTheSpacingItNeverHad.
//
// The comparison turns a run of rectangle glyphs back into rectangles by
// walking the run and adding the advance and the tracking of each character.
// Adding the tracking of a character that has none put those rectangles a
// tracking further along for every formatting character in front of them, which
// is a picture the engine never drew and a difference the oracle invented.
func TestABlockGlyphRunIsRebuiltWithoutTheSpacingItNeverHad(t *testing.T) {
	face := ahemFace(t)
	bf := ahemBlockFont(t)
	size, _ := style.FromPx(20)
	spacing, _ := style.FromPx(20)
	at := Point{}
	run := func(text string) []Op {
		v := DrawText{At: at, Text: text, Face: face, Size: size, CharSpacing: spacing,
			Color: style.RGBA{A: 1}}
		fills, ok := bf.fills(v, face.Measure)
		if !ok {
			t.Fatalf("the run %q could not be rebuilt as rectangles", text)
		}
		return fills
	}
	plain, padded := run("xx"), run(zw+"xx")
	if len(plain) != len(padded) {
		t.Fatalf("the two runs rebuilt to %d and %d rectangles; the formatting "+
			"characters draw nothing and should add none", len(plain), len(padded))
	}
	for i := range plain {
		p, q := plain[i].(FillRect), padded[i].(FillRect)
		if p.Rect != q.Rect {
			t.Errorf("rectangle %d is at %v with the formatting characters and at "+
				"%v without them; no tracking goes after a character nothing is "+
				"drawn for", i, q.Rect, p.Rect)
		}
	}
}
