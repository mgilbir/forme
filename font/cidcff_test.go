package font

import "testing"

// A CID-keyed CFF's Private DICTs, which are not where a font's usually are.
//
// An ordinary CFF has one Private DICT, named by the top DICT, and every
// charstring is read against it. A CID-keyed one has none there: it carries an
// FDArray of Font DICTs, each with a Private DICT of its own, and an FDSelect
// saying which glyph belongs to which. That is not a formality — such a font is
// usually several fonts merged, Latin and kana and han in one file, and the
// width defaults differ between them.
//
// It matters because of how Type 2 charstrings state widths. A charstring
// carries its width only when that width differs from its Private DICT's
// defaultWidthX, so the common case is that it says nothing and the default is
// the whole answer. A reader that looks for one Private DICT, does not find it,
// and falls back to zero therefore reports almost every glyph as zero units
// wide — which is what this package did before: 17,707 of Noto Sans JP's 17,936.
//
// The fixtures below are built by hand rather than read from a font, so the test
// runs with nothing fetched and says exactly which byte it is about.

// cffIndexOf assembles a CFF INDEX: a count, an offset size, the offsets
// (1-based, one past the end) and the items.
func cffIndexOf(items ...[]byte) []byte {
	out := []byte{byte(len(items) >> 8), byte(len(items)), 1}
	off := 1
	out = append(out, byte(off))
	for _, it := range items {
		off += len(it)
		out = append(out, byte(off))
	}
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

// cffOperand encodes a small non-negative integer the way a DICT does: 32 to 246
// are one byte biased by 139, and larger values use the 28 escape and two bytes.
func cffOperand(v int) []byte {
	if v >= -107 && v <= 107 {
		return []byte{byte(v + 139)}
	}
	return []byte{28, byte(v >> 8), byte(v)}
}

// privateDict is a Private DICT stating defaultWidthX (op 20) and
// nominalWidthX (op 21).
func privateDict(def, nom int) []byte {
	var b []byte
	b = append(b, cffOperand(def)...)
	b = append(b, 20)
	b = append(b, cffOperand(nom)...)
	b = append(b, 21)
	return b
}

// cidFont lays out a fixture: the two Private DICTs, an FDArray naming them, and
// an FDSelect, and returns the bytes with a top DICT map pointing into them.
func cidFont(t *testing.T, fdselect []byte, privs ...[]byte) ([]byte, map[int][]float64) {
	t.Helper()
	// A pad, so that nothing under test sits at offset zero, which a reader is
	// entitled to treat as "absent". It is filled with an operator that means
	// nothing here rather than with zeros: a pad of zeros parses as a run of
	// harmless operators, so a reader that started in the middle of it would
	// still arrive at the right Private DICT and the fixture would forgive the
	// error it exists to catch.
	data := make([]byte, 16)
	for i := range data {
		data[i] = 1 // Notice: an operator with no operands and no meaning here
	}

	offs := make([]int, len(privs))
	for i, p := range privs {
		offs[i] = len(data)
		data = append(data, p...)
	}

	fontDicts := make([][]byte, len(privs))
	for i := range privs {
		// Operator 18 takes size then offset, in that order.
		var d []byte
		d = append(d, cffOperand(len(privs[i]))...)
		d = append(d, cffOperand(offs[i])...)
		d = append(d, 18)
		fontDicts[i] = d
	}

	// A decoy Private DICT, immediately after the real ones and stating values
	// nothing should read. A Private DICT is bounded by its size and not by
	// anything in the bytes, so a reader that takes the size from the wrong
	// operand runs past the end of the one it wanted — and because a later
	// operator wins, it would land on the *right* values anyway if what followed
	// were empty. This is what makes that mistake visible.
	data = append(data, privateDict(999, 999)...)

	fdaOff := len(data)
	data = append(data, cffIndexOf(fontDicts...)...)
	fdsOff := len(data)
	data = append(data, fdselect...)

	return data, map[int][]float64{
		1230: {0, 0, 0},         // ROS: this is a CID-keyed font
		1236: {float64(fdaOff)}, // FDArray
		1237: {float64(fdsOff)}, // FDSelect
	}
}

func TestFDSelectFormat0NamesEveryGlyph(t *testing.T) {
	// The format byte, then one byte per glyph in glyph order — so six bytes
	// for five glyphs, not five.
	sel := []byte{0, 0, 0, 1, 1, 0}
	data, top := cidFont(t, sel, privateDict(500, 0), privateDict(1000, 7))

	fdOf, privs := parseCFFFDs(data, top, 5, true)
	if len(privs) != 2 {
		t.Fatalf("%d Private DICTs, want 2", len(privs))
	}
	if want := []int{0, 0, 1, 1, 0}; !sameInts(fdOf, want) {
		t.Errorf("glyphs went to FDs %v, want %v", fdOf, want)
	}
	if privs[0].def != 500 || privs[0].nom != 0 {
		t.Errorf("FD 0 is default=%v nominal=%v, want 500 and 0", privs[0].def, privs[0].nom)
	}
	if privs[1].def != 1000 || privs[1].nom != 7 {
		t.Errorf("FD 1 is default=%v nominal=%v, want 1000 and 7", privs[1].def, privs[1].nom)
	}
}

func TestFDSelectFormat3ReadsItsRanges(t *testing.T) {
	// Two ranges and the sentinel, which gives the glyph after the last range
	// rather than a range of its own: glyphs 0-2 in FD 1, glyphs 3-5 in FD 0.
	// The sentinel stops short of the last glyph on purpose. A sentinel equal to
	// the glyph count cannot show what it is for: every range but the last is
	// bounded by the next range's first glyph, so a reader that ignored the
	// sentinel and ran each range to the end of the font would be corrected by
	// the range after it and only the last would be wrong — and if that one ends
	// at the last glyph too, nothing is wrong at all.
	sel := []byte{
		3,
		0, 2, // nRanges
		0, 0, 0, // first=0, fd=0
		0, 3, 1, // first=3, fd=1
		0, 5, // sentinel: glyph 5 is past the end of the last range
	}
	data, top := cidFont(t, sel, privateDict(500, 0), privateDict(1000, 0))

	fdOf, _ := parseCFFFDs(data, top, 6, true)
	// Glyph 5 is outside every range and keeps the zero it started with. The
	// last range names FD 1 rather than FD 0 for that reason: a reader that ran
	// it to the end of the font would put glyph 5 in FD 0 as well, which is
	// where it already is, and the mistake would leave no trace.
	if want := []int{0, 0, 0, 1, 1, 0}; !sameInts(fdOf, want) {
		t.Errorf("glyphs went to FDs %v, want %v — the sentinel bounds the last "+
			"range and is not one", fdOf, want)
	}
}

// TestAGlyphTakesItsOwnFDsWidth is the whole point of the two tests above,
// stated as the number a caller actually reads.
//
// Two glyphs, neither charstring carrying a width, in Font DICTs whose defaults
// differ. Read against one Private DICT they would come back the same; read
// against their own they do not.
func TestAGlyphTakesItsOwnFDsWidth(t *testing.T) {
	sel := []byte{0, 0, 1}
	data, top := cidFont(t, sel, privateDict(500, 0), privateDict(1000, 0))
	fdOf, privs := parseCFFFDs(data, top, 2, true)

	// endchar alone: a charstring that states no width, which is the case the
	// default exists for.
	const endchar = 14
	for _, c := range []struct {
		gid  int
		want float64
	}{{0, 500}, {1, 1000}} {
		if _, has := type2CharstringWidth([]byte{endchar}, cffIndex{}, cffIndex{}); has {
			t.Fatal("the fixture charstring states a width; it must not, or the " +
				"default is never consulted and this proves nothing")
		}
		got := privs[fdOf[c.gid]].def
		if got != c.want {
			t.Errorf("glyph %d is in FD %d and takes default width %v, want %v",
				c.gid, fdOf[c.gid], got, c.want)
		}
	}
}

func TestANonCIDFontHasNoFDs(t *testing.T) {
	data, top := cidFont(t, []byte{0, 0}, privateDict(500, 0))
	// The same bytes, asked about as an ordinary font.
	if fdOf, privs := parseCFFFDs(data, top, 2, false); fdOf != nil || privs != nil {
		t.Error("an ordinary CFF was given FDs; its Private DICT is the top " +
			"DICT's own and reading an FDArray it does not have would be a guess")
	}
}

func TestAMissingFDSelectPutsEveryGlyphInTheFirstFD(t *testing.T) {
	// A CID font with an FDArray and no usable FDSelect. One Font DICT is what
	// that means; it is also the least wrong answer for a damaged font, and it
	// must not be an out-of-range index.
	data, top := cidFont(t, nil, privateDict(500, 0), privateDict(1000, 0))
	top[1237] = []float64{0} // FDSelect at offset zero: absent
	fdOf, privs := parseCFFFDs(data, top, 4, true)
	for g, fd := range fdOf {
		if fd < 0 || fd >= len(privs) {
			t.Fatalf("glyph %d went to FD %d, which does not exist", g, fd)
		}
	}
	if want := []int{0, 0, 0, 0}; !sameInts(fdOf, want) {
		t.Errorf("glyphs went to FDs %v, want %v", fdOf, want)
	}
}

func sameInts(a []int, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestParseCFFGivesEachGlyphItsOwnFDsWidth is the same rule through the door a
// caller actually uses.
//
// The tests above reach parseCFFFDs directly, which proves the FDs are read and
// proves nothing about whether the widths are taken from them: a reader that
// found every Font DICT correctly and then measured every glyph against the
// first would pass all of them. This builds a whole CID-keyed CFF and asks
// ParseCFF, which is what shape and the rules call.
func TestParseCFFGivesEachGlyphItsOwnFDsWidth(t *testing.T) {
	const endchar = 14 // a charstring stating no width, so the default decides

	// Two glyphs, in Font DICTs whose defaults differ by 500 units.
	privs := [][]byte{privateDict(500, 0), privateDict(1000, 0)}
	fdselect := []byte{0, 0, 1} // format 0: glyph 0 in FD 0, glyph 1 in FD 1

	// The pieces, laid down in the order a CFF has them so that every offset
	// below is the real distance rather than a guess.
	var data []byte
	data = append(data, 1, 0, 4, 1)                       // header: version 1.0, hdrSize 4, offSize 1
	data = append(data, cffIndexOf([]byte("Fixture"))...) // Name INDEX
	topAt := len(data)
	// The top DICT is written once the offsets it names are known, so a
	// placeholder of the right length goes in first and is overwritten below.
	// Every offset is three bytes wide whatever its value, so the placeholder
	// and the final DICT are the same length and nothing shifts underneath.
	big := func(v int) []byte { return []byte{28, byte(v >> 8), byte(v)} }
	topDictBig := func(csOff, fdaOff, fdsOff int) []byte {
		var d []byte
		d = append(d, cffOperand(0)...)
		d = append(d, cffOperand(0)...)
		d = append(d, cffOperand(0)...)
		d = append(d, 12, 30)
		d = append(d, big(csOff)...)
		d = append(d, 17)
		d = append(d, big(fdaOff)...)
		d = append(d, 12, 36)
		d = append(d, big(fdsOff)...)
		d = append(d, 12, 37)
		return d
	}
	placeholder := topDictBig(0, 0, 0)
	data = append(data, cffIndexOf(placeholder)...)
	topIndexLen := len(data) - topAt
	data = append(data, cffIndexOf()...) // String INDEX, empty
	data = append(data, cffIndexOf()...) // Global Subr INDEX, empty

	privAt := make([]int, len(privs))
	for i, p := range privs {
		privAt[i] = len(data)
		data = append(data, p...)
	}
	fontDicts := make([][]byte, len(privs))
	for i := range privs {
		var d []byte
		d = append(d, big(len(privs[i]))...)
		d = append(d, big(privAt[i])...)
		d = append(d, 18)
		fontDicts[i] = d
	}
	fdaOff := len(data)
	data = append(data, cffIndexOf(fontDicts...)...)
	fdsOff := len(data)
	data = append(data, fdselect...)
	csOff := len(data)
	data = append(data, cffIndexOf([]byte{endchar}, []byte{endchar})...)

	// Now the offsets are known, so the top DICT can say them.
	final := cffIndexOf(topDictBig(csOff, fdaOff, fdsOff))
	if len(final) != topIndexLen {
		t.Fatalf("the top DICT changed length (%d then %d); every offset after it "+
			"has moved and the fixture is describing a different font",
			topIndexLen, len(final))
	}
	copy(data[topAt:], final)

	fp := ParseCFF(data)
	if fp == nil {
		t.Fatal("ParseCFF refused the fixture")
	}
	if fp.NumGlyphs != 2 {
		t.Fatalf("%d glyphs, want 2", fp.NumGlyphs)
	}
	if fp.WidthByCID == nil {
		t.Fatal("the fixture was not read as CID-keyed, so the FDs were never consulted")
	}
	if got := fp.WidthByGID; got[0] != 500 || got[1] != 1000 {
		t.Errorf("widths came back %v, want [500 1000] — each glyph takes the "+
			"defaultWidthX of its own Font DICT, and equal widths mean one Private "+
			"DICT was used for both", got)
	}
}
