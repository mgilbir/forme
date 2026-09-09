package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// The whole pipeline over untrusted input.
//
// Every other fuzz target in this repository covers one reader: the tokenizer,
// the selector parser, a font table, an image decoder. What none of them covers
// is the thing an embedder actually calls — Build, then Layout, then Paint —
// and that is where the input has been through every one of those readers and
// their answers have to agree with each other.
//
// It matters more here than in a reader because of what a failure is. A panic
// in a decoder is a value the caller can be handed back; a panic in layout is
// the embedder's process, and a stack overflow is not even a panic — Go makes it
// fatal and unrecoverable, which is why maxBoxDepth exists. The invariants
// below are the ones a caller is entitled to whatever it was handed:
//
//   - Totality. The three stages never panic and always terminate.
//   - Boundedness. The box tree stays inside maxBoxes and maxBoxDepth, and the
//     display list is finite, whatever the document asked for.
//   - Honesty. A tree that was cut off says so, because a branch nobody laid out
//     is a page missing content and the reader has to be told.
//
// It deliberately hands over no resolver, so nothing is fetched: the surface
// under test is the markup and the stylesheet, and image decoding has a fuzz
// target of its own.

func FuzzRender(f *testing.F) {
	docs := []string{
		"",
		"<p>x</p>",
		"<div><span>a</span><b>b</b></div>",
		"<table><tr><td>a<td>b</table>",
		"<ul><li>a<li>b</ul>",
		// The shapes that have historically been hard: a block inside an
		// inline, floats, an out-of-flow box among words, bidirectional text,
		// an inline box with nothing in it, and generated content.
		"<span>a<div>b</div>c</span>",
		`<div style="float:left;width:10px;height:10px"></div>xyz`,
		`<p>a<span style="position:absolute">b</span>c</p>`,
		"<p>السلام abc</p>",
		"<p>a<span></span>b</p>",
		`<p>&#x200B;&#xAD;&#x2007;　x</p>`,
		"<ol><li><ol><li>x</ol></ol>",
		"<img><canvas width=3 height=4></canvas><map><area></map>",
	}
	sheets := []string{
		"",
		"p { color: red }",
		"div { display: flex } span { flex: 1 }",
		"div { display: grid; grid-template-columns: 1fr 2fr }",
		"p { columns: 3 }",
		"div { writing-mode: vertical-rl }",
		"p { text-autospace: normal; letter-spacing: 1px; word-spacing: 2px }",
		"p { hyphens: auto; text-wrap-style: balance }",
		"* { max-height: 1px; min-height: 2px }",
		"p::first-line { font-size: 200% } p::before { content: 'x' }",
		"div { width: 1e9px; height: -5px }",
		"@page { size: A5; margin: 1cm }",
	}
	for _, d := range docs {
		for _, s := range sheets {
			f.Add(d, s)
		}
	}

	f.Fuzz(func(t *testing.T, src, sheetSrc string) {
		checkRender(t, src, sheetSrc)
	})
}

// checkRender is the body of the fuzz target, and is a function of its own so
// that a test can drive it with the caps lowered. A bound that has never been
// seen to fire proves nothing, and neither does an assertion about one.
func checkRender(t testing.TB, src, sheetSrc string) {
	// Bounded for the reason style's own target gives: the fuzzer is looking
	// for logic faults, and a hundred-megabyte input only finds the memory it
	// takes to hold one.
	if len(src) > 1<<16 || len(sheetSrc) > 1<<16 {
		return
	}

	built := Build(Input{HTML: src, CSS: []Stylesheet{{Source: sheetSrc}}})
	if built.Root == nil {
		// A document with no root box is a legitimate answer — "" is one — and
		// there is nothing after it to check.
		return
	}

	boxes, depth := countBoxes(built.Root), depthOf(built.Root)
	if boxes > maxBoxes {
		t.Fatalf("the tree holds %d boxes and the cap is %d", boxes, maxBoxes)
	}
	if depth > maxBoxDepth {
		t.Fatalf("the tree is %d deep and the cap is %d", depth, maxBoxDepth)
	}
	if boxes == maxBoxes || depth == maxBoxDepth {
		// At a cap, so something was cut off, and a branch nobody laid out is a
		// page missing content. See the note on maxBoxes.
		if !said(built.Findings, RuleLimit) {
			t.Fatalf("the tree reached a cap (%d boxes, %d deep) and nothing "+
				"was reported", boxes, depth)
		}
	}

	w, _ := style.FromPx(800)
	h, _ := style.FromPx(600)
	rec := NewRecorder(nil)
	frag := Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	if frag == nil {
		t.Fatal("a box tree laid out to no fragment at all")
	}
	// Painting is the third stage and the one a caller sees; what is checked is
	// that it returns rather than panics, and the assignment is what keeps the
	// call from being optimised away.
	_ = Paint(frag)
}

// TestTheRenderInvariantsAreLive drives the body above with the depth cap
// lowered far enough that an ordinary document trips it.
//
// An assertion that fires only at a cap is one nothing ever reaches: a fuzzer
// would need a document a thousand elements deep to find it, and would not build
// one from these seeds in any time worth waiting. The cap is a variable for
// exactly this reason — box_test.go already lowers the box one to watch it
// decide — so the same lever drives the target's own check.
//
// What it asserts is what the target asserts: the tree came out inside the cap,
// and the cut was reported. Getting either wrong is a page silently missing
// content.
func TestTheRenderInvariantsAreLive(t *testing.T) {
	const deep = `<div><div><div><div><div><div>x</div></div></div></div></div></div>`

	was := maxBoxDepth
	defer func() { maxBoxDepth = was }()
	maxBoxDepth = 4

	// The body itself, which fails the test if the tree came out past the cap
	// or if the cut went unreported.
	checkRender(t, deep, "")

	// And the fixture really does reach the cap, so the run above was not a
	// document that stayed inside it and asserted nothing.
	built := Build(Input{HTML: deep})
	if got := depthOf(built.Root); got > maxBoxDepth {
		t.Fatalf("the tree is %d deep against a cap of %d", got, maxBoxDepth)
	}
	if !said(built.Findings, RuleLimit) {
		t.Fatal("the fixture did not reach the depth cap, so the check above " +
			"exercised nothing; it has to nest deeper than the cap")
	}
}

// said reports whether any finding carries a rule.
func said(findings []Finding, rule Rule) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
