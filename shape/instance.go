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
//     composite. A component placed by matching points is matched among the
//     points as they are at the location, and kept as a match where the
//     instance's own points make the same one, and placed at an offset where
//     they would not (pointmatch.go).
//   - Advances come from HVAR where the font has it and from gvar's phantom
//     points otherwise (varstore.go says why that order). A font with neither
//     keeps the advances hmtx already states.
//   - The weight and width classes OS/2 states, and post's italic angle, are
//     rewritten from the location, so that a face reports the weight it draws.
//     head's macStyle and the 'ital' axis are not: see instanceDesign.
//   - Vertical advances and top side bearings — what a glyph set upright is
//     advanced and hung by (vertical.go) — move as HarfBuzz moves them: the
//     advance by VVAR where the font has it and by the vertical phantom points
//     gvar moves otherwise, and the point a glyph hangs from with its top
//     phantom point. vmtx is rewritten to say both, the side bearing measured
//     from the instanced glyph's box, so that the instance hangs its glyphs
//     where HarfBuzz hangs them at the location. VORG is a CFF face's, and is
//     moved with its outlines (instanceCFF2).
//   - A composite that takes its metrics from a component (USE_MY_METRICS)
//     is given that component's phantom points, as HarfBuzz reads them, both
//     across the page and down it. fontTools' instancer gives it its own, and
//     the two differ where the composite's own points moved differently: in
//     Noto Sans at weight 700 with no HVAR, glyphs 3248 and 3360 advance 867
//     and 594 by HarfBuzz and 875 and 592 by fontTools. Following HarfBuzz
//     was chosen, since what a face advances by is what it is shaped with;
//     testdata/varinstance records the two as fontTools' disagreement.
//   - The font-wide numbers move by MVAR: the ascent, descent and line gap,
//     the x-height and cap height, the underline, the strikeout, the sub- and
//     superscript boxes and the caret. They are written as HarfBuzz reads them
//     at the location, which for the ascent, descent and line gap is not where
//     fontTools' instancer writes them — see mvar.go. The table itself is then
//     dropped with the other variation tables.
//   - Hinting is dropped: cvt, fpgm, prep and every glyph's instructions go,
//     because 'cvar' — which varies the control values — is not read, and hinting
//     a bold face by a thin one's control values is worse than not hinting it.
//   - A font whose outlines are CFF2 is cut by instanceCFF2 (cff2cff.go): its
//     charstrings blend their own variations, and the instance is written as
//     the CFF font they draw at the location. VORG, which a CFF face states
//     its vertical origins in, moves there by VVAR.
//   - A glyph VARC composes is written out as the glyf outline HarfBuzz draws
//     for it at the location, and VARC is dropped; its ink stays what
//     HarfBuzz states there (varcinstance.go).

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
// stored one — which is worse than their absence. VARC goes too, its glyphs
// written out as glyf (varcinstance.go). The hinting tables go with the
// instructions, see the note above.
var instanceDropped = map[string]bool{
	"fvar": true, "gvar": true, "avar": true, "cvar": true, "VARC": true,
	"HVAR": true, "VVAR": true, "MVAR": true, "STAT": true,
	"cvt ": true, "fpgm": true, "prep": true,
	// CFF2 outlines vary by their own blends, and a static font carries none:
	// a CFF2 font's instance carries them as CFF (instanceCFF2), and a font
	// with glyf outlines beside a CFF2 table is instanced by its glyf, as
	// Load draws it.
	"CFF2": true,
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
	if tables := font.SFNTTables(data); tables != nil && tables["CFF2"] != nil &&
		tables["glyf"] == nil && tables["CFF "] == nil {
		// CFF2 outlines, which are cut as the CFF font they draw at the
		// location: see instanceCFF2.
		if tables["VARC"] != nil {
			// VARC is among the tables an instance drops, because the glyf
			// path writes its glyphs out as outlines first (varcinstance.go).
			// Nothing writes out a composite whose leaves are CFF2, so
			// dropping the table here would leave each composite drawing its
			// empty base glyph with nothing to say so.
			return nil, errors.New("fonts: the font's variable composite glyphs (VARC) are built on " +
				"CFF2 outlines, and an instance cannot write them out; the instance is refused rather " +
				"than drawn without them")
		}
		out, normalized, limits, err := instanceCFF2(tables, coords)
		if err != nil {
			return nil, err
		}
		f, err := loadFace(out, normalized)
		if err != nil {
			return nil, err
		}
		f.cff2Limits = limits
		return f, nil
	}
	var varcInk map[int]extents
	out, normalized, err := instanceProgram(data, coords, &varcInk)
	if err != nil {
		return nil, err
	}
	f, err := loadFace(out, normalized)
	if err != nil {
		return nil, err
	}
	f.varcInk = varcInk
	return f, nil
}

// instanceProgram does the rewrite, returning the new font program and the
// location in normalized coordinates, and setting varcInk to the ink of each
// glyph it wrote out from VARC (varcinstance.go).
func instanceProgram(data []byte, want map[string]float64, varcInk *map[int]extents) ([]byte, []float64, error) {
	tables := font.SFNTTables(data)
	if tables == nil {
		return nil, nil, errors.New("fonts: not an sfnt font program (TrueType or OpenType)")
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
	// The vertical metrics, where the font has them: vmtx's advances and top
	// side bearings, and VVAR, whose header begins as HVAR's does.
	vhea, vmtx := tables["vhea"], tables["vmtx"]
	var vAdvances, vBearings []int
	var vvar *hvarTable
	if len(vhea) >= 36 && vmtx != nil {
		if vAdvances, vBearings, err = parseHmtx(vmtx, font.Be16(vhea, 34), numGlyphs); err != nil {
			return nil, nil, fmt.Errorf("fonts: vmtx: %w", err)
		}
		if t := tables["VVAR"]; t != nil {
			if vvar, err = parseHVAR(t); err != nil {
				return nil, nil, fmt.Errorf("fonts: VVAR: %w", err)
			}
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
	// The top phantom point of each glyph, and its vertical advance, at the
	// location: where it is hung and how far it moves the pen down.
	var tops []int
	var newVAdvances []int
	if vAdvances != nil {
		tops = make([]int, numGlyphs)
		newVAdvances = make([]int, numGlyphs)
	}
	// Each glyph's four phantom points once gvar has moved them, and the
	// component whose metrics a composite takes, or -1: the advances and the
	// side bearings are read from these once every glyph has them.
	phantoms := make([][4][2]float64, numGlyphs)
	useMetrics := make([]int, numGlyphs)
	// Every glyph at the location, kept until each has been moved: a component
	// placed by matching points is placed by the points of other glyphs as
	// they are there (pointmatch.go).
	varied := make([]*varGlyph, numGlyphs)
	// The em, which a font stating one outside the range the format allows
	// is refused for, as loadFace refuses it (and would refuse the instance).
	upem, err := headUnitsPerEm(head)
	if err != nil {
		return nil, nil, err
	}
	for gid := 0; gid < numGlyphs; gid++ {
		start, end := offsets[gid], offsets[gid+1]
		if start > end || int(end) > len(glyf) {
			return nil, nil, fmt.Errorf("fonts: glyph %d lies outside the glyf table", gid)
		}
		g, err := decodeVarGlyph(glyf[start:end], numGlyphs)
		if err != nil {
			return nil, nil, fmt.Errorf("fonts: glyph %d: %w", gid, err)
		}
		xMin, yMax := 0, 0
		if end-start >= 10 {
			xMin = int(int16(uint16(font.Be16(glyf[start:end], 2))))
			yMax = int(int16(uint16(font.Be16(glyf[start:end], 8))))
		}
		g.setPhantoms(xMin, bearings[gid], advances[gid])
		if vAdvances != nil {
			g.setVerticalPhantoms(yMax+vBearings[gid], vAdvances[gid])
		} else {
			// Where the face states no vertical metrics HarfBuzz hangs a glyph
			// from the top of its box, and gives it an em. Nothing reads
			// these but a match naming one.
			g.setVerticalPhantoms(yMax, upem)
		}
		if gvar != nil {
			if err := gvar.applyGlyph(gid, g, coords, &budget); err != nil {
				return nil, nil, err
			}
		}
		phantoms[gid] = g.phantoms()
		useMetrics[gid] = g.metricsComponent()
		varied[gid] = g
	}
	// The VARC glyphs written out as glyf, before anything reads the glyphs
	// as a whole: they are glyf glyphs of the instance.
	flat, err := flattenVARC(data, tables, fvar, want, varied, &budget)
	if err != nil {
		return nil, nil, err
	}
	if flat != nil {
		*varcInk = flat
	}
	if err := placeMatchedComponents(varied, &budget); err != nil {
		return nil, nil, err
	}
	for gid, g := range varied {
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

	// The metrics, from the phantom points: a composite that takes its metrics
	// from a component takes that component's phantom points, as HarfBuzz
	// takes them, whether or not it has phantom points of its own — but only
	// away from the default. At the default instance HarfBuzz reads no phantom
	// points and a glyph's metrics are the ones hmtx and vmtx store for it,
	// which for such a composite need not be its component's.
	offDefault := false
	for _, c := range coords {
		offDefault = offDefault || c != 0
	}
	for gid := 0; gid < numGlyphs; gid++ {
		ph := phantoms[gid]
		if offDefault {
			ph = metricsPhantoms(phantoms, useMetrics, gid)
		}
		origins[gid] = ph[0][0]
		if _, ok := flat[gid]; ok {
			// A VARC glyph's outline is drawn from its origin, which its
			// side bearing is measured from: see varcinstance.go.
			origins[gid] = 0
		}
		switch {
		case hvar != nil:
			newAdvances[gid] = advances[gid] + otRound(hvar.advanceDelta(gid, coords))
		default:
			newAdvances[gid] = otRound(ph[1][0] - ph[0][0])
		}
		newAdvances[gid] = max(newAdvances[gid], 0)
		if vAdvances != nil {
			// hb_ot_get_glyph_v_advances at the location: VVAR's delta where
			// the font has one, the phantom points' distance where it has gvar
			// instead, and vmtx's advance where it has neither.
			top, bottom := ph[2][1], ph[3][1]
			tops[gid] = otRound(top)
			switch {
			case vvar != nil:
				newVAdvances[gid] = vAdvances[gid] + otRound(vvar.advanceDelta(gid, coords))
			case gvar != nil:
				newVAdvances[gid] = otRound(top - bottom)
			default:
				newVAdvances[gid] = vAdvances[gid]
			}
			newVAdvances[gid] = max(newVAdvances[gid], 0)
		}
	}

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
	if vAdvances != nil {
		out["vmtx"], out["vhea"] = buildVerticalMetrics(vhea, newVAdvances, tops, bounds)
	}
	out["head"] = instanceHead(head, bounds)
	if len(flat) > 0 && len(maxp) >= 32 {
		// The VARC glyphs written out are glyf glyphs maxp's counts must cover.
		var written [][]byte
		for gid := range flat {
			written = append(written, newGlyf[newLoca[gid]:newLoca[gid+1]])
		}
		out["maxp"] = raiseMaxp(append([]byte(nil), maxp...), written)
	}
	instanceDesign(out, axes, want)
	if err := applyMVAR(out, tables["MVAR"], coords); err != nil {
		return nil, nil, err
	}
	name, err := instanceName(tables["name"], fvar, axes, want)
	if err != nil {
		return nil, nil, err
	}
	if name != nil {
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
//
// Every coordinate it returns is a whole number of 2.14 units, as HarfBuzz
// holds one (f2Dot14Location): it is the one reading of a location for every
// table of the face — gvar, HVAR, VVAR, MVAR, GPOS's devices, COLR, a CFF2
// charstring's blends and VARC.
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
	for i, c := range f2Dot14Location(axes, segments, want) {
		coords[i] = float64(c) / 16384
	}
	return coords, nil
}

// f2Dot14Location is hb_ot_var_normalize_coords: each axis's user coordinate
// — its own default where the location does not name it — normalized in
// single precision and rounded to 16.16, mapped by avar's segment maps and
// rounded to 16.16 again, and then rounded to the 2.14 every variation table
// states its regions in; every rounding takes a half up, towards +infinity.
// The result is in 2.14 units.
//
// # Why quantized, and why in exactly this order
//
// A location between two 2.14 values is not one any table of the font is
// written against, and it is not one HarfBuzz ever reaches. Noto Sans asked
// for weight 850 normalizes to 0.894995, which HarfBuzz holds to 14664/16384;
// read unrounded, the face advanced 75 glyphs and drew the outlines of 704
// simple glyphs a unit away from where HarfBuzz draws them.
//
// The order of the roundings matters as well as their precision. Weight 700
// of the same face is drawn by HarfBuzz at 9995/16384. fontTools' instancer
// maps the unrounded coordinate through avar in double precision and rounds
// once, and lands at 9994: HarfBuzz's first rounding to 16.16 moves the
// coordinate before avar sees it, and its last rounds a half of a 2.14 unit
// up. What a reader draws and what this package shapes with is HarfBuzz's,
// so the location is HarfBuzz's to the bit, and not a rounding error from it.
//
// segments is parseAvar's: nil where the font has no avar, and otherwise one
// map for each axis.
func f2Dot14Location(axes []varAxis, segments [][]avarSegment, want map[string]float64) []int {
	coords := make([]int, len(axes))
	for i, a := range axes {
		// fvar's 16.16 read into a float, as F16DOT16::to_float reads it.
		// AxisRecord::get_coordinates also widens an axis's range to take in
		// its default, which never applies here: parseFvar refuses such an
		// axis, and every location is normalized for a font it accepted.
		lo, def, hi := float32(a.min), float32(a.def), float32(a.max)
		v := def
		if w, ok := want[a.tag]; ok {
			v = float32(w)
		}
		v = min(max(v, lo), hi)
		var n float32
		switch {
		case v == def:
		case v < def:
			n = float32(float32(v-def) / float32(def-lo))
		default:
			n = float32(float32(v-def) / float32(hi-def))
		}
		coords[i] = roundf(float32(n * 65536))
	}
	for i := range segments {
		mapped := avarMapFloat(segments[i], float32(float32(coords[i])/65536))
		coords[i] = roundf(float32(mapped * 65536))
	}
	for i := range coords {
		// 16.16 to 2.14. The shift is arithmetic, so a negative half rounds
		// up too, as it does in HarfBuzz.
		coords[i] = (coords[i] + 2) >> 2
	}
	return coords
}

// roundf is roundf as HarfBuzz defines it for itself (hb-algs.hh), which is
// not C's: floorf(v + .5f), so that a half rounds towards +infinity and not
// away from zero — and the addition is a float's, so a value a hair under a
// half can round up. -2.5 is -2 here and -3 in C. A float32 widens to a
// float64 exactly, so the floor of the widened sum is the floor of the float.
func roundf(v float32) int {
	return int(math.Floor(float64(float32(v + 0.5))))
}

func axisTags(axes []varAxis) string {
	tags := make([]string, len(axes))
	for i, a := range axes {
		tags[i] = strconv.Quote(a.tag)
	}
	return strings.Join(tags, ", ")
}

// avarSegment is one point of an axis's mapping: a normalized coordinate, and
// the normalized coordinate it stands for, each an F2DOT14 read into a float
// as HarfBuzz reads it (exactly: fourteen fractional bits fit a float).
type avarSegment struct{ from, to float32 }

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
		out[i] = readSegmentMap(t, at, n)
		at += 4 * n
	}
	return out, nil
}

// readSegmentMap reads n AxisValueMaps from t at off, which the caller has
// checked lie inside it.
func readSegmentMap(t []byte, off, n int) []avarSegment {
	seg := make([]avarSegment, n)
	for j := range seg {
		seg[j] = avarSegment{from: f2dot14f(t, off+4*j), to: f2dot14f(t, off+4*j+2)}
	}
	return seg
}

// f2dot14f is F2DOT14::to_float.
func f2dot14f(b []byte, at int) float32 {
	return float32(float32(signed16(font.Be16(b, at))) * float32(1.0/16384))
}

// avarMapFloat applies one axis's segment map as SegmentMaps::map_float does,
// in single precision: an exact match takes its value, a coordinate between
// two takes the line between them, and one outside the map keeps its distance
// from the nearest end. The cases the specification leaves open — fewer than
// two maps, several maps from one coordinate, a redundant -1 or +1 at an end —
// are answered as HarfBuzz answers them, which is as CoreText does.
func avarMapFloat(m []avarSegment, value float32) float32 {
	if len(m) < 2 {
		if len(m) == 0 {
			return value
		}
		return float32(float32(value-m[0].from) + m[0].to)
	}
	start, end := 0, len(m)
	if m[start].from == -1 && m[start].to == -1 && m[start+1].from == -1 {
		start++
	}
	if m[end-1].from == 1 && m[end-1].to == 1 && m[end-2].from == 1 {
		end--
	}
	i := start
	for ; i < end; i++ {
		if value == m[i].from {
			break
		}
	}
	if i < end {
		j := i
		for ; j+1 < end; j++ {
			if value != m[j+1].from {
				break
			}
		}
		switch {
		case i == j:
			return m[i].to
		case i+2 == j:
			return m[i+1].to
		case value < 0:
			return m[j].to
		case value > 0:
			return m[i].to
		}
		if float32(math.Abs(float64(m[i].to))) < float32(math.Abs(float64(m[j].to))) {
			return m[i].to
		}
		return m[j].to
	}
	for i = start; i < end; i++ {
		if value < m[i].from {
			break
		}
	}
	if i == start {
		return float32(float32(value-m[start].from) + m[start].to)
	}
	if i == end {
		return float32(float32(value-m[end-1].from) + m[end-1].to)
	}
	before, after := m[i-1], m[i]
	denom := float32(after.from - before.from)
	return float32(before.to + float32(float32(float32(after.to-before.to)*float32(value-before.from))/denom))
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
	matches := &matchIndex{glyf: glyf, loca: loca, numGlyphs: numGlyphs, known: make([]int8, numGlyphs)}
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
		matched, err := matches.has(gid, 0)
		if err != nil {
			return nil, fmt.Errorf("fonts: glyph %d: %w", gid, err)
		}
		if matched {
			// A component placed by matching points is placed by points,
			// which a transform composed down the tree does not have.
			box, err = matchedBounds(glyf, loca, numGlyphs, gid, budget)
		} else {
			err = accumulateBounds(glyf, loca, numGlyphs, gid, identityTransform, &box, 0, budget)
		}
		if err != nil {
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
		// A component whose flags ask for its offset to be scaled has the
		// offset put through its own two-by-two, as HarfBuzz and FreeType put
		// it; the default, and a component that asks for it explicitly, is
		// the offset as written. Reading every offset unscaled measured such
		// a composite's box off by the difference, and wrote that box into
		// the instance.
		if c.flags&(compScaledOffset|compUnscaledOffset) == compScaledOffset {
			inner.dx, inner.dy = float64(c.scale[0]*g.x[i])+float64(c.scale[2]*g.y[i]),
				float64(c.scale[1]*g.x[i])+float64(c.scale[3]*g.y[i])
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

// buildVerticalMetrics writes vmtx and the fields of vhea that describe it:
// each glyph's vertical advance, and the top side bearing that hangs it from
// its top phantom point at the location, measured from the top of its
// instanced box — which is how a reader finds the point again (see
// verticalTables.glyfTopPhantom). A glyph with no outline has no box, and its
// bearing is the point itself.
func buildVerticalMetrics(vhea []byte, advances, tops []int, bounds []glyphBounds) (vmtx, newVhea []byte) {
	n := len(advances)
	tsb := make([]int, n)
	for gid := range advances {
		if bounds[gid].empty {
			tsb[gid] = tops[gid]
			continue
		}
		tsb[gid] = tops[gid] - bounds[gid].yMax
	}
	metrics := n
	for metrics > 1 && advances[metrics-1] == advances[metrics-2] {
		metrics--
	}
	vmtx = make([]byte, 4*metrics+2*(n-metrics))
	for gid := 0; gid < n; gid++ {
		if gid < metrics {
			binary.BigEndian.PutUint16(vmtx[4*gid:], uint16(clampU16(advances[gid])))
			binary.BigEndian.PutUint16(vmtx[4*gid+2:], uint16(int16(clampI16(tsb[gid]))))
			continue
		}
		binary.BigEndian.PutUint16(vmtx[4*metrics+2*(gid-metrics):], uint16(int16(clampI16(tsb[gid]))))
	}
	newVhea = append([]byte(nil), vhea...)
	maxAdv, minTSB, minBSB, maxExtent := 0, math.MaxInt32, math.MaxInt32, math.MinInt32
	for gid := 0; gid < n; gid++ {
		maxAdv = max(maxAdv, advances[gid])
		if bounds[gid].empty {
			continue
		}
		height := bounds[gid].yMax - bounds[gid].yMin
		minTSB = min(minTSB, tsb[gid])
		minBSB = min(minBSB, advances[gid]-tsb[gid]-height)
		maxExtent = max(maxExtent, tsb[gid]+height)
	}
	if minTSB == math.MaxInt32 {
		minTSB, minBSB, maxExtent = 0, 0, 0
	}
	binary.BigEndian.PutUint16(newVhea[10:], uint16(clampU16(maxAdv)))
	binary.BigEndian.PutUint16(newVhea[12:], uint16(int16(clampI16(minTSB))))
	binary.BigEndian.PutUint16(newVhea[14:], uint16(int16(clampI16(minBSB))))
	binary.BigEndian.PutUint16(newVhea[16:], uint16(int16(clampI16(maxExtent))))
	binary.BigEndian.PutUint16(newVhea[34:], uint16(metrics))
	return vmtx, newVhea
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
// It reports an error where the name has to change and the table cannot be
// rebuilt, which is not the same as there being nothing to change. Returning nil
// for both was the collision this function exists to prevent, arrived at by the
// other road: a name table the rewriter cannot read left the instance carrying
// the *default* instance's name, and the caller could not tell that from "this
// is already the right name".
func instanceName(name, fvar []byte, axes []varAxis, want map[string]float64) ([]byte, error) {
	if len(name) == 0 {
		// No name to collide with. A font that names nothing gives its
		// instances nothing to be confused about.
		return nil, nil
	}
	if named := namedInstanceName(fvar, name, axes, want); named != "" {
		if named == postScriptName(name) {
			return nil, nil
		}
		return rewrittenName(name, named)
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
		return nil, nil // the default instance, which is the name the font already has
	}
	base := postScriptName(name)
	if base == "" {
		base = "Instance"
	}
	return rewrittenName(name, base+suffix)
}

// rewrittenName is replacePostScriptName with the failure said out loud.
//
// A table this cannot rebuild is a table whose records or storage do not lie
// where it says they do. There is no answer to give: the instance must not go
// out under the default instance's name, because two weights of one face in one
// document under one /BaseFont is a document nothing downstream can take apart.
func rewrittenName(name []byte, to string) ([]byte, error) {
	out := replacePostScriptName(name, sanitizeName(to))
	if out == nil {
		return nil, fmt.Errorf("fonts: this font's name table cannot be rebuilt, "+
			"so the instance cannot be named %q and must not go out under the "+
			"default instance's name", sanitizeName(to))
	}
	return out, nil
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
	// Each distinct string is written once, and every record or language tag
	// stating it points at the one copy, as the fonts that share strings do.
	// Written once per record, a table whose records all name one long string
	// was built as that string times the records — sixty-four megabytes from a
	// thousand records on one — before the check below refused it, and a table
	// that shared strings honestly was refused for storage it did not need.
	// Storage past what the sixteen-bit offsets reach is refused as it is
	// reached rather than after it is built.
	var strs []byte
	strAt := map[string]int{}
	place := func(v []byte) (int, bool) {
		if off, ok := strAt[string(v)]; ok {
			return off, true
		}
		if len(strs)+len(v) > 0xFFFF {
			return 0, false
		}
		strAt[string(v)] = len(strs)
		strs = append(strs, v...)
		return strAt[string(v)], true
	}
	for i := range records {
		if _, ok := place(records[i].value); !ok {
			return nil
		}
	}
	for i, r := range records {
		rec := 6 + 12*i
		binary.BigEndian.PutUint16(out[rec:], uint16(r.platform))
		binary.BigEndian.PutUint16(out[rec+2:], uint16(r.encoding))
		binary.BigEndian.PutUint16(out[rec+4:], uint16(r.language))
		binary.BigEndian.PutUint16(out[rec+6:], uint16(r.id))
		binary.BigEndian.PutUint16(out[rec+8:], uint16(len(r.value)))
		binary.BigEndian.PutUint16(out[rec+10:], uint16(strAt[string(r.value)]))
	}
	for i := 0; i < langCount; i++ {
		at := 6 + 12*count + 2 + 4*i
		length := font.Be16(name, at)
		off := storage + font.Be16(name, at+2)
		if off+length > len(name) {
			return nil
		}
		tagAt, ok := place(name[off : off+length])
		if !ok {
			return nil
		}
		binary.BigEndian.PutUint16(out[at:], uint16(length))
		binary.BigEndian.PutUint16(out[at+2:], uint16(tagAt))
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

// metricsPhantoms are the phantom points a glyph's metrics are read from: its
// own, or for a composite that takes its metrics from a component, that
// component's — followed as far as components that take theirs from another,
// and no further than maxComponentDepth, which a chain of components naming
// each other in a circle reaches.
func metricsPhantoms(phantoms [][4][2]float64, useMetrics []int, gid int) [4][2]float64 {
	for depth := 0; depth < maxComponentDepth && useMetrics[gid] >= 0; depth++ {
		gid = useMetrics[gid]
	}
	return phantoms[gid]
}
