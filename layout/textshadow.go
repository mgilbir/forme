package layout

import (
	"fmt"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// CSS Text Decoration 3 §4: text-shadow.
//
// # What is drawn
//
// Each shadow is the run's text and every decoration drawn across it, moved
// by the shadow's offset, in the shadow's colour, and blurred. "The first shadow
// is on top", and all of them are under the text: §5.1's order is shadows, then
// underlines and overlines, then the text, then line-throughs. Which box a
// run's shadows are read from is the run's own, since the property inherits and
// "the green text shadow on the <span> overrides the blue text shadow on the
// <div>" — decorations are the declaring box's, and shadows are not.
//
// §4 leaves it "undefined whether a given shadow layer shadows each glyph or
// decoration independently or if the text and/or decorations are flattened and
// then shadowed". Each is shadowed on its own here: a run's glyphs as one mark,
// each decoration line as another.
//
// # What the display list says
//
// A shadow of glyphs is DrawTextShadow: the run as shadowed, and the blur. It is
// its own operation rather than a DrawText in the shadow's colour for the sake
// of the text a reader extracts from the page — a shadow is not text, and a
// DrawText is — and it is not a DrawText inside a FilterGroup for the same
// reason. A shadow of a decoration is a rectangle, so it is a FillRect when it
// is not blurred, which is exactly what it is, and a FillRect inside a
// FilterGroup when it is.
//
// The blur is stated as a standard deviation, as FilterGroup's is: §4 reads the
// blur radius as CSS Backgrounds 3 §7.1.1 does a box shadow's, a Gaussian of
// deviation half the radius.

// DrawTextShadow draws one shadow of a run of text.
//
// Run is the run as the shadow is drawn: moved by the shadow's offset and in
// the shadow's colour, and otherwise the run it shadows, so a backend shapes
// and places it exactly as it would the run. It is not text: nothing of it
// belongs to what a reader copies out of the page.
//
// StdDev is the blur's standard deviation, half the blur radius text-shadow was
// written with; zero is a sharp shadow, which is the run's glyphs in the shadow's
// colour. A PDF backend has no blur, so a blurred shadow is rasterised by the
// backend or refused and reported, as a FilterGroup's is. Run.Clip, when it is
// set, is applied after the blur.
type DrawTextShadow struct {
	Run    DrawText
	StdDev style.Unit
}

func (DrawTextShadow) isOp() {}

// shadowInk is where a shadow may put ink: its run's reserved box, grown by
// three deviations of its blur. See blurReach.
func shadowInk(v DrawTextShadow) Rect {
	r := textInkReserved(v.Run)
	if v.StdDev > 0 {
		d := v.StdDev.Mul(blurReach)
		r = r.Outset(Edges{Top: d, Right: d, Bottom: d, Left: d})
	}
	return r
}

// maxTextShadows bounds the shadows one text-shadow draws. Each is drawn under
// every run of the element and every decoration across it, so the count
// multiplies the whole of the element's text. Past it, the first ones are drawn
// — they are on top — and the rest are reported.
//
// A variable so that a test can lower it.
var maxTextShadows = 32

// textShadow is one shadow of a text-shadow list, read.
type textShadow struct {
	dx, dy, stdDev style.Unit
	colour         style.RGBA
}

// shadowsOf reads a box's text-shadow into its shadows, first on top.
//
// A computed value's lengths are already absolute but for the units the cascade
// cannot resolve, the viewport's, which the page supplies: paintLengths. A
// value that does not read is drawn as no shadow and reported; the cascade has
// already refused anything that is not CSS, so this is a unit this engine does
// not resolve.
func (p *painter) shadowsOf(b *Box) []textShadow {
	if b == nil {
		return nil
	}
	raw := ascii.TrimCSSSpace(b.Style.Get("text-shadow"))
	if raw == "" || ascii.EqualFold(raw, "none") {
		return nil
	}
	colour, _ := p.color(b, "color")
	key := shadowKey{raw: raw, fontSize: b.FontSize, colour: colour}
	if got, ok := p.shadows[key]; ok {
		return got
	}
	vals, _ := css.ParseComponentValues(raw)
	ctx := p.lengths
	ctx.FontSize = b.FontSize
	var out []textShadow
	items := splitTopLevelCommas(vals)
	for i, item := range items {
		if i >= maxTextShadows {
			p.reportOnce(b, "text-shadows", Finding{
				Rule:   RuleLimit,
				Source: AtHTML(offsetOf(b)),
				Message: fmt.Sprintf("a text-shadow of %d shadows was drawn with its first %d",
					len(items), maxTextShadows),
				Path:     PathOf(b.Element),
				Property: "text-shadow",
			})
			break
		}
		s, ok := readShadow(item, ctx, colour)
		if !ok {
			p.reportOnce(b, "text-shadow-unread", Finding{
				Rule:     RuleUnsupportedValue,
				Source:   AtHTML(offsetOf(b)),
				Message:  "the text-shadow " + quoteValue(raw) + " has a length this engine does not resolve; that shadow was not drawn",
				Path:     PathOf(b.Element),
				Property: "text-shadow",
			})
			continue
		}
		out = append(out, s)
	}
	if p.shadows == nil {
		p.shadows = map[shadowKey][]textShadow{}
	}
	p.shadows[key] = out
	return out
}

// shadowKey is what a box's shadows are memoized by: its value, and what that
// value resolves against.
type shadowKey struct {
	raw      string
	fontSize style.Unit
	colour   style.RGBA
}

// readShadow reads one shadow: a colour, which is the text's when there is none,
// and two offsets and a blur radius.
func readShadow(item []css.ComponentValue, ctx style.LengthContext, text style.RGBA) (textShadow, bool) {
	s := textShadow{colour: text}
	var lengths []style.Unit
	for _, part := range splitValueParts(item) {
		if w, ok := identOf(part); ok && w == "currentcolor" {
			s.colour = text
			continue
		}
		if c, ok := style.ParseColor(part); ok {
			s.colour = c
			continue
		}
		l, _, ok := style.ParseLength(part, ctx)
		if !ok || l.Kind != style.LengthAbsolute {
			return textShadow{}, false
		}
		lengths = append(lengths, l.Value)
	}
	if len(lengths) < 2 || len(lengths) > 3 {
		return textShadow{}, false
	}
	s.dx, s.dy = lengths[0], lengths[1]
	if len(lengths) == 3 && lengths[2] > 0 {
		s.stdDev = lengths[2].Div(2)
	}
	return s, true
}

// paintShadows paints a run's shadows, the last first so that the first is on
// top. Each shadow is §5.1's stack again, moved and recoloured: the underlines
// and overlines, then the text — its glyphs, or for a control character the box
// drawn in its place — then its emphasis marks, then the line-throughs. A mark
// is glyphs, so its shadow is a DrawTextShadow like the text's.
func (p *painter) paintShadows(shadows []textShadow, under []Rect, text *DrawText, glyphs []Rect,
	marks []DrawText, over []Rect) {
	for i := len(shadows) - 1; i >= 0; i-- {
		s := shadows[i]
		if s.colour.A == 0 {
			continue
		}
		moved := func(rs []Rect) []Op {
			out := make([]Op, 0, len(rs))
			for _, r := range rs {
				out = append(out, FillRect{
					Rect:  Rect{X: r.X.Add(s.dx), Y: r.Y.Add(s.dy), W: r.W, H: r.H},
					Color: s.colour, Overhang: true,
				})
			}
			return out
		}
		p.shadowFills(moved(under), s)
		p.shadowFills(moved(glyphs), s)
		if text != nil {
			run := *text
			run.At = Point{X: run.At.X.Add(s.dx), Y: run.At.Y.Add(s.dy)}
			run.Color = s.colour
			p.emit(DrawTextShadow{Run: run, StdDev: s.stdDev})
		}
		for _, m := range marks {
			m.At = Point{X: m.At.X.Add(s.dx), Y: m.At.Y.Add(s.dy)}
			m.Color = s.colour
			p.emit(DrawTextShadow{Run: m, StdDev: s.stdDev})
		}
		p.shadowFills(moved(over), s)
	}
}

// shadowFills emits the shadows of decoration lines: the lines themselves in
// the shadow's colour, or blurred in a FilterGroup.
func (p *painter) shadowFills(fills []Op, s textShadow) {
	if len(fills) == 0 {
		return
	}
	if s.stdDev <= 0 {
		p.emit(fills...)
		return
	}
	for _, f := range fills {
		p.emit(newFilterGroup([]FilterFunction{{Kind: FilterBlur, StdDev: s.stdDev}}, []Op{f}))
	}
}
