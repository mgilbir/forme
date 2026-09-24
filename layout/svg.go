package layout

import (
	"encoding/xml"
	"io"
	"math"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
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
// no text, no <use>, no CSS cascade and no stylesheet. An SVG this cannot
// reduce to a size and a colour is reported exactly as it was before — the
// finding narrows, it does not go quiet.
//
// What it does read is SVG 2's presentation attributes, as far as a filled
// rectangle needs them, with the inheritance §6.7 gives them: a fill, a stroke
// and a visibility written on the root are the rect's unless the rect says
// otherwise, and a display of "none" takes an element out of the picture. Every
// other attribute is either one that cannot change what a filled, unstroked
// rectangle paints — and there is a list of those, see svgAttributes — or one
// that can, and refuses the picture. An attribute on neither list refuses it
// too: the promise is "exact or refused", and an attribute nobody has looked at
// is not exact. They were not read at all, so "<svg fill=red><rect/></svg>"
// was painted black, and a rect with display="none" or visibility="hidden" was
// painted (audit C81).
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
// svgAs says how an SVG reaches the page, which decides what its own percentage
// width and height are a percentage *of*.
//
// The two are different questions and the suite asks both.
//
// An <img> holds a *picture*. CSS Images §5.4 makes a percentage "no intrinsic
// dimension" and sizes the element from the default object size, which for a
// replaced element is CSS 2.1 §10.3.2's 300 by 150.
// CSS2/visudet/replaced-elements-all-auto writes seven such images — one of them
// an SVG stating no width, no height and no viewBox — and asks for exactly that,
// and nine other documents of that family agree.
//
// An <object> holds a *document*, and the document's own viewport is the box the
// element gets. A percentage there resolves against that box, so an SVG that
// states nothing — whose width and height are 100% by SVG's own initial values —
// is as wide as its containing block. CSS2/normal-flow/replaced-intrinsic-001
// says so in a comment of its own: "intrinsic size is 100%x100%, which is
// equivalent to width:100%".
//
// # The inline <svg> element
//
// It reads as the second case and is treated as the first, which is a decision
// and not an oversight: the element *is* its own viewport, so the argument above
// applies to it word for word. Making it so costs seven of the suite's
// documents — 5941 clean passes against 5932, measured — because they write an
// <svg> with a height and no width and expect the 300 that CSS 2.1 gives it.
// Whether those seven are right about SVG or only about what browsers do is a
// question this comment cannot settle, and the suite is the arbiter here.
type svgAs uint8

const (
	// svgAsImage is an <img>, a background layer, a list marker, and — see
	// above — an inline <svg> element.
	svgAsImage svgAs = iota
	// svgAsDocument is an <object>: a document with a viewport of its own.
	svgAsDocument
)

func svgContent(data []byte, as svgAs) *ReplacedContent {
	if len(data) > maxSVGBytes {
		return nil
	}
	root, rects, ok := svgReduce(data)
	if !ok {
		return nil
	}
	return svgContentOf(root, rects, as)
}

// svgIntrinsicSize reads only what the root element states about its size.
//
// It is for a picture this cannot draw. The element's dimensions are on the
// element — width, height, viewBox — and are knowable whether or not anything
// can be made of the content, so an <svg> holding a path still gets the box it
// asked for rather than the 300 by 150 a replaced element with no dimensions
// falls back to. Giving it the default would be laying out something the
// document said nothing about, at a size it never mentioned.
func svgIntrinsicSize(data []byte, as svgAs) *ReplacedContent {
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
		if !ascii.EqualFold(se.Name.Local, "svg") {
			return nil
		}
		out := svgContentOf(se, nil, as)
		// No picture, only a size: the caller reports that nothing was drawn,
		// and a content that claimed to paint would paint nothing silently.
		out.SVG = nil
		out.Solid = nil
		return out
	}
}

// svgContentOf assembles the replaced content from a root element and the
// rectangles under it.
func svgContentOf(root xml.StartElement, rects []svgRect, as svgAs) *ReplacedContent {
	pic := &svgPicture{rects: rects, uniform: true}
	pic.width, _ = svgLength(attrOf(root, "width"))
	pic.height, _ = svgLength(attrOf(root, "height"))
	hasW, hasH := pic.width > 0, pic.height > 0
	wPct, _ := svgPercent(attrOf(root, "width"))
	hPct, _ := svgPercent(attrOf(root, "height"))
	// SVG's own initial values for the root element's width and height, which
	// are 100% and not "absent". They are read only for a document, because for
	// a picture a percentage is no dimension at all and the element falls back
	// to the default object size. See svgAs.
	if as == svgAsDocument {
		if attrOf(root, "width") == "" {
			wPct = 1
		}
		if attrOf(root, "height") == "" {
			hPct = 1
		}
	}
	pic.viewBox, pic.hasViewBox = svgViewBoxAll(attrOf(root, "viewBox"))
	// preserveAspectRatio. Only "none" is read, because it is the only value
	// that changes the mapping in a way this can express: the rest differ in
	// *where* a uniformly scaled picture sits, and the default — xMidYMid meet —
	// is what uniform means here.
	if strings.HasPrefix(strings.TrimSpace(ascii.Lower(attrOf(root, "preserveAspectRatio"))), "none") {
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
	// No entity a document declares is expanded, internal or external:
	// encoding/xml reads a DTD as an opaque directive and knows only the
	// entities in dec.Entity, which is HTML's fixed table. So a billion laughs
	// has nothing to multiply here: a reference to a declared entity is left as
	// the characters it is written in, and in an attribute this reads it is a
	// value it cannot read, so the picture is refused. maxSVGBytes and
	// maxSVGElements bound what the file itself costs to read.
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	haveRoot, elements := false, 0
	// What the root passes down to its rects, and whether the root is rendered
	// at all. A root with display="none" renders nothing, and is still the
	// size it says: the rects are read — one that could not be would still
	// refuse — and dropped.
	var inherited svgInherited
	rootShown := true
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
			if !ascii.EqualFold(se.Name.Local, "svg") {
				return xml.StartElement{}, nil, false
			}
			root, haveRoot = se, true
			var okRoot bool
			if inherited, rootShown, okRoot = svgPresentation(se, svgInherited{}, svgRootAttribute); !okRoot {
				return xml.StartElement{}, nil, false
			}
			continue
		}
		switch ascii.Lower(se.Name.Local) {
		case "title", "desc", "metadata":
			// Not drawn, and not read: what they hold is prose about the
			// picture, and it is text rather than elements, so the loop passes
			// over it already.
			continue
		case "rect":
			r, shown, okRect := svgReadRect(se, inherited)
			if !okRect {
				return xml.StartElement{}, nil, false
			}
			if shown && rootShown && r.fill.A != 0 {
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

// svgReadRect reads one <rect>, with what the root passes down to it, and says
// whether it is rendered.
//
// A rect this cannot express refuses the whole picture rather than being left
// out, because a picture missing one of its shapes is a wrong picture and looks
// like a right one. A rect that is not rendered — display="none", or hidden
// by its own visibility or the root's — is left out, which is exactly what a
// renderer does with it.
func svgReadRect(rect xml.StartElement, from svgInherited) (svgRect, bool, bool) {
	p, shown, ok := svgPresentation(rect, from, svgRectAttribute)
	if !ok {
		return svgRect{}, false, false
	}
	var out svgRect
	if out.x, ok = svgCoord(attrOf(rect, "x")); !ok {
		return svgRect{}, false, false
	}
	if out.y, ok = svgCoord(attrOf(rect, "y")); !ok {
		return svgRect{}, false, false
	}
	if out.w, ok = svgCoord(attrOf(rect, "width")); !ok {
		return svgRect{}, false, false
	}
	if out.h, ok = svgCoord(attrOf(rect, "height")); !ok {
		return svgRect{}, false, false
	}
	// A stroke is drawn along the edge and there is no operation for one.
	if p.stroke {
		return svgRect{}, false, false
	}
	if p.hidden || p.fillNone {
		// Not painted, which is a rectangle legitimately absent rather than a
		// picture this cannot read.
		return svgRect{}, false, true
	}
	out.fill = p.fill
	return out, shown, true
}

// svgInherited is the presentation an element hands its children: SVG 2's
// inherited properties, as far as this reads them.
type svgInherited struct {
	// fill is the paint, and fillNone says it is "none". Unset, it is SVG's
	// initial value, black.
	fill     style.RGBA
	fillNone bool
	fillSet  bool
	// stroke says the stroke is a paint rather than "none", its initial value.
	stroke bool
	// hidden is visibility: hidden or collapse.
	hidden bool
}

// svgPresentation reads one element's attributes over what its parent handed
// it: the inherited properties it passes on, whether it is rendered, and
// whether every attribute on it is one this can honour or ignore exactly.
//
// kind classifies the attributes the element has of its own — geometry for a
// rect, sizing for the root. Everything else is svgAttributes'.
func svgPresentation(e xml.StartElement, from svgInherited,
	kind func(name string) (svgAttrKind, bool)) (svgInherited, bool, bool) {

	p, shown := from, true
	for _, a := range e.Attr {
		k := svgAttrKindOf(a.Name, kind)
		v := strings.TrimSpace(a.Value)
		inherit := ascii.EqualFold(v, "inherit")
		switch k {
		case svgInert, svgOwn:
		case svgFill:
			if inherit {
				continue
			}
			if ascii.EqualFold(v, "none") {
				p.fill, p.fillNone, p.fillSet = style.RGBA{}, true, true
				continue
			}
			c, ok := svgFillColour(v)
			if !ok {
				return p, false, false
			}
			p.fill, p.fillNone, p.fillSet = c, false, true
		case svgStroke:
			if inherit {
				continue
			}
			p.stroke = v != "" && !ascii.EqualFold(v, "none")
		case svgVisibility:
			switch ascii.Lower(v) {
			case "inherit":
			case "visible":
				p.hidden = false
			case "hidden", "collapse":
				p.hidden = true
			default:
				return p, false, false
			}
		case svgDisplay:
			// Not inherited: "none" takes this element out, and any other
			// value leaves it in. A display that is not a keyword is not a
			// value at all and the attribute is ignored, as SVG 2 ignores an
			// invalid presentation attribute.
			if ascii.EqualFold(v, "none") {
				shown = false
			}
		case svgAlpha:
			// opacity and fill-opacity. At one they change nothing; below it
			// the rect is translucent, which a fill of the colour is not.
			if a, ok := svgAlphaValue(v); !ok || a != 1 {
				return p, false, false
			}
		case svgOverflow:
			// What shows outside the viewport. The picture is clipped to it
			// here, which is what "hidden", "scroll" and "clip" say and what
			// "visible" and "auto" do not.
			switch ascii.Lower(v) {
			case "hidden", "scroll", "clip":
			default:
				return p, false, false
			}
		default:
			return p, false, false
		}
	}
	if !p.fillSet {
		p.fill = style.RGBA{A: 1} // black, SVG's initial fill
	}
	return p, shown, true
}

// svgAlphaValue reads an <alpha-value>: a number or a percentage, clamped to
// the range [0, 1].
func svgAlphaValue(v string) (float64, bool) {
	scale := 1.0
	if strings.HasSuffix(v, "%") {
		v, scale = strings.TrimSuffix(v, "%"), 100
	}
	n, ok := parseNumber(strings.TrimSpace(v))
	if !ok {
		return 0, false
	}
	return math.Min(1, math.Max(0, n/scale)), true
}

// svgAttrKind is what an attribute can do to the picture.
type svgAttrKind uint8

const (
	// svgRefuses is an attribute that changes what is painted in a way this
	// cannot express, and every attribute nobody has classified.
	svgRefuses svgAttrKind = iota
	// svgInert changes nothing a filled, unstroked rectangle paints.
	svgInert
	// svgOwn is an attribute the element reads itself: a rect's geometry, the
	// root's size.
	svgOwn
	svgFill
	svgStroke
	svgVisibility
	svgDisplay
	svgAlpha
	svgOverflow
)

// svgAttributes classifies the attributes a root or a rect may carry, by SVG
// 2's own lists.
//
// Presentation attributes (§6.8's table) are each either read, or inert for a
// filled rectangle — a font, a text property, a marker (§11.6 puts markers on
// paths, lines and polylines, not rects), a stroke's width or dashes when
// there is no stroke, a fill-rule (a rectangle's one contour is inside by
// either rule), a filter's colour space, a rendering hint — or one that
// changes the paint and refuses: a clip, a mask, a filter, a transform. The
// core, aria and event attributes change nothing drawn; "style" is CSS, which
// is not read, and refuses. The conditional-processing attributes decide
// whether an element is rendered at all and refuse. An attribute not here
// refuses as well.
//
// It is one string rather than literals, for the reason
// rectAttributesThatChangeThePaint was: several of these are spelled exactly
// like CSS properties the style package admits it does not implement, and it
// guards that admission with a scan of this module's source for the quoted
// names. See TestUnimplementedPropertiesAreRegistered.
var svgAttributes = func() map[string]svgAttrKind {
	out := map[string]svgAttrKind{}
	for _, line := range strings.Split(`
		fill:fill stroke:stroke visibility:visibility display:display
		opacity:alpha fill-opacity:alpha overflow:overflow
		id:inert class:inert tabindex:inert lang:inert role:inert focusable:inert
		version:inert baseProfile:inert zoomAndPan:inert
		contentScriptType:inert contentStyleType:inert pathLength:inert
		fill-rule:inert clip-rule:inert color:inert
		stroke-width:inert stroke-dasharray:inert stroke-dashoffset:inert
		stroke-linecap:inert stroke-linejoin:inert stroke-miterlimit:inert
		stroke-opacity:inert paint-order:inert vector-effect:inert
		marker-start:inert marker-mid:inert marker-end:inert
		font-family:inert font-size:inert font-size-adjust:inert font-stretch:inert
		font-style:inert font-variant:inert font-weight:inert
		letter-spacing:inert word-spacing:inert text-anchor:inert
		text-decoration:inert text-overflow:inert text-rendering:inert
		white-space:inert writing-mode:inert direction:inert unicode-bidi:inert
		dominant-baseline:inert alignment-baseline:inert baseline-shift:inert
		glyph-orientation-horizontal:inert glyph-orientation-vertical:inert
		color-interpolation:inert color-interpolation-filters:inert
		color-rendering:inert shape-rendering:inert image-rendering:inert
		flood-color:inert flood-opacity:inert lighting-color:inert
		stop-color:inert stop-opacity:inert mask-type:inert
		cursor:inert pointer-events:inert
		transform:refuses transform-origin:refuses clip-path:refuses mask:refuses
		filter:refuses style:refuses rx:refuses ry:refuses
		requiredExtensions:refuses requiredFeatures:refuses systemLanguage:refuses
	`, "\n") {
		for _, f := range strings.Fields(line) {
			name, kind, _ := strings.Cut(f, ":")
			out[ascii.Lower(name)] = map[string]svgAttrKind{
				"fill": svgFill, "stroke": svgStroke, "visibility": svgVisibility,
				"display": svgDisplay, "alpha": svgAlpha, "overflow": svgOverflow,
				"inert": svgInert, "refuses": svgRefuses,
			}[kind]
		}
	}
	return out
}()

// svgRootAttribute and svgRectAttribute are the attributes each element reads
// as its own.
func svgRootAttribute(name string) (svgAttrKind, bool) {
	switch name {
	case "width", "height", "viewbox", "preserveaspectratio":
		return svgOwn, true
	case "x", "y":
		// Not read on an outermost <svg>, which is placed by the page.
		return svgInert, true
	}
	return 0, false
}

func svgRectAttribute(name string) (svgAttrKind, bool) {
	switch name {
	case "x", "y", "width", "height":
		return svgOwn, true
	}
	return 0, false
}

// svgAttrKindOf classifies one attribute by its name.
func svgAttrKindOf(n xml.Name, own func(string) (svgAttrKind, bool)) svgAttrKind {
	local := ascii.Lower(n.Local)
	switch {
	case n.Space == "xmlns" || local == "xmlns":
		// A namespace declaration.
		return svgInert
	case n.Space == "xml" || n.Space == "http://www.w3.org/XML/1998/namespace":
		// xml:space, xml:lang and xml:base: white space, language and the
		// base of references, none of which a rect has.
		return svgInert
	case n.Space != "" && n.Space != "http://www.w3.org/2000/svg":
		// Another vocabulary's attribute — an editor's own, "inkscape:label",
		// or xlink:href, which neither element read here uses. SVG 2 §4.3
		// has a renderer ignore it, and so does this.
		return svgInert
	case strings.HasPrefix(local, "aria-"), strings.HasPrefix(local, "data-"),
		strings.HasPrefix(local, "on"):
		// Accessibility, a document's own data, and event handlers, which
		// nothing here runs.
		return svgInert
	}
	if k, ok := own(local); ok {
		return k
	}
	if k, ok := svgAttributes[local]; ok {
		return k
	}
	return svgRefuses
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
		v, ok := parseNumber(strings.TrimSuffix(s, "%"))
		if !ok {
			return svgLen{}, false
		}
		return svgLen{value: v, percent: true}, true
	}
	v, ok := parseNumber(strings.TrimSuffix(s, "px"))
	if !ok {
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
		v, ok := parseNumber(f)
		if !ok {
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
	if ascii.EqualFold(s, "none") {
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
		if ascii.EqualFold(a.Name.Local, name) {
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
		if strings.HasSuffix(ascii.Lower(s), unit) {
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
	v, ok := parseNumber(s)
	if !ok || v < 0 {
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
	v, ok := parseNumber(strings.TrimSpace(strings.TrimSuffix(s, "%")))
	if !ok || v <= 0 {
		return 0, false
	}
	return v / 100, true
}
