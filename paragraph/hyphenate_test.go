package paragraph

import (
	"reflect"
	"strings"
	"testing"
)

// Liang's algorithm, checked against the answers the pattern file itself
// documents.
//
// The table below is not a set of plausible-looking break points. Three of its
// rows come from documents that state their own expectation — the working
// group's hyphens-auto-control writes "frag[A]ilis[A]tic" and "ex[A]pi[A]ali"
// in a comment, and hyphens-span-002 assumes "high-way" — and one comes from
// the pattern file's own known_bugs list, which records that en-us gives
// "de-mo-c-rat" where a person would write "dem-o-crat". An implementation that
// reproduces a documented bug of the data it reads is reading the data.

func TestTheAlgorithmGivesTheBreaksThePatternsDo(t *testing.T) {
	for _, tc := range []struct {
		word string
		want []int
		why  string
	}{
		{"highway", []int{4}, "high-way, which hyphens-span-002 assumes"},
		{"fragilistic", []int{4, 8}, "frag-ilis-tic, from hyphens-auto-control's own comment"},
		{"expiali", []int{2, 4}, "ex-pi-ali, from the same comment"},
		{"hyphenation", []int{2, 6}, "hy-phen-ation"},
		{"algorithm", []int{2, 4}, "al-go-rithm"},
		{"supercalifragilistic", []int{2, 5, 8, 13, 17}, "su-per-cal-ifrag-ilis-tic"},
		{"typesetting", []int{4, 7}, "type-set-ting"},
		{"implementation", []int{2, 5, 8, 10}, "im-ple-men-ta-tion"},
		// The pattern file's known_bugs: "de-mo-c-rat: instead of dem-o-crat".
		{"democrat", []int{2, 4, 5}, "de-mo-c-rat, which the pattern file records as its own bug"},
	} {
		got := HyphenPoints(tc.word, "en", 0, 0)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v, want %v (%s)", tc.word, got, tc.want, tc.why)
		}
	}
}

// TestTheExceptionListWins. The \hyphenation{} block is what the patterns get
// wrong, and a word on it takes its breaks from the list and from nowhere else
// — including a word written with no hyphens at all, which is the list saying
// the word must not be broken.
func TestTheExceptionListWins(t *testing.T) {
	for _, tc := range []struct {
		word string
		want []int
	}{
		{"table", []int{2}},
		{"associate", []int{2, 4}},
		{"reciprocity", []int{4}},
		// Written on the list with no hyphens in it: the patterns would divide
		// the noun where the verb divides, and both spellings are one word.
		{"present", nil},
		{"presents", nil},
		{"project", nil},
		{"projects", nil},
	} {
		if got := HyphenPoints(tc.word, "en", 0, 0); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v, want %v — the exception list decides", tc.word, got, tc.want)
		}
	}
	// And the list is matched without regard to case, because a word at the
	// start of a sentence is the same word.
	if got := HyphenPoints("Present", "en", 0, 0); len(got) != 0 {
		t.Errorf("\"Present\" gave %v; the exception list is about the word", got)
	}
}

// TestTheHyphenminsKeepLettersOffTheEdges. The pattern file states them —
// left 2, right 3 for American English — and they are what stops a hyphen
// leaving one letter stranded at the end of a line or carrying one alone to the
// next.
func TestTheHyphenminsKeepLettersOffTheEdges(t *testing.T) {
	// "elephant" is el-e-phant to the patterns, and the "el-e" point is three
	// from the end... no: it is at 3, which leaves five, so it stands.
	if got := HyphenPoints("elephant", "en", 0, 0); len(got) == 0 {
		t.Error("elephant was not hyphenated at all")
	}
	// A caller may ask for more than the language wants and never for less: the
	// mins are the dictionary's answer and a wider one is the caller's.
	wide := HyphenPoints("implementation", "en", 6, 6)
	for _, p := range wide {
		if p < 6 || len("implementation")-p < 6 {
			t.Errorf("a point at %d survived limits of 6 and 6: %v", p, wide)
		}
	}
	narrow := HyphenPoints("implementation", "en", 0, 0)
	if len(wide) >= len(narrow) {
		t.Errorf("wider limits kept %v and the language's own kept %v", wide, narrow)
	}
	// And a *narrower* one than the language wants is honoured too, which is
	// what hyphenate-limit-chars is for: the author is overriding the
	// dictionary, and an author who asks to carry two letters to the next line
	// where American English wants three has asked for two.
	//
	// "university" is un-i-ver-si-ty to the patterns. The point before "ty"
	// leaves two letters, so the language's own three drops it and an explicit
	// two keeps it.
	if got := HyphenPoints("university", "en", 0, 0); !reflect.DeepEqual(got, []int{3, 6}) {
		t.Fatalf("\"university\" gave %v under the language's own mins, want [3 6]", got)
	}
	if got := HyphenPoints("university", "en", 2, 2); !reflect.DeepEqual(got, []int{3, 6, 8}) {
		t.Errorf("\"university\" gave %v with two and two, want [3 6 8] — an explicit "+
			"limit is the limit and not a floor under the language's", got)
	}
	// A word with no room for a point inside the mins gives none.
	if got := HyphenPoints("at", "en", 0, 0); got != nil {
		t.Errorf("\"at\" gave %v", got)
	}
}

// TestOnlyWordsAreHyphenated. A hyphenation dictionary is a statement about the
// letters of a language, and it has nothing to say about a string that is not
// only letters.
func TestOnlyWordsAreHyphenated(t *testing.T) {
	for _, w := range []string{"don't", "R2D2", "co-op", "implementation.", "", "..."} {
		if got := HyphenPoints(w, "en", 0, 0); got != nil {
			t.Errorf("%q gave %v", w, got)
		}
	}
	// And a word too long to be a word. A "word" of a thousand letters is a URL
	// or a hash, and a break every second character through one is not
	// hyphenation.
	//
	// Written out of a word the patterns *do* divide, so that the cap is what
	// stops it rather than the dictionary having nothing to say.
	long := strings.Repeat("implementation", 8)
	if len(long) <= maxHyphenWord {
		t.Fatalf("the fixture is %d letters and the cap is %d", len(long), maxHyphenWord)
	}
	if got := HyphenPoints(long, "en", 0, 0); got != nil {
		t.Errorf("a word of %d letters gave %d points", len(long), len(got))
	}
	if got := HyphenPoints(long[:maxHyphenWord], "en", 0, 0); len(got) == 0 {
		t.Error("the same letters inside the cap gave no points, so the cap is not " +
			"what the line above measures")
	}
}

// TestOnlyALanguageWithPatterns is §6.1's second condition, at the level that
// knows about it. The patterns here are English, and English is the primary
// subtag: LanguageOf has already cut "en-US" down to "en".
func TestOnlyALanguageWithPatterns(t *testing.T) {
	if !HyphenatesLanguage("en") {
		t.Error("English is not hyphenated, and the patterns here are English")
	}
	for _, lang := range []Language{"", "de", "fr", "nl", "e", "eng"} {
		if HyphenatesLanguage(lang) {
			t.Errorf("%q was hyphenated with the American English patterns", lang)
		}
		if got := HyphenPoints("implementation", lang, 0, 0); got != nil {
			t.Errorf("%q gave %v", lang, got)
		}
	}
}

// TestAPatternIsReadAsLettersAndNumbers pins the split the whole algorithm rests
// on: the numbers sit *between* the letters, and there is one more of them than
// there are letters so that a number at either end has somewhere to go.
func TestAPatternIsReadAsLettersAndNumbers(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		letters string
		values  []int8
	}{
		{"ach4", "ach", []int8{0, 0, 0, 4}},
		{".ad4der", ".adder", []int8{0, 0, 0, 4, 0, 0, 0}},
		{"a1bc3d", "abcd", []int8{0, 1, 0, 3, 0}},
		{"2z", "z", []int8{2, 0}},
	} {
		letters, values := splitPattern(tc.pattern)
		if letters != tc.letters || !reflect.DeepEqual(values, tc.values) {
			t.Errorf("%q read as %q %v, want %q %v",
				tc.pattern, letters, values, tc.letters, tc.values)
		}
	}
}

// TestHyphenatePiecesSplitsAtTheOffsetsItIsGiven, and the offsets are into the
// whole text rather than into any one piece: a word may be spelled across
// several, and the caller that found the word is the one holding the numbers.
func TestHyphenatePiecesSplitsAtTheOffsetsItIsGiven(t *testing.T) {
	pieces := []Piece{{Text: "high"}, {Text: "way"}}
	got, ends := HyphenatePieces(pieces, []int{4})
	if ends {
		t.Error("a point at the boundary between two pieces was read as ending the text")
	}
	if len(got) != 2 || got[0].Text != "high" || got[1].Text != "way" {
		t.Fatalf("the pieces came out %v", got)
	}
	if !got[0].Hyphen {
		t.Error("the piece before the point does not end at a hyphen")
	}
	// A point inside a piece splits it.
	got, _ = HyphenatePieces([]Piece{{Text: "highway"}}, []int{4})
	if len(got) != 2 || got[0].Text != "high" || got[1].Text != "way" {
		t.Fatalf("\"highway\" split into %v", got)
	}
	if !got[0].Hyphen || got[0].BreakBefore {
		t.Error("the first part should end at a hyphen and not begin a line")
	}
	if got[1].Hyphen || !got[1].BreakBefore {
		t.Error("the second part should begin a line and not end at a hyphen")
	}
	// A point at the very end belongs to whatever comes after this text.
	if _, ends := HyphenatePieces([]Piece{{Text: "high"}}, []int{4}); !ends {
		t.Error("a point at the end of the text was not reported as one")
	}
	// And no points is the text untouched, with nothing allocated to say so.
	if got, ends := HyphenatePieces(pieces, nil); ends || len(got) != 2 {
		t.Errorf("an empty point list changed the pieces: %v", got)
	}
}
