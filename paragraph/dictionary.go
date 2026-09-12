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

// The four word lists, and the block each is for.
//
// One entry per script rather than a range table, because a dictionary is a
// language and the blocks are how a language is found: Thai in U+0E00, Lao in
// U+0E80 — the two share a block boundary and nothing else — Khmer in U+1780,
// Burmese in the Myanmar block. Class SA has more scripts in it than these four
// and they have no list here; UnsupportedScript is what says so.
var dictionaries = [...]struct {
	lo, hi  rune
	words   string
	longest int
	once    *sync.Once
	built   **dictionary
}{
	{0x0E00, 0x0E7F, thaiWords, thaiLongestWord, new(sync.Once), &thaiBuilt},
	{0x0E80, 0x0EFF, laoWords, laoLongestWord, new(sync.Once), &laoBuilt},
	{0x1780, 0x17FF, khmerWords, khmerLongestWord, new(sync.Once), &khmerBuilt},
	{0x1000, 0x109F, burmeseWords, burmeseLongestWord, new(sync.Once), &burmeseBuilt},
}

var (
	thaiBuilt    *dictionary
	laoBuilt     *dictionary
	khmerBuilt   *dictionary
	burmeseBuilt *dictionary
)

// wordDictionaryLanguages is the four above, named as a document names them.
//
// The table beside them is keyed by *script*, because a character is what a
// dictionary is looked up for and the four scripts are used by one language
// each. This one is keyed by language, because it answers a different question:
// CSS Text 4 §2.2 gives a virtual word separator only where the *content
// language* is one whose words the user agent can find, and a document that
// declares none is to get none however its characters read. Untagged Thai is
// still Thai to the eye and is not Thai to this rule.
var wordDictionaryLanguages = map[Language]bool{
	"th": true, // Thai
	"lo": true, // Lao
	"km": true, // Khmer
	"my": true, // Burmese
}

// HasWordDictionary reports whether this engine can find the words of a
// language that is written without spaces between them.
//
// It is the sibling of HasPhraseModel and of HyphenatesLanguage, and the three
// are asked the same way for the same reason: what a language needs is a table,
// and which tables are here is a fact about the build rather than about the
// document.
func HasWordDictionary(lang Language) bool { return wordDictionaryLanguages[lang] }

// dictionaryFor is the word list for the script a character belongs to, or nil
// where this engine has none.
//
// Built on first use, and once. A document with no Thai in it should not pay
// for twenty-six thousand words, and a document with one Thai paragraph pays
// for them once rather than once per run — which is the same trade the
// hyphenation patterns make. It matters more here than there: the four lists
// together are a hundred and seventy thousand words, and almost every document
// needs none of them.
func dictionaryFor(r rune) *dictionary {
	for i := range dictionaries {
		d := &dictionaries[i]
		if r < d.lo || r > d.hi {
			continue
		}
		d.once.Do(func() { *d.built = buildDictionary(d.words, d.longest) })
		return *d.built
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

// trailingDictionaryRun is the tail of text that a dictionary would segment
// together with whatever follows it: the longest suffix whose characters all
// belong to one dictionary's script.
//
// It is what a box hands the box after it, and the bound on how much travels.
// DictionaryBreaks segments a maximal stretch of one script and stops at the
// first character of another, so nothing beyond such a stop can change how the
// text after it divides — a space between two Thai phrases ends the run as
// surely as a Latin letter does. Without the bound a paragraph of Thai written
// in a hundred spans would carry its whole text through each of them and be
// segmented a hundred times.
//
// The empty string is the answer for the overwhelming majority of documents,
// which have no such script in them at all.
func trailingDictionaryRun(text string) string {
	var d *dictionary
	at := len(text)
	for at > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:at])
		this := dictionaryFor(r)
		if this == nil || (d != nil && this != d) {
			break
		}
		d, at = this, at-size
	}
	return text[at:]
}

// dictionaryTail is the part of text a following box has to be segmented with:
// the stretch since the last word boundary, where text ends in a script whose
// words a dictionary finds.
//
// breaks is the segmentation of this same text. Both bounds matter and neither
// is enough alone: the run bound drops text on the far side of a space or of
// another script, which DictionaryBreaks would not have segmented with this
// anyway, and the boundary bound drops the words already found inside the run.
// What is left is the part-word in progress, which is what the next box's first
// character continues.
func dictionaryTail(text string, breaks map[int]bool) string {
	run := trailingDictionaryRun(text)
	if run == "" {
		// The text ends in a character no dictionary knows, so nothing after it
		// is segmented with anything before it.
		return ""
	}
	at := len(text) - len(run)
	for off := range breaks {
		// Strictly before the end, which is the whole of what this rule is.
		//
		// A break *at* the end is the boundary itself being a word boundary, and
		// the next box is the one that has to find it — DictionaryBreaks leaves
		// out the first offset of what it segments, so a box handed nothing
		// cannot see a break at its own first character. Trimming there left
		// "<span>ภาษา</span><span>ไทย</span>" with no division at all.
		if off > at && off < len(text) {
			at = off
		}
	}
	return text[at:]
}

// DictionaryLookahead is how many bytes of the text *after* a stretch the
// segmentation of that stretch needs, where the stretch ends with r.
//
// Zero for every character no dictionary knows, which is almost every character
// in almost every document.
//
// Otherwise the longest word in the language, in bytes. That is exactly enough
// and not a margin for error: segmentWords is greedy and runs left to right, so
// the only question the text beyond a stretch can answer is what longestAt finds
// at a position inside it, and longestAt stops after the longest word. A probe
// that can see that far sees everything that could change its answer.
func DictionaryLookahead(r rune) int {
	d := dictionaryFor(r)
	if d == nil {
		return 0
	}
	// In characters, so in bytes it is that times the longest a character can
	// be. Reading a few bytes more than the longest word is free; reading fewer
	// is a word the probe cannot find.
	return d.longest * utf8.UTFMax
}
