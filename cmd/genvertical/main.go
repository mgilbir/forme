// Command genvertical generates the table layout/writingmode.go's gate reads,
// from Unicode's VerticalOrientation.txt.
//
// UAX #50 gives every character one of four values, and the question this table
// answers is which of them stand *upright* on a line of vertical text:
//
//	U  - Upright, the same orientation as in the code charts
//	R  - Rotated 90 degrees clockwise compared to the code charts
//	Tu - Transformed typographically, with fallback to Upright
//	Tr - Transformed typographically, with fallback to Rotated
//
// So U and Tu are upright and R and Tr are not. The two transformed values name
// characters a font may set with a substitution of its own — the vertical forms
// of the brackets and the kana, which "vert" and "vrt2" swap in — and their
// fallback is what a face without those features produces. This engine applies
// no vertical feature, so the fallback is what it would draw, and the fallback
// is what the table records.
//
// # Why an engine that lays out one writing mode needs it
//
// It does not need it to *set* upright text. It needs it to know that it cannot.
//
// A vertical-rl box is laid out by turning a horizontal one ninety degrees
// clockwise — see layout/writingmode.go — and that produces a page where every
// character is rotated. For a paragraph of Latin text that is exactly right:
// "text-orientation: mixed" rotates every character whose Vertical_Orientation
// is R, and Latin is R throughout. For a paragraph of Japanese it is exactly
// wrong, because ideographs are U and stand upright, and no rotation of a
// horizontal line produces an upright one.
//
// The table is what lets the engine tell the two apart and report the second
// rather than drawing it wrong. Erring towards upright is therefore the safe
// direction: a character wrongly called upright costs a finding on a page that
// would have been right, and a character wrongly called rotated costs a page
// that is wrong with nothing said about it.
//
// # The code points the data does not list
//
// The file's data is explicit code points, and what it says about the rest is
// its "# @missing:" line — the machine-readable form of a property's default,
// which UAX #44 defines for every file of the database. In 17.0.0 there is one,
// "0000..10FFFF; R", because the data has listed every upright code point
// explicitly since 15.1, unassigned ones included: the reserved code points of
// the CJK blocks, the private use areas.
//
// The header also carries a prose list of ranges whose unassigned code points
// "default to U", from the releases before the data was explicit. It is not
// read. This generator used to carry a copy of it, which had drifted from the
// header it claimed to be copied from — U+FF00..FFEF where the header now names
// only U+FFE7, among others — and made eighteen unassigned code points upright
// that the data calls R. A default read from the @missing lines cannot drift
// from the file, and a release that states an upright default there gets it:
// the defaults are applied in the order they appear, a later line overriding
// an earlier one as UAX #44 says, to every code point the data is silent on.
//
// Usage:
//
//	genvertical -version <X.Y.Z> VerticalOrientation.txt > paragraph/verticaltable.go
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

// uprightValues are the Vertical_Orientation values that stand upright.
var uprightValues = map[string]bool{"U": true, "Tu": true}

// rotatedValues are the other two. Both lists are stated so that a value the
// file gains is an error here rather than a silent omission from one of them.
var rotatedValues = map[string]bool{"R": true, "Tr": true}

type span struct {
	lo, hi rune
	class  string
}

func main() {
	version := flag.String("version", "", "the Unicode version the file came from")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: genvertical -version <X.Y.Z> <VerticalOrientation.txt>")
		os.Exit(2)
	}
	if err := ucd.Check(*version, args...); err != nil {
		fail(err.Error())
	}
	f, err := os.Open(args[0])
	if err != nil {
		fail(err.Error())
	}
	defer f.Close()

	var upright, defaults []span
	// Every code point the data mentions, whatever its value: a default
	// applies only where the file is silent.
	stated := map[rune]bool{}
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if rest, ok := strings.CutPrefix(line, "# @missing:"); ok {
			// "# @missing: 0000..10FFFF; R"
			fields := strings.Split(rest, ";")
			lo, hi, ok := parseRange(strings.TrimSpace(fields[0]))
			if len(fields) != 2 || !ok {
				fail(fmt.Sprintf("an @missing line this cannot read: %q", line))
			}
			defaults = append(defaults, span{lo, hi, value(strings.TrimSpace(fields[1]))})
			continue
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Split(line, ";")
		if len(fields) < 2 {
			continue
		}
		v := value(strings.TrimSpace(fields[1]))
		lo, hi, ok := parseRange(strings.TrimSpace(fields[0]))
		if !ok {
			continue
		}
		seen[v] = true
		for r := lo; r <= hi; r++ {
			stated[r] = true
		}
		if uprightValues[v] {
			upright = append(upright, span{lo, hi, v})
		}
	}
	if err := sc.Err(); err != nil {
		fail(err.Error())
	}
	// A value that has vanished from the file is a value renamed upstream, and
	// the characters it held would drop out of the table without a word.
	for _, values := range []map[string]bool{uprightValues, rotatedValues} {
		for v := range values {
			if !seen[v] {
				fail(fmt.Sprintf("no character has value %s; has it been renamed?", v))
			}
		}
	}
	if len(upright) == 0 {
		fail("no lines matched")
	}
	// The property has a default, and a file that states none is not the one
	// this reads: every code point the data is silent on would be nothing at
	// all, rather than rotated or upright.
	if len(defaults) == 0 {
		fail("no @missing line, so what the data does not list has no value")
	}
	// The defaults, later lines over earlier ones, minus everything the data
	// spoke about. Only an upright default contributes to the table.
	for r := rune(0); r <= 0x10FFFF; r++ {
		if stated[r] {
			continue
		}
		d := ""
		for _, m := range defaults {
			if m.lo <= r && r <= m.hi {
				d = m.class
			}
		}
		if !uprightValues[d] {
			continue
		}
		if n := len(upright); n > 0 && upright[n-1].class == "default" && upright[n-1].hi == r-1 {
			upright[n-1].hi = r
			continue
		}
		upright = append(upright, span{r, r, "default"})
	}

	var w strings.Builder
	fmt.Fprint(&w, `// Code generated by cmd/genvertical from Unicode's VerticalOrientation.txt.
// DO NOT EDIT.

package paragraph
`)
	emit(&w, "uprightRanges", upright, `// The characters that stand upright on a line of vertical text, UAX #50's
// values U and Tu. Unicode %s.
//
// %d ranges, merged from %d the file states: %s. The ideographs, the kana,
// the Hangul, the fullwidth forms and the symbols that are set square in East
// Asian text, and the unassigned and private use code points of their blocks,
// which the data lists explicitly. A "default" count would be code points the
// data is silent on and an @missing line calls upright — see cmd/genvertical.
//
// What reads it is a gate rather than a typesetting rule: this engine sets no
// upright text, and the table is how it knows to say so. See IsUpright.`, *version)
	fmt.Print(w.String())
}

func emit(w *strings.Builder, name string, spans []span, doc, version string) {
	counts := map[string]int{}
	for _, s := range spans {
		counts[s.class]++
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })
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

// value is a Vertical_Orientation value, refused if it is none of the four.
func value(v string) string {
	if !uprightValues[v] && !rotatedValues[v] {
		fail(fmt.Sprintf("%q is a Vertical_Orientation value neither list names; "+
			"decide whether it stands upright", v))
	}
	return v
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "genvertical:", msg)
	os.Exit(1)
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
