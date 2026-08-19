package brotli

import (
	"errors"
	"strings"
	"testing"
)

// Prefix codes on their own.
//
// Everything here is reachable from a real stream, and most of it is reached by
// the streams in testdata — but the two refusals are not, because a stream the
// reference compressor produced never contains a code that does not add up.
// They are what stops a corrupt font from being decoded into something
// plausible instead of being rejected, so they are exercised directly.

// codeFor builds a reader over a string of "0" and "1" in the order the bits
// are read from the stream.
func codeFor(t *testing.T, bits string) *reader {
	t.Helper()
	w := &bitWriter{}
	for _, c := range bits {
		switch c {
		case '0':
			w.write(0, 1)
		case '1':
			w.write(1, 1)
		default:
			t.Fatalf("%q is not bits", bits)
		}
	}
	return &reader{src: w.out}
}

// TestACodeIsAssignedByLengthAndThenBySymbol, which is what "canonical" means
// and is the whole of what RFC 7932 says about which symbol gets which code.
func TestACodeIsAssignedByLengthAndThenBySymbol(t *testing.T) {
	for _, tc := range []struct {
		what    string
		lengths []byte
		codes   map[string]int
	}{
		{"lengths of one, two and three",
			[]byte{1, 2, 3, 3},
			map[string]int{"0": 0, "10": 1, "110": 2, "111": 3}},
		{"four of the same length, in order of the symbol",
			[]byte{2, 2, 2, 2},
			map[string]int{"00": 0, "01": 1, "10": 2, "11": 3}},
		{"symbols that are not adjacent",
			[]byte{0, 0, 1, 0, 0, 1},
			map[string]int{"0": 2, "1": 5}},
		{"a long code beside a short one",
			[]byte{1, 0, 0, 0, 0, 0, 0, 0, 2, 3, 3},
			map[string]int{"0": 0, "10": 8, "110": 9, "111": 10}},
	} {
		h, err := newHuffman(tc.lengths)
		if err != nil {
			t.Errorf("%s: %v", tc.what, err)
			continue
		}
		for bits, want := range tc.codes {
			// Trailing ones, so that reading too few bits is visible as a wrong
			// symbol rather than as a zero that happened to be right.
			r := codeFor(t, bits+strings.Repeat("1", 16))
			got, err := h.next(r)
			if err != nil {
				t.Errorf("%s: %s: %v", tc.what, bits, err)
				continue
			}
			if got != want {
				t.Errorf("%s: %s decoded to %d, want %d", tc.what, bits, got, want)
			}
			if int(r.consumed) != len(bits) {
				t.Errorf("%s: %s took %d bits, want %d",
					tc.what, bits, r.consumed, len(bits))
			}
		}
	}
}

// TestACodeOfOneSymbolReadsNothing. It is the degenerate case and it is real: a
// meta-block whose literals are all the same byte describes a code with one
// symbol in it, and spending a bit to choose between one thing would be a bit
// per byte of output.
func TestACodeOfOneSymbolReadsNothing(t *testing.T) {
	h, err := newHuffman([]byte{0, 0, 7, 0})
	if err != nil {
		t.Fatal(err)
	}
	r := codeFor(t, "1111111111111111")
	got, err := h.next(r)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("a code of one symbol decoded to %d, want 2", got)
	}
	if r.consumed != 0 {
		t.Errorf("a code of one symbol read %d bits", r.consumed)
	}
}

// TestACodeThatDoesNotAddUpIsRefused.
//
// Both directions matter and they fail differently. An over-full code puts two
// symbols on one bit pattern, so a stream using it has no single meaning; an
// under-full one leaves patterns spelling nothing, so a corrupt stream can
// wander into one and the decoder has no answer. Neither can come out of an
// encoder, and both can come out of a damaged file.
func TestACodeThatDoesNotAddUpIsRefused(t *testing.T) {
	for _, tc := range []struct {
		what    string
		lengths []byte
		want    error
	}{
		{"three symbols sharing two one-bit codes", []byte{1, 1, 1}, errOverfull},
		{"five symbols in four two-bit codes", []byte{2, 2, 2, 2, 2}, errOverfull},
		{"one bit pattern left spelling nothing", []byte{1, 2, 3}, errIncomplete},
		{"half the patterns left over", []byte{2, 2}, errIncomplete},
		{"no symbols at all", []byte{0, 0, 0}, errIncomplete},
		{"a length longer than RFC 7932 allows", []byte{1, 16}, errOverfull},
	} {
		_, err := newHuffman(tc.lengths)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", tc.what, err, tc.want)
		}
	}
	// And a code that does add up is built, so the checks above are not
	// refusing everything.
	if _, err := newHuffman([]byte{1, 2, 3, 3}); err != nil {
		t.Errorf("a complete code was refused: %v", err)
	}
}
