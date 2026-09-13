package shape

import (
	"strings"
	"testing"
)

// The CFF custom encoding, whose length is not written down anywhere.
//
// A CFF encoding maps codes to glyphs, and the subsetter has to copy it across
// whole. Nothing states how long it is: the length follows from the format, so
// sliceEncoding works it out and returns exactly that many bytes.
//
// Nothing had run it. Every CFF font in the corpora uses a predefined encoding —
// Standard or Expert, named by an offset of 0 or 1 — so the function returned at
// its first line and the whole custom-encoding reader below was at 0% across
// every unit test and all 6253 reftest documents: both formats, the supplements,
// and every bound.
//
// Getting the arithmetic wrong is not a crash. Too short and the subsetted font
// carries an encoding cut off partway, which maps codes to the wrong glyphs;
// too long and it carries whatever followed the encoding in the original font.
// Both produce a font that loads.
//
// The numbers below are the format's own (CFF spec, Appendix B):
//
//	format 0   format, nCodes, then one byte per code        2 + nCodes
//	format 1   format, nRanges, then two bytes per range     2 + 2*nRanges
//	bit 7 set  a supplement count and three bytes each       + 1 + 3*nSups

// encodingIn puts a blob at a known offset inside something larger, with a
// sentinel after it. A reader that returns too much returns some of the
// sentinel, which is what makes the length assertion mean anything.
func encodingIn(blob []byte) (data []byte, off int) {
	head := []byte("CFFHEADER")
	data = append(append([]byte(nil), head...), blob...)
	data = append(data, strings.Repeat("\xEE", 32)...)
	return data, len(head)
}

func TestACustomCFFEncodingIsSlicedToItsOwnLength(t *testing.T) {
	for _, c := range []struct {
		name string
		blob []byte
		want int
	}{
		{"format 0, no codes", []byte{0, 0}, 2},
		{"format 0, three codes", []byte{0, 3, 'a', 'b', 'c'}, 5},
		{"format 1, no ranges", []byte{1, 0}, 2},
		{"format 1, two ranges", []byte{1, 2, 'a', 4, 'x', 2}, 6},

		// Bit 7 says a supplement list follows the base encoding: a count and
		// three bytes for each, a code and a 16-bit SID.
		{"format 0 with one supplement", []byte{0x80, 1, 'a', 1, 'z', 0, 9}, 7},
		{"format 0 with two supplements", []byte{0x80, 1, 'a', 2, 'z', 0, 9, 'y', 0, 8}, 10},
		{"format 1 with one supplement", []byte{0x81, 1, 'a', 4, 1, 'z', 0, 9}, 8},
		{"format 0 with no supplements", []byte{0x80, 2, 'a', 'b', 0}, 5},
	} {
		t.Run(c.name, func(t *testing.T) {
			data, off := encodingIn(c.blob)
			got, err := sliceEncoding(data, off)
			if err != nil {
				t.Fatalf("reading it: %v", err)
			}
			if len(got) != c.want {
				t.Errorf("the encoding came out %d bytes, want %d: %x",
					len(got), c.want, got)
			}
			if len(got) == c.want && string(got) != string(c.blob) {
				t.Errorf("the bytes are %x, want %x", got, c.blob)
			}
			// The sentinel after it must not be in the answer, which is what a
			// length computed too generously would pull in.
			if strings.Contains(string(got), "\xEE") {
				t.Errorf("the encoding carries bytes from after it: %x", got)
			}
		})
	}
}

// A predefined encoding is named by its offset and has no bytes of its own, so
// there is nothing to copy and nothing to go wrong.
func TestAPredefinedCFFEncodingHasNoBytes(t *testing.T) {
	for off, name := range map[int]string{0: "Standard", 1: "Expert"} {
		got, err := sliceEncoding([]byte("anything at all"), off)
		if err != nil || got != nil {
			t.Errorf("the %s encoding gave %x and %v; it is named by its "+
				"offset and carries no bytes", name, got, err)
		}
	}
}

// Every way the bytes can run out, because the length is computed from a count
// the font states and the font is untrusted.
func TestATruncatedCFFEncodingIsRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		data []byte
		off  int
	}{
		{"the offset is past the end", []byte("short"), 99},
		{"the offset is the end", []byte("short"), 5},
		{"format 0 with no count", []byte("CFFHEADER\x00"), 9},
		{"format 1 with no count", []byte("CFFHEADER\x01"), 9},
		{"format 0 claiming more codes than there are", []byte("CFFHEADER\x00\x20ab"), 9},
		{"format 1 claiming more ranges than there are", []byte("CFFHEADER\x01\x20ab"), 9},
		{"a supplement count past the end", []byte("CFFHEADER\x80\x02ab"), 9},
		{"supplements claiming more than there are", []byte("CFFHEADER\x80\x02ab\x20"), 9},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := sliceEncoding(c.data, c.off)
			if err == nil {
				t.Errorf("it was read as %d bytes rather than refused; the "+
					"count is the font's and the font is untrusted", len(got))
			}
		})
	}
}

// A format the specification does not define is refused rather than guessed at,
// because its length cannot be worked out and a guess would copy the wrong
// number of bytes into the subsetted font.
func TestAnUnknownCFFEncodingFormatIsRefused(t *testing.T) {
	for _, format := range []byte{2, 3, 0x7F, 0x82, 0xFF} {
		data, off := encodingIn([]byte{format, 1, 'a', 'b', 'c', 'd'})
		if got, err := sliceEncoding(data, off); err == nil {
			t.Errorf("encoding format %d was read as %d bytes; only 0 and 1 "+
				"are defined and a length cannot be guessed", format, len(got))
		}
	}
}
