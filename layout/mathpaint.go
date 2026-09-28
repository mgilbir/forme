package layout

import "github.com/mgilbir/forme/style"

// Painting what a formula draws that is not a box.
//
// A formula's boxes are painted as every box is: a token's text is a line of
// text, an <merror>'s frame a border, an <mphantom>'s content nothing, since
// it is hidden. What is left is what MathML Core has an element draw that is
// neither — a fraction bar. It is a mark of the element's own, drawn with its
// content, in its colour, and only where it is visible (§3.3.2.1: "The
// fraction bar must only be painted if the visibility of the <mfrac> element
// is visible. In that case, the fraction bar must be painted with the color
// of the <mfrac> element.").
//
// Each mark was placed by the element's layout in its content box (see
// mathPlace), so it moves with the box whatever moves it afterwards.

// mathMarks paints a fragment's marks: each a rectangle filled in the box's
// colour.
func (p *painter) mathMarks(f *Fragment) {
	if len(f.mathMarks) == 0 || isHidden(f.Box) {
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
}
