// Command geneastasian generates the two tables CSS Text's segment break
// transformation needs, from Unicode's EastAsianWidth.txt and Scripts.txt.
//
// §4.1.1 states the rule this way:
//
//	Otherwise, if the East Asian Width property of both the character before
//	and after the segment break is F, W, or H (not A), and neither side is
//	Hangul, then the segment break is removed.
//
// So a newline written between two ideographs disappears, and a newline between
// an ideograph and a Latin word becomes a space. That is not a nicety: Japanese
// and Chinese are written without spaces between words, so a paragraph that was
// hard-wrapped in the source gains a space at the end of every line it was
// wrapped at — one for each line of the source, in the middle of words, all
// through the text.
//
// Two tables rather than one, because they are two of Unicode's statements and
// combining them here would bury the rule's second half in a table nobody could
// check against the file it came from. whitespace.go is where the "and neither
// side is Hangul" lives, next to the rest of the rule.
//
// Three things about the width table are worth stating, because each is a way
// to get it quietly wrong:
//
//   - A is excluded. The ambiguous characters — the Greek letters, the box
//     drawing, U+25A0 BLACK SQUARE — are wide in an East Asian context and
//     narrow elsewhere, and the rule names F, W and H rather than saying "wide".
//     The suite tests it: segment-break-transformation-rules-005 is a black
//     square and asks for the space to be kept.
//
//   - H is included, and it is the *halfwidth* forms. They are narrow on the
//     page and they are East Asian, which is what the property is about; the
//     suite's rules-002 and rules-008 through -010 are halfwidth katakana.
//
//   - The unassigned code points of three blocks default to W rather than to
//     the file's N, and are listed here because the file says so in its header
//     rather than in its data. Leaving them out would make a newline between
//     two ideographs behave differently depending on whether the ideographs
//     happen to be assigned yet.
//
// # The second clause
//
// §4.1.1 has a second sentence that removes a segment break, and it is about
// the punctuation East Asian text is written with:
//
//	If the writing system of the segment break is Chinese, Japanese, or Yi,
//	and the character before or after the segment break is punctuation or a
//	symbol (Unicode general category P* or S*) and has an East Asian Width
//	property of A or is Emoji, and the character on the other side of the
//	segment break is F, W, or H, and not Hangul or Emoji, then the segment
//	break is removed.
//
// A quotation mark is the everyday case and is what the suite tests with: “ and
// ” are punctuation whose East Asian Width is *Ambiguous*, so the first sentence
// says nothing about them and a Japanese paragraph hard-wrapped after an opening
// quote gains a space between the quote and the word it opens.
//
// Three more of Unicode's own sets, and three rather than one derived set on
// purpose: the policy in whitespace.go then reads as the sentence above reads,
// and each table can be checked against the file it came from. Emoji is in two
// of the sentence's clauses, which is a second reason not to fold it into
// either.
//
//	go run ./cmd/geneastasian <EastAsianWidth.txt> <Scripts.txt> \
//	    <UnicodeData.txt> <emoji-data.txt> > paragraph/eastasiantable.go
package main

import (
	"bufio"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strconv"
	"strings"
)

// span is one range of characters that share a property value.
type span struct{ lo, hi rune }

// wideByDefault is what EastAsianWidth.txt's header states and its data does
// not: the unassigned code points of these blocks are W.
//
// The first three are named blocks; the last two are whole planes, "all
// undesignated code points in Planes 2 and 3, whether inside or outside of
// allocated blocks".
var wideByDefault = []span{
	{0x3400, 0x4DBF}, {0x4E00, 0x9FFF}, {0xF900, 0xFAFF},
	{0x20000, 0x2FFFD}, {0x30000, 0x3FFFD},
}

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: geneastasian <EastAsianWidth.txt> <Scripts.txt> "+
			"<UnicodeData.txt> <emoji-data.txt>")
		os.Exit(2)
	}
	wide := read(os.Args[1], func(v string) bool {
		return v == "F" || v == "W" || v == "H"
	})
	wide = append(wide, wideByDefault...)
	hangul := read(os.Args[2], func(v string) bool { return v == "Hangul" })
	// A is the whole of the ambiguous set and carries no default for the
	// unassigned code points: EastAsianWidth.txt's header gives those to N or to
	// W, never to A, so what the file says is the whole answer.
	ambiguous := read(os.Args[1], func(v string) bool { return v == "A" })
	punct := readCategory(os.Args[3], func(cat string) bool {
		return strings.HasPrefix(cat, "P") || strings.HasPrefix(cat, "S")
	})
	// Emoji, not Emoji_Presentation: the sentence says "is Emoji", and the
	// property of that name is the one that answers it. The difference is the
	// characters that are emoji only with a variation selector — "#", "*", the
	// digits — and they are in for the same reason the sentence names them.
	emoji := read(os.Args[4], func(v string) bool { return v == "Emoji" })

	var b strings.Builder
	b.WriteString(`// Code generated by cmd/geneastasian from Unicode's EastAsianWidth.txt,
// Scripts.txt, UnicodeData.txt and emoji-data.txt.
// DO NOT EDIT.

package paragraph

`)
	emit(&b, "eastAsianWideRanges", wide,
		`// The characters whose East Asian Width is F, W or H. Unicode 17.0.0.
//
// Not A, which is the ambiguous set — wide in an East Asian context and narrow
// elsewhere — and which CSS Text's segment break rule names as excluded. See
// cmd/geneastasian, which is also where the unassigned code points that default
// to W come from.`)
	emit(&b, "hangulRanges", hangul,
		`// The characters whose script is Hangul. Unicode 17.0.0.
//
// They are wide and they are the one East Asian script CSS Text's segment break
// rule carves out, because Korean is written with spaces between its words: a
// newline between two Hangul syllables is a word boundary and has to stay one.`)

	emit(&b, "eastAsianAmbiguousRanges", ambiguous,
		`// The characters whose East Asian Width is A. Unicode 17.0.0.
//
// Ambiguous: wide in an East Asian context and narrow elsewhere. The first
// sentence of CSS Text's segment break rule excludes them by name and the
// second is *about* them — a quotation mark is ambiguous, and a Japanese
// paragraph wrapped after an opening quote is what the second sentence exists
// for.`)
	emit(&b, "punctuationOrSymbolRanges", punct,
		`// The characters whose general category begins with P or S. Unicode 17.0.0.
//
// Punctuation and symbols, which is the class CSS Text's second segment break
// sentence names. It is Unicode's own set rather than the part of it the rule
// can reach, so that the policy in whitespace.go reads as the sentence does and
// this can be checked against UnicodeData.txt.`)
	emit(&b, "emojiRanges", emoji,
		`// The characters with the Emoji property. Unicode 17.0.0.
//
// It appears in both halves of CSS Text's second segment break sentence: an
// emoji on one side of the break counts as the punctuation-or-symbol that
// triggers the rule, and an emoji on the other side is carved out of the wide
// characters that would otherwise satisfy it. Emoji rather than
// Emoji_Presentation, because "is Emoji" is what the sentence says.`)

	src, err := format.Source([]byte(b.String()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "generated source does not parse:", err)
		os.Exit(1)
	}
	os.Stdout.Write(src)
}

// read returns the ranges of a UCD property file whose value the predicate
// accepts, merged where they abut.
func read(path string, want func(string) bool) []span {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	var out []span
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		fields := strings.Split(text, ";")
		if len(fields) < 2 {
			continue
		}
		if !want(strings.TrimSpace(fields[1])) {
			continue
		}
		lo, hi := parseSpan(strings.TrimSpace(fields[0]), line)
		out = append(out, span{lo, hi})
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return out
}

// merge sorts the spans and joins the ones that touch, so the table is as short
// as the data allows and the search over it is a search over ranges rather than
// over a list that happens to be contiguous.
func merge(in []span) []span {
	sort.Slice(in, func(i, j int) bool { return in[i].lo < in[j].lo })
	var out []span
	for _, s := range in {
		if n := len(out); n > 0 && s.lo <= out[n-1].hi+1 {
			if s.hi > out[n-1].hi {
				out[n-1].hi = s.hi
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

func emit(b *strings.Builder, name string, spans []span, doc string) {
	spans = merge(spans)
	fmt.Fprintf(b, "%s\n//\n// %d ranges.\nvar %s = [...]struct{ lo, hi rune }{\n",
		doc, len(spans), name)
	for _, s := range spans {
		fmt.Fprintf(b, "\t{%#04X, %#04X},\n", s.lo, s.hi)
	}
	b.WriteString("}\n\n")
}

func parseSpan(field string, line int) (rune, rune) {
	lo, hi, found := strings.Cut(field, "..")
	if !found {
		hi = lo
	}
	return parseRune(lo, line), parseRune(hi, line)
}

func parseRune(s string, line int) rune {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 16, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "line %d: %v\n", line, err)
		os.Exit(1)
	}
	return rune(v)
}

// readCategory returns the ranges of UnicodeData.txt whose general category the
// predicate accepts.
//
// A file of its own function because UnicodeData.txt is not a property file: it
// is one line per character with the category in the third field, and a range is
// written as a pair of lines whose names end ", First>" and ", Last>". The
// ranges are all ideographs, surrogates and private use — no category this is
// ever asked about — but they are read rather than skipped, because a reader
// checking this against the file should not have to know that.
func readCategory(path string, want func(string) bool) []span {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	var out []span
	var pending rune
	havePending := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		fields := strings.Split(sc.Text(), ";")
		if len(fields) < 3 {
			continue
		}
		cp := parseRune(fields[0], line)
		name, cat := fields[1], strings.TrimSpace(fields[2])
		switch {
		case strings.HasSuffix(name, ", First>"):
			pending, havePending = cp, want(cat)
			continue
		case strings.HasSuffix(name, ", Last>"):
			if havePending {
				out = append(out, span{pending, cp})
			}
			havePending = false
			continue
		}
		if want(cat) {
			out = append(out, span{cp, cp})
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return out
}
