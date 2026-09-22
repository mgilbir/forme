package brotli

import (
	"encoding/hex"
	"errors"
	"testing"
)

// An empty metadata meta-block still pads to a byte boundary.
//
// RFC 7932 §9.2 writes a metadata header as the reserved bit, MSKIPBYTES,
// MSKIPBYTES bytes of length, and then "0 - 7 bits: fill bits until the next
// byte boundary, must be all zeros" — after the length, whatever the length
// is. This returned as soon as MSKIPBYTES was zero, before the fill, and read
// the next meta-block's header out of it.
//
// It was seen from both sides and read as two things. A stream the reference
// decodes was refused, and a stream the reference refuses was read as empty;
// the second was taken for evidence that the reference is stricter than the
// RFC, when it is the same defect. Every stream here is one brotli -d 1.2.0
// was asked about, and the answer it gave is the one wanted.
func TestAnEmptyMetadataBlockPadsToAByte(t *testing.T) {
	for _, tc := range []struct {
		stream, want string
		err          error
	}{
		// Empty metadata, zero fill, then a stored block of "hello". The
		// reference prints hello.
		{"0c20000868656c6c6f03", "hello", nil},
		// Empty metadata, then the empty last meta-block. The reference
		// prints nothing and succeeds.
		{"0c03", "", nil},
		// Empty metadata as the last meta-block. Likewise.
		{"1a", "", nil},
		// Empty metadata whose fill bit is set, then ISLAST and ISLASTEMPTY.
		// The reference says "corrupt input".
		{"8c01", "", errPadding},
	} {
		in, err := hex.DecodeString(tc.stream)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Decode(in, 1<<20)
		switch {
		case tc.err != nil && !errors.Is(err, tc.err):
			t.Errorf("%s: got (%q, %v), want %v", tc.stream, got, err, tc.err)
		case tc.err == nil && (err != nil || string(got) != tc.want):
			t.Errorf("%s: got (%q, %v), want %q", tc.stream, got, err, tc.want)
		}
	}
}

// TestEveryFillBitOfAnEmptyMetadataBlockIsChecked: the header of an empty
// metadata block ends at a different bit depending on what came before it, so
// the fill is a different number of bits, and each of them must be zero. The
// window size is written in one, four or seven bits, and a stored block before
// the metadata leaves the stream on a byte boundary; between them the fill is
// one, two, three or six bits.
func TestEveryFillBitOfAnEmptyMetadataBlockIsChecked(t *testing.T) {
	windows := []struct {
		v    uint32
		bits uint
	}{
		{0, 1},               // WBITS 16
		{1 | 1<<1, 4},        // WBITS 18
		{1 | 0<<1 | 2<<4, 7}, // WBITS 10
	}
	fills := map[uint]bool{}
	for _, win := range windows {
		for _, stored := range []bool{false, true} {
			header := func(w *bitWriter) uint {
				w.write(win.v, win.bits)
				if stored {
					w.storedBlock([]byte("x"), 0)
				}
				w.write(0, 1) // not the last meta-block
				w.write(3, 2) // metadata
				w.write(0, 1) // the reserved bit
				w.write(0, 2) // MSKIPBYTES = 0: no metadata at all
				return (8 - w.n%8) % 8
			}
			// With the fill all zeros the stream is good, which is what
			// makes a refusal below about the one bit that was set.
			w := &bitWriter{}
			fill := header(w)
			fills[fill] = true
			w.write(0, fill)
			w.end()
			if _, err := Decode(w.out, 1<<20); err != nil {
				t.Errorf("window %d bits, stored %v: a zero fill of %d was refused: %v",
					win.bits, stored, fill, err)
			}
			for bit := uint(0); bit < fill; bit++ {
				w := &bitWriter{}
				header(w)
				w.write(1<<bit, fill)
				w.end()
				if _, err := Decode(w.out, 1<<20); !errors.Is(err, errPadding) {
					t.Errorf("window %d bits, stored %v: fill bit %d of %d set gave %v, want %v",
						win.bits, stored, bit, fill, err, errPadding)
				}
			}
		}
	}
	for _, f := range []uint{1, 2, 3, 6} {
		if !fills[f] {
			t.Errorf("no case had a fill of %d bits; the cases have stopped covering what they say", f)
		}
	}
}
