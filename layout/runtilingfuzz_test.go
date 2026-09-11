package layout

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/style"
)

// One word is one width, however inline boxes cut it into runs.
//
// CSS Text §8.1: the boundary between two inline elements does not break
// shaping. This engine honours it for the shaping — the runs of a word split by
// a <span> are shaped as one string — and the arithmetic has to follow: a run's
// width is a length on the page, quantized to a sixty-fourth of a pixel, and the
// sum of separately quantized parts is not the quantization of the whole. The
// merge group exists so that the runs of a group tile it exactly.
//
// That claim is made in four files and was held by one fixture: the word
// "letter", in Courier at 12px, cut six ways. A fixture covers the case somebody
// thought of, and the two defects this invariant is about were both found from
// the other end — a reftest, and a hand-written width.
//
// So it is stated here over every string the corpus reaches, cut wherever the
// fuzzer likes. What it asserts is the whole of the claim: the widths are equal,
// to the unit.
//
// # The five defects it found, all fixed
//
// Each surfaced here as a width — two runs quantized separately are a
// sixty-fourth of a pixel away from one — and every one of them turned out to be
// line breaking at the box boundary. They have regression tests of their own in
// boundarybreak_test.go.
//
//   - "0|!" is one unbreakable run and "<span>0|</span><span>!</span>" broke in
//     two: U+007C is class BA so a line may end after it, U+0021 is class EX so
//     a line may not begin with one, and nothing asked LB13 across the boundary.
//   - "中中、中" breaks after the comma and "<span>中中、</span><span>中</span>"
//     did not: a box that ran out of text while *holding* an opportunity dropped
//     it.
//   - "|!!" in three spans took a break that both exclamation marks refuse.
//   - "<span>|</span><span>!0</span>" took one the second box's *second*
//     character should have taken, which needed the scan to be handed the
//     boundary rather than told about it afterwards.
//   - "<span>0</span><span>ᦤ</span>" took none at all. That one is the other
//     direction: the opportunity is made by the second box's *first* character
//     — New Tai Lue has no dictionary here, so §5.1 falls back to every
//     typographic character unit — and an opportunity nothing before the box
//     knows about had nowhere to be reported.
//
// # Where the cuts may fall, and why that is not a convenience
//
// At grapheme cluster boundaries. A cut inside a cluster separates a combining
// mark from its base, and this engine chooses a face per cluster precisely
// because a mark positioned by a font that never saw its base lands at the
// origin — so the two documents would genuinely differ and the assertion would
// be wrong rather than the engine. facerun.go says so in as many words.
//
// # Why nowrap
//
// A line count that differs between the two documents makes the comparison
// meaningless, and "nowrap" removes the question rather than working around it:
// breaking is what it changes, and nothing about a run's width.
//
// # Why two faces, which is not belt and braces
//
// Every case is checked in the bundled Noto Sans *and* in Courier, and the
// second one earns its place: the defect this invariant was written for was a
// tiling **gated on whether the face's shaping could change** — joining forms,
// kerning, ligatures — so a face with none of those got no merge group and no
// tiling. Noto Sans has all three, so the gate never bites there.
//
// That was measured rather than assumed. The gate was planted back and fifty
// thousand executions over Noto Sans alone did not notice; the fixture in
// runsplitwidth_test.go, which is Courier, failed at once. A target that only
// exercises the face where a bug cannot appear is a target that holds nothing.

func FuzzRunTiling(f *testing.F) {
	for _, text := range tilingTexts {
		for _, cuts := range [][]byte{{1}, {2}, {1, 2}, {255}, {1, 1, 1, 1}} {
			f.Add(text, string(cuts))
		}
	}
	f.Fuzz(func(t *testing.T, text, cuts string) {
		checkRunTiling(t, text, cuts)
	})
}

// tilingTexts are the shapes where the arithmetic has been wrong or could be:
// a word a boundary divides, a ligature, a kerned pair, a cursive script whose
// letters change form, a cluster with a mark on it, white space, and the two
// directions in one line.
var tilingTexts = []string{
	"letter", "office", "AVATAR", "", "To.", "0|!", "|!!", "|!0", "x|y", "0ᦤ",
	"hello world", "a b c d", "one  two", "AA )BB", "中中、中", "\u3042\u3042 abc",
	"العربية", "ععع", "אבג",
	"देवनागरी", "क्षत्रिय", "e\u0301cole", "e\u0301\u0302x",
	"abc אבג def", "high\u00adway", "a\u200bb", "a\u200db",
	"12345", "ﬁreﬂy", "AVA To",
}

// checkRunTiling lays the text out whole and cut, and compares the widths.
func checkRunTiling(t testing.TB, text, cuts string) {
	if len(text) > 512 || len(cuts) > 64 {
		// Bounded for the reason every target here is: a long input finds the
		// memory it takes to hold one and no logic fault.
		return
	}
	if strings.ContainsAny(text, "<>&\n\r\f\v\u0085\u2028\u2029\x00") {
		// Markup this would have to escape; the characters that force a line
		// break, which would make the two documents different numbers of lines
		// and the comparison meaningless; and the null, for the reason below.
		return
	}
	if !utf8.ValidString(text) {
		// The invariant compares two documents holding the *same* text, and
		// parsing is what decides what that is. Bytes that are not valid UTF-8
		// are not the same text once they have been cut: the fuzzer found
		// "0\xe5\xb6\x00\x8a", which HTML parses whole as "0嶊" — the null is
		// dropped and the three bytes left are a character — and cut between
		// the second and third byte as "0\uFFFD" and "\uFFFD\uFFFD". Two
		// different strings, legitimately two different widths.
		//
		// So this is a precondition and not an exclusion of a hard case. What
		// happens to malformed bytes is html's business and is fuzzed there.
		return
	}
	at := cutPositions(text, cuts)
	if len(at) == 0 {
		return
	}

	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}

	for _, family := range tilingFaces {
		sheet := `#d { font-family: ` + family +
			`; font-size: 16px; white-space: nowrap }`
		whole, ok := tiledWidth(t, set, text, sheet)
		if !ok {
			continue
		}
		cut, ok := tiledWidth(t, set, spanned(text, at), sheet)
		if !ok {
			continue
		}
		if whole != cut {
			t.Fatalf("in %s, %q is %v wide whole and %v cut at %v:\n  %s\n"+
				"§8.1's boundary does not break shaping, so it may not change "+
				"the arithmetic either — the runs of a group tile it exactly.",
				family, text, whole, cut, at, spanned(text, at))
		}
	}
}

// tilingFaces are the two the claim has to hold in. "T" is the bundled Noto
// Sans, which joins, kerns and ligates; Courier is one of the standard fourteen
// and does none of the three, which is the case the tiling was once gated out
// of. See the note at the top.
var tilingFaces = []string{"T", "Courier"}

// tiledWidth is the summed width of the runs on the one line the markup makes.
//
// It reports false where there is not exactly one line, which is a document this
// invariant says nothing about rather than a failure: an empty box has no line,
// and "nowrap" leaves no other way to have more than one.
func tiledWidth(t testing.TB, set FontSet, markup, sheet string) (style.Unit, bool) {
	built := Build(Input{HTML: `<div id="d">` + markup + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults + sheet}}})
	if built.Root == nil {
		return 0, false
	}
	w, _ := style.FromPx(100000)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	if frag == nil {
		return 0, false
	}
	var found *Fragment
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if found != nil || f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "d" {
				found = f
				return
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(frag)
	if found == nil || len(found.Lines) != 1 {
		return 0, false
	}
	var out style.Unit
	for _, r := range found.Lines[0].Runs {
		out = out.Add(r.Width)
	}
	return out, true
}

// cutPositions turns the fuzzer's bytes into grapheme cluster boundaries, in
// order and without repeats.
//
// Cluster boundaries and not byte offsets: see the note at the top. An empty
// answer means there is nowhere to cut, which is every string of one cluster.
func cutPositions(text, cuts string) []int {
	bounds := segment.Boundaries(nil, text)
	if len(bounds) == 0 || len(cuts) == 0 {
		return nil
	}
	seen := map[int]bool{}
	var out []int
	for _, b := range []byte(cuts) {
		at := bounds[int(b)%len(bounds)]
		if !seen[at] {
			seen[at] = true
			out = append(out, at)
		}
	}
	sortInts(out)
	return out
}

// spanned wraps the stretches between the cuts in bare <span>s.
//
// Bare, and that is the whole fixture: a span with no declaration on it inherits
// everything, so §8.1's boundary is the only thing it introduces. A span that
// changed the colour or the size would change the page for a reason this says
// nothing about — see sharesGlyphsWith, which refuses to share a glyph across
// either.
func spanned(text string, at []int) string {
	var b strings.Builder
	prev := 0
	for _, cut := range at {
		fmt.Fprintf(&b, "<span>%s</span>", text[prev:cut])
		prev = cut
	}
	fmt.Fprintf(&b, "<span>%s</span>", text[prev:])
	return b.String()
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
