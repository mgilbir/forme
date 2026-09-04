package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/shape"
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
		// And one that changes something. A letter is medial because of the
		// letters beside it and not because of how large it is, and the suite
		// puts the two rows side by side: shaping-007 sets 100% and shaping-008
		// sets 120%, and *both* read "Test passes if the three Arabic
		// characters in each box join".
		{ain + "<span class=x>" + ain + "</span>" + ain,
			".x { font-size: 120% }", "a larger font size"},
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
	// Room, and only room. A font-size is not on this list — it changes what a
	// *pair* between two letters would measure and not which form either takes,
	// which is why sameShaping asks the two questions apart. See
	// TestABoundaryThatDoesBreakShapingLosesThePair for the other half.
	for _, tc := range []struct{ css, what string }{
		{".x { margin: 0 10px }", "a margin"},
		{".x { padding: 0 10px }", "padding"},
		{".x { border: 10px solid blue }", "a border"},
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

// A character that draws nothing does not break shaping, and the soft hyphen is
// the one a document writes inside a word.
//
// SplitAtBreaks keeps it — the mark is what says a line may end there, and
// "hyphens: none" has to be able to see it again — and the fallback puts it in
// whatever face has it, so it arrives here as an item of its own. An item is
// what the context is read from, so the run either side was given a context
// consisting of one invisible character and nothing else, and a joined Arabic
// word written with a "&shy;" in the middle came out as isolated letters.
//
// Asked of the three functions rather than of a document, because what a
// document produces depends on which face has the soft hyphen — and the rule is
// about an item that draws nothing, however it came to be one.
func TestTheShapingContextLooksThroughWhatDrawsNothing(t *testing.T) {
	face := joiningFace(t)
	items := []inlineItem{
		{Text: "\u062f\u0627\u0645\u064a", Face: face, Width: 100},
		{Text: "\u00ad", Face: face},
		{Text: "\u062f\u0649", Face: face, Width: 100},
	}
	if !drawsNothing(items[1]) {
		t.Fatal("a soft hyphen with no width was read as something that draws")
	}
	j, ok := shapingNeighbour(items, 2, -1)
	if !ok || j != 0 {
		t.Fatalf("the run before \u062f\u0649 is item %d (found=%v), want 0 — the "+
			"soft hyphen between them is not a run", j, ok)
	}
	// The characters it looked through are kept: a shaper reads them as
	// transparent, and a context with a hole in it is a different context.
	if got, want := textBetween(items, j, 2), "\u062f\u0627\u0645\u064a\u00ad"; got != want {
		t.Errorf("the context is %q, want %q", got, want)
	}
	j, ok = shapingNeighbour(items, 0, +1)
	if !ok || j != 2 {
		t.Fatalf("the run after the word is item %d (found=%v), want 2", j, ok)
	}
	if got, want := textBetween(items, 1, 3), "\u00ad\u062f\u0649"; got != want {
		t.Errorf("the context after is %q, want %q", got, want)
	}
}

// And a joiner with nothing beyond it is still the context.
//
// It is the other half of the same rule and the two pull opposite ways: looking
// *through* what draws nothing is what joins the word above, and a box holding
// nothing but a joiner and a letter has nothing on the far side to look through
// to. The suite's shaping-join-002 is that box — "&zwj;&#x0627;&zwj;" in a table
// cell, with the joiners in a different font — and dropping the context there
// puts the alef in its isolated form where the test asks for its final one.
func TestAJoinerWithNothingBeyondItIsStillTheContext(t *testing.T) {
	face := joiningFace(t)
	items := []inlineItem{
		{Text: "\u200d", Face: face},
		{Text: "\u0627", Face: face, Width: 100},
		{Text: "\u200d", Face: face},
	}
	for _, c := range []struct {
		step, want int
		what       string
	}{
		{-1, 0, "before"},
		{+1, 2, "after"},
	} {
		j, ok := shapingNeighbour(items, 1, c.step)
		if !ok || j != c.want {
			t.Errorf("the context %s the letter is item %d (found=%v), want %d — "+
				"the joiner was looked through and then thrown away",
				c.what, j, ok, c.want)
		}
	}
	// And the same where what stops the walk is not the end of the items but
	// something between them: a forced break, and an inline box's own padding.
	// Both end the context and neither takes the joiner with it.
	for _, stop := range []struct {
		item inlineItem
		what string
	}{
		{inlineItem{Forced: true}, "a forced break"},
		{inlineItem{Inset: true, InsetRight: 100, Width: 100}, "an inline box's padding"},
	} {
		with := []inlineItem{items[0], items[1], items[2], stop.item}
		if j, ok := shapingNeighbour(with, 1, +1); !ok || j != 2 {
			t.Errorf("with %s beyond it, the context after the letter is item %d "+
				"(found=%v), want 2 — the joiner is what is left", stop.what, j, ok)
		}
	}
}

// An invisible character that takes room is room between the letters, which is
// what §8.1 breaks shaping for. It is the same reading the insets above are
// given — a zero margin is a boundary and one with width is a gap — and it is
// why the width is asked as well as the characters.
func TestSomethingInvisibleThatTakesRoomStillBreaksShaping(t *testing.T) {
	face := joiningFace(t)
	items := []inlineItem{
		{Text: "\u062f\u0627\u0645\u064a", Face: face, Width: 100},
		{Text: "\u00ad", Face: face, Width: 5},
		{Text: "\u062f\u0649", Face: face, Width: 100},
	}
	if drawsNothing(items[1]) {
		t.Error("a character with an advance was read as drawing nothing")
	}
	// And a character that draws, whatever it measures. A run of no width is
	// not the same thing as a run of nothing — "font-size: 0" makes one out of
	// ordinary letters — and looking through it would give the letter beyond a
	// context it does not have.
	if drawsNothing(inlineItem{Text: "\u062f", Face: face}) {
		t.Error("a letter with no width was read as drawing nothing")
	}
	if j, ok := shapingNeighbour(items, 2, -1); j != 1 || !ok {
		t.Errorf("the context before is item %d (found=%v), want 1 — a run that "+
			"takes room is the neighbour, whatever it draws", j, ok)
	}
}

// joiningFace is a face whose letters change shape with their neighbours, which
// is what makes any of this observable.
func joiningFace(t *testing.T) *shape.Face {
	t.Helper()
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face that joins")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSansArabic-Regular.ttf"))
	if err != nil {
		t.Skip("no Arabic face: ", err)
	}
	face, err := shape.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if !face.HasJoiningForms() {
		t.Skip("the face here does not join")
	}
	return face
}
