package font

import (
	"encoding/binary"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Program is what the parsers here read out of a font program: which glyphs
// exist, their advance widths in 1/1000 of the em, which glyph a character
// maps to, and, for a CID-keyed CFF, how its CIDs and glyph indices relate.
//
// shape.Load reads NumGlyphs, WidthByGID, Cmap, GlyphBBox and CmapPartial, and
// of a CFF, GIDToCID and the Registry, Ordering and Supplement; the subsetters
// read NumGlyphs. WidthByName, WidthByCID, GlyphNames, GlyphPresent,
// GlyphNonEmpty, ComponentGID, MacCmap, SymbolCmap, CmapSubtableCount, CIDGIDs,
// GIDToFD and BudgetExhausted are read by no code outside this package but
// tests: they answered questions a PDF/A validator asked of an embedded font,
// in the repository this package came from. See doc.go.
type Program struct {
	// GlyphNames lists the glyph names defined by the program (Type1/CFF
	// non-CID); nil when the format identifies glyphs by index only.
	GlyphNames map[string]bool
	// WidthByName gives advance widths (1/1000 units) for named glyphs.
	WidthByName map[string]float64
	// NumGlyphs is the glyph count (sfnt/CFF).
	NumGlyphs int
	// GlyphPresent[gid] reports whether a TrueType (glyf-based) glyph's
	// outline data lies within the glyf table (present, possibly empty) as
	// opposed to pointing beyond a truncated table (missing). nil when the
	// font has no glyf table (e.g. CFF-flavoured OpenType).
	GlyphPresent []bool
	// GlyphNonEmpty[gid] reports whether the glyph has an outline (a non-zero
	// length glyf entry). An empty entry is a blank glyph such as space.
	GlyphNonEmpty []bool
	// ComponentGID[gid] reports whether the glyph is referenced as a component
	// of a composite glyph. Such a glyph may carry an outline solely to serve
	// as a building block (e.g. an accent) without being a directly mapped CID.
	ComponentGID []bool
	// GlyphBBox[gid] is the box the glyph's own outline occupies — xMin, yMin,
	// xMax, yMax in font units — as the glyph header states it.
	//
	// The font's word for where the ink of one glyph is, which is a far tighter
	// answer than the face-wide bounding box for anything that has to know
	// whether a particular piece of text reaches somewhere: the face box has to
	// hold an accented capital and a descending bracket, and almost no run
	// contains either.
	//
	// It is what the header says and is not verified against the outline. A
	// font may state a box larger than its contours and some do; none of the
	// uses here is harmed by a box that is too big, and reading the contours to
	// check would mean interpreting them.
	//
	// The entry for an empty glyph is the zero box, which is correct — a space
	// puts no ink anywhere — and GlyphNonEmpty distinguishes it from a glyph
	// whose header really says zero. nil when the font has no glyf table.
	GlyphBBox [][4]int
	// WidthByGID gives advance widths by glyph index, scaled to 1/1000.
	WidthByGID []float64
	// Cmap maps Unicode code points to glyph indices ((3,1) subtable), and
	// mac maps single-byte codes via the (1,0) subtable; symbolCmap maps
	// 0xF000-prefixed codes via a (3,0) subtable.
	Cmap       map[rune]int
	MacCmap    map[byte]int
	SymbolCmap map[uint16]int
	// CmapSubtableCount is the number of subtables declared in the sfnt cmap
	// table (ISO 19005-1 6.3.7 requires a symbolic TrueType font to have
	// exactly one). Zero when there is no cmap table.
	CmapSubtableCount int
	// CIDGIDs reports which CIDs have charstrings (CFF CID-keyed fonts);
	// nil when not CID-keyed.
	CIDGIDs map[int]bool
	// GIDToFD gives the index of the Font DICT each glyph belongs to, for a
	// CID-keyed CFF; nil when not CID-keyed.
	//
	// It is here because it is not otherwise observable and it is the thing a
	// rewritten font is most likely to get wrong: the hinting and the width
	// defaults a glyph is read against come from its Font DICT, and giving it
	// another font's changes nothing a shaper reads and everything a renderer
	// draws.
	GIDToFD []int
	// Registry, Ordering and Supplement are the CFF's ROS: the character
	// collection its CIDs are numbered in. Empty and zero when the font is not
	// CID-keyed.
	//
	// A PDF has to state them. ISO 32000-2 9.7.4.2 requires a CIDFont's
	// /CIDSystemInfo to be compatible with the character collection of its
	// glyph source, so a document embedding this program and declaring
	// Adobe-Identity-0 over an Adobe-Japan1 font is making a false statement
	// about which collection its CIDs belong to.
	//
	// Empty is not the same as "not CID-keyed", and GIDToCID is what answers
	// that. A malformed font can be CID-keyed and still leave these empty, by
	// naming a string it does not carry or a supplement below zero; a caller
	// that has to write a /CIDSystemInfo should treat that as a font it cannot
	// describe rather than write an empty Registry into a document.
	//
	// The three move together: either all of them are read or none is, so
	// testing Registry is enough.
	Registry, Ordering string
	Supplement         int
	// GIDToCID gives the CID of each glyph index, for a CID-keyed CFF; nil when
	// not CID-keyed.
	//
	// The two numberings are not the same and the difference is the whole
	// awkwardness of the format: a charstring is found by glyph index, and a
	// CIDFontType0 is addressed by CID, with the charset mapping between them.
	// A format that embeds one has to say which it is speaking.
	GIDToCID []int
	// WidthByCID gives advance widths by CID for CID-keyed CFF.
	WidthByCID map[int]float64
	// CmapPartial reports that a cmap subtable stopped short of its own end
	// because the font's work budget (see Budget) ran out, so the maps above
	// are missing mappings the font really declares. A consumer must not read
	// "this code is absent from the cmap" as "this code has no glyph" when it
	// is set — that is audit C46's false positive with a different cause: a
	// truncated cmap makes trueTypeGID answer "glyph 0" authoritatively, and a
	// conformant font is then reported as undefined-glyph / .notdef.
	//
	// The parser cannot report the trip itself, having nobody to report it to;
	// the caller reads this, and shape.Load refuses the font on it.
	CmapPartial bool
	// BudgetExhausted reports that the work budget the program was read under
	// ran out, so some field above holds less than the font declares.
	// CmapPartial says whether the cmap is among them; this says whether
	// anything is. The two differ because the budget is the font's and not the
	// cmap's: the glyf walk after the cmap draws on it too, and a font that
	// spends it there leaves ComponentGID short behind a cmap that is whole.
	BudgetExhausted bool
}

// --- sfnt (TrueType / OpenType) ---

func Be16(b []byte, off int) int {
	if off+2 > len(b) {
		return 0
	}
	return int(binary.BigEndian.Uint16(b[off:]))
}

func Be32(b []byte, off int) uint32 {
	if off+4 > len(b) {
		return 0
	}
	return binary.BigEndian.Uint32(b[off:])
}

// MarkComposite, given one glyph's glyf bytes, marks every glyph index it
// references as a component (when the glyph is composite, numberOfContours == -1).
func MarkComposite(g []byte, numGlyphs int, out []bool) {
	markComposite(g, numGlyphs, out, nil)
}

// markComposite is MarkComposite charged to a budget, one unit per component
// record, when b is not nil.
//
// The walk is linear in one glyph's bytes, and ParseSFNT makes it once per
// glyph — which is linear in the table only when the glyphs do not overlap. loca
// is required to ascend and nothing makes it: offsets 0, L, 0, L, … give every
// other glyph the whole of glyf, so half of 65535 glyphs each walk a table of a
// hundred thousand components. That is the same bytes named again, and it is
// what the budget is for.
func markComposite(g []byte, numGlyphs int, out []bool, b *Budget) {
	if len(g) < 2 || int16(Be16(g, 0)) != -1 {
		return
	}
	o := 10
	for o+4 <= len(g) {
		if b != nil && !b.charge(1, "the composite glyphs") {
			return
		}
		flags := Be16(g, o)
		if cgid := Be16(g, o+2); cgid >= 0 && cgid < numGlyphs {
			out[cgid] = true
		}
		o += 4
		if flags&0x0001 != 0 { // ARG_1_AND_2_ARE_WORDS
			o += 4
		} else {
			o += 2
		}
		switch {
		case flags&0x0008 != 0: // WE_HAVE_A_SCALE
			o += 2
		case flags&0x0040 != 0: // WE_HAVE_AN_X_AND_Y_SCALE
			o += 4
		case flags&0x0080 != 0: // WE_HAVE_A_TWO_BY_TWO
			o += 8
		}
		if flags&0x0020 == 0 { // no MORE_COMPONENTS
			break
		}
	}
}

// be16 as signed for the numberOfContours check.

// ParseSFNT parses a TrueType/OpenType font program under a budget of maxWork
// units; see ParseSFNTWithin.
func ParseSFNT(data []byte, maxWork int) *Program {
	return ParseSFNTWithin(data, NewBudget(maxWork))
}

// ParseSFNTWithin parses a TrueType/OpenType font program, charging the work to
// b, which a caller reading more than one part of a font shares between them so
// that the font as a whole is what is bounded.
//
// A budget that runs out does not make this return nil. It returns what it
// read, with BudgetExhausted set and, if the cmap was what went unread,
// CmapPartial — a prefix of the cmap is still correct as far as it goes, and a
// caller asking a narrow question may be able to use it. One that cannot
// refuses the font on the flags.
func ParseSFNTWithin(data []byte, b *Budget) *Program {
	tables := SFNTTables(data)
	if tables == nil {
		return nil
	}

	fp := &Program{}
	head := tables["head"]
	unitsPerEm := 1000
	if len(head) >= 20 {
		if u := Be16(head, 18); u > 0 {
			unitsPerEm = u
		}
	}
	if maxp := tables["maxp"]; len(maxp) >= 6 {
		fp.NumGlyphs = Be16(maxp, 4)
	}

	// hmtx: advance widths, scaled to 1/1000 units.
	if hhea, hmtx := tables["hhea"], tables["hmtx"]; len(hhea) >= 36 && hmtx != nil {
		numH := Be16(hhea, 34)
		fp.WidthByGID = make([]float64, fp.NumGlyphs)
		last := 0.0
		for gid := 0; gid < fp.NumGlyphs; gid++ {
			if gid < numH && 4*gid+2 <= len(hmtx) {
				last = float64(Be16(hmtx, 4*gid)) * 1000 / float64(unitsPerEm)
			}
			fp.WidthByGID[gid] = last
		}
	}

	// The cmap before the glyf walk, because the two share the budget and the
	// cmap is the one a caller cannot do without: read first, it is whole
	// whenever it fits, and a font that spends the rest on its glyphs is
	// reported as that rather than as a truncated cmap it never reached.
	if cmap := tables["cmap"]; len(cmap) >= 4 {
		readCmap(fp, cmap, b)
	}

	// loca/glyf: which glyph indices have outline data within the table.
	if head, loca, glyf := tables["head"], tables["loca"], tables["glyf"]; len(head) >= 52 && loca != nil && glyf != nil {
		longLoca := Be16(head, 50) == 1
		glyfLen := len(glyf)
		fp.GlyphPresent = make([]bool, fp.NumGlyphs)
		// A loca entry, and whether it is one this glyf can be cut at.
		//
		// The bound is checked on the value the table holds rather than after
		// it has become an int, because those are not the same question on
		// every build: a long entry above two gigabytes goes through a 32-bit
		// int as a *negative* number, which is less than every end and so
		// passes a check written only against the top — and is then sliced.
		// Compared as it is read, the answer does not depend on the word size.
		//
		// And an entry the loca does not hold is not an entry. Be16 and Be32
		// answer zero past the end of what they are given, so a loca shorter
		// than maxp's glyph count read every glyph beyond it as starting and
		// ending at zero — present and empty, a blank the font never drew.
		// HarfBuzz takes the glyph count as what the loca can address and
		// treats a glyph past it as having no outline data at all; the
		// subsetter in shape refuses such a loca outright. Absent is the
		// reading that says so: a glyph with no loca entry is not in glyf.
		offAt := func(i int) (int, bool) {
			size := 2
			if longLoca {
				size = 4
			}
			if i < 0 || size*(i+1) > len(loca) {
				return 0, false
			}
			v := uint64(Be16(loca, 2*i)) * 2
			if longLoca {
				v = uint64(Be32(loca, 4*i))
			}
			if v > uint64(glyfLen) {
				return 0, false
			}
			return int(v), true
		}
		fp.GlyphNonEmpty = make([]bool, fp.NumGlyphs)
		fp.ComponentGID = make([]bool, fp.NumGlyphs)
		fp.GlyphBBox = make([][4]int, fp.NumGlyphs)
		for gid := 0; gid < fp.NumGlyphs; gid++ {
			start, startOK := offAt(gid)
			end, endOK := offAt(gid + 1)
			// Present when the entry is well-formed and lies within the glyf
			// table (an empty glyph, start==end, is still present).
			inGlyf := startOK && endOK
			fp.GlyphPresent[gid] = inGlyf && start <= end
			fp.GlyphNonEmpty[gid] = inGlyf && start < end
			if fp.GlyphNonEmpty[gid] {
				markComposite(glyf[start:end], fp.NumGlyphs, fp.ComponentGID, b)
				// The glyph header is numberOfContours and then the four
				// bounds, all int16, so ten bytes. A composite glyph has the
				// same header, which is why this needs no case for one.
				if end-start >= 10 {
					g := glyf[start:end]
					fp.GlyphBBox[gid] = [4]int{
						int(int16(Be16(g, 2))), int(int16(Be16(g, 4))),
						int(int16(Be16(g, 6))), int(int16(Be16(g, 8))),
					}
				}
			}
		}
	}

	fp.BudgetExhausted = b.Exhausted()
	return fp
}

// readCmap chooses and reads the font's cmap subtables: the best Unicode one,
// and the last readable (3,0) symbol and (1,0) Mac Roman ones.
//
// # Each subtable once, and only the ones that are used
//
// An encoding record is eight bytes and names its subtable by offset, so a font
// can name one subtable as often as it likes. 65535 records pointing at one
// format 13 group spanning Unicode are half a megabyte of cmap, and read record
// by record they were 65535 walks of 1.1 million codes — about four and a half
// hours — each of them inside a budget that was per subtable (audit C9). And
// every Unicode subtable of equal or better rank was parsed in turn, each
// replacing the last, although only one of them is ever kept.
//
// Three things close it. The budget is the font's, so however many walks there
// are they share one bound. A subtable is parsed once per offset, however many
// records name it. And of the Unicode subtables only the one that will be kept
// is parsed: they are tried best first and the first that reads is the answer.
// The choice itself is what it always was — highest rank, and among equal ranks
// the later record — and so is the symbol and Mac rule, that the last readable
// one wins; what changed is that reaching the choice no longer means reading
// everything it passes over.
//
// A walk the budget stopped is partial, and partial is final: nothing after it
// can be read either, and settling for a worse subtable in its place would be
// answering with a mapping the font did not choose.
func readCmap(fp *Program, cmap []byte, b *Budget) {
	n := Be16(cmap, 2)
	fp.CmapSubtableCount = n

	type result struct {
		m       map[rune]int
		partial bool
	}
	parsed := map[uint32]result{}
	parse := func(off uint32) (map[rune]int, bool) {
		if r, ok := parsed[off]; ok {
			return r.m, r.partial
		}
		m, partial := parseCmapSubtable(cmap[off:], b)
		m = onlyDeclaredGlyphs(m, fp.NumGlyphs)
		parsed[off] = result{m, partial}
		return m, partial
	}

	type candidate struct {
		rank int
		off  uint32
	}
	var unicode []candidate
	var symbol, mac []uint32
	for i := 0; i < n; i++ {
		rec := 4 + 8*i
		if rec+8 > len(cmap) {
			break
		}
		plat := Be16(cmap, rec)
		enc := Be16(cmap, rec+2)
		off := Be32(cmap, rec+4)
		if uint64(off) >= uint64(len(cmap)) {
			continue
		}
		switch rank := unicodeCmapRank(plat, enc); {
		case rank > 0:
			unicode = append(unicode, candidate{rank, off})
		case plat == 3 && enc == 0:
			symbol = append(symbol, off)
		case plat == 1 && enc == 0:
			mac = append(mac, off)
		}
	}

	// Best rank first and, within a rank, the later record first: reversed,
	// and then a stable sort that keeps that order among equals.
	slices.Reverse(unicode)
	slices.SortStableFunc(unicode, func(x, y candidate) int { return y.rank - x.rank })
	for _, c := range unicode {
		m, partial := parse(c.off)
		if m != nil {
			fp.Cmap = m
			fp.CmapPartial = partial
			break
		}
		// Nothing came back, which is two things: a subtable this cannot read,
		// and one the budget stopped before it read anything. The flag is what
		// tells them apart, and dropping it with the empty map made a reader
		// that gave up look like a font with no cmap at all.
		if partial {
			fp.CmapPartial = true
			break
		}
	}

	// The last readable one of each, so tried from the end.
	lastReadable := func(offs []uint32) map[rune]int {
		for i := len(offs) - 1; i >= 0; i-- {
			m, partial := parse(offs[i])
			fp.CmapPartial = fp.CmapPartial || partial
			if m != nil || partial {
				return m // unreadable leaves the map unset, not empty
			}
		}
		return nil
	}
	if m := lastReadable(symbol); m != nil {
		fp.SymbolCmap = make(map[uint16]int, len(m))
		for r, gid := range m {
			fp.SymbolCmap[uint16(r)] = gid
		}
	}
	if m := lastReadable(mac); m != nil {
		fp.MacCmap = make(map[byte]int, len(m))
		for r, gid := range m {
			if r <= 0xFF {
				fp.MacCmap[byte(r)] = gid
			}
		}
	}
}

// onlyDeclaredGlyphs drops the mappings that name a glyph the font does not
// have.
//
// A cmap entry pointing past maxp's count is not a mapping. There is no such
// glyph: it has no outline, no advance, and nothing to put in a subset — so a
// character "mapped" to one is a character the font cannot set, which is what
// being unmapped means. Keeping the entry says the opposite to everything
// downstream, and every one of them believes it: the shaper records the index as
// used, the width table answers nought for it, the subsetter cannot keep it, and
// the page carries a code the embedded program has no glyph for. Nothing
// reports any of that, because at each step the font appeared to have said so.
//
// Dropping it puts the character back on the path it belongs to — no glyph, so
// the missing-glyph finding and §5's font fallback both see it — which is the
// answer for a character the font has not got.
//
// Found by fuzzing the subsetter: a font declaring two glyphs whose cmap named
// several hundred, of which "glyph 12385 was used and is not in the subset" was
// the first to be noticed.
//
// Nought is left alone: a font with no readable maxp declares no count rather
// than a count of none, and filtering against it would empty every cmap in a
// font whose maxp this reader could not take apart.
func onlyDeclaredGlyphs(m map[rune]int, numGlyphs int) map[rune]int {
	if m == nil || numGlyphs <= 0 {
		return m
	}
	for r, gid := range m {
		if gid < 0 || gid >= numGlyphs {
			delete(m, r)
		}
	}
	return cmapResult(m)
}

// unicodeCmapRank ranks a cmap subtable's (platform, encoding) as a source of
// code-point→GID mappings, higher being better; 0 means "not a Unicode
// subtable" and leaves the pair to the symbol/Mac cases. A font may carry
// several, so the choice has to be deliberate:
//
//	(3,10) Windows full repertoire — a superset of (3,1), reaches beyond the BMP
//	(3,1)  Windows BMP             — what ISO 32000-1 9.6.6.4 names
//	(0,4)/(0,6) Unicode full repertoire
//	(0,0..3)/(0,5) Unicode BMP / variation-sequence-era subtables
//
// The Windows platform outranks the Unicode platform at equal coverage because
// ISO 32000-1 9.6.6.4 describes code→GID lookup in terms of the Windows
// subtables, and because that ordering leaves the pre-existing choice untouched
// for every font whose only mappings are the (3,1)/(3,0)/(1,0) trio.
func unicodeCmapRank(plat, enc int) int {
	switch {
	case plat == 3 && enc == 10:
		return 4
	case plat == 3 && enc == 1:
		return 3
	case plat == 0 && (enc == 4 || enc == 6):
		return 2
	case plat == 0:
		return 1
	}
	return 0
}

// Every subtable format charges the font's Budget, one unit per code it visits.
// The expanding formats — 4, 8, 12 and 13, whose bytes describe ranges rather
// than list glyphs — are the ones that need it, since a few bytes of them can
// name all of Unicode; formats 0, 6 and 10 are bounded by their own size, and
// are charged anyway, because the budget is the font's and a font may name any
// number of them. One knob rather than one per format because the formats are
// alternative encodings of the same thing — a font's code→GID coverage — and no
// caller has a reason to trust one more than another. It used to be a counter
// per subtable, which bounded one subtable and not the table (audit C9).
//
// cmapResult returns out, or nil when it holds no mapping. A subtable that maps
// nothing is, to every caller, indistinguishable from one that could not be
// read, and the distinction that matters is nil vs non-nil: trueTypeGID treats a
// non-nil cmap as authoritative, so an empty one answers "every code is .notdef"
// where the honest answer is "unknown". Reachable from well-formed bytes — a
// 16-byte format-12 subtable declaring nGroups 0, or a budget-exhausted table
// whose every candidate mapping was skipped — so it has to be handled on the way
// out rather than assumed away.
func cmapResult(out map[rune]int) map[rune]int {
	if len(out) == 0 {
		return nil
	}
	return out
}

// budgetStop is what every cmap walk does when the budget runs out: it hands
// back what it has, nil where that is nothing, and the partial flag *set*.
//
// The flag says the walk stopped short, which is a fact about the walk and not
// about how much it had read. It used to be "len(out) > 0", so a budget that
// ran out before the first mapping reported false — and an empty map is nil to
// the caller, which is how "this font has no cmap" is spelt. The two were the
// same answer, so a reader that gave up looked like a font with nothing in it.
//
// The map stays nil, deliberately, and that is the other half of the same
// rule: trueTypeGID treats a non-nil cmap as authoritative, so an empty one
// answers .notdef for every code and reports the whole font. Nil and partial
// together are the honest pair — "nothing was read, and not because there was
// nothing".
//
// A comment rather than a function because each walk returns its own local map;
// this is what the returns point at, and what readCmap keeps by recording
// partialness whether or not a map came back.

// unicodeMaxRune is the last code point Unicode defines. Every format here that
// can express a wider code — 8, 10, 12 and 13 all carry 32-bit codes — stops at
// it, because a key past it is not a character and the callers index by rune.
const unicodeMaxRune = 0x10FFFF

// cmapCoverageGroups expands the {startCharCode, endCharCode, startGlyphID}
// group array that formats 8, 12 and 13 share, writing into out and reporting
// whether the budget stopped it early.
//
// sequential says how a group's glyphs run. Formats 8 and 12 walk the glyph IDs
// alongside the codes — code start+n is glyph startGlyphID+n. Format 13 gives
// every code in the group the same glyph, which is the whole of its purpose: it
// is how a font says "this entire plane is the one glyph I have for it", as the
// LastResort face does.
//
// That difference is also where format 13's cost is. A sequential group runs out
// of glyph IDs after 65536 codes and everything past that is dropped by the
// gid <= 0xFFFF check below; a many-to-one group's glyph stays valid for as long
// as the group runs, so every one of its codes is recorded. The ceiling is the
// same either way — Unicode has 0x110000 code points and a code is written at
// most once — but format 13 is the format that can actually reach it, so the
// work budget is doing real work here rather than standing by.
func cmapCoverageGroups(b []byte, nGroups, groupsAt int, sequential bool, bud *Budget, out map[rune]int) (spent bool) {
	for g := 0; g < nGroups; g++ {
		// Every group is charged at least one unit, so the budget bounds the
		// group loop as well as the expansion within a group.
		if !bud.charge(1, cmapWork) {
			return true
		}
		p := groupsAt + 12*g
		start := Be32(b, p)
		end := Be32(b, p+4)
		startGID := Be32(b, p+8)
		// Groups are required to be sorted and non-overlapping; see the note in
		// case 12 for why neither is enforced. An inverted group is skipped —
		// read as running upwards from start it would walk over unrelated codes.
		if start > end || start > unicodeMaxRune {
			continue
		}
		if end > unicodeMaxRune {
			end = unicodeMaxRune
		}
		for c := start; ; c++ {
			if !bud.charge(1, cmapWork) {
				return true
			}
			gid := uint64(startGID)
			if sequential {
				gid += uint64(c - start)
			}
			// Glyph indices are 16-bit; anything wider is malformed and must
			// not be recorded as if it named a glyph.
			if gid != 0 && gid <= 0xFFFF {
				out[rune(c)] = int(gid)
			}
			if c == end { // c is a uint32; end may be the largest value it holds
				break
			}
		}
	}
	return false
}

// parseCmapSubtableUnder handles cmap formats 0, 4, 6, 8, 10, 12 and 13 — every
// format whose character codes are Unicode code points. It returns nil — not an
// empty map — when the subtable cannot be read (an unsupported format, one
// truncated past use, or one that maps nothing at all): callers treat a non-nil
// cmap as authoritative, so an empty map would claim the font maps no character
// at all, which reads as "every code is .notdef" rather than "unknown".
//
// # The two formats that are not here
//
// Format 2 is the legacy high-byte CJK mapping, and its codes are not Unicode:
// they are Big5, Shift-JIS, Wansung or Johab, which is why it lives at the
// (3,3)…(3,6) encodings. ParseSFNT reaches a subtable by exactly three routes —
// the Unicode ranking, (3,0) symbol and (1,0) Mac Roman — and none of the pairs
// a format-2 subtable sits at is among them, so parsing it would be code no font
// can reach, keyed by code points its caller would then read as Unicode. The
// mapping it wants is the font's Unicode subtable, which every font carrying a
// format 2 also carries.
//
// Format 14 does not map codes to glyphs at all. It maps *pairs* — a base
// character and a variation selector — onto the glyph that pair should take, and
// answers "use the default" for most of them. That is a different function with
// a different signature, not a case in this switch, and returning nil for it is
// correct: it leaves the font's real cmap to whichever subtable holds one.
//
// The second result reports that the budget of maxWork units stopped the parse
// before the subtable's own end, so the returned map is a prefix of the font's
// real coverage. It is separate from the nil result because the mappings that
// were read are still correct — a code the map resolves resolves rightly — but a
// code it does not resolve is unknown rather than absent, and no rule may assert
// against it. Without this the budget reproduces audit C46 exactly.
//
// Because the budget is the caller's, a caller who lowers it moves where the
// prefix ends — which is safe precisely because the prefix is self-describing.
func parseCmapSubtableUnder(b []byte, maxWork int) (map[rune]int, bool) {
	return parseCmapSubtable(b, NewBudget(maxWork))
}

// cmapWork is what a budget that ran out in a cmap walk says it was reading.
const cmapWork = "the character map"

// parseCmapSubtable is parseCmapSubtableUnder, charged to a budget the caller may
// be sharing with the rest of the font.
func parseCmapSubtable(b []byte, bud *Budget) (map[rune]int, bool) {
	out := make(map[rune]int)
	switch Be16(b, 0) {
	case 0:
		if len(b) < 262 {
			return nil, false
		}
		for c := 0; c < 256; c++ {
			if !bud.charge(1, cmapWork) {
				return cmapResult(out), true // see budgetStop
			}
			if gid := int(b[6+c]); gid != 0 {
				out[rune(c)] = gid
			}
		}
	case 4:
		segX2 := Be16(b, 6)
		if segX2 == 0 || len(b) < 16+4*segX2 {
			return nil, false
		}
		endBase := 14
		startBase := endBase + segX2 + 2
		deltaBase := startBase + segX2
		rangeBase := deltaBase + segX2
		// A valid format-4 subtable partitions the BMP, so it never needs more
		// than ~65536 inner iterations. A hostile table with many segments each
		// spanning the whole range is O(segments x 65535) — seconds to minutes
		// of CPU on an untrusted font. Bound the total work (audit C10).
		for s := 0; s < segX2; s += 2 {
			end := Be16(b, endBase+s)
			start := Be16(b, startBase+s)
			delta := Be16(b, deltaBase+s)
			rangeOff := Be16(b, rangeBase+s)
			// The final segment of a conformant table is the sentinel
			// 0xFFFF..0xFFFF, which maps nothing. An inverted segment
			// (start > end) can only come from a malformed table; reading it
			// as if it ran from start upwards would walk into the following
			// segments' codes.
			if start == 0xFFFF || start > end {
				continue
			}
			// c is an int, so a segment ending at 0xFFFF stops on the ordinary
			// comparison — there is no 16-bit counter here to wrap. The wrap
			// guard this loop used to carry ("c != 0") therefore protected
			// nothing and, being false on entry, dropped every mapping of a
			// segment beginning at code 0 (audit C46).
			for c := start; c <= end; c++ {
				if !bud.charge(1, cmapWork) {
					return cmapResult(out), true // see budgetStop
				}
				var gid int
				if rangeOff == 0 {
					gid = (c + delta) & 0xFFFF
				} else {
					idx := rangeBase + s + rangeOff + 2*(c-start)
					g := Be16(b, idx)
					if g == 0 {
						continue
					}
					gid = (g + delta) & 0xFFFF
				}
				if gid != 0 {
					out[rune(c)] = gid
				}
			}
		}
	case 6:
		first := Be16(b, 6)
		count := Be16(b, 8)
		if len(b) < 10+2*count {
			return nil, false
		}
		// Character codes are 16-bit, so a first+count that runs past 0xFFFF
		// is malformed. Recording those entries would be worse than dropping
		// them: the caller narrows this map to uint16 for the (3,0) symbol
		// cmap, where code 0x10000 would alias onto code 0.
		for i := 0; i < count && first+i <= 0xFFFF; i++ {
			if !bud.charge(1, cmapWork) {
				return cmapResult(out), true // see budgetStop
			}
			if gid := Be16(b, 10+2*i); gid != 0 {
				out[rune(first+i)] = gid
			}
		}
	case 8:
		// Mixed 16-bit and 32-bit coverage: format(2) reserved(2) length(4)
		// language(4) is32[8192] nGroups(4), then the same groups as format 12.
		//
		// is32 is a bitmap saying, for each of the 65536 lead values, whether a
		// code beginning with it is one 16-bit code or the start of a 32-bit
		// one. That is a rule for decoding a *byte stream* into character
		// codes; this parser is handed the codes already, in the groups, so the
		// bitmap says nothing about the mapping and is skipped. The format is
		// deprecated and a font carrying one should be read exactly as its
		// groups are written.
		const is32Len = 8192
		if len(b) < 16+is32Len+4 {
			return nil, false
		}
		length := Be32(b, 4)
		if length < uint32(16+is32Len+4) || uint64(length) > uint64(len(b)) {
			return nil, false
		}
		b = b[:length]
		nGroups := Be32(b, 12+is32Len)
		if uint64(nGroups)*12 > uint64(len(b)-(16+is32Len)) {
			return nil, false
		}
		if cmapCoverageGroups(b, int(nGroups), 16+is32Len, true, bud, out) {
			return cmapResult(out), true // see budgetStop
		}
	case 10:
		// Trimmed array, the 32-bit twin of format 6: format(2) reserved(2)
		// length(4) language(4) startCharCode(4) numChars(4), then numChars
		// glyph indices. numChars is checked against the bytes that are
		// actually there, so the run is bounded by the subtable's own size the
		// way format 6 and format 0 are — and charged all the same, since the
		// budget is the font's.
		if len(b) < 20 {
			return nil, false
		}
		length := Be32(b, 4)
		if length < 20 || uint64(length) > uint64(len(b)) {
			return nil, false
		}
		b = b[:length]
		first := Be32(b, 12)
		count := Be32(b, 16)
		if uint64(count)*2 > uint64(len(b)-20) {
			return nil, false
		}
		for i := 0; i < int(count); i++ {
			// first+i in 64 bits: both are uint32 and their sum need not be.
			c := uint64(first) + uint64(i)
			if c > unicodeMaxRune {
				// The rest run past Unicode too, since c only climbs.
				break
			}
			if !bud.charge(1, cmapWork) {
				return cmapResult(out), true // see budgetStop
			}
			if gid := Be16(b, 20+2*i); gid != 0 {
				out[rune(c)] = gid
			}
		}
	case 12, 13:
		// Segmented coverage (12) and many-to-one range mappings (13). The two
		// share a header — format(2) reserved(2) length(4) language(4)
		// nGroups(4) — and a group array, and differ only in what a group's
		// startGlyphID means; see cmapCoverageGroups. These are the formats that
		// reach past the BMP, so their keys really can exceed 0xFFFF.
		//
		// Groups are required to be sorted and non-overlapping. Neither is
		// enforced: a table that merely lists them out of order is still
		// unambiguous except where groups overlap, and rejecting it outright
		// would return nil for a font whose mappings are perfectly readable —
		// the caller reads nil as "unknown" and stops checking. Overlaps resolve
		// to the last group written, as elsewhere in this parser.
		if len(b) < 16 {
			return nil, false
		}
		length := Be32(b, 4)
		if length < 16 || uint64(length) > uint64(len(b)) {
			return nil, false
		}
		b = b[:length]
		nGroups := Be32(b, 12)
		// nGroups is a uint32: a table may claim four billion groups it does
		// not carry. Trust the bytes, not the count.
		if uint64(nGroups)*12 > uint64(len(b)-16) {
			return nil, false
		}
		// The budget: a single group may span the whole of Unicode (0x110000
		// codes) and there may be many of them, so an unbudgeted expansion is an
		// unbounded allocation driven by the font. The cap is the format-4 one:
		// an sfnt has at most 65535 glyphs, so no honest font needs to map
		// anywhere near 2^18 code points, and the resulting map stays a few
		// megabytes at worst.
		sequential := Be16(b, 0) == 12
		if cmapCoverageGroups(b, int(nGroups), 16, sequential, bud, out) {
			return cmapResult(out), true // see budgetStop
		}
	default:
		// Formats 2 and 14 are not parsed; see the note on parseCmapSubtableUnder.
		return nil, false
	}
	return cmapResult(out), false
}

// --- CFF ---

type cffIndex struct {
	items [][]byte
}

// parseCFFIndex reads the INDEX at off, charging one unit of bud per entry.
//
// An INDEX is read once per offset that names it, and a CFF names most of its
// INDEXes once — but a Subrs INDEX is named by a Private DICT, and a CID-keyed
// font has as many of those as it has Font DICTs, each free to name any offset.
// Sixty-five thousand Font DICTs naming sixty-five thousand overlapping INDEXes
// of sixty-five thousand entries is four billion entries out of a few hundred
// kilobytes, so the entries are charged, not the INDEXes.
func parseCFFIndex(b []byte, off int, bud *Budget) (cffIndex, int) {
	var idx cffIndex
	if off+2 > len(b) {
		return idx, len(b)
	}
	count := Be16(b, off)
	if count == 0 {
		return idx, off + 2
	}
	if off+3 > len(b) {
		return idx, len(b)
	}
	offSize := int(b[off+2])
	if offSize < 1 || offSize > 4 {
		return idx, len(b)
	}
	offArray := off + 3
	readOff := func(i int) int {
		p := offArray + i*offSize
		if p+offSize > len(b) {
			return -1
		}
		v := 0
		for k := 0; k < offSize; k++ {
			v = v<<8 | int(b[p+k])
		}
		return v
	}
	dataStart := offArray + (count+1)*offSize - 1
	for i := 0; i < count; i++ {
		if !bud.charge(1, "a CFF INDEX") {
			return idx, len(b)
		}
		s, e := readOff(i), readOff(i+1)
		if s < 1 || e < s || dataStart+e > len(b) {
			return idx, len(b)
		}
		idx.items = append(idx.items, b[dataStart+s:dataStart+e])
	}
	end := dataStart + readOff(count)
	if end > len(b) || end < 0 {
		end = len(b)
	}
	return idx, end
}

// parseCFFDict extracts operator → operands from a CFF DICT.
func parseCFFDict(b []byte) map[int][]float64 {
	out := make(map[int][]float64)
	var operands []float64
	i := 0
	for i < len(b) {
		v := int(b[i])
		switch {
		case v <= 21: // operator
			op := v
			i++
			if v == 12 && i < len(b) {
				op = 1200 + int(b[i])
				i++
			}
			out[op] = append([]float64(nil), operands...)
			operands = operands[:0]
		case v == 28:
			if i+3 > len(b) {
				return out
			}
			operands = append(operands, float64(int16(binary.BigEndian.Uint16(b[i+1:]))))
			i += 3
		case v == 29:
			if i+5 > len(b) {
				return out
			}
			operands = append(operands, float64(int32(binary.BigEndian.Uint32(b[i+1:]))))
			i += 5
		case v == 30: // real number (BCD)
			i++
			var sb strings.Builder
			for i < len(b) {
				hi, lo := b[i]>>4, b[i]&0xF
				i++
				done := false
				for _, nib := range []byte{hi, lo} {
					switch {
					case nib <= 9:
						sb.WriteByte('0' + nib)
					case nib == 0xA:
						sb.WriteByte('.')
					case nib == 0xB:
						sb.WriteByte('E')
					case nib == 0xC:
						sb.WriteString("E-")
					case nib == 0xE:
						sb.WriteByte('-')
					case nib == 0xF:
						done = true
					}
					if done {
						break
					}
				}
				if done {
					break
				}
			}
			var f float64
			parseBCDReal(sb.String(), &f)
			operands = append(operands, f)
		case v >= 32 && v <= 246:
			operands = append(operands, float64(v-139))
			i++
		case v >= 247 && v <= 250:
			if i+2 > len(b) {
				return out
			}
			operands = append(operands, float64((v-247)*256+int(b[i+1])+108))
			i += 2
		case v >= 251 && v <= 254:
			if i+2 > len(b) {
				return out
			}
			operands = append(operands, float64(-(v-251)*256-int(b[i+1])-108))
			i += 2
		default:
			i++
		}
	}
	return out
}

// cffPrivate is what a Private DICT says about reading the charstrings it
// covers: the two width defaults, and the local subroutines they may call.
type cffPrivate struct {
	def, nom float64
	subrs    cffIndex
}

// cffPrivates reads the Private DICTs of one CFF, and the Subrs INDEXes they
// name, each once.
//
// A Private DICT is named by an offset and a size, and nothing stops a font
// naming one many times: every Font DICT in a CID-keyed font's FDArray may name
// the same Private DICT, and every Private DICT the same Subrs. Read once per
// reference, a Private DICT of 64 KB named by 65535 Font DICTs is four gigabytes
// of DICT parsing from a few hundred kilobytes of font. So what has been read is
// kept by where it is, and what has not is charged to the budget — the bytes of
// a DICT and the entries of an INDEX — since distinct references need not be to
// distinct bytes: two DICTs a byte apart overlap by all but one.
type cffPrivates struct {
	data  []byte
	bud   *Budget
	dicts map[[2]int]cffPrivate
	subrs map[int]cffIndex
}

func newCFFPrivates(data []byte, bud *Budget) *cffPrivates {
	return &cffPrivates{data: data, bud: bud, dicts: map[[2]int]cffPrivate{}, subrs: map[int]cffIndex{}}
}

// read reads a Private DICT named by a top or Font DICT's operator 18, whose
// two operands are its size and its offset in that order.
//
// The order is worth stating because no test can catch getting it wrong. Reading
// them the other way round starts the DICT early and ends it in exactly the same
// place — size plus offset is offset plus size — so the DICT that was wanted is
// still inside the range, still last, and CFF takes the last operator. The
// values come back right from the wrong bytes. What saves this is the
// specification and not the suite.
func (c *cffPrivates) read(priv []float64) (p cffPrivate) {
	data := c.data
	if len(priv) != 2 {
		return p
	}
	pOff, offOK := dictOffset(priv[1])
	pSize, sizeOK := dictOffset(priv[0])
	// Subtraction, not addition: pOff+pSize in a font's own numbers is a sum
	// that can leave the address space, and one that wrapped negative passed
	// this check and sliced. Both are already known to be non-negative and
	// inside a four-byte offset, so the difference cannot wrap.
	if !offOK || !sizeOK || pOff <= 0 || pOff > len(data) || pSize > len(data)-pOff {
		return p
	}
	key := [2]int{pOff, pSize}
	if p, ok := c.dicts[key]; ok {
		return p
	}
	if !c.bud.charge(pSize, "a CFF Private DICT") {
		return p
	}
	pd := parseCFFDict(data[pOff : pOff+pSize])
	if v, ok := pd[20]; ok && len(v) == 1 {
		p.def = v[0]
	}
	if v, ok := pd[21]; ok && len(v) == 1 {
		p.nom = v[0]
	}
	if v, ok := pd[19]; ok && len(v) == 1 { // Subrs, relative to the Private DICT
		if rel, ok := dictOffset(v[0]); ok {
			if so := pOff + rel; so > 0 && so < len(data) {
				subrs, seen := c.subrs[so]
				if !seen {
					subrs, _ = parseCFFIndex(data, so, c.bud)
					c.subrs[so] = subrs
				}
				p.subrs = subrs
			}
		}
	}
	c.dicts[key] = p
	return p
}

// parseCFFFDs reads a CID-keyed font's FDArray and FDSelect: the Private DICTs
// its glyphs are divided between, and which glyph belongs to which.
//
// A CID-keyed font is usually several fonts merged — Latin, kana and han in one
// file, each hinted on its own terms — and the Font DICTs are what is left of
// that. So the width defaults differ between them, and reading only the first
// would be as wrong as reading none for every glyph outside it.
//
// Returns nil for both when the font is not CID-keyed or says nothing useful,
// which leaves the caller on the top DICT's own Private DICT.
func parseCFFFDs(privates *cffPrivates, top map[int][]float64, numGlyphs int, isCID bool) ([]int, []cffPrivate) {
	if !isCID || numGlyphs == 0 {
		return nil, nil
	}
	data := privates.data
	fdaOff, haveFDA := dictInt(top, 1236) // FDArray
	fdsOff, haveFDS := dictInt(top, 1237) // FDSelect
	if !haveFDA || fdaOff <= 0 || fdaOff >= len(data) {
		return nil, nil
	}
	fontDicts, _ := parseCFFIndex(data, fdaOff, privates.bud)
	if len(fontDicts.items) == 0 {
		return nil, nil
	}
	privs := make([]cffPrivate, len(fontDicts.items))
	for i, fd := range fontDicts.items {
		d := parseCFFDict(fd)
		if priv, ok := d[18]; ok {
			privs[i] = privates.read(priv)
		}
	}

	// FDSelect: which Font DICT each glyph belongs to. Absent, or unreadable,
	// every glyph takes the first — which is what a single-FD font means and is
	// the least wrong answer for a malformed one.
	fdOf := make([]int, numGlyphs)
	if !haveFDS || fdsOff <= 0 || fdsOff >= len(data) {
		return fdOf, privs
	}
	b := data[fdsOff:]
	switch b[0] {
	case 0:
		// One byte per glyph, in glyph order.
		if !privates.bud.charge(numGlyphs, "the CFF FDSelect") {
			return fdOf, privs
		}
		for g := 0; g < numGlyphs && 1+g < len(b); g++ {
			fdOf[g] = int(b[1+g])
		}
	case 3:
		// Ranges: a first glyph and its FD, repeated, closed by a sentinel that
		// gives the glyph after the last range rather than a range of its own.
		//
		// The ranges have to be in order of their first glyph — the
		// specification says so, and it is what makes them a partition — and
		// reading stops at the first that is not. Unordered, each range ran
		// from its own first glyph to the next range's, and nothing stopped
		// that from being the whole font every other time: alternating firsts
		// of 0 and n made 65535 ranges of 32768 glyphs each, two billion
		// writes from 200 KB (audit C127). In order, the ranges are disjoint
		// and the writes come to the glyph count at most, whatever the font
		// says. The glyphs of the ranges that came before the fault keep their
		// FD and the rest keep the first, which is the answer for a font whose
		// FDSelect says nothing about them.
		//
		// The writes are charged to the budget, a unit a glyph. In order they
		// come to the glyph count, so a real font never notices; what the
		// charge is for is that the order is then something a test can count
		// rather than time. Timed, a read that stops at the second range costs
		// forty microseconds, and what the race detector does to allocating
		// the answer put a linear curve at fourteen for four.
		if len(b) < 5 {
			break
		}
		nRanges := Be16(b, 1)
		prev := -1
		for r := 0; r < nRanges; r++ {
			p := 3 + r*3
			if p+5 > len(b) {
				break
			}
			first, fd, next := Be16(b, p), int(b[p+2]), Be16(b, p+3)
			if first <= prev {
				break
			}
			prev = first
			if end := min(next, numGlyphs); end > first &&
				!privates.bud.charge(end-first, "the CFF FDSelect") {
				break
			}
			for g := first; g < next && g < numGlyphs; g++ {
				fdOf[g] = fd
			}
		}
	}
	return fdOf, privs
}

// DefaultCFFWork is the budget ParseCFF reads a font under, in the units Budget
// counts.
//
// It is about thirty times what the most expensive font this is tested against
// spends: Noto Sans SC, 65,535 CID-keyed glyphs, reads in about half a million
// units, INDEX entries and every width's charstring steps included. A font past
// it is asking for its widths to be found by a walk no real font needs.
const DefaultCFFWork = 1 << 24

// ParseCFF parses a bare CFF font (FontFile3 /Type1C or /CIDFontType0C, or
// the CFF table of an OpenType font), widths included, under a budget of
// DefaultCFFWork.
//
// It returns nil for a font it cannot read, and a font whose reading ran past
// the budget is one of those: the widths are found by interpreting the
// charstrings, and one whose interpretation was cut short would come back as
// the default width, which looks like an answer.
func ParseCFF(data []byte) *Program {
	return parseCFF(data, NewBudget(DefaultCFFWork), true)
}

// ParseCFFGlyphs reads what ParseCFF reads except the widths: the glyph count,
// the charset, which glyph belongs to which Font DICT, and the character
// collection. The work is charged to b, and it returns nil where ParseCFF
// would, including when b runs out.
//
// # Why the widths are left out
//
// They are the whole of the cost and none of what a shaper reads. A width is
// found by interpreting a glyph's charstring, subroutines and all, up to its
// first stack-clearing operator — once per glyph in the font — and an sfnt
// carries its advances in hmtx, which is where shape reads them. A 700-byte font
// whose subroutines fan out made that interpretation take six seconds inside
// shape.Load for widths nothing would look at (audit C4). The budget bounds the
// interpretation; not doing it is better than bounding it.
func ParseCFFGlyphs(data []byte, b *Budget) *Program {
	return parseCFF(data, b, false)
}

func parseCFF(data []byte, b *Budget, withWidths bool) *Program {
	if len(data) < 4 || data[0] != 1 {
		return nil
	}
	hdrSize := int(data[2])
	_, afterNames := parseCFFIndex(data, hdrSize, b)
	topDicts, afterTop := parseCFFIndex(data, afterNames, b)
	stringsIdx, afterStrings := parseCFFIndex(data, afterTop, b)
	// The Global Subr INDEX, which sits after the strings and had been skipped.
	// A charstring may reach its width through one of these — see
	// type2CharstringWidth.
	globalSubrs, _ := parseCFFIndex(data, afterStrings, b)
	if len(topDicts.items) == 0 {
		return nil
	}
	top := parseCFFDict(topDicts.items[0])

	fp := &Program{}
	// FontMatrix (top DICT op 12 7) x-scale, default 0.001; normalise
	// charstring widths to 1/1000 text-space units.
	scale := 1.0
	if fm, ok := top[1207]; ok && len(fm) >= 1 && fm[0] != 0 {
		scale = fm[0] * 1000
	}
	csOff, ok := dictInt(top, 17)
	if !ok || csOff <= 0 || csOff >= len(data) {
		return nil
	}
	charStrings, _ := parseCFFIndex(data, csOff, b)
	fp.NumGlyphs = len(charStrings.items)

	ros, isCID := top[1230] // ROS
	if isCID && len(ros) == 3 {
		// Two SIDs and a number. The SIDs name the collection — "Adobe" and
		// "Japan1" — and resolve through the same string index glyph names do,
		// so a custom collection carries its own strings rather than a
		// predefined SID.
		//
		// All three or none of them. A supplement is a version and cannot be
		// negative, and a SID that resolves to nothing names no collection, so
		// a font that gets any part of this wrong has not said which numbering
		// its CIDs are in — and half an answer is the shape a caller writes
		// into a /CIDSystemInfo without noticing.
		reg := cffSIDName(int(ros[0]), stringsIdx)
		ord := cffSIDName(int(ros[1]), stringsIdx)
		if sup := int(ros[2]); reg != "" && ord != "" && sup >= 0 {
			fp.Registry, fp.Ordering, fp.Supplement = reg, ord, sup
		}
	}
	// Private DICT: nominal/default widths.
	privates := newCFFPrivates(data, b)
	var topPriv cffPrivate
	if priv, ok := top[18]; ok && len(priv) == 2 {
		topPriv = privates.read(priv)
	}

	// A CID-keyed font has no top DICT Private entry at all. Each glyph's
	// Private DICT is reached through FDSelect, which says which Font DICT the
	// glyph belongs to, and FDArray, which holds them — so the two width
	// defaults every charstring is read against are per glyph rather than per
	// font.
	//
	// Looking for one Private DICT and not finding it is not a small error. A
	// Type 2 charstring carries its width only when that width differs from
	// defaultWidthX, so the common case is that it carries nothing and the
	// default *is* the answer. With no Private DICT the default is zero, and a
	// face comes back with almost every glyph zero units wide: 17,707 of Noto
	// Sans JP's 17,936 before this read the FDs.
	fdOf, fdPriv := parseCFFFDs(privates, top, fp.NumGlyphs, isCID)

	// charset: GID → SID (names) or CID.
	// A charset operand that is not an offset at all is read as no charset:
	// zero is the predefined ISOAdobe, which is the reading a font that says
	// nothing gets.
	charsetOff, _ := dictInt(top, 15)
	gidToSID := make([]int, fp.NumGlyphs)
	if fp.NumGlyphs > 0 {
		gidToSID[0] = 0 // .notdef
	}
	switch {
	case charsetOff >= 0 && charsetOff <= 2 && isCID:
		// A predefined charset on a CID-keyed font. The three are tables of
		// glyph *names*, and a CID font's charset holds CIDs, so none of them
		// means anything here; identity is the reading every consumer already
		// has, and it is what "Identity" ordering asks for.
		for g := 1; g < fp.NumGlyphs; g++ {
			gidToSID[g] = g
		}
	case charsetOff >= 0 && charsetOff <= 2:
		set, _ := cffPredefinedCharset(charsetOff)
		for g := 1; g < fp.NumGlyphs; g++ {
			switch {
			case set == nil: // ISOAdobe, the identity
				if g < isoAdobeCharsetLen {
					gidToSID[g] = g
				} else {
					gidToSID[g] = -1
				}
			case g < len(set):
				gidToSID[g] = set[g]
			default:
				// Past the end of the charset. A predefined charset is a fixed
				// list, so a font with more glyphs than it holds has named none
				// of them — and the identity would give those glyphs the names
				// of unrelated standard strings, which is how a width ends up
				// recorded against a glyph the font does not have.
				gidToSID[g] = -1
			}
		}
	default:
		if charsetOff > 0 && charsetOff < len(data) {
			b := data[charsetOff:]
			switch b[0] {
			case 0:
				for g := 1; g < fp.NumGlyphs; g++ {
					if 1+2*g > len(b) {
						break
					}
					gidToSID[g] = Be16(b, 1+2*(g-1))
				}
			case 1, 2:
				g := 1
				p := 1
				step := 3
				if b[0] == 2 {
					step = 4
				}
				for g < fp.NumGlyphs && p+step <= len(b) {
					first := Be16(b, p)
					var count int
					if b[0] == 1 {
						count = int(b[p+2])
					} else {
						count = Be16(b, p+2)
					}
					for k := 0; k <= count && g < fp.NumGlyphs; k++ {
						gidToSID[g] = first + k
						g++
					}
					p += step
				}
			}
		}
	}

	// Charstring widths (Type 2: optional leading width operand), one per
	// glyph, found once. They used to be found once for each map a glyph
	// appears in, which is every glyph twice.
	//
	// The two defaults come from the glyph's own Private DICT, which for a
	// CID-keyed font is whichever of the FDArray's its FDSelect names.
	var widths []float64
	if withWidths {
		widths = make([]float64, fp.NumGlyphs)
		for g, cs := range charStrings.items {
			p := topPriv
			if fdOf != nil && g < len(fdOf) {
				if i := fdOf[g]; i >= 0 && i < len(fdPriv) {
					// The local subroutines come from the same Private DICT the
					// two width defaults do, which for a CID-keyed font is the
					// one its FDSelect names — so a glyph reading its width
					// through a subr must read it through *that* FD's, not the
					// top DICT's.
					p = fdPriv[i]
				}
			}
			if w, has := type2CharstringWidth(cs, p.subrs, globalSubrs, b); has {
				widths[g] = (p.nom + w) * scale
			} else {
				widths[g] = p.def * scale
			}
		}
	}
	// Whatever ran the budget out — the charstrings, a Private DICT, an INDEX
	// — left something above unread, and this has no way to say which field
	// is short. A width that was not found reads as the default, which looks
	// like an answer; so nothing is returned rather than a font that is part
	// right.
	if b.Exhausted() {
		return nil
	}

	if isCID {
		fp.CIDGIDs = make(map[int]bool, fp.NumGlyphs)
		// Copied into a slice of its own length rather than appended onto nil,
		// which for a font with no charstrings gives nil back and makes the one
		// field that says whether this font is CID-keyed say that it is not.
		fp.GIDToCID = make([]int, fp.NumGlyphs)
		copy(fp.GIDToCID, gidToSID)
		fp.GIDToFD = append([]int(nil), fdOf...)
		if withWidths {
			fp.WidthByCID = make(map[int]float64, fp.NumGlyphs)
		}
		for g := 0; g < fp.NumGlyphs; g++ {
			cid := gidToSID[g]
			fp.CIDGIDs[cid] = true
			if withWidths {
				fp.WidthByCID[cid] = widths[g]
			}
		}
	} else {
		fp.GlyphNames = make(map[string]bool, fp.NumGlyphs)
		if withWidths {
			fp.WidthByName = make(map[string]float64, fp.NumGlyphs)
		}
		for g := 0; g < fp.NumGlyphs; g++ {
			name := cffSIDName(gidToSID[g], stringsIdx)
			// A glyph the charset does not name is not a glyph named "". It is
			// left out, as parseType1 leaves its unnamed ones out, so that a
			// consumer asking "does this font define X" is not answered by an
			// entry that stands for every unnamed glyph at once — and so that
			// WidthByName does not hold one glyph's width under a key the next
			// unnamed glyph overwrites. WidthByGID still has every glyph.
			if name == "" {
				continue
			}
			fp.GlyphNames[name] = true
			if withWidths {
				fp.WidthByName[name] = widths[g]
			}
		}
	}
	fp.WidthByGID = widths
	return fp
}

func dictInt(d map[int][]float64, op int) (int, bool) {
	if v, ok := d[op]; ok && len(v) >= 1 {
		return dictOffset(v[len(v)-1])
	}
	return 0, false
}

// dictOffset turns a DICT operand into an offset or a length, and says whether
// it is one.
//
// A DICT operand is a float64 because CFF's real-number operands are, and a
// font writes whatever it likes into one: "9.2E18" is a legal real. Converting
// that to an int is not defined in Go — the value does not fit — and what came
// out was a number that passed a bound check by being negative and was then
// used to slice. So the conversion is the check: a whole number, not negative,
// and inside what a four-byte offset can name.
func dictOffset(v float64) (int, bool) {
	if math.IsNaN(v) || v < 0 || v > math.MaxInt32 || v != math.Trunc(v) {
		return 0, false
	}
	return int(v), true
}

// type2CharstringWidth reports the optional leading width delta of a Type 2
// charstring: present when the operand count before the first stack-clearing
// operator exceeds that operator's expected arguments.
//
// # Why it interprets rather than scans
//
// The width is whatever is on the stack when the first stack-clearing operator
// arrives, and a charstring may put it there from inside a subroutine. Both
// halves of that are common: a font compressed by a subsetter routinely factors
// the hints and the opening move of every glyph into a shared subr, so the
// operator that decides the width is not in the charstring at all.
//
// Scanning for the first operator therefore answered "no width here" for a
// callsubr — and "no width" is not a refusal, it is the Private DICT's
// defaultWidthX. Every glyph in such a font came back the same width. That is
// the shape the FD Private DICTs had, where 17,707 of Noto Sans JP's 17,936
// widths read as zero: a number that silently defaults looks like an answer.
//
// So this follows callsubr and callgsubr, keeping the operand stack across the
// call as the charstring does, and stops at the first stack-clearing operator
// wherever it is reached.
//
// # What bounds it
//
// Two things, and the second is the one that matters.
//
// A subroutine can call a subroutine, and a hostile font can make that a cycle.
// The specification bounds the nesting at 10 and this bounds it at the same,
// which costs nothing legitimate — no real font nests deeply, because each level
// is a byte of overhead it exists to avoid — and turns a font that would spin
// into one that reports no width.
//
// Depth is not work, though. A subroutine that calls the next one k times, nine
// levels down, is k^8 calls inside a depth of nine, and as long as none of them
// reaches a stack-clearing operator the walk goes on: 700 bytes of font held
// shape.Load for six seconds at k=10 and would hold it for half an hour at k=20
// (audit C4). So every step — each operand, operator, call and return — is
// charged to the font's budget, and a walk that finds it empty reports no width
// with the budget marked, which ParseCFF turns into a refusal of the font. The
// budget is shared by every glyph, so it bounds the font and not one charstring:
// a font whose every glyph calls the same expensive subroutine pays for each.
func type2CharstringWidth(cs []byte, local, global cffIndex, bud *Budget) (float64, bool) {
	var operands []float64
	// The call stack: what to come back to when a subroutine returns. The
	// charstring itself is the bottom of it.
	type frame struct {
		code []byte
		at   int
	}
	stack := []frame{{code: cs}}

	for len(stack) > 0 {
		if !bud.charge(1, "the CFF charstrings") {
			return 0, false
		}
		f := &stack[len(stack)-1]
		if f.at >= len(f.code) {
			// Ran off the end of a subroutine without a return, which is
			// malformed; treat it as one rather than as a reason to stop.
			stack = stack[:len(stack)-1]
			continue
		}
		v := int(f.code[f.at])
		switch {
		case v == 28:
			if f.at+3 > len(f.code) {
				return 0, false
			}
			operands = append(operands, float64(int16(binary.BigEndian.Uint16(f.code[f.at+1:]))))
			f.at += 3
		case v == 255:
			if f.at+5 > len(f.code) {
				return 0, false
			}
			operands = append(operands, float64(int32(binary.BigEndian.Uint32(f.code[f.at+1:])))/65536)
			f.at += 5
		case v >= 32 && v <= 246:
			operands = append(operands, float64(v-139))
			f.at++
		case v >= 247 && v <= 250:
			if f.at+2 > len(f.code) {
				return 0, false
			}
			operands = append(operands, float64((v-247)*256+int(f.code[f.at+1])+108))
			f.at += 2
		case v >= 251 && v <= 254:
			if f.at+2 > len(f.code) {
				return 0, false
			}
			operands = append(operands, float64(-(v-251)*256-int(f.code[f.at+1])-108))
			f.at += 2

		case v == 10 || v == 29: // callsubr, callgsubr
			idx := local
			if v == 29 {
				idx = global
			}
			if len(operands) == 0 {
				return 0, false
			}
			n := int(operands[len(operands)-1]) + subrBias(len(idx.items))
			operands = operands[:len(operands)-1]
			f.at++
			if n < 0 || n >= len(idx.items) {
				return 0, false
			}
			if len(stack) >= maxSubrDepth {
				return 0, false
			}
			stack = append(stack, frame{code: idx.items[n]})

		case v == 11: // return
			stack = stack[:len(stack)-1]

		default:
			// The first stack-clearing operator, wherever it was reached.
			expected := -1
			switch v {
			case 1, 3, 18, 23: // hstem vstem hstemhm vstemhm
				expected = len(operands) &^ 1 // even
			case 19, 20: // hintmask cntrmask
				expected = len(operands) &^ 1
			case 21: // rmoveto
				expected = 2
			case 22, 4: // hmoveto vmoveto
				expected = 1
			case 14: // endchar
				// endchar takes no arguments, or four in the deprecated seac
				// form — adx ady bchar achar — that builds an accented glyph out
				// of two others. So a width is there when there is one operand
				// or five, and not otherwise: counting four as "more than none"
				// read a seac glyph's adx as its width (audit C196). FreeType
				// draws the line in the same place.
				if n := len(operands); n == 1 || n == 5 {
					return operands[0], true
				}
				return 0, false
			default:
				return 0, false // not a stack-clearing operator: no width info
			}
			if len(operands) > expected {
				return operands[0], true
			}
			return 0, false
		}
	}
	return 0, false
}

// maxSubrDepth is the nesting the Type 2 specification allows, and the bound a
// cyclic subroutine meets instead of spinning.
const maxSubrDepth = 10

// CFFSubrBias is subrBias, for the subsetter's seac walk. See subrBias.
func CFFSubrBias(n int) int { return subrBias(n) }

// subrBias is the number a Type 2 subroutine index is offset by, which the
// specification makes depend on how many subroutines there are so that the
// commonest indices encode in one byte.
func subrBias(n int) int {
	switch {
	case n < 1240:
		return 107
	case n < 33900:
		return 1131
	}
	return 32768
}

// cffStandardStrings is the tail-safe accessor for the 391 standard strings.
//
// A SID read from a charset is two unsigned bytes, but one read from a DICT is
// a signed operand — the one-byte form alone covers -107 to 107 — and a real
// operand converts to whatever int() makes of it, which for a value past the
// range is platform-defined and on amd64 is large and negative. So the low
// bound is checked as well as the high one: this is reached with bytes out of a
// font file, and a font file is not a promise.
func cffSIDName(sid int, idx cffIndex) string {
	if sid < 0 {
		return ""
	}
	if sid < len(cffStandardStrings) {
		return cffStandardStrings[sid]
	}
	i := sid - len(cffStandardStrings)
	if i < len(idx.items) {
		return string(idx.items[i])
	}
	return ""
}

// parseBCDReal reads the text a CFF DICT's BCD real spells — the digits, '.',
// "E", "E-" and '-' its nibbles stand for — into f.
//
// It is not strconv.ParseFloat and does not pretend to be: it takes what the
// nibbles spelled, whatever order they came in, so "1.2.3" is 1.23 and a
// second "E" is read as more of the exponent, where a strict reader would
// refuse. A malformed real is a font's own mistake in a width nothing in this
// engine reads, and the exponent bound below is the part that matters. It was
// exported, as ParseFloat, under a name promising strconv's contract to callers
// that had none; and it was also what read a Type 1 /FontMatrix, where the
// token is PostScript text and "1e-3" came out as 13 — see
// extractType1FontMatrix, which uses strconv.
func parseBCDReal(s string, f *float64) {
	var v float64
	var neg bool
	i := 0
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	}
	frac := 0.0
	scale := 0.1
	inFrac := false
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			if inFrac {
				frac += float64(c-'0') * scale
				scale /= 10
			} else {
				v = v*10 + float64(c-'0')
			}
		case c == '.':
			inFrac = true
		case c == 'E':
			// exponent: parse remainder as int
			//
			// Nothing bounds how many digits a BCD real spends on its
			// exponent, and the exponent is applied one multiplication at a
			// time — so `1E99999999` is a font stalling the parser for as long
			// as it likes, out of nine nibbles. Enough digits and the
			// accumulation overflows int instead, which turns a huge number
			// into a small one with no sign that it happened.
			//
			// 700 is past every exponent that can change an answer. A float64
			// spans roughly 1e-324 to 1e308, so whatever the value, 633 steps
			// have already carried it to infinity or to zero and further steps
			// leave it there. Clamping here is not an approximation of the
			// unbounded loop; it is the same result, reached.
			const maxExp = 700
			exp := 0
			eneg := false
			j := i + 1
			if j < len(s) && s[j] == '-' {
				eneg = true
				j++
			}
			for ; j < len(s); j++ {
				if s[j] >= '0' && s[j] <= '9' && exp <= maxExp {
					exp = exp*10 + int(s[j]-'0')
				}
			}
			if exp > maxExp {
				exp = maxExp
			}
			total := v + frac
			for k := 0; k < exp; k++ {
				if eneg {
					total /= 10
				} else {
					total *= 10
				}
			}
			if neg {
				total = -total
			}
			*f = total
			return
		}
	}
	total := v + frac
	if neg {
		total = -total
	}
	*f = total
}

// --- Type 1 ---

// parseType1 parses a Type 1 font program (FontFile): the eexec-encrypted
// private portion holds the CharStrings dictionary with glyph names and
// hsbw/sbw widths.
func parseType1(data []byte) *Program {
	// PFB segmented format: 0x80 0x01/0x02 length(4, little-endian).
	if len(data) > 6 && data[0] == 0x80 {
		var joined []byte
		i := 0
		for i+6 <= len(data) && data[i] == 0x80 {
			t := data[i+1]
			l := int(binary.LittleEndian.Uint32(data[i+2:]))
			if t == 3 || i+6+l > len(data) {
				break
			}
			joined = append(joined, data[i+6:i+6+l]...)
			i += 6 + l
		}
		data = joined
	}

	// FontMatrix (cleartext, before eexec) scales charstring units to text
	// space; default is 0.001 (1000-unit glyph space).
	scale := 1.0
	if fm := extractType1FontMatrix(data); fm != 0 {
		scale = fm * 1000
	}

	idx := strings.Index(string(data), "eexec")
	if idx < 0 {
		return nil
	}
	enc := data[idx+len("eexec"):]
	// Skip EOL whitespace after eexec.
	for len(enc) > 0 && (enc[0] == '\r' || enc[0] == '\n' || enc[0] == ' ' || enc[0] == '\t') {
		enc = enc[1:]
	}
	// Hex form detection: first 4 bytes all hex digits.
	isHexDigit := func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	}
	if len(enc) >= 4 && isHexDigit(enc[0]) && isHexDigit(enc[1]) && isHexDigit(enc[2]) && isHexDigit(enc[3]) {
		enc = decodeHexBytes(enc)
	}
	priv := eexecDecrypt(enc, 55665, 4)
	text := string(priv)

	lenIV := 4
	if li := strings.Index(text, "/lenIV"); li >= 0 {
		// sscanInt skips the leading space after "/lenIV"; use its value
		// directly (parseLeadingInt would start on the space and return 0).
		if ok, val := sscanInt(text[li+6:]); ok {
			lenIV = val
		}
	}

	fp := &Program{
		GlyphNames:  make(map[string]bool),
		WidthByName: make(map[string]float64),
	}
	// CharStrings entries: /name len RD ...bytes... ND
	pos := strings.Index(text, "/CharStrings")
	if pos < 0 {
		return nil
	}
	rest := priv[pos:]
	for {
		s := indexAfter(rest, '/')
		if s < 0 {
			break
		}
		rest = rest[s:]
		nameEnd := 0
		for nameEnd < len(rest) && !isWhitespace(rest[nameEnd]) && rest[nameEnd] != '(' && rest[nameEnd] != '{' {
			nameEnd++
		}
		name := string(rest[:nameEnd])
		rest = rest[nameEnd:]
		// Expect: <len> RD/-| <bytes> ND/|-
		var csLen int
		j := 0
		for j < len(rest) && isWhitespace(rest[j]) {
			j++
		}
		numStart := j
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		if j == numStart {
			if name == "CharStrings" || strings.HasPrefix(name, "Private") {
				continue
			}
			if strings.HasPrefix(name, "end") {
				break
			}
			continue
		}
		var lenOK bool
		if csLen, lenOK = parseLeadingInt(string(rest[numStart:j])); !lenOK {
			break
		}
		for j < len(rest) && isWhitespace(rest[j]) {
			j++
		}
		// The RD token, which is the operator that reads the bytes.
		tokStart := j
		for j < len(rest) && !isWhitespace(rest[j]) {
			j++
		}
		if j >= len(rest) || j == tokStart {
			break
		}
		// A name followed by a number is not yet a charstring entry, and every
		// real Type 1 font has one that is not: "/CharStrings 228 dict dup
		// begin" opens the dictionary the entries go in. Reading that as an
		// entry registered "CharStrings" as a glyph and swallowed the next 228
		// bytes, which is every glyph after it. The operator is what tells the
		// two apart, and it is one of two spellings by convention — the font
		// defines it, and defines it as RD or as -|.
		if tok := string(rest[tokStart:j]); tok != "RD" && tok != "-|" {
			rest = rest[j:]
			continue
		}
		j++ // single space after RD
		// Subtraction, so that a length the file chose cannot carry the sum
		// past what an int holds.
		if j > len(rest) || csLen > len(rest)-j {
			break
		}
		cs := eexecDecrypt(rest[j:j+csLen], 4330, lenIV)
		if w, ok := type1CharstringWidth(cs); ok {
			fp.WidthByName[name] = w * scale
		}
		fp.GlyphNames[name] = true
		rest = rest[j+csLen:]
		if type1CharStringsEnd(rest) {
			break
		}
	}
	delete(fp.GlyphNames, "")
	return fp
}

// type1CharStringsEnd reports whether the bytes following a CharStrings entry's
// charstring data close the dictionary. A Type 1 CharStrings dictionary
// (Adobe's Type 1 Font Format, 10.3) ends with a standalone "end" token after
// the last entry's ND (or |-) token:
//
//	/A 45 RD ~~~~~ ND
//	end
//
// so the terminator is a PostScript token in the byte stream, never a glyph
// name. Testing the glyph name for "end" instead truncated the glyph list at the
// first font defining endash (or enfilledcircbullet, or any other name
// containing "end"), which then read as a font that does not define the glyphs
// it was asked to render.
//
// It reads the ND token and the one after it; a dictionary that omits ND
// terminates on the first token, which is why both positions are compared.
func type1CharStringsEnd(b []byte) bool {
	i := 0
	for k := 0; k < 2; k++ {
		for i < len(b) && isWhitespace(b[i]) {
			i++
		}
		start := i
		for i < len(b) && !isWhitespace(b[i]) && b[i] != '/' {
			i++
		}
		if string(b[start:i]) == "end" {
			return true
		}
		if start == i {
			return false // a '/' (the next entry) or the data ran out
		}
	}
	return false
}

func indexAfter(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i + 1
		}
	}
	return -1
}

func sscanInt(s string) (bool, int) {
	i := 0
	for i < len(s) && s[i] == ' ' {
		i++
	}
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return false, 0
	}
	v, ok := parseLeadingInt(s[start:i])
	return ok, v
}

// parseLeadingInt reads the digits at the front of s, and says whether they
// came to a number.
//
// The bound is not decoration. Every digit in a PostScript file is a number the
// file chose, and nineteen of them overflow: the length of a charstring came
// out *negative*, which passed a "does this fit in what is left" check by being
// less than everything and then sliced. The cap is what a four-byte offset can
// name, which is more than any real Type 1 font has and less than anything that
// can wrap.
func parseLeadingInt(s string) (int, bool) {
	v := 0
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		v = v*10 + int(c-'0')
		n++
		if v > math.MaxInt32 {
			return 0, false
		}
	}
	return v, n > 0
}

// eexecDecrypt implements the Type 1 decryption (r=55665 for eexec,
// r=4330 for charstrings), discarding the first n plaintext bytes.
func eexecDecrypt(data []byte, r uint16, discard int) []byte {
	const c1, c2 = 52845, 22719
	out := make([]byte, 0, len(data))
	for _, c := range data {
		p := c ^ byte(r>>8)
		r = (uint16(c)+r)*c1 + c2
		out = append(out, p)
	}
	if discard >= len(out) {
		return nil
	}
	return out[discard:]
}

// type1CharstringWidth extracts the width from a decrypted Type 1
// charstring: hsbw (13) gives [sbx wx], sbw (12 7) gives [sbx sby wx wy].
func type1CharstringWidth(cs []byte) (float64, bool) {
	var operands []float64
	i := 0
	for i < len(cs) {
		v := int(cs[i])
		switch {
		case v >= 32 && v <= 246:
			operands = append(operands, float64(v-139))
			i++
		case v >= 247 && v <= 250:
			if i+2 > len(cs) {
				return 0, false
			}
			operands = append(operands, float64((v-247)*256+int(cs[i+1])+108))
			i += 2
		case v >= 251 && v <= 254:
			if i+2 > len(cs) {
				return 0, false
			}
			operands = append(operands, float64(-(v-251)*256-int(cs[i+1])-108))
			i += 2
		case v == 255:
			if i+5 > len(cs) {
				return 0, false
			}
			operands = append(operands, float64(int32(binary.BigEndian.Uint32(cs[i+1:]))))
			i += 5
		case v == 13: // hsbw
			if len(operands) >= 2 {
				return operands[1], true
			}
			return 0, false
		case v == 12:
			if i+1 < len(cs) && cs[i+1] == 7 { // sbw
				if len(operands) >= 3 {
					return operands[2], true
				}
				return 0, false
			}
			i += 2
		default:
			return 0, false
		}
	}
	return 0, false
}

// extractType1FontMatrix reads the x-scale of a Type 1 font's cleartext
// /FontMatrix (default 0.001), used to normalise charstring widths to
// 1/1000 text-space units.
func extractType1FontMatrix(data []byte) float64 {
	i := strings.Index(string(data), "/FontMatrix")
	if i < 0 {
		return 0
	}
	s := string(data[i:])
	lb := strings.IndexByte(s, '[')
	if lb < 0 {
		return 0
	}
	rb := strings.IndexByte(s[lb:], ']')
	if rb < 0 {
		return 0
	}
	fields := strings.Fields(s[lb+1 : lb+rb])
	if len(fields) < 1 {
		return 0
	}
	// A PostScript number, not a BCD real: "0.001", "1e-3" and "-.5" are
	// all spellings a font writes, and parseBCDReal read the second as 13.
	// A token strconv cannot read — a radix number, a name — is no scale,
	// and the caller's default stands.
	f, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// SFNTTables reads an sfnt table directory and returns each table's bytes by
// tag, sharing the caller's backing array. It returns nil when data is not an
// sfnt at all or the directory itself is truncated; a single table whose extent
// lies outside the file is dropped rather than failing the whole font, because
// a program missing one table still answers questions about the others.
//
// This is the one thing a font reader and a font writer share. ParseSFNT reads
// tables to answer questions about the font; shape's subsetter rewrites them.
// Beyond finding where a table is, they have nothing in common, and coupling
// them further would make a subsetter's bug look like a reader's.
func SFNTTables(data []byte) map[string][]byte {
	if len(data) < 12 {
		return nil
	}
	switch Be32(data, 0) {
	case 0x00010000, 0x74727565, 0x4F54544F: // 1.0, 'true', 'OTTO'
	default:
		return nil
	}
	// A directory that runs past the end of the file is refused whole, and it
	// is refused before numTables sizes the map: the count is the file's, and
	// sixty-five thousand of it made a map of several megabytes out of a
	// twelve-byte header.
	numTables := Be16(data, 4)
	if 12+16*numTables > len(data) {
		return nil
	}
	tables := make(map[string][]byte, numTables)
	for i := 0; i < numTables; i++ {
		rec := 12 + 16*i
		name := string(data[rec : rec+4])
		off := Be32(data, rec+8)
		length := Be32(data, rec+12)
		if uint64(off)+uint64(length) > uint64(len(data)) {
			continue
		}
		tables[name] = data[off : off+length]
	}
	return tables
}
