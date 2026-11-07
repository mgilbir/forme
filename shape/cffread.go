package shape

import "sync"

// A CFF face's subsets read the same CFF table every time: its INDEXes, its
// charset and what the seac closure inverts it into, its Private DICTs' local
// subroutines, and a CID-keyed font's FDSelect. None of it depends on which
// glyphs a subset keeps, and reading it was most of what a subset of a CFF
// face allocated: 877 KB a subset of twenty-two glyphs of Source Sans 3, for a
// program of 42 KB, and 2.2 MB of Noto Sans JP for one of 5 KB (#987).
// cffReader reads each once per face, and the face's clones share it, as they
// share glyphChecks.
//
// Only what is read is shared, never what is charged or decided: every budget
// charge, every refusal and every check a subset makes is made by each subset
// as it was, against what is read here. What the reader keeps is what a
// subset reads and does not write; nothing it returns may be written to.
//
// A read that fails is not kept, and is read again by the next subset to ask,
// with the same answer: the table does not change. And what it keeps is
// bounded: INDEX items, summed over every INDEX and Private DICT kept, past
// the table's length are read and not kept. A sound font's INDEXes come nowhere near
// that, since each item has an offset of at least a byte; a crafted one that
// names many overlapping INDEXes is read as it always was, a subset at a time.

// cffReader reads a CFF table once for every subset of the face that holds
// it. It is safe for concurrent use.
type cffReader struct {
	data []byte

	mu sync.Mutex
	// kept is how many INDEX items indexes holds, which stops at len(data).
	kept     int
	indexes  map[int]*cffIndex
	charsets map[[2]int][]int
	seac     map[int]*cffSeacRead
	privates map[[2]int]privateRegion
	// fdSelects is each glyph's Font DICT as fdSelectFDs reads it, by the
	// FDSelect's offset and the glyph count: a Font DICT is a byte, and -1
	// none, so two bytes a glyph rather than eight.
	fdSelects map[[2]int][]int16
}

// cffSeacRead is what cffSeacTables reads for the seac closure.
type cffSeacRead struct {
	charStrings, local, global [][]byte
	sidToGID                   map[int]int
}

func newCFFReader(data []byte) *cffReader {
	return &cffReader{data: data}
}

// cffReaderFor is the face's reader of the CFF table data, shared by its
// clones: the one it has where it reads these bytes, and otherwise a reader of
// data alone, which keeps what it reads for no other subset.
func (f *Face) cffReaderFor(data []byte) *cffReader {
	if c := f.checks; c != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.cff == nil && len(data) > 0 {
			c.cff = newCFFReader(data)
		}
		if r := c.cff; r != nil && sameBytes(r.data, data) {
			return r
		}
	}
	return newCFFReader(data)
}

// sameBytes is whether a and b are the same bytes in memory, not merely equal
// ones: the reader answers for the table it was made from.
func sameBytes(a, b []byte) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// index is readCFFIndex(r.data, off).
func (r *cffReader) index(off int) (*cffIndex, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if idx, ok := r.indexes[off]; ok {
		return idx, nil
	}
	idx, err := readCFFIndex(r.data, off)
	if err != nil {
		return nil, err
	}
	if r.kept+len(idx.items) <= len(r.data) {
		if r.indexes == nil {
			r.indexes = map[int]*cffIndex{}
		}
		r.indexes[off] = idx
		r.kept += len(idx.items)
	}
	return idx, nil
}

// charsetSIDs is cffCharsetSIDs(r.data, off, n).
func (r *cffReader) charsetSIDs(off, n int) ([]int, error) {
	key := [2]int{off, n}
	r.mu.Lock()
	if sids, ok := r.charsets[key]; ok {
		r.mu.Unlock()
		return sids, nil
	}
	r.mu.Unlock()
	sids, err := cffCharsetSIDs(r.data, off, n)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.charsets == nil {
		r.charsets = map[[2]int][]int{}
	}
	// A charset is one entry a glyph, and n is the face's glyph count: one per
	// key a subset asks with, of which there are two at most.
	if len(r.charsets) < 4 {
		r.charsets[key] = sids
	}
	return sids, nil
}

// seacTables is what the seac closure reads of the table (cffSeacTablesFrom),
// read once.
func (r *cffReader) seacTables(n int) (*cffSeacRead, error) {
	r.mu.Lock()
	if s, ok := r.seac[n]; ok {
		r.mu.Unlock()
		return s, nil
	}
	r.mu.Unlock()
	s, err := cffSeacTablesFrom(r, n)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seac == nil {
		r.seac = map[int]*cffSeacRead{}
	}
	if len(r.seac) < 4 {
		r.seac[n] = s
	}
	return s, nil
}

// privateParts is privateParts(region, size) of the Private DICT region at
// off, as cffPrivateRegion read it from r's table — which for one offset and
// size is always the same bytes: its DICT and its local subroutines, which a
// subset prunes into slices of its own.
func (r *cffReader) privateParts(off int, region []byte, size int) (privateRegion, error) {
	key := [2]int{off, size}
	r.mu.Lock()
	if p, ok := r.privates[key]; ok {
		r.mu.Unlock()
		return p, nil
	}
	r.mu.Unlock()
	ops, raw, subrs, err := privateParts(region, size)
	if err != nil {
		return privateRegion{}, err
	}
	p := privateRegion{ops, raw, subrs}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Its subroutines count against what the reader keeps as an INDEX's
	// items do, and the DICT as one more.
	if r.kept+len(subrs)+1 <= len(r.data) {
		if r.privates == nil {
			r.privates = map[[2]int]privateRegion{}
		}
		r.privates[key] = p
		r.kept += len(subrs) + 1
	}
	return p, nil
}

// fdSelect is each of n glyphs' Font DICT, from the FDSelect sliceFDSelect
// read at off: fdSelectFDs(b, n), read once.
func (r *cffReader) fdSelect(off int, b []byte, n int) []int16 {
	key := [2]int{off, n}
	r.mu.Lock()
	defer r.mu.Unlock()
	if fds, ok := r.fdSelects[key]; ok {
		return fds
	}
	wide := fdSelectFDs(b, n)
	fds := make([]int16, n)
	for g, fd := range wide {
		fds[g] = int16(fd) // a byte, or -1
	}
	if r.fdSelects == nil {
		r.fdSelects = map[[2]int][]int16{}
	}
	// One for each FDSelect a subset asks with, of which a font has one.
	if len(r.fdSelects) < 2 {
		r.fdSelects[key] = fds
	}
	return fds
}
