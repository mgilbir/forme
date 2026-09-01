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
// # The unassigned ranges
//
// The file's data is explicit code points, and its *header* states that certain
// ranges of unassigned code points default to U rather than to the property's
// R. They are listed in defaultUpright below, copied from that header, because
// the file says so in prose and not in a field. Within those ranges an assigned
// character carries whatever value the data gives it; only the code points the
// data does not mention take the default.
//
// Reading the data alone would make an unassigned code point in the middle of
// the CJK blocks rotated, which is the unsafe direction and is the one shape of
// this mistake that no document would ever reveal.
//
// Usage:
//
//	genvertical VerticalOrientation.txt > paragraph/verticaltable.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// uprightValues are the Vertical_Orientation values that stand upright.
var uprightValues = map[string]bool{"U": true, "Tu": true}

// rotatedValues are the other two. Both lists are stated so that a value the
// file gains is an error here rather than a silent omission from one of them.
var rotatedValues = map[string]bool{"R": true, "Tr": true}

// defaultUpright are the ranges whose *unassigned* code points default to U,
// from the header of VerticalOrientation.txt. See the note above.
var defaultUpright = [...]struct {
	lo, hi rune
	what   string
}{
	{0x18B0, 0x18FF, "Canadian Syllabics Extended"},
	{0x2065, 0x2065, "Reserved Default_Ignorable_Code_Point"},
	{0x2150, 0x218F, "Number Forms"},
	{0x2400, 0x245F, "Control Pictures & OCR"},
	{0x2BB8, 0x2BFF, "Symbols"},
	{0x2E80, 0xA4CF, "CJK-Related & Yi"},
	{0xA960, 0xA97F, "Hangul Jamo Extended-A"},
	{0xAC00, 0xD7FF, "Hangul Syllables & Jamo Extended-B"},
	{0xE000, 0xFAFF, "PUA & CJK Compatibility Ideographs"},
	{0xFE10, 0xFE1F, "Vertical Forms"},
	{0xFE50, 0xFE6F, "Small Form Variants"},
	{0xFF00, 0xFFEF, "Halfwidth and Fullwidth Forms"},
	{0x1F200, 0x1F2FF, "Enclosed Ideographic Supplement"},
	{0x20000, 0x3FFFD, "CJK Unified Ideographs Extensions"},
	{0xF0000, 0x10FFFD, "Supplementary Private Use Areas"},
}

type span struct {
	lo, hi rune
	class  string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: genvertical <VerticalOrientation.txt>")
		os.Exit(2)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	version := "unknown"
	var upright []span
	// Every code point the data mentions, whatever its value: a default range
	// applies only where the file is silent.
	stated := map[rune]bool{}
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "# VerticalOrientation-") {
			version = strings.TrimSuffix(strings.TrimPrefix(line, "# VerticalOrientation-"), ".txt")
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Split(line, ";")
		if len(fields) < 2 {
			continue
		}
		value := strings.TrimSpace(fields[1])
		lo, hi, ok := parseRange(strings.TrimSpace(fields[0]))
		if !ok {
			continue
		}
		if !uprightValues[value] && !rotatedValues[value] {
			fmt.Fprintf(os.Stderr, "genvertical: %q is a Vertical_Orientation value "+
				"neither list names; decide whether it stands upright\n", value)
			os.Exit(1)
		}
		seen[value] = true
		for r := lo; r <= hi; r++ {
			stated[r] = true
		}
		if uprightValues[value] {
			upright = append(upright, span{lo, hi, value})
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// A value that has vanished from the file is a value renamed upstream, and
	// the characters it held would drop out of the table without a word.
	for value := range uprightValues {
		if !seen[value] {
			fmt.Fprintf(os.Stderr, "genvertical: no character has value %s; has it been renamed?\n", value)
			os.Exit(1)
		}
	}
	for value := range rotatedValues {
		if !seen[value] {
			fmt.Fprintf(os.Stderr, "genvertical: no character has value %s; has it been renamed?\n", value)
			os.Exit(1)
		}
	}
	if len(upright) == 0 {
		fmt.Fprintln(os.Stderr, "genvertical: no lines matched")
		os.Exit(1)
	}
	// The header's ranges, minus everything the data spoke about.
	for _, d := range defaultUpright {
		start := rune(-1)
		for r := d.lo; r <= d.hi+1; r++ {
			if r <= d.hi && !stated[r] {
				if start < 0 {
					start = r
				}
				continue
			}
			if start >= 0 {
				upright = append(upright, span{start, r - 1, "unassigned"})
				start = -1
			}
		}
	}

	var w strings.Builder
	fmt.Fprint(&w, `// Code generated by cmd/genvertical from Unicode's VerticalOrientation.txt.
// DO NOT EDIT.

package paragraph
`)
	emit(&w, "uprightRanges", upright, `// The characters that stand upright on a line of vertical text, UAX #50's
// values U and Tu. Unicode %s.
//
// %d ranges, merged from %d the file and its header state separately: %s.
// The ideographs, the kana, the Hangul, the fullwidth forms and the symbols
// that are set square in East Asian text. "unassigned" is the header's ranges
// of unassigned code points that default to U — see cmd/genvertical for why
// they are not in the data and why leaving them out would be the dangerous
// direction to be wrong in.
//
// What reads it is a gate rather than a typesetting rule: this engine sets no
// upright text, and the table is how it knows to say so. See IsUpright.`, version)
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
