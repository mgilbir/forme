package layout

import "github.com/mgilbir/forme/style"

// Painting what a formula draws that is not a box.
//
// A formula's boxes are painted as every box is: a token's text is a line of
// text, an <merror>'s frame a border, an <mphantom>'s content nothing, since
// it is hidden. What is left is what MathML Core has an element draw that is
// neither — a fraction bar, a radical's sign and its overbar, an operator
// stretched or enlarged past its text. Each is a mark of the element's own,
// drawn with its content, in its colour, and only where it is visible (§3.3.2.1
// and §3.3.3.1 say so of the bar and the radical in as many words, and
// §3.2.4.3 of an operator's text: "The text of the operator must only be
// painted if the visibility of the <mo> element is visible. In that case, it
// must be painted with the color of the <mo> element.").
//
// A rule is a FillRect. A radical sign or an operator that is its font's glyph
// for its character is a DrawText of that character; a size variant or an
// assembly is DrawGlyphs, since no character names it.
//
// Each mark was placed by the element's layout in its content box (see
// mathPlace), so it moves with the box whatever moves it afterwards.

// mathMarks paints a fragment's marks: its rules and its glyphs, in the box's
// colour.
func (p *painter) mathMarks(f *Fragment) {
	if len(f.mathMarks) == 0 && len(f.mathGlyphs) == 0 || isHidden(f.Box) {
		return
	}
	colour, ok := p.color(f.Box, "color")
	if !ok {
		colour = style.RGBA{A: 1}
	}
	c := f.ContentRect()
	for _, r := range f.mathMarks {
		rect := Rect{X: c.X.Add(r.X), Y: c.Y.Add(r.Y), W: r.W, H: r.H}
		if rect.Empty() {
			continue
		}
		p.emit(FillRect{Rect: rect, Color: colour})
	}
	for _, g := range f.mathGlyphs {
		at := Point{X: c.X.Add(g.at.X), Y: c.Y.Add(g.at.Y)}
		if g.glyphs == nil {
			p.emit(DrawText{At: at, Text: g.text, Face: g.face, Size: g.size, Color: colour})
			continue
		}
		// An assembly is as many glyphs as MaxMathAssemblyGlyphs allows, and
		// is charged to the work budget as that many marks: emit charges the
		// one, and this the rest.
		if n := len(g.glyphs) - 1; n > 0 && !p.rec.chargeMark(int64(n)*costOp, "the marks past that point") {
			continue
		}
		p.emit(DrawGlyphs{At: at, Text: g.text, Glyphs: g.glyphs, Face: g.face, Size: g.size, Color: colour})
	}
}
