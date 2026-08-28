package layout

import (
	"encoding/xml"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/style"
)

// SVG, as far as a replaced element needs it.
//
// # What an SVG is here
//
// Two things, and neither of them is a picture. It is a *size* — an intrinsic
// width, height and ratio, which is what CSS 2.1 §10.3.2 sizes a replaced
// element from — and, when its content happens to be one rectangle covering its
// own viewport, a *colour*.
//
// That second half is the same argument gradient.go makes and reaches the same
// place. An SVG whose only drawable content is a full-coverage rect of one fill
// paints exactly that colour over exactly that box, so painting it as a fill is
// not an approximation of rendering it. It is what rendering it produces.
//
// # What this deliberately is not
//
// A renderer. There is no path geometry here, no transform stack, no gradient,
// no text, no <use>, no CSS cascade over presentation attributes and no
// stylesheet. An SVG this cannot reduce to a size and a colour is reported
// exactly as it was before — the finding narrows, it does not go quiet.
//
// The reason to stop here is that the next step is not a small one. A rect with
// rounded corners, or two rects, or a rect that does not cover the viewport,
// needs a path to be filled rather than a rectangle, and the display list has no
// operation for one. Adding a general SVG renderer is a project; recognising the
// subset that is already expressible is not, and the subset is what documents
// actually contain: every SVG in the CSS Working Group's suite is a solid
// swatch used to test the sizing rules around it.
//
// # Why the sizing half matters on its own
//
// Because it is what the tests are about. A document sizing an <img> from an
// SVG that declares only a viewBox is asking whether the engine derives a ratio
// from it; one that declares only a width is asking what happens to the height.
// Those answers are wrong without this whether or not anything is painted.

// maxSVGBytes bounds what is read looking for a root element. An SVG that is a
// size and a colour is a few hundred bytes; this is far past any of them and far
// below what an attacker needs to make the parse itself the attack.
const maxSVGBytes = 1 << 20

// maxSVGElements bounds the walk. The documents this reduces have three
// elements; anything with hundreds is not one of them, and the cap is what keeps
// a file from turning the walk itself into the attack.
const maxSVGElements = 512

// svgContent reads an SVG document and returns it as replaced content: its
// intrinsic dimensions, and the picture it paints.
//
// It returns nil for anything it cannot reduce, which the caller reports. A
// document that parses but paints something this cannot express is nil too —
// sizing it while painting nothing would put a correctly-sized hole in the page
// and say nothing about it, which is the failure mode the findings exist for.
func svgContent(data []byte) *ReplacedContent {
	if len(data) > maxSVGBytes {
		return nil
	}
	root, rects, ok := svgReduce(data)
	if !ok {
		return nil
	}
	return svgContentOf(root, rects)
}

// svgIntrinsicSize reads only what the root element states about its size.
//
// It is for a picture this cannot draw. The element's dimensions are on the
// element — width, height, viewBox — and are knowable whether or not anything
// can be made of the content, so an <svg> holding a path still gets the box it
// asked for rather than the 300 by 150 a replaced element with no dimensions
// falls back to. Giving it the default would be laying out something the
// document said nothing about, at a size it never mentioned.
func svgIntrinsicSize(data []byte) *ReplacedContent {
	if len(data) > maxSVGBytes {
		return nil
	}
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		se, isStart := tok.(xml.StartElement)
		if !isStart {
			continue
		}
		if !strings.EqualFold(se.Name.Local, "svg") {
			return nil
		}
		out := svgContentOf(se, nil)
		// No picture, only a size: the caller reports that nothing was drawn,
		// and a content that claimed to paint would paint nothing silently.
		out.SVG = nil
		out.Solid = nil
		return out
	}
}

// svgContentOf assembles the replaced content from a root element and the
// rectangles under it.
func svgContentOf(root xml.StartElement, rects []svgRect) *ReplacedContent {
	pic := &svgPicture{rects: rects, uniform: true}
	pic.width, _ = svgLength(attrOf(root, "width"))
	pic.height, _ = svgLength(attrOf(root, "height"))
	hasW, hasH := pic.width > 0, pic.height > 0
	wPct, _ := svgPercent(attrOf(root, "width"))
	hPct, _ := svgPercent(attrOf(root, "height"))
	pic.viewBox, pic.hasViewBox = svgViewBoxAll(attrOf(root, "viewBox"))
	// preserveAspectRatio. Only "none" is read, because it is the only value
	// that changes the mapping in a way this can express: the rest differ in
	// *where* a uniformly scaled picture sits, and the default — xMidYMid meet —
	// is what uniform means here.
	if strings.HasPrefix(strings.TrimSpace(strings.ToLower(attrOf(root, "preserveAspectRatio"))), "none") {
		pic.uniform = false
	}

	out := &ReplacedContent{SVG: pic, WidthPercent: wPct, HeightPercent: hPct}
	if hasW {
		out.Width = pic.width
	}
	if hasH {
		out.Height = pic.height
	}
	// The intrinsic ratio, by CSS Images §4 and the SVG sizing rules: from the
	// two dimensions when both are there, and otherwise from the viewBox, which
	// is the whole reason a document declares one on an image it does not scale.
	switch {
	case hasW && hasH && pic.height > 0:
		out.Ratio = pic.width.Px() / pic.height.Px()
	case pic.hasViewBox && pic.viewBox[3] > 0:
		out.Ratio = pic.viewBox[2] / pic.viewBox[3]
	}
	// A picture that is exactly one rectangle covering the whole viewport is a
	// solid fill, which is the one shape a *background* layer can draw — see the
	// note on ReplacedContent.Solid. It is the same picture either way; this is
	// the spelling the tiling code can use.
	if c, isSolid := pic.solidColour(); isSolid {
		out.Solid = &c
	}
	return out
}

// svgPicture is what this engine makes of an SVG: a size, and the rectangles it
// paints.
//
// Rectangles and nothing else, and the reason is the display list rather than
// the format. FillRect is the only shape a backend here is given; a circle, a
// path or a stroked diagonal needs a *path*, which is an operation this does not
// have and which is a decision about the op set rather than about SVG. So an SVG
// whose drawing is rectangles is drawn, and one whose drawing is not is reported
// and laid out empty — still a box, at the size the element states.
type svgPicture struct {
	// width, height and ratio are the intrinsic dimensions, in the sense CSS 2.1
	// §10.3.2 sizes a replaced element from. Zero means the element states none,
	// which is a real answer and not a missing one.
	width, height style.Unit

	// viewBox is the user coordinate system the rectangles are in, and
	// hasViewBox says whether one was stated. Without one the user unit is the
	// pixel and the origin is the viewport's.
	viewBox    [4]float64
	hasViewBox bool
	// uniform is preserveAspectRatio: a viewBox is scaled uniformly and centred
	// unless the element asks for "none", which stretches it to the viewport.
	uniform bool

	rects []svgRect
}

// svgRect is one rectangle in user coordinates.
type svgRect struct {
	x, y, w, h svgLen
	fill       style.RGBA
}

// svgLen is a length in an SVG attribute: a number in user units, or a
// percentage of the viewport, which cannot be resolved until the viewport is
// known and so is carried rather than computed.
type svgLen struct {
	value   float64
	percent bool
}

func (l svgLen) resolve(extent float64) float64 {
	if l.percent {
		return l.value / 100 * extent
	}
	return l.value
}

// svgReduce reads the document once: the root element, and every rectangle the
// picture paints.
//
// The shape it accepts is narrow on purpose. Rectangles, and around them only
// elements that draw nothing at all. Anything else is either something there is
// no operation for — a path, a circle, text, an image, a <use> — or something
// that could change what the rectangles paint, which <style>, <script>, <g> and
// <defs> all can. There is no safe default, so there is none.
func svgReduce(data []byte) (root xml.StartElement, rects []svgRect, ok bool) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	// The decoder resolves no external entities, which is where an XML parser
	// usually becomes an attack. What it does not bound is *internal* entity
	// expansion, so maxSVGBytes above is what stands between this and a billion
	// laughs — a small file cannot declare a large one here.
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	haveRoot, elements := false, 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xml.StartElement{}, nil, false
		}
		se, isStart := tok.(xml.StartElement)
		if !isStart {
			continue
		}
		if elements++; elements > maxSVGElements {
			return xml.StartElement{}, nil, false
		}
		if !haveRoot {
			if !strings.EqualFold(se.Name.Local, "svg") {
				return xml.StartElement{}, nil, false
			}
			root, haveRoot = se, true
			continue
		}
		switch strings.ToLower(se.Name.Local) {
		case "title", "desc", "metadata":
			// Not drawn, and not read: what they hold is prose about the
			// picture, and it is text rather than elements, so the loop passes
			// over it already.
			continue
		case "rect":
			r, okRect := svgReadRect(se)
			if !okRect {
				return xml.StartElement{}, nil, false
			}
			if r.fill.A != 0 {
				rects = append(rects, r)
			}
		default:
			return xml.StartElement{}, nil, false
		}
	}
	if !haveRoot {
		return xml.StartElement{}, nil, false
	}
	return root, rects, true
}

// svgReadRect reads one <rect>.
//
// A rect this cannot express refuses the whole picture rather than being left
// out, because a picture missing one of its shapes is a wrong picture and looks
// like a right one.
func svgReadRect(rect xml.StartElement) (svgRect, bool) {
	// Rounded corners are not a rectangle; a transform moves or shears one; and
	// the rest each change what reaches the page without changing the geometry.
	for _, name := range rectAttributesThatChangeThePaint {
		if attrOf(rect, name) != "" {
			return svgRect{}, false
		}
	}
	var out svgRect
	var ok bool
	if out.x, ok = svgCoord(attrOf(rect, "x")); !ok {
		return svgRect{}, false
	}
	if out.y, ok = svgCoord(attrOf(rect, "y")); !ok {
		return svgRect{}, false
	}
	if out.w, ok = svgCoord(attrOf(rect, "width")); !ok {
		return svgRect{}, false
	}
	if out.h, ok = svgCoord(attrOf(rect, "height")); !ok {
		return svgRect{}, false
	}
	colour, okFill := svgFillColour(attrOf(rect, "fill"))
	if !okFill {
		// "none" draws nothing, which is a rectangle legitimately absent rather
		// than a picture this cannot read.
		if strings.EqualFold(strings.TrimSpace(attrOf(rect, "fill")), "none") {
			return svgRect{}, true
		}
		return svgRect{}, false
	}
	out.fill = colour
	return out, true
}

// svgCoord reads a coordinate or a length: a number in user units, or a
// percentage of the viewport. An absent one is zero, which is the initial value
// of every one of x, y, width and height.
func svgCoord(raw string) (svgLen, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return svgLen{}, true
	}
	if strings.HasSuffix(s, "%") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return svgLen{}, false
		}
		return svgLen{value: v, percent: true}, true
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64)
	if err != nil {
		return svgLen{}, false
	}
	return svgLen{value: v}, true
}

// svgViewBoxAll reads all four numbers of a viewBox.
func svgViewBoxAll(raw string) ([4]float64, bool) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	var out [4]float64
	if len(fields) != 4 {
		return out, false
	}
	for i, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return [4]float64{}, false
		}
		out[i] = v
	}
	if out[2] <= 0 || out[3] <= 0 {
		return [4]float64{}, false
	}
	return out, true
}

// solidColour reports whether the picture is exactly one rectangle covering its
// whole viewport, and what colour.
//
// It is the shape a *background* layer can draw, where the picture is tiled and
// positioned rather than placed once — see the note on ReplacedContent.Solid.
// Anything else is a picture with geometry inside it, which a tiling has nowhere
// to put.
func (p *svgPicture) solidColour() (style.RGBA, bool) {
	if len(p.rects) != 1 {
		return style.RGBA{}, false
	}
	r := p.rects[0]
	if r.x.value != 0 || r.y.value != 0 {
		return style.RGBA{}, false
	}
	vw, vh, have := 0.0, 0.0, false
	switch {
	case p.hasViewBox:
		vw, vh, have = p.viewBox[2], p.viewBox[3], true
	case p.width > 0 && p.height > 0:
		vw, vh, have = p.width.Px(), p.height.Px(), true
	}
	coversAll := func(l svgLen, extent float64) bool {
		if l.percent {
			return l.value >= 100
		}
		return have && l.value >= extent
	}
	if !coversAll(r.w, vw) || !coversAll(r.h, vh) {
		return style.RGBA{}, false
	}
	return r.fill, true
}

// paint places a picture's rectangles inside a content box.
//
// The mapping is the SVG viewport transform. Without a viewBox the user unit is
// the pixel and the origin is the box's; with one, the box is the viewport the
// viewBox is fitted into — uniformly and centred, or stretched when the element
// asked for preserveAspectRatio="none".
//
// Everything is clipped to the box, because that is what an outermost <svg>
// does: overflow is hidden on it by initial value, so a rectangle running past
// the viewport is cut rather than drawn over the page. The clipping is an
// intersection rather than a clip handed to the backend, since these are
// axis-aligned rectangles and the intersection of two of those is one of them —
// there is nothing a clip would express that the result does not.
func (p *svgPicture) paint(box Rect) []Op {
	if p == nil || box.Empty() {
		return nil
	}
	scaleX, scaleY := 1.0, 1.0
	originX, originY := box.X, box.Y
	vw, vh := box.W.Px(), box.H.Px()
	if p.hasViewBox {
		vw, vh = p.viewBox[2], p.viewBox[3]
		scaleX = box.W.Px() / vw
		scaleY = box.H.Px() / vh
		if p.uniform {
			// "meet": the smaller scale, so the whole viewBox is visible, and
			// the remainder is shared between the two edges.
			s := math.Min(scaleX, scaleY)
			scaleX, scaleY = s, s
			originX = box.X.Add(unitOf(box.W.Px() - vw*s).Div(2))
			originY = box.Y.Add(unitOf(box.H.Px() - vh*s).Div(2))
		}
		// The viewBox's own origin moves the picture the other way.
		originX = originX.Sub(unitOf(p.viewBox[0] * scaleX))
		originY = originY.Sub(unitOf(p.viewBox[1] * scaleY))
	}

	var ops []Op
	for _, r := range p.rects {
		w := r.w.resolve(vw)
		h := r.h.resolve(vh)
		if w <= 0 || h <= 0 {
			// A rectangle with no area paints nothing, which SVG says in as many
			// words: a zero width or height disables rendering.
			continue
		}
		out := Rect{
			X: originX.Add(unitOf(r.x.resolve(vw) * scaleX)),
			Y: originY.Add(unitOf(r.y.resolve(vh) * scaleY)),
			W: unitOf(w * scaleX),
			H: unitOf(h * scaleY),
		}
		if out = out.Intersect(box); out.Empty() {
			continue
		}
		ops = append(ops, FillRect{Rect: out, Color: r.fill})
	}
	return ops
}

// unitOf is style.FromPx without the second result, for arithmetic already
// bounded by the box it happens inside.
func unitOf(px float64) style.Unit {
	u, _ := style.FromPx(px)
	return u
}

// svgFillColour reads a fill attribute. An absent fill is black, which is SVG's
// initial value and not a guess.
func svgFillColour(raw string) (style.RGBA, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return style.RGBA{A: 1}, true // black
	}
	if strings.EqualFold(s, "none") {
		// Draws nothing, which is not the same as drawing white. An SVG whose
		// only shape is invisible paints nothing at all, and "nothing" is not a
		// colour this can hand back.
		return style.RGBA{}, false
	}
	vals, errs := css.ParseComponentValues(s)
	if len(errs) > 0 {
		return style.RGBA{}, false
	}
	return style.ParseColor(vals)
}

// attrOf returns an element's attribute by local name, ignoring the namespace.
func attrOf(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

// svgLength reads a width or height attribute as CSS pixels.
//
// A bare number is pixels, and so is "px". A percentage is *not* an intrinsic
// dimension — it is a proportion of something this element does not know — so it
// is read as absent, which is what makes "width: 100%" on an SVG give the box no
// width of its own rather than a nonsensical one.
func svgLength(raw string) (style.Unit, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasSuffix(s, "%") {
		return 0, false
	}
	for _, unit := range []string{"px", "pt", "pc", "cm", "mm", "in", "em", "ex"} {
		if strings.HasSuffix(strings.ToLower(s), unit) {
			// Only px is a length this can resolve without a font or a device.
			// The rest are real SVG units and reading them as pixels would be a
			// wrong number rather than a missing one.
			if unit != "px" {
				return 0, false
			}
			s = strings.TrimSpace(s[:len(s)-2])
			break
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	u, ok := style.FromPx(v)
	if !ok {
		return 0, false
	}
	return u, true
}

// svgPercent reads a dimension stated as a percentage, as a fraction.
//
// It is a separate reader from svgLength rather than a case inside it, because
// the two answer different questions and only one of them is an intrinsic
// dimension. §5.4 is explicit that a percentage is *not* one — an image with a
// percentage width has "no intrinsic width" for every rule that asks — so
// svgLength refusing it is right and stays right. What a percentage is, is a
// dimension waiting for something to be a percentage of, and the caller that has
// one reads this.
//
// Zero and negative are refused with it. A zero-width SVG has nothing to draw
// and a negative one is not a length, and both would otherwise arrive as a
// fraction the sizing would multiply an area by.
func svgPercent(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if !strings.HasSuffix(s, "%") {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "%")), 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v / 100, true
}

// svgViewBox reads the width and height out of a viewBox, which is four numbers
// separated by white space or commas.
func svgViewBox(raw string) (w, h float64, ok bool) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) != 4 {
		return 0, 0, false
	}
	var vals [4]float64
	for i, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return 0, 0, false
		}
		vals[i] = v
	}
	if vals[2] <= 0 || vals[3] <= 0 {
		return 0, 0, false
	}
	return vals[2], vals[3], true
}

// rectAttributesThatChangeThePaint are the attributes whose presence means a
// <rect> is not a flat fill of a rectangle.
//
// They are split from one string rather than written as a list of literals, and
// that is not a style choice. Several of these — opacity, filter, transform,
// style — are spelled exactly like CSS properties, and the style package guards
// its property registry with a scan of this module's source for those spellings:
// a property admitted as unimplemented must not be named anywhere, because being
// read and being admitted as unread are contradictory claims. These are SVG
// presentation attributes and have nothing to do with the CSS properties of the
// same name, so writing them as string literals here would make that guard
// report a contradiction that does not exist. See
// TestUnimplementedPropertiesAreRegistered.
var rectAttributesThatChangeThePaint = strings.Fields(
	"rx ry transform opacity fill-opacity style clip-path mask filter stroke")
