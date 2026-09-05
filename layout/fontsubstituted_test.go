package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/shape"
)

// embeddedFallback is a face that is always there.
//
// The corpus faces the tests beside this one use are fetched, so a test that
// needs one skips where they are not — and a *coverage* test that skips is a
// rule nothing has ever been seen to raise. That is what happened: the whole
// suite passed locally with the fonts in place and the plain "make test" run in
// CI reported that font-substituted had never fired. Noto Sans is checked into
// the repository and covers Cyrillic, which none of the base-14 faces do.
func embeddedFallback(t *testing.T) *shape.Face {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	return face
}

// The line between a family that could not be resolved and a family that
// resolved and had no glyph.
//
// It is one line and two rules, and everything below is about where it falls.
// The engine reports both — an author who asked for a face and did not get it
// wants to know either way — and counts only the first as something the engine
// did not do. §7.1's companion signal reads that count, so getting the line
// wrong in either direction is a reftest number that means something other than
// what it says.

// facingFindings returns the findings about which face set the text.
func facingFindings(findings []Finding) []Finding {
	var out []Finding
	for _, f := range findings {
		if aboutTheFace(f) {
			out = append(out, f)
		}
	}
	return out
}

// TestAFamilyNobodyHasIsAGap. The page is set in a face the document never
// named and the engine had nothing better to offer, which is the case the
// unsupported set is for: a reftest whose two documents both landed here agree
// about type neither of them asked for.
func TestAFamilyNobodyHasIsAGap(t *testing.T) {
	_, findings := layoutWith(t, StandardFonts(),
		`<p id="p">text</p>`,
		`#p { font-family: "Nonesuch Display" }`)

	got := facingFindings(findings)
	if len(got) != 1 {
		t.Fatalf("a family nobody has raised %d findings about the face, want 1: %v",
			len(got), findings)
	}
	if got[0].Rule != RuleFontFallback {
		t.Errorf("it was raised as %q, want %q", got[0].Rule, RuleFontFallback)
	}
	if !got[0].Unsupported() {
		t.Error("a family that could not be resolved does not count as something " +
			"this engine did not do, so a reftest set in the wrong type on both " +
			"sides would be counted as having proved something")
	}
	if !strings.Contains(got[0].Message, "Nonesuch") {
		t.Errorf("the message %q does not name the family that was asked for", got[0].Message)
	}
}

// TestAFamilyWithoutTheGlyphIsNotAGap is the other side of the line, and the
// one the change was for. Helvetica is here, it loaded, and it has no ש — so
// font matching goes on to the next face, which is what CSS asks for and what
// every browser does. Nothing was declined, so nothing is counted as declined.
func TestAFamilyWithoutTheGlyphIsNotAGap(t *testing.T) {
	fired[RuleFontSubstituted] = true

	noto := embeddedFallback(t)
	_, findings := layoutWith(t, oneFaceSet{fallback: noto, standard: StandardFonts()},
		`<p id="p">привет</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)

	got := facingFindings(findings)
	if len(got) != 1 {
		t.Fatalf("a family without the glyph raised %d findings about the face, "+
			"want 1: %v", len(got), findings)
	}
	if got[0].Rule != RuleFontSubstituted {
		t.Errorf("it was raised as %q, want %q", got[0].Rule, RuleFontSubstituted)
	}
	if got[0].Unsupported() {
		t.Error("going on to the next face is what CSS asks for, and counting it " +
			"as something this engine did not do discounts a reftest that " +
			"compared exactly what it meant to")
	}
	// Still said out loud. The change is about what the finding is counted as
	// and not about whether it is raised.
	if !strings.Contains(got[0].Message, "Helvetica") ||
		!strings.Contains(got[0].Message, noto.Name()) {
		t.Errorf("the message %q does not say which family gave way to which face",
			got[0].Message)
	}
}

// TestOnlyOneOfTheTwoIsCountedAsAGap. Both tests above read one document each
// and would pass just as well if the table said the same thing about both
// rules, so this is the comparison neither of them makes.
//
// What is *not* here is a check that the two rules are different strings. It
// was, and it could not be made to fail: both are keys of defaultSeverity, so
// giving them one value is a duplicate key in a map literal and the package
// stops compiling. The compiler holds that one, and a check nothing can break
// reads as protection that is not there.
func TestOnlyOneOfTheTwoIsCountedAsAGap(t *testing.T) {
	if !unsupportedRules[RuleFontFallback] {
		t.Error("a family that could not be resolved is not counted as a gap")
	}
	if unsupportedRules[RuleFontSubstituted] {
		t.Error("a family that resolved and lacked the glyph is counted as a gap")
	}
}
