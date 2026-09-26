package paragraph

import (
	"math"
	"unicode/utf8"

	"github.com/mgilbir/forme/internal/charprop"
	"github.com/mgilbir/forme/segment"
)

// What the breaker learns about a run it is cutting, once.
//
// A word too long for its line is cut, the rest begins the next line, and the
// rest is cut again. Every question a cut asks about the stretch it makes — how
// many characters come before it, where its clusters end, how many units a
// letter-spacing goes after, how many word separators it holds — has an answer
// that is a difference of two counts over the whole run. Answered by counting
// the stretch instead, the question is asked of everything still to come at
// every line: the rest of the word is read once per line, which is quadratic in
// exactly the input overflow-wrap is reached for.
//
// So each count is tabulated over the run the first time a cut asks for it, and
// every cut after that is a subtraction.
//
// # Which run, and how it is found again
//
// The run is named by its text, and found again by comparing texts rather than
// by hashing one. Every line of a word broken across lines names the same run
// through the same string — a stretch keeps the run it was cut from, and the
// line that resumes a word divides the original item again — so the comparison
// is of two equal pointers, which is one instruction however long the word is.
// Hashing it, which is what a map keyed on the text does, reads every byte of
// the word once per lookup, and that was the next quadratic behind the ones
// above.
//
// A run that is equal to the one held but spelled in another string — the same
// word written twice in one document — is found by reading it once, and the
// held one then takes that spelling, so that the comparisons after it are of
// one pointer again rather than a byte-by-byte read of the whole word for every
// line.
//
// One run is held at a time. Breaking is sequential: a paragraph is filled from
// its start, and a run is cut on consecutive lines until it is used up, so the
// run a cut asks about is the run the cut before it asked about. A search that
// jumped between two long runs line by line would build the tables of each again
// on every jump, which is linear work per jump and not a wrong answer. The one
// fact that was already memoised for every run — its cluster boundaries — still
// is, so a second visit to a run finds those without walking it again.
type runIndex struct {
	text string

	// bounds is segment.Boundaries of the run, found once.
	bounds     []int
	haveBounds bool

	// runes[i] is the number of characters in text[:i], at every byte offset.
	runes []int32

	// units is what letter-spacing and an upright setting count. See unitIndex.
	units *unitIndex

	// words[i] is the number of word separators in text[:i], at every byte
	// offset.
	words []int32
}

// runOf is the index of the run with this text, the held one where it is that
// run.
func (br *Breaker) runOf(text string) *runIndex {
	if r := br.run; r != nil && r.text == text {
		r.text = text
		return r
	}
	r := &runIndex{text: text}
	if all, ok := br.bounds[text]; ok {
		r.bounds, r.haveBounds = all, true
	}
	br.run = r
	return r
}

// clusterBounds is where the run may be cut: segment.Boundaries of it, found
// once for the run and kept for as long as the breaker is.
func (br *Breaker) clusterBounds(r *runIndex) []int {
	if !r.haveBounds {
		r.bounds, r.haveBounds = segment.Boundaries(nil, r.text), true
		if br.bounds == nil {
			br.bounds = map[string][]int{}
		}
		br.bounds[r.text] = r.bounds
	}
	return r.bounds
}

// tabulable says a run is short enough for its counts to fit the tables below,
// whose entries are 32 bits. A longer one is counted, as it always was: a string
// of two gigabytes is not a document this engine will be asked to set, and the
// answer for one is still right.
func tabulable(text string) bool { return len(text) < math.MaxInt32 }

// runesTo is the number of characters in the run before byte at.
//
// at has to be where a character begins, or either end of the run, which is
// what cutWithinText makes of every cut. Everywhere else it is counted. A byte
// that begins a character is never swallowed by the decoding of the one before
// it — a sequence consumes continuation bytes and an invalid byte is a character
// of its own — so at such a byte the count over the whole run and the count over
// the prefix agree.
func (r *runIndex) runesTo(at int) int {
	if at <= 0 {
		return 0
	}
	if at > len(r.text) || (at < len(r.text) && !utf8.RuneStart(r.text[at])) || !tabulable(r.text) {
		return utf8.RuneCountInString(r.text[:min(at, len(r.text))])
	}
	if r.runes == nil {
		// Every offset up to and including a character's first byte carries the
		// number of characters that begin before it.
		r.runes = make([]int32, len(r.text)+1)
		n, k := int32(0), 0
		for i := range r.text {
			for ; k <= i; k++ {
				r.runes[k] = n
			}
			n++
		}
		for ; k <= len(r.text); k++ {
			r.runes[k] = n
		}
	}
	return int(r.runes[at])
}

// wordSeparators is countWordSeparators of text[from:to], from a table.
//
// It answers only where both ends begin a character, for the reason runesTo
// gives; anywhere else it counts.
func (r *runIndex) wordSeparators(from, to int) int {
	if from < 0 || to > len(r.text) || from > to || !runeEdge(r.text, from) ||
		!runeEdge(r.text, to) || !tabulable(r.text) {
		from, to = max(from, 0), min(to, len(r.text))
		if from >= to {
			return 0
		}
		return countWordSeparators(r.text[from:to])
	}
	if r.words == nil {
		r.words = make([]int32, len(r.text)+1)
		n := int32(0)
		for i := 0; i < len(r.text); {
			c, size := rune(r.text[i]), 1
			if c >= utf8.RuneSelf {
				c, size = utf8.DecodeRuneInString(r.text[i:])
			}
			if isWordSeparator(c) {
				n++
			}
			for k := i + 1; k <= i+size; k++ {
				r.words[k] = n
			}
			i += size
		}
	}
	return int(r.words[to] - r.words[from])
}

// runeEdge reports whether a byte offset is where a character begins, or the end.
func runeEdge(s string, i int) bool {
	return i == 0 || i == len(s) || (i > 0 && i < len(s) && utf8.RuneStart(s[i]))
}

// unitIndex answers two counts over any stretch of one run that begins and ends
// on a cluster boundary: SpacedUnits, which is what letter-spacing goes after,
// and UprightUnits, which is what a run set upright advances by.
//
// UprightUnits counts the clusters with a character of their own in them — a
// base, as opposed to a mark or something that draws nothing — and nothing
// carries from one cluster to the next, so a stretch's count is the difference
// of two.
//
// SpacedUnits is not that simple, and the difference is the whole of why this is
// a type. scanCursiveTracking carries its answer from one cluster to the next: a
// cluster of nothing but marks takes the cursive answer of the base before it.
// Counted afresh over a stretch, the scan starts with no base in front of it —
// so the marks at the head of a stretch, before its first base, are counted
// where in the whole run they may not be. Everything from that first base on is
// the same in both scans, because the base sets the answer the marks after it
// take.
//
// So the fresh count over a stretch is the whole run's count over it, plus the
// clusters of marks at its head wherever the run carried a cursive answer into
// them. The one earlier table this replaces declined every run with a mark or a
// cursive letter in it, for exactly this reason, and those runs were counted at
// every cut — an Arabic word with a letter-spacing, or Latin written with
// combining accents, was quadratic to break where plain Latin was not.
type unitIndex struct {
	// edge holds a bit for every byte offset that is a cluster boundary,
	// counting both ends of the run. A stretch that does not begin and end on
	// one is not a stretch these counts describe.
	edge []uint64
	// spaced[i] is the number of units the whole run's scan counts in the
	// clusters that end at or before byte i; based[i] is the number of
	// clusters with a base among them.
	spaced, based []int32
	// Where the run has a cluster of marks with no base in it, the three
	// things the correction needs, all read at cluster boundaries: markOnly[i]
	// is how many such clusters end at or before i, lead[i] how many of them
	// there are from i up to the next cluster with a base, and cursive holds a
	// bit for every boundary the scan reaches carrying a cursive answer. All
	// three are nil for a run with none, which is nearly every run.
	markOnly, lead []int32
	cursive        []uint64
}

// unitsOf is the run's unitIndex, built the first time it is asked for, or nil
// where the run is too long to tabulate.
func (br *Breaker) unitsOf(r *runIndex) *unitIndex {
	if r.units == nil && tabulable(r.text) {
		r.units = newUnitIndex(r.text, br.clusterBounds(r))
	}
	return r.units
}

// newUnitIndex builds the table from the run's clusters.
//
// The walk is scanCursiveTracking's own, cluster by cluster and rune by rune, so
// that the table and the count cannot disagree about which cluster is passed
// over, which one is a unit, or what a cluster of marks inherits.
func newUnitIndex(text string, bounds []int) *unitIndex {
	n := len(text)
	idx := &unitIndex{
		edge:   make([]uint64, n/64+1),
		spaced: make([]int32, n+1),
		based:  make([]int32, n+1),
	}
	setBit(idx.edge, 0)
	setBit(idx.edge, n)

	// The kind of each cluster, for the backward pass that finds lead.
	const (
		empty = iota
		markOnly
		hasBase
	)
	var kinds []uint8
	var starts []int
	cursive := false
	spaced, based, marks := int32(0), int32(0), int32(0)
	for k, start := 0, 0; start < n; k++ {
		end := n
		if k < len(bounds) {
			end = bounds[k]
		}
		if cursive {
			if idx.cursive == nil {
				idx.cursive = make([]uint64, n/64+1)
			}
			setBit(idx.cursive, start)
		}
		kind := empty
		for _, r := range text[start:end] {
			switch {
			case IsDefaultIgnorable(r):
				continue
			case charprop.Is(r, charprop.Mn|charprop.Me):
				if kind == empty {
					kind = markOnly
				}
				continue
			}
			kind = hasBase
			cursive = IsCursiveScript(r)
		}
		switch kind {
		case markOnly:
			// Counted where the scan carries no cursive answer into it, which
			// is the whole run's reading; a stretch that begins with it reads
			// it afresh, which is what the correction is for.
			if !cursive {
				spaced++
			}
			marks++
		case hasBase:
			if !cursive {
				spaced++
			}
			based++
		}
		kinds = append(kinds, uint8(kind))
		starts = append(starts, start)
		for i := start + 1; i <= end; i++ {
			idx.spaced[i], idx.based[i] = spaced, based
		}
		setBit(idx.edge, end)
		start = end
	}
	if marks == 0 {
		// No cluster the correction is about, so nothing it needs.
		idx.cursive = nil
		return idx
	}
	idx.markOnly = make([]int32, n+1)
	idx.lead = make([]int32, n+1)
	m := int32(0)
	for c, start := range starts {
		end := n
		if c+1 < len(starts) {
			end = starts[c+1]
		}
		if kinds[c] == markOnly {
			m++
		}
		for i := start + 1; i <= end; i++ {
			idx.markOnly[i] = m
		}
	}
	lead := int32(0)
	for c := len(starts) - 1; c >= 0; c-- {
		switch kinds[c] {
		case hasBase:
			lead = 0
		case markOnly:
			lead++
		}
		idx.lead[starts[c]] = lead
	}
	return idx
}

// covers reports whether the stretch text[from:to] is one these counts describe.
func (idx *unitIndex) covers(from, to int) bool {
	return from >= 0 && from <= to && to < len(idx.spaced) &&
		hasBit(idx.edge, from) && hasBit(idx.edge, to)
}

// spacedUnits is SpacedUnits(text[from:to]), for a stretch covers accepts.
func (idx *unitIndex) spacedUnits(from, to int) int {
	n := idx.spaced[to] - idx.spaced[from]
	if idx.lead != nil && hasBit(idx.cursive, from) {
		// The clusters of marks at the head of the stretch, which the whole
		// run's scan read as cursive and a fresh scan reads as units. They run
		// from the stretch's start to its first base, or to its end if that
		// comes first.
		n += min(idx.lead[from], idx.markOnly[to]-idx.markOnly[from])
	}
	return int(n)
}

// uprightUnits is UprightUnits(text[from:to]), for a stretch covers accepts.
func (idx *unitIndex) uprightUnits(from, to int) int {
	return int(idx.based[to] - idx.based[from])
}

func setBit(bits []uint64, i int) { bits[i/64] |= 1 << (uint(i) % 64) }

func hasBit(bits []uint64, i int) bool {
	return i >= 0 && i/64 < len(bits) && bits[i/64]&(1<<(uint(i)%64)) != 0
}
