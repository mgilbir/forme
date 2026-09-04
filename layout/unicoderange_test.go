package layout

import (
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// unicode-range: a face used for the characters it was declared for, and passed
// over for the rest.
//
// This is what the descriptor is for and it is the ordinary way a page with a
// large script is served: one webfont for Latin, another for Greek or Cyrillic
// or CJK, both named in one font-family list. Without it the first family in the
// list sets everything, which is a page in a face the author excluded from it.
//
// The engine used to parse the descriptor, report it and throw it away, on the
// reasoning that one face was chosen per box. That stopped being true when runs
// were cut per grapheme cluster for a different reason — see facerun.go — and
// this is what the machinery was already able to do.

// twoRangedFamilies is a document naming two webfonts with disjoint ranges, which
// is the shape the CSS Working Group's own tests use. The two files are real
// faces with different metrics, so which one set a character is visible in the
// run's width as well as in its face.
func twoRangedFamilies(t *testing.T, text string) []faceRun {
	t.Helper()
	res := &fileResolver{files: map[string][]byte{
		"a.ttf": realFont(),
		"b.ttf": realFont(),
	}}
	built := Build(Input{
		HTML: `<div id="d">` + text + `</div>`,
		CSS: []Stylesheet{{Source: `
			@font-face { font-family: OnlyA; src: url(a.ttf); unicode-range: U+0061 }
			@font-face { font-family: OnlyB; src: url(b.ttf); unicode-range: U+0062 }
			#d { font-family: OnlyA, OnlyB }`}},
		Resources: res,
	})
	if built.Root == nil {
		t.Fatalf("no boxes; findings: %v", built.Findings)
	}
	box := findBox(t, built.Root, "d")
	l := &layouter{fontSet: built.Fonts, rec: NewRecorder(nil), fonts: map[fontKey]resolvedFont{}}
	primary, _ := l.fontFor(box)
	return l.faceRunsFor(box, primary, text)
}

// TestEachFamilySetsTheCharactersItWasDeclaredFor is the property, and the
// fixture is the one the suite writes: two families whose ranges do not overlap.
func TestEachFamilySetsTheCharactersItWasDeclaredFor(t *testing.T) {
	runs := twoRangedFamilies(t, "ab")
	if len(runs) != 2 {
		t.Fatalf("%d runs for ab, want one per family: %v", len(runs), runsText(runs))
	}
	if runs[0].Text != "a" || runs[1].Text != "b" {
		t.Errorf("the text was cut as %v, want a then b", runsText(runs))
	}
	if runs[0].Face == runs[1].Face {
		t.Errorf("both characters were set in one face; each family declared a " +
			"unicode-range holding exactly one of them")
	}
}

// TestAFamilyWithNoRangeStillSetsEverything is the containment case. The walk
// added here runs per cluster, and it must not change a document that declares
// no unicode-range at all — which is almost every document.
func TestAFamilyWithNoRangeStillSetsEverything(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"a.ttf": realFont()}}
	built := Build(Input{
		HTML: `<div id="d">abc</div>`,
		CSS: []Stylesheet{{Source: `
			@font-face { font-family: Plain; src: url(a.ttf) }
			#d { font-family: Plain }`}},
		Resources: res,
	})
	box := findBox(t, built.Root, "d")
	l := &layouter{fontSet: built.Fonts, rec: NewRecorder(nil), fonts: map[fontKey]resolvedFont{}}
	primary, _ := l.fontFor(box)
	runs := l.faceRunsFor(box, primary, "abc")
	if len(runs) != 1 || runs[0].Text != "abc" {
		t.Errorf("a family with no unicode-range cut its text into %v; it covers "+
			"everything and its text must arrive whole", runsText(runs))
	}
}

// TestACharacterNoNamedFamilyCoversFallsThrough. Both families exclude "c", so
// the document has named nothing for it — and what happens then is what happened
// before this existed: it stays with the primary face and is reported missing if
// that face cannot set it.
//
// The important half is that it does not vanish and does not silently take a
// webfont the author scoped away from it.
func TestACharacterNoNamedFamilyCoversFallsThrough(t *testing.T) {
	runs := twoRangedFamilies(t, "acb")
	var got string
	for _, r := range runs {
		got += r.Text
	}
	if got != "acb" {
		t.Errorf("the text came back as %q, want %q: no character may be dropped "+
			"by the family walk", got, "acb")
	}
}

// TestTheRestrictionIsCheckedPerCluster, not per box. A box whose text is wholly
// inside one family's range is set by that family; the point of the walk is that
// the answer can differ within one box, and a fixture with one character in it
// would pass whether or not that were true.
func TestTheRestrictionIsCheckedPerCluster(t *testing.T) {
	runs := twoRangedFamilies(t, "aab")
	if len(runs) != 2 {
		t.Fatalf("%d runs, want two: %v", len(runs), runsText(runs))
	}
	if runs[0].Text != "aa" {
		t.Errorf("the leading run is %q, want aa — adjacent clusters that "+
			"chose the same face are one run", runs[0].Text)
	}
}

// TestAFamilyListIsWalkedInOrder: the first family that covers a character wins,
// which is the cascade's own rule for a font-family list.
func TestAFamilyListIsWalkedInOrder(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{
		"a.ttf": realFont(), "b.ttf": realFont(),
	}}
	built := Build(Input{
		HTML: `<div id="d">a</div>`,
		CSS: []Stylesheet{{Source: `
			@font-face { font-family: First; src: url(a.ttf); unicode-range: U+0061 }
			@font-face { font-family: Second; src: url(b.ttf); unicode-range: U+0061 }
			#d { font-family: First, Second }`}},
		Resources: res,
	})
	set, ok := built.Fonts.(*documentFonts)
	if !ok {
		t.Fatalf("the document's fonts are %T", built.Fonts)
	}
	var first, second *shape.Face
	for _, f := range set.faces {
		switch f.rule.family {
		case "First":
			first = f.face
		case "Second":
			second = f.face
		}
	}
	if first == nil || second == nil {
		t.Fatalf("both families should have loaded: %v", built.Findings)
	}
	box := findBox(t, built.Root, "d")
	l := &layouter{fontSet: built.Fonts, rec: NewRecorder(nil), fonts: map[fontKey]resolvedFont{}}
	primary, _ := l.fontFor(box)
	runs := l.faceRunsFor(box, primary, "a")
	if len(runs) != 1 {
		t.Fatalf("%d runs for one character", len(runs))
	}
	if runs[0].Face != first {
		t.Errorf("the second family set a character the first also covers; a " +
			"font-family list is walked in order")
	}
}

func runsText(runs []faceRun) []string {
	out := make([]string, 0, len(runs))
	for _, r := range runs {
		out = append(out, r.Text)
	}
	return out
}

// TestASecondNamedFamilyIsNotASubstitution.
//
// This is the finding half, and it is where honouring the descriptor first went
// wrong. A document naming two webfonts with disjoint ranges uses its *second*
// family for half its text. From inside faceRunsFor that looks exactly like a
// substitution — no run kept the primary face — and the report said "no face for
// 'high-a-only, deep-b-only' could set any of this text", which is untrue twice
// over: a face one of those families provided set all of it, and the author got
// precisely what they asked for.
//
// Two of the suite's reftests moved from clean to tainted on that finding alone
// before it was distinguished.
func TestASecondNamedFamilyIsNotASubstitution(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"a.ttf": realFont(), "b.ttf": realFont()}}
	built := Build(Input{
		HTML: `<div id="d">bb</div>`,
		CSS: []Stylesheet{{Source: `
			@font-face { font-family: OnlyA; src: url(a.ttf); unicode-range: U+0061 }
			@font-face { font-family: OnlyB; src: url(b.ttf); unicode-range: U+0062 }
			#d { font-family: OnlyA, OnlyB }`}},
		Resources: res,
	})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	for _, f := range rec.Findings() {
		if aboutTheFace(f) {
			t.Errorf("using the second family the document named was reported as a "+
				"substitution: %s", f.Message)
		}
	}
}

// The other half — that a *real* substitution is still reported — is
// TestAFamilyThatSetsNothingIsReported in facerun_test.go, which was written
// before any of this and passes unchanged. That is the better evidence than a
// second fixture here would be: the distinction added above had to leave an
// existing test alone, and it did.
