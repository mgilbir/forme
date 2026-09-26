package shape

// What HarfBuzz's Hebrew shaper adds to normalisation (hb-ot-shaper-hebrew.cc):
// one reordering of the points, and the presentation forms for a face that
// positions no marks.
//
// The Hebrew model is otherwise the default one — the font's features over the
// run as it stands — and plan.go names what it does not do.

// reorderHebrewMarks moves a meteg, or any mark written below, in front of a
// sheva or hiriq that follows a patah or qamats, within one run of marks
// already in canonical order: reorder_marks_hebrew.
//
// Canonical order sorts the points by their fixed-position classes, which puts
// a patah or qamats, then a sheva or hiriq, then the meteg. Set that way a
// meteg lands under the second vowel, where the text meant it under the first:
// the order the Masoretic texts are written in and the SBL fonts are made for
// is patah or qamats, meteg, then sheva or hiriq. HarfBuzz makes the one swap
// and no other, the first time the pattern occurs in a run, and the two marks
// it swaps become one cluster.
func (n *normalizer) reorderHebrewMarks(start, end int) {
	for i := start + 2; i < end; i++ {
		c0 := reorderClass(n.out[i-2])
		c1 := reorderClass(n.out[i-1])
		c2 := reorderClass(n.out[i])
		// The classes are the reordered ones: patah (Unicode's 17) and qamats
		// (18) are 20 and 21, sheva (10) and hiriq (14) are 22 and 23, and
		// meteg (22) is 25. A mark below keeps its 220.
		if (c0 == 20 || c0 == 21) && (c1 == 22 || c1 == 23) && (c2 == 25 || c2 == cccBelow) {
			lo := min(n.off[i-1], n.off[i])
			n.out[i-1], n.out[i] = n.out[i], n.out[i-1]
			n.off[i-1], n.off[i] = lo, lo
			return
		}
	}
}

// composeHebrew is the composition HarfBuzz's Hebrew shaper allows beyond
// Unicode's, for a face with no mark positioning of its own: compose_hebrew.
//
// Unicode excludes the Hebrew presentation forms from composition — a letter
// with a dagesh stays two characters in every normalisation form — because a
// modern font draws the point and places it. A font that positions nothing
// cannot place it, and draws the letter well only as the presentation form it
// carries for it; HarfBuzz composes the pair into that form, where the face has
// a glyph for it, and so does this. The pairs are HarfBuzz's.
func composeHebrew(a, b rune) (rune, bool) {
	switch b {
	case 0x05B4: // hiriq
		if a == 0x05D9 { // yod
			return 0xFB1D, true
		}
	case 0x05B7: // patah
		switch a {
		case 0x05F2: // yiddish double yod
			return 0xFB1F, true
		case 0x05D0: // alef
			return 0xFB2E, true
		}
	case 0x05B8: // qamats
		if a == 0x05D0 { // alef
			return 0xFB2F, true
		}
	case 0x05B9: // holam
		if a == 0x05D5 { // vav
			return 0xFB4B, true
		}
	case 0x05BC: // dagesh
		switch {
		case a >= 0x05D0 && a <= 0x05EA:
			if f := hebrewDageshForms[a-0x05D0]; f != 0 {
				return f, true
			}
		case a == 0xFB2A: // shin with shin dot
			return 0xFB2C, true
		case a == 0xFB2B: // shin with sin dot
			return 0xFB2D, true
		}
	case 0x05BF: // rafe
		switch a {
		case 0x05D1: // bet
			return 0xFB4C, true
		case 0x05DB: // kaf
			return 0xFB4D, true
		case 0x05E4: // pe
			return 0xFB4E, true
		}
	case 0x05C1: // shin dot
		switch a {
		case 0x05E9: // shin
			return 0xFB2A, true
		case 0xFB49: // shin with dagesh
			return 0xFB2C, true
		}
	case 0x05C2: // sin dot
		switch a {
		case 0x05E9: // shin
			return 0xFB2B, true
		case 0xFB49: // shin with dagesh
			return 0xFB2D, true
		}
	}
	return 0, false
}

// hebrewDageshForms is each letter from alef to tav with a dagesh, as its
// presentation form, or 0 for the five letters Unicode encodes none for:
// het, final mem, final nun, ayin and final tsadi.
var hebrewDageshForms = [...]rune{
	0xFB30, 0xFB31, 0xFB32, 0xFB33, 0xFB34, 0xFB35, 0xFB36, 0, // alef .. het
	0xFB38, 0xFB39, 0xFB3A, 0xFB3B, 0xFB3C, 0, 0xFB3E, 0, // tet .. final nun
	0xFB40, 0xFB41, 0, 0xFB43, 0xFB44, 0, 0xFB46, 0xFB47, // nun .. qof
	0xFB48, 0xFB49, 0xFB4A, // resh, shin, tav
}

// hasMarkFeature reports whether the rules the run selected declare 'mark', in
// GSUB or in GPOS, with lookups or without: HarfBuzz's has_gpos_mark, which is
// whether its map holds the feature at all.
func (l *layout) hasMarkFeature() bool {
	_, sub := l.featureLookups["mark"]
	_, pos := l.gposFeatures["mark"]
	return sub || pos
}
