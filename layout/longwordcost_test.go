package layout

import (
	"runtime"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// TestOneLongWordDoesNotShapeAPrefixPerLine guards the shape of fault that
// SplitHead's note describes being fixed once already, and that came back.
//
// A word broken across many lines is shaped once, and every line's question is
// then a sum over the glyphs it covers. Measured piecewise instead, the rest of
// the word is shaped again at every line it is cut at, which is quadratic in the
// word — twenty thousand characters took twelve seconds and nine gigabytes when
// that note was written.
//
// It came back through LineEndCorrection. That takes the pair kerning off a
// line's last run by measuring the same stretch twice, and the obvious way to
// ask "with nothing after it" is to measure the group truncated at the item's
// end. The truncation is a *different string for every line*, so the shaping
// memo never answers twice and each line end shapes another prefix of the word.
//
// # The face has to kern, or this measures nothing
//
// The correction is only computed where there is a kern to give back. Written
// against Courier — which does not kern, and which every other cost test here
// uses because its arithmetic is readable — the expensive branch is never
// entered at all and the bound holds however the work is done. That version was
// written first and a plant walked straight through it. So the text is "AV"
// repeated in a face that states the pair.
//
// # Why allocation and not time
//
// The wall clock cannot separate the two. Laying this word out grows about 3.6x
// per doubling either way — there is a second, older super-linearity in this
// path that this test is not about — so the fault is a constant factor of about
// three on a curve that is already steep, and a bound tight enough to catch it
// trips on a loaded machine. Allocation separates them and does not care what
// else is running:
//
//	                shaped once      a prefix per line
//	 8000 chars           62 MB                 248 MB
//	16000 chars          136 MB                 957 MB
//
// The bound is 400 MB: nearly three times the honest cost, so ordinary growth
// does not trip it, and under half the faulty one.
func TestOneLongWordDoesNotShapeAPrefixPerLine(t *testing.T) {
	const chars = 16000
	const budgetMB = 400

	set, _ := notoNamed(t)
	built := Build(Input{
		HTML: `<div id="d">` + strings.Repeat("AV", chars/2) + "</div>",
		CSS: []Stylesheet{{Source: noDefaults +
			`#d { font-family: T; font-size: 16px; overflow-wrap: break-word }`}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(1000000)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	frag := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	runtime.ReadMemStats(&after)

	// The word really is broken across many lines, or nothing above happened:
	// a bound satisfied by a document that set one line is not a bound.
	lines := 0
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		lines += len(f.Lines)
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(frag)
	if lines < 100 {
		t.Fatalf("the word set %d lines; at %d characters in 600px it is "+
			"hundreds, and a fault that is per-line needs lines to show",
			lines, chars)
	}

	if mb := (after.TotalAlloc - before.TotalAlloc) / (1 << 20); mb > budgetMB {
		t.Errorf("laying out one %d-character word allocated %d MB, over the "+
			"%d MB budget. The measured cost is about 136 MB; a prefix shaped "+
			"per line is about 957 MB, which is what this guards",
			chars, mb, budgetMB)
	}
}

// TestOneLongWordDoesNotWalkItsClustersPerLine is the other half of the same
// document's cost, and a different fault in a different place.
//
// overflows() asks trailingSpacing() of a candidate before deciding whether it
// fits, and trailingSpacing asked §8.2 whether cursive tracking suppresses the
// letter-spacing after the item's last character. Answering that walks the
// item's grapheme clusters — and during a fill the item is the *whole remaining
// run*, so one long word walked the rest of the word at every candidate the
// line considered. It was 57% of the time in a CPU profile of laying one out.
//
// The answer was never needed. Every branch of trailingSpacing adds either zero
// or the declared letter-spacing, and where there is no letter-spacing those are
// the same number — which is almost every document. So the question is not asked
// unless the answer can matter.
//
//	                 asked only when it matters      asked always
//	16000 chars            44 ms,     42 MB        127 ms,   107 MB
//	32000 chars            97 ms,     88 MB        402 ms,   370 MB
//
// That is what took this path from about 3.3x per doubling to about 2.2x — from
// quadratic to linear, which is the shape and not the constant.
//
// Allocation again rather than the clock, for the reason above: it does not care
// what else is running. The bound is 200 MB at thirty-two thousand characters,
// twice the honest cost and under half the faulty one.
func TestOneLongWordDoesNotWalkItsClustersPerLine(t *testing.T) {
	const chars = 32000
	const budgetMB = 200

	// Courier and no letter-spacing, which is the case the short-circuit is for
	// and the one nearly every document is.
	built := Build(Input{
		HTML: `<div id="d">` + strings.Repeat("a", chars) + "</div>",
		CSS: []Stylesheet{{Source: noDefaults +
			`#d { font-family: Courier; font-size: 16px; overflow-wrap: break-word }`}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(1000000)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	frag := Layout(built.Root, Size{W: w, H: h}, nil, NewRecorder(nil))
	runtime.ReadMemStats(&after)

	lines := 0
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		lines += len(f.Lines)
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(frag)
	if lines < 100 {
		t.Fatalf("the word set %d lines; a fault that is per-line needs lines "+
			"to show", lines)
	}

	if mb := (after.TotalAlloc - before.TotalAlloc) / (1 << 20); mb > budgetMB {
		t.Errorf("laying out one %d-character word allocated %d MB, over the "+
			"%d MB budget. The measured cost is about 88 MB; walking the "+
			"clusters at every candidate is about 370 MB, which is what this "+
			"guards", chars, mb, budgetMB)
	}
}
