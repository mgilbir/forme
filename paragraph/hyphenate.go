package paragraph

import (
	"strings"
	"sync"

	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/internal/charprop"
	"github.com/mgilbir/forme/shape"
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
// # Which languages
//
// The ones there is a table for, and no others: a document in a language this
// has no patterns for is not hyphenated and says so. That is the honest shape
// rather than a limitation to be worked around — hyphenating German with English
// patterns produces breaks that are not merely wrong but unreadable, which is
// worse than not breaking at all.
//
// Five tables, one generated file each, listed in hyphenSources. Adding a sixth
// is a line there, an entry in cmd/internal/tables and a file in the Makefile's
// HYPHEN_FILES.

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
	src := hyphenationFor(lang)
	if src == nil {
		return nil
	}
	runes := []rune(word)
	if len(runes) > maxHyphenWord {
		return nil
	}
	// The patterns are written in one spelling of the language's letters and
	// text arrives in either: "café" is four characters or five, Unicode says
	// they are the same word, and a table keyed on the four has nothing to say
	// about the five. So the word is composed before it is looked up, and the
	// points that come back are put into the caller's own positions again — a
	// point after the "e" of a decomposed "é" is a point after the mark that
	// follows it, because the two are one letter and a line cannot be broken
	// between them.
	letters, at := shape.ComposeCanonically(runes)
	// Zero means "whatever the language says", which is the hyphenmins its own
	// pattern file states. A number is the number: hyphenate-limit-chars is the
	// author overriding the dictionary, and an author who asks to keep two
	// letters back where the language wants three has asked for two.
	if left <= 0 {
		left = src.left
	}
	if right <= 0 {
		right = src.right
	}
	// A word with no room for a point inside the two mins has none, whatever
	// else is asked. That is also hyphenate-limit-chars' *first* value under
	// "auto": the shortest word this will divide is one that can hold both
	// halves, and the property's own minimum is applied by the caller on top.
	if len(letters) < left+right {
		return nil
	}
	lower := make([]rune, len(letters))
	for i, r := range letters {
		// A mark the composition could not take up is not a letter and stops
		// this as any other non-letter does: the word is spelled with something
		// the dictionary was not written over, and a dictionary that has nothing
		// to say says nothing rather than guessing.
		if !charprop.Is(r, charprop.L) {
			return nil
		}
		lower[i] = simpleLower(r)
	}

	t := src.table()
	points, ok := t.exceptions[string(lower)]
	if !ok {
		points = t.points(lower)
	}
	return atSourceRunes(withinMins(points, len(letters), left, right), at)
}

// atSourceRunes turns points counted in composed letters into points counted in
// the runes the caller passed.
//
// A point after the i-th letter is a point after every rune that went into those
// letters, which is the highest source index among them: the caller's own
// offsets are what it will draw a hyphen at, and a hyphen between a letter and
// its own accent is not a place the word divides. Two letters that came from one
// rune — a decomposition Unicode does not allow to be put back together — would
// otherwise land on the same point twice, so a repeat is dropped.
func atSourceRunes(points []int, at []int) []int {
	if len(points) == 0 {
		return points
	}
	out := points[:0:0]
	high, k := -1, 0
	for _, p := range points {
		for ; k < p && k < len(at); k++ {
			if at[k] > high {
				high = at[k]
			}
		}
		if n := high + 1; len(out) == 0 || out[len(out)-1] != n {
			out = append(out, n)
		}
	}
	return out
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

// HyphenationOf reads a lang attribute as the key of the table its words are
// divided with.
//
// The tag whole, as OrthographyOf reads it and for the same reason: what decides
// is sometimes the script. "zh" is Chinese, which has no hyphenation to speak
// of, and "zh-Latn" is Chinese romanised — a text of Latin syllables that
// divides between them. Everything else is keyed on the primary subtag, which is
// what "en-US" and "en-GB-oxendict" have in common, unless a variant subtag
// names a spelling of the language its table is not for; see
// orthographicVariants.
//
// A document that declares no language gets nothing. The tag is what the author
// wrote, and hyphenating undeclared text as English is guessing at the language
// of words one has not read.
//
// en-GB is hyphenated with the American patterns, which is wrong in the small:
// the two traditions differ over where a word divides. It is what a browser
// without the British patterns installed does, it is far closer than not
// breaking at all, and the alternative is a second table for a few hundred
// words.
func HyphenationOf(tag string) Language {
	tag = ascii.Lower(ascii.TrimSpace(tag))
	if tag == "" {
		return ""
	}
	primary, rest, _ := strings.Cut(tag, "-")
	script, region := "", ""
	var variants []string
	for rest != "" {
		var sub string
		sub, rest, _ = strings.Cut(rest, "-")
		switch {
		case len(sub) == 1:
			// A singleton begins an extension or a private use part — "-u-",
			// "-x-" — and nothing after it is a subtag of the language: in
			// "de-x-1901" the 1901 is private, not the traditional spelling.
			rest = ""
		case len(sub) == 4 && isAlpha(sub):
			// A script subtag is four letters, which is what tells it from a
			// region (two letters or three digits) and from a variant. The
			// first one wins.
			if script == "" {
				script = sub
			}
		case len(sub) == 2 && isAlpha(sub), len(sub) == 3 && isDigits(sub):
			if region == "" {
				region = sub
			}
		case len(sub) >= 5 || len(sub) == 4 && sub[0] >= '0' && sub[0] <= '9':
			// A variant is five to eight characters, or four beginning with a
			// digit, which is how "1901" is told from a script.
			variants = append(variants, sub)
		}
	}
	if script != "" && romanised[primary] == script {
		return Language(primary + "-" + script)
	}
	if spelling := orthographicVariants[primary]; spelling != nil {
		for _, v := range variants {
			if key := spelling(v, region); key != "" {
				return key
			}
		}
	}
	return Language(primary)
}

// isDigits reports whether s is ASCII digits and nothing else.
func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// orthographicVariants is the languages spelled in more than one orthography,
// each with patterns of its own, and the key each variant subtag names.
//
// German has two, and hyph-utf8 publishes three tables for them: the reformed
// spelling of 1996 (hyph-de-1996), the traditional spelling of 1901
// (hyph-de-1901), and the traditional spelling as Switzerland writes it, with
// no ß (hyph-de-ch-1901). BCP 47 registers "1901" and "1996" as variants of
// "de" by exactly those names — "Traditional German orthography" and "German
// orthography of 1996" — so the tag says which the text is in.
//
// The table here is the reformed one, under "de": the spelling a German text
// is in when its tag does not say otherwise, which is what Android — where
// Chromium's hyphenation comes from — assumes as well, aliasing "de" to
// "de-1996". Swiss text with no variant, "de-CH", takes it too: Switzerland
// adopted the reform, and its "ss" for "ß" is a spelling the reformed patterns
// divide already, because German writes "ss" as well.
//
// A text tagged 1901 is keyed apart from "de", and that key has no table. The
// two spellings divide differently, and not only where the words differ: the
// reform allowed "st" to be divided, so "Fenster" is "Fens-ter" under the
// reformed patterns and "Fen-ster" under the traditional ones. Reformed breaks
// in traditional text are wrong breaks, so a document that asks for the old
// spelling is told it was not hyphenated rather than hyphenated wrongly. Swiss
// and Liechtenstein text in the old spelling is keyed apart again — the same
// grouping Android makes — because upstream it is a third table and it would
// be a third one here.
var orthographicVariants = map[string]func(variant, region string) Language{
	"de": func(variant, region string) Language {
		if variant != "1901" {
			return ""
		}
		if region == "ch" || region == "li" {
			return "de-ch-1901"
		}
		return "de-1901"
	},
}

// romanised is the languages whose *romanisation* has a table of its own, and
// the script subtag that names it.
//
// One entry, and the shape is what matters: the table is for pinyin, and pinyin
// is not Chinese in any sense the patterns care about — it is a Latin
// orthography that happens to be written under a Chinese tag. A language whose
// own script is what it is hyphenated in never appears here.
var romanised = map[string]string{"zh": "latn"}

// HyphenatesLanguage reports whether there is a table for a key.
//
// The key is HyphenationOf's answer and not LanguageOf's. The two differ for
// exactly the tags romanised names, and passing the wrong one there means
// hyphenating Han text with pinyin patterns.
func HyphenatesLanguage(lang Language) bool { return hyphenationFor(lang) != nil }

// hyphenSource is one language's pattern file as cmd/genhyphen wrote it out,
// together with the table built from it on first use.
type hyphenSource struct {
	// key is what HyphenationOf resolves a lang attribute to.
	key Language
	// left and right are the hyphenmins the file states for typesetting.
	left, right int
	// patterns and exceptions are the two blocks, one entry per line.
	patterns, exceptions string

	once  sync.Once
	built *hyphenPatterns
}

// hyphenSources is every table this engine has, one generated file each.
//
// Five languages and not the hundred hyph-utf8 publishes, because each is a
// table checked into the repository and Hungarian's alone is half a megabyte.
// These five are the ones the suite asks for by name and that anybody publishes
// patterns for. The suite asks for two more, Uyghur and Cree, and no hyphenation
// resource exists for either — not in hyph-utf8, not in LibreOffice's or
// Mozilla's dictionaries, not in Android's — so text in them is read as
// "manual" and reported, as it is in every browser.
var hyphenSources = []*hyphenSource{
	&englishHyphenation,
	&dutchHyphenation,
	&hungarianHyphenation,
	&pinyinHyphenation,
	&germanHyphenation,
}

// hyphenationFor finds the table for a key, or nil.
//
// A scan of five rather than a map, because it is five: the loop is shorter than
// the hash and a document that hyphenates nothing never reaches it.
func hyphenationFor(lang Language) *hyphenSource {
	if lang == "" {
		return nil
	}
	for _, s := range hyphenSources {
		if s.key == lang {
			return s
		}
	}
	return nil
}

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

// table builds this language's patterns on first use.
//
// Lazily, because a document that hyphenates nothing should not pay for sixty
// thousand patterns, and once, because a document that hyphenates one word
// hyphenates a hundred. Per language and not once for all of them: a page of
// English should not build the Hungarian table, which is ten times the size.
func (s *hyphenSource) table() *hyphenPatterns {
	s.once.Do(func() {
		t := &hyphenPatterns{
			byLetters:  make(map[string][]int8, 5000),
			exceptions: make(map[string][]int, 16),
		}
		for _, p := range strings.Split(s.patterns, "\n") {
			if p = ascii.TrimSpace(p); p == "" {
				continue
			}
			letters, values := splitPattern(p)
			t.byLetters[letters] = values
			if n := len([]rune(letters)); n > t.longest {
				t.longest = n
			}
		}
		for _, w := range strings.Split(s.exceptions, "\n") {
			if w = ascii.TrimSpace(w); w == "" {
				continue
			}
			word, points := splitException(w)
			t.exceptions[word] = points
		}
		s.built = t
	})
	return s.built
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
		word.WriteRune(simpleLower(r))
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
