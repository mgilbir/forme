package fonttest

import "encoding/binary"

// The legacy 'kern' table, which is how a font written before GPOS states its
// kerning. A modern face states it in GPOS and a shaper reads the 'kern' table
// only when there is nothing there, so a fixture for it has to be a font with
// no GPOS at all — which is what makes a synthetic one the only way to reach it.

// KernSubtable is one subtable of the legacy table.
//
// Coverage is the field as the format defines it: bit 0 set means horizontal,
// bit 1 means the values are minimums rather than adjustments, bit 2 means
// cross-stream, bit 3 means override, and the high byte is the subtable format.
// It is given raw rather than as flags so that a test can write a combination
// the builder would not otherwise produce — the point of most of them is to be
// rejected.
//
// Pairs is always written as a format 0 body whatever Coverage claims the format
// is, so that "a reader must not read this as format 0" can be stated.
type KernSubtable struct {
	Coverage int
	Pairs    []KernPair
}

// KernHorizontal is the coverage of the one kind of subtable this engine
// applies: horizontal, adjustments rather than minimums, not cross-stream, not
// an override, format 0.
const KernHorizontal = 0x0001

// LegacyKern builds a version 0 'kern' table — the Microsoft one, with 16-bit
// counts and lengths, not Apple's version 1.
func LegacyKern(subs []KernSubtable) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint16(out[0:], 0) // version
	binary.BigEndian.PutUint16(out[2:], uint16(len(subs)))
	for _, s := range subs {
		body := kernFormat0Body(s.Pairs)
		head := make([]byte, 6)
		binary.BigEndian.PutUint16(head[0:], 0) // subtable version
		// length covers the header as well as the body, which is what a reader
		// steps by to reach the next subtable.
		binary.BigEndian.PutUint16(head[2:], uint16(6+len(body)))
		binary.BigEndian.PutUint16(head[4:], uint16(s.Coverage))
		out = append(out, head...)
		out = append(out, body...)
	}
	return out
}

// kernFormat0Body is nPairs, the three binary-search fields, and the records.
//
// The search fields are what a font would carry and no reader here consults;
// they are written correctly anyway, because a fixture that is not a real table
// is a fixture that proves the reader tolerant rather than right.
func kernFormat0Body(pairs []KernPair) []byte {
	sorted := append([]KernPair(nil), pairs...)
	sortKernPairs(sorted)

	// searchRange is the largest power of two no greater than nPairs, times the
	// six bytes of a record; entrySelector is that power's exponent.
	entrySelector, pow := 0, 1
	for pow*2 <= len(sorted) {
		pow *= 2
		entrySelector++
	}
	searchRange := 6 * pow
	if len(sorted) == 0 {
		searchRange = 0
	}

	body := make([]byte, 8+6*len(sorted))
	binary.BigEndian.PutUint16(body[0:], uint16(len(sorted)))
	binary.BigEndian.PutUint16(body[2:], uint16(searchRange))
	binary.BigEndian.PutUint16(body[4:], uint16(entrySelector))
	binary.BigEndian.PutUint16(body[6:], uint16(6*len(sorted)-searchRange))
	for i, p := range sorted {
		rec := 8 + 6*i
		binary.BigEndian.PutUint16(body[rec+0:], uint16(p.Left))
		binary.BigEndian.PutUint16(body[rec+2:], uint16(p.Right))
		binary.BigEndian.PutUint16(body[rec+4:], uint16(int16(p.Adjust)))
	}
	return body
}

// sortKernPairs orders by left glyph then right, which is the order the format
// requires so that the binary search above works.
func sortKernPairs(a []KernPair) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && kernPairLess(a[j], a[j-1]); j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func kernPairLess(x, y KernPair) bool {
	if x.Left != y.Left {
		return x.Left < y.Left
	}
	return x.Right < y.Right
}
