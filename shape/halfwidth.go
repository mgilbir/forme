package shape

import "github.com/mgilbir/forme/font"

// The half-width forms a CJK font states for its full-width punctuation.
//
// A full-width bracket is the punctuation glyph plus half an em of blank, and
// which half the blank is on is what the mark is: an opening bracket carries it
// in front and a closing one behind. Trimming one — CSS Text §8.2's
// text-spacing-trim — takes that half away, which for a closing bracket is a
// shorter advance and nothing else and for an opening one is a shorter advance
// *and* the ink moved back into the space it vacated.
//
// So the trimmed form is not "half the advance". It is a pair of numbers per
// glyph, and the font states them: OpenType's 'halt' feature is a type 1
// adjustment carrying exactly that placement and advance. Noto Sans CJK's
// subset gives U+FF09 an XAdvance of -500 with no placement, and U+FF08 -500
// of each — the same trim seen from its two sides.
//
// # Why this is read here and not turned on
//
// 'halt' is an optional positioning feature and defaultPositionFeatures is
// deliberately without it, for the reason written there: reading lookups from
// every feature a script *offers* turns the optional ones on for every
// document, and 'palt' next to it narrows every full-width kana to its ink.
// Nothing here changes that. The feature is not applied to any run; its
// adjustments are read into a table of their own so that a caller with a
// character to trim can ask the font what the trimmed form is, one glyph at a
// time, and get the font's answer rather than an assumption about which half
// the blank was on.
//
// That is the whole of the mechanism, and it is not font-feature-settings: a
// document cannot reach it, no feature is enabled by it, and asking about a
// glyph the feature does not cover answers that there is no trimmed form.

// readHalfWidth reads 'halt' into the layout's own table, applying nothing.
//
// Only type 1 subtables, because that is what the feature is: a fixed
// adjustment to a glyph wherever it occurs. A font stating it contextually
// would be stating something else, and the trim would not be a property of the
// glyph any more.
func (l *layout) readHalfWidth(gpos []byte, feats tableFeatures) {
	budget := subtableBudget(gpos)
	for _, tag := range featureTags(gpos, feats.sel) {
		if tag != "halt" {
			continue
		}
		for _, lookup := range featureLookups(gpos, tag, feats) {
			kind, _, _, subs := subtables(lookup, 9, &budget)
			if kind != 1 {
				continue
			}
			for _, sub := range subs {
				l.halfWidthSubtable(sub)
			}
		}
	}
}

// halfWidthSubtable is singlePosSubtable writing to the half-width table.
//
// It is a copy of that function's shape rather than a parameter on it, because
// the two are not the same operation seen twice: one fills the adjustments that
// are applied to every run, and this fills a table that is applied to nothing
// and only answered from. Sharing the destination behind a flag would put the
// optional feature one wrong argument away from the default set, which is the
// mistake the note above exists to prevent.
func (l *layout) halfWidthSubtable(sub []byte) {
	if len(sub) < 6 {
		return
	}
	covered := coverageGlyphs(sub, font.Be16(sub, 2), &l.covWork)
	format := font.Be16(sub, 0)
	valueFormat := font.Be16(sub, 4)
	size := valueSize(valueFormat)
	switch format {
	case 1:
		adj := readValueRecord(sub[6:], valueFormat)
		if adj == (singleAdjust{}) {
			return
		}
		for _, gid := range covered {
			l.setHalfWidth(gid, adj)
		}
	case 2:
		n := font.Be16(sub, 6)
		for i := 0; i < n && i < len(covered); i++ {
			off := 8 + i*size
			if off+size > len(sub) {
				break
			}
			if adj := readValueRecord(sub[off:], valueFormat); adj != (singleAdjust{}) {
				l.setHalfWidth(covered[i], adj)
			}
		}
	}
}

func (l *layout) setHalfWidth(gid int, adj singleAdjust) {
	if l.halfWidth == nil {
		l.halfWidth = map[int]singleAdjust{}
	}
	l.halfWidth[gid] = adj
}

// HalfWidthTrim is what the face says a glyph's full-width blank is worth, in
// font units: how much narrower the trimmed form is, and how far its ink moves.
//
// dx is the placement — negative where the blank was in front of the mark and
// the ink moves back into it — and dAdvance the change in advance, which is
// negative for every trim there is. ok is false for a glyph the font states no
// trimmed form for, which is every glyph in a font without 'halt' and every
// non-punctuation glyph in a font with it.
func (f *Face) HalfWidthTrim(gid int) (dx, dAdvance int, ok bool) {
	if f.layout == nil || f.layout.halfWidth == nil {
		return 0, 0, false
	}
	adj, ok := f.layout.halfWidth[gid]
	if !ok || adj.xAdvance == 0 {
		return 0, 0, false
	}
	return adj.xPlacement, adj.xAdvance, true
}
