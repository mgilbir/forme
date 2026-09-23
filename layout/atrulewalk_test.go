package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/style"
)

// The @font-face rules a document's sheets hold are found by the cascade's own
// walk, so one inside an @media, @supports or @layer whose condition holds is a
// face like any other. Only the top-level ones were taken out of the sheets
// before, and one written anywhere else reached the cascade as an at-rule "not
// applied yet" (audit C138).

func TestAFontFaceInsideAConditionalIsLoaded(t *testing.T) {
	for _, sheet := range []string{
		`@media print { @font-face { font-family: Trial; src: url(trial.ttf) } }`,
		`@supports (display: block) { @font-face { font-family: Trial; src: url(trial.ttf) } }`,
		`@layer fonts { @font-face { font-family: Trial; src: url(trial.ttf) } }`,
	} {
		res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
		built := Build(Input{HTML: docWithFontFace(sheet), Resources: res})
		if face, ok := built.Fonts.Face("Trial", false, false); !ok || face == nil {
			t.Errorf("%s: the family did not resolve; findings: %v", sheet, built.Findings)
		}
		for _, f := range built.Findings {
			if strings.Contains(f.Message, "@font-face") {
				t.Errorf("%s reported %q", sheet, f.Message)
			}
		}
	}

	// A condition that does not hold keeps the face out, as it keeps a style
	// rule out.
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	built := Build(Input{
		HTML:      docWithFontFace(`@media screen { @font-face { font-family: Trial; src: url(trial.ttf) } }`),
		Resources: res,
	})
	if _, ok := built.Fonts.Face("Trial", false, false); ok {
		t.Error("an @font-face inside @media screen was loaded for paper")
	}

	// One inside a style rule is not an @font-face rule: CSS Nesting allows it
	// nowhere there. It is not loaded, and it is not silent.
	built = Build(Input{
		HTML:      docWithFontFace(`p { @font-face { font-family: Trial; src: url(trial.ttf) } }`),
		Resources: &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}},
	})
	if _, ok := built.Fonts.Face("Trial", false, false); ok {
		t.Error("an @font-face inside a style rule was loaded")
	}
	said := false
	for _, f := range built.Findings {
		if f.Rule == RuleInvalidCSS && strings.Contains(f.Message, "inside a style rule") {
			said = true
		}
	}
	if !said {
		t.Errorf("an @font-face inside a style rule was dropped silently: %v", built.Findings)
	}
}

// TestALaterLayerDefinesTheFace: Cascade 5 §6.4.3 orders a name-defining
// at-rule by its layer, so of two @font-face rules for one family the one in
// the later layer is the later one — and the loader lets the later one win a
// tie — whatever order the text puts them in.
func TestALaterLayerDefinesTheFace(t *testing.T) {
	sheet := `@layer a, b; ` +
		`@layer b { @font-face { font-family: Trial; src: url(later.ttf) } } ` +
		`@layer a { @font-face { font-family: Trial; src: url(earlier.ttf) } }`
	faces := fontFacesOf(prepareFor(t, sheet).FontFaces)
	if len(faces) != 2 {
		t.Fatalf("%d faces were handed over, want 2", len(faces))
	}
	if last := pageText(faces[1].rule.Block); !strings.Contains(last, "later.ttf") {
		t.Errorf("the face in the later layer is not the later one: %s", last)
	}
}

// prepareFor reads one author stylesheet the way Build does.
func prepareFor(t *testing.T, src string) *style.Prepared {
	t.Helper()
	rules, errs := css.ParseStylesheet(src)
	if len(errs) != 0 {
		t.Fatalf("the fixture did not parse: %v", errs)
	}
	return style.Prepare([]style.Sheet{{Origin: style.OriginAuthor, Rules: rules}}, style.Media{})
}
