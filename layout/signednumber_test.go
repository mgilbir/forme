package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// TestASignedLineHeightIsAMultiplier, and the range the property keeps for
// itself.
//
// "line-height: +5" is five times the font size: the sign is part of CSS's
// <number> and says nothing else. A negative one is not a line-height at all —
// §10.8.1 says the value must be non-negative — so it is invalid and the
// property falls back as though nothing had been said. The parser takes the
// sign because the grammar has one; the range is the property's own.
func TestASignedLineHeightIsAMultiplier(t *testing.T) {
	height := func(css string) style.Unit {
		got := fillsOf(paintOf(t, `<div id="d">X</div>`,
			`#d { font-family: Courier; font-size: 20px; width: 20px;
			      background: rgb(0,0,255); `+css+` }`), blue)
		if len(got) != 1 {
			t.Fatalf("%d boxes for %q", len(got), css)
		}
		return got[0].H
	}
	plain := height(`line-height: 5`)
	if got := height(`line-height: +5`); got != plain {
		t.Errorf("\"+5\" gave %v and \"5\" gave %v; the sign is part of the number",
			got, plain)
	}
	if got := height(`line-height: +5.0`); got != plain {
		t.Errorf("\"+5.0\" gave %v, want %v", got, plain)
	}
	// A negative multiplier is invalid, so the property is as though unset —
	// which is "normal", and nothing like five times the size.
	normal := height(``)
	if got := height(`line-height: -5`); got != normal {
		t.Errorf("\"-5\" gave %v and no declaration gives %v; a negative "+
			"line-height is invalid and falls back", got, normal)
	}
}

// TestASignedTabSizeIsACount is the same pair for the other caller, which has
// the same range and had to say so for itself when the parser stopped doing it.
func TestASignedTabSizeIsACount(t *testing.T) {
	width := func(css string) style.Unit {
		var xs []style.Unit
		for _, op := range paintOf(t, `<div id="d">a	b</div>`,
			`#d { font-family: Courier; font-size: 20px; white-space: pre; `+css+` }`) {
			if v, ok := op.(DrawText); ok {
				xs = append(xs, v.At.X)
			}
		}
		if len(xs) < 2 {
			t.Fatalf("%d runs for %q", len(xs), css)
		}
		return xs[len(xs)-1].Sub(xs[0])
	}
	plain := width(`tab-size: 4`)
	if got := width(`tab-size: +4`); got != plain {
		t.Errorf("\"+4\" gave %v and \"4\" gave %v", got, plain)
	}
	if got, def := width(`tab-size: -4`), width(``); got != def {
		t.Errorf("\"-4\" gave %v and no declaration gives %v; a negative tab-size "+
			"is invalid and falls back to eight", got, def)
	}
}
