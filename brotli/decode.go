package brotli

// The meta-block loop: RFC 7932 §9.
//
// A stream is a sequence of meta-blocks, each of which describes its own prefix
// codes and then spends them on a sequence of commands. A command is "insert
// this many literals, then copy this many bytes from this far back" — the two
// halves in one symbol, because they correlate, and that is most of why Brotli
// is smaller than DEFLATE at the same window size.

const (
	numLiteralSymbols  = 256
	numCommandSymbols  = 704
	numBlockLenSymbols = 26
	numDistanceShort   = 16
	maxDistanceBits    = 24

	// The largest distance a stream may name. Chosen so that adding three to a
	// remembered distance — which the short codes do — cannot overflow.
	maxAllowedDistance = 0x7FFFFFFC

	// A meta-block's length is at most six nibbles, so no block-length count
	// can reach this and one set to it never expires.
	blockLengthCap = 1 << 24
)

type decoder struct {
	r     *reader
	out   []byte
	limit int

	maxBack int

	// The four most recent distances. Repeating one costs a two-bit code
	// rather than a full distance, and structured data repeats them constantly
	// — a table of records refers back one record at a time.
	distRB  [4]int
	distIdx int
}

// extend makes room for n more bytes and returns where they start.
func (d *decoder) extend(n int) (int, bool) {
	at := len(d.out)
	if n < 0 || at+n > d.limit || at+n < at {
		return 0, false
	}
	if cap(d.out) < at+n {
		size := 2 * cap(d.out)
		if size < at+n {
			size = at + n
		}
		if size < 4096 {
			size = 4096
		}
		bigger := make([]byte, at, size)
		copy(bigger, d.out)
		d.out = bigger
	}
	d.out = d.out[:at+n]
	return at, true
}

// blocks is the block-switching state of one meta-block, for each of the three
// things that can switch: literals, commands and distances.
//
// A block type is a choice of prefix code that lasts for a stated number of
// symbols. It is what lets one meta-block hold both a run of English and a run
// of base64 without paying for a code that describes their union.
type blocks struct {
	types    [3]int
	length   [3]int
	typeCode [3]*huffman
	lenCode  [3]*huffman
	recent   [3][2]int
	current  [3]int
}

// blockLenRange is RFC 7932 §9.3's block-count code: a symbol, and how many
// extra bits follow it, and what the count starts at.
var blockLenRange = [numBlockLenSymbols]struct {
	offset int
	bits   uint
}{
	{1, 2}, {5, 2}, {9, 2}, {13, 2}, {17, 3}, {25, 3}, {33, 3}, {41, 3},
	{49, 4}, {65, 4}, {81, 4}, {97, 4}, {113, 5}, {145, 5}, {177, 5}, {209, 5},
	{241, 6}, {305, 6}, {369, 7}, {497, 8}, {753, 9}, {1265, 10}, {2289, 11},
	{4337, 12}, {8433, 13}, {16625, 24},
}

func readBlockLength(r *reader, h *huffman) (int, error) {
	sym, err := h.next(r)
	if err != nil {
		return 0, err
	}
	v := blockLenRange[sym]
	return v.offset + int(r.take(v.bits)), nil
}

// switchBlock reads the next block type and how long it lasts.
//
// The type is coded relative to what came before: symbol 0 means the type
// before last and symbol 1 means one past the last, which between them cover
// almost every switch a real encoder makes.
func (b *blocks) switchBlock(r *reader, which int) error {
	// A meta-block with one block type describes no code for switching between
	// them, so a switch cannot happen and asking for one is a malformed stream.
	// Saying so here rather than reading through a code that was never built is
	// not a nicety: the count that runs down to this is larger than any
	// meta-block, so only a stream that has already gone wrong arrives.
	if b.types[which] < 2 {
		return errNoBlockSwitch
	}
	t, err := b.typeCode[which].next(r)
	if err != nil {
		return err
	}
	n, err := readBlockLength(r, b.lenCode[which])
	if err != nil {
		return err
	}
	b.length[which] = n
	switch t {
	case 0:
		t = b.recent[which][0]
	case 1:
		t = b.recent[which][1] + 1
	default:
		t -= 2
	}
	if t >= b.types[which] {
		t -= b.types[which]
	}
	b.recent[which][0], b.recent[which][1] = b.recent[which][1], t
	b.current[which] = t
	return nil
}

// metaBlock reads one meta-block and reports whether it was the last.
func (d *decoder) metaBlock() (bool, error) {
	r := d.r
	last := r.take(1) == 1
	if last && r.take(1) == 1 {
		// An empty last meta-block, which is how a stream that has already
		// said everything ends.
		return true, nil
	}

	nibbles := int(r.take(2))
	length := 0
	if nibbles == 3 {
		// Metadata: bytes that are not part of the output at all. Nothing here
		// wants them, but they have to be stepped over exactly.
		if r.take(1) != 0 {
			return false, errReserved
		}
		n := int(r.take(2))
		if n == 0 {
			return last, nil
		}
		for i := 0; i < n; i++ {
			v := int(r.take(8))
			if i+1 == n && n > 1 && v == 0 {
				return false, errExuberant
			}
			length |= v << uint(8*i)
		}
		length++
		if !r.align() {
			return false, errPadding
		}
		if _, ok := r.bytes(length); !ok {
			return false, errTruncated
		}
		return last, nil
	}

	nibbles += 4
	for i := 0; i < nibbles; i++ {
		v := int(r.take(4))
		// The last nibble may not be zero, because a length written in more
		// nibbles than it needs has two encodings and only one is legal.
		if i+1 == nibbles && nibbles > 4 && v == 0 {
			return false, errExuberant
		}
		length |= v << uint(4*i)
	}
	stored := false
	if !last {
		stored = r.take(1) == 1
	}
	length++

	if stored {
		// Bytes as they are, for data that compressing would only make larger.
		if !r.align() {
			return false, errPadding
		}
		b, ok := r.bytes(length)
		if !ok {
			return false, errTruncated
		}
		at, ok := d.extend(length)
		if !ok {
			return false, errTooLarge
		}
		copy(d.out[at:], b)
		return last, nil
	}
	if err := d.compressed(length); err != nil {
		return false, err
	}
	return last, nil
}
