package paragraph

import (
	"strings"
	"sync"
	"unicode/utf8"
)

// CSS Text §5.1's lexical line breaking: where a line may end in a script that
// writes no spaces between its words.
//
// UAX #14 gives those scripts class SA — "Complex Context Dependent" — and then
// declines to say where a line may end in them, because there is no rule to
// give. Where one word ends and the next begins in "กรุงเทพคือสวยงาม" is a fact
// about Thai vocabulary and not about Thai characters, so the only way to know
// it is to have the vocabulary. See cmd/gendict for where the vocabulary comes
// from and what travels with it.
//
// # What this does with it
//
// Longest match, left to right: at each position it takes the longest word the
// dictionary has there. That is the classic maximal-matching segmenter, and it
// is what gets "กรุงเทพคือสวยงาม" right — "กรุง" is a word too, and taking it
// would leave "เทพ" standing where "กรุงเทพ" was written.
//
// One word of lookahead was written here as well — choose the word that leaves
// the best word after it, rather than the longest — and it is not here now.
// Nothing distinguishes the two: no fixture, and the suite reads 5845 either
// way. It is a real refinement of the algorithm and this engine has no document
// that can see it, so it went the way of every other rule here that could not
// be shown working.
//
// # Where it does not reach
//
// A stretch no word matches falls back to the boundary between typographic
// character units, which is what §5.1 requires of a UA that cannot do better:
// "some form of fallback line breaking must occur even if the UA doesn't know
// how to perform it correctly. Overflowing is not allowed." It is a place the
// words are not, and it is somewhere — see UnsupportedScript, which is the
// report a script with no vocabulary at all still gets.
//
// The text is one box's, so a word split across an inline boundary is two runs
// and is segmented as two. That is the same limit every other rule in this
// package has at a box edge.

// dictionary is a word list built for lookup.
type dictionary struct {
	// nodes maps a prefix to what it is: a word, the start of a longer word, or
	// both. It is one map rather than a tree because the probe below asks about
	// each prefix in turn, and a map answers that in one step where a tree
	// answers it in as many steps as the prefix is long.
	nodes map[string]uint8
	// longest is the longest word, in characters, which is where a probe stops.
	longest int
}

const (
	// isWord says the string is in the dictionary.
	isWord uint8 = 1 << iota
	// isPrefix says some longer word begins with it, so a probe that has got
	// this far has somewhere to go.
	isPrefix
)

// longestAt is the length in bytes of the longest dictionary word at the start
// of s, or zero.
func (d *dictionary) longestAt(s string) int {
	best := 0
	for i, n := 0, 0; i < len(s) && n < d.longest; n++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		got := d.nodes[s[:i]]
		if got&isWord != 0 {
			best = i
		}
		if got&isPrefix == 0 {
			break
		}
	}
	return best
}

// buildDictionary reads a generated word list.
//
// Every proper prefix of every word is entered as well, which is what lets a
// probe stop as soon as the characters it has read begin nothing. Without it
// the probe would read to the longest word in the language at every position,
// which for Thai is twenty characters of map lookups per character of text.
func buildDictionary(words string, longest int) *dictionary {
	d := &dictionary{nodes: make(map[string]uint8, 1<<18), longest: longest}
	for _, w := range strings.Split(words, "\n") {
		if w == "" {
			continue
		}
		d.nodes[w] |= isWord
		for i := range w {
			if i == 0 {
				continue
			}
			d.nodes[w[:i]] |= isPrefix
		}
	}
	return d
}

var (
	thaiOnce  sync.Once
	thaiBuilt *dictionary
)

// dictionaryFor is the word list for the script a character belongs to, or nil
// where this engine has none.
//
// Built on first use, and once. A document with no Thai in it should not pay
// for twenty-six thousand words, and a document with one Thai paragraph pays
// for them once rather than once per run — which is the same trade the
// hyphenation patterns make.
func dictionaryFor(r rune) *dictionary {
	if r >= 0x0E00 && r <= 0x0E7F {
		thaiOnce.Do(func() { thaiBuilt = buildDictionary(thaiWords, thaiLongestWord) })
		return thaiBuilt
	}
	return nil
}

// HasDictionary reports whether this engine can segment the script a character
// belongs to.
//
// It is the question UnsupportedScript asks before it says a line was broken in
// the wrong place: a script with a word list is broken where its words are, and
// there is nothing to report.
func HasDictionary(r rune) bool { return dictionaryFor(r) != nil }

// DictionaryBreaks reports the byte offsets in a run at which a line may end,
// for the scripts that need a word list to say.
//
// The offsets are where a word *begins*, the first excepted: a break before the
// first character of a run is not one the run offers, and the caller has its own
// answer about the boundary it sits at.
//
// A stretch the dictionary does not recognise contributes no boundary inside
// itself, and the search resumes at the first character that begins a word
// again. Guessing inside it would put a break in the middle of a word this
// engine simply does not have, which is worse than the fallback §5.1 allows.
func DictionaryBreaks(text string) map[int]bool {
	var out map[int]bool
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		d := dictionaryFor(r)
		if d == nil {
			i += size
			continue
		}
		// A stretch of one script, segmented as one. Stopping at the first
		// character of another would cut a word at a punctuation mark the
		// dictionary knows about.
		end := i
		for end < len(text) {
			r, size := utf8.DecodeRuneInString(text[end:])
			if dictionaryFor(r) != d {
				break
			}
			end += size
		}
		for _, at := range segmentWords(d, text[i:end]) {
			if out == nil {
				out = map[int]bool{}
			}
			out[i+at] = true
		}
		i = end
	}
	return out
}

// segmentWords divides one stretch of a single script into words, and returns the
// offsets each word after the first begins at.
func segmentWords(d *dictionary, s string) []int {
	var out []int
	for i := 0; i < len(s); {
		best := d.longestAt(s[i:])
		if best == 0 {
			// Nothing here begins a word, and §5.1 does not let that be the end
			// of it: "some form of fallback line breaking must occur even if
			// the UA doesn't know how to perform it correctly. Overflowing is
			// not allowed." So a stretch the vocabulary does not have is broken
			// the way a script with no vocabulary at all is — between
			// typographic character units, which is a place the words are not
			// and is somewhere.
			//
			// One character at a time rather than skipping to the next word,
			// because the search resumes after each: a word that begins in the
			// middle of the unrecognised stretch is found there.
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
			if i < len(s) {
				out = append(out, i)
			}
			continue
		}
		i += best
		if i < len(s) {
			out = append(out, i)
		}
	}
	return out
}
