package layout

import (
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// CSS Fonts 4 §5.2, face by face: the width, the style and the weight, in that
// order, and the unicode-range of the faces the three leave.

// face builds a matchable the way a rule does.
func face(width, weight valueRange, st faceStyle) matchable {
	return matchable{width: width, weight: weight, slope: st.offer()}
}

func one(v float64) valueRange { return valueRange{v, v} }

// chosen is which of the faces §5.2 keeps for a request, one expected.
func chosen(t *testing.T, faces []matchable, r FontRequest) int {
	t.Helper()
	keep, _ := matchFaces(faces, r)
	if len(keep) != 1 {
		t.Fatalf("%s: §5.2 kept %v, want one face", formatRequest(r), keep)
	}
	return keep[0]
}

// TestWidthIsMatchedFirstAndNarrowerFirst: §5.2's width search, and its
// place ahead of the style and the weight — a condensed request takes the
// condensed face whatever its weight, and at or below 100% a narrower face is
// preferred to a wider one however much nearer the wider one is.
func TestWidthIsMatchedFirstAndNarrowerFirst(t *testing.T) {
	normal := faceStyle{kind: styleNormal}
	faces := []matchable{
		face(one(75), one(700), normal),  // 0: condensed bold
		face(one(100), one(400), normal), // 1: normal
		face(one(125), one(400), normal), // 2: expanded
	}
	for _, tc := range []struct {
		width float64
		want  int
	}{
		{75, 0}, {100, 1}, {125, 2},
		{87.5, 0}, // at or below 100%: narrower first, 75 over 100
		{99, 0},
		{101, 2}, // above: wider first, 125 over 100
		{200, 2}, // nothing wider: the widest below
		{50, 0},  // nothing narrower: the narrowest above
	} {
		r := normalRequest
		r.Width = tc.width
		if got := chosen(t, faces, r); got != tc.want {
			t.Errorf("width %v chose face %d, want %d", tc.width, got, tc.want)
		}
	}
	// A variable face declaring a range offers every width in it.
	faces = []matchable{face(valueRange{62.5, 100}, one(400), normal), face(one(125), one(400), normal)}
	r := normalRequest
	r.Width = 80
	keep, m := matchFaces(faces, r)
	if len(keep) != 1 || keep[0] != 0 || m.width != 80 {
		t.Errorf("width 80 against 62.5..100 and 125 kept %v at %v, want the range at 80", keep, m.width)
	}
}

// TestStyleIsSearchedAsSection52Says: the style search for each kind of
// request over the families §2.4 describes — upright and italic, upright and
// oblique, italic and oblique, and a backslanted one.
func TestStyleIsSearchedAsSection52Says(t *testing.T) {
	normal := faceStyle{kind: styleNormal}
	italic := faceStyle{kind: styleItalic}
	oblique := func(lo, hi float64) faceStyle { return faceStyle{kind: styleOblique, angles: valueRange{lo, hi}} }
	req := func(slope FontSlope, angle float64) FontRequest {
		r := normalRequest
		r.Slope, r.Angle = slope, angle
		return r
	}
	w, n := one(100), one(400)
	for _, tc := range []struct {
		name   string
		family []faceStyle
		r      FontRequest
		want   int
		at     slopeMatch
	}{
		// Upright and italic.
		{"italic", []faceStyle{normal, italic}, req(SlopeItalic, 0), 1, slopeMatch{false, 1}},
		{"oblique 20", []faceStyle{normal, italic}, req(SlopeOblique, 20), 1, slopeMatch{false, 1}},
		{"oblique 5", []faceStyle{normal, italic}, req(SlopeOblique, 5), 1, slopeMatch{false, 1}},
		{"oblique 0", []faceStyle{normal, italic}, req(SlopeOblique, 0), 0, slopeMatch{true, 0}},
		{"oblique -10", []faceStyle{normal, italic}, req(SlopeOblique, -10), 0, slopeMatch{true, 0}},
		{"normal", []faceStyle{normal, italic}, req(SlopeNormal, 0), 0, slopeMatch{true, 0}},
		// Upright and a variable oblique.
		{"italic over an oblique range", []faceStyle{normal, oblique(10, 20)}, req(SlopeItalic, 0), 1, slopeMatch{true, 11}},
		{"oblique past the range", []faceStyle{normal, oblique(10, 20)}, req(SlopeOblique, 30), 1, slopeMatch{true, 20}},
		{"oblique short of it", []faceStyle{normal, oblique(10, 20)}, req(SlopeOblique, 5), 1, slopeMatch{true, 10}},
		{"oblique within it", []faceStyle{normal, oblique(10, 20)}, req(SlopeOblique, 14), 1, slopeMatch{true, 14}},
		// Italic and oblique: each takes its own, and normal the oblique,
		// whose angle is looked at before an italic value.
		{"italic of both", []faceStyle{italic, oblique(14, 14)}, req(SlopeItalic, 0), 0, slopeMatch{false, 1}},
		{"oblique of both", []faceStyle{italic, oblique(14, 14)}, req(SlopeOblique, 14), 1, slopeMatch{true, 14}},
		{"normal of both", []faceStyle{italic, oblique(14, 14)}, req(SlopeNormal, 0), 1, slopeMatch{true, 14}},
		// Backslanted: a negative request is the positive search mirrored.
		{"back within", []faceStyle{normal, oblique(-20, -10)}, req(SlopeOblique, -15), 1, slopeMatch{true, -15}},
		{"back short", []faceStyle{normal, oblique(-20, -10)}, req(SlopeOblique, -5), 1, slopeMatch{true, -10}},
		{"back past", []faceStyle{normal, oblique(-20, -10)}, req(SlopeOblique, -40), 1, slopeMatch{true, -20}},
		{"upright over back", []faceStyle{normal, oblique(-20, -10)}, req(SlopeNormal, 0), 0, slopeMatch{true, 0}},
		{"forward over back", []faceStyle{normal, oblique(-20, -10)}, req(SlopeOblique, 12), 0, slopeMatch{true, 0}},
		// Two obliques either side of a request: at or past 11 degrees the
		// steeper is looked at first, short of it the shallower; and the same
		// mirrored for a backslant.
		{"steep between", []faceStyle{oblique(5, 5), oblique(25, 25)}, req(SlopeOblique, 15), 1, slopeMatch{true, 25}},
		{"shallow between", []faceStyle{oblique(5, 5), oblique(25, 25)}, req(SlopeOblique, 8), 0, slopeMatch{true, 5}},
		{"back steep between", []faceStyle{oblique(-5, -5), oblique(-25, -25)}, req(SlopeOblique, -15), 1, slopeMatch{true, -25}},
		{"back shallow between", []faceStyle{oblique(-5, -5), oblique(-25, -25)}, req(SlopeOblique, -8), 0, slopeMatch{true, -5}},
		// An italic family only: normal takes it, there being nothing else.
		{"normal of italic", []faceStyle{italic}, req(SlopeNormal, 0), 0, slopeMatch{false, 1}},
	} {
		faces := make([]matchable, len(tc.family))
		for i, s := range tc.family {
			faces[i] = face(w, n, s)
		}
		keep, m := matchFaces(faces, tc.r)
		if len(keep) != 1 || keep[0] != tc.want || m.slope != tc.at {
			t.Errorf("%s: kept %v at %+v, want face %d at %+v", tc.name, keep, m.slope, tc.want, tc.at)
		}
	}
}

// TestStyleIsMatchedBeforeWeight: bold italic from an upright bold and an
// italic regular is the italic regular.
func TestStyleIsMatchedBeforeWeight(t *testing.T) {
	faces := []matchable{
		face(one(100), one(700), faceStyle{kind: styleNormal}),
		face(one(100), one(400), faceStyle{kind: styleItalic}),
	}
	r := normalRequest
	r.Weight, r.Slope = 700, SlopeItalic
	if got := chosen(t, faces, r); got != 1 {
		t.Errorf("bold italic chose face %d, want the italic", got)
	}
}

// TestAFamilyIsMatchedBeforeItsUnicodeRanges is §5.2 against a family whose
// bold face covers only Latin: bold Greek is not set in the regular face,
// which has the letters, but falls to the next family the document named — the
// style is chosen from the whole family, and only then are the chosen faces'
// ranges asked. It used to ask the ranges first.
func TestAFamilyIsMatchedBeforeItsUnicodeRanges(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{
		"regular.ttf": realFont(), "bold.ttf": realFont(), "condensed.ttf": realFont(),
	}}
	built := Build(Input{
		HTML: docWithFontFace(`
			@font-face { font-family: Trial; src: url(regular.ttf); }
			@font-face { font-family: Trial; src: url(bold.ttf); font-weight: 700;
				unicode-range: U+0-7F; }
			@font-face { font-family: Trial; src: url(condensed.ttf); font-stretch: 50% 80%; }`),
		Resources: res,
	})
	set, ok := built.Fonts.(*documentFonts)
	if !ok {
		t.Fatalf("the document's set is %T; findings: %v", built.Fonts, built.Findings)
	}
	refOf := func(face *shape.Face) string {
		for _, df := range set.faces {
			if df.face == face {
				return df.ref
			}
		}
		return "<unknown>"
	}
	bold := normalRequest
	bold.Weight = 700
	condensed := normalRequest
	condensed.Width = 60
	for _, tc := range []struct {
		text string
		r    FontRequest
		want string
	}{
		{"a", bold, "bold.ttf"},
		{"α", bold, ""},
		{"α", normalRequest, "regular.ttf"},
		{"a", normalRequest, "regular.ttf"},
		{"a", condensed, "condensed.ttf"},
		{"", bold, "bold.ttf"},
	} {
		face, ok := set.FaceForFamilyStyled("Trial", tc.text, tc.r)
		got := ""
		if ok {
			got = refOf(face)
		}
		if got != tc.want {
			t.Errorf("%q at %s chose %q, want %q", tc.text, formatRequest(tc.r), got, tc.want)
		}
	}
}

// recordingSet is a StyledFontSet that says what it was asked, and a plain
// one beside it that says which booleans.
type recordingSet struct {
	mu     sync.Mutex
	asked  []FontRequest
	flags  [][2]bool
	styled bool
	face   *shape.Face
}

func (s *recordingSet) Face(family string, bold, italic bool) (*shape.Face, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flags = append(s.flags, [2]bool{bold, italic})
	return s.face, true
}

type styledRecordingSet struct{ *recordingSet }

func (s styledRecordingSet) FaceStyled(family string, r FontRequest) (*shape.Face, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, r)
	return s.face, true
}

// TestLayoutAsksForTheNumbers: a set that takes a FontRequest is asked the
// computed weight, width and style; one that takes two booleans is asked what
// §5.2 would choose between a regular and a bold, an upright and an italic.
func TestLayoutAsksForTheNumbers(t *testing.T) {
	std, _ := StandardFonts().Face("serif", false, false)
	doc := `<p id="p">x</p>`
	sheet := `p { font-family: Trial; font-weight: 550; font-stretch: condensed; font-style: oblique 10deg }`
	run := func(set FontSet) {
		built := Build(Input{HTML: doc, CSS: []Stylesheet{{Source: sheet}}, Fonts: set})
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(600)
		Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
	}

	styled := styledRecordingSet{&recordingSet{face: std}}
	run(styled)
	want := FontRequest{Weight: 550, Width: 75, Slope: SlopeOblique, Angle: 10}
	found := false
	for _, r := range styled.asked {
		found = found || r == want
	}
	if !found {
		t.Errorf("a StyledFontSet was asked %v, never %v", styled.asked, want)
	}
	if len(styled.flags) != 0 {
		t.Errorf("a StyledFontSet was asked the booleans too: %v", styled.flags)
	}

	plain := &recordingSet{face: std}
	run(plain)
	found = false
	for _, f := range plain.flags {
		found = found || f == [2]bool{true, true}
	}
	if !found {
		t.Errorf("a plain set was asked %v; weight 550 and oblique 10deg are bold and italic", plain.flags)
	}
}

// TestFontStyleRightIsReported: "right" asks for an italic leaning left, and a
// face is chosen without knowing which way it leans.
func TestFontStyleRightIsReported(t *testing.T) {
	built := Build(Input{HTML: `<p style="font-style: right">x</p>`})
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(600)
	rec := NewRecorder(nil)
	Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	found := false
	for _, f := range rec.Findings() {
		if f.Rule == RuleUnsupportedValue && strings.Contains(f.Message, "right") {
			found = true
		}
	}
	if !found {
		t.Errorf("font-style: right was not reported: %v", rec.Findings())
	}
	// And "left", which is what an italic is, is not.
	built = Build(Input{HTML: `<p style="font-style: left">x</p>`})
	rec = NewRecorder(nil)
	Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	for _, f := range rec.Findings() {
		if f.Property == "font-style" {
			t.Errorf("font-style: left was reported: %v", f)
		}
	}
}

// TestABoxIsSetInTheFaceItsWidthChooses is the whole path, through the
// cascade, the font set and the per-box cache: two paragraphs that differ
// only in font-stretch are set in the two faces their widths choose, and two
// that differ only in font-style: oblique's angle in the two its angles do.
func TestABoxIsSetInTheFaceItsWidthChooses(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{
		"normal.ttf": realFont(), "condensed.ttf": realFont(),
		"back.ttf": realFont(),
	}}
	built := Build(Input{
		HTML: `<style>
			@font-face { font-family: Trial; src: url(normal.ttf); }
			@font-face { font-family: Trial; src: url(condensed.ttf); font-stretch: condensed; }
			@font-face { font-family: Trial; src: url(back.ttf); font-style: oblique -20deg -5deg; }
			p { font-family: Trial }
		</style><p id="a">x</p><p id="b" style="font-stretch: 80%">x</p>
		<p id="c" style="font-style: oblique 10deg">x</p><p id="d" style="font-style: oblique -10deg">x</p>`,
		Resources: res,
	})
	set := built.Fonts.(*documentFonts)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(2000)
	root := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
	refOf := func(id string) string {
		face := linesOf(t, root, id)[0].Runs[0].Face
		for _, df := range set.faces {
			if df.face == face {
				return df.ref
			}
		}
		return "<unknown>"
	}
	for id, want := range map[string]string{"a": "normal.ttf", "b": "condensed.ttf", "c": "normal.ttf", "d": "back.ttf"} {
		if got := refOf(id); got != want {
			t.Errorf("#%s is set in %s, want %s", id, got, want)
		}
	}
}

// formatRequest renders a request for a message.
func formatRequest(r FontRequest) string {
	var b strings.Builder
	b.WriteString("weight ")
	b.WriteString(strconv.FormatFloat(r.Weight, 'g', -1, 64))
	b.WriteString(", width ")
	b.WriteString(strconv.FormatFloat(r.Width, 'g', -1, 64))
	b.WriteString("%, ")
	switch r.Slope {
	case SlopeItalic:
		b.WriteString("italic")
	case SlopeOblique:
		b.WriteString("oblique ")
		b.WriteString(strconv.FormatFloat(r.Angle, 'g', -1, 64))
		b.WriteString("deg")
	default:
		b.WriteString("upright")
	}
	return b.String()
}
