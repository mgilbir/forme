package html

import (
	"strings"
	"testing"
	"time"
)

// Foreign content — SVG and MathML — and why its subtree is skipped rather than
// parsed on.
//
// An unknown *HTML* element is dropped and its content parsed on, and that is
// right: the content is HTML, a browser shows it, and a <fancy-callout> that has
// lost its box has not lost its words. A foreign element is the opposite case.
// Its children mean nothing to an HTML layout and their text is not text of the
// document, so parsing on splices it into the flow around it.
//
// That is what "<svg><text>x</text></svg>" did: an x in the surrounding
// paragraph, in the paragraph's font, on the paragraph's baseline, nowhere near
// where the picture would have been. Worse than the missing picture, because a
// hole is visibly a hole and a stray letter reads as the document's own.

// textOf returns the document's text content, joined, which is what would reach
// a page.
func textOf(n *Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*Node)
	walk = func(n *Node) {
		if n.Type == TextNode {
			b.WriteString(n.Text)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// TestForeignContentDoesNotReachTheFlow is the bug.
func TestForeignContentDoesNotReachTheFlow(t *testing.T) {
	for _, tc := range []struct {
		what string
		src  string
	}{
		{"an svg with text in it",
			`<p>before</p><svg width="10"><text x="0" y="9">LEAK</text></svg><p>after</p>`},
		{"nested svg",
			`<p>before</p><svg><svg><text>LEAK</text></svg><text>LEAK</text></svg><p>after</p>`},
		{"mathml",
			`<p>before</p><math><mi>LEAK</mi></math><p>after</p>`},
		{"an svg holding what looks like html",
			`<p>before</p><svg><foreignObject><div>LEAK</div></foreignObject></svg><p>after</p>`},
	} {
		doc, _, _ := Parse(tc.src)
		got := textOf(doc)
		if strings.Contains(got, "LEAK") {
			t.Errorf("%s: the document's text is %q; foreign content reached the flow",
				tc.what, got)
		}
		// The document around it must be untouched — skipping a subtree must not
		// swallow what follows it.
		if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
			t.Errorf("%s: the document's text is %q; the skip took its neighbours "+
				"with it", tc.what, got)
		}
	}
}

// TestAnUnknownHTMLElementKeepsItsContent is the other half of the distinction,
// and the reason this is not simply "drop the subtree of anything unknown".
//
// A custom element is HTML. A browser lays it out as an inline box and shows its
// text, and so does this once the element itself is dropped — the box is lost
// and reported, the words are not. Skipping its content would be a regression
// dressed as a fix.
func TestAnUnknownHTMLElementKeepsItsContent(t *testing.T) {
	doc, _, _ := Parse(`<p>a</p><my-widget>KEPT</my-widget><p>b</p>`)
	if got := textOf(doc); !strings.Contains(got, "KEPT") {
		t.Errorf("the document's text is %q; a custom element's words are HTML and "+
			"a browser shows them", got)
	}
}

// TestAForeignElementIsReportedAndSaysWhy. The finding has to name the second
// half — that the content went too — because "not an element this engine lays
// out" would leave an author looking for their SVG's text somewhere on the page.
func TestAForeignElementIsReportedAndSaysWhy(t *testing.T) {
	_, errs, _ := Parse(`<svg><text>x</text></svg>`)
	var found *Error
	for i := range errs {
		if strings.Contains(errs[i].Message, "<svg>") {
			found = &errs[i]
		}
	}
	if found == nil {
		t.Fatalf("an svg was skipped with no report: %v", errs)
	}
	if !found.Unsupported {
		t.Errorf("the report is %q, marked as malformed markup; it is correct HTML "+
			"this engine does not lay out", found.Message)
	}
	if !strings.Contains(found.Message, "content") {
		t.Errorf("the report is %q and does not say the content went with it",
			found.Message)
	}
	// And the children are not reported one by one: the subtree was skipped, so
	// nothing in it was ever read as an element.
	for _, e := range errs {
		if strings.Contains(e.Message, "<text>") {
			t.Errorf("a child of the skipped subtree was reported: %q", e.Message)
		}
	}
}

// TestASelfClosingForeignElementSkipsNothing. HTML has no self-closing syntax for
// its own elements, but a foreign element really does — "<svg/>" is an empty
// svg, and there is no end tag to skip to. Skipping anyway would consume the
// rest of the document.
func TestASelfClosingForeignElementSkipsNothing(t *testing.T) {
	doc, _, _ := Parse(`<p>before</p><svg/><p>after</p>`)
	got := textOf(doc)
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("the document's text is %q; a self-closing svg swallowed what "+
			"followed it", got)
	}
}

// TestAnUnclosedForeignElementEndsAtTheDocument. A document that opens an <svg>
// and never closes it is malformed, and the skip must terminate anyway rather
// than run off the end looking for a tag that is not there.
func TestAnUnclosedForeignElementEndsAtTheDocument(t *testing.T) {
	done := make(chan string, 1)
	go func() {
		doc, _, _ := Parse(`<p>before</p><svg><text>LEAK`)
		done <- textOf(doc)
	}()
	select {
	case got := <-done:
		if strings.Contains(got, "LEAK") {
			t.Errorf("the document's text is %q; the unclosed subtree reached the flow", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parsing an unclosed <svg> did not terminate")
	}
}
