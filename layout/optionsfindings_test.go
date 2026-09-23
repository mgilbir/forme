package layout

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// The entry surface says what it means: the options Compose takes, and the
// places the findings it returns point to.

// TestAnOptionThatIsNotANumberIsReplacedAndSaidSo (audit C133): a NaN answers
// false to every comparison, so "MinScale: NaN" passed both of checkOptions'
// guards and refused every document "past the floor of NaN%". An infinity did
// the same for the font-size floor. Every float option is read by one check,
// finite first.
func TestAnOptionThatIsNotANumberIsReplacedAndSaidSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"a NaN scale", Options{MinScale: math.NaN()}},
		{"an infinite scale", Options{MinScale: math.Inf(1)}},
		{"a negatively infinite scale", Options{MinScale: math.Inf(-1)}},
		{"a NaN font size", Options{MinFontSizePt: math.NaN()}},
		{"an infinite font size", Options{MinFontSizePt: math.Inf(1)}},
		{"a negatively infinite font size", Options{MinFontSizePt: math.Inf(-1)}},
	} {
		got := Compose(Input{HTML: `<p>hello</p>`}, tc.opts)
		said := false
		for _, f := range got.Findings {
			if f.Rule == RuleInvalidCSS && strings.Contains(f.Message, "not a number") {
				said = true
			}
		}
		if !said {
			t.Errorf("%s: nothing said the option was replaced: %v", tc.name, got.Findings)
		}
		if got.Refused {
			t.Errorf("%s: an ordinary document was refused: %v", tc.name, got.Findings)
		}
	}
}

// TestNoFindingPointsAtTheFirstByteByOmission (audit C87): a finding raised
// with no Source had the zero value, which renders as "[html byte 0]" — the
// top of the markup — for things that are in no file at all, or in a
// stylesheet. The document starts with a comment, so no element is at byte
// nought and any finding placed there is placed wrongly.
func TestNoFindingPointsAtTheFirstByteByOmission(t *testing.T) {
	const lead = "<!-- the first bytes are this comment -->"
	for _, tc := range []struct {
		name string
		in   Input
		opts Options
		rule Rule
		// at is the markup the finding is about, where it is about an
		// element; empty for one about the whole document or a stylesheet.
		at string
	}{
		{"invalid options", Input{HTML: lead + `<p>x</p>`}, Options{MinScale: -1}, RuleInvalidCSS, ""},
		{"a page with no room", Input{HTML: lead + `<p>x</p>`,
			CSS: []Stylesheet{{Source: `@page { size: 1px }`}}}, Options{}, RuleInvalidCSS, ""},
		{"the scale floor", Input{HTML: lead + `<div style="width: 5000px">x</div>`},
			Options{MinScale: 0.9}, RuleMinScale, ""},
		{"the font-size floor", Input{HTML: lead + `<p style="font-size: 2px">x</p>`},
			Options{}, RuleMinFontSize, "<p"},
		{"an import that fails", Input{HTML: lead + `<style>@import "missing.css";</style><p>x</p>`},
			Options{}, RuleResourceBlocked, ""},
		{"a word that cannot be broken", Input{HTML: lead +
			`<p style="width: 10px">` + strings.Repeat("m", 40) + `</p>`}, Options{},
			RuleUnbreakableOverflow, "<p"},
		{"a font-family nobody has", Input{HTML: lead +
			`<p style="font-family: NoSuchFamily">x</p>`}, Options{}, RuleFontFallback, "<p"},
		{"an aspect-ratio not applied", Input{HTML: lead +
			`<div style="height: 10px; aspect-ratio: 2">x</div>`}, Options{}, RuleUnsupportedValue, "<div"},
	} {
		got := Compose(tc.in, tc.opts)
		found := false
		for _, f := range got.Findings {
			if f.Source.HTMLOffset == 0 {
				t.Errorf("%s: %q is placed at byte nought of the markup", tc.name, f.Error())
			}
			if f.Source.HTMLOffset >= 0 && f.Source.CSSOffset >= 0 {
				t.Errorf("%s: %q is placed in the markup and a stylesheet at once", tc.name, f.Error())
			}
			if f.Rule != tc.rule {
				continue
			}
			found = true
			if want := strings.Index(tc.in.HTML, tc.at); tc.at != "" && f.Source.HTMLOffset != want {
				t.Errorf("%s: %q is placed at %+v, want the element at byte %d",
					tc.name, f.Error(), f.Source, want)
			}
		}
		if !found {
			t.Errorf("%s: no %s finding to check: %v", tc.name, tc.rule, got.Findings)
		}
	}
	// And the zero Source, however a Finding comes by it, is recorded and
	// rendered as no place.
	rec := NewRecorder(nil)
	rec.ReportDetail(Finding{Rule: RuleLimit, Message: "m"})
	if got := rec.Findings()[0].Source; got != NoSource {
		t.Errorf("a finding raised with no source was recorded at %+v", got)
	}
	if s := (Finding{Rule: RuleLimit, Message: "m"}).Error(); strings.Contains(s, "byte") {
		t.Errorf("a finding with no source renders a place: %s", s)
	}
	if s := (Finding{Rule: RuleLimit, Message: "m", Source: AtHTML(0)}).Error(); !strings.Contains(s, "[html byte 0]") {
		t.Errorf("a finding really at the first byte lost its place: %s", s)
	}
}

// TestAFailedImportPointsAtTheImport: a finding about an @import is about the
// rule, and the rule's place is known.
func TestAFailedImportPointsAtTheImport(t *testing.T) {
	for _, tc := range []struct {
		name, sheet string
		in          Input
	}{
		{"a <style>", "", Input{HTML: `<style>@import "missing.css";</style><p>x</p>`}},
		{"a named sheet", "css/a.css", Input{HTML: `<p>x</p>`,
			CSS: []Stylesheet{{Name: "css/a.css", Source: ` @import "missing.css";`}}}},
	} {
		got := Build(tc.in)
		found := false
		for _, f := range got.Findings {
			if f.Rule != RuleResourceBlocked || !strings.Contains(f.Message, "missing.css") {
				continue
			}
			found = true
			if f.Source.HTMLOffset != -1 || f.Source.CSSOffset < 0 || f.Source.Sheet != tc.sheet {
				t.Errorf("%s: the failed import is placed at %+v", tc.name, f.Source)
			}
		}
		if !found {
			t.Errorf("%s: the failed import was not reported: %v", tc.name, got.Findings)
		}
	}
}

// TestTheRefusedPageSaysWhosePageItIs (audit C140): an @page that set only the
// size left the caller's margins, and the refusal said they were the rule's.
func TestTheRefusedPageSaysWhosePageItIs(t *testing.T) {
	got := Compose(Input{HTML: `<p>x</p>`, CSS: []Stylesheet{{Source: `@page { size: 1px }`}}}, Options{})
	for _, f := range got.Findings {
		if f.Rule != RuleInvalidCSS || !strings.Contains(f.Message, "margins") {
			continue
		}
		if strings.Contains(f.Message, "@page rule's") {
			t.Errorf("the caller's margins are blamed on the @page rule: %s", f.Message)
		}
		if !strings.Contains(f.Message, "settled on") {
			t.Errorf("the refusal does not say which page: %s", f.Message)
		}
		return
	}
	t.Errorf("the margins wider than a 1px sheet were not refused: %v", got.Findings)
}

// TestAllowScaleUpDoesWhatItSays (audit C86): the option enlarges a document
// that is narrower than the sheet, and an ordinary document — whose root is a
// block as wide as the page — is not underfull across it, which is what its
// documentation now says.
func TestAllowScaleUpDoesWhatItSays(t *testing.T) {
	plain := Compose(Input{HTML: `<p>hi</p>`}, Options{AllowScaleUp: true})
	if plain.Scale != 1 {
		t.Errorf("an ordinary document was scaled by %v; it fills the width already", plain.Scale)
	}
	card := Compose(Input{HTML: `<html style="width: 100px"><body style="margin: 0"><p>hi</p></body></html>`},
		Options{AllowScaleUp: true})
	if card.Scale <= 1 {
		t.Errorf("a document a hundred pixels wide was scaled by %v; it asked to be enlarged", card.Scale)
	}
	off := Compose(Input{HTML: `<html style="width: 100px"><body style="margin: 0"><p>hi</p></body></html>`},
		Options{})
	if off.Scale != 1 {
		t.Errorf("without the option the document was scaled by %v", off.Scale)
	}
}

// TestAControlReadsItsSizeTheWayHTMLDoes (audit C135): cols, rows and size are
// read by HTML's rules for parsing non-negative integers, as a canvas's width
// already was — "40px" is forty and "30.5" is thirty — and a number too large
// for any page is clamped and reported, not quietly the default.
func TestAControlReadsItsSizeTheWayHTMLDoes(t *testing.T) {
	for _, tc := range []struct {
		markup string
		chars  int
		lines  int
		limit  bool
	}{
		{`<textarea cols="40px" rows="3.5"></textarea>`, 40, 3, false},
		{`<textarea cols=" 7" rows="+2"></textarea>`, 7, 2, false},
		{`<input size="30.5">`, 30, 1, false},
		{`<input size="0">`, defaultInputSize, 1, false},
		{`<input size="-4">`, defaultInputSize, 1, false},
		{`<textarea cols="99999999999999999999"></textarea>`, maxControlChars, defaultControlRows, true},
		{`<textarea cols="` + strconv.Itoa(maxControlChars+1) + `"></textarea>`,
			maxControlChars, defaultControlRows, true},
		{`<select size="3px"><option>a<option>b<option>c<option>d</select>`, 0, 3, false},
		{`<select size="1.5"><option>a<option>b</select>`, 0, 1, false},
	} {
		built := Build(Input{HTML: tc.markup})
		var c *Control
		var walk func(*Box)
		walk = func(b *Box) {
			if b.Control != nil && c == nil {
				c = b.Control
			}
			for _, k := range b.Children {
				walk(k)
			}
		}
		walk(built.Root)
		if c == nil {
			t.Errorf("%s: no control", tc.markup)
			continue
		}
		if c.Chars != tc.chars || c.Lines != tc.lines {
			t.Errorf("%s: %d characters by %d lines, want %d by %d",
				tc.markup, c.Chars, c.Lines, tc.chars, tc.lines)
		}
		if got := hasRule(built.Findings, RuleLimit); got != tc.limit {
			t.Errorf("%s: limit reported %v, want %v: %v", tc.markup, got, tc.limit, built.Findings)
		}
	}
}

// TestACanvasTooLargeToLayOutIsReported: a canvas's width is read by the same
// rules, and one no length here can hold is laid out at the default size with
// that said, where it was silent.
func TestACanvasTooLargeToLayOutIsReported(t *testing.T) {
	built := Build(Input{HTML: `<canvas width="99999999999"></canvas>`})
	if !hasRule(built.Findings, RuleLimit) {
		t.Errorf("a canvas too large to lay out was not reported: %v", built.Findings)
	}
	if built := Build(Input{HTML: `<canvas width="80px"></canvas>`}); len(built.Findings) != 0 {
		t.Errorf("an ordinary canvas was reported: %v", built.Findings)
	}
}

// TestAnSVGsDeclaredEntitiesAreNotExpanded is svg.go's claim about entities
// (audit C140, which found the comment saying the opposite): a reference to
// an entity the document declares is not expanded, so it cannot be a billion
// laughs, and in an attribute it is a value the reader refuses.
func TestAnSVGsDeclaredEntitiesAreNotExpanded(t *testing.T) {
	body := `<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY c "green">]>` +
		`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` +
		`<rect width="100%" height="100%" fill="&c;"/></svg>`
	if got := svgContent([]byte(body), svgAsImage); got != nil {
		t.Errorf("a declared entity was expanded into %v", got.Solid)
	}
}

// TestASelectsSizeIsItsDisplaySize: a <select> whose size says three is a list
// box showing its options, by the same reading as its rows — HTML's display
// size — and not a drop-down showing one.
func TestASelectsSizeIsItsDisplaySize(t *testing.T) {
	built := Build(Input{HTML: `<select size="3px"><option>aa<option selected>bb<option>cc</select>`})
	box := boxFor(built.Root, "select")
	if box == nil {
		t.Fatal("no select box")
	}
	if got := textIn(box); !strings.Contains(got, "aa") || !strings.Contains(got, "cc") {
		t.Errorf("a select of size 3 shows %q; a list box shows every option", got)
	}
}
