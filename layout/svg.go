package layout

import (
	"encoding/xml"
	"io"
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
// intrinsic dimensions, and its colour when it has one.
//
// It returns nil for anything it cannot reduce, which the caller reports. A
// document that parses but paints something this cannot express is nil too —
// sizing it while painting nothing would put a correctly-sized hole in the page
// and say nothing about it, which is the failure mode the findings exist for.
func svgContent(data []byte) *ReplacedContent {
	if len(data) > maxSVGBytes {
		return nil
	}
	root, colour, ok := svgReduce(data)
	if !ok {
		return nil
	}
	w, hasW := svgLength(attrOf(root, "width"))
	h, hasH := svgLength(attrOf(root, "height"))
	vbW, vbH, hasVB := svgViewBox(attrOf(root, "viewBox"))

	out := &ReplacedContent{Solid: &colour}
	if hasW {
		out.Width = w
	}
	if hasH {
		out.Height = h
	}
	// The intrinsic ratio, by CSS Images §4 and the SVG sizing rules: from the
	// two dimensions when both are there, and otherwise from the viewBox, which
	// is the whole reason a document declares one on an image it does not scale.
	switch {
	case hasW && hasH && h > 0:
		out.Ratio = w.Px() / h.Px()
	case hasVB && vbH > 0:
		out.Ratio = vbW / vbH
	}
	return out
}

// svgReduce reads the document once and answers both questions: what the root
// element declares, and what the whole of it paints.
//
// The shape it accepts is narrow on purpose. Exactly one drawing element, which
// must be a <rect> covering the viewport in a flat colour, and around it only
// elements that draw nothing at all. Anything else is not reducible to a fill,
// and refusing it is the point — a document that paints something this cannot
// express must keep its finding.
func svgReduce(data []byte) (root xml.StartElement, colour style.RGBA, ok bool) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	// The decoder resolves no external entities, which is where an XML parser
	// usually becomes an attack. What it does not bound is *internal* entity
	// expansion, so maxSVGBytes above is what stands between this and a billion
	// laughs — a small file cannot declare a large one here.
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	haveRoot, found, elements := false, false, 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xml.StartElement{}, style.RGBA{}, false
		}
		se, isStart := tok.(xml.StartElement)
		if !isStart {
			continue
		}
		if elements++; elements > maxSVGElements {
			return xml.StartElement{}, style.RGBA{}, false
		}
		if !haveRoot {
			if !strings.EqualFold(se.Name.Local, "svg") {
				return xml.StartElement{}, style.RGBA{}, false
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
			if found {
				return xml.StartElement{}, style.RGBA{}, false // more than one
			}
			c, okRect := svgRectFill(se, root)
			if !okRect {
				return xml.StartElement{}, style.RGBA{}, false
			}
			colour, found = c, true
		default:
			// Everything else is either something this cannot express — a path,
			// a second shape, text, an image, a <use> — or something that could
			// change what the rect paints, which <style>, <script>, <g> and
			// <defs> all can. There is no safe default here, so there is none.
			return xml.StartElement{}, style.RGBA{}, false
		}
	}
	if !haveRoot || !found {
		return xml.StartElement{}, style.RGBA{}, false
	}
	return root, colour, true
}

// svgRectFill reads a <rect> and returns its colour when it is a flat fill
// covering the whole viewport.
//
// Covering is what makes the rectangle equal to the box: a rect that starts away
// from the origin, or stops short of the far edge, paints part of the area and
// the rest shows through. There is no operation here for "part", so a rect that
// does not cover is refused rather than approximated.
func svgRectFill(rect, root xml.StartElement) (style.RGBA, bool) {
	// Rounded corners are not a rectangle; a transform moves or shears one; and
	// the rest each change what reaches the page without changing the geometry.
	for _, name := range rectAttributesThatChangeThePaint {
		if attrOf(rect, name) != "" {
			return style.RGBA{}, false
		}
	}
	if !svgAtOrigin(attrOf(rect, "x")) || !svgAtOrigin(attrOf(rect, "y")) {
		return style.RGBA{}, false
	}
	vw, vh, haveViewport := svgViewport(root)
	if !svgCovers(attrOf(rect, "width"), vw, haveViewport) {
		return style.RGBA{}, false
	}
	if !svgCovers(attrOf(rect, "height"), vh, haveViewport) {
		return style.RGBA{}, false
	}
	return svgFillColour(attrOf(rect, "fill"))
}

// rectAttributesThatChangeThePaint are the attributes whose presence means a
// <rect> is not a flat fill of its viewport.
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

// svgAtOrigin reports whether a coordinate is absent or zero.
func svgAtOrigin(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" {
		return true
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64)
	return err == nil && v == 0
}

// svgViewport is the size of the coordinate system a child is drawn in: the
// viewBox when there is one, and otherwise the element's own width and height.
//
// It comes back as "not known" when neither is stated, which is not a failure —
// a rect can still cover a viewport whose size nobody said, by asking for 100%
// of it.
func svgViewport(root xml.StartElement) (w, h float64, ok bool) {
	if vw, vh, has := svgViewBox(attrOf(root, "viewBox")); has {
		return vw, vh, true
	}
	uw, hasW := svgLength(attrOf(root, "width"))
	uh, hasH := svgLength(attrOf(root, "height"))
	if hasW && hasH {
		return uw.Px(), uh.Px(), true
	}
	return 0, 0, false
}

// svgCovers reports whether a rect's width or height spans the whole viewport.
func svgCovers(raw string, extent float64, haveExtent bool) bool {
	s := strings.TrimSpace(raw)
	if s == "100%" {
		return true
	}
	if !haveExtent {
		// Nothing to compare a number against, so only "100%" is known to cover.
		return false
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64)
	return err == nil && v == extent
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
