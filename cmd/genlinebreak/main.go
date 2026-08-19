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
//	go run ./cmd/genlinebreak <LineBreak.txt> > paragraph/linebreaktable.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
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
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: genlinebreak <LineBreak.txt>")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	version := "unknown"
	var spans, glue []span
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "# LineBreak-") {
			version = strings.TrimSuffix(strings.TrimPrefix(line, "# LineBreak-"), ".txt")
		}
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
		if forbidden[class] {
			spans = append(spans, span{lo, hi, class})
		}
		if binding[class] {
			glue = append(glue, span{lo, hi, class})
		}
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
	if len(spans) == 0 || len(glue) == 0 {
		fmt.Fprintln(os.Stderr, "genlinebreak: no lines matched")
		os.Exit(1)
	}

	var w strings.Builder
	fmt.Fprintf(&w, `// Code generated by cmd/genlinebreak from Unicode's LineBreak.txt.
// DO NOT EDIT.

package paragraph
`)
	emit(&w, "noBreakBeforeRanges", spans, `// The characters a line may not begin with. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// They are the classes UAX #14 forbids a break in front of unconditionally —
// see cmd/genlinebreak for which rules those are and which were left out. What
// this package does with them is decided in linebreak.go; this table is
// Unicode's statement rather than a policy.`, version)
	emit(&w, "bindingRanges", glue, `// The characters that hold on to an atomic inline beside them. Unicode %s.
//
// %d ranges, merged from %d the file states separately: %s.
// CSS Text §5.1 names the three classes: a picture may be wrapped away from the
// word next to it, and may not be wrapped away from a character of one of
// these. The one exception the rule makes — U+00A0, which is class GL and
// breaks anyway, for compatibility with what the web already does — is in
// linebreak.go, because it is a decision rather than a property.`, version)
	fmt.Print(w.String())
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
