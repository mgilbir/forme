package shape

import "testing"

// Shaping a run with the text either side of it.
//
// A cursive script chooses each letter's shape from its neighbours, and a run is
// not always a whole word: CSS Text §8.1 keeps shaping across an inline element
// boundary, so "ب<span>ب</span>ب" is one joined word set as three runs. Shaped a
// run at a time every letter comes out isolated, which for a reader of the
// script is the difference between a word and three letters standing apart.
//
// The face is the synthetic one arabic_test.go builds: one dual-joining letter
// with its four forms at four different advances, so what is chosen and what it
// measures are both exact and neither depends on a font being downloaded.
//
//	glyph 1  the isolated letter, 500 units
//	glyph 2  its initial form,    300
//	glyph 3  its medial form,     250
//	glyph 4  its final form,      400

// TestARunInContextTakesTheFormItsNeighboursCallFor.
func TestARunInContextTakesTheFormItsNeighboursCallFor(t *testing.T) {
	f := arabicFace(t)
	const b = string(rune(beh))
	for _, tc := range []struct {
		before, after string
		want          int
		form          string
	}{
		{"", "", 1, "isolated, which is a letter standing alone"},
		{"", b, 2, "initial, because a letter follows it"},
		{b, b, 3, "medial, because letters stand either side"},
		{b, "", 4, "final, because a letter precedes it"},
		// A letter that does not join to it leaves it as it was.
		{"x", "x", 1, "isolated between two characters that do not join"},
		{"", "x", 1, "isolated before one"},
	} {
		got, _ := f.ShapeGlyphsInContext(b, tc.before, tc.after)
		if len(got) != 1 {
			t.Fatalf("before=%q after=%q: %d glyphs", tc.before, tc.after, len(got))
		}
		if got[0].GID != tc.want {
			t.Errorf("before=%q after=%q: glyph %d, want %d — %s",
				tc.before, tc.after, got[0].GID, tc.want, tc.form)
		}
	}
}

// TestThreeRunsInContextAreTheWholeWord is the property stated as the suite
// states it: the pieces together must be what the one run is.
func TestThreeRunsInContextAreTheWholeWord(t *testing.T) {
	f := arabicFace(t)
	const b = string(rune(beh))
	whole, _ := f.ShapeGlyphs(b + b + b)
	if len(whole) != 3 {
		t.Fatalf("the whole word shaped to %d glyphs", len(whole))
	}
	// Right-to-left, so the whole run comes back reversed: the last letter is
	// drawn first. The pieces are shaped one at a time, in logical order.
	want := []int{whole[2].GID, whole[1].GID, whole[0].GID}
	got := []int{
		first(t, f, b, "", b+b),
		first(t, f, b, b, b),
		first(t, f, b, b+b, ""),
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("piece %d is glyph %d, want %d; the three runs must be the same "+
				"three letters in the same three forms as the one run %v",
				i, got[i], want[i], gids(whole))
		}
	}
}

func first(t *testing.T, f *Face, s, before, after string) int {
	t.Helper()
	g, _ := f.ShapeGlyphsInContext(s, before, after)
	if len(g) != 1 {
		t.Fatalf("%q in %q/%q shaped to %d glyphs", s, before, after, len(g))
	}
	return g[0].GID
}

// TestContextSurvivesTheDirectionOverride.
//
// A backend is handed a right-to-left run as an override character followed by
// the text — see layout's ShapedText — so the text is never the first thing in
// the string being shaped. The first version of this took a sub-run's context
// from the string around it and *replaced* the caller's, so the override alone
// stood in for the word the letters were supposed to join to. Every Arabic run
// in every document went through that path, and the whole feature did nothing.
func TestContextSurvivesTheDirectionOverride(t *testing.T) {
	f := arabicFace(t)
	const b, rlo = string(rune(beh)), "‮"
	if got, want := first(t, f, rlo+b, b, b), first(t, f, b, b, b); got != want {
		t.Errorf("with the override the letter is glyph %d and without it %d; the "+
			"override is transparent to joining and changes no form", got, want)
	}
}

// TestNoContextIsTheOldAnswer. Every run of every document goes through this
// path, and one that gives no context may not come out differently than before.
func TestNoContextIsTheOldAnswer(t *testing.T) {
	f := arabicFace(t)
	const b = string(rune(beh))
	for _, s := range []string{b, b + b, b + b + b, "x", "", " "} {
		want, wm := f.ShapeGlyphs(s)
		got, gm := f.ShapeGlyphsInContext(s, "", "")
		if len(got) != len(want) || gm != wm {
			t.Fatalf("%q: %d glyphs and %d missing against %d and %d",
				s, len(got), gm, len(want), wm)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q: glyph %d differs with no context given", s, i)
			}
		}
	}
}

// TestContextCannotChangeAScriptThatDoesNotJoin is the containment half. The
// context is read only where a letter takes a positional form, so text in every
// other script must be shaped identically whatever is handed over — which is
// what makes this inert for all but a handful of the documents there are.
func TestContextCannotChangeAScriptThatDoesNotJoin(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	for _, s := range []string{"abc", "ΚΑΛΗ", "office"} {
		want, _ := f.ShapeGlyphs(s)
		for _, ctx := range []struct{ before, after string }{
			{"x", "y"}, {string(rune(beh)), string(rune(beh))}, {"", "z"}, {"z", ""},
		} {
			got, _ := f.ShapeGlyphsInContext(s, ctx.before, ctx.after)
			if len(got) != len(want) {
				t.Fatalf("%q with %q/%q: %d glyphs, want %d",
					s, ctx.before, ctx.after, len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("%q with %q/%q: glyph %d changed", s, ctx.before, ctx.after, i)
				}
			}
		}
	}
}

// TestMeasuringAgreesWithTheFormChosen. A joined letter is not as wide as an
// isolated one, so a run measured without its context and drawn with it is
// measured to one width and painted at another — the fault this package is
// arranged throughout to prevent.
//
// The advances are the synthetic face's and are all different, which is what
// lets this be an equality rather than an inequality.
func TestMeasuringAgreesWithTheFormChosen(t *testing.T) {
	f := arabicFace(t)
	const b = string(rune(beh))
	const size = 1000 // one unit per font unit, so the numbers are the face's
	for _, tc := range []struct {
		before, after string
		want          float64
	}{
		{"", "", 500},
		{"", b, 300},
		{b, b, 250},
		{b, "", 400},
	} {
		got := f.MeasureShapedInContext(b, size, tc.before, tc.after)
		if got != tc.want {
			t.Errorf("before=%q after=%q: measured %v, want %v", tc.before, tc.after, got, tc.want)
		}
		// And what is drawn occupies what was measured.
		if drawn := MeasureGlyphs(mustShape(t, f, b, tc.before, tc.after), size); drawn != got {
			t.Errorf("before=%q after=%q: the glyphs occupy %v and the measurement said %v",
				tc.before, tc.after, drawn, got)
		}
	}
}

func mustShape(t *testing.T, f *Face, s, before, after string) []Glyph {
	t.Helper()
	g, _ := f.ShapeGlyphsInContext(s, before, after)
	return g
}

// TestTheContextIsTrimmedToWhatIsAsked. The joining scan walks outward past the
// transparent characters and stops at the first that is not, so only that much
// of the context can change an answer: a caller handing over a paragraph must
// get the same result as one handing over a letter.
func TestTheContextIsTrimmedToWhatIsAsked(t *testing.T) {
	f := arabicFace(t)
	const b = string(rune(beh))
	short := first(t, f, b, b, b)
	if long := first(t, f, b, "a whole sentence before it "+b, b+" and one after"); long != short {
		t.Errorf("a long context gave glyph %d and a one-letter one %d", long, short)
	}
	// A mark between the letter and its neighbour is transparent and must be
	// looked past rather than stopped at.
	if marked := first(t, f, b, b+string(rune(fatha)), string(rune(fatha))+b); marked != short {
		t.Errorf("with a fatha either side the letter is glyph %d, want %d", marked, short)
	}
}
