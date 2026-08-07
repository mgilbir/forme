package shape

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/font"
)

// Loading a variable font at a chosen point in its design space.
//
// # Why this is not a setting
//
// A variable font stores one set of outlines and a table of deltas that move
// their points. The stored outlines are the *default instance* — the point where
// every axis sits at its own default value — and everything else in the design
// space exists only as deltas that have to be applied to get it. So Load, which
// hands back what glyf stores, can only ever hand back the default, and there is
// no flag that makes it hand back anything else.
//
// That default is not the neutral choice it sounds like. Across the fonts Google
// publishes under the Open Font Licence, 217 of the 748 faces with a weight axis
// default to something lighter than Regular and 105 of those to Thin, so taking
// the default gets a light face nearly a third of the time. Worse, five of them
// are called Regular in their PostScript name while their outlines are Thin or
// ExtraLight: the legacy name records can spell only four styles, so a face whose
// default is Thin has nowhere honest to say so. Nothing about the stored font
// tells a caller it got a light one.
//
// LoadInstance is the answer: name a point in the design space and get the face
// that was drawn there. It rewrites glyf, loca and hmtx and returns an ordinary
// static font, because that is what the design point *is* once it is chosen, and
// because everything downstream — subsetting, embedding, a PDF reader — takes a
// static font and would have to instance the thing itself otherwise.
//
// # What is instanced and what is not
//
//   - Outlines move, by gvar, including the points no tuple lists (glyfpoints.go
//     and gvar.go).
//   - Composite components move: their offsets are the points gvar varies for a
//     composite.
//   - Advances come from HVAR where the font has it and from gvar's phantom
//     points otherwise (varstore.go says why that order). A font with neither
//     keeps the advances hmtx already states.
//   - The weight and width classes OS/2 states, and post's italic angle, are
//     rewritten from the location, so that a face reports the weight it draws.
//     head's macStyle and the 'ital' axis are not: see instanceDesign.
//   - Vertical metrics do *not* move. MVAR — which varies ascent, descent, cap
//     height and the rest of the font-wide numbers — is dropped rather than
//     applied, so those stay at the default instance's values. For the bundled
//     face the whole of MVAR moves the ascent by at most a few units across the
//     weight axis; it is a real gap and a small one, and it is stated here rather
//     than guessed at in the code.
//   - Hinting is dropped: cvt, fpgm, prep and every glyph's instructions go,
//     because 'cvar' — which varies the control values — is not read, and hinting
//     a bold face by a thin one's control values is worse than not hinting it.
//   - CFF2 is refused. Its outlines vary through a different mechanism entirely
//     and none of this touches it.

// maxInstanceAxes bounds fvar's axis count. The format allows 65535; the fonts
// that exist have between one and five, and every axis multiplies the work each
// tuple costs.
const maxInstanceAxes = 64

// maxInstanceWork bounds the point arithmetic instancing one font may ask for,
// shared across every glyph. A tuple costs one unit per point of its glyph
// whatever it lists, because the points it does not list are inferred from the
// ones it does, and that walks the glyph.
//
// The budget is what stops a small file from asking for a large amount of work:
// a glyph may declare sixty thousand points and four thousand tuples in a few
// kilobytes of gvar, which is a quarter of a billion operations for that glyph
// alone. The largest of the fonts here — 4,515 glyphs, 23,062 tuples — spends
// about two million.
const maxInstanceWork = 1 << 28

// maxComponentDepth bounds how deep a composite glyph may nest when its bounding
// box is computed. Composites nest two or three deep in practice (an accented
// letter built from a letter built from nothing); a font whose components refer
// to each other in a circle nests forever, and this is what stops it.
const maxComponentDepth = 8

// instanceDropped are the tables an instance does not carry. The variation
// tables describe a design space this font no longer has, and would be read by
// anything downstream as deltas from a default instance that is no longer the
// stored one — which is worse than their absence. The hinting tables go with the
// instructions, see the note above.
var instanceDropped = map[string]bool{
	"fvar": true, "gvar": true, "avar": true, "cvar": true,
	"HVAR": true, "VVAR": true, "MVAR": true, "STAT": true,
	"cvt ": true, "fpgm": true, "prep": true,
}

// varAxis is one axis of the design space in user coordinates — the numbers a
// caller names, such as 400 for Regular weight.
type varAxis struct {
	tag           string
	min, def, max float64
	nameID        int
}

// LoadInstance parses a variable font and prepares the face it draws at one
// point in its design space, ready to embed like any other.
//
// The coordinates are in user space and named by axis tag, so a Regular weight
// at three-quarter width is map[string]float64{"wght": 400, "wdth": 75}. An axis
// the caller does not name stays at its own default. An axis the *font* does not
// have is an error rather than a value ignored: a caller asking for a weight from
// a font with no weight axis has asked for something it will not get, and would
// otherwise get the default silently — which is the whole problem this exists to
// fix.
//
// A value outside its axis's range is clamped to the range, as the specification
// requires of any tool that reads a location. Asking for weight 700 from a face
// that stops at 600 gets the boldest it has.
//
// The face this returns is static: its outlines are the ones drawn at that
// location, and it carries no variation tables. Load remains the way to read a
// font as it stands, which for a variable font is its default instance.
func LoadInstance(data []byte, coords map[string]float64) (*Face, error) {
	out, normalized, err := instanceProgram(data, coords)
	if err != nil {
		return nil, err
	}
	f, err := loadFace(out, normalized)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// instanceProgram does the rewrite, returning the new font program and the
// location in normalized coordinates.
func instanceProgram(data []byte, want map[string]float64) ([]byte, []float64, error) {
	tables := font.SFNTTables(data)
	if tables == nil {
		return nil, nil, errors.New("fonts: not an sfnt font program (TrueType or OpenType)")
	}
	if _, ok := tables["CFF2"]; ok {
		return nil, nil, errors.New("fonts: CFF2 variable fonts are not supported; their outlines vary by a mechanism this does not read")
	}
	fvar := tables["fvar"]
	if fvar == nil {
		return nil, nil, errors.New("fonts: the font has no fvar table, so it is not a variable font and has no design space to be loaded at")
	}
	head, hhea, hmtx, maxp := tables["head"], tables["hhea"], tables["hmtx"], tables["maxp"]
	glyf, loca := tables["glyf"], tables["loca"]
	if glyf == nil || loca == nil {
		return nil, nil, errors.New("fonts: the font has no glyf outlines to instance")
	}
	if len(head) < 54 || len(hhea) < 36 || len(maxp) < 6 || hmtx == nil {
		return nil, nil, errors.New("fonts: the font lacks head, hhea, hmtx or maxp")
	}

	axes, err := parseFvar(fvar)
	if err != nil {
		return nil, nil, err
	}
	coords, err := normalizeLocation(axes, tables["avar"], want)
	if err != nil {
		return nil, nil, err
	}

	numGlyphs := font.Be16(maxp, 4)
	if numGlyphs <= 0 {
		return nil, nil, fmt.Errorf("fonts: the font declares %d glyphs; every font has at least .notdef", numGlyphs)
	}
	offsets, err := parseLoca(loca, numGlyphs, binary.BigEndian.Uint16(head[50:]) == 1)
	if err != nil {
		return nil, nil, err
	}
	advances, bearings, err := parseHmtx(hmtx, font.Be16(hhea, 34), numGlyphs)
	if err != nil {
		return nil, nil, err
	}

	var gvar *gvarTable
	if t := tables["gvar"]; t != nil {
		if gvar, err = parseGvar(t, numGlyphs, len(axes)); err != nil {
			return nil, nil, err
		}
	}
	var hvar *hvarTable
	if t := tables["HVAR"]; t != nil {
		if hvar, err = parseHVAR(t); err != nil {
			return nil, nil, err
		}
	}

	budget := int64(maxInstanceWork)
	newGlyf := make([]byte, 0, len(glyf))
	newLoca := make([]uint32, numGlyphs+1)
	// The left side bearing point of each glyph after its deltas: the bearing
	// this instance writes is measured from it, and it is not recoverable from
	// the outline afterwards.
	origins := make([]float64, numGlyphs)
	newAdvances := make([]int, numGlyphs)
	for gid := 0; gid < numGlyphs; gid++ {
		start, end := offsets[gid], offsets[gid+1]
		if start > end || int(end) > len(glyf) {
			return nil, nil, fmt.Errorf("fonts: glyph %d lies outside the glyf table", gid)
		}
		g, err := decodeVarGlyph(glyf[start:end], numGlyphs)
		if err != nil {
			return nil, nil, fmt.Errorf("fonts: glyph %d: %w", gid, err)
		}
		xMin := 0
		if end > start {
			xMin = int(int16(uint16(font.Be16(glyf[start:end], 2))))
		}
		g.setPhantoms(xMin, bearings[gid], advances[gid])
		if gvar != nil {
			if err := gvar.applyGlyph(gid, g, coords, &budget); err != nil {
				return nil, nil, err
			}
		}
		left, adv := g.advance()
		origins[gid] = left
		switch {
		case hvar != nil:
			newAdvances[gid] = advances[gid] + otRound(hvar.advanceDelta(gid, coords))
		default:
			newAdvances[gid] = otRound(adv)
		}
		if newAdvances[gid] < 0 {
			newAdvances[gid] = 0
		}
		b, err := encodeVarGlyph(g)
		if err != nil {
			return nil, nil, fmt.Errorf("fonts: glyph %d: %w", gid, err)
		}
		newLoca[gid] = uint32(len(newGlyf))
		newGlyf = append(newGlyf, b...)
		for len(newGlyf)%4 != 0 { // glyf entries are long-aligned
			newGlyf = append(newGlyf, 0)
		}
	}
	newLoca[numGlyphs] = uint32(len(newGlyf))

	bounds, err := fillCompositeBounds(newGlyf, newLoca, numGlyphs, &budget)
	if err != nil {
		return nil, nil, err
	}

	out := map[string][]byte{}
	for tag, b := range tables {
		if !instanceDropped[tag] {
			out[tag] = b
		}
	}
	out["glyf"] = newGlyf
	locaBytes := make([]byte, 4*(numGlyphs+1))
	for i, off := range newLoca {
		binary.BigEndian.PutUint32(locaBytes[4*i:], off)
	}
	out["loca"] = locaBytes
	out["hmtx"], out["hhea"] = buildMetrics(hhea, newAdvances, origins, bounds)
	out["head"] = instanceHead(head, bounds)
	instanceDesign(out, axes, want)
	if name := instanceName(tables["name"], fvar, axes, want); name != nil {
		out["name"] = name
	}
	return assembleSFNT(out), coords, nil
}

// parseFvar reads the axis records: what each axis is called and the range of
// values it takes, in the user coordinates a caller names.
//
// The named instances that follow the axes are read separately and only for
// their names — see instanceName. Nothing here needs them to reach a point in
// the design space, because a location is a coordinate and not a name.
func parseFvar(t []byte) ([]varAxis, error) {
	if len(t) < 16 {
		return nil, errors.New("fonts: the fvar table is too short to hold its header")
	}
	if font.Be16(t, 0) != 1 {
		return nil, fmt.Errorf("fonts: fvar is version %d, which this does not read", font.Be16(t, 0))
	}
	at := font.Be16(t, 4)
	count := font.Be16(t, 8)
	size := font.Be16(t, 10)
	if count == 0 {
		return nil, errors.New("fonts: fvar declares no axes, so the font has no design space")
	}
	if count > maxInstanceAxes {
		return nil, fmt.Errorf("fonts: fvar declares %d axes, more than the %d this reads", count, maxInstanceAxes)
	}
	if size < 20 {
		return nil, fmt.Errorf("fonts: fvar states axis records of %d bytes; one is 20", size)
	}
	if at < 16 || at+size*count > len(t) {
		return nil, errors.New("fonts: fvar's axis records lie outside the table")
	}
	axes := make([]varAxis, count)
	for i := range axes {
		rec := at + size*i
		axes[i] = varAxis{
			tag:    string(t[rec : rec+4]),
			min:    fixed1616At(t, rec+4),
			def:    fixed1616At(t, rec+8),
			max:    fixed1616At(t, rec+12),
			nameID: font.Be16(t, rec+18),
		}
		a := axes[i]
		if !(a.min <= a.def && a.def <= a.max) {
			return nil, fmt.Errorf("fonts: fvar's %s axis runs %g..%g with a default of %g", a.tag, a.min, a.max, a.def)
		}
	}
	return axes, nil
}

// fixed1616At reads a signed fixed-point number with sixteen fractional bits,
// which is how fvar states a user-space coordinate.
func fixed1616At(b []byte, off int) float64 {
	return float64(int32(font.Be32(b, off))) / 65536
}

// normalizeLocation turns the user-space location a caller named into the
// -1..1 coordinates every variation table is written in: -1 at an axis's
// minimum, 0 at its default, 1 at its maximum, and what avar says in between.
//
// Zero is the default instance on every axis by construction, which is the fact
// the whole format rests on and the one that makes zero ambiguous everywhere
// else: a coordinate of zero is both "where the stored outlines already are" and
// an ordinary value a caller may have asked for. It is not the same as an axis
// nobody mentioned — that one is normalized from its own default and also comes
// out zero, which is why nothing downstream may treat zero as "unset".
func normalizeLocation(axes []varAxis, avar []byte, want map[string]float64) ([]float64, error) {
	known := make(map[string]bool, len(axes))
	for _, a := range axes {
		known[a.tag] = true
	}
	for tag, v := range want {
		if !known[tag] {
			return nil, fmt.Errorf("fonts: the font has no %q axis; it has %s", tag, axisTags(axes))
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("fonts: the %q coordinate is not a number", tag)
		}
	}
	segments, err := parseAvar(avar, len(axes))
	if err != nil {
		return nil, err
	}
	coords := make([]float64, len(axes))
	for i, a := range axes {
		v, ok := want[a.tag]
		if !ok {
			v = a.def
		}
		coords[i] = normalizeAxis(a, v)
		if segments != nil {
			coords[i] = piecewiseMap(segments[i], coords[i])
		}
	}
	return coords, nil
}

func axisTags(axes []varAxis) string {
	tags := make([]string, len(axes))
	for i, a := range axes {
		tags[i] = strconv.Quote(a.tag)
	}
	return strings.Join(tags, ", ")
}

func normalizeAxis(a varAxis, v float64) float64 {
	switch {
	case v > a.max:
		v = a.max
	case v < a.min:
		v = a.min
	}
	switch {
	case v == a.def:
		return 0
	case v < a.def:
		if a.def == a.min {
			return 0
		}
		return (v - a.def) / (a.def - a.min)
	default:
		if a.max == a.def {
			return 0
		}
		return (v - a.def) / (a.max - a.def)
	}
}

// avarSegment is one point of an axis's mapping: a normalized coordinate, and
// the normalized coordinate it stands for.
type avarSegment struct{ from, to float64 }

// parseAvar reads the axis variations table, which bends the normalized scale so
// that the middle of an axis need not be the middle of what it draws — the point
// halfway along a weight axis is rarely the weight halfway between the two ends.
//
// A font without one maps every coordinate to itself.
func parseAvar(t []byte, axisCount int) ([][]avarSegment, error) {
	if len(t) == 0 {
		return nil, nil
	}
	if len(t) < 8 {
		return nil, errors.New("fonts: the avar table is too short to hold its header")
	}
	if v := font.Be16(t, 0); v != 1 {
		return nil, fmt.Errorf("fonts: avar is version %d, which this does not read", v)
	}
	if minor := font.Be16(t, 2); minor != 0 {
		// Version 1.1 adds a second mapping stage on top of the segment maps.
		// Reading only the first would place the location somewhere the font
		// does not draw, silently.
		return nil, fmt.Errorf("fonts: avar is version 1.%d, whose extra mapping this does not read", minor)
	}
	if n := font.Be16(t, 6); n != axisCount {
		return nil, fmt.Errorf("fonts: avar maps %d axes and fvar declares %d", n, axisCount)
	}
	out := make([][]avarSegment, axisCount)
	at := 8
	for i := 0; i < axisCount; i++ {
		if at+2 > len(t) {
			return nil, errors.New("fonts: avar's segment maps are cut short")
		}
		n := font.Be16(t, at)
		at += 2
		if at+4*n > len(t) {
			return nil, errors.New("fonts: avar's segment maps are cut short")
		}
		seg := make([]avarSegment, n)
		for j := range seg {
			seg[j] = avarSegment{from: f2Dot14At(t, at), to: f2Dot14At(t, at+2)}
			at += 4
		}
		out[i] = seg
	}
	return out, nil
}

// piecewiseMap applies one axis's segment map: an exact match takes its value, a
// coordinate between two takes the line between them, and one outside the map
// keeps its distance from the nearest end.
func piecewiseMap(seg []avarSegment, v float64) float64 {
	if len(seg) == 0 {
		return v
	}
	lo, hi := seg[0], seg[0]
	var below, above *avarSegment
	for i := range seg {
		s := seg[i]
		if s.from == v {
			return s.to
		}
		if s.from < lo.from {
			lo = s
		}
		if s.from > hi.from {
			hi = s
		}
		if s.from < v && (below == nil || s.from > below.from) {
			below = &seg[i]
		}
		if s.from > v && (above == nil || s.from < above.from) {
			above = &seg[i]
		}
	}
	if v < lo.from {
		return v + lo.to - lo.from
	}
	if v > hi.from {
		return v + hi.to - hi.from
	}
	if below == nil || above == nil {
		return v
	}
	return below.to + float64((above.to-below.to)*(v-below.from)/(above.from-below.from))
}

// parseHmtx reads the advance and left side bearing of every glyph. The table
// states both for the first numberOfHMetrics glyphs and a bearing alone for the
// rest, which all share the last advance — how a font with a long monospaced
// tail is stored.
func parseHmtx(t []byte, numberOfHMetrics, numGlyphs int) (advances, bearings []int, err error) {
	if numberOfHMetrics <= 0 {
		return nil, nil, errors.New("fonts: hhea states no horizontal metrics")
	}
	if numberOfHMetrics > numGlyphs {
		numberOfHMetrics = numGlyphs
	}
	if 4*numberOfHMetrics > len(t) {
		return nil, nil, fmt.Errorf("fonts: hmtx holds %d bytes, too few for %d metrics", len(t), numberOfHMetrics)
	}
	advances = make([]int, numGlyphs)
	bearings = make([]int, numGlyphs)
	last := 0
	for gid := 0; gid < numGlyphs; gid++ {
		if gid < numberOfHMetrics {
			last = font.Be16(t, 4*gid)
			advances[gid] = last
			bearings[gid] = int(int16(uint16(font.Be16(t, 4*gid+2))))
			continue
		}
		advances[gid] = last
		// A truncated tail of bearings is common enough in the wild, and a
		// bearing is recomputed from the outline here anyway.
		if at := 4*numberOfHMetrics + 2*(gid-numberOfHMetrics); at+2 <= len(t) {
			bearings[gid] = int(int16(uint16(font.Be16(t, at))))
		}
	}
	return advances, bearings, nil
}

// glyphBounds is one glyph's bounding box in the instanced font.
type glyphBounds struct {
	xMin, yMin, xMax, yMax int
	empty                  bool
}

// fillCompositeBounds computes each glyph's bounding box and writes the
// composites' back into the glyf entries.
//
// A composite's box cannot be known while it is being instanced: it is the box
// of its components *after* they have been instanced, and they may not have been
// yet. So this is a second pass over the finished outlines, which also has the
// property that a composite is measured from the same bytes a reader will
// measure it from.
func fillCompositeBounds(glyf []byte, loca []uint32, numGlyphs int, budget *int64) ([]glyphBounds, error) {
	bounds := make([]glyphBounds, numGlyphs)
	for gid := 0; gid < numGlyphs; gid++ {
		start, end := loca[gid], loca[gid+1]
		if start >= end {
			bounds[gid] = glyphBounds{empty: true}
			continue
		}
		entry := glyf[start:end]
		if int16(uint16(font.Be16(entry, 0))) >= 0 {
			bounds[gid] = glyphBounds{
				xMin: int(int16(uint16(font.Be16(entry, 2)))),
				yMin: int(int16(uint16(font.Be16(entry, 4)))),
				xMax: int(int16(uint16(font.Be16(entry, 6)))),
				yMax: int(int16(uint16(font.Be16(entry, 8)))),
			}
			continue
		}
		var box floatBounds
		if err := accumulateBounds(glyf, loca, numGlyphs, gid, identityTransform, &box, 0, budget); err != nil {
			return nil, fmt.Errorf("fonts: glyph %d: %w", gid, err)
		}
		if !box.set {
			bounds[gid] = glyphBounds{empty: true}
			continue
		}
		bounds[gid] = glyphBounds{
			xMin: otRound(box.xMin), yMin: otRound(box.yMin),
			xMax: otRound(box.xMax), yMax: otRound(box.yMax),
		}
		b := bounds[gid]
		if b.xMin < math.MinInt16 || b.xMax > math.MaxInt16 || b.yMin < math.MinInt16 || b.yMax > math.MaxInt16 {
			return nil, fmt.Errorf("fonts: glyph %d's instanced bounding box is outside the range glyf can store", gid)
		}
		putBounds(entry, b.xMin, b.yMin, b.xMax, b.yMax)
	}
	return bounds, nil
}

// transform is a component's placement: the two-by-two it is drawn through and
// the offset it is drawn at.
type transform struct{ a, b, c, d, dx, dy float64 }

var identityTransform = transform{a: 1, d: 1}

func (t transform) apply(x, y float64) (float64, float64) {
	// The conversions keep the multiplies from being fused into the adds; see
	// the note in gvar.go's infer.
	return float64(t.a*x) + float64(t.c*y) + t.dx, float64(t.b*x) + float64(t.d*y) + t.dy
}

// concat composes an outer placement with an inner one: the inner is applied
// first. A component's offset is not put through the outer scale, which is what
// the format's unscaled-offset default says and what every reader does.
func (t transform) concat(in transform) transform {
	dx, dy := t.apply(in.dx, in.dy)
	return transform{
		a:  float64(t.a*in.a) + float64(t.c*in.b),
		b:  float64(t.b*in.a) + float64(t.d*in.b),
		c:  float64(t.a*in.c) + float64(t.c*in.d),
		d:  float64(t.b*in.c) + float64(t.d*in.d),
		dx: dx, dy: dy,
	}
}

type floatBounds struct {
	xMin, yMin, xMax, yMax float64
	set                    bool
}

func (f *floatBounds) add(x, y float64) {
	if !f.set {
		f.xMin, f.yMin, f.xMax, f.yMax, f.set = x, y, x, y, true
		return
	}
	f.xMin, f.xMax = math.Min(f.xMin, x), math.Max(f.xMax, x)
	f.yMin, f.yMax = math.Min(f.yMin, y), math.Max(f.yMax, y)
}

// accumulateBounds walks a glyph's points through a placement, following
// components into the glyphs they name.
func accumulateBounds(glyf []byte, loca []uint32, numGlyphs, gid int, t transform, box *floatBounds, depth int, budget *int64) error {
	if depth > maxComponentDepth {
		return fmt.Errorf("its components nest more than %d deep", maxComponentDepth)
	}
	if gid < 0 || gid >= numGlyphs {
		return nil
	}
	start, end := loca[gid], loca[gid+1]
	if start >= end {
		return nil
	}
	g, err := decodeVarGlyph(glyf[start:end], numGlyphs)
	if err != nil {
		return err
	}
	if *budget -= int64(g.numPoints()); *budget < 0 {
		return fmt.Errorf("measuring this font's composites needs more than %d point operations", int64(maxInstanceWork))
	}
	if !g.composite {
		for i := 0; i < g.numOutlinePoints(); i++ {
			box.add(t.apply(g.x[i], g.y[i]))
		}
		return nil
	}
	for i, c := range g.comps {
		inner := transform{
			a: c.scale[0], b: c.scale[1], c: c.scale[2], d: c.scale[3],
			dx: g.x[i], dy: g.y[i],
		}
		if err := accumulateBounds(glyf, loca, numGlyphs, c.glyph, t.concat(inner), box, depth+1, budget); err != nil {
			return err
		}
	}
	return nil
}

// buildMetrics writes hmtx and the fields of hhea that describe it.
//
// The bearing is measured from the glyph's own origin — where its first phantom
// point ended up — rather than assumed to be its xMin, because a font is free to
// place the two apart and instancing does not move them together.
func buildMetrics(hhea []byte, advances []int, origins []float64, bounds []glyphBounds) (hmtx, newHhea []byte) {
	n := len(advances)
	lsb := make([]int, n)
	for gid := range advances {
		if bounds[gid].empty {
			lsb[gid] = 0
			continue
		}
		lsb[gid] = otRound(float64(bounds[gid].xMin) - origins[gid])
	}
	// The trailing glyphs that share one advance need no advance of their own,
	// which is what numberOfHMetrics is for.
	metrics := n
	for metrics > 1 && advances[metrics-1] == advances[metrics-2] {
		metrics--
	}
	hmtx = make([]byte, 4*metrics+2*(n-metrics))
	for gid := 0; gid < n; gid++ {
		if gid < metrics {
			binary.BigEndian.PutUint16(hmtx[4*gid:], uint16(clampU16(advances[gid])))
			binary.BigEndian.PutUint16(hmtx[4*gid+2:], uint16(int16(clampI16(lsb[gid]))))
			continue
		}
		binary.BigEndian.PutUint16(hmtx[4*metrics+2*(gid-metrics):], uint16(int16(clampI16(lsb[gid]))))
	}

	newHhea = append([]byte(nil), hhea...)
	maxAdv, minLSB, minRSB, maxExtent := 0, math.MaxInt32, math.MaxInt32, math.MinInt32
	for gid := 0; gid < n; gid++ {
		if advances[gid] > maxAdv {
			maxAdv = advances[gid]
		}
		if bounds[gid].empty {
			continue
		}
		width := bounds[gid].xMax - bounds[gid].xMin
		rsb := advances[gid] - lsb[gid] - width
		if lsb[gid] < minLSB {
			minLSB = lsb[gid]
		}
		if rsb < minRSB {
			minRSB = rsb
		}
		if e := lsb[gid] + width; e > maxExtent {
			maxExtent = e
		}
	}
	if minLSB == math.MaxInt32 {
		minLSB, minRSB, maxExtent = 0, 0, 0
	}
	binary.BigEndian.PutUint16(newHhea[10:], uint16(clampU16(maxAdv)))
	binary.BigEndian.PutUint16(newHhea[12:], uint16(int16(clampI16(minLSB))))
	binary.BigEndian.PutUint16(newHhea[14:], uint16(int16(clampI16(minRSB))))
	binary.BigEndian.PutUint16(newHhea[16:], uint16(int16(clampI16(maxExtent))))
	binary.BigEndian.PutUint16(newHhea[34:], uint16(metrics))
	return hmtx, newHhea
}

func clampU16(v int) int {
	if v < 0 {
		return 0
	}
	if v > 0xFFFF {
		return 0xFFFF
	}
	return v
}

func clampI16(v int) int {
	if v < math.MinInt16 {
		return math.MinInt16
	}
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	return v
}

// instanceHead updates the font-wide bounding box and says that loca is now in
// its long form, which is the only form this writes.
func instanceHead(head []byte, bounds []glyphBounds) []byte {
	out := append([]byte(nil), head...)
	var box floatBounds
	for _, b := range bounds {
		if b.empty {
			continue
		}
		box.add(float64(b.xMin), float64(b.yMin))
		box.add(float64(b.xMax), float64(b.yMax))
	}
	if box.set {
		putBounds(out[34:], int(box.xMin), int(box.yMin), int(box.xMax), int(box.yMax))
	}
	binary.BigEndian.PutUint16(out[50:], 1)
	return out
}

// instanceDesign updates the tables that state in plain numbers which face this
// is: OS/2's weight and width classes and post's italic angle.
//
// Nothing in them is variation data, and instancing does not touch the outlines
// they describe, so it would be easy to leave them alone — and they would then
// say the *default* instance's weight for a face cut somewhere else.
// Descriptor().Weight is what a caller reads to find out what was drawn, and a
// face reporting Thin while drawing Bold is the misreporting this file exists to
// end.
//
// Only the three registered axes whose meaning is a number in these tables are
// read. The 'ital' axis and head's macStyle bits are left alone: macStyle is two
// bits and the axis is continuous, so there is no honest value to write for a
// face halfway along it. An axis a foundry defined for itself has no field here
// and none is invented for it.
func instanceDesign(out map[string][]byte, axes []varAxis, want map[string]float64) {
	// A copy per table, written once: the tables map still holds the bytes the
	// caller's font program was parsed out of, and those are not this
	// function's to write into.
	os2 := append([]byte(nil), out["OS/2"]...)
	post := append([]byte(nil), out["post"]...)
	for _, a := range axes {
		v, ok := want[a.tag]
		if !ok {
			continue
		}
		v = clampAxis(a, v)
		switch {
		case a.tag == "wght" && len(os2) >= 6:
			binary.BigEndian.PutUint16(os2[4:], uint16(clampInt(otRound(v), 1, 1000)))
			out["OS/2"] = os2
		case a.tag == "wdth" && len(os2) >= 8:
			binary.BigEndian.PutUint16(os2[6:], uint16(widthClass(v)))
			out["OS/2"] = os2
		case a.tag == "slnt" && len(post) >= 8:
			angle := math.Max(-90, math.Min(90, v))
			binary.BigEndian.PutUint32(post[4:], uint32(int32(math.Round(angle*65536))))
			out["post"] = post
		}
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// widthClassBounds are the midpoints between the nine widths OS/2's width class
// names, in the percentages the 'wdth' axis is stated in — 100 is normal, 75 is
// condensed — so a width falls into the class it is nearest.
var widthClassBounds = [8]float64{56.25, 68.75, 81.25, 93.75, 106.25, 118.75, 137.5, 175}

// widthClass is the OS/2 width class a 'wdth' percentage falls in, from 1
// (ultra-condensed) to 9 (ultra-expanded). A percentage exactly on a boundary
// takes the wider class, which is where fontTools' own bisection puts it.
func widthClass(percent float64) int {
	n := 1
	for _, b := range widthClassBounds {
		if percent < b {
			break
		}
		n++
	}
	return n
}

// instanceName rewrites the PostScript name so that it says which instance this
// is, and returns nil when there is nothing to change.
//
// It matters because that name becomes /BaseFont in a PDF, and because the name
// a variable font carries is the *default* instance's. Leaving it alone would
// put a document's bold text under a name that says Regular — and worse, two
// weights of one face into one document under one name, where nothing downstream
// could tell them apart.
//
// A location that is one of the font's own named instances takes that instance's
// PostScript name, which is the name its publisher chose. Anything else is named
// from its coordinates, which is ugly and unambiguous — the two properties that
// matter for a name nobody reads and everything compares.
func instanceName(name, fvar []byte, axes []varAxis, want map[string]float64) []byte {
	if len(name) == 0 {
		return nil
	}
	if named := namedInstanceName(fvar, name, axes, want); named != "" {
		if named == postScriptName(name) {
			return nil
		}
		return replacePostScriptName(name, sanitizeName(named))
	}
	suffix := ""
	for _, a := range axes {
		v, ok := want[a.tag]
		if !ok {
			continue
		}
		if v = clampAxis(a, v); v == a.def {
			continue
		}
		suffix += "-" + a.tag + strconv.FormatFloat(v, 'g', -1, 64)
	}
	if suffix == "" {
		return nil // the default instance, which is the name the font already has
	}
	base := postScriptName(name)
	if base == "" {
		base = "Instance"
	}
	return replacePostScriptName(name, sanitizeName(base+suffix))
}

// namedInstanceName is the PostScript name the font itself gives a location,
// when the location is one of the instances it names and that instance has one.
//
// fvar's instance records are the font's own list of the points worth going to,
// each with the names its publisher chose. They are only useful to something
// that can go there, which is what this file is for; "NotoSans-Bold" is a better
// /BaseFont than a name spelled out of coordinates, and it is the name the
// separately published static build carries.
func namedInstanceName(fvar, name []byte, axes []varAxis, want map[string]float64) string {
	if len(fvar) < 16 || len(name) == 0 {
		return ""
	}
	at := font.Be16(fvar, 4) + font.Be16(fvar, 8)*font.Be16(fvar, 10)
	count := font.Be16(fvar, 12)
	size := font.Be16(fvar, 14)
	// A record is a subfamily name, flags, one coordinate per axis and — only
	// if the record is long enough to hold it — a PostScript name.
	minimum := 4 + 4*len(axes)
	if size < minimum+2 || at+size*count > len(fvar) {
		return ""
	}
	for i := 0; i < count; i++ {
		rec := at + size*i
		matches := true
		for j, a := range axes {
			v, ok := want[a.tag]
			if !ok {
				v = a.def
			}
			if clampAxis(a, v) != fixed1616At(fvar, rec+4+4*j) {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		if id := font.Be16(fvar, rec+minimum); id != 0xFFFF {
			return nameByID(name, id)
		}
		return ""
	}
	return ""
}

func clampAxis(a varAxis, v float64) float64 {
	if v < a.min {
		return a.min
	}
	if v > a.max {
		return a.max
	}
	return v
}

// replacePostScriptName rewrites every name-table record carrying name ID 6.
//
// The table is rebuilt rather than patched because its strings are packed into
// one block that records index into, and may overlap: there is nowhere to write
// a longer string and no way to know which other record shares the old one.
func replacePostScriptName(name []byte, psName string) []byte {
	if len(name) < 6 {
		return nil
	}
	format := font.Be16(name, 0)
	count := font.Be16(name, 2)
	storage := font.Be16(name, 4)
	if 6+12*count > len(name) {
		return nil
	}
	// Format 1 states language tags of its own after the records, and records
	// name their language by pointing into that list, so it travels with them.
	langAt := 6 + 12*count
	langCount := 0
	if format == 1 {
		if langAt+2 > len(name) {
			return nil
		}
		langCount = font.Be16(name, langAt)
		if langAt+2+4*langCount > len(name) {
			return nil
		}
	}

	type record struct {
		platform, encoding, language, id int
		value                            []byte
	}
	records := make([]record, 0, count)
	for i := 0; i < count; i++ {
		rec := 6 + 12*i
		r := record{
			platform: font.Be16(name, rec),
			encoding: font.Be16(name, rec+2),
			language: font.Be16(name, rec+4),
			id:       font.Be16(name, rec+6),
		}
		length := font.Be16(name, rec+8)
		off := storage + font.Be16(name, rec+10)
		if off+length > len(name) {
			return nil
		}
		r.value = name[off : off+length]
		if r.id == 6 {
			r.value = encodeNameString(psName, r.platform)
		}
		records = append(records, r)
	}

	head := 6 + 12*count
	if format == 1 {
		head += 2 + 4*langCount
	}
	out := make([]byte, head)
	binary.BigEndian.PutUint16(out[0:], uint16(format))
	binary.BigEndian.PutUint16(out[2:], uint16(count))
	binary.BigEndian.PutUint16(out[4:], uint16(head))
	var strs []byte
	for i, r := range records {
		rec := 6 + 12*i
		binary.BigEndian.PutUint16(out[rec:], uint16(r.platform))
		binary.BigEndian.PutUint16(out[rec+2:], uint16(r.encoding))
		binary.BigEndian.PutUint16(out[rec+4:], uint16(r.language))
		binary.BigEndian.PutUint16(out[rec+6:], uint16(r.id))
		binary.BigEndian.PutUint16(out[rec+8:], uint16(len(r.value)))
		binary.BigEndian.PutUint16(out[rec+10:], uint16(len(strs)))
		strs = append(strs, r.value...)
	}
	for i := 0; i < langCount; i++ {
		at := 6 + 12*count + 2 + 4*i
		length := font.Be16(name, at)
		off := storage + font.Be16(name, at+2)
		if off+length > len(name) {
			return nil
		}
		binary.BigEndian.PutUint16(out[at:], uint16(length))
		binary.BigEndian.PutUint16(out[at+2:], uint16(len(strs)))
		strs = append(strs, name[off:off+length]...)
	}
	if format == 1 {
		binary.BigEndian.PutUint16(out[6+12*count:], uint16(langCount))
	}
	// The storage offset is a sixteen-bit field: a name table whose records do
	// not fit under it cannot be written at all.
	if head > 0xFFFF || len(strs) > 0xFFFF {
		return nil
	}
	return append(out, strs...)
}

// encodeNameString writes a name in the encoding its platform uses: two bytes
// per character for Windows and Unicode, one for Macintosh.
func encodeNameString(s string, platform int) []byte {
	if platform == 1 { // Macintosh, single byte
		return []byte(s)
	}
	out := make([]byte, 0, 2*len(s))
	for _, r := range s {
		if r > 0xFFFF {
			r = 0xFFFD
		}
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}
