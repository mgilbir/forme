package shape

import "github.com/mgilbir/forme/internal/charprop"

// 'stch': a mark that stretches to span the word it is written over.
//
// The Syriac abbreviation mark, U+070F, is drawn as a bar over the letters of
// the abbreviated word, as long as the word is. A font cannot know how long
// that is, so it states the bar as pieces: its 'stch' feature takes the mark
// apart with a multiple substitution into an odd number of glyphs, of which the
// ones at even places are fixed — the ends, and any piece in the middle — and
// the ones at odd places are repeated as often as it takes. The shaper does the
// repeating once the word is positioned: HarfBuzz records which glyphs are
// which straight after the feature is applied (record_stch), and stretches them
// over the rest of the word at the very end (apply_stch). This is both, number
// for number, in font units and integers as HarfBuzz rounds.
//
// The stretched glyphs take no advance: they are laid over the word, each copy
// offset from where the mark stands, the word's own width shared out between
// the fixed pieces and as many copies of the repeating ones as fill it. Where a
// whole number of copies does not fill it exactly one more is added and all of
// them are drawn a little closer together, so that the bar ends where the word
// does.
//
// HarfBuzz bounds the stretch at 256 glyphs, fixed and repeated together, and
// so does this: past that the bar is drawn shorter than the word. The bound is
// HarfBuzz's rule for what the answer is rather than a limit on this package's
// work, and the answer is HarfBuzz's with it.

// What record_stch makes of a glyph a multiple substitution produced under
// 'stch'.
const (
	stchNone uint8 = iota
	stchFixed
	stchRepeating
)

// stchMaxGlyphs is HarfBuzz's STCH_MAX_GLYPHS: the most glyphs one stretch may
// be drawn with.
const stchMaxGlyphs = 256

// isStchWord reports whether a character is part of the word a stretch spans:
// HB_ARABIC_GENERAL_CATEGORY_IS_WORD. The letters, the marks, the numbers and
// the symbols are, and so are the unassigned and private-use characters, whose
// category says nothing; the cased letters are not, which is HarfBuzz's choice
// — a Syriac word has none, and a Latin letter beside a Syriac mark is not part
// of what it abbreviates.
func isStchWord(r rune) bool {
	return charprop.Is(r, charprop.Cn|charprop.Co|charprop.Lm|charprop.Lo|charprop.Mc|
		charprop.Me|charprop.Mn|charprop.Nd|charprop.Nl|charprop.No|charprop.Sc|
		charprop.Sk|charprop.Sm|charprop.So)
}

// recordStch marks the glyphs 'stch' took apart: record_stch. It is called
// straight after the stage the feature is in, so every glyph that is one of
// several made from one is one of the stretch's pieces: the feature's stage
// holds nothing else that multiplies.
func recordStch(buf []Glyph) {
	for i := range buf {
		if !buf[i].multiplied {
			continue
		}
		if buf[i].lig.comp%2 == 1 {
			buf[i].stch = stchRepeating
		} else {
			buf[i].stch = stchFixed
		}
	}
}

// applyStch stretches every run of recorded pieces over the word before it,
// in the order the glyphs are drawn: apply_stch. rtl says the run is drawn
// right to left, and buf is in the order it is drawn.
func (f *Face) applyStch(buf []Glyph, rtl bool) []Glyph {
	any := false
	for i := range buf {
		if buf[i].stch != stchNone {
			any = true
			break
		}
	}
	if !any {
		return buf
	}
	// HarfBuzz walks the buffer from its end in right-to-left order, so a
	// left-to-right run is turned round for the walk and back after it.
	if !rtl {
		reverseGlyphs(buf)
	}
	width := func(g Glyph) int { return f.units(f.advanceGID(g.GID)) }
	// Written from the end backwards, as HarfBuzz writes its buffer, and
	// turned the right way round at the end.
	var out []Glyph
	for i := len(buf); i > 0; i-- {
		if buf[i-1].stch == stchNone {
			out = append(out, buf[i-1])
			continue
		}
		var wTotal, wFixed, wRepeating int64
		nFixed, nRepeating := 0, 0
		end := i
		for i > 0 && buf[i-1].stch != stchNone {
			i--
			if buf[i].stch == stchFixed {
				wFixed += int64(width(buf[i]))
				nFixed++
			} else {
				wRepeating += int64(width(buf[i]))
				nRepeating++
			}
		}
		start := i
		// The rest of the word, which the stretch spans.
		for ctx := i; ctx > 0 && buf[ctx-1].stch == stchNone && buf[ctx-1].word; {
			ctx--
			wTotal += int64(f.units(buf[ctx].XAdvance))
		}
		i++ // the loop's decrement brings it back to start

		// How many more times each repeating piece is drawn.
		copies := int64(0)
		remaining := wTotal - wFixed
		if remaining > wRepeating && wRepeating > 0 {
			copies = remaining/wRepeating - 1
		}
		// One more, drawn closer together, where that fits the word better.
		overlap := int64(0)
		if shortfall := remaining - wRepeating*(copies+1); shortfall > 0 && nRepeating > 0 {
			copies++
			if excess := (copies+1)*wRepeating - remaining; excess > 0 {
				overlap = excess / (copies * int64(nRepeating))
				remaining = 0
			}
		}
		maxCopies := int64(0)
		if nRepeating > 0 && nFixed+nRepeating < stchMaxGlyphs {
			maxCopies = int64((stchMaxGlyphs - nFixed - nRepeating) / nRepeating)
		}
		copies = min(copies, maxCopies)

		x := remaining / 2
		for k := end; k > start; k-- {
			g := buf[k-1]
			w := int64(width(g))
			repeat := int64(1)
			if g.stch == stchRepeating {
				repeat += copies
			}
			g.XAdvance = 0
			for n := int64(0); n < repeat; n++ {
				if rtl {
					x -= w
					if n > 0 {
						x += overlap
					}
				}
				g.XOffset = f.scale(int(x))
				out = append(out, g)
				if !rtl {
					x += w
					if n > 0 {
						x -= overlap
					}
				}
			}
		}
	}
	// out is in the order the walk met the glyphs, which is the order they
	// are drawn in reversed for a right-to-left run and, having been turned
	// round for the walk, the order they are drawn in for a left-to-right one.
	if rtl {
		reverseGlyphs(out)
	}
	return out
}
