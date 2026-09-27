// Command genlinebreak generates the set of characters a line may not begin
// with, from Unicode's own LineBreak.txt.
//
// UAX #14 states line breaking as pair rules over a character's Line_Break
// class, and a handful of those rules are unconditional prohibitions written
// "× X" — do not break before a character of class X, whatever precedes it.
// Those are the ones here, because they are the ones that can be answered by
// looking at one character:
//
//	LB11   × WJ                    a word joiner joins
//	LB13   × CL × CP × EX × SY     a line does not begin with ")", "]", "!" or "/"
//	LB15d  × IS                    nor with ";", "," or "."
//	LB21   × BA × HY × NS          nor with a hyphen, a dash, or a character
//	                               that cannot start a line
//	LB22   × IN                    nor with an ellipsis
//
// The expressions are quoted from UAX #14 rather than from its prose, and the
// difference has bitten twice. LB21's prose says "do not break before
// hyphen-minus, other hyphens, …", and the "other hyphens" — U+2010 and the
// dashes beside it — are class HH as of Unicode 16, which appears in no × rule
// at all: LB20a handles them, and it needs the character before as well as the
// one after, so it is not this table's business. And LB13 used to carry IS and
// no longer does — it moved to LB15d in revision 53, unchanged in effect, and
// reading only the rule it used to be in loses the full stop.
//
// LB15c is the one exception to LB15d and is left out because it is not a
// property of one character: "SP ÷ IS NU" breaks before a decimal point that
// follows a space, so that "subtract .5" may wrap before the number. It needs
// no code — see linebreak.go for why an opportunity a space already offered is
// not one this set withdraws.
//
// Everything else UAX #14 says is left out, and deliberately. The rules that
// depend on what came before — LB12a's "× GL unless after a space", LB15's
// quotation marks, LB25's numbers — cannot be a set of characters, and a set
// that pretended otherwise would forbid breaks that are allowed. The rules
// about mandatory breaks, spaces and combining marks are elsewhere in this
// package, where the characters they concern are already handled one at a time.
//
// The second table is CSS Text's, not UAX #14's, and it is here because it
// reads the same file. §5.1: "For Web-compatibility there is a soft wrap
// opportunity before and after each replaced element or other atomic inline,
// even when adjacent to a character that would normally suppress them,
// including U+00A0 NO-BREAK SPACE. However, with the exception of U+00A0
// NO-BREAK SPACE, there must be no soft wrap opportunity between atomic inlines
// and adjacent characters belonging to the Unicode GL, WJ, or ZWJ line breaking
// classes." So: those three classes, and the exception is applied in
// linebreak.go, where a policy belongs.
//
// What is *not* generated is the mirror question — where a line may not end,
// which is LB14's "OP SP* ×" and its neighbours. It needs no table here: the
// opportunity this package offers after an ideograph is deferred until the next
// character is known, so a break after an opening bracket is one that was never
// offered rather than one that has to be withdrawn.
//
// Every class in the file has to appear in one of the two lists below. A
// Unicode release that adds one is a build failure rather than a set of
// characters that quietly changed sides — which is how HH would have gone
// unnoticed, since it was carved out of BA.
//
// # The whole property
//
// The sets above are what this package asked of the file while it implemented
// a subset of UAX #14. It implements all of it now — paragraph/uax14.go runs
// the rules in full, and paragraph/linebreakconformance_test.go holds it to
// every case of LineBreakTest.txt — and the rules need every character's class,
// so the whole of the property is emitted too, as lineBreakClassRanges. The
// sets stay, because the exported questions they answer are asked by callers
// outside the rules.
//
// One more property is read for the rules and it is not in LineBreak.txt.
// LB30b keeps an emoji modifier with an unassigned Extended_Pictographic code
// point before it — a pictograph a later release may assign — and that is
// emoji-data.txt's Extended_Pictographic intersected with the code points
// UnicodeData.txt does not assign. Both are read here rather than asked of Go's
// package unicode, for the reason cmd/pinnedunicode_test.go gives.
//
//	go run ./cmd/genlinebreak -version <X.Y.Z> <LineBreak.txt> <emoji-data.txt> \
//		<UnicodeData.txt> > paragraph/linebreaktable.go
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/cmd/internal/ucd"
)

// forbidden is the Line_Break classes a break may not fall in front of.
var forbidden = map[string]bool{
	"WJ": true, // LB11
	"CL": true, // LB13
	"CP": true,
	"EX": true,
	"SY": true,
	"IS": true, // LB15d
	"BA": true, // LB21
	"HY": true,
	"NS": true,
	"IN": true, // LB22
}

// permitted is every other class, listed so that neither list can be silently
// incomplete. A class here is one no unconditional × rule names — which is not
// the same as one a line may always begin with, only one this table cannot
// answer for.
var permitted = map[string]bool{
	// Mandatory breaks and the characters around them, handled where the
	// characters themselves are: a segment break is a break.
	"BK": true, "CR": true, "LF": true, "NL": true, "SP": true, "ZW": true,
	"CM": true, "ZWJ": true,
	// Conditional: what may precede decides. LB12a for GL, LB15 for QU,
	// LB19 and LB20 for CB, LB23 to LB30 for the alphanumerics.
	"GL": true, "QU": true, "CB": true, "AL": true, "NU": true, "PR": true,
	"PO": true, "HL": true, "SG": true, "XX": true, "AI": true,
	"SA": true, "B2": true, "BB": true, "HH": true,
	// An opening bracket is a fine thing to begin a line with. It is ending
	// one on it that LB14 forbids, and that is the other question — see the
	// note above about why it needs no table.
	"OP": true,
	// Ideographs and the scripts that break like them: a line may begin with
	// any of these, which is the whole reason CJK wraps without spaces.
	"ID": true, "CJ": true, "EB": true, "EM": true, "RI": true,
	"H2": true, "H3": true, "JL": true, "JT": true, "JV": true,
	// Brahmic clusters, whose breaking is LB28a's own business.
	"AK": true, "AP": true, "AS": true, "VF": true, "VI": true,
}

// The three sets CSS Text §5.3's line-break tailoring needs on top of UAX #14's
// default, which is what "normal" already is.
//
// §5.3 states the tailoring as lists of characters and of Line_Break classes,
// and both are here for the same reason the rest of this file is: the lists are
// Unicode's and the *policy* — which value uses which — belongs in linebreak.go.
//
//   - strictNoBreak: a line may not begin with one of these under "strict".
//     Class CJ is UAX #14's Conditional Japanese Starter, which is exactly the
//     small kana and the prolonged sound mark, and the report's own rule is to
//     resolve it to NS under a strict tailoring and to ID otherwise. The
//     hyphens beside it in §5.3 are named one code point at a time rather than
//     by a class, so they are not here: 〜 and ゠ are class NS, the *base* table
//     already forbids them, and letting them through again under "normal" and
//     "loose" is a policy — paragraph/linebreak.go's isEastAsianHyphen.
//   - looseBreak: a line *may* begin with one of these under "loose", which
//     means taking them back out of the base table. §5.3 names the iteration
//     marks and the centred punctuation one code point at a time and names two
//     whole classes, IN and PO.
//   - prefixBreak: class PR, which is the one rule stated the other way round —
//     under "loose" a line may end *after* a prefix, which no other value
//     allows.
var strictNoBreakClasses = map[string]bool{"CJ": true}

// The characters "loose" allows a line to begin with, code point by code point:
// the iteration marks and the centred punctuation. §5.3's hyphens are not
// here, because "normal" allows them too; see strictNoBreakClasses above.
var looseBreakRunes = []rune{
	0x3005, 0x303B, 0x309D, 0x309E, 0x30FD, 0x30FE, // iteration marks
	0x30FB, 0xFF1A, 0xFF1B, 0xFF65, 0x203C, 0x2047, 0x2048, 0x2049, 0xFF01, 0xFF1F,
}

// And the two classes it names whole.
var looseBreakClasses = map[string]bool{"IN": true, "PO": true}

// inseparableClasses is UAX #14's class for the ellipses, and "line-break:
// loose" is the one value that lets a line break inside a run of them.
//
// It is the same class looseBreakClasses already asks for, and it is asked for
// twice because the two rules are different: that one is about a line *beginning*
// with an ellipsis, which is LB22 relaxed, and this one is about a break between
// two of them, which is an opportunity nothing else creates. §5.3 states them as
// one sentence — "breaks are allowed ... between inseparable characters (such as
// U+2025 and U+2026)" — and an engine that reads only the first half offers a
// line beginning with an ellipsis it can never break in front of.
var inseparableClasses = map[string]bool{"IN": true}

// openClasses is UAX #14's class for an opening bracket, which LB14 forbids a
// line to end after: "OP SP* ×".
//
// It is here for the same reason prefixClasses is. Ordinary text offers no
// opportunity after a bracket, so nothing asks; "word-break: break-all" offers
// one at every character boundary in a word, and then the question is real. §5.2
// allows breaking "between typographic character units" and word-break-break-all-020
// says in its own assertion what that does not reach: "break-all does not affect
// rules governing the soft wrap opportunities created by punctuation".
var openClasses = map[string]bool{"OP": true}

// dictionaryClasses is UAX #14's SA: the South East Asian scripts whose words
// are found by lexical analysis rather than by looking for a space.
//
// LB1 resolves SA to AL, which is "no opportunity anywhere", and notes that an
// implementation with a dictionary does better. This engine has no dictionary
// and CSS Text §5.1 does not accept the resolution either way: "some form of
// fallback line breaking must occur even if the UA doesn't know how to perform
// it correctly. Overflowing is not allowed." So the fallback is the boundary
// between two typographic character units, which is where the words are not.
var dictionaryClasses = map[string]bool{"SA": true}

// ideographicClasses is what breaks like an ideograph: a character a line may
// end after and begin with, which is what lets CJK wrap without spaces.
//
// ID is Unicode's own Ideographic class. CJ is the Conditional Japanese
// Starter — the small kana and the prolonged sound mark — which UAX #14 leaves
// to a tailoring and CSS Text §5.3 resolves to ID under every value but
// "strict"; the strict prohibition is strictNoBreakRanges above, so this is
// where the other three values get their answer. H2 and H3 are the Hangul
// syllables, which wrap the same way and which no reader of this table would
// think to look for under "ideographic".
//
// The conjoining jamo are not here but in jamoClasses: they break like an
// ideograph only between syllables, and a table that said they break like one
// everywhere would be read by callers that do not ask where the syllable ends.
//
// It replaces six ranges typed out by hand — the two main CJK blocks, the
// compatibility ideographs, kana, Hangul syllables, and everything from
// U+20000 to U+2FA1F. Halfwidth katakana, the fullwidth Latin letters,
// extensions G and H, Yi, Bopomofo, the Kangxi radicals and the enclosed CJK
// numerals are all class ID and were in none of them, so none of them wrapped
// at all.
var ideographicClasses = map[string]bool{"ID": true, "CJ": true, "H2": true, "H3": true}

// jamoClasses is the Hangul conjoining jamo, UAX #14's JL, JV and JT: a
// syllable spelt in the letters it is made of rather than as one precomposed
// character.
//
// Outside a syllable they break as the syllables H2 and H3 do. LB26 and LB27
// are the only rules that name them, and LB27 gives them what LB23a gives ID;
// between two syllables LB31 allows the break, as it does between two
// precomposed ones. Inside a syllable LB26 forbids it: JL × (JL | JV | H2 | H3),
// (JV | H2) × (JV | JT), (JT | H3) × JT. Those are UAX #29's GB6, GB7 and GB8
// class for class, so the syllable is a grapheme cluster, and SplitAtBreaks
// never cuts inside one — which is what lets these share the ideograph's
// opportunity without offering one between the pieces of a syllable.
//
// They were once deliberately left out of ideographicClasses for want of that,
// and a paragraph of syllables spelt in jamo then had no opportunity in it at
// all.
var jamoClasses = map[string]bool{"JL": true, "JV": true, "JT": true}

// prefixClasses is the class a line may end after under "loose" and no other
// value: a currency sign or a number sign that belongs to the figure following
// it.
var prefixClasses = map[string]bool{"PR": true}

// postfixClasses is the class every value but "loose" forbids a line to begin
// with. It is not in UAX #14's unconditional rules — nothing there says a line
// may not start with a per-cent sign — so it is the one part of the base table
// this adds to rather than takes away from.
var postfixClasses = map[string]bool{"PO": true}

// binding is the classes that hold on to an atomic inline beside them, CSS
// Text §5.1. It overlaps forbidden and is not a subset of it: WJ is in both, GL
// is in neither of UAX #14's unconditional rules, and ZWJ is a rule about what
// follows rather than what precedes.
var binding = map[string]bool{
	"GL":  true,
	"WJ":  true,
	"ZWJ": true,
}

type span struct {
	lo, hi rune
	class  string
}

func main() {
	version := flag.String("version", "", "the Unicode version the file came from")
	flag.Parse()
	args := flag.Args()
	if len(args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: genlinebreak -version <X.Y.Z> <LineBreak.txt> "+
			"<emoji-data.txt> <UnicodeData.txt>")
		os.Exit(2)
	}
	if err := ucd.Check(*version, args...); err != nil {
		fmt.Fprintln(os.Stderr, "genlinebreak:", err)
		os.Exit(1)
	}
	pictUnassigned, err := extPictUnassigned(args[1], args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "genlinebreak:", err)
		os.Exit(1)
	}
	f, err := os.Open(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	var spans, glue, strict, loose, prefix, postfix, inseparable, open, dict []span
	var ideographic, jamo, all []span
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Split(line, ";")
		if len(fields) < 2 {
			continue
		}
		class := strings.TrimSpace(fields[1])
		seen[class] = true
		if !forbidden[class] && !permitted[class] {
			fmt.Fprintf(os.Stderr, "genlinebreak: %s is a line-break class neither list "+
				"names; decide whether a line may begin with one\n", class)
			os.Exit(1)
		}
		lo, hi, ok := parseRange(strings.TrimSpace(fields[0]))
		if !ok {
			continue
		}
		all = append(all, span{lo, hi, class})
		if forbidden[class] {
			spans = append(spans, span{lo, hi, class})
		}
		if binding[class] {
			glue = append(glue, span{lo, hi, class})
		}
		if strictNoBreakClasses[class] {
			strict = append(strict, span{lo, hi, class})
		}
		if looseBreakClasses[class] {
			loose = append(loose, span{lo, hi, class})
		}
		if prefixClasses[class] {
			prefix = append(prefix, span{lo, hi, class})
		}
		if postfixClasses[class] {
			postfix = append(postfix, span{lo, hi, class})
		}
		if dictionaryClasses[class] {
			dict = append(dict, span{lo, hi, class})
		}
		if openClasses[class] {
			open = append(open, span{lo, hi, class})
		}
		if ideographicClasses[class] {
			ideographic = append(ideographic, span{lo, hi, class})
		}
		if jamoClasses[class] {
			jamo = append(jamo, span{lo, hi, class})
		}
		if inseparableClasses[class] {
			inseparable = append(inseparable, span{lo, hi, class})
		}
	}
	// The characters §5.3 names one at a time go in beside the classes.
	for _, r := range looseBreakRunes {
		loose = append(loose, span{r, r, "named"})
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Every class asked for has to exist in the file. A class renamed upstream
	// would otherwise drop out of the set silently, and the characters it holds
	// would quietly become places a line may begin.
	for class := range binding {
		if !seen[class] {
			fmt.Fprintf(os.Stderr, "genlinebreak: no character has class %s; has it been renamed?\n", class)
			os.Exit(1)
		}
	}
	for class := range forbidden {
		if !seen[class] {
			fmt.Fprintf(os.Stderr, "genlinebreak: no character has class %s; has it been renamed?\n", class)
			os.Exit(1)
		}
	}
	for class := range strictNoBreakClasses {
		if !seen[class] {
			fmt.Fprintf(os.Stderr, "genlinebreak: no character has class %s; has it been renamed?\n", class)
			os.Exit(1)
		}
	}
	for _, set := range []map[string]bool{looseBreakClasses, prefixClasses, postfixClasses,
		inseparableClasses, openClasses,
		dictionaryClasses, ideographicClasses, jamoClasses} {
		for class := range set {
			if !seen[class] {
				fmt.Fprintf(os.Stderr, "genlinebreak: no character has class %s; has it been renamed?\n", class)
				os.Exit(1)
			}
		}
	}
	if len(spans) == 0 || len(glue) == 0 {
		fmt.Fprintln(os.Stderr, "genlinebreak: no lines matched")
		os.Exit(1)
	}

	var w strings.Builder
	fmt.Fprintf(&w, `// Code generated by cmd/genlinebreak from Unicode's LineBreak.txt,
// emoji-data.txt and UnicodeData.txt. DO NOT EDIT.

package paragraph

// lineBreakUnicodeVersion is the release these tables were generated from. The
// conformance test reads it, so LineBreakTest.txt from one release run against
// tables from another is a failure rather than a puzzle.
const lineBreakUnicodeVersion = %q
`, *version)
	emit(&w, "noBreakBeforeRanges", spans, `// The characters a line may not begin with. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// They are the classes UAX #14 forbids a break in front of unconditionally —
// see cmd/genlinebreak for which rules those are and which were left out. What
// this package does with them is decided in linebreak.go; this table is
// Unicode's statement rather than a policy.`, *version)
	emit(&w, "bindingRanges", glue, `// The characters that hold on to an atomic inline beside them. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// CSS Text §5.1 names the three classes: a picture may be wrapped away from the
// word next to it, and may not be wrapped away from a character of one of
// these. The one exception the rule makes — U+00A0, which is class GL and
// breaks anyway, for compatibility with what the web already does — is in
// linebreak.go, because it is a decision rather than a property.`, *version)
	emit(&w, "strictNoBreakRanges", strict, `// The characters "line-break: strict" adds to the set a line may not begin
// with. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// Class CJ, the Conditional Japanese Starter: the small kana and the prolonged
// sound mark. UAX #14 leaves the class to a tailoring to resolve, and CSS Text
// §5.3 is that tailoring — NS under strict, ID under everything else.`, *version)
	emit(&w, "looseBreakRanges", loose, `// The characters "line-break: loose" allows a line to begin with, taking them
// back out of the set above. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// The iteration marks and the centred punctuation are named by §5.3 one code
// point at a time and appear here as "named"; IN and PO are classes it names
// whole.`, *version)
	emit(&w, "prefixRanges", prefix, `// The characters "line-break: loose" allows a line to end *after*, which no
// other value does. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.`, *version)
	emit(&w, "postfixRanges", postfix, `// The characters every value but "loose" forbids a line to begin with.
// Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// UAX #14 has no unconditional rule about them — nothing there says a line may
// not start with a per-cent sign — so this is the one part of the tailoring
// that adds to the base table rather than taking away from it.`, *version)
	emit(&w, "ideographicRanges", ideographic, `// The characters that break like an ideograph, UAX #14's classes ID and CJ and
// the Hangul syllables H2 and H3. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// A line may end after one of these and begin with one, which is the whole of
// how CJK wraps without spaces. The prohibitions that take some of those
// opportunities back — a line may not begin with a small kana under
// "line-break: strict" — are the tables above; this one is the opportunity.
//
// See ideographicClasses in cmd/genlinebreak for what is deliberately left
// out, and for the six hand-typed ranges this replaces.`, *version)
	emit(&w, "jamoRanges", jamo, `// The Hangul conjoining jamo, UAX #14's classes JL, JV and JT. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// They break as the Hangul syllables do, between one syllable and the next and
// never inside one. See jamoClasses in cmd/genlinebreak for why the grapheme
// cluster is what says where the syllable ends.`, *version)
	emit(&w, "dictionaryRanges", dict, `// The scripts whose words are found with a dictionary, UAX #14's class SA.
// Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// Thai, Lao, Khmer, Myanmar, Tai Le, New Tai Lue, Tai Tham and their
// neighbours: written without spaces and without a mark between words either,
// so the only way to know where a line may break is to know the language. See
// dictionaryClasses in cmd/genlinebreak for what is done instead.`, *version)
	emit(&w, "openRanges", open, `// The opening brackets, UAX #14's class OP. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// LB14 forbids a line to end after one — "OP SP* ×" — and nothing in ordinary
// text asks, because ordinary text offers no opportunity there to forbid.
// "word-break: break-all" offers one at every character boundary in a word, and
// §5.2 does not reach past the punctuation rules to take it: see gluedPair.`, *version)
	emit(&w, "inseparableRanges", inseparable, `// The ellipses, UAX #14's class IN. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// "line-break: loose" is the one value that lets a line break inside a run of
// them — §5.3's "breaks are allowed ... between inseparable characters" — and
// nothing else in this file creates that opportunity, because LB22 is a
// prohibition and a relaxed prohibition still needs something to relax. The
// same characters are in looseBreakRanges for the other half of the sentence,
// which is a line *beginning* with one.`, *version)
	emitClasses(&w, all, *version)
	emit(&w, "extPictUnassignedRanges", pictUnassigned, `// The Extended_Pictographic code points no character is assigned to yet.
// Unicode %s.
//
// %d ranges, merged from %d: %s.
// UAX #14's LB30b keeps an emoji modifier with one of these before it, as it
// keeps one with an emoji base: "[\p{Extended_Pictographic}&\p{Cn}] × EM". A
// pictograph assigned in a later release is likely to be a base, and text
// written with it should not break differently for a reader whose tables are
// older. Read from emoji-data.txt and UnicodeData.txt; see cmd/genlinebreak.`, *version)
	fmt.Print(w.String())
}

// emitClasses writes every character's Line_Break class, which is what UAX #14's
// rules are stated over: runs of one class merged, the classes named by the Go
// constants paragraph/uax14.go defines for them. A code point the file does not
// list is XX, which is what its "@missing" line says, and is left out of the
// table for the lookup to answer.
func emitClasses(w *strings.Builder, spans []span, version string) {
	sort.Slice(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })
	var merged []span
	for _, s := range spans {
		if s.class == "XX" {
			continue
		}
		if n := len(merged); n > 0 && merged[n-1].class == s.class && s.lo == merged[n-1].hi+1 {
			merged[n-1].hi = s.hi
			continue
		}
		if n := len(merged); n > 0 && s.lo <= merged[n-1].hi {
			fmt.Fprintf(os.Stderr, "genlinebreak: %04X..%04X overlaps the range before it\n",
				s.lo, s.hi)
			os.Exit(1)
		}
		merged = append(merged, s)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, `// The Line_Break class of every character, as LineBreak.txt states it.
// Unicode %s.
//
// %d ranges, from %d lines. What is not here is XX, which the file's "@missing"
// line gives every code point it does not list. The rules in uax14.go resolve
// the classes that need resolving — AI, SA, CJ, SG and XX — so this is the
// file's statement and not a policy.
var lineBreakClassRanges = [...]lineBreakRange{
`, version, len(merged), len(spans))
	for _, s := range merged {
		fmt.Fprintf(w, "\t{0x%04X, 0x%04X, lb%s},\n", s.lo, s.hi, s.class)
	}
	fmt.Fprintln(w, "}")
}

// extPictUnassigned is Extended_Pictographic, from emoji-data.txt, minus every
// code point UnicodeData.txt assigns.
func extPictUnassigned(emojiPath, unicodeDataPath string) ([]span, error) {
	assigned, err := ucd.UnicodeData(unicodeDataPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(emojiPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []span
	found := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Split(line, ";")
		if len(fields) < 2 || strings.TrimSpace(fields[1]) != "Extended_Pictographic" {
			continue
		}
		found = true
		lo, hi, ok := parseRange(strings.TrimSpace(fields[0]))
		if !ok {
			return nil, fmt.Errorf("%s: cannot read the range %q", emojiPath, fields[0])
		}
		for r := lo; r <= hi; r++ {
			if _, ok := assigned[r]; !ok {
				out = append(out, span{r, r, "ExtPict&Cn"})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !found || len(out) == 0 {
		return nil, fmt.Errorf("%s has no unassigned Extended_Pictographic code point; "+
			"has the property been renamed?", emojiPath)
	}
	return out, nil
}

// emit writes one table: the ranges sorted and merged, under a comment that
// says which classes went into it and how many of each.
func emit(w *strings.Builder, name string, spans []span, doc, version string) {
	counts := map[string]int{}
	for _, s := range spans {
		counts[s.class]++
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })
	// Adjacent runs are stated separately in the file, by class, and are one
	// range as far as this lookup is concerned.
	merged := []span{spans[0]}
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.lo <= last.hi+1 {
			if s.hi > last.hi {
				last.hi = s.hi
			}
			continue
		}
		merged = append(merged, s)
	}

	classes := make([]string, 0, len(counts))
	for c := range counts {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	byClass := make([]string, 0, len(classes))
	for _, c := range classes {
		byClass = append(byClass, fmt.Sprintf("%s %d", c, counts[c]))
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, doc+"\n", version, len(merged), len(spans), strings.Join(byClass, ", "))
	fmt.Fprintf(w, "var %s = [...]struct{ lo, hi rune }{\n", name)
	for _, s := range merged {
		fmt.Fprintf(w, "\t{0x%04X, 0x%04X},\n", s.lo, s.hi)
	}
	fmt.Fprintln(w, "}")
}

func parseRange(s string) (rune, rune, bool) {
	lo, hi, found := strings.Cut(s, "..")
	a, err := strconv.ParseUint(lo, 16, 32)
	if err != nil {
		return 0, 0, false
	}
	if !found {
		return rune(a), rune(a), true
	}
	b, err := strconv.ParseUint(hi, 16, 32)
	if err != nil {
		return 0, 0, false
	}
	return rune(a), rune(b), true
}
