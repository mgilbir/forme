// Package brotli decompresses Brotli streams, RFC 7932.
//
// It is here for WOFF 2, which compresses a font's tables with Brotli and is
// how the web serves fonts. Nothing else in this engine needs it, and it only
// decompresses: a renderer reads fonts and never writes them.
//
// Brotli is LZ77 with three refinements, and each is a section below: prefix
// codes chosen per block rather than per stream (huffman.go), a context model
// that picks the code for a literal from the two bytes before it (the tables in
// tables.go), and a 122,784-byte dictionary of common text every stream may
// refer to as though it had already been decoded (dictionary.go). The last is
// why it beats gzip on a small file: a short document has little of its own to
// point back at.
package brotli

import "errors"

// Errors a malformed stream produces. They are distinguished because a font
// that fails to decompress is reported to the caller, and "the window is larger
// than RFC 7932 allows" and "this stopped in the middle" want different answers.
var (
	errTruncated     = errors.New("brotli: the stream ends in the middle of what it was saying")
	errPadding       = errors.New("brotli: the padding before a stored block is not zero")
	errReserved      = errors.New("brotli: a reserved bit is set")
	errExuberant     = errors.New("brotli: a length is written with more digits than it needs")
	errLargeWindow   = errors.New("brotli: a large window, which RFC 7932 does not define")
	errWindow        = errors.New("brotli: a window size outside the range RFC 7932 allows")
	errDistance      = errors.New("brotli: a reference to text before the start of the stream")
	errTooLarge      = errors.New("brotli: the stream decompresses to more than the caller allows")
	errNoBlockSwitch = errors.New("brotli: a switch to another block type where the meta-block declared only one")
)

// Decode decompresses a Brotli stream.
//
// limit bounds the output, and there is no default for it because there is no
// safe one: a Brotli stream states its window size but not its output size, and
// a few hundred bytes can decompress to gigabytes. A caller that knows the
// answer — WOFF 2 puts it in the header — should pass it exactly, and one that
// does not should pass what it is willing to spend.
func Decode(src []byte, limit int) ([]byte, error) {
	d := &decoder{r: &reader{src: src}, limit: limit}
	bits, err := windowBits(d.r)
	if err != nil {
		return nil, err
	}
	if bits < 10 || bits > 24 {
		return nil, errWindow
	}
	// The window is sixteen bytes short of a power of two, because the sixteen
	// shortest distance codes name recent distances rather than absolute ones
	// and the range they would have covered is spent on those instead.
	d.maxBack = 1<<uint(bits) - 16
	d.distRB = [4]int{16, 15, 11, 4}

	for {
		last, err := d.metaBlock()
		if err != nil {
			return nil, err
		}
		if last {
			break
		}
	}
	if d.r.overrun() {
		return nil, errTruncated
	}
	return d.out, nil
}

// windowBits reads RFC 7932 §9.1's window size, which is written in one bit
// when it is the common value and in up to seven otherwise.
func windowBits(r *reader) (int, error) {
	if r.take(1) == 0 {
		return 16, nil
	}
	if n := int(r.take(3)); n != 0 {
		return 17 + n, nil
	}
	switch n := int(r.take(3)); {
	case n == 1:
		// The bit pattern a large-window stream uses. Those are an extension
		// to RFC 7932 with windows up to a gigabyte, and no font is one.
		return 0, errLargeWindow
	case n != 0:
		return 8 + n, nil
	}
	return 17, nil
}
