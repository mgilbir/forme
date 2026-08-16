package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Falling back within a run rather than for the whole box.
//
// The case is a sentence of one script with a word or a letter of another in it,
// which is what a citation, a name or a technical term makes — and it is the
// case the whole-box question has no answer for, because no single face covers
// both halves.

// drawnRuns returns the text of each run and the face it was set in, in paint
// order, which is what a reader of the page actually gets.
func drawnRuns(root *Fragment) []struct {
	Text string
	Face string
} {
	var out []struct {
		Text string
		Face string
	}
	for _, op := range Paint(root) {
		d, ok := op.(DrawText)
		if !ok || strings.TrimSpace(d.Text) == "" {
			continue
		}
		name := "<none>"
		if d.Face != nil {
			name = d.Face.Name()
		}
		out = append(out, struct {
			Text string
			Face string
		}{d.Text, name})
	}
	return out
}

// faceUsedFor returns the face the given text was drawn in, or "".
func faceUsedFor(root *Fragment, text string) string {
	for _, r := range drawnRuns(root) {
		if r.Text == text {
			return r.Face
		}
	}
	return ""
}

// TestOneLetterOfAnotherScriptTakesTheFaceThatHasIt.
//
// The whole-box question has no answer here and that is the point: the Hebrew
// face has no Latin, and the Latin face has no Hebrew, so asking either to set
// the whole sentence fails and the box used to keep the family's face and report
// the letter missing.
func TestOneLetterOfAnotherScriptTakesTheFaceThatHasIt(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	root, findings := layoutWith(t, set,
		`<p id="p">Force bidi: א here</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)

	if got := faceUsedFor(root, "א"); got != hebrew.Name() {
		t.Errorf("the alef was set in %q, want %q", got, hebrew.Name())
	}
	// And the Latin around it is still the family's face, not the Hebrew one.
	if got := faceUsedFor(root, "Force"); got == hebrew.Name() {
		t.Errorf("the Latin was set in the Hebrew face %q", got)
	}
	for _, f := range findings {
		if f.Rule == RuleGlyphMissing {
			t.Errorf("a character set by a fallback face was still reported "+
				"missing: %s", f.Error())
		}
	}
}

// TestTheFirstClusterOfARunIsExamined.
//
// segment.Boundaries gives the offsets *inside* a string and deliberately omits
// both ends, so a loop over it that does not put the leading zero back skips the
// first cluster of every run. That is invisible whenever the run begins with
// text the primary face has — which is almost every run — and shows only when a
// run *is* the foreign word, which is exactly the case this file exists for.
//
// It cost a reftest: the two lines of bidi-glyph-mirroring-002 set the same
// alef, one of them as its own run and one of them not, so one got the Hebrew
// face and the other did not and the two lines stopped being identical.
func TestTheFirstClusterOfARunIsExamined(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	// The alef is alone between two spaces, so the piece handed to the splitter
	// is exactly one cluster and there is no leading text to hide the fault.
	root, _ := layoutWith(t, set,
		`<p id="p">a א b</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)
	if got := faceUsedFor(root, "א"); got != hebrew.Name() {
		t.Errorf("an alef alone in its run was set in %q, want %q — the first "+
			"cluster of the run was not examined", got, hebrew.Name())
	}
}

// TestTextOneFaceCoversIsNotSplit is the containment argument for the whole
// change: a document that does not mix scripts must produce exactly what it
// produced before, which means one run holding the text it started with.
func TestTextOneFaceCoversIsNotSplit(t *testing.T) {
	// The fallback here covers Latin *as well*, which is what makes this an
	// assertion rather than a tautology: with a Hebrew-only fallback the primary
	// face wins by default because nothing else can set the letters at all, and
	// a splitter that abandoned the primary face at every cluster would look
	// correct. This one would move the whole sentence.
	latin := loadNoto(t, "NotoSans-Regular.ttf")
	set := oneFaceSet{fallback: latin, standard: StandardFonts()}
	root, findings := layoutWith(t, set,
		`<p id="p">the quick brown fox</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)
	for _, f := range findings {
		t.Errorf("plain Latin reported %s", f.Error())
	}
	for _, r := range drawnRuns(root) {
		if r.Face == latin.Name() {
			t.Errorf("the run %q went to the fallback face although the family's "+
				"face can set it", r.Text)
		}
	}
}

// TestTheFamilysFaceKeepsEverythingItCanSet.
//
// The property that makes this a *fallback* rather than a re-facing: one letter
// the family cannot set must move one letter, not the paragraph around it. A
// splitter that asked for a face at every cluster rather than only at the ones
// that need one would hand the whole sentence to whatever the fallback list
// offers first, and the page would silently change typeface at the first
// citation in it.
//
// The fixture needs all three of its parts to bite. The fallback covers the
// Latin as well, so there is somewhere for the Latin to go wrongly; the text has
// a character the family cannot set, so the cheap "nothing is missing" answer
// does not apply; and the Latin is in the *same piece* as that character, with
// no space between them, because the cheap answer is asked per piece and a
// sentence with spaces in it protects every word but the one that needs it.
func TestTheFamilysFaceKeepsEverythingItCanSet(t *testing.T) {
	latin := loadNoto(t, "NotoSans-Regular.ttf")
	set := oneFaceSet{fallback: latin, standard: StandardFonts()}
	root, _ := layoutWith(t, set,
		`<p id="p">theאfox</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)

	for _, r := range drawnRuns(root) {
		if r.Text == "א" {
			continue
		}
		if r.Face == latin.Name() {
			t.Errorf("%q went to the fallback face although the family's face can "+
				"set it; one foreign letter re-faced the sentence around it", r.Text)
		}
	}
}

// TestAFaceChangeIsNotABreakOpportunity.
//
// A run split at a face boundary is still one word, and a line may not end
// inside it. The breaker breaks only where an item says BreakBefore, so the
// continuation runs must not say it — otherwise a Hebrew letter in the middle of
// a Latin word becomes a place to wrap.
func TestAFaceChangeIsNotABreakOpportunity(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	// A word with the alef inside it, *after* a real break opportunity, in a box
	// far too narrow for it. The space before it is what makes this a test: the
	// piece carries BreakBefore, so a continuation run that copied the piece's
	// flag instead of clearing it would offer a second opportunity inside the
	// word — and without the leading word there is no flag to copy and the fault
	// is invisible.
	root, _ := layoutWith(t, set,
		`<p id="p">one aaaaaאaaaaa</p>`,
		`#p { font-family: Helvetica; font-size: 20px; width: 60px }`)

	f := find(t, root, "p")
	// Two lines: "one" and the long word, which overflows rather than breaking.
	if len(f.Lines) != 2 {
		t.Fatalf("%d lines, want the two the fixture describes", len(f.Lines))
	}
	// The word is on one line: every run of it shares a baseline.
	var ys []style.Unit
	for _, line := range f.Lines {
		for _, run := range line.Runs {
			if strings.ContainsAny(run.Text, "א") || strings.HasPrefix(run.Text, "aaaaa") {
				ys = append(ys, line.Rect.Y)
			}
		}
	}
	for i := range ys {
		if ys[i] != ys[0] {
			t.Errorf("the word is spread over more than one line (%v); a change of "+
				"face is not a break opportunity", ys)
			break
		}
	}
}

// TestACharacterNoFaceHasIsStillReported: the fallback is not an excuse to stop
// reporting. A cluster no available face can set stays with the family's face
// and is named, which is what happened before any of this and is still the only
// true answer.
func TestACharacterNoFaceHasIsStillReported(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	// U+16A0 RUNIC LETTER FEHU: neither the standard faces nor the Hebrew one.
	_, findings := layoutWith(t, set,
		`<p id="p">a ᚠ b</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)
	var said bool
	for _, f := range findings {
		if f.Rule == RuleGlyphMissing && strings.Contains(f.Message, "16A0") {
			said = true
		}
	}
	if !said {
		t.Errorf("a character no face can set was not reported: %v", findings)
	}
}
