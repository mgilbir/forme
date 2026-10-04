package font

import (
	"encoding/binary"
	"errors"
	"sort"
)

// A font collection wrapped as WOFF 2: W3C WOFF 2.0 §5.3.
//
// A collection — a .ttc or .otc — is several fonts sharing the tables they have
// in common, and WOFF 2 keeps the sharing: its table directory lists each table
// once, and a collection directory after it says, font by font, its flavor
// and which of those tables are its own. What comes out here is the collection
// again, each shared table written once and named by every font that has it,
// which CollectionOffsets and CollectionTables read.
//
// A transformed table is rebuilt once, with what its font's other tables tell
// it: a glyf and the loca it writes are each font's pair, and an hmtx is
// rebuilt from the left edges its font's glyf found and the count its font's
// hhea states. A font whose glyf was rebuilt for another font takes what that
// rebuild found. The head's checkSumAdjustment is of the font that names it
// last, which is all one number can be of where fonts share a head.

// woff2CollectionFont is one font of a collection directory: its flavor and the
// tables of the directory that are its own.
type woff2CollectionFont struct {
	flavor uint32
	tables []int
}

// u255 reads a 255UInt16: one byte, or one of three escapes saying a second
// byte or a 16-bit word follows (W3C WOFF 2.0 §4.1).
func (r *woff2Reader) u255() (int, bool) {
	b, ok := r.u8()
	if !ok {
		return 0, false
	}
	switch b {
	case 253:
		if r.at+2 > len(r.b) {
			return 0, false
		}
		v := int(binary.BigEndian.Uint16(r.b[r.at:]))
		r.at += 2
		return v, true
	case 254:
		b, ok := r.u8()
		return int(b) + 506, ok
	case 255:
		b, ok := r.u8()
		return int(b) + 253, ok
	}
	return int(b), true
}

// readWOFF2Collection reads the collection directory: its version, and each
// font's flavor and table indices, each index one of the table directory's.
func readWOFF2Collection(r *woff2Reader, numTables int) (uint32, []woff2CollectionFont, error) {
	short := errors.New("fonts: the WOFF 2 collection directory is cut short")
	version, ok := r.u32()
	if !ok {
		return 0, nil, short
	}
	if version != 0x00010000 && version != 0x00020000 {
		return 0, nil, errors.New("fonts: the WOFF 2 collection directory states a version this format does not define")
	}
	numFonts, ok := r.u255()
	if !ok {
		return 0, nil, short
	}
	if numFonts == 0 {
		return 0, nil, errors.New("fonts: the WOFF 2 collection holds no fonts")
	}
	// A font's entry is at least six bytes — a count, a flavor and an index —
	// so no more fonts are believed than the bytes left could describe.
	if numFonts > (len(r.b)-r.at)/6 {
		return 0, nil, errors.New("fonts: the WOFF 2 collection states more fonts than its directory holds")
	}
	fonts := make([]woff2CollectionFont, numFonts)
	for i := range fonts {
		n, ok := r.u255()
		if !ok {
			return 0, nil, short
		}
		if n == 0 || n > maxWOFFTables || n > len(r.b)-r.at {
			return 0, nil, errors.New("fonts: a font of the WOFF 2 collection states a number of tables it cannot have")
		}
		if fonts[i].flavor, ok = r.u32(); !ok {
			return 0, nil, short
		}
		fonts[i].tables = make([]int, n)
		for k := range fonts[i].tables {
			idx, ok := r.u255()
			if !ok {
				return 0, nil, short
			}
			if idx >= numTables {
				return 0, nil, errors.New("fonts: a font of the WOFF 2 collection names a table the directory does not have")
			}
			fonts[i].tables[k] = idx
		}
	}
	return version, fonts, nil
}

// rebuildCollection assembles the collection: its header, each font's table
// directory, and every table any font names, once.
func rebuildCollection(version uint32, fonts []woff2CollectionFont, tables []woff2Table, body []byte) ([]byte, error) {
	head := 12 + 4*len(fonts)
	if version == 0x00020000 {
		head += 12 // the DSIG tag, length and offset, all zero: there is none
	}
	dirs := make([]int, len(fonts))
	at := head
	for i, font := range fonts {
		dirs[i] = at
		at += 12 + 16*len(font.tables)
	}
	if at > maxWOFFSfntSize {
		return nil, errors.New("fonts: the WOFF 2 collection's directories come to more than this engine will hold")
	}
	out := make([]byte, at)
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], version)
	binary.BigEndian.PutUint32(out[8:], uint32(len(fonts)))
	for i, d := range dirs {
		binary.BigEndian.PutUint32(out[12+4*i:], uint32(d))
	}

	done := make([]bool, len(tables))
	sums := make([]uint32, len(tables))
	// What rebuilding each glyf found, for a font that shares it with one
	// rebuilt before.
	glyfs := map[int]woff2Font{}
	// The glyf and the metric count each transformed hmtx was rebuilt from,
	// which a font sharing it has to have too.
	type hmtxFrom struct{ glyf, metrics int }
	hmtxs := map[int]hmtxFrom{}
	for _, font := range fonts {
		order := append([]int(nil), font.tables...)
		sort.Ints(order)
		tags := map[uint32]bool{}
		for _, idx := range order {
			if tags[tables[idx].tag] {
				return nil, errors.New("fonts: a font of the WOFF 2 collection names the same table twice")
			}
			tags[tables[idx].tag] = true
		}
		var f woff2Font
		glyf, loca := -1, -1
		for _, idx := range order {
			switch tables[idx].tag {
			case tagGlyf:
				glyf = idx
			case tagLoca:
				loca = idx
			}
		}
		if (glyf < 0) != (loca < 0) {
			return nil, errors.New("fonts: a font of the WOFF 2 collection has one of glyf and loca and not the other")
		}
		if glyf >= 0 {
			if tables[glyf].transformed != tables[loca].transformed {
				return nil, errors.New("fonts: a font of the WOFF 2 collection transformed one of glyf and loca and not the other")
			}
			if glyf > loca {
				return nil, errors.New("fonts: a font of the WOFF 2 collection names its loca table before the glyf table it comes out of")
			}
			if g, ok := glyfs[glyf]; ok {
				if g.loca != &tables[loca] {
					return nil, errors.New("fonts: two fonts of the WOFF 2 collection share a glyf and not its loca")
				}
				f = g
			}
			f.loca = &tables[loca]
		}
		// The tables in the directory's order, each written the first time a
		// font names it: glyf before its loca, and before the hmtx rebuilt
		// from it, as in a single font.
		for _, idx := range order {
			t := &tables[idx]
			if done[idx] {
				if err := readHhea(t, body, &f); err != nil {
					return nil, err
				}
				if from, ok := hmtxs[idx]; ok && (from.glyf != glyf || from.metrics != int(f.numHMetrics)) {
					return nil, errors.New("fonts: two fonts of the WOFF 2 collection share an hmtx " +
						"rebuilt from another glyf or hhea than one of them has")
				}
				continue
			}
			var err error
			if out, sums[idx], err = rebuildTable(out, t, body, &f); err != nil {
				return nil, err
			}
			done[idx] = true
			switch {
			case t.tag == tagGlyf:
				glyfs[idx] = f
			case t.tag == tagHmtx && t.transformed:
				hmtxs[idx] = hmtxFrom{glyf, int(f.numHMetrics)}
			}
		}
	}

	// Each font's directory: its header, and a record for each of its tables
	// in tag order, named where the table was written.
	for i, font := range fonts {
		d := dirs[i]
		n := len(font.tables)
		order := append([]int(nil), font.tables...)
		sort.Slice(order, func(a, b int) bool { return tables[order[a]].tag < tables[order[b]].tag })
		binary.BigEndian.PutUint32(out[d:], font.flavor)
		binary.BigEndian.PutUint16(out[d+4:], uint16(n))
		entrySelector := uint16(0)
		for 1<<(entrySelector+1) <= uint16(n) {
			entrySelector++
		}
		searchRange := uint16(16) << entrySelector
		binary.BigEndian.PutUint16(out[d+6:], searchRange)
		binary.BigEndian.PutUint16(out[d+8:], entrySelector)
		binary.BigEndian.PutUint16(out[d+10:], uint16(16*n)-searchRange)
		var total uint32
		var headTable *woff2Table
		for k, idx := range order {
			t := &tables[idx]
			rec := d + 12 + 16*k
			binary.BigEndian.PutUint32(out[rec:], t.tag)
			binary.BigEndian.PutUint32(out[rec+4:], sums[idx])
			binary.BigEndian.PutUint32(out[rec+8:], t.dstOffset)
			binary.BigEndian.PutUint32(out[rec+12:], t.dstLength)
			total += sums[idx]
			if t.tag == tagHead {
				headTable = t
			}
		}
		total += computeULongSum(out[d : d+12+16*n])
		if headTable != nil {
			if headTable.dstLength < 12 {
				return nil, errors.New("fonts: the WOFF 2's head table is too short")
			}
			binary.BigEndian.PutUint32(out[headTable.dstOffset+8:], 0xB1B0AFBA-total)
		}
	}
	return out, nil
}
