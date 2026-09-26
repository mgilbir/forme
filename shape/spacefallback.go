package shape

// Drawing a character the face has no glyph for, in any spelling, with a glyph
// it does have: HarfBuzz's two stand-ins.
//
// Normalisation asks the face for a character and then for its canonical
// decomposition. When neither is there HarfBuzz tries two more things before it
// settles for .notdef, both in hb-ot-shape-normalize.cc's
// decompose_current_character:
//
//   - A space separator is drawn with the face's U+0020, and given the width
//     the separator is for — an em for an ideographic space, half of one for an
//     en space, a sixteenth for a hair space — rather than the ordinary space's.
//     hb-ot-shape-fallback.cc's _hb_ot_shape_fallback_spaces sets the width,
//     after the font's own advances and before its positioning rules.
//   - U+2011 NON-BREAKING HYPHEN is drawn as U+2010 HYPHEN. It is the one
//     character that is a no-break version of another and is not a space, and
//     it differs from its partner only in where a line may break, which is
//     settled before any face is asked.
//
// Neither is a canonical equivalence, which is why normalize.go does not make
// them: a stand-in draws something in place of the character rather than
// spelling the character another way.
//
// They used to be left out on purpose. The argument was that .notdef, counted
// as missing, is an answer a caller can see and act on, and layout does act on
// it: it goes to another face for the cluster. But the answer a reader gets from
// a browser is HarfBuzz's, and HarfBuzz never reports these as missing, so no
// browser goes looking. A face with a space and no ideographic space sets an
// ideographic space in itself, an em wide. The WPT's CJK subset faces are
// cut down that way — NotoSansCJKjp-Regular-subset-halt-min.otf has U+0020 and
// not U+3000 — and this package drew .notdef where HarfBuzz draws the space.
//
// It is only the faces shaped as HarfBuzz shapes them. A face set by character
// code (a standard face, or one embedded as a simple font) already sets every
// character it has no code for as its space, at the space's width, and says so;
// see missingByCode.

// spaceKind is what a space separator the face has no glyph for is set as,
// once it is drawn with the face's U+0020. It is HarfBuzz's
// hb_unicode_funcs_t::space_t, and the values that divide an em are the
// divisor, as HarfBuzz's are.
type spaceKind uint8

const (
	notSpace  spaceKind = 0
	spaceEm   spaceKind = 1
	spaceEm2  spaceKind = 2
	spaceEm3  spaceKind = 3
	spaceEm4  spaceKind = 4
	spaceEm5  spaceKind = 5
	spaceEm6  spaceKind = 6
	spaceEm16 spaceKind = 16
	// space4Em18 is four eighteenths of an em, the medium mathematical space.
	space4Em18 spaceKind = 17
	// spaceAsSpace keeps the width of the face's own space.
	spaceAsSpace spaceKind = 18
	// spaceFigure is as wide as a digit.
	spaceFigure spaceKind = 19
	// spacePunctuation is as wide as a full stop, or a comma.
	spacePunctuation spaceKind = 20
	// spaceNarrow is half the face's space.
	spaceNarrow spaceKind = 21
)

// spaceKindOf is HarfBuzz's space_fallback_type: the space separators it will
// draw with U+0020, and at what width. U+1680 OGHAM SPACE MARK is a space
// separator too and is not here, because HarfBuzz leaves it out: it is drawn
// as a mark on a stem line, not as a gap.
func spaceKindOf(r rune) spaceKind {
	switch r {
	case 0x0020, 0x00A0:
		return spaceAsSpace
	case 0x2000, 0x2002: // en quad, en space
		return spaceEm2
	case 0x2001, 0x2003: // em quad, em space
		return spaceEm
	case 0x2004: // three-per-em space
		return spaceEm3
	case 0x2005: // four-per-em space
		return spaceEm4
	case 0x2006: // six-per-em space
		return spaceEm6
	case 0x2007:
		return spaceFigure
	case 0x2008:
		return spacePunctuation
	case 0x2009: // thin space
		return spaceEm5
	case 0x200A: // hair space
		return spaceEm16
	case 0x202F: // narrow no-break space
		return spaceNarrow
	case 0x205F: // medium mathematical space
		return space4Em18
	case 0x3000: // ideographic space
		return spaceEm
	}
	return notSpace
}

// standIn is the glyph this face draws for a character it has no glyph for,
// neither whole nor as its canonical decomposition, and what kind of space it
// is set as, if it is one. ok is false where there is no stand-in either, and
// the character is .notdef.
//
// It is asked after the character and its decomposition, as HarfBuzz asks it:
// an en quad decomposes to an en space, and a face with the en space draws that.
func (f *Face) standIn(r rune) (gid int, kind spaceKind, ok bool) {
	if !f.composite() {
		return 0, notSpace, false
	}
	if kind = spaceKindOf(r); kind != notSpace {
		if gid, ok = f.GlyphID(' '); ok {
			return gid, kind, true
		}
		return 0, notSpace, false
	}
	if r == 0x2011 {
		if gid, ok = f.GlyphID(0x2010); ok {
			return gid, notSpace, true
		}
	}
	return 0, notSpace, false
}

// StandsIn reports whether the face draws a character only with a stand-in:
// it has no glyph for it, whole or as its canonical decomposition, and draws
// it as its own space or hyphen, as HarfBuzz does.
//
// Shaping counts such a character as set, and for the face a run was given
// that is the answer: HarfBuzz sets an ideographic space in a face with only
// U+0020, and so does every browser shaping with it. It is not the answer for
// a library choosing which face to give a run in the first place. A browser's
// fallback for a character no named family has chooses by what the faces
// have, so a pan-Latin face that would stand in for an ideographic space is
// not a face with one, and a CJK face is chosen for it. A caller's
// FallbackFontSet asks this to choose the same way.
func (f *Face) StandsIn(r rune) bool {
	var parts [4]rune
	if _, ok := f.drawnAs(r, 0, parts[:0]); ok {
		return false
	}
	_, _, stood := f.standIn(r)
	return stood
}

// standInAdvance is the advance of a glyph standing in for a space separator,
// in thousandths of an em, given the advance it has now: the face's own for
// the glyph, or what a substitution left it.
//
// The arithmetic is _hb_ot_shape_fallback_spaces', in font units and integers,
// because the answer is compared with HarfBuzz's unit for unit. A fraction of
// an em is rounded to the nearest unit; four eighteenths of one, and half a
// narrow space, are truncated. A figure space is as wide as the first digit the
// face has, and a punctuation space as a full stop or else a comma; a face with
// neither keeps its space's width, as it does in HarfBuzz.
func (f *Face) standInAdvance(kind spaceKind, advance float64) float64 {
	upem := f.unitsPerEm
	switch kind {
	case spaceEm, spaceEm2, spaceEm3, spaceEm4, spaceEm5, spaceEm6, spaceEm16:
		n := int(kind)
		return f.scale((upem + n/2) / n)
	case space4Em18:
		return f.scale(upem * 4 / 18)
	case spaceFigure:
		for d := '0'; d <= '9'; d++ {
			if gid, ok := f.GlyphID(d); ok {
				return f.advanceGID(gid)
			}
		}
	case spacePunctuation:
		if gid, ok := f.GlyphID('.'); ok {
			return f.advanceGID(gid)
		}
		if gid, ok := f.GlyphID(','); ok {
			return f.advanceGID(gid)
		}
	case spaceNarrow:
		return f.scale(f.units(advance) / 2)
	}
	return advance
}

// setStandInSpaces gives each glyph standing in for a space separator the
// width the separator is for. It runs once the substitutions are done and
// before any positioning rule, which is where HarfBuzz sets them: a font's
// kerning then applies to the width the space has, as it would to any glyph.
//
// A glyph a ligature made is left alone, as HarfBuzz leaves one. It stands for
// more than the space, so the space's width is not its width; and a ligature
// product never carries the kind, because it is made afresh.
//
// In a run set upright the separator's length is down the page, and is set in
// the vertical advance instead. See standInYAdvance.
func (f *Face) setStandInSpaces(buf []Glyph, vertical bool) {
	for i := range buf {
		k := buf[i].space
		switch {
		case k == notSpace:
		case vertical:
			buf[i].YAdvance = f.standInYAdvance(k, buf[i].YAdvance)
		default:
			buf[i].XAdvance = f.standInAdvance(k, buf[i].XAdvance)
		}
	}
}

// standInYAdvance is standInAdvance for a run set upright: the same fractions
// of an em and the same glyphs asked, as lengths down the page — negative, as
// Glyph.YAdvance is — with a figure or punctuation space as long as the
// glyph's vertical advance. The arithmetic is HarfBuzz's for a vertical
// buffer, whose negative lengths truncate towards zero.
func (f *Face) standInYAdvance(kind spaceKind, advance float64) float64 {
	upem := f.unitsPerEm
	vertical := func(gid int) float64 {
		a, _, _ := f.verticalUnits(gid)
		return -f.scale(a)
	}
	switch kind {
	case spaceEm, spaceEm2, spaceEm3, spaceEm4, spaceEm5, spaceEm6, spaceEm16:
		n := int(kind)
		return f.scale(-((upem + n/2) / n))
	case space4Em18:
		return f.scale(-upem * 4 / 18)
	case spaceFigure:
		for d := '0'; d <= '9'; d++ {
			if gid, ok := f.GlyphID(d); ok {
				return vertical(gid)
			}
		}
	case spacePunctuation:
		if gid, ok := f.GlyphID('.'); ok {
			return vertical(gid)
		}
		if gid, ok := f.GlyphID(','); ok {
			return vertical(gid)
		}
	case spaceNarrow:
		return f.scale(f.units(advance) / 2)
	}
	return advance
}
