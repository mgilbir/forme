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
// subset.go). name is cut for every subset, whatever its numbering, by
// subsetName below.
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

// subsetNameIDs are the name IDs a subset's name table keeps: what says whose
// font it is and on what terms, and what names it.
//
//   - 0 copyright, 7 trademark, 13 licence description and 14 licence URL: the
//     notices. A licence may ask that they travel with the font software, and a
//     subset is the font software a document carries; whether this one may go
//     without them is not a question forme answers on a font's behalf.
//   - 1, 2, 4, 16, 17, 21 and 22, the family, subfamily and full names; 6, the
//     PostScript name a PDF's /BaseFont is; 3, the unique identifier; and 5, the
//     version. They are what a reader or a font tool shows of the font, and what
//     Load reads it by.
//
// The rest go. 8 to 12 — manufacturer, designer, description and their URLs —
// say who made the font and are not what it is licensed under; 18 is a Mac
// QuickDraw name; 19 is sample text; 20 a PostScript CID findfont name. 23, 24
// and 25 are named by CPAL and fvar, and every ID from 256 up by fvar, STAT,
// GSUB, GPOS or CPAL: a subset carries none of those tables (requiredTables,
// and the CFF subsets' own list), so in a subset nothing names them.
var subsetNameIDs = map[int]bool{
	0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true,
	13: true, 14: true, 16: true, 17: true, 21: true, 22: true,
}

// subsetName is the name table a subset carries: the records of the IDs
// subsetNameIDs keeps, each in the Windows and Unicode platforms' records, and
// in the Macintosh platform's only where neither of those states it. The
// Macintosh records duplicate the others in a single-byte encoding for readers
// that no longer exist, and where a font has nothing else they are what names
// it, so they stay.
//
// It returns nil where the table cannot be read or written again, and the
// subset carries the table as the font has it: a name a subset could have done
// without is no reason to refuse it.
func subsetName(name []byte) []byte {
	return rebuildName(name, func(records []nameRecord) []nameRecord {
		modern := map[int]bool{}
		for _, r := range records {
			if r.platform == 0 || r.platform == 3 {
				modern[r.id] = true
			}
		}
		kept := records[:0]
		for _, r := range records {
			if subsetNameIDs[r.id] && (r.platform != 1 || !modern[r.id]) {
				kept = append(kept, r)
			}
		}
		return kept
	})
}
