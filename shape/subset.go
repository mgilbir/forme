package shape

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/mgilbir/forme/font"
)

// Subsetting: writing a font program that carries only the glyphs a document
// uses. For a large face this is the difference between a file of tens of
// megabytes and one of tens of kilobytes, and it is the reason a PDF may embed
// a CJK font at all.
//
// # Glyph indices are retained, not renumbered
//
// The subset keeps every glyph index the original had. A glyph that is kept
// occupies its original index; one that is dropped becomes an empty entry at
// that index. The alternative — packing the kept glyphs down into a dense range
// — produces a smaller file, and this does not do it, for three reasons that
// all point the same way:
//
//   - The character codes are already written. Encode produced glyph indices
//     into the content stream before anything knew which glyphs would survive,
//     and with Identity-H those codes are CIDs. Renumbering would invalidate
//     every stream already drawn, or require a CIDToGIDMap stream to undo the
//     renumbering — which costs two bytes per CID and gives much of the saving
//     back.
//   - Composite glyphs reference their components by index. Retaining indices
//     means the glyf data of a composite needs no rewriting at all; renumbering
//     means rewriting the interior of every composite glyph, which is where a
//     subsetter's subtle bugs live.
//   - The CID is the glyph index throughout — in /CIDToGIDMap /Identity, in
//     /CIDSet, in /W. One numbering end to end is a property worth paying for.
//
// What is left on the table is loca, which stays at full length: four bytes per
// glyph in the original font, 80 KB for a 20,000-glyph CJK face against the
// megabytes of outline data that glyf sheds. The trade is deliberate, and it is
// the thing to revisit if that ever matters more than the correctness it buys.
// hmtx was left at full length too, and is no longer: it was the same four
// bytes a glyph again, and a table whose glyphs past the last kept one can
// share a metric need not carry one for each (subsettables.go).
//
// A CID-keyed CFF is the exception, and is renumbered. The codes written for it
// are CIDs, not glyph indices, and its charset maps them to glyphs — so a
// subset that writes a new charset for the glyphs it keeps changes no code a
// document holds. See cffsubset.go and cffrenumber.go.
//
// # What is dropped
//
// Everything not needed to render the kept glyphs: layout tables (GSUB, GPOS,
// GDEF), kerning, hinting programs, and any vendor table. What remains is the
// set a conforming reader requires, plus OS/2 where the original had one, and
// of those the ones that describe glyphs describe the kept ones: post names
// none, hmtx stops at the last kept glyph, and cmap maps only the characters
// whose glyph was kept (subsettables.go). name keeps the font's notices —
// copyright, trademark, licence — and the names it goes by, and drops the
// rest: the descriptive records, the IDs only tables a subset does not carry
// refer to, and the Macintosh duplicates (subsetName).

// requiredTables are the tables a subset font keeps. Hinting (cvt, fpgm, prep)
// is dropped with the rest: it improves rendering at small sizes and costs
// bytes, and a subset that keeps it must keep all of it to stay coherent —
// which is why the glyphs' own instructions go too. See stripInstructions.
var requiredTables = []string{"cmap", "glyf", "head", "hhea", "hmtx", "loca", "maxp", "name", "post", "OS/2"}

// Subset returns a font program carrying only the glyphs this face has encoded,
// together with .notdef and the components any kept composite glyph needs.
//
// Each glyph the face used is decided first, as CheckGlyph decides it, and a
// subset that would have to carry a glyph whose data cannot be carried as the
// font states it is refused with a *GlyphError naming the glyphs: none is
// copied as it stands, and none is quietly dropped. See glyphcheck.go.
//
// It reports an error too when the original font cannot be taken apart — a
// truncated loca, a glyf that does not agree with it — rather than emitting a
// program that claims glyphs it does not have.
func (f *Face) Subset() ([]byte, error) {
	data, _, err := f.SubsetGlyphs()
	return data, err
}

// subset is Subset, also returning the glyph indices it kept. SubsetGlyphs
// hands out both, and they must be the same set: a format that describes the
// glyphs the program carries (PDF's /CIDSet) is describing exactly these, and
// computing that twice is how the two come to disagree.
//
// What it may cost is w (subsetwork.go).
func (f *Face) subset(w *subsetWork) ([]byte, []int, error) {
	notdef, err := f.usedVerdicts(w)
	if err != nil {
		return nil, nil, err
	}
	if f.cff2 != nil {
		return f.subsetCFF2(w)
	}
	if f.cff {
		return f.subsetOpenTypeCFF(notdef, w)
	}
	tables := f.sfntTables()
	t, err := f.glyfTables()
	if err != nil {
		return nil, nil, err
	}
	n := t.numGlyphs

	keep, err := f.keepSet(t, notdef, w)
	if err != nil {
		return nil, nil, err
	}
	flat, err := f.flattenKeptVARC(keep, w)
	if err != nil {
		return nil, nil, err
	}

	// Rebuild glyf and loca. A dropped glyph gets a zero-length entry at its own
	// index, which is what makes this a retained-index subset: the numbering is
	// unchanged and a composite's component references stay valid untouched.
	// So does a .notdef that cannot be kept, which is not in keep.
	// glyf grows as the glyphs are written rather than starting at the
	// table's size: overlapping entries can come to many times the table, and
	// w stops them where the subset's bytes run out.
	var newGlyf []byte
	// loca is written as the long form it goes out in, once (#985).
	locaBytes := make([]byte, 4*(n+1))
	for gid := 0; gid < n; gid++ {
		binary.BigEndian.PutUint32(locaBytes[4*gid:], uint32(len(newGlyf)))
		if !keep[gid] {
			continue
		}
		if err := w.poll(); err != nil {
			return nil, nil, err
		}
		b, ok := flat[gid]
		if !ok {
			entry, err := t.entry(gid)
			if err != nil {
				return nil, nil, fmt.Errorf("fonts: glyph %d %w", gid, err)
			}
			if b, err = stripInstructions(entry); err != nil {
				return nil, nil, fmt.Errorf("fonts: internal: glyph %d was decided keepable and %w", gid, err)
			}
		}
		if err := w.write((len(b)+3)&^3, "the glyf table"); err != nil {
			return nil, nil, err
		}
		newGlyf = append(newGlyf, b...)
		for len(newGlyf)%4 != 0 { // glyf entries are long-aligned
			newGlyf = append(newGlyf, 0)
		}
	}
	binary.BigEndian.PutUint32(locaBytes[4*n:], uint32(len(newGlyf)))

	out := map[string][]byte{}
	for _, tag := range requiredTables {
		if b, ok := tables[tag]; ok {
			out[tag] = b
		}
	}
	out["glyf"] = newGlyf
	if len(flat) > 0 {
		out["hmtx"] = flatBearings(out["hmtx"], f.longMetrics, flat)
	}
	// And the count that describes what was just taken out. maxp is copied from
	// the original, so it has to be copied *out* of it before it is written to:
	// the map holds the caller's own table bytes, and a face is shared.
	if maxp, ok := out["maxp"]; ok && len(maxp) >= 32 {
		newMaxp := append([]byte(nil), maxp...)
		binary.BigEndian.PutUint16(newMaxp[26:], 0) // maxSizeOfInstructions
		out["maxp"] = raiseMaxp(newMaxp, flatGlyphs(flat))
	}
	// Always write the long loca form: the short form stores offsets halved, so
	// it cannot represent an odd offset, and choosing between them is one more
	// thing to get wrong for no benefit at this size.
	if err := w.write(4*(n+1), "the loca table"); err != nil {
		return nil, nil, err
	}
	out["loca"] = locaBytes

	newHead := append([]byte(nil), t.head...)
	binary.BigEndian.PutUint16(newHead[50:], 1) // indexToLocFormat: long
	out["head"] = newHead

	if err := f.leanTables(out, keep, w); err != nil {
		return nil, nil, err
	}

	prog, err := w.assemble(out, false)
	if err != nil {
		return nil, nil, err
	}
	return prog, keptGlyphs(keep), nil
}

// keptGlyphs is the glyphs a subset carries, by the face's own indices: those
// keep says, and .notdef, which every subset carries whether it was kept or
// written empty.
func keptGlyphs(keep []bool) []int {
	out := make([]int, 0, countTrue(keep)+1)
	out = append(out, 0)
	for gid := 1; gid < len(keep); gid++ {
		if keep[gid] {
			out = append(out, gid)
		}
	}
	return out
}

// glyfTables are a TrueType face's head, loca and glyf, checked as far as a
// subset needs them as a whole: present, and loca long enough to place every
// glyph the font declares. Whether each glyph lies inside glyf is the glyph's
// own question (entry).
type glyfTables struct {
	head, loca, glyf []byte
	long             bool
	numGlyphs        int
}

func (f *Face) glyfTables() (glyfTables, error) {
	tables := f.sfntTables()
	if tables == nil {
		return glyfTables{}, errors.New("fonts: the font program is no longer an sfnt")
	}
	t := glyfTables{head: tables["head"], loca: tables["loca"], glyf: tables["glyf"], numGlyphs: f.prog.NumGlyphs}
	if len(t.head) < 54 || t.loca == nil || t.glyf == nil {
		return glyfTables{}, errors.New("fonts: the font program lacks head, loca or glyf")
	}
	t.long = binary.BigEndian.Uint16(t.head[50:]) == 1
	// loca holds one offset per glyph plus a terminator.
	need := 2 * (t.numGlyphs + 1)
	if t.long {
		need = 4 * (t.numGlyphs + 1)
	}
	if len(t.loca) < need {
		return glyfTables{}, fmt.Errorf("fonts: loca holds %d bytes, too few for %d glyphs", len(t.loca), t.numGlyphs)
	}
	return t, nil
}

// entry is glyph gid's glyf record, and an error for one loca places outside
// glyf, or ending before it starts.
func (t glyfTables) entry(gid int) ([]byte, error) {
	var start, end int
	if t.long {
		start, end = int(font.Be32(t.loca, 4*gid)), int(font.Be32(t.loca, 4*gid+4))
	} else {
		start, end = 2*font.Be16(t.loca, 2*gid), 2*font.Be16(t.loca, 2*gid+2)
	}
	if start > end || end > len(t.glyf) {
		return nil, errors.New("lies outside the glyf table")
	}
	return t.glyf[start:end], nil
}

// stripInstructions removes a glyph's hinting instructions, leaving its outline
// exactly as it was.
//
// It is the other half of dropping cvt, fpgm and prep. A glyph's instructions
// call functions defined in fpgm and read values out of cvt, so a subset that
// keeps the instructions and drops those tables has handed a rasteriser a
// program whose first CALL is to a function nobody defined. That is an error by
// the specification; what a rasteriser does with it is its own business, and
// FreeType abandons hinting for the glyph while others are less careful.
//
// The whole of the outline is left alone — no point is decoded and no
// coordinate re-encoded — because the instructions are a contiguous run with a
// length in front of them and taking them out is a splice. Every Noto face in
// the corpus carries them: .notdef alone is forty-two bytes of program in eight
// of them.
//
// A composite says it has instructions with a flag on its last component, so
// there the splice is a truncation and the flag is cleared. The component
// references themselves are untouched, which is the property the whole
// subsetter is built on.
//
// It is given glyphs the subset has decided it can keep (glyphcheck.go), whose
// records have been read whole, and an error is a record that is not one of
// those: it used to hand such a record back as it was, which is how a subset
// came to carry bytes it could not read.
func stripInstructions(g []byte) ([]byte, error) {
	if len(g) == 0 {
		return g, nil // an empty glyph: a space, and nothing to strip
	}
	if len(g) < 10 {
		return nil, errors.New("its record is too short for a glyph header")
	}
	contours := int(int16(binary.BigEndian.Uint16(g)))
	if contours >= 0 {
		at := 10 + 2*contours
		if at+2 > len(g) {
			return nil, errors.New("its instruction length is past the end of its record")
		}
		n := int(binary.BigEndian.Uint16(g[at:]))
		if n == 0 {
			return g, nil
		}
		if at+2+n > len(g) {
			return nil, errors.New("its instructions run past the end of its record")
		}
		out := make([]byte, 0, len(g)-n)
		out = append(out, g[:at]...)
		out = append(out, 0, 0) // instructionLength
		return append(out, g[at+2+n:]...), nil
	}

	// A composite: walk to the last component, which is the one that says
	// whether instructions follow it.
	for at := 10; ; {
		if at+4 > len(g) {
			return nil, errors.New("its component records run past the end of its record")
		}
		flags := int(binary.BigEndian.Uint16(g[at:]))
		flagsAt := at
		at += 4 // the flags and the component's glyph index
		if flags&compArgsAreWords != 0 {
			at += 4
		} else {
			at += 2
		}
		switch {
		case flags&compHaveScale != 0:
			at += 2
		case flags&compHaveXYScale != 0:
			at += 4
		case flags&compHave2x2 != 0:
			at += 8
		}
		if at > len(g) {
			return nil, errors.New("its component records run past the end of its record")
		}
		if flags&compMoreComponents != 0 {
			continue
		}
		if flags&compHaveInstructions == 0 {
			return g, nil
		}
		out := append([]byte(nil), g[:at]...)
		binary.BigEndian.PutUint16(out[flagsAt:], uint16(flags&^compHaveInstructions))
		return out, nil
	}
}

// keepSet decides which glyph indices survive: .notdef where it can be kept,
// every glyph this face used, and — transitively — every component those
// glyphs are built from.
//
// The closure matters. An accented letter is usually a composite referring to a
// base letter and a mark, and dropping either leaves a glyph that renders as
// nothing. It follows components of components, because a component may itself
// be composite.
//
// Every glyph it starts from has been decided keepable, which decided its
// components too (glyphcheck.go), so the components it reads are the ones that
// decision read, by the same decoder; a VARC glyph names none, since what it
// is written as is its own.
//
// It is a worklist: each glyph kept is read once, and what it names is queued if
// it was not kept already. It was a fixed point instead, re-reading every kept
// glyph and allocating a fresh set of the whole font per round, and each round
// adds only one level of components — a chain of composites, each built on the
// one before, took as many rounds as it had links: n rounds of n glyphs, and n
// sets of n, for one used glyph at the top.
//
// Each glyph kept and each component read is a unit of w's work.
func (f *Face) keepSet(t glyfTables, notdef bool, w *subsetWork) ([]bool, error) {
	n := t.numGlyphs
	keep := make([]bool, n)
	var work []int
	add := func(gid int) {
		if !keep[gid] {
			keep[gid] = true
			work = append(work, gid)
		}
	}
	if notdef {
		add(0)
	}
	for gid := range f.used {
		if gid >= 0 && gid < n {
			add(gid)
		}
	}
	for len(work) > 0 {
		if err := w.poll(); err != nil {
			return nil, err
		}
		gid := work[len(work)-1]
		work = work[:len(work)-1]
		comps, err := f.glyfComponents(t, gid, true)
		if err != nil {
			return nil, fmt.Errorf("fonts: internal: glyph %d was decided keepable and %w", gid, err)
		}
		if err := w.charge(1+len(comps), "the composite glyphs"); err != nil {
			return nil, err
		}
		for _, c := range comps {
			add(c)
		}
	}
	return keep, nil
}

func countTrue(b []bool) int {
	n := 0
	for _, v := range b {
		if v {
			n++
		}
	}
	return n
}

// parseLoca reads the glyph offset table. Entry i is where glyph i starts and
// entry i+1 where it ends, so a font with n glyphs has n+1 entries; the short
// form stores each offset halved.
func parseLoca(loca []byte, n int, long bool) ([]uint32, error) {
	need := 2 * (n + 1)
	if long {
		need = 4 * (n + 1)
	}
	if len(loca) < need {
		return nil, fmt.Errorf("fonts: loca holds %d bytes, too few for %d glyphs", len(loca), n)
	}
	out := make([]uint32, n+1)
	for i := 0; i <= n; i++ {
		if long {
			out[i] = binary.BigEndian.Uint32(loca[4*i:])
		} else {
			out[i] = uint32(binary.BigEndian.Uint16(loca[2*i:])) * 2
		}
	}
	return out, nil
}

// assembleSFNT writes a table directory and the tables, four-byte aligned with
// the checksums the format specifies.
func assembleSFNT(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags) // the directory is ordered by tag

	n := len(tags)
	searchRange, entrySelector := 16, 0
	for searchRange*2 <= 16*n {
		searchRange *= 2
		entrySelector++
	}
	dir := make([]byte, 12+16*n)
	binary.BigEndian.PutUint32(dir[0:], 0x00010000)
	binary.BigEndian.PutUint16(dir[4:], uint16(n))
	binary.BigEndian.PutUint16(dir[6:], uint16(searchRange))
	binary.BigEndian.PutUint16(dir[8:], uint16(entrySelector))
	binary.BigEndian.PutUint16(dir[10:], uint16(16*n-searchRange))

	// Each table starts on a four-byte boundary, and the last is not padded
	// after it: the length is known before anything is written, so the
	// program is allocated once rather than grown by every table (#985).
	size := len(dir)
	for _, tag := range tags {
		size = (size+3)&^3 + len(tables[tag])
	}
	out := make([]byte, size)
	at := len(dir)
	for i, tag := range tags {
		body := tables[tag]
		at = (at + 3) &^ 3
		rec := 12 + 16*i
		copy(dir[rec:], tag)
		binary.BigEndian.PutUint32(dir[rec+4:], sfntChecksum(body))
		binary.BigEndian.PutUint32(dir[rec+8:], uint32(at))
		binary.BigEndian.PutUint32(dir[rec+12:], uint32(len(body)))
		at += copy(out[at:], body)
	}
	copy(out, dir)
	return out
}

func sfntChecksum(b []byte) uint32 {
	var sum uint32
	for i := 0; i+4 <= len(b); i += 4 {
		sum += binary.BigEndian.Uint32(b[i:])
	}
	if r := len(b) % 4; r != 0 {
		var last [4]byte
		copy(last[:], b[len(b)-r:])
		sum += binary.BigEndian.Uint32(last[:])
	}
	return sum
}
