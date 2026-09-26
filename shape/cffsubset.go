package shape

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/mgilbir/forme/font"
)

// Subsetting a CFF program. There are two rules, by the kind of program.
//
// A CFF that is not CID-keyed follows the glyf subsetter's: glyph indices are
// retained, and a dropped glyph becomes an empty one — for CFF a charstring of
// a single endchar operator, a glyph that draws nothing. It is not
// zero-length, because a zero-length charstring is not a charstring; a renderer
// asked for one is entitled to reject the font. The numbering has to be kept
// because it is what a document addresses such a program by: embedded as a
// CIDFontType0, a CFF with no CID-keyed Top DICT has its CIDs used directly as
// glyph indices (ISO 32000-2 9.7.4.2), and Encode wrote glyph indices into the
// content stream before anything knew which glyphs would survive. So the
// charset, the encoding and both subroutine INDEXes are copied through
// untouched, and the only structure that changes is the one whose size
// changed.
//
// A CID-keyed CFF is renumbered, and was not always. The same rule was applied
// to it, for the reason above — renumbering means rewriting the charset, the
// FDSelect and every call into a subroutine INDEX, where keeping the numbering
// copies them — and the cost was the whole font: every glyph slot, every
// charset entry, every subroutine, and the cmap and hmtx around them. Three
// characters of Noto Sans JP made a subset of 1,680,632 bytes. For this kind of
// program renumbering changes nothing a document depends on, because it is
// addressed by CID, and the charset that maps a CID to a glyph is written for
// the subset with each kept glyph's own CID. The same three characters now make
// 4,548 bytes. See cffrenumber.go.

// subsetCFF rewrites a CFF program to carry outlines only for the kept glyphs.
//
// The kept slice is indexed by glyph, as the glyf subsetter's is, so both take
// the same decision from the same place.
//
// What a font can make this repeat — a Private DICT and its subroutine INDEX
// read again for every Font DICT that names them, and a CID-keyed font's
// subroutines run for the kept glyphs that call them — is charged to budget,
// and a font that asks for more than it holds is refused with the budget's
// error.
//
// It returns, for a CID-keyed program, which is renumbered, the original index
// of each of the subset's glyphs in order; nil for any other program, whose
// numbering is kept.
func subsetCFF(data []byte, keep []bool, budget *font.Budget) ([]byte, []int, error) {
	if len(data) > maxCFFSize {
		return nil, nil, fmt.Errorf("fonts: CFF program of %d bytes is too large to subset", len(data))
	}
	if len(data) < 4 {
		return nil, nil, errors.New("fonts: CFF program is too short to hold a header")
	}
	hdrSize := int(data[2])
	if hdrSize < 4 || hdrSize > len(data) {
		return nil, nil, fmt.Errorf("fonts: CFF header size %d is not usable", hdrSize)
	}

	nameIndex, err := readCFFIndex(data, hdrSize)
	if err != nil {
		return nil, nil, err
	}
	topIndex, err := readCFFIndex(data, nameIndex.end)
	if err != nil {
		return nil, nil, err
	}
	if len(topIndex.items) != 1 {
		return nil, nil, fmt.Errorf("fonts: CFF holds %d Top DICTs; only a single-font program is supported", len(topIndex.items))
	}
	stringIndex, err := readCFFIndex(data, topIndex.end)
	if err != nil {
		return nil, nil, err
	}
	gsubrIndex, err := readCFFIndex(data, stringIndex.end)
	if err != nil {
		return nil, nil, err
	}
	gsubrs := gsubrIndex.items // what the subset carries, which may be fewer

	top, topRaw, err := parseCFFDict(topIndex.items[0])
	if err != nil {
		return nil, nil, err
	}

	var charStringsOff, charsetOff, encodingOff, privSize, privOff int
	var fdArrayOff, fdSelectOff int
	var isCID, hasCharset bool
	for _, e := range top {
		switch e.op {
		case opROS:
			isCID = true
		case opFDArray:
			if len(e.operands) == 1 {
				fdArrayOff = e.operands[0]
			}
		case opFDSelect:
			if len(e.operands) == 1 {
				fdSelectOff = e.operands[0]
			}
		case opCharStrings:
			if len(e.operands) == 1 {
				charStringsOff = e.operands[0]
			}
		case opCharset:
			hasCharset = true
			if len(e.operands) == 1 {
				charsetOff = e.operands[0]
			}
		case opEncoding:
			if len(e.operands) == 1 {
				encodingOff = e.operands[0]
			}
		case opPrivate:
			if len(e.operands) == 2 {
				privSize, privOff = e.operands[0], e.operands[1]
			}
		}
	}
	if charStringsOff <= 0 {
		return nil, nil, errors.New("fonts: CFF Top DICT names no CharStrings")
	}
	charStrings, err := readCFFIndex(data, charStringsOff)
	if err != nil {
		return nil, nil, err
	}
	n := len(charStrings.items)
	if n != len(keep) {
		return nil, nil, fmt.Errorf("fonts: the CFF holds %d glyphs but %d were decided on", n, len(keep))
	}

	// order is the subset's glyphs, by their index in this font, when it is
	// renumbered — a CID-keyed font, see cffrenumber.go — and nil when the
	// numbering is kept. .notdef is glyph 0 either way.
	var order []int
	var csBlob, charsetBlob []byte
	if isCID {
		if n == 0 {
			return nil, nil, errors.New("fonts: a CID-keyed CFF holds no glyphs, not even .notdef")
		}
		order = append(make([]int, 0, n), 0)
		for g := 1; g < n; g++ {
			if keep[g] {
				order = append(order, g)
			}
		}
		// The charset read as what it is in a CID-keyed font, the CID of each
		// glyph, and written again for the kept glyphs alone: each keeps the
		// CID it had, which is the number a document addresses it by. The
		// charstrings wait for the subroutines, below.
		cids, err := cffCharsetSIDs(data, charsetOff, n)
		if err != nil {
			return nil, nil, err
		}
		kept := make([]int, len(order))
		for i, g := range order {
			kept[i] = cids[g]
		}
		charsetBlob = cidCharset(kept)
	} else {
		// The new CharStrings: kept glyphs verbatim, dropped ones a bare
		// endchar.
		newCharStrings := make([][]byte, n)
		for i := 0; i < n; i++ {
			if keep[i] {
				newCharStrings[i] = charStrings.items[i]
				continue
			}
			newCharStrings[i] = []byte{14} // endchar
		}
		csBlob = writeCFFIndex(newCharStrings)

		// The charset, copied through unchanged. Its contents do not depend on
		// which glyphs survive, because the numbering did not change; only
		// where it sits does. The same holds for the encoding below.
		if charsetBlob, err = sliceCharset(data, charsetOff, n); err != nil {
			return nil, nil, err
		}
	}
	encodingBlob, err := sliceEncoding(data, encodingOff)
	if err != nil {
		return nil, nil, err
	}
	var privBlob []byte
	if privSize > 0 {
		var err error
		if privBlob, err = cffPrivateRegion(data, privOff, privSize, budget); err != nil {
			return nil, nil, err
		}
	}

	// The CID-keyed structures. A CID-keyed font has no Top DICT Private entry:
	// each glyph's Private DICT is reached through FDSelect, which says which
	// Font DICT a glyph belongs to, and FDArray, which holds them.
	//
	// The FDSelect is written again for the renumbered glyphs, each in the Font
	// DICT it was in. The FDArray keeps every Font DICT, so the numbers the
	// FDSelect states do not change; what changes in a Font DICT is its Private
	// DICT's size and where it sits, because each names its own by absolute
	// offset and they are about to move — and the Private DICTs themselves,
	// which lose the local subroutines no kept glyph calls.
	//
	// Font DICTs may share a Private DICT, and then share its copy: each
	// region is read and written once, and every Font DICT naming it is
	// pointed at the one copy. And the regions written may come to no more
	// than the font held. In a font whose regions are its own they cannot,
	// because they do not overlap; regions that overlap — a thousand Private
	// DICTs a byte apart, each naming the same large INDEX behind them — would
	// have made the subset a thousand times the font.
	var fdSelectBlob, fdArrayBlob []byte
	var fdPrivBlobs [][]byte // the distinct regions, in the order first named
	var regionParts []privateRegion
	var regionSize []int // the Private DICT's own size, which its Font DICTs state
	var fontDicts [][]cffOp
	var fontDictsRaw [][][]byte
	var fdPrivSizes []int
	var fdRegion []int // for each Font DICT, which of fdPrivBlobs it names; -1 none
	if isCID {
		var err error
		fdSelectBlob, err = sliceFDSelect(data, fdSelectOff, n)
		if err != nil {
			return nil, nil, err
		}
		fdIndex, err := readCFFIndex(data, fdArrayOff)
		if err != nil {
			return nil, nil, err
		}
		if len(fdIndex.items) == 0 {
			return nil, nil, errors.New("fonts: a CID-keyed CFF names an empty FDArray")
		}
		regions := map[[2]int]int{}
		written := 0
		for _, fd := range fdIndex.items {
			if !budget.Charge(len(fd), "the CFF Font DICTs") {
				return nil, nil, budget.Err()
			}
			ops, raw, err := parseCFFDict(fd)
			if err != nil {
				return nil, nil, err
			}
			size, off := 0, 0
			for _, e := range ops {
				if e.op == opPrivate && len(e.operands) == 2 {
					size, off = e.operands[0], e.operands[1]
				}
			}
			region := -1
			if size > 0 {
				key := [2]int{off, size}
				r, seen := regions[key]
				if !seen {
					blob, err := cffPrivateRegion(data, off, size, budget)
					if err != nil {
						return nil, nil, err
					}
					if written += len(blob); written > len(data) {
						return nil, nil, errors.New("fonts: a CID-keyed CFF's Private DICTs " +
							"overlap, and copying each would make the subset larger than the font")
					}
					ops, raw, subrs, err := privateParts(blob, size)
					if err != nil {
						return nil, nil, err
					}
					r = len(fdPrivBlobs)
					regions[key] = r
					fdPrivBlobs = append(fdPrivBlobs, blob)
					regionParts = append(regionParts, privateRegion{ops, raw, subrs})
					regionSize = append(regionSize, size)
				}
				region = r
			}
			fontDicts = append(fontDicts, ops)
			fontDictsRaw = append(fontDictsRaw, raw)
			fdPrivSizes = append(fdPrivSizes, size)
			fdRegion = append(fdRegion, region)
		}

		// Each kept glyph's Font DICT, which the new FDSelect states for its
		// new number. A glyph the FDSelect gives none, or one past the
		// FDArray, has no Private DICT to be drawn with, and a font that keeps
		// one is refused rather than given another's.
		fds := fdSelectFDs(fdSelectBlob, n)
		newFDs := make([]int, len(order))
		for i, g := range order {
			if fd := fds[g]; fd < 0 || fd >= len(fontDicts) {
				return nil, nil, fmt.Errorf("fonts: a CID-keyed CFF's FDSelect gives glyph %d "+
					"no Font DICT of the %d its FDArray holds", g, len(fontDicts))
			}
			newFDs[i] = fds[g]
		}
		fdSelectBlob = writeFDSelect(newFDs)

		// The charstrings, and the subroutines they call: every INDEX loses
		// what no kept glyph reaches, and the calls are renumbered, or — where
		// that cannot be done exactly — the subroutines are kept whole.
		glyphs := make([][]byte, len(order))
		regionOf := make([]int, len(order))
		for i, g := range order {
			glyphs[i] = charStrings.items[g]
			regionOf[i] = fdRegion[newFDs[i]]
		}
		locals := make([][][]byte, len(regionParts))
		for r, part := range regionParts {
			locals[r] = part.subrs
		}
		prunedGlyphs, prunedGlobal, prunedLocals, pruned, err := pruneSubrs(
			glyphs, regionOf, gsubrs, locals, budget)
		if err != nil {
			return nil, nil, err
		}
		if pruned {
			glyphs, gsubrs = prunedGlyphs, prunedGlobal
			for r, part := range regionParts {
				fdPrivBlobs[r], regionSize[r] = rebuildPrivate(part.ops, part.raw, prunedLocals[r])
			}
		}
		csBlob = writeCFFIndex(glyphs)
	}

	// writeFontDicts rebuilds the FDArray with each Font DICT's Private DICT
	// named at the offset it will really sit at. Every rewritten operand is five
	// bytes, as in the Top DICT, so the INDEX's size does not depend on the
	// values and one measuring pass is enough.
	writeFontDicts := func(at []int) []byte {
		out := make([][]byte, len(fontDicts))
		for i, ops := range fontDicts {
			var d []byte
			for k, e := range ops {
				if e.op == opPrivate {
					where, size := 0, fdPrivSizes[i]
					if r := fdRegion[i]; r >= 0 {
						where, size = at[r], regionSize[r]
					}
					d = append(d, cffInt(size)...)
					d = append(d, cffInt(where)...)
					d = append(d, byte(opPrivate))
					continue
				}
				d = append(d, fontDictsRaw[i][k]...)
			}
			out[i] = d
		}
		return writeCFFIndex(out)
	}
	if isCID {
		fdArrayBlob = writeFontDicts(make([]int, len(fdPrivBlobs)))
	}

	// Lay the font out, then write the Top DICT with the offsets that layout
	// produced. Every rewritten operand is five bytes, so the DICT's size does
	// not depend on the values going into it and one pass is enough.
	rewrite := func(charset, encoding, charstrings, private, fdArray, fdSelect int) []byte {
		var out []byte
		for i, e := range top {
			switch e.op {
			case opCharset:
				out = append(out, cffInt(charset)...)
				out = append(out, byte(opCharset))
			case opEncoding:
				out = append(out, cffInt(encoding)...)
				out = append(out, byte(opEncoding))
			case opCharStrings:
				out = append(out, cffInt(charstrings)...)
				out = append(out, byte(opCharStrings))
			case opPrivate:
				out = append(out, cffInt(privSize)...)
				out = append(out, cffInt(private)...)
				out = append(out, byte(opPrivate))
			case opFDArray:
				out = append(out, cffInt(fdArray)...)
				out = append(out, 12, 36)
			case opFDSelect:
				out = append(out, cffInt(fdSelect)...)
				out = append(out, 12, 37)
			default:
				out = append(out, topRaw[i]...) // verbatim, operands and all
			}
		}
		return out
	}

	// A renumbered font states its charset, whatever the original did: a
	// CID-keyed font with none reads as the identity, and the kept glyphs'
	// CIDs are not that. An entry is added for the rewrite to fill in.
	if isCID && !hasCharset {
		top = append(top, cffOp{op: opCharset})
		topRaw = append(topRaw, nil)
	}

	// The Top DICT's size is stable, so compute it once against placeholders.
	topBlob := rewrite(0, 0, 0, 0, 0, 0)
	prefix := hdrSize
	prefix += len(writeCFFIndex(nameIndex.items))
	prefix += len(writeCFFIndex([][]byte{topBlob}))
	prefix += len(writeCFFIndex(stringIndex.items))
	prefix += len(writeCFFIndex(gsubrs))

	charsetAt := prefix
	encodingAt := charsetAt + len(charsetBlob)
	charStringsAt := encodingAt + len(encodingBlob)
	privateAt := charStringsAt + len(csBlob)
	// A CID-keyed font puts its FDArray and FDSelect after the charstrings, and
	// the Private DICTs the Font DICTs name after those. privateAt is unused in
	// that case: there is no Top DICT Private entry to point anywhere.
	fdArrayAt := privateAt
	fdSelectAt := fdArrayAt + len(fdArrayBlob)
	fdPrivAt := make([]int, len(fdPrivBlobs))
	if isCID {
		at := fdSelectAt + len(fdSelectBlob)
		for i, b := range fdPrivBlobs {
			fdPrivAt[i] = at
			at += len(b)
		}
		// Now that the Private DICTs have addresses, the Font DICTs can name
		// them. The INDEX keeps the size it was measured at.
		if rebuilt := writeFontDicts(fdPrivAt); len(rebuilt) != len(fdArrayBlob) {
			return nil, nil, errors.New("fonts: internal: the CID FDArray changed size when its offsets were filled in")
		} else {
			fdArrayBlob = rebuilt
		}
	}

	// A predefined charset or encoding is named by a small number rather than
	// an offset, and must keep that number — except the charset of a
	// renumbered font, which is written out whatever the original was.
	if charsetOff <= 2 && !isCID {
		charsetAt = charsetOff
	}
	if encodingOff <= 1 {
		encodingAt = encodingOff
	}
	topBlob = rewrite(charsetAt, encodingAt, charStringsAt, privateAt, fdArrayAt, fdSelectAt)

	out := make([]byte, 0, len(data))
	out = append(out, data[:hdrSize]...)
	out = append(out, writeCFFIndex(nameIndex.items)...)
	out = append(out, writeCFFIndex([][]byte{topBlob})...)
	out = append(out, writeCFFIndex(stringIndex.items)...)
	out = append(out, writeCFFIndex(gsubrs)...)
	out = append(out, charsetBlob...)
	out = append(out, encodingBlob...)
	out = append(out, csBlob...)
	if isCID {
		out = append(out, fdArrayBlob...)
		out = append(out, fdSelectBlob...)
		for _, b := range fdPrivBlobs {
			out = append(out, b...)
		}
		want := fdSelectAt + len(fdSelectBlob)
		for _, b := range fdPrivBlobs {
			want += len(b)
		}
		if len(out) != want {
			return nil, nil, errors.New("fonts: internal: the CID CFF layout did not match what the DICTs were told")
		}
		return out, order, nil
	}
	out = append(out, privBlob...)
	if len(out) != privateAt+len(privBlob) {
		return nil, nil, errors.New("fonts: internal: the CFF layout did not match what the Top DICT was told")
	}
	return out, order, nil
}

// sliceFDSelect returns the FDSelect's bytes.
//
// Like the charset, its length is not stored: it follows from the format and the
// glyph count, so it has to be measured rather than read.
func sliceFDSelect(data []byte, off, nGlyphs int) ([]byte, error) {
	if off <= 0 || off >= len(data) {
		return nil, errors.New("fonts: a CID-keyed CFF's FDSelect lies outside the font")
	}
	b := data[off:]
	switch b[0] {
	case 0:
		// The format byte, then one byte per glyph.
		size := 1 + nGlyphs
		if size > len(b) {
			return nil, errors.New("fonts: truncated CFF FDSelect")
		}
		return b[:size], nil
	case 3:
		// The format byte, a range count, three bytes per range, and a sentinel
		// giving the glyph after the last range.
		if len(b) < 5 {
			return nil, errors.New("fonts: truncated CFF FDSelect")
		}
		size := 3 + int(binary.BigEndian.Uint16(b[1:]))*3 + 2
		if size > len(b) {
			return nil, errors.New("fonts: truncated CFF FDSelect")
		}
		return b[:size], nil
	}
	return nil, fmt.Errorf("fonts: CFF FDSelect format %d is not one this reads", b[0])
}

// sliceCharset returns the charset's bytes. Its length is not stored anywhere:
// it follows from the format and the glyph count, which is why this has to
// walk it rather than copy a range.
func sliceCharset(data []byte, off, nGlyphs int) ([]byte, error) {
	if off <= 2 {
		return nil, nil // predefined: ISOAdobe, Expert or ExpertSubset
	}
	if off >= len(data) {
		return nil, errors.New("fonts: the CFF charset lies outside the font")
	}
	c := data[off:]
	switch c[0] {
	case 0:
		size := 1 + 2*(nGlyphs-1)
		if size > len(c) {
			return nil, errors.New("fonts: truncated CFF charset")
		}
		return c[:size], nil
	case 1, 2:
		step := 3
		if c[0] == 2 {
			step = 4
		}
		covered, size := 1, 1 // glyph 0 is .notdef and is not listed
		for covered < nGlyphs {
			if size+step > len(c) {
				return nil, errors.New("fonts: truncated CFF charset")
			}
			nLeft := int(c[size+2])
			if step == 4 {
				nLeft = int(binary.BigEndian.Uint16(c[size+2:]))
			}
			covered += nLeft + 1
			size += step
		}
		return c[:size], nil
	}
	return nil, fmt.Errorf("fonts: CFF charset format %d is not one the format defines", c[0])
}

// sliceEncoding returns the encoding's bytes, whose length likewise follows
// from its format.
func sliceEncoding(data []byte, off int) ([]byte, error) {
	if off <= 1 {
		return nil, nil // predefined: Standard or Expert
	}
	if off >= len(data) {
		return nil, errors.New("fonts: the CFF encoding lies outside the font")
	}
	e := data[off:]
	format := e[0]
	var size int
	switch format &^ 0x80 {
	case 0:
		if len(e) < 2 {
			return nil, errors.New("fonts: truncated CFF encoding")
		}
		size = 2 + int(e[1])
	case 1:
		if len(e) < 2 {
			return nil, errors.New("fonts: truncated CFF encoding")
		}
		size = 2 + 2*int(e[1])
	default:
		return nil, fmt.Errorf("fonts: CFF encoding format %d is not one the format defines", format)
	}
	if format&0x80 != 0 { // supplements follow
		if size >= len(e) {
			return nil, errors.New("fonts: truncated CFF encoding supplements")
		}
		size += 1 + 3*int(e[size])
	}
	if size > len(e) {
		return nil, errors.New("fonts: truncated CFF encoding")
	}
	return e[:size], nil
}

// cffPrivateRegion is a Private DICT and what travels with it: the DICT
// itself and, where it names local subroutines, everything after it to the end
// of their INDEX. The Top DICT's Private and each Font DICT's are read by it, so
// both are held to the same two checks.
//
// Local subroutines are named from inside the DICT, as a distance from its
// start, so as long as the DICT moves as a unit with them that offset stays
// correct and needs no rewriting. "As a unit" is the whole of it, and it is why
// the region runs from the DICT's start to the INDEX's end rather than being
// the DICT and the INDEX: nothing in the format says the INDEX begins where the
// DICT ends, a font is free to leave bytes between them, and copying the two
// out separately and writing them back to back closes that gap — which moves
// the INDEX and leaves the operand naming the distance it used to be at.
//
// An INDEX said to begin inside the DICT is refused: its bytes are the DICT's,
// and the region taken to its end could stop short of the size the Font DICT
// goes on stating, so that a reader of the subset took the next region's bytes
// for the end of this one. The Top DICT's path refused it; the CID-keyed path
// took the region from the DICT's start to the INDEX's end without asking.
//
// The DICT's bytes and the INDEX's entries are charged to budget.
func cffPrivateRegion(data []byte, off, size int, budget *font.Budget) ([]byte, error) {
	if off < 0 || size < 0 || off > len(data) || size > len(data)-off {
		return nil, errors.New("fonts: a CFF Private DICT lies outside the font")
	}
	if !budget.Charge(size, "a CFF Private DICT") {
		return nil, budget.Err()
	}
	privOps, _, err := parseCFFDict(data[off : off+size])
	if err != nil {
		return nil, err
	}
	end := off + size
	for _, e := range privOps {
		if e.op != opSubrs || len(e.operands) != 1 || e.operands[0] <= 0 {
			continue
		}
		if e.operands[0] < size {
			return nil, errors.New("fonts: a CFF Private DICT names local " +
				"subroutines inside itself")
		}
		idx, err := readCFFIndex(data, off+e.operands[0])
		if err != nil {
			return nil, err
		}
		if !budget.Charge(len(idx.items), "a CFF local subroutine INDEX") {
			return nil, budget.Err()
		}
		// An INDEX that begins at or after the DICT's end ends after it: its
		// count alone is two bytes. The check the Top DICT's path made that it
		// did not end before the DICT could not fail, and is gone.
		end = idx.end
	}
	return data[off:end], nil
}

// subsetOpenTypeCFF rebuilds an OpenType font around a subsetted CFF table.
func (f *Face) subsetOpenTypeCFF() ([]byte, []int, error) {
	tables := font.SFNTTables(f.data)
	if tables == nil || tables["CFF "] == nil {
		return nil, nil, errors.New("fonts: the font no longer carries a CFF table")
	}
	n := f.prog.NumGlyphs
	keep := make([]bool, n)
	keep[0] = true // .notdef
	for gid := range f.used {
		if gid >= 0 && gid < n {
			keep[gid] = true
		}
	}
	// A charstring that reuses another shape does it through a subroutine or
	// through a seac, and only the first of those is carried along by keeping
	// the subroutine INDEXes whole. A seac names two other *glyphs*, so it
	// needs a closure exactly as a composite glyf glyph does. See cffseac.go.
	//
	// Both walks draw on one allowance, of the size Load's has, and it is this
	// subset's own: what reading the font cost at Load is not charged again.
	budget := font.NewBudget(maxFontWork)
	if err := cffSeacClosure(tables["CFF "], keep, budget); err != nil {
		return nil, nil, err
	}

	sub, order, err := subsetCFF(tables["CFF "], keep, budget)
	if err != nil {
		return nil, nil, err
	}
	out := map[string][]byte{}
	for _, tag := range []string{"cmap", "head", "hhea", "hmtx", "maxp", "name", "post", "OS/2"} {
		if b, ok := tables[tag]; ok {
			out[tag] = b
		}
	}
	out["CFF "] = sub
	// A renumbered CFF has to be described by tables numbered the same way:
	// maxp's count, the metrics, and the character map all name glyphs by
	// index, and the originals name the old ones.
	if order != nil {
		if err := renumberSFNT(out, order, n, f.prog.Cmap); err != nil {
			return nil, nil, err
		}
	}

	kept := make([]int, 0, len(keep))
	for gid, k := range keep {
		if k {
			kept = append(kept, gid)
		}
	}
	return assembleOTTO(out), kept, nil
}

// assembleOTTO writes an OpenType font whose outlines are CFF.
func assembleOTTO(tables map[string][]byte) []byte {
	out := assembleSFNT(tables)
	binary.BigEndian.PutUint32(out[0:], 0x4F54544F) // 'OTTO'
	return out
}
