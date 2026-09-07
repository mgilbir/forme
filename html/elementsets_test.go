package html

import (
	"strings"
	"testing"
)

// What this package refuses, and what it merely does not recognise.
//
// The two were run together in the comments for a while, and they are not the
// same decision. An element outside knownElements is one nothing here has a rule
// about, and it is laid out as the ordinary inline HTML gives it; an element in
// droppedElements is one this engine will not render, each for a stated reason.
// These pin both, so that a comment describing one of them cannot go quietly out
// of date again.

// elementsIn is every element name in a parsed tree, in document order.
func elementsIn(doc *Node) []string {
	var out []string
	doc.Walk(func(n *Node) bool {
		if n.Type == ElementNode {
			out = append(out, n.Name)
		}
		return true
	})
	return out
}

// holds reports whether a name is in the list.
func holds(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// TestAnUnknownElementIsKept. A custom element's box is an ordinary inline one
// and a stylesheet may select it: dropping it lost every rule the author wrote.
func TestAnUnknownElementIsKept(t *testing.T) {
	for _, name := range []string{"fancy-callout", "my-widget", "sep", "inline-block"} {
		doc, errs, ok := Parse("<p>before<" + name + ">x</" + name + ">after</p>")
		if !ok || len(errs) != 0 {
			t.Errorf("<%s>: ok=%v with %d findings; an element nobody has heard "+
				"of is an element all the same", name, ok, len(errs))
		}
		if got := elementsIn(doc); !holds(got, name) {
			t.Errorf("<%s> is not in the tree: %v", name, got)
		}
		if text := textIn(doc); !strings.Contains(text, "x") {
			t.Errorf("<%s>: its content is gone; the text is %q", name, text)
		}
	}
}

// TestEveryDroppedElementIsDroppedAndReported is the other side, over the whole
// list rather than over the three names a comment happens to mention.
func TestEveryDroppedElementIsDroppedAndReported(t *testing.T) {
	for name, why := range droppedElements {
		src := "<div>before<" + name + ">"
		if !voidElements[name] {
			src += "x</" + name + ">"
		}
		src += "after</div>"

		doc, errs, ok := Parse(src)
		if got := elementsIn(doc); holds(got, name) {
			t.Errorf("<%s> is in the tree although it is dropped (%s): %v",
				name, why, got)
		}
		if len(errs) == 0 {
			t.Errorf("<%s> was dropped in silence; a renderer that ignores an "+
				"element quietly is one the author cannot tell read it", name)
		}
		// Dropping is a refusal, and a refusal is what ok reports.
		if ok {
			t.Errorf("<%s> was dropped and ok is still true", name)
		}
		if text := textIn(doc); !strings.Contains(text, "before") ||
			!strings.Contains(text, "after") {
			t.Errorf("<%s> took the text around it with it: %q", name, text)
		}
	}
}

// TestAReplacedElementKeepsItsBoxEvenWithNothingInIt.
//
// <iframe> and <object> were in droppedElements and are not: the browsing
// context and the plugin are refused and always will be, but the *box* is on the
// page whether or not anything was loaded into it, and dropping the element
// threw the box away with the thing it could not run. Twenty-seven reftests
// passed for the wrong reason while an iframe made no box at all.
func TestAReplacedElementKeepsItsBoxEvenWithNothingInIt(t *testing.T) {
	for _, name := range []string{"iframe", "object"} {
		if droppedElements[name] != "" {
			t.Errorf("<%s> is in droppedElements; its box is on the page even "+
				"with nothing in it", name)
		}
		// And known, which is what the package's own documentation says of it:
		// an unknown element is kept too, so its presence in the tree does not
		// tell the two apart.
		if !knownElements[name] {
			t.Errorf("<%s> is not in knownElements; a replaced element's box is "+
				"the reason it is there", name)
		}
		doc, _, _ := Parse(`<p><` + name + `></` + name + `></p>`)
		if got := elementsIn(doc); !holds(got, name) {
			t.Errorf("<%s> is not in the tree: %v", name, got)
		}
	}
	// The content of an iframe is the fallback a browser without frames would
	// show, and a browser with them never renders it.
	doc, _, _ := Parse(`<p><iframe>fallback</iframe></p>`)
	if text := textIn(doc); strings.Contains(text, "fallback") {
		t.Errorf("an iframe's content was laid out: %q", text)
	}
}

// textIn is all the text of a tree.
func textIn(doc *Node) string {
	var b strings.Builder
	doc.Walk(func(n *Node) bool {
		if n.Type == TextNode {
			b.WriteString(n.Text)
		}
		return true
	})
	return b.String()
}
