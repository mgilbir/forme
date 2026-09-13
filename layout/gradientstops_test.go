package layout

import (
	"math"
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS Images 3 §3.4.2: a colour stop with no position of its own is spread
// evenly between the placed stops either side of it.
//
// "linear-gradient(red, red, red 50%, blue 50%, blue)" has one stop with nowhere
// to be. The first and last are placed at the ends of the line by the rule above
// it, so the run always has a placed stop on each side, and the unplaced one
// lands halfway between them — at 25%, not at 0 and not at 50.
//
// That whole loop was at 0% across every unit test and all 6253 reftest
// documents. Two things had to coincide for it: this engine paints only gradients
// whose bands are solid, so every colour change must fall on a hard stop, and
// the gradients written that way all place every stop they have. A three-stop
// gradient with an unpositioned middle is the commonest form there is in real
// CSS — it is just never also constant.
//
// Getting it wrong is a band in the wrong place, or none: the unplaced stop
// would sit at zero, so the band before it would have no width and the one after
// it would start at the beginning of the box.

// stopsOf reads a gradient and returns its stops once the positions are settled.
// It asks the parser directly, because the positions are what this is about and
// a painted band is two of them added together.
func stopsOf(t *testing.T, raw string) []bandStop {
	t.Helper()
	built := Build(Input{HTML: `<div id="d">x</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#d { font-size: 16px }`}}})
	if built.Root == nil || len(built.Root.Children) == 0 {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(400)
	l := newLayouter(built.Root, Size{W: w, H: h}, nil, NewRecorder(nil))
	g, ok := l.bandsOf(built.Root.Children[0], raw)
	if !ok {
		t.Fatalf("the gradient %q was not read; it must be one this engine "+
			"paints as solid bands or the positions are never worked out", raw)
	}
	return g.stops
}

// percentsOf is each stop's position, which for these gradients is a percentage.
func percentsOf(t *testing.T, stops []bandStop) []float64 {
	t.Helper()
	out := make([]float64, len(stops))
	for i, s := range stops {
		if !s.placed {
			t.Errorf("stop %d came out unplaced; every stop has a position by "+
				"the end, which is what the rest of the gradient is read against", i)
		}
		if s.at.Kind != style.LengthPercent {
			t.Fatalf("stop %d is a %v rather than a percentage", i, s.at.Kind)
		}
		out[i] = s.at.Percent
	}
	return out
}

func samePercents(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			return false
		}
	}
	return true
}

func TestAnUnpositionedStopIsSpreadBetweenThePlacedOnes(t *testing.T) {
	for _, c := range []struct {
		gradient string
		want     []float64
		why      string
	}{
		// One unplaced stop between the auto-placed 0% and an explicit 50%.
		{"linear-gradient(red, red, red 50%, blue 50%, blue)",
			[]float64{0, 25, 50, 50, 100},
			"halfway between the two placed stops around it"},

		// Two in a row: the run is divided into three, not halved twice.
		{"linear-gradient(red, red, red, red 60%, blue 60%, blue)",
			[]float64{0, 20, 40, 60, 60, 100},
			"a run of two divides the gap into three"},

		// Between two stops the author placed, so neither end is the rule above.
		{"linear-gradient(red 20%, red, red 60%, blue 60%, blue)",
			[]float64{20, 40, 60, 60, 100},
			"between 20% and 60% is 40%"},

		// A run that reaches the end, where the last stop was placed at 100% by
		// the rule above rather than written.
		{"linear-gradient(red 40%, blue 40%, blue, blue)",
			[]float64{40, 40, 70, 100},
			"between 40% and the 100% the last stop was given"},

		// And one at each end of the same gradient.
		{"linear-gradient(red, red, red 50%, blue 50%, blue, blue)",
			[]float64{0, 25, 50, 50, 75, 100},
			"two separate runs, each spread within its own pair"},
	} {
		t.Run(c.gradient, func(t *testing.T) {
			got := percentsOf(t, stopsOf(t, c.gradient))
			if !samePercents(got, c.want) {
				t.Errorf("the stops came out at %v%%, want %v%% — %s",
					got, c.want, c.why)
			}
		})
	}
}

// A gradient that places every stop is left alone, which is what says the
// spreading is the rule firing rather than the positions being recomputed.
func TestAGradientThatPlacesEveryStopIsUnchanged(t *testing.T) {
	got := percentsOf(t, stopsOf(t,
		"linear-gradient(red 10%, red 30%, blue 30%, blue 90%)"))
	want := []float64{10, 30, 30, 90}
	if !samePercents(got, want) {
		t.Errorf("the stops came out at %v%%, want %v%% — every one was "+
			"written, including the first and last, so none of §3.4.2's rules "+
			"has anything to do", got, want)
	}
}
