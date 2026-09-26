package font

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A Type 1 font program whose eexec portion is hexadecimal.
//
// The format allows either form — binary, or the same bytes written as hex
// digits — and a font that had to survive a channel that was not eight-bit
// clean is in the second. ParseType1 detects it by looking at the first four
// bytes after "eexec" and unhexes when all four are hex digits.
//
// Nothing had ever run that path. decodeHexBytes was at 0% coverage: every
// fixture and every corpus font is in the binary form, so the detection branch
// was taken nowhere and a reader that unhexed nothing, or unhexed wrongly, would
// have gone on passing. What it would look like in a document is not an error
// but a font with no glyphs in it, since the decrypted text would be noise and
// no charstring name would be found in it.
func TestAType1ProgramInHexadecimalIsRead(t *testing.T) {
	want := []string{"A", "B", "C", "D", "E"}

	// The binary form first, so the comparison is against this reader's own
	// answer for the same program rather than against a list written twice.
	binary := ParseType1(fonttest.Type1Program(want))
	if binary == nil {
		t.Fatal("the binary form was not read at all")
	}

	for _, c := range []struct {
		name string
		opts fonttest.Type1Hex
	}{
		{"one line", fonttest.Type1Hex{}},
		{"wrapped at 64, as Adobe's tools write it", fonttest.Type1Hex{Wrap: 64}},
		{"wrapped at 4, a break the moment the format allows one", fonttest.Type1Hex{Wrap: 4}},
		{"wrapped at 7, so a line break falls inside a byte", fonttest.Type1Hex{Wrap: 7}},
		{"upper case", fonttest.Type1Hex{Upper: true}},
		{"upper case, wrapped at 64", fonttest.Type1Hex{Wrap: 64, Upper: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			fp := ParseType1(fonttest.Type1ProgramHex(want, c.opts))
			if fp == nil {
				t.Fatal("the hexadecimal form was not read at all")
			}
			for _, n := range want {
				if !fp.GlyphNames[n] {
					t.Errorf("the glyph %q is missing; the same program in "+
						"binary declares it, so the hex was decoded wrongly "+
						"and the decrypted text is noise", n)
				}
			}
			if len(fp.GlyphNames) != len(binary.GlyphNames) {
				t.Errorf("the hexadecimal form declares %d glyphs and the "+
					"binary form %d: %v", len(fp.GlyphNames),
					len(binary.GlyphNames), fp.GlyphNames)
			}
		})
	}
}

// The detection window is four bytes, and that is the format's rule rather than
// this reader's shortcut.
//
// The Type 1 spec says the form is told apart by examining the first four bytes
// after "eexec": all four hexadecimal means the hex form. It is a rule with a
// duty on the writer — a binary program must not open with four hex digits, and
// a hex one must not break its line inside them — and a reader cannot do better,
// because the two forms are otherwise indistinguishable.
//
// So a hex program with white space in that window is not a hex program. This
// pins the boundary, because widening the window is exactly the "fix" somebody
// reaches for on meeting a font like this, and it would make every binary
// program whose fifth byte is a digit ambiguous.
func TestTheHexFormIsDetectedOnFourBytesAndNoMore(t *testing.T) {
	want := []string{"A", "B"}
	if fp := ParseType1(fonttest.Type1ProgramHex(want, fonttest.Type1Hex{Wrap: 4})); fp == nil {
		t.Error("a program wrapped at 4 was not read; the break is after the " +
			"detection window, so the four bytes are digits and this is hex")
	}
	if fp := ParseType1(fonttest.Type1ProgramHex(want, fonttest.Type1Hex{Wrap: 1})); fp != nil &&
		len(fp.GlyphNames) > 0 {
		t.Errorf("a program wrapped at 1 was read as hexadecimal and gave %d "+
			"glyphs; a newline inside the four-byte window means the format "+
			"does not call this the hex form, and taking it would make the "+
			"binary form ambiguous", len(fp.GlyphNames))
	}
}

// White space is not the only thing between the digits. The format's hex strings
// tolerate anything that is not a digit, and a font carried through a PDF may
// have picked up a comment or a stray delimiter.
func TestAType1ProgramInHexadecimalToleratesRubbishBetweenTheDigits(t *testing.T) {
	want := []string{"A", "B"}
	hex := fonttest.Type1ProgramHex(want, fonttest.Type1Hex{})

	// Sprinkle non-digits through the hex, after the header so that the
	// four-byte detection still sees digits.
	head := len("%!PS-AdobeFont-1.0\n/FontMatrix [0.001 0 0 0.001 0 0] readonly def\ncurrentfile eexec\n")
	var dirty []byte
	dirty = append(dirty, hex[:head+8]...)
	for i, b := range hex[head+8:] {
		dirty = append(dirty, b)
		if i%5 == 4 {
			dirty = append(dirty, " \t\r\n%>"[i/5%6])
		}
	}

	fp := ParseType1(dirty)
	if fp == nil {
		t.Fatal("a hexadecimal program with non-digits in it was not read at all")
	}
	for _, n := range want {
		if !fp.GlyphNames[n] {
			t.Errorf("the glyph %q is missing; a hex string ignores what is "+
				"not a digit, so this is the same program", n)
		}
	}
}

// An odd number of digits is a damaged program, and the point is that the reader
// stays inside it.
//
// A hex string with an odd digit count is padded with a trailing zero, which
// changes the final byte — so the decrypted text is the right program with its
// last byte wrong. There is no right answer to give; what there is to hold is
// that the reader neither panics nor runs past the end, and answers something.
//
// This cannot tell padding from dropping the digit, because the damage lands in
// a trailer nothing parses either way: planting "drop it instead" leaves every
// assertion here passing. The padding rule itself is pinned a few lines down,
// against the decoder rather than through a whole font.
func TestAnOddNumberOfHexDigitsIsNotFatal(t *testing.T) {
	want := []string{"A", "B", "C"}
	fp := ParseType1(fonttest.Type1ProgramHex(want, fonttest.Type1Hex{Odd: true}))
	if fp == nil {
		t.Fatal("an odd digit count gave no program at all; it is padded with " +
			"a trailing zero, so the program is damaged in its last byte and " +
			"readable up to there")
	}
	// The damage is in the last byte of a trailer, well past the charstrings,
	// so the glyphs are still there. That is worth stating: if it ever stops
	// being true the padding rule has changed.
	for _, n := range want {
		if !fp.GlyphNames[n] {
			t.Errorf("the glyph %q is missing; the padded byte is the last one "+
				"of the program and the charstrings are before it", n)
		}
	}
}

// The decoder's own rules, at the level they are stated.
//
// Through a font these are invisible: a program is a few hundred bytes and the
// places a single wrong one shows are few, so "pad the odd digit" and "drop it"
// give the same glyphs. They are the rules a PDF hex string is defined by, so
// they are held directly.
func TestDecodeHexBytes(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		want []byte
	}{
		{"lower case", "48656c6c6f", []byte("Hello")},
		{"upper case", "48656C6C6F", []byte("Hello")},
		{"mixed case", "48656C6c6F", []byte("Hello")},
		{"white space between digits", "48 65\t6c\r\n6c 6f", []byte("Hello")},
		{"white space inside a byte", "4 8 6 5", []byte("He")},
		{"anything that is not a digit", "48%65>6c(6c)6f", []byte("Hello")},
		// §7.3.4.3: "if the final digit is missing ... it shall be assumed to
		// be 0". Dropping it instead would give one byte fewer and a different
		// last byte, and both readings are silent.
		{"an odd digit count is padded with a zero", "48656c6c6", []byte("Hell`")},
		{"one digit", "4", []byte{0x40}},
		{"no digits at all", "", []byte{}},
		{"nothing but rubbish", " \t\r\n%>()", []byte{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := decodeHexBytes([]byte(c.in))
			if string(got) != string(c.want) {
				t.Errorf("decoding %q gave %q (% x), want %q (% x)",
					c.in, got, got, c.want, c.want)
			}
		})
	}
}
