package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS 2.1 §5.12.1's ::first-line.
//
// It generates no box and inserts nothing: what it changes is the type the first
// formatted line is *set* in, which is why it has to reach the line breaking. A
// first line in twice the size holds half the words, and the second line begins
// where the first one ended.
//
// §5.12.1 makes it behave "like an inline-level element", so its properties are
// inherited by the line's content and a descendant that declared its own value
// keeps it. That is the half most easily got wrong, and the suite's own
// text-fit/grow-per-line reference is the fixture for it: a ::first-line rule
// that sets only a colour, over a paragraph whose first word is in a <span> with
// a font-size of its own.

// firstLines is the text, size and colour of each line of #d.
func firstLines(t *testing.T, cssSrc, htmlSrc string) []string {
	t.Helper()
	root := layoutOf(t, 600, htmlSrc, cssSrc)
	var out []string
	for _, ln := range find(t, root, "d").Lines {
		var parts []string
		size := 0.0
		for _, r := range ln.Runs {
			parts = append(parts, r.Text)
			size = r.Size.Px()
		}
		out = append(out, strings.Join(parts, "")+" @"+trimF(size))
	}
	return out
}

func trimF(v float64) string { return fmt.Sprintf("%g", v) }

const flCSS = `#d { font-family: Courier; font-size: 20px; width: 300px }`

// TestAFirstLineFontSizeChangesWhereTheLineBreaks is the bug, and the reason the
// pseudo-element cannot be a painting-time affair: at forty pixels a
// twelve-pixel-per-character face fits half as many words.
func TestAFirstLineFontSizeChangesWhereTheLineBreaks(t *testing.T) {
	got := firstLines(t, flCSS+` #d::first-line { font-size: 40px }`,
		`<div id="d">hello there world again</div>`)
	want := []string{"hello there @40", "world again @20"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q", got, want)
	}
}

// TestOnlyTheFirstLineIsRestyled, which is the other half of the same fixture:
// everything after it is set as the block says.
func TestOnlyTheFirstLineIsRestyled(t *testing.T) {
	root := layoutOf(t, 600, `<div id="d">aaaa bbbb cccc dddd</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 150px }
		#d::first-line { letter-spacing: 10px }`)
	lines := find(t, root, "d").Lines
	if len(lines) < 2 {
		t.Fatalf("%d lines, want at least two", len(lines))
	}
	// Courier at 20px is 12px a character, so "aaaa" is 48px on its own and 88
	// with ten pixels after each of the four.
	widthOf := func(ln LineFragment) float64 {
		for _, r := range ln.Runs {
			if r.Text == "aaaa" || r.Text == "bbbb" {
				return r.Width.Px()
			}
		}
		return 0
	}
	if got := widthOf(lines[0]); got != 88 {
		t.Errorf("the first line's word is %gpx wide, want 88 (4 x 12 plus 4 x 10)", got)
	}
	if got := widthOf(lines[1]); got != 48 {
		t.Errorf("the second line's word is %gpx wide, want 48 — nothing after the "+
			"first line is restyled", got)
	}
}

// TestADescendantThatDeclaredItsOwnValueKeepsIt. §5.12.1's pseudo-element is an
// ancestor of the line's content, not a replacement for it — so a <span> with a
// font-size of its own is set in that size on the first line like anywhere else.
func TestADescendantThatDeclaredItsOwnValueKeepsIt(t *testing.T) {
	root := layoutOf(t, 600,
		`<div id="d"><span id="s">aaaa</span> bbbb cccc</div>`,
		flCSS+` #d::first-line { font-size: 40px } #s { font-size: 30px }`)
	var sizes []float64
	for _, r := range find(t, root, "d").Lines[0].Runs {
		sizes = append(sizes, r.Size.Px())
	}
	if len(sizes) == 0 || sizes[0] != 30 {
		t.Errorf("the span's run is at %v, want 30 — it declared its own size and "+
			"the pseudo-element is its ancestor", sizes)
	}
	if len(sizes) < 2 || sizes[len(sizes)-1] != 40 {
		t.Errorf("the text outside the span is at %v, want 40 — it has no size of "+
			"its own, so the pseudo-element's reaches it", sizes)
	}
}

// TestTheFirstLineColourIsTheFirstLinesOnly.
func TestTheFirstLineColourIsTheFirstLinesOnly(t *testing.T) {
	root := layoutOf(t, 600, `<div id="d">aaaa bbbb cccc dddd eeee ffff gggg</div>`,
		flCSS+` #d { color: black } #d::first-line { color: lime }`)
	lines := find(t, root, "d").Lines
	if len(lines) < 2 {
		t.Fatalf("one line only")
	}
	colourOf := func(ln LineFragment) string {
		for _, r := range ln.Runs {
			if strings.TrimSpace(r.Text) == "" {
				continue
			}
			return r.Box.Style["color"]
		}
		return ""
	}
	if got := colourOf(lines[0]); got != "lime" {
		t.Errorf("the first line's colour is %q, want lime", got)
	}
	if got := colourOf(lines[1]); got != "black" {
		t.Errorf("the second line's colour is %q, want black", got)
	}
}

// TestAPropertyCSSExcludesIsSilent. §5.12.1 names what may apply; "margin" is not
// on the list, so a declaration of it on a ::first-line is not a dropped
// declaration — it is one CSS says has no meaning, and there is nothing to tell
// an author about it.
func TestAPropertyCSSExcludesIsSilent(t *testing.T) {
	rec := NewRecorder(nil)
	built := Build(Input{
		HTML: `<div id="d">hello there</div>`,
		CSS:  []Stylesheet{{Source: flCSS + ` #d::first-line { margin: 10px; width: 5px }`}},
	})
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(1000)
	Layout(built.Root, Size{W: w, H: h}, nil, rec)
	for _, f := range rec.Findings() {
		t.Errorf("something was reported: %q", f.Message)
	}
}

// TestAPropertyCSSIncludesAndThisEngineDropsIsReported, which is the other half:
// an author who writes a ::first-line background will not see it, and has no
// other way to find out.
func TestAPropertyCSSIncludesAndThisEngineDropsIsReported(t *testing.T) {
	rec := NewRecorder(nil)
	built := Build(Input{
		HTML: `<div id="d">hello there</div>`,
		CSS:  []Stylesheet{{Source: flCSS + ` #d::first-line { background: orange }`}},
	})
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(1000)
	Layout(built.Root, Size{W: w, H: h}, nil, rec)
	found := false
	for _, f := range rec.Findings() {
		if f.Property == "background-color" {
			found = true
			if !f.Unsupported() {
				t.Errorf("the finding is not marked unsupported: %q", f.Message)
			}
		}
	}
	if !found {
		t.Errorf("the ::first-line background was dropped without a word")
	}
}

// TestAFirstLineThatSaysNothingCostsNothing is the containment argument. The
// rest of this file builds a second item stream for the whole paragraph, and a
// rule that changes nothing about how the line is measured must not reach it.
func TestAFirstLineThatSaysNothingCostsNothing(t *testing.T) {
	// The same values the block already has.
	got := firstLines(t, flCSS+` #d::first-line { font-size: 20px; color: black }`,
		`<div id="d">hello there world again</div>`)
	plain := firstLines(t, flCSS, `<div id="d">hello there world again</div>`)
	if strings.Join(got, "|") != strings.Join(plain, "|") {
		t.Errorf("the lines are %q, want %q", got, plain)
	}
}

// TestNoFirstLineRuleIsUnchanged, which is every document that does not write
// one.
func TestNoFirstLineRuleIsUnchanged(t *testing.T) {
	got := firstLines(t, flCSS, `<div id="d">hello there world again</div>`)
	if len(got) != 1 || !strings.HasSuffix(got[0], "@20") {
		t.Errorf("the lines are %q, want one at 20px", got)
	}
}
