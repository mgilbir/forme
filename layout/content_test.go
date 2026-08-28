package layout

import (
	"strings"
	"testing"
)

// Generated content.
//
// These are the only boxes in the tree that no element generated and no
// anonymous-box rule required: they come from a declaration, which makes this
// the one place the stylesheet adds to the document rather than describing it.

// TestGeneratedContentBracketsTheChildren pins where the two boxes go. ::before
// and ::after surround the element's own content rather than replacing it, and
// an engine that appended both would put the marker after the text.
func TestGeneratedContentBracketsTheChildren(t *testing.T) {
	got := bodyBoxes(t, `<p>middle</p>`,
		`p::before { content: "[" } p::after { content: "]" }`)
	want := `p block
  p inline
    text "["
  text "middle"
  p inline
    text "]"
`
	if got != want {
		t.Errorf("got:\n%swant:\n%s", got, want)
	}
}

// TestNoContentGeneratesNothing pins that a pseudo-element with nothing to say
// produces no box at all, which is not the same as producing an empty one. An
// empty inline still contributes a line box, so a list of items each preceded by
// an invisible one would be spaced as though something were there.
func TestNoContentGeneratesNothing(t *testing.T) {
	for _, sheet := range []string{
		`p::before { content: none }`,
		`p::before { content: normal }`,
		`p::before { color: red }`, // styled, but says no content
	} {
		got := bodyBoxes(t, `<p>x</p>`, sheet)
		want := `p block
  text "x"
`
		if got != want {
			t.Errorf("%s\ngot:\n%swant:\n%s", sheet, got, want)
		}
	}

	// And a pseudo-element nothing mentions costs nothing either.
	got := bodyBoxes(t, `<p>x</p>`)
	if strings.Count(got, "p ") != 1 {
		t.Errorf("an unstyled element generated pseudo-element boxes:\n%s", got)
	}
}

// TestContentIsWhiteSpaceProcessed pins that a content string goes through CSS
// Text §4 exactly as document text does.
//
// This test used to assert the opposite — that the string was taken exactly as
// written, on the reasoning that the author had typed the characters they wanted
// and "content: '  '" was a way to indent a marker. That reasoning is wrong and
// the test was pinning the bug: generated content is put in an anonymous inline
// box, and nothing exempts that box from the white-space processing every other
// inline gets. The suite says so directly in
// css/CSS2/generated-content/content-white-space-004, which puts tabs and
// newlines in a content string and requires it to set identically to the same
// words written in the markup.
func TestContentIsWhiteSpaceProcessed(t *testing.T) {
	// Under the initial value a run of spaces is one space, a segment break is a
	// space, and the leading and trailing spaces survive Phase I — where they go
	// is the line breaker's question and not this one.
	got := bodyBoxes(t, `<p>x</p>`, `p::before { content: "  a  b  " }`)
	if !strings.Contains(got, `text " a b "`) {
		t.Errorf("the content string was not collapsed:\n%s", got)
	}
	got = bodyBoxes(t, `<p>x</p>`, `p::before { content: "a\A b" }`)
	if !strings.Contains(got, `text "a b"`) {
		t.Errorf("the segment break did not become a space:\n%s", got)
	}

	// And "white-space: pre" keeps every one of them, which is the half that
	// makes the rule a rule rather than an unconditional collapse.
	got = bodyBoxes(t, `<p>x</p>`, `p::before { content: "  a  b  "; white-space: pre }`)
	if !strings.Contains(got, `text "  a  b  "`) {
		t.Errorf("white-space: pre did not preserve the string:\n%s", got)
	}
	got = bodyBoxes(t, `<p>x</p>`, `p::before { content: "a\A b"; white-space: pre }`)
	if !strings.Contains(got, "text \"a\\nb\"") {
		t.Errorf("white-space: pre did not preserve the segment break:\n%s", got)
	}
}

// TestContentConcatenates pins that a value of several parts becomes one string,
// which is how attr() is used with punctuation around it.
func TestContentConcatenates(t *testing.T) {
	got := bodyBoxes(t, `<p>x</p>`, `p::before { content: "(" "a" ")" }`)
	if !strings.Contains(got, `text "(a)"`) {
		t.Errorf("the parts were not joined:\n%s", got)
	}
}

// TestContentAttr pins attr(), which is the one function this engine produces.
// A missing attribute contributes the empty string, which is what makes attr()
// safe to use for data that is sometimes there.
func TestContentAttr(t *testing.T) {
	got := bodyBoxes(t, `<p data-label="Note">x</p>`,
		`p::before { content: attr(data-label) ": " }`)
	if !strings.Contains(got, `text "Note: "`) {
		t.Errorf("attr() did not produce the attribute:\n%s", got)
	}

	// A missing attribute is the empty string, so only the literal remains.
	got = bodyBoxes(t, `<p>x</p>`, `p::before { content: attr(data-missing) ">" }`)
	if !strings.Contains(got, `text ">"`) {
		t.Errorf("a missing attribute did not contribute the empty string:\n%s", got)
	}
}

// TestAttrMatchesTheCaseTheDocumentLanguageDoes is §12.2's one question about
// attr() that has two answers.
//
// HTML lowercases an attribute name as it is parsed, so "attr(Title)" and
// "attr(title)" are the same request and both find the attribute. XML does not,
// so the two are different requests and only one of them finds anything. The
// suite writes the same document in both languages to say so:
// content-attr-case-001's assert is "the attr(x) function selects the attribute
// even when case does not match" and content-attr-case-002's is "in XHTML that
// attr(x) does not select the attribute when the case does not match".
//
// The XHTML fixture is recognised the way this engine recognises one at all —
// the xmlns declaration, which is what looksLikeXML reads — because no content
// type reaches it.
func TestAttrMatchesTheCaseTheDocumentLanguageDoes(t *testing.T) {
	const sheet = `p::before { content: "[" attr(Title) "]" }`
	got := bodyBoxes(t, `<p title="yes">x</p>`, sheet)
	if !strings.Contains(got, `text "[yes]"`) {
		t.Errorf("in HTML, attr(Title) did not select the title attribute:\n%s", got)
	}
	got = bodyBoxes(t, `<html xmlns="http://www.w3.org/1999/xhtml">`+
		`<body><p title="yes">x</p></body></html>`, sheet)
	if !strings.Contains(got, `text "[]"`) {
		t.Errorf("in XHTML, attr(Title) selected something; the name is "+
			"case-sensitive there:\n%s", got)
	}
	// And the name written as the document writes it still selects it, in both.
	for _, doc := range []string{
		`<p title="yes">x</p>`,
		`<html xmlns="http://www.w3.org/1999/xhtml"><body><p title="yes">x</p></body></html>`,
	} {
		got := bodyBoxes(t, doc, `p::before { content: "[" attr(title) "]" }`)
		if !strings.Contains(got, `text "[yes]"`) {
			t.Errorf("attr(title) did not select it:\n%s", got)
		}
	}
}

// TestUnproducibleContentIsReported pins that content this engine cannot make is
// named rather than dropped. A marker that silently failed to appear leaves a
// page that still looks finished.
func TestUnproducibleContentIsReported(t *testing.T) {
	cases := map[string]string{
		// counter(), counters() and the quote keywords are produced now and so
		// are not here. Nor is a counter function with the wrong arguments, which
		// used to be: that is not content this engine cannot make, it is not CSS,
		// and the cascade now drops the declaration so that an earlier one stands
		// — see TestAMalformedCounterFunctionDropsTheDeclaration next door in
		// style. resolveContent still refuses it, because a computed style can be
		// built by hand and the initial value travels the same path, but nothing
		// a stylesheet can write reaches that refusal any more.
		// An identifier that is not one of the keywords the property defines.
		`p::before { content: elephant }`: "elephant",
	}
	for sheet, want := range cases {
		got := build(t, `<p>x</p>`, sheet)

		var found *Finding
		for i := range got.Findings {
			if got.Findings[i].Property == "content" {
				f := got.Findings[i]
				found = &f
			}
		}
		if found == nil {
			t.Errorf("%s was dropped silently: %v", sheet, got.Findings)
			continue
		}
		if found.Rule != RuleUnsupportedValue {
			t.Errorf("%s was reported as %s, want an unsupported value", sheet, found.Rule)
		}
		if !strings.Contains(strings.ToLower(found.Message), want) {
			t.Errorf("%s reported %q, which does not say it was a %s", sheet, found.Message, want)
		}
	}
}

// An image in generated content, which is a picture in a line and not a word.
//
// "content: url(x.png)" was refused and reported — "an image in generated
// content needs a resolver" — and the resolver it named was there all along: the
// one that fetches every <img>, every background and every list marker. What was
// missing was a *box*, because the value was a string and a string cannot hold a
// picture.
//
// The value is a list of pieces now. "content: counter(n) url(x.png) 'Before'"
// is a number, a picture and a word, in that order, and the picture is a
// replaced inline box between two runs of text.

// TestAnImageInGeneratedContentIsDrawn.
func TestAnImageInGeneratedContentIsDrawn(t *testing.T) {
	res := mapResolver{"mark.png": encodePNG(t, 20, 10)}
	ops := paintWith(t, res, `<p id=p>x</p>`,
		`p::before { content: url(mark.png) } p { font-size: 0 }`)
	var drawn []Rect
	for _, op := range ops {
		if d, ok := op.(DrawImage); ok {
			drawn = append(drawn, d.Rect)
		}
	}
	if len(drawn) != 1 {
		t.Fatalf("%d images drawn, want 1", len(drawn))
	}
	if drawn[0].W != bgpx(20) || drawn[0].H != bgpx(10) {
		t.Errorf("the picture is %v by %v, want its own 20 by 10",
			drawn[0].W, drawn[0].H)
	}
}

// TestAnImageAndItsWordsKeepTheirOrder is what the list of pieces is for: a
// declaration naming both puts them on the line in the order it named them.
func TestAnImageAndItsWordsKeepTheirOrder(t *testing.T) {
	res := mapResolver{"mark.png": encodePNG(t, 20, 10)}
	ops := paintWith(t, res, `<p id=p>x</p>`,
		`p::before { content: "A" url(mark.png) "B" }
		 p { font-family: Courier; font-size: 20px }`)
	var xs []float64
	var kinds []string
	for _, op := range ops {
		switch v := op.(type) {
		case DrawText:
			if v.Text == "A" || v.Text == "B" {
				xs, kinds = append(xs, v.At.X.Px()), append(kinds, v.Text)
			}
		case DrawImage:
			xs, kinds = append(xs, v.Rect.X.Px()), append(kinds, "img")
		}
	}
	if len(kinds) != 3 {
		t.Fatalf("the line holds %v, want the word, the picture and the word", kinds)
	}
	// Sorted by where they sit rather than by when they were painted: a picture
	// is drawn in a later stacking layer than the text around it, so paint order
	// says nothing about the order on the line.
	for i := range xs {
		for j := i + 1; j < len(xs); j++ {
			if xs[j] < xs[i] {
				xs[i], xs[j] = xs[j], xs[i]
				kinds[i], kinds[j] = kinds[j], kinds[i]
			}
		}
	}
	if kinds[0] != "A" || kinds[1] != "img" || kinds[2] != "B" {
		t.Errorf("left to right the line holds %v at %v, want [A img B]", kinds, xs)
	}
}

// TestAnImageInGeneratedContentThatCannotBeReadIsReported. It is a blocked
// resource, which is what an <img> naming a missing file is, and not an
// unsupported value — the value is one this engine produces.
func TestAnImageInGeneratedContentThatCannotBeReadIsReported(t *testing.T) {
	got := build(t, `<p>x</p>`, `p::before { content: url(mark.png) }`)
	var found *Finding
	for i := range got.Findings {
		if got.Findings[i].Property == "content" {
			f := got.Findings[i]
			found = &f
		}
	}
	if found == nil {
		t.Fatalf("a picture that could not be read was dropped silently: %v", got.Findings)
	}
	if found.Rule != RuleResourceBlocked {
		t.Errorf("reported as %s, want a blocked resource", found.Rule)
	}
}

// TestAnEmptyReferenceNamesNothing. url("") is an <img> with an empty src: it
// names no file, so there is no reference for a resolver to have refused and
// nothing to report. Treating it as a file gives a page a broken picture and a
// finding, both about a document that asked for neither.
func TestAnEmptyReferenceNamesNothing(t *testing.T) {
	got := build(t, `<p id=p>x</p>`, `p::before { content: "A" url("") "B" }`)
	for _, f := range got.Findings {
		if f.Property == "content" {
			t.Errorf("url(\"\") was reported: %s", f.Message)
		}
	}
	ops := paintOf(t, `<p id=p>x</p>`,
		`p::before { content: "A" url("") "B" } p { font-family: Courier; font-size: 20px }`)
	for _, op := range ops {
		if _, ok := op.(DrawImage); ok {
			t.Error("a picture was drawn for a reference that names nothing")
		}
	}
	// And the words either side of it are one run rather than two, which is the
	// half that says the reference was passed over rather than recorded as a
	// piece that produces nothing: a piece between them would end the first run
	// and begin a second, and the page would be the same but the line would not.
	root := layoutOf(t, 600, `<p id="p">x</p>`,
		`p::before { content: "A" url("") "B" } p { font-family: Courier; font-size: 20px }`)
	var runs []string
	for _, l := range find(t, root, "p").Lines {
		for _, r := range l.Runs {
			runs = append(runs, r.Text)
		}
	}
	if len(runs) != 2 || runs[0] != "AB" {
		t.Errorf("the line holds %q, want the two words as one run and the "+
			"element's own text", runs)
	}
}

// TestPseudoElementInheritsFromItsOwner pins the inheritance that makes
// "p { color: red } p::before { content: '>' }" draw a red marker without the
// author saying so twice. It inherits from the element it belongs to, not from
// that element's parent.
func TestPseudoElementInheritsFromItsOwner(t *testing.T) {
	built := build(t, `<div><p>x</p></div>`,
		`div { color: rgb(1, 1, 1) } p { color: rgb(2, 2, 2) } p::before { content: ">" }`)

	var before *Box
	var walk func(*Box)
	walk = func(b *Box) {
		if before != nil {
			return
		}
		if b.Element != nil && b.Element.Name == "p" && len(b.Children) > 0 &&
			b.Children[0].IsText() && b.Children[0].Text == ">" {
			before = b
			return
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	if before == nil {
		t.Fatal("the generated box was not found")
	}
	if got := before.Style["color"]; got != "rgb(2, 2, 2)" {
		t.Errorf("the marker's colour is %q; it inherits from the <p>, not the <div>", got)
	}
}

// TestPseudoElementRulesDoNotStyleTheElement pins the separation the other way.
// A rule with a pseudo-element styles that and nothing else, and one without
// never styles a pseudo-element — getting it backwards gives every ::before its
// element's whole style twice over, and gives the element the marker's.
func TestPseudoElementRulesDoNotStyleTheElement(t *testing.T) {
	built := build(t, `<p>x</p>`,
		`p::before { content: ">"; color: rgb(9, 9, 9) } p { color: rgb(1, 1, 1) }`)

	var p, before *Box
	var walk func(*Box)
	walk = func(b *Box) {
		if b.Element != nil && b.Element.Name == "p" {
			if len(b.Children) > 0 && b.Children[0].IsText() && b.Children[0].Text == ">" {
				before = b
			} else if p == nil {
				p = b
			}
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	if p == nil || before == nil {
		t.Fatalf("boxes not found: p=%v before=%v", p != nil, before != nil)
	}
	if p.Style["color"] != "rgb(1, 1, 1)" {
		t.Errorf("the element took the marker's colour: %q", p.Style["color"])
	}
	if before.Style["color"] != "rgb(9, 9, 9)" {
		t.Errorf("the marker did not take its own colour: %q", before.Style["color"])
	}
	// And the element has no content of its own from the pseudo-element's rule.
	if p.Style["content"] != "normal" {
		t.Errorf("the element's content is %q; the ::before rule set it", p.Style["content"])
	}
}

// TestGeneratedContentCanBeABlock pins that display applies to a pseudo-element
// like anything else, so a generated box can take a line of its own.
func TestGeneratedContentCanBeABlock(t *testing.T) {
	got := bodyBoxes(t, `<p>x</p>`,
		`p::before { content: "above"; display: block }`)
	if !strings.Contains(got, "p block\n  p block\n") {
		t.Errorf("a block ::before did not take a line of its own:\n%s", got)
	}

	// display:none on a pseudo-element produces nothing, as it does anywhere.
	got = bodyBoxes(t, `<p>x</p>`, `p::before { content: "gone"; display: none }`)
	if strings.Contains(got, "gone") {
		t.Errorf("display:none on a ::before still generated a box:\n%s", got)
	}
}

// TestResolveContentRefusesAMalformedCounterCall is the fixture for the two
// refusals in resolveContent that no stylesheet can reach any more.
//
// §12.2's grammar for the counter functions is checked where the sheet is
// prepared, and a call with the wrong arguments takes the whole declaration with
// it — so the reader can only meet one through the door this test uses, which is
// a computed value built by hand. That door is real: the initial value travels
// the same path, and this package's own tests build styles directly.
//
// Without a fixture the refusals would be a guard nobody had ever seen decide
// anything, which is worse than no guard because it reads as defence.
func TestResolveContentRefusesAMalformedCounterCall(t *testing.T) {
	for _, raw := range []string{
		`counter()`,
		`counters(c)`,
	} {
		got := resolveContent(raw, nil, nil, quoteList{}, 0)
		if got.unsupported == "" {
			t.Errorf("%q was read as %q rather than refused", raw, got.text())
		}
	}

	// And the calls that are legal still produce, so the refusal is not a
	// blanket one. The counter that was never created reads as zero, which is
	// §12.4.3's own answer.
	if got := resolveContent(`counter(c)`, nil, nil, quoteList{}, 0); got.text() != "0" {
		t.Errorf("counter(c) produced %q, %q", got.text(), got.unsupported)
	}
	if got := resolveContent(`counters(c, ".")`, nil, nil, quoteList{}, 0); got.unsupported != "" {
		t.Errorf(`counters(c, ".") was refused: %s`, got.unsupported)
	}
}
