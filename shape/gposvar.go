package shape

import "github.com/mgilbir/forme/font"

// Positioning at a point in a variable font's design space.
//
// A variable font's kerning and its mark anchors vary with its outlines: a bold
// "AV" needs a different kern from a thin one, and an acute sits higher over a
// heavier "e". GPOS says so without a second table. A ValueRecord may carry,
// beside each number, the offset of a Device table, and an Anchor of format 3
// the same beside each coordinate; a Device table whose DeltaFormat is 0x8000 is
// not the hinting correction the format was first written for but a
// VariationIndex — an outer and an inner index into the item variation store
// GDEF carries (varstore.go). The number the record states is the default
// instance's, and the store's delta at the location is added to it.
//
// None of that was read. An instance cut by LoadInstance carries GPOS and GDEF
// unchanged, and was shaped with the default instance's kerning and anchors
// whatever weight it had been cut at: Noto Sans states 20,823 such devices, so
// every kerned pair and every attached mark of a bold instance sat where the
// regular one puts it.
//
// # The arithmetic, and whose it is
//
// HarfBuzz's. Device::get_x_delta for a VariationIndex scales the store's delta
// and rounds it on its own, and the record's number is added to that; anchors
// are the same, the coordinate and then its device's delta rounded. The
// rounding is half up — HarfBuzz defines its own roundf as floor(x + 0.5), so
// that every platform rounds alike — which is otRound: -1.5 is -1, not the -2
// the C library's roundf makes of it. VariedLayout.ttf's 'dist' feature holds
// exactly that case at weight 250. Since the record's number is whole, the
// stated value plus the delta rounded is the sum rounded.
//
// Hinting devices — DeltaFormat 1 to 3 — adjust by pixels at a size and are
// not read, as they were not before: layout sets text at no pixel size.

// deviceDeltas is what a face needs to read a VariationIndex: GDEF's store, and
// the normalized location the face was cut at. A nil one varies nothing, which
// is every face read at its default instance.
type deviceDeltas struct {
	store  *varStore
	coords []float64
}

// readDeviceDeltas reads GDEF's item variation store, where the face was cut
// somewhere other than its default instance; nil otherwise.
//
// At the default every coordinate is zero, every region's scalar is zero and
// every delta with it — so a face from Load is not given a store it would only
// ever multiply by nothing, and its positioning costs what it did.
//
// The store's offset arrived in GDEF 1.3, after the mark glyph sets 1.2 added,
// so it is at byte fourteen of a header no shorter than eighteen. A store that
// cannot be read varies nothing: the stated values are the default instance's,
// which is text set a little off rather than text misread.
func readDeviceDeltas(gdef []byte, coords []float64) *deviceDeltas {
	off := false
	for _, c := range coords {
		off = off || c != 0
	}
	if !off || len(gdef) < 18 || font.Be16(gdef, 0) != 1 || font.Be16(gdef, 2) < 3 {
		return nil
	}
	at := int(font.Be32(gdef, 14))
	if at <= 0 || at >= len(gdef) {
		return nil
	}
	store, err := parseVarStore(gdef[at:])
	if err != nil {
		return nil
	}
	return &deviceDeltas{store: store, coords: coords}
}

// at is the delta the Device table at base[off:] states at the face's location,
// rounded as HarfBuzz rounds it, and zero for a hinting device, an absent one,
// or one the table does not hold.
func (d *deviceDeltas) at(base []byte, off int) int {
	if d == nil || off <= 0 || off+6 > len(base) {
		return 0
	}
	if font.Be16(base, off+4) != 0x8000 {
		return 0
	}
	return otRound(d.store.delta(font.Be16(base, off), font.Be16(base, off+2), d.coords))
}

// valueRecordAt reads the ValueRecord at sub[at:] with its device deltas added.
// sub is the table that holds the record, because that is what the record's
// Device offsets are measured from: a single adjustment subtable, a class pair
// subtable — or, for a listed pair, the pair set and not the subtable, which is
// the case the specification words least clearly and HarfBuzz reads this way.
//
// It is readValueRecord for a face that may be varied, and is exactly that for
// one that is not.
func valueRecordAt(sub []byte, at, format int, dv *deviceDeltas) singleAdjust {
	if at < 0 || at > len(sub) {
		return singleAdjust{}
	}
	out := readValueRecord(sub[at:], format)
	if dv == nil || format&0x0070 == 0 {
		return out
	}
	// The four device offsets follow the four values, in the same order, and
	// only those whose bits are set are there at all.
	off := at + 2*popcount4(format)
	device := func(bit int) int {
		if format&bit == 0 {
			return 0
		}
		o := off
		off += 2
		if o+2 > len(sub) {
			return 0
		}
		return dv.at(sub, font.Be16(sub, o))
	}
	out.xPlacement += device(0x0010)
	out.yPlacement += device(0x0020)
	out.xAdvance += device(0x0040)
	return out
}

// yAdvanceAt is valueYAdvance for a record of a subtable at sub, with its
// device delta added: the advance down the page a run set upright applies.
func yAdvanceAt(sub []byte, at, format int, dv *deviceDeltas) int {
	if at < 0 || at > len(sub) {
		return 0
	}
	v := valueYAdvance(sub[at:], format)
	if dv == nil || format&0x0080 == 0 {
		return v
	}
	o := at + 2*popcount4(format) + 2*popcount4(format>>4&0x7)
	if o+2 > len(sub) {
		return v
	}
	return v + dv.at(sub, font.Be16(sub, o))
}

// popcount4 counts the bits of the low nibble of a ValueFormat: how many of the
// four fields each nibble names are present.
func popcount4(format int) int {
	n := 0
	for b := 0; b < 4; b++ {
		if format&(1<<b) != 0 {
			n++
		}
	}
	return n
}
