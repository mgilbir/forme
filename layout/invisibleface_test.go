package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A character that draws nothing does not choose a face.
//
// faceRunsFor cuts a run wherever the face changes, and it asks each cluster
// which face can set it. That question has no useful answer for a character
// that sets no paper: the standard fourteen encode U+00AD through WinAnsi and
// so are held to "have" it, while the letters around it move to a face that has
// *them* — so a soft hyphen inside an Arabic word cut the word into three runs,
// each shaped alone, with the standard face's quarter-em advance for the soft
// hyphen opening a gap in the middle.
//
// The rule is the one the shaping context beside it follows: what draws nothing
// and takes no room is not a boundary.

// A soft hyphen inside a word does not move the word into another face.
func TestASoftHyphenDoesNotCutAWordIntoTwoFaces(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, "دامىدى"); !ok {
		t.Skip("no face here can set the fixture")
	}
	frag := kernLayout(t, faces, `<div id="d" lang="ug" dir="rtl">دامي&shy;دى</div>`,
		`#d { font-size: 20px; hyphens: manual }`)
	lines := linesOf(t, frag, "d")
	if len(lines) != 1 {
		t.Fatalf("the fixture wrapped: %q", lineTexts(lines))
	}
	seen := map[string]bool{}
	for _, r := range lines[0].Runs {
		if r.Face != nil && r.Text != "" {
			seen[r.Face.Name()] = true
		}
	}
	if len(seen) != 1 {
		var names []string
		for n := range seen {
			names = append(names, n)
		}
		t.Errorf("the word is set in %d faces, %v — the soft hyphen took a face "+
			"of its own and cut the word in three", len(seen), names)
	}
	// And it takes no room: the word is as wide with the mark as without it.
	plain := cjkNaturalWidth(t, faces, `<div id="d" lang="ug" dir="rtl">داميدى</div>`,
		`#d { font-size: 20px }`)
	marked := cjkNaturalWidth(t, faces, `<div id="d" lang="ug" dir="rtl">دامي&shy;دى</div>`,
		`#d { font-size: 20px; hyphens: manual }`)
	// Within a layout unit, which is the rounding of measuring two runs against
	// one: the mark ends a piece, so the word is measured in halves either side
	// of it. What this is about is a quarter of an em, which is what the
	// standard faces give U+00AD and what the word had opening a hole in it.
	if d := marked.Sub(plain); d > 1 || d < -1 {
		t.Errorf("the word is %v wide with the mark in it and %v without; a soft "+
			"hyphen that is not at a break takes no room", marked, plain)
	}
}

// And a character that draws nothing is not missing from the page.
//
// The finding says "the character is missing from the page and from the text
// extracted out of it", and neither half is true of a joiner or a variation
// selector or a soft hyphen. It used to be left to a coincidence — shaping
// answers "nothing missing" for almost all of them — and U+180E MONGOLIAN VOWEL
// SEPARATOR is the one it does not, because Unicode reclassified it from a
// space to a format character in 6.3 and the faces did not follow.
// findingsWithFace lays a document out with one face the document itself
// supplies, and returns everything the engine had to say about it.
func findingsWithFace(t *testing.T, file, htmlSrc, cssSrc string) []Finding {
	t.Helper()
	root := os.Getenv("WPT_TESTS")
	if root == "" {
		t.Skip("set WPT_TESTS (or run `make test-wpt`) for the suite's own faces")
	}
	data, err := os.ReadFile(filepath.Join(root, "fonts", file))
	if err != nil {
		t.Skip("no ", file, ": ", err)
	}
	res := &fileResolver{files: map[string][]byte{"f.ttf": data}}
	built := Build(Input{HTML: htmlSrc, Resources: res, CSS: []Stylesheet{{
		Source: noDefaults + `@font-face { font-family: Trial; src: url(f.ttf) } ` + cssSrc,
	}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	return append(append([]Finding{}, built.Findings...), rec.Findings()...)
}

// A character that draws nothing is not missing from the page.
//
// The finding says "the character is missing from the page and from the text
// extracted out of it", and neither half is true of a joiner or a variation
// selector or a soft hyphen. It used to be left to a coincidence — shaping
// answers "nothing missing" for almost all of them — and Ahem is the face that
// shows the coincidence failing: it reports U+180E MONGOLIAN VOWEL SEPARATOR
// missing, because Unicode reclassified that character from a space to a format
// character in 6.3 and the font did not follow. The suite's
// line-breaking-atomic-015 writes it.
func TestACharacterThatDrawsNothingIsNotReportedMissing(t *testing.T) {
	// Each at the *start* of its run, which is where the face is asked about it
	// at all: shaping "a\u180eb" as a whole reports nothing missing, and
	// shaping "\u180eb" reports one. That is the coincidence this rule stops
	// depending on — the answer was right for the wrong reason and only most of
	// the time.
	for _, tc := range []struct{ text, what string }{
		{"᠎b", "U+180E MONGOLIAN VOWEL SEPARATOR"},
		{"­b", "a soft hyphen"},
		{"‍b", "a zero width joiner"},
		{"︀b", "a variation selector"},
	} {
		for _, f := range findingsWithFace(t, "Ahem.ttf",
			`<div id="p">`+tc.text+`</div>`, `#p { font-family: Trial }`) {
			if f.Rule == RuleGlyphMissing {
				t.Errorf("%s was reported missing: %s", tc.what, f.Message)
			}
		}
	}
	// And a character that really is missing still is, so the rows above say
	// something. Ahem has no Devanagari.
	found := false
	for _, f := range findingsWithFace(t, "Ahem.ttf",
		`<div id="p">aकb</div>`, `#p { font-family: Trial }`) {
		if f.Rule == RuleGlyphMissing {
			found = true
		}
	}
	if !found {
		t.Error("a Devanagari letter in Ahem was not reported missing, so the " +
			"rows above say nothing")
	}
}
