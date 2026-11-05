package shape

import (
	"encoding/binary"
)

// The tables around a subset's outlines, cut to what it keeps.
//
// A subset that keeps the font's glyph numbering — a glyf one, and a CFF one
// that is not CID-keyed — used to carry the tables around its outlines as the
// font had them: post with the name of every glyph, an hmtx with a long metric
// for every glyph, and the font's whole cmap. For twenty-one glyphs of a Latin
// face of 2,620 those were 41 KB of a 55 KB subset whose outlines were 2 KB
// (#979). They now say what the subset holds and no more, and each is still
// what the specification asks it to be:
//
//   - post is format 3: its header, with no glyph names, which OpenType allows
//     of any font. The names a font gives its glyphs are not what draws them.
//   - hmtx's long metrics stop at the last glyph kept, or before it where the
//     advances up to it end in a run of equal ones, and hhea's
//     numberOfHMetrics says so. Every kept glyph keeps its advance and left
//     side bearing; a glyph past the long metrics reads the last one's
//     advance, as the table defines; and a glyph the subset dropped past them,
//     which is empty, has a left side bearing of 0.
//   - cmap maps the characters of the face's own map whose glyph was kept, in
//     a (3,1) format 4 subtable and, where a character lies past the BMP, a
//     (3,10) format 12 one, as the CID-keyed subset's always has. A face
//     embedded as a simple font (LoadSimple) keeps its cmap whole: a reader
//     finds such a font's glyphs through it, by the subtables a symbolic font
//     carries as much as by Unicode ones.
//
// The numbering does not change, so nothing a document wrote moves. loca stays
// at full length: four bytes a glyph is what keeping the numbering costs (see
// subset.go). name is kept as the font has it, since OpenType requires it and
// it says whose font this is.
//
// A CID-keyed subset is renumbered, and renumberSFNT writes its tables for the
// glyphs it keeps already; it takes its post from postWithoutNames too.

// leanTables cuts the tables of a subset that keeps the face's numbering —
// out, as it will be written — to the glyphs keep keeps. The tables it writes
// again are spent from w.
func (f *Face) leanTables(out map[string][]byte, keep []bool, w *subsetWork) error {
	n := len(keep)
	if post, ok := out["post"]; ok {
		out["post"] = postWithoutNames(post)
	}
	if hmtx, hhea := out["hmtx"], out["hhea"]; hmtx != nil && len(hhea) >= 36 {
		newHmtx, newHhea := trimHMetrics(hmtx, hhea, n, keep)
		if len(newHmtx) != len(hmtx) {
			if err := w.write(len(newHmtx)+len(newHhea), "the horizontal metrics"); err != nil {
				return err
			}
		}
		out["hmtx"], out["hhea"] = newHmtx, newHhea
	}
	if _, ok := out["cmap"]; ok && !f.simple && f.prog != nil {
		m := make(map[rune]int)
		for r, g := range f.prog.Cmap {
			if g > 0 && g < n && keep[g] {
				m[r] = g
			}
		}
		out["cmap"] = writeCmap(m)
		if err := w.write(len(out["cmap"]), "the cmap table"); err != nil {
			return err
		}
	}
	return nil
}

// postWithoutNames is a post table as format 3: its header, with the version
// that says no glyph names follow. A table too short to have one is kept.
func postWithoutNames(post []byte) []byte {
	if len(post) < 32 {
		return post
	}
	out := append([]byte(nil), post[:32]...)
	binary.BigEndian.PutUint32(out, 0x00030000)
	return out
}

// lastKept is the highest glyph keep keeps, 0 (.notdef) when it keeps none.
func lastKept(keep []bool) int {
	for g := len(keep) - 1; g > 0; g-- {
		if keep[g] {
			return g
		}
	}
	return 0
}

// trimHMetrics is the hmtx of a subset of n glyphs, keep the ones it keeps,
// whose long metrics stop at the last kept glyph, and before it where the
// advances up to it end in a run of equal ones, with the hhea saying how many
// there are. Every kept glyph keeps its advance and left side bearing; a
// glyph past the long metrics reads the last one's advance, and one the
// subset does not keep, which is empty, a left side bearing of 0. Tables it
// cannot read as the hhea states them are kept as they are.
func trimHMetrics(hmtx, hhea []byte, n int, keep []bool) ([]byte, []byte) {
	upto := lastKept(keep) + 1
	long := int(binary.BigEndian.Uint16(hhea[34:]))
	if long < 1 || long > n || len(hmtx) < 4*long+2*(n-long) {
		return hmtx, hhea
	}
	adv := func(g int) uint16 {
		if g >= long {
			g = long - 1
		}
		return binary.BigEndian.Uint16(hmtx[4*g:])
	}
	lsb := func(g int) []byte {
		if g < long {
			return hmtx[4*g+2 : 4*g+4]
		}
		return hmtx[4*long+2*(g-long) : 4*long+2*(g-long)+2]
	}
	k := min(long, upto)
	for k > 1 && adv(k-2) == adv(k-1) {
		k--
	}
	if k == long {
		return hmtx, hhea
	}
	out := make([]byte, 0, 4*k+2*(n-k))
	for g := 0; g < k; g++ {
		out = binary.BigEndian.AppendUint16(out, adv(g))
		out = append(out, lsb(g)...)
	}
	for g := k; g < n; g++ {
		if keep[g] {
			out = append(out, lsb(g)...)
		} else {
			out = append(out, 0, 0)
		}
	}
	newHhea := append([]byte(nil), hhea...)
	binary.BigEndian.PutUint16(newHhea[34:], uint16(k))
	return out, newHhea
}
