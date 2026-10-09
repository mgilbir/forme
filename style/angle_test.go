package style

import (
	"math"
	"testing"

	"github.com/mgilbir/forme/css"
)

// calc() of an angle: CSS Values 4 §10's arithmetic, over the four angle units.

func angleValues(t *testing.T, src string) []css.ComponentValue {
	t.Helper()
	vals, errs := css.ParseComponentValues(src)
	if len(errs) > 0 {
		t.Fatalf("%q does not tokenize: %v", src, errs)
	}
	return vals
}

// TestAnAngleIsReadInDegrees: every unit, bare and in calc(), and the
// arithmetic CSS Values 4 allows between them.
func TestAnAngleIsReadInDegrees(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want float64
	}{
		{"45deg", 45}, {"100grad", 90}, {"0.25turn", 90}, {"3.141592653589793rad", 180},
		{"calc(45deg + 0.1turn)", 81},
		{"calc(1turn - 90deg)", 270},
		{"calc(90deg / 2)", 45},
		{"calc(2 * 0.5turn)", 360},
		{"calc((10deg + 20deg) * 3)", 90},
		{"calc(-45deg)", -45},
		{"calc(calc(100grad) - 1rad * 0)", 90},
		// §10.9.2: a NaN at the top of a calculation is censored to zero.
		{"calc(1e308deg * 10 - 1e308deg * 10)", 0},
		{"calc(nan * 1deg)", 0},
		{"min(10deg, 0.25turn)", 10},
		{"atan2(1, -1)", 135},
		{"calc(acos(-1) / 2)", 90},
	} {
		got, ok := ParseAngle(angleValues(t, tc.src))
		if !ok || math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: %v, %v; want %v", tc.src, got, ok, tc.want)
		}
	}
	for _, bad := range []string{
		"calc(45deg + 10px)", // an angle and a length
		"calc(45)",           // a number
		"calc(0)",            // a number, though <zero> may stand for an angle
		"calc(10%)",          // a percentage is not an <angle>
		"calc(45deg + 10%)",
		"calc(45deg * 2deg)", // an angle squared
		"calc(45deg / 0)",    // an infinity, which an angle has no largest value for
		"calc(45deg / 1deg)", // a quotient of two dimensions
		"calc(45deg +5deg)",  // "+" needs space on both sides
		"45deg 10deg",        // two
		"45px", "45", "10%", "calc()",
		// Past a float: an angle has no largest value to stand for it.
		"calc(1e308deg * 10)",
		"min(10deg, 10%)", // a percentage is not an <angle>, even here
	} {
		if got, ok := ParseAngle(angleValues(t, bad)); ok {
			t.Errorf("%s was read as %v degrees", bad, got)
		}
	}
}

// TestAnAnglePercentageKeepsItsParts: a percentage is of something only the
// caller knows, so an <angle-percentage> comes back as its two parts.
func TestAnAnglePercentageKeepsItsParts(t *testing.T) {
	for _, tc := range []struct {
		src      string
		deg, pct float64
	}{
		{"25%", 0, 25}, {"45deg", 45, 0},
		{"calc(25% + 45deg)", 45, 25},
		{"calc(50% - 0.25turn)", -90, 50},
		{"calc((10% + 10deg) * 2)", 20, 20},
	} {
		deg, pct, ok := ParseAnglePercentage(angleValues(t, tc.src))
		if !ok || math.Abs(deg-tc.deg) > 1e-9 || math.Abs(pct-tc.pct) > 1e-9 {
			t.Errorf("%s: %v deg %v%% %v; want %v deg %v%%", tc.src, deg, pct, ok, tc.deg, tc.pct)
		}
	}
	for _, bad := range []string{"calc(25% + 1px)", "calc(3)", "10px"} {
		if _, _, ok := ParseAnglePercentage(angleValues(t, bad)); ok {
			t.Errorf("%s was read as an angle-percentage", bad)
		}
	}
}

// TestALengthIsNotAnAngle: the angle units joined calc's arithmetic without
// becoming lengths — an expression of angles is not a length wherever one is
// read, nor an angle added to a length.
func TestALengthIsNotAnAngle(t *testing.T) {
	for _, bad := range []string{"calc(10deg)", "calc(10px + 10deg)", "calc(10% + 1turn)"} {
		if l, ok := evalLength(angleValues(t, bad)[0], LengthContext{FontSize: 16}); ok {
			t.Errorf("%s was read as the length %+v", bad, l)
		}
	}
	if l, ok := evalLength(angleValues(t, "calc(10% + 2px * 3)")[0], LengthContext{}); !ok ||
		l.Kind != LengthCalc || l.Percent != 10 || l.Value.Px() != 6 {
		t.Errorf("a length-percentage is %+v %v", l, ok)
	}
}
