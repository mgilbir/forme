package html

import (
	"strings"
	"testing"
)

// What an end tag does to the elements still open inside it.
//
// HTML's rule for a block's end tag is to generate the implied end tags, report
// a parse error if the current node is not the one being closed, and then "pop
// elements from the stack of open elements until an element with the same tag
// name has been popped". The error is a message to the author; the popping
// happens either way.
//
// This parser reported and then left the stack alone, which is the strict-XML
// reading. An unclosed <span> inside a <div> therefore escaped the </div> and
// swallowed the rest of the document: everything after it was a descendant of
// the span, inheriting its font, its colour and its size.
//
// css-text/tab-size/tab-size-integer-004 is the fixture that showed it. It
// writes "<div><span>&#09;X<span></div>" twice, deliberately leaving spans open,
// and its second line came out set in 128px — two ems of two ems of two ems.

// TestAnEndTagClosesWhatIsOpenInsideIt is the bug.
func TestAnEndTagClosesWhatIsOpenInsideIt(t *testing.T) {
	got := body(t, `<div><span>a<span></div><div>b</div>`)
	want := `<div>
  <span>
    "a"
    <span>
<div>
  "b"
`
	if got != want {
		t.Errorf("the body is\n%s\nwant\n%s", got, want)
	}
}

// TestTheRestOfTheDocumentIsNotSwallowed, which is the consequence and the one
// an author sees: the second div is a sibling of the first, not a descendant of
// something inside it.
func TestTheRestOfTheDocumentIsNotSwallowed(t *testing.T) {
	doc, _, _ := Parse(`<div><span>a<span></div><div id="after">b</div>`)
	el := doc.Element("body")
	if el == nil {
		t.Fatal("no body")
	}
	if n := len(el.Children); n != 2 {
		t.Errorf("the body has %d children, want two divs:\n%s", n, tree(el))
	}
}

// TestItIsStillReported. The popping is the recovery; the message is what tells
// the author their markup does not nest, and it must not be lost with the
// refusal.
func TestItIsStillReported(t *testing.T) {
	_, errs, ok := Parse(`<div><span>a</div>`)
	if ok {
		t.Errorf("the document was accepted, and its tags do not nest")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "tags have to nest") {
			found = true
			if e.Unsupported {
				t.Errorf("the finding is marked unsupported: %q", e.Message)
			}
		}
	}
	if !found {
		t.Errorf("nothing said the tags do not nest: %v", errs)
	}
}

// TestAnOptionalEndTagIsStillSilent is the containment argument. The elements
// HTML lets end without a tag of their own are closed by this path too, and
// closing one is not a mistake — a document full of <li> without </li> must not
// be full of findings.
func TestAnOptionalEndTagIsStillSilent(t *testing.T) {
	for _, src := range []string{
		`<ul><li>a<li>b</ul>`,
		`<table><tr><td>a<td>b</tr></table>`,
		`<div><p>a</div>`,
	} {
		_, errs, ok := Parse(src)
		if !ok || len(errs) != 0 {
			t.Errorf("%q was reported: %v", src, errs)
		}
	}
}

// TestNestedElementsOfTheSameNameCloseTheInnermost, which is what "until an
// element with the same tag name has been popped" means: one </div> closes one
// div, not all of them.
func TestNestedElementsOfTheSameNameCloseTheInnermost(t *testing.T) {
	got := body(t, `<div id="outer"><div id="inner">a</div>b</div>`)
	want := `<div> id="outer"
  <div> id="inner"
    "a"
  "b"
`
	if got != want {
		t.Errorf("the body is\n%s\nwant\n%s", got, want)
	}
}

// TestAnEndTagForNothingOpenStillCloses nothing. The stack is searched first,
// and a name that is not on it leaves it exactly as it was — otherwise a stray
// "</div>" in the middle of a document would tear down whatever it landed in.
func TestAnEndTagForNothingOpenStillClosesNothing(t *testing.T) {
	got := body(t, `<span>a</div>b</span>`)
	// One text node, because the stray tag added nothing between the two halves.
	want := `<span>
  "ab"
`
	if got != want {
		t.Errorf("the body is\n%s\nwant\n%s", got, want)
	}
}
