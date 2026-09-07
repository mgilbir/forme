package layout

import (
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/style"
)

// How deeply boxes may nest.
//
// The walk that makes them is recursive, and so is every walk over what it
// makes — fixup, §17.4's table wrapper, layout, painting. The box *count* is
// capped and that bounds the work; it does not bound the stack, because a tree
// a hundred thousand deep is a hundred thousand frames in each of those walks
// and well under the count.
//
// The html package caps a parsed document at 256 elements deep. It does not cap
// this: BuildBoxes is exported and takes an *html.Node, so a caller assembling a
// tree by hand reaches box generation with whatever depth it built — and the
// answer was a stack overflow, which in Go is fatal and cannot be recovered by
// the caller that caused it.

// nestedNodes builds an element tree n deep by hand, which is what an API
// caller can do and a parsed document cannot.
func nestedNodes(n int) *html.Node {
	root := &html.Node{Type: html.ElementNode, Name: "html"}
	at := root
	for i := 0; i < n; i++ {
		child := &html.Node{Type: html.ElementNode, Name: "div", Parent: at}
		at.Children = append(at.Children, child)
		at = child
	}
	at.Children = append(at.Children, &html.Node{Type: html.TextNode, Text: "x", Parent: at})
	return root
}

// TestAHandBuiltTreeIsBoundedBeforeTheStackIs.
func TestAHandBuiltTreeIsBoundedBeforeTheStackIs(t *testing.T) {
	// Deeper than the cap, and deep enough that the old code's recursion is
	// what this is about rather than the tree's size: 4,000 boxes is a
	// four-thousandth of the box cap.
	doc := nestedNodes(4000)
	rec := NewRecorder(nil)
	styled := style.Apply(doc, nil)

	root := BuildBoxes(doc, styled, rec)
	if root == nil {
		t.Fatal("a tree of four thousand divs produced no boxes at all")
	}
	if got := depthOf(root); got > maxBoxDepth {
		t.Errorf("the box tree is %d deep and the cap is %d", got, maxBoxDepth)
	}
	if rec.Count(RuleLimit) == 0 {
		t.Error("the tree was cut off and nothing was reported; a branch that " +
			"was not laid out is a page missing content, and the reader has to " +
			"be told")
	}
	// Once, not once per box.
	if n := rec.Count(RuleLimit); n != 1 {
		t.Errorf("the depth cap was reported %d times, want once", n)
	}
}

// TestTheCounterWalkStopsWhereBoxGenerationDoes is the half the box tree
// cannot show.
//
// Box generation is not the first walk over the document: the counters are
// settled before any box exists, because a counter's value depends on what came
// before an element and the box walk cannot answer that while descending. That
// walk recurses too, so capping only the box walk would leave the cap doing
// nothing — a deep tree reaches the counters first, and the frames it spends
// there are spent whether or not a box is ever made.
//
// The witness is not the clock. A hundred thousand frames is survivable, so a
// run that is merely deep still finishes; what the bound has to be seen doing
// is *stopping*, and the visible sign of that is how many elements came back
// numbered.
func TestTheCounterWalkStopsWhereBoxGenerationDoes(t *testing.T) {
	// Deeper than the cap, and every level a list item, so that an unbounded
	// walk is one entry per level and a bounded one cannot be.
	const deep = 4000
	doc := nestedNodes(deep)

	styles := map[*html.Node]style.ComputedStyle{}
	var describe func(*html.Node)
	describe = func(n *html.Node) {
		if n.Type == html.ElementNode {
			styles[n] = style.ComputedStyle{
				"display":           "block",
				"counter-increment": "list-item",
			}
		}
		for _, c := range n.Children {
			describe(c)
		}
	}
	describe(doc)

	got := computeCounters(doc, styles, nil)
	if len(got.elements) == 0 {
		t.Fatal("no element came back numbered, so this test is watching " +
			"nothing; the fixture has to make list items for the count to mean " +
			"anything")
	}
	if len(got.elements) > maxBoxDepth {
		t.Errorf("the counter walk numbered %d elements in a tree %d deep; the "+
			"cap is %d, and a branch box generation will not build is a branch "+
			"whose counters nothing reads", len(got.elements), deep, maxBoxDepth)
	}
}

// nestedOptgroups builds a select whose optgroups nest n deep, with the one
// option at the bottom. Markup cannot express this — an <optgroup> start tag
// closes the open one — but a tree built through the API can.
func nestedOptgroups(n int) *html.Node {
	root := &html.Node{Type: html.ElementNode, Name: "html"}
	sel := &html.Node{Type: html.ElementNode, Name: "select", Parent: root}
	root.Children = append(root.Children, sel)
	at := sel
	for i := 0; i < n; i++ {
		g := &html.Node{Type: html.ElementNode, Name: "optgroup", Parent: at}
		at.Children = append(at.Children, g)
		at = g
	}
	opt := &html.Node{Type: html.ElementNode, Name: "option", Parent: at}
	opt.Children = append(opt.Children, &html.Node{Type: html.TextNode, Text: "x", Parent: opt})
	at.Children = append(at.Children, opt)
	return root
}

// deepWalkEnv names the case a child process is to run. See
// TestNoWalkOverAHandBuiltTreeExhaustsTheStack.
const deepWalkEnv = "FORME_DEEP_WALK"

// blockStyles gives every element in a tree the same declarations, without
// recursing to do it — these fixtures are deeper than the stack the child
// process is allowed, so the test's own helpers have to be flat too.
func blockStyles(doc *html.Node, decls style.ComputedStyle) map[*html.Node]style.ComputedStyle {
	out := map[*html.Node]style.ComputedStyle{}
	doc.Walk(func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			out[n] = decls
		}
		return true
	})
	return out
}

// deepWalks are the walks a hand-built tree drives, each with the stack it is
// allowed and the depth it is given.
//
// The figures are not tight, deliberately: each cap is several times what its
// walk needs with the bound in place, and a small fraction of what the same walk
// needs without one. Measured on this machine, at Go 1.26:
//
//   - document: bounded, the whole pipeline fits in under 2 MB — a thousand
//     levels at about 2 kB of frames each. Unbounded it wants a hundred
//     thousand of those levels, some 200 MB, so the 8 MB cap sits four times
//     above what passes and twenty-five times below what fails.
//   - counters, options: the walks are one small frame per level, so a
//     megabyte holds tens of thousands of them; the caps are far enough apart
//     from both ends for the same reason.
var deepWalks = map[string]struct {
	stack int
	run   func()
}{
	// The whole pipeline, which is how a caller reaches this: style the tree,
	// then build boxes from it.
	"document": {stack: 8 << 20, run: func() {
		doc := nestedNodes(100000)
		BuildBoxes(doc, style.Apply(doc, nil), NewRecorder(nil))
	}},
	// The counter walk, called directly. It runs before any box exists, and its
	// frames are small enough that the pipeline case above would not notice it
	// — a hundred thousand of them fit in the stack the box walk needs for a
	// thousand.
	"counters": {stack: 1 << 20, run: func() {
		doc := nestedNodes(50000)
		computeCounters(doc, blockStyles(doc, style.ComputedStyle{"display": "block"}), nil)
	}},
	// The options walk, which descends below the box that reached it and so is
	// not covered by the depth cap at all. Called directly for the same reason
	// as the counters: through the pipeline it would be the pipeline's stack
	// being measured.
	"options": {stack: 1 << 20, run: func() {
		optionsOf(nestedOptgroups(50000).Children[0])
	}},
}

// TestNoWalkOverAHandBuiltTreeExhaustsTheStack is the witness the clock cannot
// be.
//
// What C99 names is not a wrong answer but a fatal one: a Go stack overflow is
// not a panic and cannot be recovered, so the process the engine was embedded in
// dies. That cannot be asserted in the process running the test, and it cannot
// be seen by timing either — a hundred thousand frames is survivable, so an
// unbounded walk that is merely deep still finishes and still passes.
//
// So each walk is run in a child process whose goroutine stacks are capped well
// below Go's a-gigabyte default. Bounded, the walk fits with room to spare and
// the child exits zero. Unbounded, it does not, and the child dies exactly the
// way an embedder would.
func TestNoWalkOverAHandBuiltTreeExhaustsTheStack(t *testing.T) {
	if name := os.Getenv(deepWalkEnv); name != "" {
		c, ok := deepWalks[name]
		if !ok {
			t.Fatalf("no such deep walk: %q", name)
		}
		debug.SetMaxStack(c.stack)
		c.run()
		return
	}
	for name := range deepWalks {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0],
				"-test.run=^TestNoWalkOverAHandBuiltTreeExhaustsTheStack$")
			cmd.Env = append(os.Environ(), deepWalkEnv+"="+name)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("the %s walk did not survive a capped stack: %v\n%s",
					name, err, headOfDump(out, 8))
			}
		})
	}
}

// headOfDump is as much of a stack dump as says what happened.
func headOfDump(b []byte, n int) string {
	lines := strings.Split(string(b), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "...")
	}
	return strings.Join(lines, "\n")
}

// TestADocumentThatNestsNormallyIsNotCutOff is the other half. The cap is four
// times what the parser allows, so nothing a document can express reaches it —
// and a cap that fired on ordinary markup would be worse than none.
func TestADocumentThatNestsNormallyIsNotCutOff(t *testing.T) {
	// The deepest a parsed document can be, wrapped in a table at every level
	// so that §17.4's anonymous boxes are generated too.
	const deep = 200
	src := strings.Repeat(`<div style="display: table-cell">`, deep) + "x"
	built := Build(Input{HTML: src})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	for _, f := range built.Findings {
		if f.Rule == RuleLimit && strings.Contains(f.Message, "nests boxes") {
			t.Errorf("a document %d elements deep hit the depth cap: %s", deep, f.Message)
		}
	}
}

// depthOf is how many levels the box tree has.
func depthOf(b *Box) int {
	best := 0
	for _, c := range b.Children {
		if d := depthOf(c); d > best {
			best = d
		}
	}
	return best + 1
}
