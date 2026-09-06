package layout

import (
	"strings"
	"testing"
)

// oneByOneGIF is the smallest picture that decodes, as a data URI.
const oneByOneGIF = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"

// TestContentWiderThanThePageIsScaled is the defect stated as a test.
//
// The natural size was the root's border box alone, and a block-level root's
// width is the page's by construction — so it reported every document as
// needing exactly the space it was given and horizontal scale-to-fit could
// never fire. A table two thousand pixels wide came out at its full width, off
// the paper, with no finding of any kind. That is the commonest way a document
// written for a screen fails on paper, and it was the one case this engine's
// central promise did not cover.
//
// Each of these needs more width than an A4 page has. Each must be scaled.
func TestContentWiderThanThePageIsScaled(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a wide block with only text in it",
			`<div style="width:2000px;text-align:right">far right</div>`},
		{"a wide block with a background",
			`<div style="width:2000px;height:10px;background:red"></div>`},
		{"a wide table",
			`<table style="width:2000px"><tr><td style="text-align:right">far right</td></tr></table>`},
		{"a wide body",
			`<body style="width:2000px">x</body>`},
		{"a wide floated block",
			`<div style="float:left;width:2000px;height:10px;background:red"></div>`},
		{"an absolutely positioned block off to the right",
			`<div style="position:absolute;left:2000px;width:50px;height:10px;background:red"></div>`},
		{"a picture wider than the page",
			`<img style="width:2000px;height:20px" src="` + oneByOneGIF + `">`},
		{"an unbreakable run of text",
			`<div style="width:100px;white-space:nowrap">` + strings.Repeat("a", 300) + `</div>`},
	} {
		got := Compose(Input{HTML: tc.src}, Options{MinScale: 0.01})
		if got.Scale >= 1 {
			t.Errorf("%s: scale %v, want less than 1 — the content is wider than the page "+
				"and was drawn off it", tc.name, got.Scale)
		}
		if got.NaturalSize.W <= A4.Content().W {
			t.Errorf("%s: the natural width came to %.0f px, no more than the page's %.0f",
				tc.name, got.NaturalSize.W.Px(), A4.Content().W.Px())
		}
	}
}

// TestTheHeightAxisIsUnchanged is the axis that always worked, kept working.
// The root's height does grow with its content, which is why the defect above
// survived: the axis that reaches its bound in ordinary use was right.
func TestTheHeightAxisIsUnchanged(t *testing.T) {
	got := Compose(Input{HTML: `<div style="height:3000px"></div>`}, Options{MinScale: 0.01})
	if got.Scale >= 1 {
		t.Errorf("a three-thousand-pixel-tall document was not scaled: %v", got.Scale)
	}
}

// TestAnOrdinaryDocumentIsNotScaled is the other side of it. A measurement that
// finds overflow everywhere would shrink every page, and a wrong scale is worse
// than none: it is applied silently.
func TestAnOrdinaryDocumentIsNotScaled(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a paragraph", `<p>hello</p>`},
		{"nested blocks", `<div><div><p>hello</p></div></div>`},
		{"a table", `<table><tr><td>a</td><td>b</td></tr></table>`},
		{"a list", `<ul><li>one</li><li>two</li></ul>`},
		{"a float beside text", `<div style="float:left;width:50px;height:20px"></div><p>hello</p>`},
		{"an inline box with a background", `<p><span style="background:red">x</span> y</p>`},
		{"a picture", `<img style="width:20px;height:20px" src="` + oneByOneGIF + `">`},
		{"a paragraph of wrapped text", `<p>` + strings.Repeat("word ", 200) + `</p>`},
	} {
		got := Compose(Input{HTML: tc.src}, Options{})
		if got.Scale != 1 {
			t.Errorf("%s: scale %v, want 1 — nothing here needs more room than the page has "+
				"(natural size %.0f x %.0f, page %.0f x %.0f)",
				tc.name, got.Scale, got.NaturalSize.W.Px(), got.NaturalSize.H.Px(),
				A4.Content().W.Px(), A4.Content().H.Px())
		}
		if got.Refused {
			t.Errorf("%s: refused: %v", tc.name, got.Findings)
		}
	}
}

// TestThePageOverflowGuardSeesEveryKindOfInk is C37: the guard read the fills
// and nothing else, so the same box off the same page was refused or silent
// depending on whether what filled it was a colour or a picture.
//
// Negative coordinates are the case, because scaling cannot fix them: a page is
// scaled about its origin, so content to the left of it stays to the left of
// it. That makes this the one thing left for the guard to say.
func TestThePageOverflowGuardSeesEveryKindOfInk(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a filled box", `<div style="position:absolute;left:-50px;top:-50px;` +
			`width:100px;height:100px;background:red"></div>`},
		{"a picture", `<img style="position:absolute;left:-50px;top:-50px;` +
			`width:100px;height:100px" src="` + oneByOneGIF + `">`},
		{"a repeated background", `<div style="position:absolute;left:-50px;top:-50px;` +
			`width:100px;height:100px;background:url(` + oneByOneGIF + `) repeat"></div>`},
	} {
		got := Compose(Input{HTML: tc.src}, Options{})
		if !hasRule(got.Findings, RuleOverflowPage) {
			t.Errorf("%s at (-50,-50) raised %v, want the page-overflow finding",
				tc.name, ruleNames(got.Findings))
		}
	}
}

// TestTheOverflowMessageNamesTheBoxAndHowFarOut pins the report, since a
// message that named an innocent box and quoted its far corner sent a reader to
// the wrong element.
func TestTheOverflowMessageNamesTheBoxAndHowFarOut(t *testing.T) {
	got := Compose(Input{HTML: `<div style="position:absolute;left:-50px;top:-50px;` +
		`width:100px;height:100px;background:red"></div>`}, Options{})
	var msg string
	for _, f := range got.Findings {
		if f.Rule == RuleOverflowPage {
			msg = f.Message
		}
	}
	if msg == "" {
		t.Fatalf("no page-overflow finding: %v", ruleNames(got.Findings))
	}
	for _, want := range []string{"-50.0,-50.0", "reaching 50.0 px outside"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the finding says %q, which does not contain %q", msg, want)
		}
	}
}

// TestTheGuardIsSilentOnADocumentTheScaleAccountedFor is what the guard's own
// documentation claims about it — "it should never fire … a self-check". It
// used to be the ordinary outcome for any wide box with a background, at Error
// severity, with a message blaming the engine.
func TestTheGuardIsSilentOnADocumentTheScaleAccountedFor(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a wide block with a background",
			`<div style="width:2000px;height:10px;background:red"></div>`},
		{"a tall block with a background",
			`<div style="width:10px;height:3000px;background:red"></div>`},
		{"a wide picture",
			`<img style="width:2000px;height:20px" src="` + oneByOneGIF + `">`},
	} {
		got := Compose(Input{HTML: tc.src}, Options{MinScale: 0.01})
		if hasRule(got.Findings, RuleOverflowPage) {
			t.Errorf("%s: the scale was computed from this box and the guard fired anyway, "+
				"so the two disagree: %v", tc.name, got.Findings)
		}
	}
}
