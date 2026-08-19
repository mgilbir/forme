package brotli

import (
	_ "embed"
	"errors"
)

// The static dictionary, RFC 7932 §8 and Appendix A.
//
// Every Brotli stream may refer to a fixed 122,784-byte block of text — words
// and fragments of English, HTML, CSS and JavaScript — as though it had already
// been decoded. It is why Brotli beats gzip on small files: a 200-byte HTML
// document has nothing of its own to refer back to, and the dictionary is what
// it refers to instead.
//
// A reference names a word and one of the 121 transforms in tables.go, which
// put text before or after it, cut letters off either end, or upper-case it. A
// dictionary of 13,504 words with 121 transforms reaches far more strings than
// the words alone, and that is the point of them.

//go:embed dictionary.bin
var dictionary []byte

// wordBits is how many words the dictionary holds of each length: entry n is
// the number of bits it takes to index a word of n bytes, and zero means there
// are none. Words run from four bytes to twenty-four.
var wordBits = [25]uint{
	0, 0, 0, 0, 10, 10, 11, 11,
	10, 10, 10, 10, 10, 9, 9, 8,
	7, 7, 8, 7, 7, 6, 6, 5, 5,
}

// wordOffsets is where each length's words begin. It follows from wordBits —
// the words of one length are laid end to end and the next length starts after
// them — and is computed rather than written down so the two cannot disagree.
var wordOffsets = func() (offsets [26]int) {
	at := 0
	for n, bits := range wordBits {
		offsets[n] = at
		if bits != 0 {
			at += n << bits
		}
	}
	offsets[25] = at
	return offsets
}()

// transform is what a reference does to a word: text before it, one of the
// operations below, and text after it.
type transform struct {
	prefix string
	kind   int
	suffix string
}

// The operations, numbered as RFC 7932 §8 numbers them. The two shifting ones
// belong to the shared-dictionary extension rather than to RFC 7932 and cannot
// appear in the table tables.go generates; they are named so that the switch
// below is visibly complete rather than silently short.
const (
	identity       = 0
	omitLast1      = 1
	omitLast9      = 9
	upperCaseFirst = 10
	upperCaseAll   = 11
	omitFirst1     = 12
	omitFirst9     = 20
	shiftFirst     = 21
	shiftAll       = 22
)

var (
	errNoSuchWord      = errors.New("brotli: a reference to a dictionary word that does not exist")
	errNoSuchTransform = errors.New("brotli: a reference to a transform that does not exist")
	errEmptyWord       = errors.New("brotli: a dictionary reference that transforms away to nothing")
)

// word returns one dictionary reference, transformed and ready to append.
//
// index is what the distance beyond the window encodes: the low bits name the
// word among those of that length, and the rest name the transform.
func word(dst []byte, length, index int) ([]byte, error) {
	if length < 4 || length >= len(wordBits) || wordBits[length] == 0 {
		return nil, errNoSuchWord
	}
	bits := wordBits[length]
	which := index & (1<<bits - 1)
	kind := index >> bits
	if kind >= len(transforms) {
		return nil, errNoSuchTransform
	}
	at := wordOffsets[length] + which*length
	return applyTransform(dst, dictionary[at:at+length], transforms[kind])
}

// applyTransform appends the transformed word to dst.
func applyTransform(dst, w []byte, t transform) ([]byte, error) {
	dst = append(dst, t.prefix...)
	body := len(dst)

	n := len(w)
	switch {
	case t.kind <= omitLast9:
		// Identity is nought of these, so this covers it too.
		n -= t.kind
	case t.kind >= omitFirst1 && t.kind <= omitFirst9:
		skip := t.kind - (omitFirst1 - 1)
		if skip > n {
			skip = n
		}
		w, n = w[skip:], n-skip
	}
	if n < 0 {
		n = 0
	}
	dst = append(dst, w[:n]...)

	switch t.kind {
	case upperCaseFirst:
		upperCase(dst[body:])
	case upperCaseAll:
		for at := body; at < len(dst); {
			at += upperCase(dst[at:])
		}
	case shiftFirst, shiftAll:
		// Only a shared-dictionary stream can name these, and this reads the
		// dictionary RFC 7932 defines. Refusing is right: guessing would put
		// wrong bytes in the output and nothing downstream would notice.
		return nil, errNoSuchTransform
	}

	dst = append(dst, t.suffix...)
	if n == 0 && t.prefix == "" && t.suffix == "" {
		return dst, errEmptyWord
	}
	return dst, nil
}

// upperCase upper-cases the character at the front of b and returns its length
// in bytes.
//
// It is RFC 7932 §8's own rule and not Unicode's, which matters: for a
// three-byte character it flips a bit that is not a case distinction at all.
// The dictionary was built with this rule, so a word transformed any other way
// would not be the word the encoder meant.
func upperCase(b []byte) int {
	if len(b) == 0 {
		return 1
	}
	if b[0] < 0xC0 {
		if b[0] >= 'a' && b[0] <= 'z' {
			b[0] ^= 32
		}
		return 1
	}
	if b[0] < 0xE0 {
		if len(b) > 1 {
			b[1] ^= 32
		}
		return 2
	}
	if len(b) > 2 {
		b[2] ^= 5
	}
	return 3
}
