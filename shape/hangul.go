package shape

// Hangul: syllables drawn whole where the face can, jamo drawn with the
// features that assemble them where it cannot, and the tone marks put in front
// of their syllable. It is HarfBuzz's Hangul shaper (hb-ot-shaper-hangul.cc).
//
// A Hangul syllable is written either as one precomposed character or as the
// two or three jamo it is made of — a leading consonant, a vowel and an
// optional trailing consonant — and a font draws one or the other well, seldom
// a mixture. So before anything else each syllable is put into the one spelling
// the face can draw: composed where the face has the whole syllable, and taken
// apart into its jamo where it does not but has them. Jamo that stay jamo are
// marked for 'ljmo', 'vjmo' and 'tjmo', the features a font assembles them
// with, and kept out of 'calt', which some fonts (Noto Sans CJK among them)
// write their jamo lookups under and which is not meant for them.
//
// Unicode's own normalisation is not applied on top: HarfBuzz sets Hangul with
// none beyond drawing a character the face lacks as its decomposition, because
// its composition would mix syllables and jamo in one word. See normalization.
//
// The tone marks, U+302E and U+302F, are written after the syllable they belong
// to and drawn before it. One that follows no syllable is shown against a
// dotted circle, as a mark with nothing to sit on is in the syllabic models.
// A tone mark the face draws with no width is left where it is written: it is
// made to be drawn over what precedes it.

// The jamo features, as a Hangul run marks each character.
const (
	jamoNone uint8 = iota
	jamoL
	jamoV
	jamoT
)

// The jamo and syllable ranges, as HarfBuzz reads them. The combining ranges
// are the ones Unicode's algorithm composes; the others are the Old Hangul
// jamo, which compose into nothing.
func isHangulL(r rune) bool {
	return r >= 0x1100 && r <= 0x115F || r >= 0xA960 && r <= 0xA97C
}

func isHangulV(r rune) bool {
	return r >= 0x1160 && r <= 0x11A7 || r >= 0xD7B0 && r <= 0xD7C6
}

func isHangulT(r rune) bool {
	return r >= 0x11A8 && r <= 0x11FF || r >= 0xD7CB && r <= 0xD7FB
}

func isCombiningL(r rune) bool { return r >= hangulLBase && r < hangulLBase+hangulLCount }
func isCombiningV(r rune) bool { return r >= hangulVBase && r < hangulVBase+hangulVCount }
func isCombiningT(r rune) bool { return r > hangulTBase && r < hangulTBase+hangulTCount }
func isHangulSyllable(r rune) bool {
	return r >= hangulSBase && r < hangulSBase+hangulSCount
}
func isHangulTone(r rune) bool { return r == 0x302E || r == 0x302F }

// hangulPreprocess puts each syllable of a run into the spelling the face can
// draw, and each tone mark in front of its syllable: preprocess_text_hangul.
// It returns the runes it was given when there is no Hangul in them.
//
// A syllable that becomes several characters, or several that become one,
// becomes one cluster, at the earliest of the offsets that went into it.
func (f *Face) hangulPreprocess(runes []rune, offsets []int) ([]rune, []int) {
	any := false
	for _, r := range runes {
		if isHangulL(r) || isHangulSyllable(r) || isHangulTone(r) {
			any = true
			break
		}
	}
	if !any {
		return runes, offsets
	}
	out := make([]rune, 0, len(runes)+4)
	off := make([]int, 0, len(runes)+4)
	emit := func(r rune, o int) {
		out = append(out, r)
		off = append(off, o)
	}
	// oneCluster gives out[from:] the earliest of their offsets.
	oneCluster := func(from int) {
		lo := off[from]
		for _, o := range off[from:] {
			lo = min(lo, o)
		}
		for k := from; k < len(off); k++ {
			off[k] = lo
		}
	}
	zeroWidth := func(r rune) bool {
		gid, ok := f.GlyphID(r)
		return ok && f.advanceGID(gid) == 0
	}
	// The extent of the syllable just emitted, valid only while start < end.
	start, end := 0, 0
	for i := 0; i < len(runes); {
		u := runes[i]
		if isHangulTone(u) {
			if start < end && end == len(out) {
				// After a syllable: in front of it, unless it is drawn over it.
				emit(u, offsets[i])
				if !zeroWidth(u) {
					oneCluster(start)
					copy(out[start+1:], out[start:end])
					out[start] = u
				}
			} else if f.hasGlyph(dottedCircle) {
				// After nothing it can sit on: against a dotted circle, on the
				// side it is drawn on.
				if zeroWidth(u) {
					emit(dottedCircle, offsets[i])
					emit(u, offsets[i])
				} else {
					emit(u, offsets[i])
					emit(dottedCircle, offsets[i])
				}
			} else {
				emit(u, offsets[i])
			}
			i++
			start, end = len(out), len(out)
			continue
		}
		start = len(out)
		if isHangulL(u) && i+1 < len(runes) && isHangulV(runes[i+1]) {
			l, v := u, runes[i+1]
			var t rune
			if i+2 < len(runes) && isHangulT(runes[i+2]) {
				t = runes[i+2]
			}
			n := 2
			if t != 0 {
				n = 3
			}
			// Composed, where the face has the syllable.
			if isCombiningL(l) && isCombiningV(v) && (t == 0 || isCombiningT(t)) {
				s := hangulSBase + (l-hangulLBase)*hangulNCount + (v-hangulVBase)*hangulTCount
				if t != 0 {
					s += t - hangulTBase
				}
				if f.hasGlyph(s) {
					lo := offsets[i]
					for _, o := range offsets[i+1 : i+n] {
						lo = min(lo, o)
					}
					emit(s, lo)
					i += n
					end = start + 1
					continue
				}
			}
			// Otherwise as jamo, with the features that assemble them.
			for k := range n {
				emit(runes[i+k], offsets[i+k])
			}
			oneCluster(start)
			i += n
			end = start + n
			continue
		}
		if isHangulSyllable(u) {
			has := f.hasGlyph(u)
			si := u - hangulSBase
			lIndex, vIndex, tIndex := si/hangulNCount, (si%hangulNCount)/hangulTCount, si%hangulTCount
			// A syllable with no trailing consonant and one after it: the two
			// as one syllable, where the face has it.
			if tIndex == 0 && i+1 < len(runes) && isCombiningT(runes[i+1]) {
				if s := u + runes[i+1] - hangulTBase; f.hasGlyph(s) {
					emit(s, min(offsets[i], offsets[i+1]))
					i += 2
					end = start + 1
					continue
				}
			}
			// Taken apart where the face has not the syllable, or where a
			// trailing consonant that composes with nothing follows it.
			trailing := tIndex == 0 && i+1 < len(runes) && isHangulT(runes[i+1])
			if !has || trailing {
				l := hangulLBase + lIndex
				v := hangulVBase + vIndex
				t := hangulTBase + tIndex
				if f.hasGlyph(l) && f.hasGlyph(v) && (tIndex == 0 || f.hasGlyph(t)) {
					emit(l, offsets[i])
					emit(v, offsets[i])
					if tIndex != 0 {
						emit(t, offsets[i])
					}
					i++
					if has && tIndex == 0 {
						// The trailing consonant that made it come apart is
						// part of the syllable.
						emit(runes[i], offsets[i])
						i++
					}
					oneCluster(start)
					end = len(out)
					continue
				}
			}
			if has {
				end = start + 1
			}
		}
		emit(u, offsets[i])
		i++
	}
	return out, off
}

// hangulFeatures is which jamo feature each character of a preprocessed run is
// for: the leading, vowel and trailing jamo of every syllable the run spells as
// jamo, and nothing for anything else.
//
// It reads the run preprocessing produced rather than being recorded by it,
// because normalisation runs in between and may change the run's length
// elsewhere. It finds exactly what preprocessing marked: every syllable left as
// jamo is a leading and a vowel jamo with at most one trailing one, which is
// the shape this reads, and a character that preprocessing did not mark is not
// a leading jamo followed by a vowel one.
func hangulFeatures(runes []rune) []uint8 {
	var feats []uint8
	for i := 0; i+1 < len(runes); i++ {
		if !isHangulL(runes[i]) || !isHangulV(runes[i+1]) {
			continue
		}
		if feats == nil {
			feats = make([]uint8, len(runes))
		}
		feats[i], feats[i+1] = jamoL, jamoV
		if i+2 < len(runes) && isHangulT(runes[i+2]) {
			feats[i+2] = jamoT
			i++
		}
		i++
	}
	return feats
}

// markJamo gives each glyph of a Hangul run the mask its jamo feature is for,
// and every glyph that is not a jamo the one 'calt' is for. feats is
// hangulFeatures of the run's characters, one to one with the glyphs, or nil.
func markJamo(buf []Glyph, runes []rune, feats []uint8) {
	for i := range buf {
		if feats != nil {
			switch feats[i] {
			case jamoL:
				buf[i].mask |= maskLjmo
			case jamoV:
				buf[i].mask |= maskVjmo
			case jamoT:
				buf[i].mask |= maskTjmo
			}
		}
		if r := runes[i]; !isHangulL(r) && !isHangulV(r) && !isHangulT(r) {
			buf[i].mask |= maskCaltNotJamo
		}
	}
}

// markToneCircles gives each dotted circle a tone mark was put against the
// tone mark's own properties, as HarfBuzz gives them: it makes the circle by
// replacing the tone mark with the two characters, and each keeps what the
// tone mark was — a spacing mark of class 224. So the circle is a mark too,
// and where it comes first, the tone mark is not placed against it.
//
// The circle is found by its offset: preprocessing gives it the tone mark's,
// and a circle the text itself holds has its own.
func markToneCircles(buf []Glyph, runes []rune, offsets []int) {
	for i, r := range runes {
		if r != dottedCircle {
			continue
		}
		for _, j := range [2]int{i - 1, i + 1} {
			if j >= 0 && j < len(runes) && isHangulTone(runes[j]) && offsets[j] == offsets[i] {
				buf[i].class = classOfRune(runes[j])
				buf[i].umark = unicodeMarkOf(runes[j])
				break
			}
		}
	}
}

// keepShown is what dropHiddenCharacters leaves of a per-character slice: the
// entries of the characters it keeps, in order.
func keepShown(per []uint8, runes []rune) []uint8 {
	out := per[:0:0]
	for i, r := range runes {
		if !hiddenBeforeShaping(r) {
			out = append(out, per[i])
		}
	}
	return out
}
