package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// word-spacing takes a percentage, and the percentage is of the font size.
//
// css-text-4 added the value and word-spacing-percent-001 states the basis in
// its own assertion: "percentage values of word-spacing are relative to the
// current font-size". It writes "word-spacing: 1em" and "word-spacing: 100%" on
// two lines and asks for them to be identical.
//
// This engine resolved every percentage here against a basis of zero, so
// "word-spacing: 100%" was silently no spacing at all — the worst kind of wrong,
// because nothing was reported and the page merely came out narrow.
//
// letter-spacing is deliberately not the same: css-text-4 gives it no percentage
// at all, so its basis stays zero.

// wordGap is the distance from the start of one word to the start of the next,
// which is the space's advance plus whatever word-spacing added to it.
func wordGap(t *testing.T, htmlSrc, cssSrc string) style.Unit {
	t.Helper()
	var first, second style.Unit
	seen := 0
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		v, ok := op.(DrawText)
		if !ok || strings.TrimSpace(v.Text) == "" {
			continue
		}
		switch seen {
		case 0:
			first = v.At.X
		case 1:
			second = v.At.X
		}
		seen++
	}
	if seen < 2 {
		t.Fatalf("the fixture drew %d words, want two: %s", seen, htmlSrc)
	}
	return second.Sub(first)
}

func TestAPercentageWordSpacingIsOfTheFontSize(t *testing.T) {
	const src = `<div>a b</div>`
	pct := wordGap(t, src, `div { font-size: 20px; word-spacing: 100% }`)
	em := wordGap(t, src, `div { font-size: 20px; word-spacing: 1em }`)
	if pct != em {
		t.Errorf("\"100%%\" gives a gap of %v and \"1em\" gives %v; the "+
			"percentage is of the font size and the two are the same length",
			pct, em)
	}
	none := wordGap(t, src, `div { font-size: 20px }`)
	if pct <= none {
		t.Errorf("\"100%%\" gives a gap of %v against %v with no word-spacing at "+
			"all; a percentage that resolves to nothing is the bug this fixes",
			pct, none)
	}
}

func TestANegativePercentageClosesTheGap(t *testing.T) {
	// word-spacing-001's first line: "-100%" in a face whose space is one em
	// closes the gap entirely.
	const src = `<div>a b</div>`
	neg := wordGap(t, src, `div { font-size: 20px; word-spacing: -100% }`)
	em := wordGap(t, src, `div { font-size: 20px; word-spacing: -1em }`)
	if neg != em {
		t.Errorf("\"-100%%\" gives %v and \"-1em\" gives %v", neg, em)
	}
}

func TestTheBasisIsTheElementsOwnSize(t *testing.T) {
	// word-spacing-percent-001's fourth line. The percentage inherits as a
	// percentage, so the inner element resolves it against its own 20px and not
	// against the 2px its parent was set at.
	inner := wordGap(t, `<div id=outer><div id=inner>a b</div></div>`,
		`#outer { font-size: 20px; word-spacing: 100%; font-size: 2px }
		 #inner { font-size: 20px }`)
	direct := wordGap(t, `<div id=inner>a b</div>`,
		`#inner { font-size: 20px; word-spacing: 100% }`)
	if inner != direct {
		t.Errorf("an inherited \"100%%\" gives the inner element a gap of %v and "+
			"the same declaration on it directly gives %v; the percentage is "+
			"resolved by whoever uses it", inner, direct)
	}
}

func TestLetterSpacingTakesNoPercentage(t *testing.T) {
	// css-text-4 gives letter-spacing no percentage, so one resolves to nothing
	// and the text is set as though the declaration were not there.
	const src = `<div>ab</div>`
	var pct, none style.Unit
	for _, op := range paintOf(t, src, `div { font-size: 20px; letter-spacing: 100% }`) {
		if v, ok := op.(DrawText); ok {
			pct = v.CharSpacing
		}
	}
	for _, op := range paintOf(t, src, `div { font-size: 20px }`) {
		if v, ok := op.(DrawText); ok {
			none = v.CharSpacing
		}
	}
	if pct != none {
		t.Errorf("\"letter-spacing: 100%%\" gives %v of spacing against %v with "+
			"none declared; the property takes no percentage", pct, none)
	}
}
