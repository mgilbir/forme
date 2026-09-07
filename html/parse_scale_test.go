package html

import (
	"strings"
	"testing"
	"time"
)

// TestParseIsLinearInDocumentSize guards against a shape of bug this parser had
// and nothing else could see.
//
// The tokenizer used to ask "does the rest of the file start with <!doctype?" by
// lowercasing the rest of the file — at every "<". That is quadratic, and it was
// invisible in every correctness test because it produces the right answer: a
// megabyte of small elements simply took sixty-six seconds. Anyone able to hand
// this engine a document could hang it.
//
// The guard is a wall-clock bound, which is not the kind of assertion to reach
// for lightly. It is the right one here because the quantity under test *is*
// time and because the margin is three orders of magnitude: the document below
// parses in about forty milliseconds and took over a minute before the fix. A
// bound of five seconds cannot be tripped by a slow machine and cannot be
// survived by a reintroduction.
func TestParseIsLinearInDocumentSize(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40000; i++ {
		b.WriteString("<div><span>x</span></div>")
	}
	src := b.String()

	start := time.Now()
	doc, _, _ := Parse(src)
	elapsed := time.Since(start)

	if doc == nil {
		t.Fatal("the document produced no tree")
	}
	if elapsed > 5*time.Second {
		t.Errorf("parsing %d bytes of small elements took %v; it is linear work and "+
			"should take tens of milliseconds, so this is the quadratic scan coming back",
			len(src), elapsed)
	}
}

// TestCaseInsensitiveSearchIsCorrect pins the two helpers the fix introduced,
// because a faster search that is also wrong would be worse than the slow one.
func TestCaseInsensitiveSearchIsCorrect(t *testing.T) {
	for _, tc := range []struct {
		s, prefix string
		want      bool
	}{
		{"<!DOCTYPE html>", "<!doctype", true},
		{"<!DoCtYpE html>", "<!doctype", true},
		{"<!doctype html>", "<!doctype", true},
		{"<!docty", "<!doctype", false},
		{"<div>", "<!doctype", false},
		{"", "", true},
	} {
		if got := hasPrefixFold(tc.s, tc.prefix); got != tc.want {
			t.Errorf("hasPrefixFold(%q, %q) = %v, want %v", tc.s, tc.prefix, got, tc.want)
		}
	}

	for _, tc := range []struct {
		s, sub string
		want   int
	}{
		{"abc</STYLE>", "</style", 3},
		{"abc</style>", "</style", 3},
		{"</styl", "</style", -1},
		{"xx</Style></style>", "</style", 2},
		{"nothing", "</style", -1},
		// The needle's own case must not matter either, since findEndTag builds
		// it from a tag name that has already been folded once.
		{"abc</style>", "</STYLE", 3},
	} {
		if got := indexFold(tc.s, tc.sub); got != tc.want {
			t.Errorf("indexFold(%q, %q) = %d, want %d", tc.s, tc.sub, got, tc.want)
		}
	}
}

// TestMergingTextIsLinearInTheNumberOfRuns guards the other quadratic this
// parser had, which the test above cannot see.
//
// Adjacent text runs are merged so that no element has two text children in a
// row, and a document decides how many times that merge happens: everything the
// tokenizer drops without producing a token splits a run in two. "x<!---->"
// repeated is one text node assembled a byte at a time, and assembling it with
// "+=" copies the whole node each time — six megabytes took thirty-six seconds and
// the parse reported no problem at all, because nothing was wrong with the
// document.
//
// The bound is wall clock for the same reason the test above gives: the
// quantity under test is time, and the margin is three orders of magnitude.
func TestMergingTextIsLinearInTheNumberOfRuns(t *testing.T) {
	const runs = 600000
	src := strings.Repeat("x<!---->", runs)

	start := time.Now()
	doc, errs, ok := Parse(src)
	elapsed := time.Since(start)

	if !ok {
		t.Fatalf("a document of text and comments was refused: %v", errs)
	}
	if got := doc.TextContent(); len(got) != runs {
		t.Fatalf("the merged text is %d bytes, want %d", len(got), runs)
	}
	if elapsed > 5*time.Second {
		t.Errorf("merging %d text runs took %v; the merge is linear work and should take "+
			"tens of milliseconds, so this is the quadratic concatenation coming back", runs, elapsed)
	}
}

// TestTextRunsSplitByDroppedConstructsMergeExactly pins what the accumulator
// must produce, since a buffer reused across text nodes is a place for one
// node's text to end up in another.
func TestTextRunsSplitByDroppedConstructsMergeExactly(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a comment inside a run", "<p>ab<!---->cd</p>", "abcd"},
		{"several comments", "<p>a<!---->b<!---->c<!---->d</p>", "abcd"},
		{"a processing instruction", "<p>ab<?x?>cd</p>", "abcd"},
		{"a declaration", "<p>ab<!x>cd</p>", "abcd"},
		{"an element between two runs", "<p>ab<b>B</b>cd</p>", "abBcd"},
		{"a run either side of a child", "<p>a<!---->b<b>B</b>c<!---->d</p>", "abBcd"},
		{"a run resumed after an element", "<p>a<b>B</b>c<!---->d</p>", "aBcd"},
		{"nested elements each with runs", "<p>a<!---->b<b>c<!---->d</b>e<!---->f</p>", "abcdef"},
		{"a run at the very end of the document", "<p>a<!---->b", "ab"},
	} {
		doc, _, _ := Parse(tc.src)
		if doc == nil {
			t.Fatalf("%s: no tree", tc.name)
		}
		if got := doc.TextContent(); got != tc.want {
			t.Errorf("%s: text is %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestNoElementHasTwoTextChildrenInARow states the invariant the merge exists
// for, over the same documents.
func TestNoElementHasTwoTextChildrenInARow(t *testing.T) {
	for _, src := range []string{
		"<p>a<!---->b</p>",
		"<p>a<!---->b<b>c</b>d<!---->e</p>",
		"<p>a<?x?>b<!y>c</p>",
		"a<!---->b<p>c</p>d<!---->e",
	} {
		doc, _, _ := Parse(src)
		var walk func(*Node)
		walk = func(n *Node) {
			for i := 1; i < len(n.Children); i++ {
				if n.Children[i-1].Type == TextNode && n.Children[i].Type == TextNode {
					t.Errorf("%q: <%s> has two text children in a row, %q then %q",
						src, n.Name, n.Children[i-1].Text, n.Children[i].Text)
				}
			}
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(doc)
	}
}
