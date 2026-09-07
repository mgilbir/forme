package brotli

import (
	"errors"
	"testing"
)

// A meta-block says how many bytes it holds, and the commands in it have to
// produce exactly that many.
//
// RFC 7932 §9.3: "The number of uncompressed bytes produced by the commands of
// a meta-block must be equal to MLEN." A command that would produce more is not
// a command whose tail is ignored — it is a stream no encoder wrote, and the
// reference decoder refuses it. This one stopped at MLEN and returned what it
// had, which is bytes past the declared length, written and kept.
//
// The fixture is built by hand because no encoder produces one, and both
// fixtures were run through the reference decoder first — which is what makes
// them a statement about the format rather than a restatement of this decoder's
// reading of it:
//
//	$ brotli -d < overproduce.br   # 2 declared, 5 inserted
//	corrupt input [con]            # exit 1
//	$ brotli -d < exact.br         # 5 declared, 5 inserted
//	AAAAA                          # exit 0

// simpleCode writes a prefix code of one symbol, which reads no bits when it is
// used. size is the alphabet, which decides how wide the symbol is written.
func (w *bitWriter) simpleCode(sym, size int) {
	w.write(1, 2) // HSKIP 1: a simple code
	w.write(0, 2) // one symbol
	width := uint(bitsFor(size - 1))
	if width == 0 {
		width = 1
	}
	w.write(uint32(sym), width)
}

// compressedBlock writes a meta-block of mlen bytes whose every command is the
// one at cmdSym and whose every literal is lit.
func (w *bitWriter) compressedBlock(mlen, cmdSym, lit int) {
	w.write(0, 1)               // not the last meta-block
	w.write(0, 2)               // four nibbles of length
	w.write(uint32(mlen-1), 16) //
	w.write(0, 1)               // compressed rather than stored
	w.write(0, 1)               // NBLTYPESL: one literal block type
	w.write(0, 1)               // NBLTYPESI: one insert-and-copy type
	w.write(0, 1)               // NBLTYPESD: one distance type
	w.write(0, 2)               // NPOSTFIX
	w.write(0, 4)               // NDIRECT
	w.write(0, 2)               // the literal block type's context mode
	w.write(0, 1)               // NTREESL: one literal code, so no map
	w.write(0, 1)               // NTREESD: one distance code, so no map
	w.simpleCode(lit, numLiteralSymbols)
	w.simpleCode(cmdSym, numCommandSymbols)
	w.simpleCode(0, numDistanceShort+(maxDistanceBits<<1))
}

// insertOnlyCommand is a command symbol whose insert length is n literals with
// no extra bits, so a fixture can state one exactly.
func insertOnlyCommand(t *testing.T, n int) int {
	t.Helper()
	for sym, c := range commandLut {
		if c.insertBits == 0 && c.insertOffset == n && c.copyBits == 0 {
			return sym
		}
	}
	t.Fatalf("no command code inserts exactly %d literals with no extra bits", n)
	return 0
}

// TestAMetaBlockThatProducesMoreThanItDeclaredIsRefused.
func TestAMetaBlockThatProducesMoreThanItDeclaredIsRefused(t *testing.T) {
	// Two bytes declared, five literals produced by the one command.
	w := &bitWriter{}
	w.write(0, 1) // a 16-bit window
	w.compressedBlock(2, insertOnlyCommand(t, 5), 'A')
	w.end()

	out, err := Decode(w.out, 1<<20)
	if !errors.Is(err, errOverproduced) {
		t.Errorf("a meta-block declaring 2 bytes and inserting 5 gave %q and %v, "+
			"want the overproduction refused", out, err)
	}
}

// TestAMetaBlockThatProducesExactlyWhatItDeclaredIsAccepted is the control, and
// it is the one that says the bound is on "more" rather than on "any". The last
// command of a meta-block is allowed to finish it with its insert alone and
// leave its copy unread, which is what this does.
func TestAMetaBlockThatProducesExactlyWhatItDeclaredIsAccepted(t *testing.T) {
	w := &bitWriter{}
	w.write(0, 1) // a 16-bit window
	w.compressedBlock(5, insertOnlyCommand(t, 5), 'A')
	w.end()

	out, err := Decode(w.out, 1<<20)
	if err != nil {
		t.Fatalf("a meta-block declaring 5 bytes and inserting 5 was refused: %v", err)
	}
	if string(out) != "AAAAA" {
		t.Errorf("it decoded to %q, want %q", out, "AAAAA")
	}
}

// TestAStreamEndsWhereItSaysItDoes.
//
// RFC 7932 §9.2 requires the bits from the end of the last meta-block to the
// end of the byte to be zero, and there is nothing after that byte: a stream is
// exactly as long as it says it is. Both were accepted, so bytes appended to a
// stream were read as a stream that ended where it did.
//
// Both fixtures were run through the reference decoder first:
//
//	$ brotli -d < padding.br    # exit 1, corrupt input
//	$ brotli -d < trailing.br   # exit 1, corrupt input
//	$ brotli -d < clean.br      # exit 0, "hi"
func TestAStreamEndsWhereItSaysItDoes(t *testing.T) {
	clean := func() *bitWriter {
		w := &bitWriter{}
		w.write(0, 1) // a 16-bit window
		w.storedBlock([]byte("hi"), 0)
		w.end()
		return w
	}

	if out, err := Decode(clean().out, 1<<20); err != nil || string(out) != "hi" {
		t.Fatalf("the clean stream gave %q and %v", out, err)
	}

	padded := clean()
	padded.pad(1)
	if out, err := Decode(padded.out, 1<<20); !errors.Is(err, errTailPadding) {
		t.Errorf("a stream whose tail padding is not zero gave %q and %v", out, err)
	}

	trailing := clean()
	trailing.pad(0)
	trailing.raw([]byte{0x41, 0x42})
	if out, err := Decode(trailing.out, 1<<20); !errors.Is(err, errTrailing) {
		t.Errorf("a stream with two bytes after it gave %q and %v", out, err)
	}

	// And a stream that ends exactly on a byte boundary needs no padding at
	// all, which is the case the check must not refuse.
	if _, err := Decode(clean().out, 1<<20); err != nil {
		t.Errorf("the clean stream was refused: %v", err)
	}
}

// TestAWordThatTransformsAwayToNothingIsNotAnError.
//
// A transform may leave nothing — OmitFirst9 over a four-letter word — and the
// reference decoder appends nothing and carries on. RFC 7932 states no rule
// against it at any distance.
//
// This refused one below a distance of 120 and allowed it above, on a threshold
// with no authority behind it: a stream the reference decodes came back as an
// error, and which answer you got depended on how far into the dictionary the
// reference reached. What stops a command producing nothing from meaning
// nothing forever is the reader running past the end of the input, which is
// TestAStreamThatProducesNothingForEverIsRefused.
func TestAWordThatTransformsAwayToNothingIsNotAnError(t *testing.T) {
	// Find a length and a transform that leave nothing: the word is four bytes
	// and the transform cuts more than four off the front.
	var index int
	found := false
	for kind, tr := range transforms {
		// omitFirst1 is 12, so cutting five or more off the front is 16 and up.
		if tr.prefix == "" && tr.suffix == "" &&
			tr.kind >= omitFirst1+4 && tr.kind <= omitFirst9 {
			index = kind << wordBits[4]
			found = true
			break
		}
	}
	if !found {
		t.Skip("no transform in the table cuts a four-letter word away to nothing")
	}
	out, err := word(nil, 4, index)
	if err != nil {
		t.Errorf("a word that transforms away to nothing gave %v", err)
	}
	if len(out) != 0 {
		t.Errorf("it produced %q, want nothing", out)
	}
}
