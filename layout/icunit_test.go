package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// The "ic" unit, CSS Values §5.1.4: "equal to the used advance measure of the
// 水 (CJK water ideograph, U+6C34) glyph found in the font used to render it".
//
// It is what an author sizes a box in ideographs with — "width: 4ic" is a box
// four Han characters wide — and it is exact where "em" is only nearly right,
// because a CJK face's ideographs fill its em square and its Latin does not.
//
// It had been on the list of units this engine parses and declines to resolve,
// beside "cap" and "lh", which was one unit too many: unlike those it needs
// nothing the face layer does not already give, and the suite's own references
// are built on it. white-space-intrinsic-size-022 draws four ideographs in a
// "width: max-content" box and a "width: 4ic" box beside it, and asserts the
// two come out the same width — so an unresolved "ic" made the *reference*
// full-width and the test failed on a document this engine was getting right.

// icWidth lays out a box with a declared width and returns what it came to.
func icWidth(t *testing.T, css string, set FontSet) style.Unit {
	t.Helper()
	built := Build(Input{HTML: `<div id="d">x</div>`,
		CSS: []Stylesheet{{Source: `#d { ` + css + ` }`}}})
	if built.Root == nil {
		t.Fatal("no boxes")
	}
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
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
		t.Fatal("no fragment for the box")
	}
	return found.ContentRect().W
}

// TestIcIsOneEmInAFaceWithNoIdeograph is §5.1.4's own fallback, and it is the
// specified answer rather than a guess: "in the cases where it is impossible or
// impractical to determine the ideographic advance measure, it must be assumed
// to be 1em".
//
// It is also the case nearly every document reaches. A face is chosen from the
// declared family, and none of the fourteen standard faces has an ideograph in
// it, so "4ic" on a Latin element is four ems.
//
// The number that would be wrong is the one measuring the face's notdef box:
// Courier gives that 600/1000 of the em, so "4ic" would come out at 48px where
// the specification asks for 80. That is not a rounding difference — it is a box
// three fifths the size the author wrote.
func TestIcIsOneEmInAFaceWithNoIdeograph(t *testing.T) {
	set := StandardFonts()
	em := icWidth(t, `font-family: Courier; font-size: 20px; width: 4em`, set)
	ic := icWidth(t, `font-family: Courier; font-size: 20px; width: 4ic`, set)
	if want, _ := style.FromPx(80); em != want {
		t.Fatalf("4em came to %v, want %v; the fixture is not what it claims", em, want)
	}
	if ic != em {
		t.Errorf("4ic came to %v and 4em to %v; a face with no water ideograph has no "+
			"ideographic advance to state, and §5.1.4 says the answer is one em", ic, em)
	}
}

// oneFace is a FontSet with a single face in it, whatever family is asked for.
//
// A webfont will not do here: the only CJK face in the checkout is nine
// megabytes, and the engine refuses to parse a webfont larger than eight — a
// cap that is there for a reason and is not something a test may lift. So the
// face is loaded the way the reftest harness loads its fallbacks and handed
// straight to the box.
type oneFace struct{ f *shape.Face }

func (o oneFace) Face(string, bool, bool) (*shape.Face, bool) { return o.f, true }

// TestIcIsTheFacesOwnAdvanceWhenItHasTheIdeograph is the other branch, and it
// has to be asked about the *decision* rather than about the number.
//
// Every face that has a water ideograph makes it exactly one em wide — that is
// what a full-width character is — so for Noto Sans JP the two branches produce
// the same length, and a fixture comparing widths would pass with the face never
// consulted. What differs is whether the face was asked, which is what this
// reads: the advance is measured and reported as known, where Courier's is
// declined.
//
// The consequence of getting that wrong is in the test above, and it is why the
// decision matters even where the number does not: measuring a face that has no
// ideograph measures its notdef box, and Courier's is three fifths of the em.
func TestIcIsTheFacesOwnAdvanceWhenItHasTheIdeograph(t *testing.T) {
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face with ideographs")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSansJP-VF.ttf"))
	if err != nil {
		t.Skipf("reading the CJK face: %v", err)
	}
	jp, err := loadSuiteFace(data)
	if err != nil {
		t.Skipf("parsing the CJK face: %v", err)
	}
	if missesVisible(jp, "水") {
		t.Skip("the CJK face has no water ideograph, so it cannot answer this")
	}

	got, ok := icAdvanceWith(t, oneFace{jp})
	if !ok {
		t.Fatalf("the face was not asked for its ideographic advance, and it has the glyph")
	}
	// And what it answered is that glyph's advance, measured the way the text
	// would be. One em for this face, which is what makes the number alone
	// unable to tell the two branches apart.
	if want := newBreaker(nil).Measure(jp, "水", twentyPx(t)); got != want {
		t.Errorf("the ideographic advance is %v and the face measures 水 at %v", got, want)
	}

	// The control, through the same call: a Latin face declines, so the
	// resolver falls back to §5.1.4's one em rather than to a notdef box.
	if _, ok := icAdvanceWith(t, StandardFonts()); ok {
		t.Errorf("a Latin face answered with an ideographic advance; it has no " +
			"ideograph, and what would be measured is its notdef box")
	}
}

// icAdvanceWith asks one box's face for its ideographic advance.
func icAdvanceWith(t *testing.T, set FontSet) (style.Unit, bool) {
	t.Helper()
	built := Build(Input{HTML: `<div id="d">x</div>`,
		CSS: []Stylesheet{{Source: `#d { font-family: Courier; font-size: 20px }`}}})
	if built.Root == nil {
		t.Fatal("no boxes")
	}
	box := findBox(t, built.Root, "d")
	l := &layouter{fontSet: set, rec: NewRecorder(nil), fonts: map[fontKey]resolvedFont{}}
	l.br = newBreaker(l)
	return l.icAdvance(box)
}

func twentyPx(t *testing.T) style.Unit {
	t.Helper()
	u, ok := style.FromPx(20)
	if !ok {
		t.Fatal("20px is not representable")
	}
	return u
}

// TestTheOtherFaceUnitsStillAnswerAsTheyDid is the containment case for the
// refactor this needed. The three units are gathered by one walk now, so a
// mistake in it would change "ch" and "ex" as readily as "ic".
//
// Courier at 20px advances 12px a character, so "4ch" is 48px; and it states no
// x-height, so "4ex" is §5.1.2's half an em, 40px.
func TestTheOtherFaceUnitsStillAnswerAsTheyDid(t *testing.T) {
	set := StandardFonts()
	for _, tc := range []struct {
		value string
		want  float64
	}{
		{"4ch", 48},
		{"4em", 80},
		{"4ic", 80},
	} {
		got := icWidth(t, `font-family: Courier; font-size: 20px; width: `+tc.value, set)
		if want, _ := style.FromPx(tc.want); got != want {
			t.Errorf("width: %s came to %v, want %v", tc.value, got, want)
		}
	}

	// "ex" is the odd one and is asked against the face rather than against a
	// number written here, because Courier *does* state an x-height — it is not
	// §5.1.2's half-em fallback, and writing 40px would be asserting the
	// fallback while claiming to assert the unit.
	face, found := StandardFonts().Face("Courier", false, false)
	if !found {
		t.Fatal("no Courier")
	}
	d := face.Descriptor()
	if !d.Has(shape.MetricXHeight) || d.XHeight <= 0 {
		t.Skip("Courier states no x-height, so there is nothing to compare against")
	}
	want := twentyPx(t).Mul(float64(d.XHeight) / float64(face.UnitsPerEm())).Mul(4)
	if got := icWidth(t, `font-family: Courier; font-size: 20px; width: 4ex`, set); got != want {
		t.Errorf("width: 4ex came to %v and four times the face's x-height is %v",
			got, want)
	}
}

// TestAUnitIsOnlyFoundAfterADigit pins the cheap scan that decides whether a
// face is worth resolving at all.
//
// It is an over-approximation on purpose — a false positive costs one face
// lookup — but a false *negative* is silent and wrong: the unit would be
// resolved against no font, and "4ic" would come out as whatever a zero advance
// makes of it. The digit is what keeps a family name from matching, and
// "italic" is the name that would.
func TestAUnitIsOnlyFoundAfterADigit(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		a, b byte
		want bool
	}{
		{"4ic", 'i', 'c', true},
		{"0.125ic", 'i', 'c', true},
		{"4IC", 'i', 'c', true},
		{"1em 4ic", 'i', 'c', true},
		{"italic", 'i', 'c', false},
		{"4em", 'i', 'c', false},
		{"4ch", 'c', 'h', true},
		{"inherit", 'c', 'h', false},
		{"4ex", 'e', 'x', true},
		{"4ic", 'e', 'x', false},
	} {
		if got := usesUnit(tc.raw, tc.a, tc.b); got != tc.want {
			t.Errorf("usesUnit(%q, %q, %q) = %v, want %v",
				tc.raw, tc.a, tc.b, got, tc.want)
		}
	}
}

// TestTwoFacesDoNotShareAnAnswer pins the memoization key, which is the one
// thing here that a document can get wrong in a way nothing else notices.
//
// "ch" resolves against the face, so two boxes at the same size in different
// fonts have different answers for the same declaration. Leaving the metrics
// out of the key would let the first font to parse "4ch" decide it for every
// other — a memoization bug, and those produce a wrong page only in a document
// that uses two faces, which is why it needs a test with two.
func TestTwoFacesDoNotShareAnAnswer(t *testing.T) {
	set := StandardFonts()
	courier := icWidth(t, `font-family: Courier; font-size: 20px; width: 4ch`, set)
	times := icWidth(t, `font-family: Times; font-size: 20px; width: 4ch`, set)
	if courier == times {
		t.Errorf("4ch is %v in Courier and %v in Times; the advance of \"0\" differs "+
			"between them, so the two must not share a cached answer", courier, times)
	}
	// Both in one document, so the two answers are computed by one layouter with
	// one cache — which is the shape the bug needs and the shape a page has.
	built := Build(Input{
		HTML: `<div id="a">x</div><div id="b">x</div>`,
		CSS: []Stylesheet{{Source: `
			div { font-size: 20px; width: 4ch }
			#a { font-family: Courier }
			#b { font-family: Times }`}},
	})
	if built.Root == nil {
		t.Fatal("no boxes")
	}
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	widths := map[string]style.Unit{}
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, ok := f.Box.Element.Attr("id"); ok {
				widths[id] = f.ContentRect().W
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(frag)
	if widths["a"] != courier || widths["b"] != times {
		t.Errorf("in one document 4ch came to %v and %v, and on their own to %v and "+
			"%v; one of the two took the other's cached answer",
			widths["a"], widths["b"], courier, times)
	}
}
