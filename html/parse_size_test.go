package html

import (
	"strings"
	"testing"
)

// TestATreeComesBackFromEveryDocument is the guard on Parse's own contract.
//
// "The tree is returned even when ok is false" is the sentence a caller reads
// before deciding whether to check the pointer, and one input in the language
// broke it: a document over the byte cap returned nil. The caller two packages
// away that reports every finding to an author read that finding and then
// dereferenced the tree, so the largest documents were the ones that crashed
// instead of being refused.
//
// The oversize case is the one that was wrong; the rest are here because the
// contract is about every document, and a table is how the next one gets added.
func TestATreeComesBackFromEveryDocument(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"nothing at all", ""},
		{"one byte over the cap", strings.Repeat("a", maxInputBytes+1)},
		{"deeper than the depth cap", strings.Repeat("<div>", maxDepth+50)},
		{"more nodes than the node cap", strings.Repeat("<b>x</b>", 200000)},
		{"a comment that never closes", "<!--"},
		{"markup that is only refusals", strings.Repeat("<", 1000)},
	} {
		doc, _, _ := Parse(tc.src)
		if doc == nil {
			t.Errorf("%s: Parse returned no tree, which its own documentation forbids", tc.name)
			continue
		}
		if doc.Type != DocumentNode {
			t.Errorf("%s: the root is a %v, want a document node", tc.name, doc.Type)
		}
	}
}

// TestADocumentOverTheCapIsTheEmptyFrame says what that tree is: none of the
// document was read, so what comes back is what an empty document parses to,
// with one finding naming the cap.
func TestADocumentOverTheCapIsTheEmptyFrame(t *testing.T) {
	src := strings.Repeat("<p>text that will never be read</p>", (maxInputBytes/35)+1)
	if len(src) <= maxInputBytes {
		t.Fatalf("the fixture is %d bytes, which is inside the cap of %d", len(src), maxInputBytes)
	}
	doc, errs, ok := Parse(src)
	if ok {
		t.Fatal("a document over the byte cap was accepted")
	}
	if doc == nil {
		t.Fatal("a document over the byte cap produced no tree")
	}
	if len(errs) != 1 {
		t.Fatalf("a document over the byte cap produced %d findings, want the one naming the cap: %v",
			len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "larger than this engine will read") {
		t.Errorf("the finding is %q, want the one naming the cap", errs[0].Message)
	}
	if got := doc.TextContent(); got != "" {
		t.Errorf("the tree carries %d bytes of text; none of the document was read", len(got))
	}

	empty, _, _ := Parse("")
	if got, want := shapeOf(doc), shapeOf(empty); got != want {
		t.Errorf("the tree is %s, want the frame an empty document parses to, %s", got, want)
	}
}

// shapeOf spells a tree's element structure, which is all this comparison is
// about.
func shapeOf(n *Node) string {
	var b strings.Builder
	var walk func(*Node)
	walk = func(n *Node) {
		switch n.Type {
		case ElementNode:
			b.WriteString("<" + n.Name + ">")
		case TextNode:
			b.WriteString("#text")
		default:
			b.WriteString("#doc")
		}
		if len(n.Children) > 0 {
			b.WriteString("(")
			for _, c := range n.Children {
				walk(c)
			}
			b.WriteString(")")
		}
	}
	walk(n)
	return b.String()
}

// TestEveryBoundSaysItIsABound is the third kind of finding this parser can
// raise, and the one it used not to have.
//
// Malformed markup is the author's to fix and an unsupported element is the
// engine's; a bound that was reached is neither. The document is correct, the
// engine implements it, and part of it was not read anyway. Every one of these
// used to arrive at a caller indistinguishable from a stray "<", which sends an
// author looking for a mistake that is not there.
func TestEveryBoundSaysItIsABound(t *testing.T) {
	for _, tc := range []struct{ name, src, says string }{
		{"the byte cap", strings.Repeat("a", maxInputBytes+1), "larger than this engine will read"},
		{"the node cap", strings.Repeat("<b>x</b>", maxNodes/2+10), "more elements than this engine will build"},
		{"the depth cap", strings.Repeat("<div>", maxDepth+10), "nested more deeply than this engine will read"},
		{"the finding cap", strings.Repeat("<", maxErrors+50), "further problems in this document were not reported"},
	} {
		_, errs, _ := Parse(tc.src)
		var found *Error
		for i := range errs {
			if strings.Contains(errs[i].Message, tc.says) {
				found = &errs[i]
			}
		}
		if found == nil {
			t.Errorf("%s: no finding says %q; got %v", tc.name, tc.says, errs)
			continue
		}
		if !found.Limit {
			t.Errorf("%s: %q is not marked as a bound, so a caller reads it as malformed markup",
				tc.name, found.Message)
		}
		if found.Unsupported {
			t.Errorf("%s: %q is marked unsupported; the engine implements it and stopped short",
				tc.name, found.Message)
		}
	}
}

// TestOrdinaryFindingsAreNotBounds is the other side of it: nothing that is
// actually the author's mistake may claim to be a limit, or the flag says
// nothing.
func TestOrdinaryFindingsAreNotBounds(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a stray less-than sign", "<p>a < b</p>"},
		{"an unclosed comment", "<p><!--x"},
		{"an element that is never closed", "<div>"},
		{"a processing instruction", "<?x?>"},
	} {
		_, errs, _ := Parse(tc.src)
		if len(errs) == 0 {
			t.Errorf("%s: raised nothing", tc.name)
			continue
		}
		for _, e := range errs {
			if e.Limit {
				t.Errorf("%s: %q claims to be a bound that was reached", tc.name, e.Message)
			}
		}
	}
}
