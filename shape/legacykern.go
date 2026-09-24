package shape

import "github.com/mgilbir/forme/font"

// The legacy kern table, applied as HarfBuzz applies it.
//
// A font written before GPOS states its kerning in 'kern': pairs of glyphs and
// one number each. HarfBuzz applies it where GPOS offers the run no 'kern'
// feature — so beside a GPOS that positions only marks — and only for the
// models that position a font which states nothing (shaperModel
// .fallbackPosition): an Indic or universal-engine font is expected to state
// its spacing in GPOS.
//
// And it applies the number in a particular way, which is most of why this is
// its own file: half of it on the first glyph's advance and the rest on the
// second's, moving the second glyph back by the same half so that it lands
// where the whole adjustment would have put it. The pen ends up in the same
// place as if the first glyph had taken all of it, but the glyphs do not: Abel
// kerns F against Á by -22, which HarfBuzz sets as 11 off each advance and the
// Á drawn 11 back, and this set as 22 off the F's. Over the Google Fonts tree
// that was 756 of the Latin strings that differed, and most of the Cyrillic
// and Greek ones in the same faces.
//
// A subtable that kerns across the line moves the second glyph up or down
// instead, and each glyph after it rises with it: HarfBuzz chains the whole run
// together as a cursive joint would, and the heights accumulate.
//
// Only format 0, the list of pairs, is read, in either version of the table.
// Format 2's class arrays and Apple's state-machine formats are not: a face
// stating one is kerned by its format 0 subtables and no others, where
// HarfBuzz would apply all of them. Of the 181 faces in the Google Fonts tree
// with a kern table, one states anything else — Coda, whose Apple table has a
// format 2 subtable — and its GPOS offers 'kern', so the table is not applied.

// legacyKern is a kern table as a positioning pass reads it.
type legacyKern struct {
	// subtables are the format 0 subtables that kern along the line or across
	// it, in the order the table states them. Each is applied as a pass of its
	// own and they add up.
	subtables []legacyKernSubtable
	// has says the table has any subtables at all, which is what decides
	// whether HarfBuzz applies it; stateMachine and crossStream are what it
	// says about the model's own mark handling (see positioningFor).
	has, stateMachine, crossStream bool
}

// legacyKernSubtable is one format 0 subtable: its pairs, sorted as the
// format requires, and whether it kerns across the line.
type legacyKernSubtable struct {
	pairs []byte
	n     int
	cross bool
}

// present reports whether the table is one HarfBuzz applies.
func (k *legacyKern) present() bool { return k.has }

// readLegacyKern reads a kern table, in the OpenType version or Apple's.
func readLegacyKern(kern []byte) legacyKern {
	var out legacyKern
	if len(kern) < 4 {
		return out
	}
	if font.Be16(kern, 0) == 0 {
		// The OpenType version: a 16-bit count, and each subtable a version,
		// a 16-bit length, a format byte and a coverage byte whose bits are
		// horizontal (0), minimum (1), cross-stream (2) and override (3).
		n := font.Be16(kern, 2)
		out.has = n != 0
		off := 4
		for i := 0; i < n && i < maxSubtables && off+6 <= len(kern); i++ {
			length := font.Be16(kern, off+2)
			format, coverage := kern[off+4], kern[off+5]
			end := off + length
			if length < 6 || end > len(kern) {
				// A length that does not fit is the last subtable running to
				// the end, which is how a font states one larger than the
				// field can say.
				end = len(kern)
			}
			out.add(kern[off+6:end], format, coverage&0x01 != 0, coverage&0x04 != 0)
			if length < 6 {
				break
			}
			off += length
		}
		return out
	}
	if len(kern) < 8 || font.Be16(kern, 0) != 1 || font.Be16(kern, 2) != 0 {
		return out
	}
	// Apple's version 1.0: a 32-bit count, and each subtable a 32-bit length,
	// a coverage byte whose bits are vertical (7), cross-stream (6) and
	// variation (5), a format byte and a tuple index.
	n := int(font.Be32(kern, 4))
	// HarfBuzz asks this version whether it is there by its version number,
	// which is never zero.
	out.has = true
	off := 8
	for i := 0; i < n && i < maxSubtables && off+8 <= len(kern); i++ {
		length := int(font.Be32(kern, off))
		coverage, format := kern[off+4], kern[off+5]
		if length < 8 || length > len(kern)-off {
			break
		}
		if coverage&0x20 == 0 {
			out.add(kern[off+8:off+length], format, coverage&0x80 == 0, coverage&0x40 != 0)
		}
		off += length
	}
	return out
}

// add records one subtable of either version.
func (k *legacyKern) add(sub []byte, format byte, horizontal, cross bool) {
	if format == 1 {
		k.stateMachine = true
	}
	if cross {
		k.crossStream = true
	}
	if format != 0 || !horizontal || len(sub) < 8 {
		return
	}
	n := min(font.Be16(sub, 0), (len(sub)-8)/6)
	k.subtables = append(k.subtables, legacyKernSubtable{pairs: sub[8 : 8+6*n], n: n, cross: cross})
}

// value is what the subtable states for a pair, or zero: the pairs are sorted
// by the two glyphs together, and searched as HarfBuzz searches them.
func (st *legacyKernSubtable) value(left, right int) int {
	key := uint32(left)<<16 | uint32(right)
	lo, hi := 0, st.n-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		rec := 6 * mid
		switch k := font.Be32(st.pairs, rec); {
		case key < k:
			hi = mid - 1
		case key > k:
			lo = mid + 1
		default:
			return signed16(font.Be16(st.pairs, rec+4))
		}
	}
	return 0
}

// applyLegacyKern applies the kern table over a run, subtable by subtable.
// HarfBuzz's hb_kern_machine_t: each glyph is paired with the next one that is
// not a mark, and the walk moves on to that one.
//
// The table's pairs are left and right as they are drawn, not first and second
// as they are written, so a right-to-left run is walked from its end — as
// HarfBuzz walks it, having turned the buffer round for the table. The glyph
// on the left takes the first half and the one on the right the second.
func (sh shaper) applyLegacyKern(buf []Glyph) {
	at := func(k int) int {
		if sh.rtl {
			return len(buf) - 1 - k
		}
		return k
	}
	chained := false
	for s := range sh.l.legacyKern.subtables {
		st := &sh.l.legacyKern.subtables[s]
		if st.cross && !chained {
			// The first subtable that kerns across the line ties the run into
			// one chain, so that a glyph raised carries every glyph after it.
			chained = true
			for i := range buf {
				sh.gp.kind[i] = attachCursive
				if sh.rtl {
					sh.gp.chain[i] = 1
				} else {
					sh.gp.chain[i] = -1
				}
			}
		}
		for k := 0; k < len(buf); {
			n := k + 1
			for n < len(buf) && sh.l.isMark(buf[at(n)]) {
				n++
			}
			if n >= len(buf) {
				break
			}
			i, j := at(k), at(n)
			if v := st.value(buf[i].GID, buf[j].GID); v != 0 {
				if st.cross {
					buf[j].YOffset = sh.f.scale(v)
				} else {
					first := v >> 1
					second := v - first
					buf[i].XAdvance += sh.f.scale(first)
					buf[j].XAdvance += sh.f.scale(second)
					buf[j].XOffset += sh.f.scale(second)
				}
			}
			k = n
		}
	}
}
