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

// TestOneMissingCharacterDoesNotMoveTheWholeBox.
//
// The fallback used to be asked per box: one character the family could not set
// sent *every word* of the paragraph to another face, with that face's metrics
// and that face's line breaks. The finding said so, which was honest, and the
// page was still set in a font nobody chose.
//
// Now the family keeps everything it can set and only the character that needed
// moving moves. The way to see it is that the English is laid out identically
// whether or not the alef is there.
func TestOneMissingCharacterDoesNotMoveTheWholeBox(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	const css = `#p { font-family: Helvetica; font-size: 20px }`

	plain, _ := layoutWith(t, set, `<p id="p">the quick brown fox</p>`, css)
	mixed, _ := layoutWith(t, set, `<p id="p">the quick brown fox א</p>`, css)

	want := map[string]style.Unit{}
	for _, line := range find(t, plain, "p").Lines {
		for _, r := range line.Runs {
			if strings.TrimSpace(r.Text) != "" {
				want[r.Text] = r.X
			}
		}
	}
	if len(want) == 0 {
		t.Fatal("the plain paragraph drew nothing")
	}
	for _, line := range find(t, mixed, "p").Lines {
		for _, r := range line.Runs {
			at, ok := want[r.Text]
			if !ok {
				continue
			}
			if r.X != at {
				t.Errorf("%q is at %v with an alef in the paragraph and at %v "+
					"without one; one character moved the whole box", r.Text, r.X, at)
			}
		}
	}
}

// TestTheFamilyFaceIsKeptEvenWhenAnotherCouldSetEverything.
//
// The sharpest case, and the one the old code got most wrong: a face that can
// set the *whole* paragraph is exactly the face the whole-box fallback chose, so
// a document whose family lacked one character was set entirely in it.
//
// It is also the shape a broad last-resort face makes common rather than rare —
// a face covering most of Unicode can set almost any whole paragraph, so almost
// every document with one foreign character in it was this case. What is
// reported about it is TestAFallbackForOneWordIsNotReported's subject; what is
// *drawn* is this one's.
func TestTheFamilyFaceIsKeptEvenWhenAnotherCouldSetEverything(t *testing.T) {
	latin := loadNoto(t, "NotoSans-Regular.ttf")
	set := oneFaceSet{fallback: latin, standard: StandardFonts()}
	// NotoSans can set all of this and Helvetica cannot: U+0250 is Latin
	// Extended-B, which the standard fourteen do not carry.
	root, _ := layoutWith(t, set,
		`<p id="p">the quick ɐ fox</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)

	for _, r := range drawnRuns(root) {
		if r.Text == "ɐ" {
			continue
		}
		if r.Face == latin.Name() {
			t.Errorf("%q was set in %q although Helvetica has it; a face that "+
				"could set the whole paragraph took the whole paragraph",
				r.Text, r.Face)
		}
	}
}

// TestAFallbackForOneWordIsNotReported.
//
// The finding is about a family that could not do the job, not about a fallback
// happening. A word of another script inside a sentence is set in another face
// because that is what fallback *is*: the page is right, and the text around it
// keeps the metrics the author asked for.
//
// This matters more than it reads, because the library ends in a face that can
// set almost any *whole* paragraph. A report keyed on "could one face have set
// all of this" fires on every document with one foreign character in it — which
// is what it did, on eighty-eight of the suite's, until it was keyed on which
// characters actually moved.
func TestAFallbackForOneWordIsNotReported(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	_, findings := layoutWith(t, set,
		`<p id="p">the quick א fox</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)
	for _, f := range findings {
		if f.Rule == RuleFontFallback {
			t.Errorf("one word of Hebrew in an English sentence reported %s", f.Error())
		}
	}
}

// TestAFamilyThatSetsNothingIsReported is the other half, and the case the
// finding exists for: a caller choosing a font to embed needs to know the family
// it named cannot set the paragraph at all.
func TestAFamilyThatSetsNothingIsReported(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	_, findings := layoutWith(t, set,
		`<p id="p">שלום</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)
	var said bool
	for _, f := range findings {
		if f.Rule == RuleFontFallback {
			said = true
		}
	}
	if !said {
		t.Errorf("a paragraph the family set none of was substituted silently: %v",
			findings)
	}
}

// A generic family, whose resolution is not a substitution.
//
// CSS Fonts §5.1: a generic family is a keyword the user agent maps to a family
// of its choosing, and the choice may depend on the script. A document that says
// "serif" and gets a face that can set its text has been given what it asked
// for — the mapping is the answer.
//
// Twenty-eight of the suite's reftests were held back by the substitution
// finding on exactly that, and they are the strongest case there is: they name
// no font at all. They write "content: counter(test, georgian)", inherit the
// initial font-family, and were told that no face for "serif" could set text the
// document itself had generated.

// TestAGenericFamilyResolvingIsNotASubstitution.
func TestAGenericFamilyResolvingIsNotASubstitution(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	for _, family := range []string{"serif", "sans-serif", "monospace", "serif, sans-serif"} {
		_, findings := layoutWith(t, set,
			`<p id="p">שלום</p>`,
			`#p { font-family: `+family+`; font-size: 20px }`)
		for _, f := range findings {
			if f.Rule == RuleFontFallback {
				t.Errorf("font-family: %s reported a substitution: %s", family, f.Message)
			}
		}
	}
}

// TestNamingARealFamilyKeepsTheFinding is the other half, and the line this
// draws. An author who wrote "Kartuli, serif" asked for Kartuli; a page set in
// something else is one they would want to know about, and the generic behind it
// is a fallback they wrote rather than the whole of their request.
func TestNamingARealFamilyKeepsTheFinding(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	for _, family := range []string{
		"Helvetica",
		"Kartuli, serif",
		// Georgia is in the standard-family map, which answers a different
		// question: which of the fourteen to use for a name. It is a family a
		// document really asked for by name, and reading that map as the generic
		// list would treat this document as having asked for nothing.
		"Georgia",
	} {
		_, findings := layoutWith(t, set,
			`<p id="p">שלום</p>`,
			`#p { font-family: `+family+`; font-size: 20px }`)
		var said bool
		for _, f := range findings {
			if f.Rule == RuleFontFallback {
				said = true
			}
		}
		if !said {
			t.Errorf("font-family: %s was substituted silently: %v", family, findings)
		}
	}
}

// TestTheGenericTestIsNotPassingByAccident. Every case above rests on the
// fallback face really being used, so a fixture that quietly set nothing would
// satisfy them all. This is the control.
func TestTheGenericTestIsNotPassingByAccident(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	root, _ := layoutWith(t, set,
		`<p id="p">שלום</p>`,
		`#p { font-family: serif; font-size: 20px }`)
	if got := faceUsedFor(root, "שלום"); got != hebrew.Name() {
		t.Errorf("the Hebrew was set in %q, not the fallback face %q; the tests "+
			"above would pass with no substitution happening at all",
			got, hebrew.Name())
	}
}

// Which text the question is asked of.
//
// A family that cannot set a paragraph is worth reporting; a family that cannot
// set one span of it is not, because the span is not what the reader sees. The
// two are the same code asked over different text, and the difference is only
// where the answer is given — so these tests write one paragraph several ways
// and check the answer does not depend on the markup.
//
// The suite writes that shape often enough to be worth the care: a <br> between
// two lines of Japanese makes a box of one character each, and a currency sign
// in a <span> makes a box of one character. Thirty-four of its reftests were
// held back by an answer given per box.

// substitutionFindings returns the substitution findings' messages.
func substitutionFindings(findings []Finding) []string {
	var out []string
	for _, f := range findings {
		if f.Rule == RuleFontFallback {
			out = append(out, f.Message)
		}
	}
	return out
}

// TestAFamilyIsAskedOfTheParagraphHoweverItIsDivided.
func TestAFamilyIsAskedOfTheParagraphHoweverItIsDivided(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	for _, body := range []string{
		`the quick א fox`,
		`the quick <span>א</span> fox`,
		`the quick<br>א<br>fox`,
		`<span>א</span> fox`,
		`the quick <span>א</span>`,
		`<span>א</span><span>the</span><span>א</span>`,
	} {
		_, findings := layoutWith(t, set,
			`<p id="p">`+body+`</p>`,
			`#p { font-family: Helvetica; font-size: 20px }`)
		if said := substitutionFindings(findings); len(said) != 0 {
			t.Errorf("a paragraph written %q, which Helvetica sets most of, "+
				"reported %v", body, said)
		}
	}
}

// TestAParagraphTheFamilySetsNoneOfIsReportedHoweverItIsDivided is the other
// half: dividing it changes nothing, because none of the pieces were set either.
func TestAParagraphTheFamilySetsNoneOfIsReportedHoweverItIsDivided(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	for _, body := range []string{
		`שלום`,
		`<span>ש</span>לום`,
		`ש<br>לום`,
	} {
		_, findings := layoutWith(t, set,
			`<p id="p">`+body+`</p>`,
			`#p { font-family: Helvetica; font-size: 20px }`)
		if said := substitutionFindings(findings); len(said) != 1 {
			t.Errorf("a paragraph written %q, which Helvetica sets none of, "+
				"reported %v, want one finding", body, said)
		}
	}
}

// TestAnInlineBlockDoesNotAnswerForTheParagraphAroundIt.
//
// An inline-block is laid out where it sits on the line, so its own paragraph
// finishes while the paragraph holding it is half gathered. Answering there
// answers from the first half — and the first half here is the Hebrew letter,
// with the English that Helvetica sets still to come.
func TestAnInlineBlockDoesNotAnswerForTheParagraphAroundIt(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	_, findings := layoutWith(t, set,
		`<p id="p">א<span id="ib">x</span> fox</p>`,
		`#p { font-family: Helvetica; font-size: 20px }
		 #ib { display: inline-block; font-family: Courier }`)
	if said := substitutionFindings(findings); len(said) != 0 {
		t.Errorf("the inline-block finishing mid-paragraph answered for the "+
			"paragraph around it: %v", said)
	}
}

// TestAFamilyIsReportedOnceHoweverManyParagraphsCannotUseIt. The finding is
// about the family, and a document naming one in a hundred paragraphs has one
// gap in it, not a hundred.
//
// The paragraphs sit at different depths on purpose. The recorder already folds
// two identical findings about the same place into one, so three siblings would
// pass whether the rule was kept or not.
func TestAFamilyIsReportedOnceHoweverManyParagraphsCannotUseIt(t *testing.T) {
	hebrew := loadHebrew(t)
	set := oneFaceSet{fallback: hebrew, standard: StandardFonts()}
	_, findings := layoutWith(t, set,
		`<p class="p">שלום</p><div><p class="p">שלום</p></div>`+
			`<blockquote><p class="p">שלום</p></blockquote>`,
		`.p { font-family: Helvetica; font-size: 20px }`)
	if said := substitutionFindings(findings); len(said) != 1 {
		t.Errorf("three paragraphs in one family reported %v, want one finding",
			said)
	}
}
