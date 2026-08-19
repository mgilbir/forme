package brotli

// The bit reader.
//
// Brotli packs bits into bytes least-significant first, so a value spanning two
// bytes has its low part in the earlier one. Prefix codes are the exception and
// are packed most-significant first, which is why they are read a bit at a time
// in huffman.go rather than peeked at whole.
//
// The whole stream is in memory — a font is a file, and the caller has it — so
// this reads from a slice and never blocks. What it does have to do is refuse a
// truncated stream, and the way it does that is worth stating: reading past the
// end yields zero bits rather than failing on the spot, and the count of bits
// handed out is checked against the input's length at the end. Failing on the
// spot would mean a bounds check in the innermost loop of the decoder; this way
// the check happens once, and a stream that ran off the end is still refused
// rather than silently decoded from imaginary zeroes.
type reader struct {
	src []byte
	pos int    // the next byte to move into acc
	acc uint64 // bits, least-significant first
	n   uint   // how many of acc's bits are valid

	// consumed counts every bit handed out, which is how a truncated stream is
	// caught: it may not exceed the input's own length in bits.
	consumed uint64
}

// fill tops the accumulator up to at least 57 bits, so any read of 24 bits or
// fewer — which is every read Brotli makes — is served without a second check.
func (r *reader) fill() {
	for r.n <= 56 {
		var b byte
		if r.pos < len(r.src) {
			b = r.src[r.pos]
		}
		r.pos++
		r.acc |= uint64(b) << r.n
		r.n += 8
	}
}

// take reads n bits, which must be 24 or fewer.
func (r *reader) take(n uint) uint32 {
	if r.n < n {
		r.fill()
	}
	v := uint32(r.acc & (1<<n - 1))
	r.acc >>= n
	r.n -= n
	r.consumed += uint64(n)
	return v
}

// bit reads one bit. It is take(1) without the branch, because prefix codes
// call it up to fifteen times per symbol and that is the decoder's hot loop.
func (r *reader) bit() uint32 {
	if r.n == 0 {
		r.fill()
	}
	v := uint32(r.acc & 1)
	r.acc >>= 1
	r.n--
	r.consumed++
	return v
}

// overrun reports whether more bits were handed out than the input holds, which
// means the stream was truncated and everything decoded near the end was read
// from zeroes this reader invented.
func (r *reader) overrun() bool {
	return r.consumed > uint64(len(r.src))*8
}

// align discards bits up to the next byte boundary and reports whether they
// were all zero, which RFC 7932 §9.2 requires of the padding before an
// uncompressed meta-block. A non-zero pad means the stream is not what it says.
func (r *reader) align() bool {
	pad := (8 - r.consumed%8) % 8
	return pad == 0 || r.take(uint(pad)) == 0
}

// bytes reads n whole bytes, which is only meaningful directly after align. It
// returns a slice of the input rather than a copy; the caller appends it to the
// output and does not hold it.
func (r *reader) bytes(n int) ([]byte, bool) {
	at := int(r.consumed / 8)
	if n < 0 || at+n > len(r.src) {
		return nil, false
	}
	out := r.src[at : at+n]
	// Move the whole reader past them: the accumulator holds bits that have
	// now been consumed, so it is emptied rather than shifted.
	r.consumed += uint64(n) * 8
	r.acc, r.n, r.pos = 0, 0, at+n
	return out, true
}
