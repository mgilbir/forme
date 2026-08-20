package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Shaping across an inline element boundary, CSS Text §8.1.
//
// "ع<span>ع</span>ع" is one Arabic word written as three text nodes, and the
// specification says the boundary between two inline elements does not break
// shaping. Shaped a run at a time, every letter comes out in its isolated form:
// three letters standing apart where a reader of the script expects one joined
// word. The suite has twenty-three tests of it, and a <b>, an <em> or a coloured
// <span> inside a word is ordinary markup rather than a corner.
//
// The face has to be one that really joins, so these need the fallback fonts.
// What is asserted is never a glyph number: it is that the split word draws the
// same marks as the whole one, and that the cases §8.1 says must *not* join do
// not — which is a claim about this engine rather than about Noto.

// arabicDoc lays out one document in a face that joins and returns the glyph
// ids drawn, in order.
func arabicDoc(t *testing.T, markup, css string) []int {
	t.Helper()
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face that joins")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSansArabic-Regular.ttf"))
	if err != nil {
		t.Skip("no Arabic face: ", err)
	}
	res := &fileResolver{files: map[string][]byte{"ar.ttf": data}}
	ops := paintWith(t, res, `<div id="d" dir="rtl">`+markup+`</div>`,
		`@font-face { font-family: Joins; src: url(ar.ttf) }
		 #d { font-family: Joins; font-size: 40px } `+css)
	var out []int
	for _, op := range ops {
		v, ok := op.(DrawText)
		if !ok {
			continue
		}
		glyphs, _ := ShapedGlyphs(v)
		for _, g := range glyphs {
			out = append(out, g.GID)
		}
	}
	return out
}

// sameGlyphs compares two glyph sequences as multisets: the split word draws its
// letters in three calls and the whole word in one, so the *order* the display
// list happens to put them in is not the claim.
func sameGlyphs(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	count := map[int]int{}
	for _, g := range a {
		count[g]++
	}
	for _, g := range b {
		count[g]--
		if count[g] < 0 {
			return false
		}
	}
	return true
}

const ain = "ع" // dual-joining, so it takes all four forms

// TestAWordSplitByAnInlineElementStillJoins is the feature, over every wrapper
// the suite names.
func TestAWordSplitByAnInlineElementStillJoins(t *testing.T) {
	whole := arabicDoc(t, ain+ain+ain, "")
	if len(whole) != 3 {
		t.Fatalf("the whole word drew %d glyphs", len(whole))
	}
	for _, tc := range []struct{ markup, css, what string }{
		{ain + "<span>" + ain + "</span>" + ain, "", "a bare span"},
		{ain + "<span class=x>" + ain + "</span>" + ain,
			".x { color: red }", "a colour"},
		{ain + "<b>" + ain + "</b>" + ain, "", "a <b>, with only a regular face to set it in"},
		{ain + "<em>" + ain + "</em>" + ain, "", "an <em>, likewise"},
		{ain + "<span class=x>" + ain + "</span>" + ain,
			".x { margin: 0; padding: 0; border: 0 }", "margin, padding and border at zero"},
		{ain + "<span class=x>" + ain + "</span>" + ain,
			".x { font-size: 100% }", "a font-size that changes nothing"},
		{ain + "<span class=x>" + ain + "</span>" + ain,
			".x { text-decoration: underline }", "a decoration"},
		{ain + "<span class=x>" + ain + "</span>" + ain,
			".x { outline: 1px solid blue }", "an outline, which takes no room in the flow"},
		// Nested, and split at both ends of the middle letter.
		{"<span>" + ain + "</span><span>" + ain + "</span><span>" + ain + "</span>", "",
			"three spans"},
	} {
		got := arabicDoc(t, tc.markup, tc.css)
		if !sameGlyphs(got, whole) {
			t.Errorf("%s: drew %v, want the same letters as the unsplit word %v",
				tc.what, got, whole)
		}
	}
}

// TestAWordSplitByRoomDoesNotJoin is the other half, and the half a rule written
// a little too wide would destroy: §8.1 breaks shaping where the two sides are
// not adjacent, and the suite sets four things that separate them.
func TestAWordSplitByRoomDoesNotJoin(t *testing.T) {
	whole := arabicDoc(t, ain+ain+ain, "")
	apart := arabicDoc(t, ain+"<span class=x>"+ain+"</span>"+ain,
		".x { unicode-bidi: isolate }")
	if sameGlyphs(apart, whole) {
		t.Fatalf("an isolate drew the joined word; this fixture cannot tell the two "+
			"apart and the rows below prove nothing: %v", apart)
	}
	for _, tc := range []struct{ css, what string }{
		{".x { margin: 0 10px }", "a margin"},
		{".x { padding: 0 10px }", "padding"},
		{".x { border: 10px solid blue }", "a border"},
		{".x { font-size: 120% }", "a larger font size"},
	} {
		got := arabicDoc(t, ain+"<span class=x>"+ain+"</span>"+ain, tc.css)
		if sameGlyphs(got, whole) {
			t.Errorf("%s: the letters joined across it; there is room between them",
				tc.what)
		}
	}
}

// TestAnIsolationBoundaryBreaksShaping.
//
// A <bdi>, a dir="auto" and unicode-bidi: isolate all raise the embedding level
// of what is inside them, and two runs at different levels are on opposite sides
// of a directional boundary — their characters are not adjacent in the reordered
// text and need not be adjacent on the page. The suite's shaping-012 and -013
// say it plainly: "Test passes if the three Arabic characters DON'T join".
func TestAnIsolationBoundaryBreaksShaping(t *testing.T) {
	whole := arabicDoc(t, ain+ain+ain, "")
	for _, tc := range []struct{ markup, css, what string }{
		{ain + "<bdi>" + ain + "</bdi>" + ain, "", "a <bdi>"},
		{ain + `<span dir="auto">` + ain + "</span>" + ain, "", `a dir="auto"`},
		{ain + "<span class=x>" + ain + "</span>" + ain,
			".x { unicode-bidi: isolate }", "unicode-bidi: isolate"},
	} {
		if got := arabicDoc(t, tc.markup, tc.css); sameGlyphs(got, whole) {
			t.Errorf("%s: the letters joined across an isolation boundary", tc.what)
		}
	}
}

// TestThePaddingThatIsBetweenThemIsTheOneThatCounts.
//
// Which physical side of an inline box faces a boundary depends on which way the
// text runs, not on which end of the box the edge is. In right-to-left text the
// earlier word is drawn to the right, so a box that opens between two runs
// presents its *right* edge to the boundary — its padding-right is between the
// words and its padding-left is on the far side.
//
// The suite states it in eight rows, boundary-shaping-009. Reading the width
// that §8.6 assigned to the inset item answers half of them and gets the other
// half backwards, and does so differently depending on the enclosing element's
// direction, which changes nothing about where the padding is drawn.
func TestThePaddingThatIsBetweenThemIsTheOneThatCounts(t *testing.T) {
	whole := arabicDoc(t, ain+ain+ain, "")
	for _, tc := range []struct {
		css   string
		joins bool
		what  string
	}{
		{".x { padding-right: 10px }", false, "a padding on the side facing the word before it"},
		{".x { padding-left: 10px }", true, "one on the far side"},
		{".x { margin-right: 10px }", false, "a margin on the near side"},
		{".x { margin-left: 10px }", true, "one on the far side"},
	} {
		got := arabicDoc(t, ain+"<span class=x>"+ain+ain+"</span>", tc.css)
		if sameGlyphs(got, whole) != tc.joins {
			verb := "did not join"
			if !tc.joins {
				verb = "joined"
			}
			t.Errorf("%s: the letters %s across it", tc.what, verb)
		}
	}
}

// TestAJoinedWordIsNarrowerThanLettersApart, which is why the context has to be
// settled before the lines are filled rather than at paint time: a line filled
// from widths measured without it is filled to the wrong widths, and the page
// then overflows or stops short with nothing in either measurement to say so.
func TestAJoinedWordIsNarrowerThanLettersApart(t *testing.T) {
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face that joins")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSansArabic-Regular.ttf"))
	if err != nil {
		t.Skip("no Arabic face: ", err)
	}
	width := func(markup, css string) float64 {
		t.Helper()
		res := &fileResolver{files: map[string][]byte{"ar.ttf": data}}
		built := Build(Input{
			HTML: `<div id="d" dir="rtl">` + markup + `</div>`,
			CSS: []Stylesheet{{Source: `@font-face { font-family: Joins; src: url(ar.ttf) }
				#d { font-family: Joins; font-size: 40px; width: max-content } ` + css}},
			Resources: res,
		})
		if built.Root == nil {
			t.Fatalf("no boxes: %v", built.Findings)
		}
		w, _ := style.FromPx(4000)
		h, _ := style.FromPx(10000)
		frag := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
		return find(t, frag, "d").ContentRect().W.Px()
	}
	whole := width(ain+ain+ain, "")
	split := width(ain+"<span>"+ain+"</span>"+ain, "")
	if split != whole {
		t.Errorf("the split word is %gpx wide and the whole one %gpx; the context "+
			"decides the letter and the letter decides the advance", split, whole)
	}
	// The control, so this cannot pass with every width the same: three letters
	// with room between them are wider than the word, and the room is part of it.
	apart := width(ain+"<span class=x>"+ain+"</span>"+ain, ".x { margin: 0 10px }")
	if apart <= whole {
		t.Errorf("letters set apart measure %gpx against the joined word's %gpx",
			apart, whole)
	}
}
