package brotli

import "errors"

// Prefix codes: how a Brotli stream describes the codes it then uses.
//
// The codes themselves are canonical and packed most-significant bit first, so
// a symbol is read by taking one bit at a time and asking, at each length,
// whether the code so far is one of that length's. That is slower than a lookup
// table indexed by the next fifteen bits, and it is what is here because a
// fifteen-bit table is 32768 entries and a meta-block may describe two hundred
// and fifty-six codes at once. The tables would be the larger cost by far.

const maxCodeLength = 15

// huffman is one canonical prefix code: how many codes there are of each
// length, and the symbols in the order the codes run.
type huffman struct {
	counts  [maxCodeLength + 1]uint16
	symbols []uint16

	// only is the symbol of a code that has just one, which reads no bits at
	// all. RFC 7932 §3.4 allows it and a stream of one repeated byte uses it.
	only  int
	alone bool
}

var (
	errIncomplete = errors.New("brotli: a prefix code leaves some bit patterns unassigned")
	errOverfull   = errors.New("brotli: a prefix code assigns more codes than its lengths allow")
	errNoCode     = errors.New("brotli: a bit pattern that is not one of the prefix code's")
)

// newHuffman builds a code from one length per symbol, where zero means the
// symbol is absent.
func newHuffman(lengths []byte) (*huffman, error) {
	h := &huffman{only: -1}
	n := 0
	last := 0
	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		if l > maxCodeLength {
			return nil, errOverfull
		}
		h.counts[l]++
		n++
		last = sym
	}
	switch n {
	case 0:
		return nil, errIncomplete
	case 1:
		// One symbol, and no bits are spent saying which.
		h.only, h.alone = last, true
		return h, nil
	}
	// Kraft's inequality, which for a prefix code that wastes nothing is an
	// equality. Both directions are errors: an over-full code has two symbols
	// on one pattern, an under-full one has patterns that decode to nothing.
	left := 1
	for l := 1; l <= maxCodeLength; l++ {
		left <<= 1
		left -= int(h.counts[l])
		if left < 0 {
			return nil, errOverfull
		}
	}
	if left != 0 {
		return nil, errIncomplete
	}

	var offsets [maxCodeLength + 2]int
	at := 0
	for l := 1; l <= maxCodeLength; l++ {
		offsets[l] = at
		at += int(h.counts[l])
	}
	h.symbols = make([]uint16, at)
	for sym, l := range lengths {
		if l > 0 {
			h.symbols[offsets[l]] = uint16(sym)
			offsets[l]++
		}
	}
	return h, nil
}

// next reads one symbol.
func (h *huffman) next(r *reader) (int, error) {
	if h.alone {
		return h.only, nil
	}
	code, first, index := 0, 0, 0
	for l := 1; l <= maxCodeLength; l++ {
		code |= int(r.bit())
		count := int(h.counts[l])
		if code-first < count {
			return int(h.symbols[index+code-first]), nil
		}
		index += count
		first = (first + count) << 1
		code <<= 1
	}
	return 0, errNoCode
}

// codeLengthCode is the fixed prefix code RFC 7932 §3.5 gives for the lengths
// of the code-length code — the code that describes the code that describes the
// code. Its lengths are stated by the specification and are not read from the
// stream.
var codeLengthCode = mustHuffman([]byte{2, 4, 3, 2, 2, 4})

// codeLengthOrder is the order §3.5 sends those eighteen lengths in: the ones
// most likely to be used first, so a code that uses few of them can stop early.
var codeLengthOrder = [18]byte{1, 2, 3, 4, 0, 5, 17, 6, 16, 7, 8, 9, 10, 11, 12, 13, 14, 15}

func mustHuffman(lengths []byte) *huffman {
	h, err := newHuffman(lengths)
	if err != nil {
		panic("brotli: " + err.Error())
	}
	return h
}

var (
	errBadSimple    = errors.New("brotli: a simple prefix code names a symbol twice or one outside its alphabet")
	errBadLengths   = errors.New("brotli: the code lengths of a prefix code do not describe a whole code")
	errRunPastEnd   = errors.New("brotli: a repeated code length runs past the end of the alphabet")
	errBadContexts  = errors.New("brotli: a context map is longer than the contexts it maps")
	errTooManyTrees = errors.New("brotli: a context map names a tree that was not read")
)

// readCode reads one prefix code descriptor, RFC 7932 §3.5.
//
// limit is the alphabet: no symbol may reach it. size is what the alphabet
// would be at its largest, and is what the width of a simple code's symbols is
// computed from — the two differ only for distances, where the alphabet the
// window allows is smaller than the one a symbol can name.
func readCode(r *reader, size, limit int) (*huffman, error) {
	// Two bits, and the value 1 means the code is given as a handful of
	// symbols rather than as a length per symbol. Every other value is a count
	// of leading lengths to take as zero without reading them.
	hskip := int(r.take(2))
	if hskip != 1 {
		return readComplexCode(r, hskip, limit)
	}
	return readSimpleCode(r, size, limit)
}

// readSimpleCode reads a code of one to four symbols, whose lengths follow from
// how many there are rather than being spelled out.
func readSimpleCode(r *reader, size, limit int) (*huffman, error) {
	n := int(r.take(2)) + 1
	width := uint(bitsFor(size - 1))
	if width == 0 {
		width = 1
	}
	syms := make([]int, n)
	for i := range syms {
		syms[i] = int(r.take(width))
		if syms[i] >= limit {
			return nil, errBadSimple
		}
		for j := 0; j < i; j++ {
			if syms[j] == syms[i] {
				return nil, errBadSimple
			}
		}
	}
	// One symbol is the degenerate code: it reads no bits, because there is
	// nothing to say. A meta-block holding a run of one byte uses it.
	if n == 1 {
		return &huffman{only: syms[0], alone: true}, nil
	}
	// The shapes the rest can have. Four symbols have two, and one bit says
	// which; the others have only one.
	//
	// The lengths go to the symbols in the order they were read, and nothing
	// is sorted here: a canonical code already orders the symbols that share a
	// length, and that ordering is the whole of what the specification says
	// about which symbol gets which code. Sorting across the lengths as well
	// would move the short code to the smallest symbol, and RFC 7932 gives it
	// to the first one written.
	var shape []byte
	switch n {
	case 2:
		shape = []byte{1, 1}
	case 3:
		shape = []byte{1, 2, 2}
	case 4:
		if r.take(1) == 0 {
			shape = []byte{2, 2, 2, 2}
		} else {
			shape = []byte{1, 2, 3, 3}
		}
	}
	lengths := make([]byte, limit)
	for i, sym := range syms {
		lengths[sym] = shape[i]
	}
	return newHuffman(lengths)
}

// bitsFor is how many bits it takes to write v, which is what a simple code's
// symbols are read as.
func bitsFor(v int) int {
	n := 0
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}

// readComplexCode reads a length for every symbol in the alphabet, RFC 7932
// §3.5's second form. It is two codes deep: eighteen lengths describe a code,
// and that code spells out the alphabet's lengths, with two symbols meaning
// "repeat" so that the long runs of zeroes a sparse alphabet produces cost
// almost nothing.
func readComplexCode(r *reader, hskip, limit int) (*huffman, error) {
	var clLengths [18]byte
	// The budget: a complete code over the eighteen spends exactly 32.
	space, used := 32, 0
	for i := hskip; i < 18; i++ {
		l, err := codeLengthCode.next(r)
		if err != nil {
			return nil, err
		}
		clLengths[codeLengthOrder[i]] = byte(l)
		if l == 0 {
			continue
		}
		used++
		space -= 32 >> uint(l)
		if space <= 0 {
			break
		}
	}
	if space != 0 && used != 1 {
		return nil, errBadLengths
	}
	cl, err := newHuffman(clLengths[:])
	if err != nil {
		return nil, err
	}

	lengths := make([]byte, limit)
	// The same budget one scale up: a complete code over the alphabet spends
	// exactly 32768. Reading stops the moment it is spent, and every symbol
	// after that has no code, which is what leaves the slice's zeroes in place.
	space = 1 << 15
	prev := byte(8) // the length a "repeat the last one" refers to before there is one
	repeatLen, repeat := byte(0), 0
	for sym := 0; sym < limit && space > 0; {
		l, err := cl.next(r)
		if err != nil {
			return nil, err
		}
		if l < 16 {
			lengths[sym] = byte(l)
			sym++
			if l != 0 {
				prev = byte(l)
				space -= 1 << 15 >> uint(l)
			}
			repeat = 0
			continue
		}
		// 16 repeats the last non-zero length, 17 repeats zero. The count is
		// cumulative: a second 16 in a row extends the run rather than starting
		// a new one, which is how a run longer than ten is written.
		extra := uint(l - 14)
		want := byte(0)
		if l == 16 {
			want = prev
		}
		if repeatLen != want {
			repeat, repeatLen = 0, want
		}
		was := repeat
		if repeat > 0 {
			repeat = (repeat - 2) << extra
		}
		repeat += int(r.take(extra)) + 3
		grew := repeat - was
		if sym+grew > limit {
			return nil, errRunPastEnd
		}
		for i := 0; i < grew; i++ {
			lengths[sym] = repeatLen
			sym++
		}
		if repeatLen != 0 {
			space -= grew << (15 - repeatLen)
		}
	}
	if space != 0 {
		return nil, errBadLengths
	}
	return newHuffman(lengths)
}

// readVarLenUint8 reads RFC 7932 §9.2's short encoding for a small count: zero
// in one bit, one in four, and anything up to 255 in eleven.
func readVarLenUint8(r *reader) int {
	if r.take(1) == 0 {
		return 0
	}
	n := r.take(3)
	if n == 0 {
		return 1
	}
	// The three bits are how many follow, and the value is those with the
	// leading one put back — so the encoding is the number written in binary
	// with its top bit implied.
	return 1<<n + int(r.take(uint(n)))
}

// readContextMap reads RFC 7932 §7.3's map from context to prefix code.
//
// The map is one byte per context and there are up to 16384 of them, so it is
// run-length encoded over its zeroes and then, usually, move-to-front coded —
// the map is mostly one tree with a few exceptions, and both of those exist to
// make that shape cheap.
func readContextMap(r *reader, size int) ([]byte, int, error) {
	trees := readVarLenUint8(r) + 1
	m := make([]byte, size)
	if trees == 1 {
		return m, trees, nil
	}
	runBits := 0
	if r.take(1) == 1 {
		runBits = int(r.take(4)) + 1
	}
	code, err := readCode(r, trees+runBits, trees+runBits)
	if err != nil {
		return nil, 0, err
	}
	for at := 0; at < size; {
		sym, err := code.next(r)
		if err != nil {
			return nil, 0, err
		}
		switch {
		case sym == 0:
			m[at] = 0
			at++
		case sym <= runBits:
			// A run of zeroes, whose length is a power of two plus that many
			// extra bits.
			run := (1 << uint(sym)) + int(r.take(uint(sym)))
			if at+run > size {
				return nil, 0, errBadContexts
			}
			at += run
		default:
			v := sym - runBits
			if v >= trees {
				return nil, 0, errTooManyTrees
			}
			m[at] = byte(v)
			at++
		}
	}
	if r.take(1) == 1 {
		inverseMoveToFront(m)
	}
	return m, trees, nil
}

// inverseMoveToFront undoes the coding that writes each tree as how recently it
// was last used. It is what makes a map that names one tree over and over cost
// a run of zeroes rather than a run of that tree's number.
func inverseMoveToFront(m []byte) {
	var order [256]byte
	for i := range order {
		order[i] = byte(i)
	}
	for i, b := range m {
		// As an int, because a map may name tree 255 and a byte's 255+1 is 0 —
		// which is a slice of negative length and a panic, on a document that
		// merely uses every prefix code a meta-block is allowed.
		at := int(b)
		v := order[at]
		m[i] = v
		copy(order[1:at+1], order[:at])
		order[0] = v
	}
}
