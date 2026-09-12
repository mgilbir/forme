package layout

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/style"
)

// One text sets one page, however inline boxes cut it.
//
// CSS Text §8.1: the boundary between two inline elements does not break
// shaping. A <span> with nothing declared on it has no margin, no border, no
// padding and the same font as what it is inside, so it puts nothing on the
// page and takes nothing off it. The text it contains breaks where the same text
// written plainly breaks, and each line holds the same characters at the same
// width.
//
// That is a stronger claim than FuzzRunTiling's and it is the same claim. That
// target lays everything out under "white-space: nowrap" and compares the total
// width, which is deliberate — a line count that differs makes a total
// meaningless, and removing the question is what let it state the arithmetic
// cleanly. The cost is that nothing it does can reach a *line edge*: no
// wrapping, so no trimming, no hanging, no last line. This is the other half.
//
// # What it compares, and the two things left out of that on purpose
//
// Per line: the visible text, and the summed width of the runs.
//
// **The bidi controls are dropped from the text.** What is compared is what a
// reader sees, and which side of an invisible character a line ends on is not
// that. Keeping them found only that "a ‭ b" ends its first line before the
// control and "<span>a </span><span>‭ b</span>" after it, which is the same page.
//
// **The width is compared as well as the text, and it is the half that earns
// its place.** A space that was removed at a line edge and a space that was kept
// read the same in the text; only the number says which happened. The defect
// this target was written for is exactly that: a bidi control at the end of a
// line stopped §4.1.1 removing the space in front of it, so the first line of
// "a &#x202D; b" was a character wider in one spelling than the other and showed
// the same two letters either way.
//
// # Widths, and why five of them
//
// A defect that changes where a line breaks only shows at a width where the
// break matters, and which width that is depends on the text. So every case is
// laid out at five, from narrower than one character to wider than the seeds.
// The narrow end is where overflow-wrap and the last-resort rules live and the
// wide end is where the last line is the only line.
//
// # Two faces, for the reason FuzzRunTiling has two
//
// Courier does not join, kern or ligate, and a merge group that goes wrong there
// goes unnoticed in Noto Sans, which does all three. See the note on
// tilingFaces.
//
// # The four defects it has found
//
// Three within a minute of it existing, and all of them at a line edge, which is
// the half FuzzRunTiling cannot reach:
//
//   - a bidi control at the end of a line stopped §4.1.1 removing the space in
//     front of it, so a first line was a character wider in one spelling than
//     the other and showed the same letters either way;
//   - a control at the *start* of a line made the line look occupied, so the
//     rule that lets an overlong word overflow an empty line did not fire and
//     the document got an empty line taking a line's height;
//   - and the collapsing rule the first of those is the sibling of, which
//     FuzzRunTiling had already found from the other direction.
//
// The fourth came later and is the first that is a line *count*:
// "<span>0|-</span><span>!00</span>" set two lines where "0|-!00" sets three.
// The box left a taken break and a hold at one offset and could say only one of
// them. See paragraph.Trailing.

func FuzzBoundaryLines(f *testing.F) {
	for _, text := range tilingTexts {
		for _, cuts := range [][]byte{{1}, {2}, {1, 2}, {255}, {1, 1, 1, 1}} {
			f.Add(text, string(cuts))
		}
	}
	f.Fuzz(func(t *testing.T, text, cuts string) {
		checkBoundaryLines(t, text, cuts)
	})
}

// boundaryLineWidths are the widths every case is set at. See the note above.
var boundaryLineWidths = []float64{10, 16, 25, 40, 70}

// checkBoundaryLines lays the text out whole and cut and compares the lines.
func checkBoundaryLines(t testing.TB, text, cuts string) {
	if len(text) > 512 || len(cuts) > 64 {
		// Bounded for the reason every target here is: a long input finds the
		// memory it takes to hold one and no logic fault.
		return
	}
	if strings.ContainsAny(text, "<>&\x00") {
		// Markup this would have to escape, and the null — see the note on
		// checkRunTiling, which excludes it for what HTML parsing does to it.
		//
		// The forced-break characters are *not* excluded here, unlike there: a
		// line count that differs is the finding rather than a confounder, and a
		// break the author wrote has to fall in the same place either way.
		return
	}
	if !utf8.ValidString(text) {
		// Two documents holding the same text is the premise, and parsing is
		// what decides what that is. See checkRunTiling, where the reasoning and
		// the input that taught it are written out.
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

	cut := spanned(text, at)
	for _, family := range tilingFaces {
		sheet := `#d { font-family: ` + family + `; font-size: 16px }`
		for _, px := range boundaryLineWidths {
			whole, ok := linesOfSpanned(t, set, text, sheet, px)
			if !ok {
				continue
			}
			got, ok := linesOfSpanned(t, set, cut, sheet, px)
			if !ok {
				continue
			}
			if !sameBoundaryLines(whole, got) {
				t.Fatalf("in %s at %gpx, %q set\n  %v\nand the same text cut at "+
					"%v set\n  %v\n  %s\n§8.1's boundary does not break shaping, "+
					"and a span with nothing on it puts nothing on the page: the "+
					"lines hold the same characters at the same width.",
					family, px, text, whole, at, got, cut)
			}
		}
	}
}

// boundaryLine is one line's visible text and the width its runs take.
type boundaryLine struct {
	Text  string
	Width style.Unit
}

func (l boundaryLine) String() string { return fmt.Sprintf("%q@%v", l.Text, l.Width) }

func sameBoundaryLines(a, b []boundaryLine) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// linesOfSpanned lays markup out in a box of the given width and returns a line
// at a time.
//
// It reports false where the markup made no box at all, which is a document this
// invariant says nothing about rather than a failure: text that collapses away
// entirely produces none.
func linesOfSpanned(t testing.TB, set FontSet, markup, sheet string, px float64) ([]boundaryLine, bool) {
	built := Build(Input{HTML: `<div id="d">` + markup + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults + sheet}}})
	if built.Root == nil {
		return nil, false
	}
	w, _ := style.FromPx(px)
	// Tall enough that nothing is cut off the bottom: the claim is about the
	// lines the text sets, and a fragment that ran out of page would compare
	// two prefixes.
	h, _ := style.FromPx(1000000)
	frag := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
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
	if found == nil {
		return nil, false
	}
	out := make([]boundaryLine, 0, len(found.Lines))
	for _, line := range found.Lines {
		var one boundaryLine
		var b strings.Builder
		for _, r := range line.Runs {
			for _, c := range r.Text {
				// See the note above: an invisible character is not part of
				// what a reader sees, and which side of one a line ends on is
				// the same page either way.
				if !isBidiControl(c) {
					b.WriteRune(c)
				}
			}
			one.Width = one.Width.Add(r.Width)
		}
		one.Text = b.String()
		out = append(out, one)
	}
	return out, true
}
