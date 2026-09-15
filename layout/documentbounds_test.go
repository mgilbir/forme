package layout

import (
	"fmt"
	"strings"
	"testing"
)

// The bounds a document can reach, and the documents that reach them.
//
// Each of these was found by raising the cap and running this package: every
// one of them passed, because no fixture here writes a document large enough to
// be refused. A bound nothing exercises is the same as no bound — the next edit
// to the line is unopposed — so these are the documents that exercise them.

// TestACounterNameBeyondTheBoundIsNotCreated is the bound on how many distinct
// counters one document may have.
//
// counter-reset names them and a stylesheet may name as many as it likes, each
// one a map entry that lives as long as the document.
func TestACounterNameBeyondTheBoundIsNotCreated(t *testing.T) {
	// One declaration naming more counters than the bound allows.
	const names = maxCounterNames + 100
	var reset strings.Builder
	for i := 0; i < names; i++ {
		fmt.Fprintf(&reset, "c%d 7 ", i)
	}
	css := fmt.Sprintf(`body { counter-reset: %s }
		#early::before { content: counter(c0) }
		#late::before { content: counter(c%d) }`, reset.String(), names-1)
	got := generatedText(t, `<p id="early">a</p><p id="late">b</p>`, css)

	// The early one is there, or the fixture never reached the counters at all.
	if !strings.Contains(got, "7") {
		t.Fatalf("the document reads %q; the first counter was not set, so this "+
			"says nothing about the bound", got)
	}
	// The late one was past the bound and must not have been created: an
	// uncreated counter reads as zero, not as the seven the declaration asked
	// for. Without the bound it reads seven, which is what makes this a test.
	if strings.Count(got, "7") > 1 {
		t.Errorf("the document reads %q; a counter named past the bound of %d was "+
			"created anyway", got, maxCounterNames)
	}
}

// TestTheCounterDepthCannotBeReachedThroughADocument records that this bound
// sits above what the rest of the engine can produce.
//
// The counter stack holds one entry per tree depth — reset at the same depth
// replaces rather than pushes — and the parser will not build a tree deeper than
// its own bound, which is smaller than this one. So maxCounterDepth cannot bind,
// and a test that claimed to exercise it would be asserting nothing.
//
// This asserts the relationship instead. It is the thing that would change: if
// the parser's depth bound is ever raised past this one, a document could reach
// it, and this fails and says so rather than leaving a bound that silently
// starts mattering.
func TestTheCounterDepthCannotBeReachedThroughADocument(t *testing.T) {
	deepest := 0
	for _, n := range []int{64, 128, 200, 250, 260, 300, 400, 520, 600} {
		src := strings.Repeat(`<div style="counter-reset:c">`, n) +
			`<p id="deep">x</p>` + strings.Repeat(`</div>`, n)
		if strings.Contains(generatedText(t, src, `#deep::before { content: counter(c) }`), "0") {
			deepest = n
		}
	}
	if deepest == 0 {
		t.Fatal("no nesting produced a counter at all; the fixture is wrong")
	}
	if deepest >= maxCounterDepth {
		t.Errorf("a document reached a nesting of %d and the counter bound is %d; "+
			"it can now be reached, and wants a document that reaches it rather "+
			"than this", deepest, maxCounterDepth)
	}
	t.Logf("the deepest document that builds nests %d; the counter bound is %d",
		deepest, maxCounterDepth)
}

// TestARepeatBeyondTheBoundIsRefused is the bound on grid's repeat().
//
// "repeat(1000000, 1px)" is nine characters of CSS and a million tracks.
func TestARepeatBeyondTheBoundIsRefused(t *testing.T) {
	ok := layoutOf(t, 600, `<div id="g"><i>a</i></div>`,
		fmt.Sprintf(`#g { display: grid; grid-template-columns: repeat(%d, 1px) }`,
			maxRepeatedTracks/2))
	if ok == nil {
		t.Fatal("a repeat within the bound produced nothing")
	}
	// Past the bound the declaration is refused rather than expanded, and the
	// document still lays out. Without the bound this is a million tracks.
	frag := layoutOf(t, 600, `<div id="g"><i>a</i></div>`,
		fmt.Sprintf(`#g { display: grid; grid-template-columns: repeat(%d, 1px) }`,
			maxRepeatedTracks+1))
	if frag == nil {
		t.Error("a repeat past the bound produced no fragment at all; it is refused, " +
			"which leaves the document laid out without it")
	}
}

// TestASpanAndACanvasDimensionSurviveAnAbsurdAttribute is behaviour, and it is
// deliberately not claimed as coverage of maxSpanValue or maxCanvasDigits.
//
// Both of those bound *parsing*, not results. spanValue stops reading digits
// once the number passes the bound because "the digits after this one cannot
// change the answer" — every caller clamps to its own smaller limit — and the
// canvas reader refuses a number of too many digits before handing it to Atoi.
// So raising either changes nothing a document can see: a canvas width of ten,
// eleven and thirty digits all lay out identically, because what the attribute
// states is clamped afterwards whatever it says, and a rowspan is clamped to
// maxRowSpan whether it was read as a million or as a million million.
//
// That was measured rather than assumed, and it is why there is no test here
// that fails when those two are raised. A test that claimed to cover them would
// be asserting the clamp below them, not the bound. What is left worth having
// is that an absurd attribute does not break the document, which is this.
func TestASpanAndACanvasDimensionSurviveAnAbsurdAttribute(t *testing.T) {
	for _, src := range []string{
		`<table><tr><td rowspan="99999999999">a</td><td>b</td></tr></table>`,
		`<table><tr><td colspan="99999999999">a</td></tr><tr><td>b</td></tr></table>`,
		`<table><tr><td rowspan="0">a</td></tr></table>`,
		`<table><tr><td rowspan="` + strings.Repeat("9", 5000) + `">a</td></tr></table>`,
		`<canvas width="` + strings.Repeat("9", 5000) + `" height="10"></canvas>`,
		`<canvas width="99999999999" height="10"></canvas>`,
	} {
		if frag := layoutOf(t, 600, src); frag == nil {
			t.Errorf("a document with an attribute of %d bytes produced no fragment",
				len(src))
		}
	}
}
