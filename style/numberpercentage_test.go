package style

import (
	"math"
	"testing"
)

// TestANumberOrAPercentageIsReadAsANumber: ParseNumberPercentage, which the
// filter functions' amounts are read with, takes a <number> or a <percentage>
// written out or in calc(), a percentage being a hundredth, and refuses what
// does not type-check as one of the two.
func TestANumberOrAPercentageIsReadAsANumber(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want float64
	}{
		{"0.5", 0.5}, {"50%", 0.5}, {"250%", 2.5}, {"0", 0},
		{"calc(0.5)", 0.5}, {"calc(50%)", 0.5},
		{"calc(0.25 + 0.5)", 0.75}, {"calc(50% - 0.5 * 100%)", 0},
		{"calc(2 * 30%)", 0.6}, {"calc(90% / 3)", 0.3}, {"calc(-1)", -1},
		{" 50% ", 0.5},
		// §10.9.2: a NaN at the top of a calculation is censored to zero.
		{"calc(1e308% * 10 - 1e308% * 10)", 0}, {"calc(NaN)", 0},
		{"min(50%, 30%)", 0.3}, {"max(0.2, 0.5)", 0.5}, {"clamp(0, 2, 1)", 1},
		{"calc(round(down, 0.75, 0.5) * 2)", 1},
	} {
		got, ok := ParseNumberPercentage(angleValues(t, tc.src))
		if !ok || math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("%q: %v, %v; want %v", tc.src, got, ok, tc.want)
		}
	}
	for _, src := range []string{
		"", "1px", "10deg", "calc(0.5 + 10%)", "calc(10px)", "calc(1em * 2)",
		"calc(10deg)", "calc(50% * 50%)", "0.5 0.5", "calc(1e308 * 10)",
		"auto", "min(50%, 0.3)", "calc(infinity)",
	} {
		if got, ok := ParseNumberPercentage(angleValues(t, src)); ok {
			t.Errorf("%q was read as %v", src, got)
		}
	}
}
