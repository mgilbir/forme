package font

import (
	"os"
	"testing"
)

// The ROS of a CID-keyed CFF: the character collection its CIDs are numbered
// in, which a PDF embedding the program has to state.
//
// ISO 32000-2 9.7.4.2 requires a CIDFont's /CIDSystemInfo to be compatible with
// the collection of its glyph source, so a consumer that cannot read this has
// to either guess or refuse. pdf0 refused, which is what this is for.

// TestCFFReportsItsCharacterCollection reads a real CID-keyed face, because the
// thing under test is a structure no synthetic fixture here builds: a ROS whose
// SIDs resolve through the font's own string index.
func TestCFFReportsItsCharacterCollection(t *testing.T) {
	data, err := os.ReadFile("../testdata/notocjk/NotoSansJP-Regular.otf")
	if err != nil {
		t.Skip("run `make notocjk` for a CID-keyed face to read: ", err)
	}
	p := ParseCFF(SFNTTables(data)["CFF "])
	if p == nil {
		t.Fatal("the CFF did not parse")
	}
	if p.GIDToCID == nil {
		t.Fatal("the face is not CID-keyed, so it cannot exercise the ROS")
	}
	// Noto's CJK faces are numbered in their own collection rather than in one
	// of Adobe's published ones, which is what Adobe-Identity-0 says. The point
	// of reading it is that a face saying Adobe-Japan1 must not be embedded as
	// this.
	if p.Registry != "Adobe" || p.Ordering != "Identity" || p.Supplement != 0 {
		t.Errorf("the ROS is %q-%q-%d, want Adobe-Identity-0",
			p.Registry, p.Ordering, p.Supplement)
	}
}

// smallCFF lays out a whole CFF whose Top DICT begins with the given bytes, so
// a test can say what the ROS is — or that there is none — and have everything
// after it still line up.
//
// The DICT is written twice, because the offsets it names are only known once
// the pieces are placed, and the second pass must not change its length or
// every one of those offsets moves under it. Each is three bytes whatever its
// value, and the fixture checks rather than trusts that.
//
// The string INDEX carries one string, so a SID of 391 resolves and 392 does
// not: that is the boundary a reader has to get right, and it cannot be tested
// against a font whose strings are empty.
func smallCFF(t testing.TB, topPrefix []byte) []byte {
	return smallCFFGlyphs(t, topPrefix, 1)
}

// smallCFFGlyphs is smallCFF with the glyph count said out loud, because zero
// is a case: a font can carry a ROS and no charstrings at all, and every slice
// sized from the glyph count is empty there.
func smallCFFGlyphs(t testing.TB, topPrefix []byte, glyphs int) []byte {
	t.Helper()
	const endchar = 14
	big := func(v int) []byte { return []byte{28, byte(v >> 8), byte(v)} }

	priv := privateDict(500, 0)
	top := func(csOff, privOff int) []byte {
		d := append([]byte(nil), topPrefix...)
		d = append(d, big(csOff)...)
		d = append(d, 17) // CharStrings
		d = append(d, big(len(priv))...)
		d = append(d, big(privOff)...)
		d = append(d, 18) // Private: size then offset
		return d
	}

	var data []byte
	data = append(data, 1, 0, 4, 1) // header: version 1.0, hdrSize 4, offSize 1
	data = append(data, cffIndexOf([]byte("Fixture"))...)
	topAt := len(data)
	data = append(data, cffIndexOf(top(0, 0))...)
	topLen := len(data) - topAt
	data = append(data, cffIndexOf([]byte("Custom"))...) // String INDEX: SID 391
	data = append(data, cffIndexOf()...)                 // Global Subr INDEX
	privOff := len(data)
	data = append(data, priv...)
	csOff := len(data)
	cs := make([][]byte, glyphs)
	for i := range cs {
		cs[i] = []byte{endchar}
	}
	data = append(data, cffIndexOf(cs...)...)

	final := cffIndexOf(top(csOff, privOff))
	if len(final) != topLen {
		t.Fatalf("the top DICT changed length (%d then %d); everything after it "+
			"has moved and the fixture describes a different font", topLen, len(final))
	}
	copy(data[topAt:], final)
	return data
}

// rosDict is a Top DICT prefix stating a ROS with the given operands.
func rosDict(operands ...int) []byte {
	var d []byte
	for _, v := range operands {
		d = append(d, cffOperand(v)...)
	}
	return append(d, 12, 30)
}

// TestAPlainCFFHasNoCharacterCollection: the fields are meaningless for a font
// whose glyphs are numbered by index, and must stay empty rather than carrying
// a default a caller might write into a document.
//
// The fixture is synthetic rather than a real face, because the assertion is
// about a font that has no ROS at all, and a test that reads a file which may
// not be there can pass by never running.
func TestAPlainCFFHasNoCharacterCollection(t *testing.T) {
	p := ParseCFF(smallCFF(t, nil))
	if p == nil {
		t.Fatal("ParseCFF refused the fixture")
	}
	if p.GIDToCID != nil {
		t.Fatal("the fixture states no ROS and was read as CID-keyed anyway, so " +
			"it cannot say anything about a font that is not")
	}
	if p.Registry != "" || p.Ordering != "" || p.Supplement != 0 {
		t.Errorf("a CFF that is not CID-keyed reported the collection %q-%q-%d",
			p.Registry, p.Ordering, p.Supplement)
	}
}

// TestTheROSNamesTheStringsTheFontCarries: the two SIDs resolve the way a glyph
// name does — the standard strings first, then the font's own INDEX — so a
// collection Adobe never published still comes back by name.
func TestTheROSNamesTheStringsTheFontCarries(t *testing.T) {
	// SID 391 is the first string past the standard ones, which is the one the
	// fixture carries; the supplement is an ordinary number and not a SID.
	p := ParseCFF(smallCFF(t, rosDict(390, 391, 5)))
	if p == nil {
		t.Fatal("ParseCFF refused the fixture")
	}
	if p.GIDToCID == nil {
		t.Fatal("the fixture states a ROS and was not read as CID-keyed")
	}
	if want := cffStandardStrings[390]; p.Registry != want {
		t.Errorf("registry is %q, want the standard string %q", p.Registry, want)
	}
	if p.Ordering != "Custom" {
		t.Errorf("ordering is %q, want %q from the font's own string INDEX",
			p.Ordering, "Custom")
	}
	if p.Supplement != 5 {
		t.Errorf("supplement is %d, want 5", p.Supplement)
	}
}

// TestAHostileROSIsReadWithoutPanicking.
//
// ParseCFF reads a font file, which is attacker-controlled binary whenever a
// document names its own. Every other SID in the format arrives as two unsigned
// bytes, but a ROS operand is a DICT operand and those are signed: the one-byte
// form alone covers -107 to 107. A negative one indexes the standard strings
// below zero.
//
// Nothing here asserts what the collection comes back as, because for these
// inputs there is no right answer — only that the parser survives and does not
// invent one.
func TestAHostileROSIsReadWithoutPanicking(t *testing.T) {
	for _, c := range []struct {
		name     string
		operands []int
	}{
		{"negative registry", []int{-1, 0, 0}},
		{"negative ordering", []int{0, -107, 0}},
		{"both negative", []int{-42, -42, -42}},
		{"past the string INDEX", []int{9999, 9999, 0}},
		{"one operand short", []int{0, 0}},
		{"no operands", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := ParseCFF(smallCFF(t, rosDict(c.operands...)))
			if p == nil {
				return
			}
			// A name it could not resolve must come back empty rather than as
			// whatever was next to it in memory.
			for _, s := range []string{p.Registry, p.Ordering} {
				if s == "" {
					continue
				}
				if len(c.operands) != 3 {
					t.Errorf("a ROS with %d operands named the collection %q",
						len(c.operands), s)
				}
			}
		})
	}
}

// TestACIDKeyedFontWithNoCharstringsStillSaysSo.
//
// GIDToCID is the field that answers "is this CID-keyed", and every consumer
// asks it that way. Built by appending onto a nil slice it comes back nil for a
// font with no glyphs — so a font carrying a ROS reported a character
// collection and, in the same breath, that it had no CIDs to number.
func TestACIDKeyedFontWithNoCharstringsStillSaysSo(t *testing.T) {
	p := ParseCFF(smallCFFGlyphs(t, rosDict(390, 391, 0), 0))
	if p == nil {
		t.Skip("ParseCFF refuses a font with no charstrings, which is also an answer")
	}
	if p.NumGlyphs != 0 {
		t.Fatalf("%d glyphs, want 0 — the fixture is not the one described", p.NumGlyphs)
	}
	if p.GIDToCID == nil {
		t.Error("a font stating a ROS reports GIDToCID nil, which every caller " +
			"reads as 'not CID-keyed' — and it went on to report a collection")
	}
}

// TestAROSItCannotFullyReadIsNotReported: the three fields move together, so a
// caller can test one and write all three. Reporting the half that parsed is
// how a document ends up declaring a numbering no font uses.
func TestAROSItCannotFullyReadIsNotReported(t *testing.T) {
	for _, c := range []struct {
		name     string
		operands []int
	}{
		// 391 is the one string this fixture carries, so 392 is the first that
		// resolves to nothing.
		{"registry names no string", []int{392, 391, 0}},
		{"ordering names no string", []int{390, 392, 0}},
		{"registry SID is negative", []int{-1, 391, 0}},
		// A supplement is a version of the collection and counts up from zero.
		{"supplement is negative", []int{390, 391, -91}},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := ParseCFF(smallCFF(t, rosDict(c.operands...)))
			if p == nil {
				t.Fatal("ParseCFF refused the fixture")
			}
			if p.GIDToCID == nil {
				t.Fatal("the fixture states a ROS and was not read as CID-keyed, " +
					"so it says nothing about how an unreadable one is reported")
			}
			if p.Registry != "" || p.Ordering != "" || p.Supplement != 0 {
				t.Errorf("a ROS this reader cannot fully make out came back as "+
					"%q-%q-%d", p.Registry, p.Ordering, p.Supplement)
			}
		})
	}
}
