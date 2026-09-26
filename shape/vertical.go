package shape

import "github.com/mgilbir/forme/font"

// Vertical metrics: where a glyph set upright in a vertical line stands, and
// how far it moves the pen.
//
// A run set sideways needs none of this. Its glyphs are the horizontal ones
// turned a quarter, and the advance down the page is the advance across it. A
// run set *upright* — CSS Writing Modes' "text-orientation: upright", every
// ideograph of a vertical Japanese line — is a different arrangement of the
// same glyphs: each stands as it does in the font and is hung from a point
// above it, its vertical origin, and the pen moves down from one origin to the
// next by the glyph's vertical advance. Neither number can be had from the
// horizontal metrics. A backend given only those can centre each glyph in an
// em box and step an em at a time, and that is wrong wherever the font says
// otherwise: a CJK face hangs its ideographs from the top of its ideographic
// em box, not from its centre, and a proportional vertical face does not
// advance by an em.
//
// So a run shaped with Features.Vertical comes back with both, per glyph —
// Glyph.YAdvance and Glyph.VOriginX and VOriginY — from what the font states,
// and from what HarfBuzz falls back to where it states nothing. Each answer is
// hb-ot-font.cc's, at the release the oracle is pinned to, because a vertical
// face is tested against HarfBuzz as a horizontal one is.
//
// # The advance
//
// 'vmtx' states it, per glyph, when 'vhea' says how many of its records carry
// one. A face with neither is given the height of its line — ascender less
// descender, as the horizontal metrics state them — for every glyph, which is
// HarfBuzz's synthesis and the only one a face with no vertical metrics
// admits.
//
// # The origin
//
// Across the line the origin is half the glyph's horizontal advance, so that
// the glyph is centred on the line's middle whatever the font. Along it, the
// first of these the face can answer:
//
//   - 'VORG', which states it outright for a CFF face: a default, and the
//     glyphs that differ from it.
//   - For a TrueType face with 'vmtx', the top of the glyph's box plus its top
//     side bearing — the glyph's top phantom point, which is where a TrueType
//     rasterizer puts the origin. A composite takes it from the component that
//     says USE_MY_METRICS, as it does its horizontal metrics.
//   - Otherwise the glyph's ink centred in the line's height: half of what the
//     line has to spare above the ink.
//   - And where the ink cannot be had, the ascender.
//
// # What is not here
//
// The ink of a CFF glyph is in its charstrings, which this package does not
// interpret for their bounds (see fallback.go, which has the same gap). So a
// CFF face with no VORG takes the last of the four rather than the third, and
// HarfBuzz — which does interpret them — centres the ink. A CFF face set in
// vertical text states VORG almost without exception; the Noto CJK faces all
// do. A face from LoadInstance keeps its default instance's vertical metrics:
// VVAR and the phantom points gvar moves are not read (see instance.go).

// verticalTables is what a face keeps of the tables its vertical metrics are
// read from, each checked once at load so that a glyph's metrics are a few
// indexed reads.
type verticalTables struct {
	// vmtx is the table, and longMetrics, bearings and advances are how much
	// of it HarfBuzz reads as each kind of record: the first longMetrics
	// records carry an advance and a top side bearing, the records up to
	// bearings a side bearing alone, and past those, up to advances, an
	// advance alone. longMetrics is zero for a face with no vertical metrics.
	vmtx                            []byte
	longMetrics, bearings, advances int
	// vorg is the VORG table where its header and its records are all there,
	// and nil otherwise; vorgCount is how many records it has and vorgDefault
	// the origin of every glyph they do not name.
	vorg        []byte
	vorgCount   int
	vorgDefault int
	// glyf and loca are the outlines, kept only for a TrueType face with
	// vmtx: the top of a glyph's box is read from its header, and a
	// composite's from the component whose metrics it uses. numGlyphs is the
	// count maxp states, which is how far loca is read.
	glyf, loca []byte
	longLoca   bool
	numGlyphs  int
	// composites is where each composite that takes its metrics from a
	// component is hung, walked once at load. See resolveComposites.
	composites map[int]compositeTop
}

// readVerticalTables takes what a face's vertical metrics need out of its
// tables. Every count is clamped to what the table actually holds, the way
// HarfBuzz's accelerators clamp them, so a table that claims more records than
// it has is read as the records it has.
func readVerticalTables(tables map[string][]byte, numGlyphs int, budget *font.Budget) verticalTables {
	var v verticalTables
	vmtx := tables["vmtx"]
	if vhea := tables["vhea"]; len(vhea) >= 36 && font.Be16(vhea, 0) == 1 {
		n := font.Be16(vhea, 34)
		left := len(vmtx)
		if n*4 > left {
			n = left / 4
		}
		left -= n * 4
		bearings := max(numGlyphs, n)
		if (bearings-n)*2 > left {
			bearings = n + left/2
		}
		left -= (bearings - n) * 2
		if n > 0 {
			v.vmtx = vmtx
			v.longMetrics, v.bearings = n, bearings
			v.advances = bearings + left/2
		}
	}
	if vorg := tables["VORG"]; len(vorg) >= 8 && font.Be16(vorg, 0) == 1 {
		n := font.Be16(vorg, 6)
		if 8+4*n <= len(vorg) {
			v.vorg, v.vorgCount = vorg, n
			v.vorgDefault = signed16(font.Be16(vorg, 4))
		}
	}
	if v.longMetrics > 0 {
		head := tables["head"]
		glyf, loca := tables["glyf"], tables["loca"]
		if len(glyf) > 0 && len(loca) > 0 && len(head) >= 54 {
			v.glyf, v.loca = glyf, loca
			v.longLoca = signed16(font.Be16(head, 50)) != 0
			v.numGlyphs = numGlyphs
		}
	}
	// A face with VORG takes no origin from its glyphs, so its composites are
	// never walked.
	if v.glyf != nil && v.vorg == nil {
		v.resolveComposites(budget)
	}
	return v
}

// vAdvance is a glyph's advance as vmtx states it, in font units: HarfBuzz's
// get_advance_without_var_unscaled.
func (v *verticalTables) vAdvance(gid int) int {
	switch {
	case gid < 0:
		return 0
	case gid < v.bearings:
		return font.Be16(v.vmtx, 4*min(gid, v.longMetrics-1))
	case v.advances == v.bearings:
		// Past the last record there is only the last advance.
		return v.vAdvance(v.bearings - 1)
	}
	// A table that runs on past its side bearings lists advances for the
	// glyphs beyond them, as a font with more than 65,535 glyphs does.
	at := 4*v.longMetrics + 2*(v.bearings-v.longMetrics) +
		2*min(gid-v.bearings, v.advances-v.bearings-1)
	return font.Be16(v.vmtx, at)
}

// topSideBearing is a glyph's top side bearing as vmtx states it, in font
// units, and zero for a glyph past the last record that carries one.
func (v *verticalTables) topSideBearing(gid int) int {
	switch {
	case gid < 0 || gid >= v.bearings:
		return 0
	case gid < v.longMetrics:
		return signed16(font.Be16(v.vmtx, 4*gid+2))
	}
	return signed16(font.Be16(v.vmtx, 4*v.longMetrics+2*(gid-v.longMetrics)))
}

// vorgOrigin is a glyph's vertical origin as VORG states it: its own record,
// found by the binary search the format's order allows, or the default.
func (v *verticalTables) vorgOrigin(gid int) int {
	lo, hi := 0, v.vorgCount-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		switch g := font.Be16(v.vorg, 8+4*mid); {
		case gid < g:
			hi = mid - 1
		case gid > g:
			lo = mid + 1
		default:
			return signed16(font.Be16(v.vorg, 8+4*mid+2))
		}
	}
	return v.vorgDefault
}

// glyfBytes is a glyph's bytes in glyf, and false where loca does not give it
// a place there — which HarfBuzz reads as a glyph with no header at all.
func (v *verticalTables) glyfBytes(gid int) ([]byte, bool) {
	if gid < 0 || gid >= v.numGlyphs {
		return nil, false
	}
	var start, end int
	if v.longLoca {
		if 4*gid+8 > len(v.loca) {
			return nil, false
		}
		start, end = int(font.Be32(v.loca, 4*gid)), int(font.Be32(v.loca, 4*gid+4))
	} else {
		if 2*gid+4 > len(v.loca) {
			return nil, false
		}
		start, end = 2*font.Be16(v.loca, 2*gid), 2*font.Be16(v.loca, 2*gid+2)
	}
	if start > end || end > len(v.glyf) {
		return nil, false
	}
	return v.glyf[start:end], true
}

// The two composite glyph flags the walk below reads beside the ones
// glyfpoints.go names: the one it is for, and the one that widens a
// component's glyph index to 24 bits, which changes how long its record is.
const (
	compUseMyMetrics = 0x0200
	compGID24        = 0x2000
)

// maxPhantomEdges bounds how many glyphs the walk for one glyph's origin may
// visit, and maxPhantomDepth how deeply it may nest: HarfBuzz's
// HB_MAX_GRAPH_EDGE_COUNT and HB_MAX_NESTING_LEVEL, and what it gives up at is
// what this gives up at, so that a font asking for more gets the answer
// HarfBuzz gives it rather than a different one.
const (
	maxPhantomEdges = 16384
	maxPhantomDepth = 64
)

// glyfTopPhantom is the top phantom point of a TrueType glyph: the top of its
// box plus its top side bearing, or for a composite, the top phantom point of
// the last component flagged USE_MY_METRICS. It is HarfBuzz's
// get_v_origin_with_var_unscaled at the default instance, which walks the
// glyph for its phantom points alone.
//
// A composite that takes its point from a component was walked at load (see
// resolveComposites), so what is left here is one header and one vmtx record.
//
// ok is false where the walk fails — a malformed simple glyph, a composite
// nested past the bound — and HarfBuzz then answers the em, which the caller
// does too.
func (v *verticalTables) glyfTopPhantom(gid int) (top int, ok bool) {
	if gid >= v.numGlyphs {
		// HarfBuzz answers zero here, before it has walked anything.
		return 0, true
	}
	if t, walked := v.composites[gid]; walked {
		return t.top, t.ok
	}
	top, ok, _ = v.topPhantomAt(gid, 0, nil)
	return top, ok
}

// compositeTop is a composite glyph's top phantom point as the walk at load
// found it, and whether the walk succeeded.
type compositeTop struct {
	top int
	ok  bool
}

// resolveComposites walks, once, every composite glyph that takes its metrics
// from a component, and records where each is hung: the walk is the one part
// of a glyph's vertical origin that is not a fixed number of reads, and a
// font decides how long it is.
//
// It is done at load, and charged to the font's budget with everything else
// Load reads, rather than per glyph each time a run is shaped: a font can make
// one composite's walk as long as HarfBuzz's bounds allow — sixteen thousand
// glyphs — and then name that composite in every position of every run. Here
// a font that costs more than the budget to walk is refused at load, as one
// whose character map or charstrings cost too much is, and one that costs
// less is walked once. A glyph visited and a component record read are each a
// unit.
//
// Only a composite that names a component USE_MY_METRICS is walked; every
// other glyph is hung from its own header, which is read when it is asked for.
func (v *verticalTables) resolveComposites(budget *font.Budget) {
	for gid := 0; gid < v.numGlyphs; gid++ {
		g, placed := v.glyfBytes(gid)
		if !placed || len(g) < 10 || signed16(font.Be16(g, 0)) >= 0 || !usesComponentMetrics(g) {
			continue
		}
		w := phantomWalk{budget: budget, tortoise: -1}
		top, ok, spent := v.topPhantomAt(gid, 0, &w)
		if spent {
			return
		}
		if v.composites == nil {
			v.composites = map[int]compositeTop{}
		}
		v.composites[gid] = compositeTop{top: top, ok: ok}
	}
}

// usesComponentMetrics reports whether a composite glyph flags any of its
// components USE_MY_METRICS. It reads the records the way topPhantomAt does,
// and stops where they stop.
func usesComponentMetrics(g []byte) bool {
	found := false
	eachComponent(g, func(flags, _ int) bool {
		found = flags&compUseMyMetrics != 0
		return !found
	})
	return found
}

// eachComponent calls fn with each component record of a composite glyph —
// its flags and its glyph index — in order, until fn returns false or the
// records end: at one without MORE_COMPONENTS, or at one the glyph's bytes do
// not hold whole. It is HarfBuzz's composite iterator, which stops at the same
// places.
func eachComponent(g []byte, fn func(flags, gid int) bool) {
	for at := 10; at+4 <= len(g); {
		flags := font.Be16(g, at)
		size := 6
		if flags&compArgsAreWords != 0 {
			size = 8
		}
		if flags&compGID24 != 0 {
			size++
		}
		switch {
		case flags&compHaveScale != 0:
			size += 2
		case flags&compHaveXYScale != 0:
			size += 4
		case flags&compHave2x2 != 0:
			size += 8
		}
		if at+size > len(g) {
			return
		}
		comp := font.Be16(g, at+2)
		if flags&compGID24 != 0 {
			comp = comp<<8 | int(g[at+4])
		}
		if !fn(flags, comp) || flags&compMoreComponents == 0 {
			return
		}
		at += size
	}
}

// phantomWalk is what one walk for a glyph's top phantom point keeps as it
// goes: how many glyphs it has visited, the font's budget it is charged to,
// and HarfBuzz's decycler over the composites it is inside.
//
// The decycler is HarfBuzz's own, and not a set of the glyphs on the path,
// because the two answer differently for a font whose components name each
// other and the answer is compared unit for unit. HarfBuzz keeps one node per
// composite it is inside, each holding the component it is visiting, and
// checks a component only against the node halfway up the stack — a
// tortoise that moves down one node for every two a walk goes in — which
// finds every cycle, a little later than a set would.
type phantomWalk struct {
	edges  int
	budget *font.Budget
	// visiting is each node's component, from the outermost composite in;
	// tortoise is the node a component is checked against, and awake
	// whether it moves on the next node in or out.
	visiting []int
	tortoise int
	awake    bool
}

// enter and leave are a node's construction and destruction in HarfBuzz's
// hb_decycler_node_t.
func (w *phantomWalk) enter() {
	w.awake = !w.awake
	if len(w.visiting) == 0 {
		w.tortoise = 0
	} else if w.awake {
		w.tortoise++
	}
	w.visiting = append(w.visiting, -1)
}

func (w *phantomWalk) leave() {
	w.visiting = w.visiting[:len(w.visiting)-1]
	if w.awake {
		w.tortoise--
	}
	w.awake = !w.awake
}

// visit records that the innermost node is visiting a component, and reports
// whether it may: false where the tortoise is visiting the same one, which is
// a cycle.
func (w *phantomWalk) visit(gid int) bool {
	me := len(w.visiting) - 1
	w.visiting[me] = gid
	return w.tortoise == me || w.visiting[w.tortoise] != gid
}

// topPhantomAt is one level of glyfTopPhantom's walk, which w — nil for a
// glyph read at lookup rather than walked at load — bounds and charges.
// spent reports that the font's budget ran out, and the walk stops where it
// did.
func (v *verticalTables) topPhantomAt(gid, depth int, w *phantomWalk) (top int, ok, spent bool) {
	if w != nil {
		if depth > maxPhantomDepth || w.edges > maxPhantomEdges {
			return 0, false, false
		}
		w.edges++
		if !w.budget.Charge(1, "glyf composites for vertical origins") {
			return 0, false, true
		}
	}
	g, placed := v.glyfBytes(gid)
	if !placed {
		// No bytes where loca points, or none it can point at: HarfBuzz reads
		// a glyph with no header and no index, so no side bearing either.
		return 0, true, false
	}
	if len(g) < 10 {
		// Too short for a header: an empty glyph, whose box is all zeroes.
		return v.topSideBearing(gid), true, false
	}
	contours := signed16(font.Be16(g, 0))
	top = signed16(font.Be16(g, 8)) + v.topSideBearing(gid)
	switch {
	case contours > 0:
		// The one part of a simple glyph read here is the check HarfBuzz
		// makes before it would read the points: that the end-point list and
		// the instruction length after it are there, and that the last end
		// point leaves a point for each contour.
		if 10+2*(contours+1) > len(g) {
			return 0, false, false
		}
		if font.Be16(g, 10+2*(contours-1))+1 < contours {
			return 0, false, false
		}
		return top, true, false
	case contours == 0:
		return top, true, false
	case w == nil:
		// A composite reached at lookup rather than at load takes its point
		// from no component — resolveComposites walked every one that does
		// — so it is its own.
		return top, true, false
	}
	w.enter()
	defer w.leave()
	ok = true
	eachComponent(g, func(flags, comp int) bool {
		if !w.budget.Charge(1, "glyf composites for vertical origins") {
			spent = true
			return false
		}
		if !w.visit(comp) || flags&compUseMyMetrics == 0 {
			return true
		}
		t, tok, tspent := v.topPhantomAt(comp, depth+1, w)
		switch {
		case tspent:
			spent = true
			return false
		case !tok:
			ok = false
			return false
		}
		top = t
		return true
	})
	if spent || !ok {
		return 0, false, spent
	}
	return top, true, false
}

// fontExtentsUnits is the face's ascender and descender as HarfBuzz reads them
// for its font extents, in font units: OS/2's typographic pair where the font
// asks for it to be used, hhea's otherwise, and four fifths of an em and the
// rest of it below for a face with neither. The ascender is never below the
// baseline and the descender never above it, whatever sign the font wrote.
func (f *Face) fontExtentsUnits() (ascender, descender int) {
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	switch {
	case f.useTypoMetrics && f.declared&MetricTypoMetrics != 0:
		return abs(f.typoAscent), -abs(f.typoDescent)
	case f.declared&MetricLineGap != 0 || f.std != nil:
		// hhea's, which is where a face's ascent and descent come from — and
		// for a standard face, the AFM's, which stand in for them.
		return abs(f.ascent), -abs(f.descent)
	}
	ascender = int(float64(f.unitsPerEm) * .8)
	return ascender, ascender - f.unitsPerEm
}

// verticalUnits is a glyph's vertical advance and vertical origin in font
// units, the origin measured from the glyph's own horizontal origin: what
// HarfBuzz's hb_ot_get_glyph_v_advances and hb_ot_get_glyph_v_origins answer
// for a font at its own em. The advance is how far down the pen moves, and is
// positive. See the top of this file for the order the answers are sought in.
func (f *Face) verticalUnits(gid int) (advance, originX, originY int) {
	v := &f.vert
	ascender, descender := f.fontExtentsUnits()
	if v.longMetrics > 0 {
		advance = v.vAdvance(gid)
	} else {
		advance = ascender - descender
	}
	// Half the horizontal advance, halved as an integer: HarfBuzz divides a
	// position, and a position is a whole number of units at the font's em.
	// A face with no horizontal metrics is given half an em for the advance,
	// as HarfBuzz gives it.
	hAdvance := f.advanceUnits(gid)
	if f.longMetrics <= 0 {
		hAdvance = f.unitsPerEm / 2
	}
	originX = hAdvance / 2
	switch {
	case v.vorg != nil:
		originY = v.vorgOrigin(gid)
	case v.longMetrics > 0 && v.glyf != nil:
		top, ok := v.glyfTopPhantom(gid)
		if !ok {
			top = f.unitsPerEm
		}
		originY = top
	default:
		if ext, ok := f.glyphExtents(gid); ok {
			// The line's height less the ink's, halved, above the ink —
			// floored, as HarfBuzz's shift floors it. The line's height and
			// not the glyph's advance, even where vmtx states one: HarfBuzz
			// centres in the font's height whatever the glyph advances by.
			originY = ext.yBearing + (ascender-descender+ext.height)>>1
		} else {
			originY = ascender
		}
	}
	return advance, originX, originY
}

// verticalRune is verticalUnits for a face set by character code, which has
// no glyph index to ask about the standard faces by: the character's glyph
// for a simple face, and for a standard one the AFM's metrics — the line's
// height for the advance, half the width across, and the character's box
// centred in the line where the AFM gives one.
func (f *Face) verticalRune(r rune) (advance, originX, originY int) {
	if f.std == nil {
		gid, ok := f.prog.Cmap[r]
		if !ok {
			gid = 0
		}
		return f.verticalUnits(gid)
	}
	ascender, descender := f.fontExtentsUnits()
	advance = ascender - descender
	width, _ := f.stdAdvance(r)
	originX = int(width) / 2
	originY = ascender
	if lo, hi, ok := f.glyphInk(r); ok && (lo != 0 || hi != 0) {
		originY = hi + (advance+lo-hi)>>1
	}
	return advance, originX, originY
}

// setVertical gives every glyph of a run set upright its vertical advance and
// origin, and moves it off the pen by the origin, so that positioning works
// on it as HarfBuzz's does: the pen moves down, not across, and a glyph's
// offsets are from where its origin is hung. hb_ot_position_default for a
// top-to-bottom run. Whatever the substitutions left in the advance and the
// offsets is replaced, as HarfBuzz clears them first.
func (f *Face) setVertical(buf []Glyph) {
	for i := range buf {
		advance, x, y := f.verticalUnits(buf[i].GID)
		buf[i].XAdvance = 0
		buf[i].YAdvance = -f.scale(advance)
		buf[i].VOriginX, buf[i].VOriginY = f.scale(x), f.scale(y)
		buf[i].XOffset, buf[i].YOffset = -buf[i].VOriginX, -buf[i].VOriginY
	}
}

// unhangFromOrigin is the last step of shaping a run upright: each glyph's
// offsets were worked out, as HarfBuzz works them out, from a pen standing at
// the glyph's origin, and are given back as offsets from where the glyph is
// hung. That is what Glyph.XOffset and YOffset say in a horizontal run too —
// what the font's positioning moved the glyph by — and what a format that
// places a glyph by its origin itself wants: PDF's vertical writing takes the
// origin from the font's W2 array and the offsets from the text.
func unhangFromOrigin(buf []Glyph) {
	for i := range buf {
		buf[i].XOffset += buf[i].VOriginX
		buf[i].YOffset += buf[i].VOriginY
	}
}

// verticalForm is the vertical presentation form HarfBuzz sets a character in
// when a run is set upright in a face with no 'vert' feature and a glyph for
// the form: the comma, the full stop and the brackets that are drawn
// differently down a line than across it. It is hb_unicode_funcs_t's
// vertical_char_for, and the character itself where there is none.
func verticalForm(r rune) rune {
	switch r {
	case 0x2013:
		return 0xFE32
	case 0x2014:
		return 0xFE31
	case 0x2025:
		return 0xFE30
	case 0x2026:
		return 0xFE19
	case 0x3001:
		return 0xFE11
	case 0x3002:
		return 0xFE12
	case 0x3008:
		return 0xFE3F
	case 0x3009:
		return 0xFE40
	case 0x300A:
		return 0xFE3D
	case 0x300B:
		return 0xFE3E
	case 0x300C:
		return 0xFE41
	case 0x300D:
		return 0xFE42
	case 0x300E:
		return 0xFE43
	case 0x300F:
		return 0xFE44
	case 0x3010:
		return 0xFE3B
	case 0x3011:
		return 0xFE3C
	case 0x3014:
		return 0xFE39
	case 0x3015:
		return 0xFE3A
	case 0x3016:
		return 0xFE17
	case 0x3017:
		return 0xFE18
	case 0xFE4F:
		return 0xFE34
	case 0xFF01:
		return 0xFE15
	case 0xFF08:
		return 0xFE35
	case 0xFF09:
		return 0xFE36
	case 0xFF0C:
		return 0xFE10
	case 0xFF1A:
		return 0xFE13
	case 0xFF1B:
		return 0xFE14
	case 0xFF1F:
		return 0xFE16
	case 0xFF3B:
		return 0xFE47
	case 0xFF3D:
		return 0xFE48
	case 0xFF3F:
		return 0xFE33
	case 0xFF5B:
		return 0xFE37
	case 0xFF5D:
		return 0xFE38
	}
	return r
}

// rotateForVertical puts each character of an upright run that has a vertical
// presentation form into it, where the face has a glyph for the form: what
// HarfBuzz does for a face with no 'vert', whose own vertical forms are
// therefore only reachable by character. runes is changed in place.
func (f *Face) rotateForVertical(runes []rune) {
	for i, r := range runes {
		if v := verticalForm(r); v != r && f.hasGlyph(v) {
			runes[i] = v
		}
	}
}
