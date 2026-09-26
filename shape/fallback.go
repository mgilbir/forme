package shape

import (
	"math"

	"github.com/mgilbir/forme/font"
)

// Placing marks without the font: what a model with a fallback does for a face
// that positions nothing of its own.
//
// A face with no GPOS — or, for the Hebrew model, none under 'hebr' — says
// nothing about where its marks go, and without it every accent is drawn where
// its own outline puts it: for most such fonts, on the origin of the glyph
// after the letter, with its advance cancelled. HarfBuzz places them instead,
// from the glyphs' ink and the marks' combining classes: an accent above is
// centred over its letter and lifted clear of it, one below is hung under it,
// a second above is stacked over the first. It is what every font without mark
// positioning is drawn with in a browser, and a font is tested against it.
//
// Over the Google Fonts tree this was most of what still differed from
// HarfBuzz: the M+ families, Cardo, Lunasima and Libertinus Sans for Hebrew,
// which state no Hebrew positioning at all, and some thirteen hundred Latin,
// Greek and Cyrillic strings in faces with no GPOS table.
//
// This is hb-ot-shape-fallback.cc's mark positioning, number for number: the
// arithmetic is in font units and in integers, rounding where HarfBuzz rounds,
// because the answer is compared unit for unit.
//
// # What is not here
//
// The ink of a glyph is read from the glyph header of a TrueType face. A CFF
// face's ink is in its charstrings, which this package does not interpret for
// their bounds, and a colour or bitmap face's is in tables it does not read for
// it either; for those HarfBuzz would place the marks and this cancels their
// advances and leaves them where they are, which is what HarfBuzz itself does
// for a glyph whose extents it cannot get.

// unicodeMark is what a character says about itself as a mark: whether it is
// one (general category Mn, Mc or Me), whether it takes no room (Mn), and its
// combining class as HarfBuzz orders marks (reorderClass) and then, for a
// non-spacing one, as it places them without the font (fallbackClass). It
// travels on the glyph the character became — see Glyph.umark.
type unicodeMark struct {
	mark, nonSpacing bool
	class            uint8
}

// unicodeMarkOf is what a character says about itself as a mark.
func unicodeMarkOf(r rune) unicodeMark {
	if !isCombiningMark(r) {
		return unicodeMark{}
	}
	m := unicodeMark{mark: true, nonSpacing: isNonSpacingMark(r), class: reorderClass(r)}
	if m.nonSpacing {
		m.class = fallbackClass(r, m.class)
	}
	return m
}

// The combining classes that say where a mark is drawn, Unicode's own numbers.
const (
	cccAttachedBelowLeft  = 200
	cccAttachedBelow      = 202
	cccAttachedAbove      = 214
	cccAttachedAboveRight = 216
	cccBelowLeft          = 218
	cccBelow              = 220
	cccBelowRight         = 222
	cccAboveLeft          = 228
	cccAbove              = 230
	cccAboveRight         = 232
	cccDoubleBelow        = 233
	cccDoubleAbove        = 234
)

// fallbackClass is where a non-spacing mark is drawn, as a combining class,
// for a character whose own class is an order rather than a place: the fixed
// positions Unicode gives the Hebrew points, the Arabic vowels, the Thai, Lao
// and Tibetan signs, which say which mark is which and nothing about where it
// goes. HarfBuzz's recategorize_combining_class, whose cases are the reordered
// classes (reorderClass) and not Unicode's — which is why it is given both.
func fallbackClass(r rune, class uint8) uint8 {
	if class >= 200 {
		return class
	}
	// Thai and Lao need some per-character work.
	if r&^0xFF == 0x0E00 {
		if class == 0 {
			switch r {
			case 0x0E31, 0x0E34, 0x0E35, 0x0E36, 0x0E37, 0x0E47, 0x0E4C, 0x0E4D, 0x0E4E:
				return cccAboveRight
			case 0x0EB1, 0x0EB4, 0x0EB5, 0x0EB6, 0x0EB7, 0x0EBB, 0x0ECC, 0x0ECD:
				return cccAbove
			case 0x0EBC:
				return cccBelow
			}
		} else if r == 0x0E3A {
			// The Thai virama is below and to the right.
			return cccBelowRight
		}
	}
	switch class {
	// Hebrew: sheva, the hatafs, hiriq, tsere, segol, patah, qamats, qubuts
	// and meteg, by their reordered classes.
	case 22, 15, 16, 17, 23, 18, 19, 20, 21, 24, 25:
		return cccBelow
	case 13: // rafe
		return cccAttachedAbove
	case 10: // shin dot
		return cccAboveRight
	case 11, 14: // sin dot, holam
		return cccAboveLeft
	case 26: // point varika
		return cccAbove
	case 12: // dagesh, which stays where the class puts it
		return class
	// Arabic and Syriac: fathatan, dammatan, fatha, damma, shadda, sukun and
	// the superscript alef and alaph above; kasratan and kasra below.
	case 28, 29, 31, 32, 27, 34, 35, 36:
		return cccAbove
	case 30, 33:
		return cccBelow
	// Thai: sara u and sara uu, and the tone marks.
	case 3:
		return cccBelowRight
	case 107:
		return cccAboveRight
	// Lao: sign u and uu, and the tone marks.
	case 118:
		return cccBelow
	case 122:
		return cccAbove
	// Tibetan: the vowel signs aa, i and u.
	case 129:
		return cccBelow
	case 132:
		return cccAbove
	case 131:
		return cccBelow
	}
	return class
}

// extents is a glyph's ink in font units, as HarfBuzz measures it: the left
// side bearing, the top of the ink, its width, and its height — negative,
// since HarfBuzz measures down from the top.
type extents struct {
	xBearing, yBearing, width, height int
}

// glyphExtents is a glyph's ink, where the face can say.
//
// It is the glyph header's box with the left side bearing hmtx states, which
// is how HarfBuzz reads a TrueType glyph at the instance a face was cut at. An
// empty glyph has no ink and says so. A face with no glyf table cannot answer;
// see the note at the top of this file.
func (f *Face) glyphExtents(gid int) (extents, bool) {
	if f.prog == nil || f.prog.GlyphBBox == nil || gid < 0 || gid >= len(f.prog.GlyphBBox) {
		return extents{}, false
	}
	if !f.prog.GlyphNonEmpty[gid] {
		return extents{}, true
	}
	b := f.prog.GlyphBBox[gid]
	lsb := min(b[0], b[2])
	if v, ok := f.leftSideBearing(gid); ok {
		lsb = v
	}
	return extents{
		xBearing: lsb,
		yBearing: max(b[1], b[3]),
		width:    max(b[0], b[2]) - min(b[0], b[2]),
		height:   min(b[1], b[3]) - max(b[1], b[3]),
	}, true
}

// leftSideBearing is a glyph's left side bearing as hmtx states it.
func (f *Face) leftSideBearing(gid int) (int, bool) {
	n := f.longMetrics
	switch {
	case n <= 0:
		return 0, false
	case gid < n:
		at := 4*gid + 2
		if at+2 > len(f.hmtx) {
			return 0, false
		}
		return signed16(font.Be16(f.hmtx, at)), true
	}
	at := 4*n + 2*(gid-n)
	if at+2 > len(f.hmtx) {
		return 0, false
	}
	return signed16(font.Be16(f.hmtx, at)), true
}

// advanceUnits is a glyph's advance as hmtx states it, in font units.
func (f *Face) advanceUnits(gid int) int {
	n := f.longMetrics
	if n <= 0 || gid < 0 {
		return 0
	}
	if gid >= n {
		gid = n - 1
	}
	if 4*gid+2 > len(f.hmtx) {
		return 0
	}
	return font.Be16(f.hmtx, 4*gid)
}

// units is a scaled distance back in the font's units: what f.scale was
// handed, for a distance that came from font units in the first place.
func (f *Face) units(v float64) int {
	return int(math.Round(v * float64(f.unitsPerEm) / 1000))
}

// fallbackMarkPositions places every mark of the run against its base. It is
// _hb_ot_shape_fallback_mark_position: the run is cut into clusters at each
// glyph that is not a mark, and each cluster's marks are placed around its
// first glyph that is not one.
func (sh shaper) fallbackMarkPositions(buf []Glyph, adjust bool) {
	start := 0
	for i := 1; i < len(buf); i++ {
		if !buf[i].umark.mark {
			sh.fallbackCluster(buf, start, i, adjust)
			start = i
		}
	}
	sh.fallbackCluster(buf, start, len(buf), adjust)
}

func (sh shaper) fallbackCluster(buf []Glyph, start, end int, adjust bool) {
	if end-start < 2 {
		return
	}
	for i := start; i < end; i++ {
		if buf[i].umark.mark {
			continue
		}
		j := i + 1
		for j < end && buf[j].umark.mark {
			j++
		}
		sh.fallbackAroundBase(buf, i, j, adjust)
		i = j - 1
	}
}

// fallbackAroundBase places the marks after a base, up to end. HarfBuzz's
// position_around_base.
//
// Along the line a mark is centred over the base's advance rather than its
// ink — which works for a glyph with no ink at all — or set to its left or
// right edge for the classes that say so. Across it, each mark of a class is
// stacked clear of the ink below it: the base's, and then every mark of the
// same class already placed. A ligature's marks are placed over the component
// they belong to, as a share of its advance.
func (sh shaper) fallbackAroundBase(buf []Glyph, base, end int, adjust bool) {
	f := sh.f
	baseExt, ok := f.glyphExtents(buf[base].GID)
	if !ok {
		// Without the base's ink there is nothing to place against: the marks'
		// advances are cancelled and they stay where they are.
		for i := base + 1; i < end; i++ {
			if buf[i].umark.nonSpacing {
				if adjust {
					buf[i].XOffset -= buf[i].XAdvance
					buf[i].YOffset -= buf[i].YAdvance
				}
				buf[i].XAdvance, buf[i].YAdvance = 0, 0
			}
		}
		return
	}
	baseExt.yBearing += f.units(buf[base].YOffset)
	baseExt.xBearing = 0
	baseExt.width = f.advanceUnits(buf[base].GID)

	// A ligature's parts, as HarfBuzz counts them: only for a glyph whose
	// class is ligature, which for a font with GDEF is what GDEF says of it.
	comps := buf[base].lig.comps
	if sh.l.classOf(buf[base]) != classLigature {
		comps = 1
	}
	ligID := buf[base].lig.id
	// Where the pen is, relative to the base's origin, as each mark is
	// reached: before the base's advance when the run is drawn forwards, and
	// moving back by the advance of anything between. Down the page too, for
	// a run set upright, whose advances are vertical ones.
	var xOff, yOff float64
	if !sh.rtl {
		xOff, yOff = -buf[base].XAdvance, -buf[base].YAdvance
	}
	component := baseExt
	lastComp := -1
	lastClass := uint8(255)
	var cluster extents
	for i := base + 1; i < end; i++ {
		class := buf[i].umark.class
		if class == 0 {
			if !sh.rtl {
				xOff -= buf[i].XAdvance
				yOff -= buf[i].YAdvance
			} else {
				xOff += buf[i].XAdvance
				yOff += buf[i].YAdvance
			}
			continue
		}
		if comps > 1 {
			// A mark from outside the ligature goes on its last part. One
			// numbered zero inside it is left at minus one, as HarfBuzz
			// leaves it: only a mark that is itself a ligature is numbered
			// so, and no font places one by this.
			comp := buf[i].lig.comp - 1
			if ligID == 0 || ligID != buf[i].lig.id || comp >= comps {
				comp = comps - 1
			}
			if comp != lastComp {
				lastComp = comp
				lastClass = 255
				component = baseExt
				// The parts in the order the line is drawn. A run set upright
				// has no horizontal direction of its own, and HarfBuzz takes
				// its script's; this takes left to right, which differs only
				// for a ligature of a right-to-left script, set upright, in a
				// face that positions none of its marks.
				if !sh.rtl {
					component.xBearing += comp * component.width / comps
				} else {
					component.xBearing += (comps - 1 - comp) * component.width / comps
				}
				component.width /= comps
			}
		}
		if class != lastClass {
			lastClass = class
			cluster = component
		}
		buf[i].XAdvance, buf[i].YAdvance = 0, 0
		if x, y, ok := sh.fallbackPlace(buf[i].GID, &cluster, class); ok {
			buf[i].XOffset = f.scale(x) + xOff
			buf[i].YOffset = f.scale(y) + yOff
			continue
		}
		// A mark whose own ink cannot be had keeps its offset, and only moves
		// back with the pen.
		buf[i].XOffset += xOff
		buf[i].YOffset += yOff
	}
}

// fallbackPlace is where one mark of a class goes against the ink below it,
// in font units, and moves that ink to include the mark: HarfBuzz's
// position_mark. A mark whose own ink cannot be had is not placed.
func (sh shaper) fallbackPlace(gid int, base *extents, class uint8) (x, y int, ok bool) {
	f := sh.f
	mark, ok := f.glyphExtents(gid)
	if !ok {
		return 0, 0, false
	}
	gap := f.unitsPerEm / 16

	switch {
	case (class == cccDoubleBelow || class == cccDoubleAbove) && !sh.features.Vertical:
		// Over the join to the next base, which is to one side of this one
		// only on a horizontal line. HarfBuzz centres the mark in a run set
		// upright, as below.
		if !sh.rtl {
			x = base.xBearing + base.width - mark.width/2 - mark.xBearing
		} else {
			x = base.xBearing - mark.width/2 - mark.xBearing
		}
	case class == cccAttachedBelowLeft || class == cccBelowLeft || class == cccAboveLeft:
		x = base.xBearing - mark.xBearing
	case class == cccAttachedAboveRight || class == cccBelowRight || class == cccAboveRight:
		x = base.xBearing + base.width - mark.width - mark.xBearing
	default:
		// Centred, which is also what every class this does not name gets.
		x = base.xBearing + (base.width-mark.width)/2 - mark.xBearing
	}

	switch class {
	case cccDoubleBelow, cccBelowLeft, cccBelow, cccBelowRight,
		cccAttachedBelowLeft, cccAttachedBelow:
		if class != cccAttachedBelowLeft && class != cccAttachedBelow {
			base.height -= gap
		}
		y = base.yBearing + base.height - mark.yBearing
		// Never shift a mark below up into its base.
		if (gap > 0) == (y > 0) {
			base.height -= y
			y = 0
		}
		base.height += mark.height
	case cccDoubleAbove, cccAboveLeft, cccAbove, cccAboveRight,
		cccAttachedAbove, cccAttachedAboveRight:
		if class != cccAttachedAbove && class != cccAttachedAboveRight {
			base.yBearing += gap
			base.height -= gap
		}
		y = base.yBearing - mark.yBearing - mark.height
		// Nor a mark above too far down into it.
		if (gap > 0) != (y > 0) {
			correction := -(y / 2)
			base.yBearing += correction
			base.height -= correction
			y += correction
		}
		base.yBearing -= mark.height
		base.height += mark.height
	}
	return x, y, true
}
