package paragraph

import (
	"strings"
	"sync"
	"unicode"
)

// Automatic hyphenation, CSS Text §6.1: where a word may be broken when the
// document has not said.
//
// # The algorithm
//
// Liang's, which is the one every typesetting system uses and the one the
// pattern files in hyphentable.go are written for. A word is wrapped in a
// boundary marker — "highway" becomes ".highway." — and every substring of it
// is looked up in the pattern table. A pattern carries a number at each of its
// own inter-letter positions, and the numbers of every pattern that matched are
// taken at their *maximum* at each position of the word. An odd number is a
// place the word may break and an even one is a place it may not.
//
// That is the whole rule, and the reason it is stated so plainly is that it is
// easy to write a version that is nearly it. The maximum and not the sum: two
// patterns disagreeing about a position are not evidence, and adding them turns
// two "no" votes into a "yes". Every substring and not only the ones anchored at
// a boundary: a pattern is a statement about the letters wherever they fall.
//
// # Why the exceptions are not a shortcut
//
// The \hyphenation{} list is what the patterns get wrong, and a word on it takes
// its breaks from the list and from nowhere else — including a word written with
// no hyphens at all, which is the list saying "do not break this". "present" and
// "project" are both there, because the patterns would break them where the noun
// and the verb differ.
//
// # What is not here
//
// One language. The table is American English and the caller asks for it by
// language tag; a document in anything else is not hyphenated and says so. That
// is the honest shape rather than a limitation to be worked around — hyphenating
// German with English patterns produces breaks that are not merely wrong but
// unreadable, which is worse than not breaking at all.

// maxHyphenWord is the longest word this will hyphenate.
//
// The work is linear in the word's length and the table is bounded, so this is
// not protecting the algorithm from a cost it cannot pay. It is protecting the
// *result*: a "word" of ten thousand letters is a URL or a hash or a base64
// blob, and offering a break every second character through the middle of one
// is not hyphenation. Browsers stop as well, and the line breaking has
// overflow-wrap for exactly that case.
const maxHyphenWord = 100

// HyphenPoints is where a word may be broken, as indexes into it in runes.
//
// An index i means a line may end after the word's first i runes, with a hyphen
// printed there. The result is in increasing order and never includes a point
// inside the first left or the last right runes: those are the hyphenmins, which
// keep a hyphen from leaving one letter stranded at the end of a line or
// carrying one alone to the next.
//
// Nothing is returned for a language this has no patterns for, for a word too
// short to have a point inside the mins, or for a word holding anything but
// letters — a hyphenation dictionary is a statement about the letters of a
// language, and "co-op", "R2D2" and "don't" are not words it has anything to say
// about.
func HyphenPoints(word string, lang Language, left, right int) []int {
	if !HyphenatesLanguage(lang) {
		return nil
	}
	runes := []rune(word)
	if len(runes) > maxHyphenWord {
		return nil
	}
	// Zero means "whatever the language says", which is the hyphenmins its own
	// pattern file states. A number is the number: hyphenate-limit-chars is the
	// author overriding the dictionary, and an author who asks to keep two
	// letters back where the language wants three has asked for two.
	if left <= 0 {
		left = enUSHyphenLeft
	}
	if right <= 0 {
		right = enUSHyphenRight
	}
	// A word with no room for a point inside the two mins has none, whatever
	// else is asked. That is also hyphenate-limit-chars' *first* value under
	// "auto": the shortest word this will divide is one that can hold both
	// halves, and the property's own minimum is applied by the caller on top.
	if len(runes) < left+right {
		return nil
	}
	lower := make([]rune, len(runes))
	for i, r := range runes {
		if !unicode.IsLetter(r) {
			return nil
		}
		lower[i] = unicode.ToLower(r)
	}

	t := hyphenTable()
	if points, ok := t.exceptions[string(lower)]; ok {
		return withinMins(points, len(runes), left, right)
	}
	return withinMins(t.points(lower), len(runes), left, right)
}

// withinMins drops the points the hyphenmins forbid.
func withinMins(points []int, n, left, right int) []int {
	out := points[:0:0]
	for _, p := range points {
		if p >= left && n-p >= right {
			out = append(out, p)
		}
	}
	return out
}

// HyphenatesLanguage reports whether the patterns here are the ones a language
// is hyphenated with.
//
// English only, and English is the primary subtag: LanguageOf has already cut
// "en-US" and "en-GB-oxendict" down to "en". A document that declares no
// language is not English — the tag is what the author wrote, and hyphenating
// undeclared text as English is guessing at the language of words one has not
// read.
//
// en-GB is hyphenated with the American patterns, which is wrong in the small:
// the two traditions differ over where a word divides. It is what a browser
// without the British patterns installed does, it is far closer than not
// breaking at all, and the alternative is a second table for a few hundred
// words.
func HyphenatesLanguage(lang Language) bool { return lang == "en" }

// hyphenPatterns is the table the algorithm reads, built once.
type hyphenPatterns struct {
	// byLetters maps a pattern's letters to the numbers it carries, one more
	// than there are letters: values[i] is the number before the pattern's i-th
	// letter, and the last is the number after its last.
	byLetters map[string][]int8
	// longest is the longest pattern, which bounds the substrings worth trying.
	longest int
	// exceptions maps a word to the points the \hyphenation{} list gives it.
	exceptions map[string][]int
}

var (
	hyphenOnce  sync.Once
	hyphenBuilt *hyphenPatterns
)

// hyphenTable builds the table on first use.
//
// Lazily, because a document that hyphenates nothing should not pay for five
// thousand patterns, and once, because a document that hyphenates one word
// hyphenates a hundred.
func hyphenTable() *hyphenPatterns {
	hyphenOnce.Do(func() {
		t := &hyphenPatterns{
			byLetters:  make(map[string][]int8, 5000),
			exceptions: make(map[string][]int, 16),
		}
		for _, p := range strings.Split(enUSPatterns, "\n") {
			if p = strings.TrimSpace(p); p == "" {
				continue
			}
			letters, values := splitPattern(p)
			t.byLetters[letters] = values
			if n := len([]rune(letters)); n > t.longest {
				t.longest = n
			}
		}
		for _, w := range strings.Split(enUSExceptions, "\n") {
			if w = strings.TrimSpace(w); w == "" {
				continue
			}
			word, points := splitException(w)
			t.exceptions[word] = points
		}
		hyphenBuilt = t
	})
	return hyphenBuilt
}

// splitPattern separates a pattern's letters from its numbers.
//
// ".ach4" is the letters ".ach" with a 4 after the "h"; "a1bc3d" is "abcd" with
// a 1 between "a" and "b" and a 3 between "c" and "d". The values slice is one
// longer than the letters so that a number at either end has somewhere to sit.
func splitPattern(p string) (string, []int8) {
	var letters strings.Builder
	values := make([]int8, 0, len(p)+1)
	values = append(values, 0)
	for _, r := range p {
		if r >= '0' && r <= '9' {
			values[len(values)-1] = int8(r - '0')
			continue
		}
		letters.WriteRune(r)
		values = append(values, 0)
	}
	return letters.String(), values
}

// splitException reads one entry of the \hyphenation{} list into the word and
// the points it allows.
func splitException(w string) (string, []int) {
	var word strings.Builder
	var points []int
	n := 0
	for _, r := range w {
		if r == '-' {
			points = append(points, n)
			continue
		}
		word.WriteRune(unicode.ToLower(r))
		n++
	}
	return word.String(), points
}

// points runs the algorithm over a lower-cased word.
func (t *hyphenPatterns) points(word []rune) []int {
	// The boundary marker is a character of the pattern language rather than of
	// the word: ".ach4" is "a word beginning with ach", and without the marker
	// it would match every "ach" anywhere.
	marked := make([]rune, 0, len(word)+2)
	marked = append(marked, '.')
	marked = append(marked, word...)
	marked = append(marked, '.')

	// values[i] is the number between marked[i-1] and marked[i].
	values := make([]int8, len(marked)+1)
	for i := range marked {
		// Only as far as the longest pattern: a substring longer than any
		// pattern cannot match one.
		hi := i + t.longest
		if hi > len(marked) {
			hi = len(marked)
		}
		for j := i + 1; j <= hi; j++ {
			got, ok := t.byLetters[string(marked[i:j])]
			if !ok {
				continue
			}
			for k, v := range got {
				// The maximum and not the sum. Two patterns that disagree about
				// a position are not evidence for breaking there.
				if v > values[i+k] {
					values[i+k] = v
				}
			}
		}
	}

	var out []int
	// The positions inside the word: values[0] and values[1] are before and
	// after the leading marker, and neither is a place in the word at all.
	for i := 1; i < len(word); i++ {
		if values[i+1]%2 == 1 {
			out = append(out, i)
		}
	}
	return out
}

// HyphenatePieces splits a text node's pieces at the points automatic
// hyphenation offers, marking each part but the last as ending at a hyphen.
//
// points are rune offsets into the node's whole text, which is the pieces'
// texts run together. They are offsets into the *node* and not into a word
// because a word is not a node: "high<span>way</span>" is one word written in
// two, and the caller is the pass that gathered it. See the layout side, which
// walks an inline formatting context to find the words before anything is asked
// of a dictionary.
//
// endsAtHyphen reports that the last point was at the very end of the text, so
// the opportunity belongs to whatever box comes next — the same thing a soft
// hyphen at the end of a node does, and for the same reason.
//
// A piece that already ends at a soft hyphen keeps that flag: the author's mark
// and the dictionary's points are both places the word may break, and §6.1 does
// not make the second replace the first.
func HyphenatePieces(pieces []Piece, points []int) ([]Piece, bool) {
	if len(points) == 0 {
		return pieces, false
	}
	total := 0
	for _, p := range pieces {
		total += len([]rune(p.Text))
	}

	out := make([]Piece, 0, len(pieces)+len(points))
	endsAtHyphen := false
	at := 0 // the offset the next piece starts at
	next := 0
	for _, p := range pieces {
		runes := []rune(p.Text)
		end := at + len(runes)
		// The points that fall inside this piece. One at its very start belongs
		// to the piece before it, which has already been emitted with its own
		// Hyphen flag set below.
		cut := 0
		for next < len(points) && points[next] <= at {
			next++
		}
		for next < len(points) && points[next] < end {
			part := p
			part.Text = string(runes[cut : points[next]-at])
			part.Hyphen = true
			part.BreakBefore = cut > 0 || p.BreakBefore
			out = append(out, part)
			cut = points[next] - at
			next++
		}
		last := p
		last.Text = string(runes[cut:])
		if cut > 0 {
			// It begins at a hyphen this made, which is a place a line may
			// begin — the flag a soft hyphen sets on the piece after it.
			last.BreakBefore = true
		}
		if next < len(points) && points[next] == end {
			// The point is at this piece's end. There is nothing to split, and
			// what it marks is that the piece ends at a hyphen; the piece after
			// it — in this node or in the next box — begins a line.
			last.Hyphen = true
			if end == total {
				endsAtHyphen = true
			}
			next++
		}
		out = append(out, last)
		at = end
	}
	return out, endsAtHyphen
}

// wordHyphenPoints is HyphenPoints over a word gathered from a document, which
// is letters and nothing else: the caller ends a word at the first character a
// dictionary has nothing to say about.
func wordHyphenPoints(word string, lang Language) []int {
	return HyphenPoints(word, lang, 0, 0)
}
