package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
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
//   - Repeatability. The same document laid out twice gives the same display
//     list and the same findings, down to the layout unit.
//
// The last one is what the whole of style.Unit is for. §5.1's argument for fixed
// point over float64 ends "it makes layout bit-reproducible", and the reftest
// comparison and the caching an embedder does both rest on it — but the stages
// feeding this range over maps, and a map is where reproducibility goes. There
// was a test of it and it was one document of four elements compared by its box
// rectangles; this is every document the corpus reaches, compared by what is
// drawn.
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
	// Painting is the third stage and the one a caller sees.
	ops := Paint(frag)

	// And the same document again, from the same source, which has to give the
	// same page. Built again rather than laid out again: the box tree is what
	// the cascade produced, and the cascade is one of the stages that ranges
	// over maps.
	again := Build(Input{HTML: src, CSS: []Stylesheet{{Source: sheetSrc}}})
	if again.Root == nil {
		t.Fatal("the same document built a root box once and not twice")
	}
	rec2 := NewRecorder(nil)
	ops2 := Paint(Layout(again.Root, Size{W: w, H: h}, again.Fonts, rec2))
	if got, want := renderKey(ops2), renderKey(ops); got != want {
		t.Fatalf("two runs of the same document drew different pages:\n%s\n%s",
			want, got)
	}
	if got, want := findingKey(rec2.Findings()), findingKey(rec.Findings()); got != want {
		t.Fatalf("two runs of the same document reported differently:\n%s\n%s",
			want, got)
	}
	if got, want := findingKey(again.Findings), findingKey(built.Findings); got != want {
		t.Fatalf("two builds of the same document reported differently:\n%s\n%s",
			want, got)
	}
}

// renderKey renders a display list as text, exactly.
//
// Exactly, which is what makes it different from normaliseOps and from
// sketchOps: those two compare *two documents* and are right to drop a mark that
// paints nothing, because two documents may reach an empty box by different
// routes. Two runs of one document may not reach anything by different routes at
// all, so nothing here is dropped and every field that decides what is drawn is
// in the key.
//
// A face is written by name rather than by pointer. Two runs share the standard
// faces and would compare equal by address today, and the day they do not is the
// day this test starts failing for a reason that is not a difference in the page.
func renderKey(ops []Op) string {
	var b strings.Builder
	for _, op := range ops {
		switch v := op.(type) {
		case FillRect:
			fmt.Fprintf(&b, "fill %v %v overhang=%v\n", v.Rect, v.Color, v.Overhang)
		case DrawText:
			fmt.Fprintf(&b, "text %q at %v,%v %s %v %v rtl=%v sideways=%v "+
				"anticlockwise=%v upright=%v pre=%q post=%q merge=%q,%q "+
				"kerns=%v spacing=%v features=%+v clip=%v\n",
				v.Text, v.At.X, v.At.Y, faceName(v.Face), v.Size, v.Color,
				v.RTL, v.Sideways, v.Anticlockwise, v.Upright,
				v.PreContext, v.PostContext, v.MergePre, v.MergePost,
				v.ContextKerns, v.CharSpacing, v.Features, v.Clip)
		case DrawImage:
			fmt.Fprintf(&b, "image %v %q\n", v.Rect, v.Key)
		case TileImage:
			fmt.Fprintf(&b, "tile %+v\n", tileKey(v))
		default:
			fmt.Fprintf(&b, "op %T\n", op)
		}
	}
	return b.String()
}

// faceName is a face's name, or a word for the absence of one.
func faceName(f *shape.Face) string {
	if f == nil {
		return "<none>"
	}
	return f.Name()
}

// tileKey is a tiling without the picture, which has no comparable identity.
func tileKey(v TileImage) string {
	return fmt.Sprintf("%v %q %v %v %v", v.Clip, v.Key, v.Tile, v.StepX, v.StepY)
}

// findingKey renders a report as text. The order is part of it: page.go says the
// findings are in a deterministic order, and an order that changes between runs
// is a report an embedder cannot diff.
func findingKey(findings []Finding) string {
	var b strings.Builder
	for _, f := range findings {
		fmt.Fprintf(&b, "%s|%s|%s|%s|%v\n", f.Rule, f.Severity, f.Property, f.Path, f.Message)
	}
	return b.String()
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
