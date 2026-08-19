package brotli

import (
	"errors"
	"testing"
	"time"
)

// The parts of RFC 7932 §9.2 that no encoder produces, built by hand.
//
// A stored meta-block, a metadata meta-block, and the two ways a length may be
// written illegally are all in the format and none of them comes out of the
// reference compressor at any setting, so no captured stream reaches them. What
// makes these fixtures trustworthy rather than a restatement of this decoder's
// own reading is that the reference decoder was run on each one first: it
// produces "hello, stored world" for the two that are legal and refuses the
// three that are not, which is exactly what is asserted below.

// bitWriter writes a Brotli stream the way one is packed: bits into bytes,
// least-significant first.
type bitWriter struct {
	out []byte
	n   uint
}

func (w *bitWriter) write(v uint32, bits uint) {
	for i := uint(0); i < bits; i++ {
		if w.n%8 == 0 {
			w.out = append(w.out, 0)
		}
		if v&(1<<i) != 0 {
			w.out[len(w.out)-1] |= 1 << (w.n % 8)
		}
		w.n++
	}
}

// pad fills to the next byte boundary. The bit is a parameter because what goes
// in the padding is the thing under test in one of the cases below.
func (w *bitWriter) pad(bit uint32) {
	for w.n%8 != 0 {
		w.write(bit, 1)
	}
}

func (w *bitWriter) raw(b []byte) {
	w.out = append(w.out, b...)
	w.n += uint(len(b)) * 8
}

// end writes the empty last meta-block that finishes a stream.
func (w *bitWriter) end() { w.write(1, 1); w.write(1, 1) }

// storedBlock writes a meta-block holding bytes as they are.
func (w *bitWriter) storedBlock(payload []byte, padBit uint32) {
	w.write(0, 1)                       // not the last meta-block
	w.write(0, 2)                       // its length is written in four nibbles
	w.write(uint32(len(payload)-1), 16) // and is one less than it is
	w.write(1, 1)                       // stored rather than compressed
	w.pad(padBit)
	w.raw(payload)
}

// metadataBlock writes bytes that are not part of the output.
func (w *bitWriter) metadataBlock(meta []byte, reserved uint32) {
	w.write(0, 1)                   // not the last meta-block
	w.write(3, 2)                   // metadata rather than data
	w.write(reserved, 1)            // a bit RFC 7932 §9.2 reserves
	w.write(1, 2)                   // its length is written in one byte
	w.write(uint32(len(meta)-1), 8) //
	w.pad(0)
	w.raw(meta)
}

const storedText = "hello, stored world"

// TestAStoredBlockIsCopiedThrough. Incompressible data is stored rather than
// compressed, and every stream holding a JPEG has one of these in it.
func TestAStoredBlockIsCopiedThrough(t *testing.T) {
	w := &bitWriter{}
	w.write(0, 1) // a 16-bit window
	w.storedBlock([]byte(storedText), 0)
	w.end()
	got, err := Decode(w.out, 1<<20)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(got) != storedText {
		t.Errorf("a stored block came out as %q, want %q", got, storedText)
	}
}

// TestThePaddingBeforeAStoredBlockMustBeZero.
//
// The bits between the header and the bytes are there to reach a byte boundary
// and §9.2 requires them to be zero. Nothing downstream depends on their value,
// which is exactly why they are worth checking: a stream where they are not
// zero is not the stream it says it is, and the cheapest place to find that out
// is here.
func TestThePaddingBeforeAStoredBlockMustBeZero(t *testing.T) {
	w := &bitWriter{}
	w.write(0, 1)
	w.storedBlock([]byte(storedText), 1)
	w.end()
	if _, err := Decode(w.out, 1<<20); !errors.Is(err, errPadding) {
		t.Errorf("padding of ones gave %v, want %v", err, errPadding)
	}
}

// TestMetadataIsSteppedOverRatherThanOutput. A metadata meta-block carries
// bytes for whatever wrapped the stream — not for whoever decompresses it — and
// they must not reach the output or shift what follows them.
func TestMetadataIsSteppedOverRatherThanOutput(t *testing.T) {
	w := &bitWriter{}
	w.write(0, 1)
	w.metadataBlock([]byte("skip me"), 0)
	w.storedBlock([]byte(storedText), 0)
	w.end()
	got, err := Decode(w.out, 1<<20)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(got) != storedText {
		t.Errorf("the output is %q, want %q — metadata is not content", got, storedText)
	}
}

// TestAReservedBitIsRefused. §9.2 reserves one bit of a metadata header. A
// stream that sets it means something this does not implement, and reading on
// as though it did not would be a guess.
func TestAReservedBitIsRefused(t *testing.T) {
	w := &bitWriter{}
	w.write(0, 1)
	w.metadataBlock([]byte("skip me"), 1)
	w.storedBlock([]byte(storedText), 0)
	w.end()
	if _, err := Decode(w.out, 1<<20); !errors.Is(err, errReserved) {
		t.Errorf("a set reserved bit gave %v, want %v", err, errReserved)
	}
}

// TestALengthWrittenWithMoreNibblesThanItNeedsIsRefused. A meta-block's length
// has one spelling, and §9.2 makes the second one illegal rather than
// equivalent: a decoder that accepted both would accept two streams for one
// document, which is how a signature over compressed bytes stops meaning
// anything.
func TestALengthWrittenWithMoreNibblesThanItNeedsIsRefused(t *testing.T) {
	w := &bitWriter{}
	w.write(0, 1)
	w.write(0, 1)                          // not the last meta-block
	w.write(1, 2)                          // five nibbles of length
	w.write(uint32(len(storedText)-1), 20) // where four would have done
	w.write(1, 1)                          // stored
	w.pad(0)
	w.raw([]byte(storedText))
	w.end()
	if _, err := Decode(w.out, 1<<20); !errors.Is(err, errExuberant) {
		t.Errorf("a length with a leading zero nibble gave %v, want %v", err, errExuberant)
	}
}

// TestAnEmptyStreamIsAStream: the shortest legal Brotli there is.
func TestAnEmptyStreamIsAStream(t *testing.T) {
	w := &bitWriter{}
	w.write(0, 1)
	w.end()
	got, err := Decode(w.out, 1<<20)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(got) != 0 {
		t.Errorf("an empty stream produced %d bytes", len(got))
	}
}

// TestTheBlockCountCodeIsRFC7932s, §9.3's table written out as ranges.
//
// The decoder holds it as a start and a width, which is the form it is used in
// and not the form the specification states it in. The two have to agree, and
// the ranges also have to tile: every count from one upwards is expressible
// exactly once, which is the property that makes the table a code at all.
func TestTheBlockCountCodeIsRFC7932s(t *testing.T) {
	from := [26]int{1, 5, 9, 13, 17, 25, 33, 41, 49, 65, 81, 97, 113, 145,
		177, 209, 241, 305, 369, 497, 753, 1265, 2289, 4337, 8433, 16625}
	bits := [26]uint{2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5,
		5, 5, 6, 6, 7, 8, 9, 10, 11, 12, 13, 24}
	if len(blockLenRange) != 26 {
		t.Fatalf("%d block-count symbols, want 26", len(blockLenRange))
	}
	for i, got := range blockLenRange {
		if got.offset != from[i] || got.bits != bits[i] {
			t.Errorf("block count %d starts at %d with %d extra bits, want %d and %d",
				i, got.offset, got.bits, from[i], bits[i])
		}
		if i > 0 {
			if want := from[i-1] + 1<<bits[i-1]; from[i] != want {
				t.Errorf("block count %d starts at %d and the one before it ends "+
					"at %d; the ranges have to meet", i, from[i], want)
			}
		}
	}
	// The largest block a meta-block can hold is smaller than the largest count
	// this can express, so no count ever runs out.
	if last := from[25] + 1<<bits[25]; last <= blockLengthCap {
		t.Errorf("the largest block count is %d and a meta-block may hold %d",
			last, blockLengthCap)
	}
}

// TestAStreamThatProducesNothingForEverIsRefused.
//
// This is a crafted stream and it is worth saying exactly what it does, because
// nothing found it by chance: nine hundred thousand fuzzed inputs went past it.
//
// Every part of it is legal. It declares one prefix code for each of the three
// things a meta-block needs and every one of those codes has a single symbol in
// it, which RFC 7932 allows and which costs no bits to read. The command that
// single symbol names inserts nothing and copies four bytes from a distance
// that is past the window — so it is a reference to the static dictionary, word
// 1020 of the four-letter ones, transform 63, which cuts five letters off a
// four-letter word. What that transform leaves is nothing.
//
// §8 tolerates a reference that comes to nothing when the distance is large,
// which this one is. So the command is legal, produces no output, and moves the
// meta-block no closer to its stated length. The stream then simply stops, and
// past the end the bit reader hands out zeroes, which decode to that same
// command again, for ever.
//
// It used to run sixteen million commands and then read through a prefix code
// the meta-block never described, which is a nil pointer.
func TestAStreamThatProducesNothingForEverIsRefused(t *testing.T) {
	w := &bitWriter{}
	w.write(0, 1)      // a 16-bit window
	w.write(0, 1)      // not the last meta-block
	w.write(0, 2)      // four nibbles of length
	w.write(65535, 16) // 65536 bytes to produce
	w.write(0, 1)      // compressed
	w.write(0, 1)      // one literal block type
	w.write(0, 1)      // one command block type
	w.write(0, 1)      // one distance block type
	w.write(0, 6)      // no postfix bits and no direct distances
	w.write(0, 2)      // the LSB6 context model
	w.write(0, 1)      // one literal prefix code
	w.write(0, 1)      // one distance prefix code
	simple := func(symbol uint32, width uint) {
		w.write(1, 2) // a simple code
		w.write(0, 2) // of one symbol
		w.write(symbol, width)
	}
	simple('A', 8)  // a literal no command ever inserts
	simple(130, 10) // insert nothing, copy four, spell out the distance
	simple(44, 6)   // a distance whose range starts past the window
	// And there the stream ends.

	done := make(chan error, 1)
	go func() {
		_, err := Decode(w.out, 1<<20)
		done <- err
	}()
	select {
	case err := <-done:
		// Truncation is the reason it stops, and asserting which reason
		// matters: the other guard against this — refusing a block switch a
		// meta-block described no code for — would also stop it, sixteen
		// million commands later.
		if !errors.Is(err, errTruncated) {
			t.Errorf("gave %v, want %v", err, errTruncated)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("did not finish")
	}
}

// TestASwitchToABlockTypeThatWasNeverDescribedIsRefused, directly, because the
// stream above is stopped by the truncation check before it gets here and no
// stream small enough to keep reaches it: the counter that leads here is larger
// than any meta-block, so sixteen million commands have to go by first.
func TestASwitchToABlockTypeThatWasNeverDescribedIsRefused(t *testing.T) {
	var b blocks
	b.types = [3]int{1, 1, 1}
	for which := 0; which < 3; which++ {
		err := b.switchBlock(&reader{src: []byte{0, 0, 0, 0}}, which)
		if !errors.Is(err, errNoBlockSwitch) {
			t.Errorf("switching block %d gave %v, want %v", which, err, errNoBlockSwitch)
		}
	}
}
