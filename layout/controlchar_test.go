package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Control characters must be visible — CSS Text 3 §white-space-processing, and
// the title of sixty of the suite's documents.
//
// The engine set them as spaces and reported a missing glyph. The report was
// exact and the answer to it was to draw the thing, not to stop saying so.

// controlDoc is a paragraph with one control character in the middle of it.
func controlDoc(r rune) string {
	return `<div id="p">a` + string(r) + `b</div>`
}

const controlCSS = `#p { font-family: Courier; font-size: 20px }`

// paintedFor lays out and paints one document.
func paintedFor(t *testing.T, htmlSrc string) []Op {
	t.Helper()
	return Paint(layoutOf(t, 600, htmlSrc, controlCSS))
}

// TestAControlCharacterIsDrawn is the requirement in one line: something is on
// the page where it is.
func TestAControlCharacterIsDrawn(t *testing.T) {
	ops := paintedFor(t, controlDoc(rune(1)))
	var marks int
	for _, op := range ops {
		if r, ok := op.(FillRect); ok && !r.Rect.Empty() {
			marks++
		}
	}
	if marks == 0 {
		t.Error("U+0001 put nothing on the page; CSS Text 3 requires a control " +
			"character to be rendered as a visible glyph")
	}
}

// TestAControlCharacterIsNotDrawnAsText.
//
// Two reasons, and the second is the one that is easy to forget. No face has a
// glyph for it, so setting it emits .notdef — a box of the font's own beside the
// box drawn for it. And the character would go into the text extracted from the
// page, where a control character is exactly what a reader does not want back.
func TestAControlCharacterIsNotDrawnAsText(t *testing.T) {
	for _, r := range []rune{1, 7, 0x1F, 0x7F, 0x9F} {
		for _, op := range paintedFor(t, controlDoc(r)) {
			d, ok := op.(DrawText)
			if !ok {
				continue
			}
			if strings.ContainsRune(d.Text, r) {
				t.Errorf("U+%04X was drawn as text in %q", r, d.Text)
			}
		}
	}
}

// TestAControlCharacterKeepsItsAdvance.
//
// The face gives it the width of its .notdef and layout has already spent it, so
// the box is drawn *inside* that width and nothing moves. A document with one of
// these in it is laid out exactly as it was before this existed — which is the
// containment argument, and it is why this change touches no line breaking.
func TestAControlCharacterKeepsItsAdvance(t *testing.T) {
	withCtrl := layoutOf(t, 600, controlDoc(rune(1)), controlCSS)
	// Courier sets every glyph at 600/1000 of the em, .notdef included, so the
	// three characters are three of them.
	per, _ := style.FromPx(12)
	f := find(t, withCtrl, "p")
	var total style.Unit
	for _, line := range f.Lines {
		for _, r := range line.Runs {
			total = total.Add(r.Width)
		}
	}
	if want := per.Mul(3); total != want {
		t.Errorf("the line is %v wide and three Courier characters are %v; the "+
			"control character's advance changed", total, want)
	}
}

// TestAControlCharacterIsNoLongerReportedMissing: nothing asked a face for a
// glyph, so nothing is missing from the page, and saying otherwise would be the
// engine reporting a fault it has just fixed.
func TestAControlCharacterIsNoLongerReportedMissing(t *testing.T) {
	rec := NewRecorder(nil)
	built := Build(Input{
		HTML: controlDoc(rune(1)),
		CSS:  []Stylesheet{{Source: controlCSS}},
	})
	Layout(built.Root, Size{W: picPx(600), H: picPx(10000)}, nil, rec)
	for _, f := range rec.Findings() {
		if f.Rule == RuleGlyphMissing {
			t.Errorf("a control character that was drawn was reported missing: %s",
				f.Error())
		}
	}
}

// TestTabAndNewlineAreNotDrawnAsBoxes.
//
// The specification names them out of the rule, and each is white space whose
// meaning the white-space processing has already acted on. Form feed goes with
// them: CSS 2.1 counts it among the white space a document may be written with,
// so a box for one would be a mark on the page where an author put a page break
// in their source.
func TestTabAndNewlineAreNotDrawnAsBoxes(t *testing.T) {
	for _, r := range []rune{'\t', '\n', '\r', '\f'} {
		if isVisibleControl(r) {
			t.Errorf("U+%04X is treated as a visible control character", r)
		}
	}
	// And through the whole engine: a preserved tab draws no box.
	ops := Paint(layoutOf(t, 600,
		"<div id=\"p\">a\tb</div>",
		`#p { font-family: Courier; font-size: 20px; white-space: pre }`))
	for _, op := range ops {
		if r, ok := op.(FillRect); ok && !r.Rect.Empty() {
			t.Errorf("a preserved tab drew a mark at %v", r.Rect)
		}
	}
}

// TestTheBoxIsInsideTheCharactersOwnAdvance: it must not reach into the letters
// either side, or a page of ordinary text with one stray byte in it comes out
// with two characters struck through.
func TestTheBoxIsInsideTheCharactersOwnAdvance(t *testing.T) {
	root := layoutOf(t, 600, controlDoc(rune(1)), controlCSS)
	f := find(t, root, "p")
	per, _ := style.FromPx(12)
	// The control is the second of three characters, so its advance runs from
	// one character in to two.
	lo := f.ContentRect().X.Add(per)
	hi := lo.Add(per)
	var found int
	for _, op := range Paint(root) {
		r, ok := op.(FillRect)
		if !ok || r.Rect.Empty() {
			continue
		}
		found++
		if r.Rect.X < lo || r.Rect.Right() > hi {
			t.Errorf("a mark runs from %v to %v, outside the character's advance "+
				"of %v to %v", r.Rect.X, r.Rect.Right(), lo, hi)
		}
	}
	if found == 0 {
		t.Fatal("nothing was drawn, so this asserts nothing")
	}
}

// TestTheMarkTakesTheTextsColour, because it stands in for a character: a box
// in black on a page of grey text would read as something the document contains
// rather than as something it could not show.
func TestTheMarkTakesTheTextsColour(t *testing.T) {
	root := layoutOf(t, 600, controlDoc(rune(1)),
		`#p { font-family: Courier; font-size: 20px; color: #008000 }`)
	want := style.RGBA{R: 0, G: 128, B: 0, A: 1}
	var seen bool
	for _, op := range Paint(root) {
		if r, ok := op.(FillRect); ok && !r.Rect.Empty() {
			seen = true
			if r.Color != want {
				t.Errorf("the mark is %v and the text is %v", r.Color, want)
			}
		}
	}
	if !seen {
		t.Fatal("nothing was drawn")
	}
}
