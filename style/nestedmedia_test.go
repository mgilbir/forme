package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// mediaCascade runs an author stylesheet against a document and returns the
// computed styles and the findings. The medium is paper, which is the only one
// this engine has.
func mediaCascade(t *testing.T, htmlSrc, authorCSS string) (Styled, *docFor) {
	t.Helper()
	author, errs := css.ParseStylesheet(authorCSS)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	doc := parseDoc(t, htmlSrc)
	got := ApplyIn(doc, []Sheet{{Origin: OriginAuthor, Rules: author}}, nil, Media{})
	return got, &docFor{t: t, doc: doc, styles: got.Styles}
}

// TestADeclarationInsideANestedMediaIsApplied is the defect.
//
// A nested @media holds a style block — declarations belonging to the rule it
// is written inside, and rules relative to it. It was read as a rule list, the
// way the top-level form is, so "color: red" became a qualified rule with no
// block and the parser discarded it. "p { color: blue; @media print { color:
// red } }" came out blue on paper, and the parse error went nowhere.
//
// That is the shape every stylesheet written since nesting arrived uses.
func TestADeclarationInsideANestedMediaIsApplied(t *testing.T) {
	const src = `#a { color: rgb(0, 0, 255); @media print { color: rgb(255, 0, 0) } }`
	_, doc := mediaCascade(t, `<p id="a">x</p>`, src)
	if got := doc.style("#a")["color"]; !strings.Contains(got, "255, 0, 0") {
		t.Errorf("on paper the colour is %q, want the red the nested @media asked for", got)
	}
}

// TestANestedMediaThatDoesNotMatchChangesNothing is the other half: the block is
// for another medium and this is not it.
func TestANestedMediaThatDoesNotMatchChangesNothing(t *testing.T) {
	const src = `#a { color: rgb(0, 0, 255); @media screen { color: rgb(255, 0, 0) } }`
	_, doc := mediaCascade(t, `<p id="a">x</p>`, src)
	if got := doc.style("#a")["color"]; !strings.Contains(got, "0, 0, 255") {
		t.Errorf("on paper the colour is %q, want the blue outside the @media", got)
	}
}

// TestANestedMediaIsOrderedWhereItWasWritten pins the cascade's answer: an
// @media adds no specificity and no priority, so what wins between it and its
// neighbours is which came last.
func TestANestedMediaIsOrderedWhereItWasWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the block last",
			`#a { color: rgb(0, 0, 255); @media print { color: rgb(255, 0, 0) } }`, "255, 0, 0"},
		{"the block first",
			`#a { @media print { color: rgb(255, 0, 0) } color: rgb(0, 0, 255) }`, "0, 0, 255"},
	} {
		_, doc := mediaCascade(t, `<p id="a">x</p>`, tc.src)
		if got := doc.style("#a")["color"]; !strings.Contains(got, tc.want) {
			t.Errorf("%s: the colour is %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestARuleInsideANestedMediaIsStillRelativeToItsParent keeps the other half of
// a style block working: a nested @media may hold rules as well as
// declarations, and those are written against the rule the block is in.
func TestARuleInsideANestedMediaIsStillRelativeToItsParent(t *testing.T) {
	const src = `#a { color: rgb(0, 0, 255); @media print { & em { color: rgb(0, 128, 0) } } }`
	_, doc := mediaCascade(t, `<p id="a">x <em id="e">y</em></p>`, src)
	if got := doc.style("#e")["color"]; !strings.Contains(got, "0, 128, 0") {
		t.Errorf("the nested rule gave the em %q, want the green it asked for", got)
	}
}

// TestATopLevelMediaStillHoldsRules is the case that always worked, kept
// working: outside a style rule, an @media block is a rule list.
func TestATopLevelMediaStillHoldsRules(t *testing.T) {
	const src = `#a { color: rgb(0, 0, 255) } @media print { #a { color: rgb(255, 0, 0) } }`
	_, doc := mediaCascade(t, `<p id="a">x</p>`, src)
	if got := doc.style("#a")["color"]; !strings.Contains(got, "255, 0, 0") {
		t.Errorf("the colour is %q, want the red the @media asked for", got)
	}
}

// TestAnErrorInsideAMediaBlockIsReported is the finding that was thrown away.
//
// The errors inside a conditional block are the author's to act on exactly as
// the ones outside it are, and the parser's were assigned to nothing.
func TestAnErrorInsideAMediaBlockIsReported(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a bad selector at the top level",
			`@media print { p:: { color: red } }`},
		{"a bad declaration in a nested block",
			`#a { @media print { color: } }`},
	} {
		got, _ := mediaCascade(t, `<p id="a">x</p>`, tc.src)
		if len(got.Findings) == 0 {
			t.Errorf("%s: %q raised nothing", tc.name, tc.src)
		}
	}
}
