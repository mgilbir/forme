package paragraph

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// CSS Text §5.2's "auto-phrase": where a line may end in a language whose words
// run together and whose phrases do not.
//
// The value allows a line to end at a phrase boundary and suppresses the
// implicit opportunities inside a phrase. A phrase is not a word — "東京へ" is
// one phrase and two words — so the word lists in dictionary.go cannot answer
// this even for a language they cover, and Japanese has no list here anyway.
//
// # What a model is and why it is one
//
// Japanese is agglutinative: a phrase is a content word with its particles and
// its inflections stuck to it, so the boundary is decided by what the
// characters *are* rather than by where a dictionary entry ends. BudouX scores
// each boundary from the characters around it — three either side, taken singly,
// in pairs and in triples — and calls a positive total a phrase boundary. See
// cmd/genphrase for where the weights come from and what travels with them.
//
// It is a model and not a rule, which means it is sometimes wrong, and the
// suite says so where it matters: word-break-auto-phrase-001 calls the
// decomposition it asks for "reasonable points" and its own comment allows
// another. What the value promises is not a correct analysis but a consistent
// one, which is why the model this engine uses is the model the references were
// written from.
//
// # Where it does not reach
//
// One language. Chinese and Thai have models upstream and neither is here: no
// document in the suite asks for either, Thai already breaks at the words its
// dictionary knows, and a table nothing can be shown to need is a table nobody
// can check. The Makefile's phrases target takes the language as an argument, so
// adding one is fetching a file.
//
// The text is one box's, as everywhere else in this package. A phrase split
// across an inline boundary is two runs and is scored as two, which loses the
// context either side of the boundary: "東京<span>へ</span>行きましょう。" is
// three runs and none of them has the window the model needs.

// phraseModel is BudouX's model: a weight for each of thirteen features of the
// text around a boundary, and the score a boundary starts from.
type phraseModel struct {
	// groups are the feature weights, indexed by the constants below. One map
	// per group rather than one map keyed by group and text, because the scan
	// asks every group about a different substring and a combined key would
	// mean building thirteen strings per boundary.
	groups [phraseGroups]map[string]int
	// bias is what a boundary scores before any feature is counted, doubled.
	// See japanesePhraseBias.
	bias int
}

// The feature groups, in BudouX's order: the six characters around a boundary
// taken singly, the three pairs and the four triples.
//
// uw3 and uw4 are the two characters the boundary falls between, so the groups
// are named from the boundary outwards rather than from the start of the
// window: uw1 is three characters before it and uw6 is three after.
const (
	uw1 = iota
	uw2
	uw3
	uw4
	uw5
	uw6
	bw1
	bw2
	bw3
	tw1
	tw2
	tw3
	tw4
	phraseGroups
)

// phraseGroupNames maps a group's name in the generated table to its index. It
// is the only place the two orders are tied together.
var phraseGroupNames = map[string]int{
	"UW1": uw1, "UW2": uw2, "UW3": uw3, "UW4": uw4, "UW5": uw5, "UW6": uw6,
	"BW1": bw1, "BW2": bw2, "BW3": bw3,
	"TW1": tw1, "TW2": tw2, "TW3": tw3, "TW4": tw4,
}

// buildPhraseModel reads a generated feature table.
//
// A malformed line is skipped rather than reported: the table is generated and
// checked in beside the code that reads it, so a line this cannot parse is a
// build that was never run rather than input from a document.
func buildPhraseModel(features string, bias int) *phraseModel {
	m := &phraseModel{bias: bias}
	for i := range m.groups {
		m.groups[i] = map[string]int{}
	}
	for _, line := range strings.Split(features, "\n") {
		name, rest, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		text, weight, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		g, ok := phraseGroupNames[name]
		if !ok {
			continue
		}
		n, err := strconv.Atoi(weight)
		if err != nil {
			continue
		}
		m.groups[g][text] = n
	}
	return m
}

var (
	japaneseModelOnce sync.Once
	japaneseModel     *phraseModel
)

// phraseModelFor is the model for a writing system, or nil where this engine
// has none.
//
// Built on first use, and once, for the reason dictionaryFor builds its word
// lists that way: almost every document needs none of it.
func phraseModelFor(w WritingSystem) *phraseModel {
	if w != WritingSystemJapanese {
		return nil
	}
	japaneseModelOnce.Do(func() {
		japaneseModel = buildPhraseModel(japanesePhraseFeatures, japanesePhraseBias)
	})
	return japaneseModel
}

// HasPhraseModel reports whether this engine can find the phrases of a writing
// system.
//
// It is the question "auto-phrase" asks before it suppresses anything: §5.2
// gives the value effect only "if the UA supports phrase-based line breaking
// for the content language", and a UA that does not is to break as "normal"
// does. The suite's word-break-auto-phrase-fallback-002 is exactly that
// document — untagged content, which is no language at all.
func HasPhraseModel(w WritingSystem) bool { return phraseModelFor(w) != nil }

// PhrasesUnfound reports whether a run has phrases in it that "auto-phrase"
// would keep whole and this engine cannot find.
//
// Three things have to be true at once, and the three are why the finding fires
// where it does. The text has to have the writing the rule is about in it, or
// there are no phrases to keep. The *language* has to be one whose phrases the
// value is about, which is a declaration and not a guess — §5.2 gives the value
// effect only "if the UA supports phrase-based line breaking for the content
// language", so untagged text gets "normal" and gets it correctly, and a
// document that declares nothing is not told a feature is missing. And this
// engine has to have no model for that language, which today means Chinese: one
// model is here, BudouX publishes two more, and a page of Chinese under this
// value is set the way "normal" sets it.
func PhrasesUnfound(text string, w WritingSystem) bool {
	return w.ChineseOrJapanese() && !HasPhraseModel(w) && NeedsPhraseBreaking(text)
}

// PhraseBreaks says, for each place inside a run where a phrase could be found,
// whether one begins there.
//
// The result is keyed by byte offset and it has three answers, not two. An
// offset that is absent is one the model has nothing to say about — neither
// character it falls between is written in the model's own script, so what the
// ordinary rules allow there stands. An offset that is present and false is
// inside a phrase, and is where "auto-phrase" suppresses. An offset present and
// true is a phrase boundary.
//
// The distinction is the value's fallback written down. A document that tags
// Thai as Japanese is the suite's word-break-auto-phrase-fallback-001, and its
// assert says what must happen: "a run of Thai characters is not a phrase in
// Japanese", so the Thai breaks where Thai breaks. Marking those offsets false
// would suppress every opportunity in the run and set the paragraph as one
// unbreakable word.
//
// The scoring reads the whole run and the coverage is decided per boundary,
// which are two different questions and were once one. A Japanese sentence with
// a Latin word in it is scored with the Latin word in the window — that is what
// the model was trained on and what every other implementation gives it — while
// the boundaries *inside* the Latin word are left to the ordinary rules, which
// have their own answer about where an English word may break.
//
// As with DictionaryBreaks, the offsets are where a phrase *begins* and the
// first is excepted: a break before the first character of a run is not one the
// run offers.
func PhraseBreaks(text string, w WritingSystem) map[int]bool {
	m := phraseModelFor(w)
	if m == nil || !hasJapaneseScript(text) {
		return nil
	}
	var out map[int]bool
	m.scorePhrases(text, func(at int, boundary bool) {
		if !touchesJapaneseScript(text, at) {
			return
		}
		if out == nil {
			out = map[int]bool{}
		}
		out[at] = boundary
	})
	return out
}

// hasJapaneseScript reports whether a run has anything in it the model is about.
// It is asked first so that a run with none — which is almost every run in
// almost every document, even under a Japanese language tag — costs one scan
// rather than a table of scores.
func hasJapaneseScript(text string) bool {
	for _, r := range text {
		if inJapaneseScript(r) {
			return true
		}
	}
	return false
}

// touchesJapaneseScript reports whether the boundary at a byte offset has the
// model's script on either side of it.
func touchesJapaneseScript(text string, at int) bool {
	if r, _ := utf8.DecodeRuneInString(text[at:]); inJapaneseScript(r) {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:at])
	return inJapaneseScript(r)
}

// scorePhrases calls found for every boundary inside a run, with whether a
// phrase begins there.
//
// The offsets of the run's characters are gathered first because the model
// reads three characters either side of each boundary, and a walk that has
// reached a boundary has not read the three after it. That is one int per
// character — paid only by a run with Japanese in it, and only under a value
// that asks for this.
func (m *phraseModel) scorePhrases(s string, found func(at int, boundary bool)) {
	offs := make([]int, 0, len(s)/3+1)
	for i := range s {
		offs = append(offs, i)
	}
	offs = append(offs, len(s))
	n := len(offs) - 1
	// A run of one character has no interior boundary, and the boundary in front
	// of it is not this run's to offer.
	for i := 1; i < n; i++ {
		// The score is doubled, because the bias is: see japanesePhraseBias.
		score := m.bias
		at := func(g, from, to int) {
			if from < 0 || to > n {
				return
			}
			score += 2 * m.groups[g][s[offs[from]:offs[to]]]
		}
		at(uw1, i-3, i-2)
		at(uw2, i-2, i-1)
		at(uw3, i-1, i)
		at(uw4, i, i+1)
		at(uw5, i+1, i+2)
		at(uw6, i+2, i+3)
		at(bw1, i-2, i)
		at(bw2, i-1, i+1)
		at(bw3, i, i+2)
		at(tw1, i-3, i)
		at(tw2, i-2, i+1)
		at(tw3, i-1, i+2)
		at(tw4, i, i+3)
		found(offs[i], score > 0)
	}
}

// inJapaneseScript reports whether a character is one the Japanese model is
// about.
//
// Japanese is written in three scripts at once and punctuated in a fourth, so
// this is several blocks and not one: Han, hiragana, katakana in both their
// widths, and the CJK symbols and punctuation that separate them. The fullwidth
// forms are here for the same reason the halfwidth katakana are — a document
// that writes "０１" writes Japanese digits, and the model has weights for them.
//
// It is deliberately not IsIdeographic, which includes Hangul: Korean is not
// what this model was trained on, and a Korean paragraph tagged as Japanese
// would have every opportunity in it suppressed by a model that has never seen
// the characters. The narrower test is the safe one — a boundary left outside
// this keeps the ordinary rules, which is the fallback §5.2 asks for anyway.
func inJapaneseScript(r rune) bool {
	switch {
	case r >= 0x3000 && r <= 0x303F: // CJK symbols and punctuation
		return true
	case r >= 0x3040 && r <= 0x30FF: // Hiragana and katakana
		return true
	case r >= 0x31F0 && r <= 0x31FF: // Katakana phonetic extensions
		return true
	case r >= 0x3400 && r <= 0x4DBF: // CJK unified ideographs, extension A
		return true
	case r >= 0x4E00 && r <= 0x9FFF: // CJK unified ideographs
		return true
	case r >= 0xF900 && r <= 0xFAFF: // CJK compatibility ideographs
		return true
	case r >= 0xFF01 && r <= 0xFF9F: // Fullwidth forms and halfwidth katakana
		return true
	case r >= 0x20000 && r <= 0x2FA1F: // Extension B and beyond
		return true
	}
	return false
}
