package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
)

// Dotted and dashed rounded borders: see roundeddash.go.

// quarterPerimeter is a quarter of an ellipse's perimeter, from the complete
// elliptic integral of the second kind computed by the arithmetic–geometric
// mean: P = 2π/M(a,b) · (a² − Σ 2^(n−1) cₙ²), with c₀² = a² − b² and
// cₙ₊₁ = (aₙ − bₙ)/2. It converges quadratically and is exact to the last few
// bits of a float, which makes it the reference arcLength is checked against.
func quarterPerimeter(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	sum := (a*a - b*b) / 2 // 2^(−1) c₀²
	an, bn := a, b
	pow := 0.5
	for i := 0; i < 64 && an-bn > 1e-15*an; i++ {
		cn := (an - bn) / 2
		an, bn = (an+bn)/2, math.Sqrt(an*bn)
		pow *= 2
		sum += pow * cn * cn
	}
	return 2 * math.Pi / (an + bn) * 2 * (a*a - sum) / 4
}

// TestAnArcIsMeasuredToAThousandthOfAPixel holds arcLength to the perimeter of
// the whole ellipse, from circles to one a thousand times as wide as it is
// tall, and to the circle's closed form on a piece of a quarter.
func TestAnArcIsMeasuredToAThousandthOfAPixel(t *testing.T) {
	if got, want := quarterPerimeter(10, 10), math.Pi*10/2; math.Abs(got-want) > 1e-12 {
		t.Fatalf("the reference is wrong: a circle's quarter is %v, want %v", got, want)
	}
	for _, r := range [][2]float64{{10, 10}, {40, 10}, {10, 40}, {300, 7}, {2000, 2}, {2, 2000}, {2000, 1500}} {
		c := corner(r[0], r[1])
		got := arcLength(c, 180, 270)
		want := quarterPerimeter(c.X.Px(), c.Y.Px())
		if math.Abs(got-want) > 1e-3 {
			t.Errorf("a quarter of the %v by %v ellipse is %.6f, want %.6f", r[0], r[1], got, want)
		}
		// And the quarters add up whichever way round they are asked for.
		if half := arcLength(c, 0, 180); math.Abs(half-2*want) > 2e-3 {
			t.Errorf("half of the %v by %v ellipse is %.6f, want %.6f", r[0], r[1], half, 2*want)
		}
	}
	// A piece of a circle is its angle times its radius.
	if got, want := arcLength(corner(30, 30), 200, 237), 37*math.Pi/180*30; math.Abs(got-want) > 1e-9 {
		t.Errorf("37 degrees of a 30px circle is %v, want %v", got, want)
	}
}

// dotsOf is the circles among a list's dotted marks: centre, radius, and the
// region clip they were drawn in.
type dot struct{ x, y, r float64 }

func dotsOf(t *testing.T, ops []Op) [][]dot {
	t.Helper()
	var sides [][]dot
	for _, g := range groupsOf(ops) {
		var side []dot
		for _, op := range g.Ops {
			fp, ok := op.(FillPath)
			if !ok {
				continue
			}
			for _, s := range fp.Path {
				if s.Op == ArcTo && s.SweepAngle == 360 {
					side = append(side, dot{s.Center.X.Px(), s.Center.Y.Px(), s.RadiusX.Px()})
				}
			}
		}
		if len(side) > 0 {
			sides = append(sides, side)
		}
	}
	return sides
}

// TestADottedCircleHasItsDotsOnItsMiddle is §3.2's round dots going round: a
// hundred-pixel box with "border-radius: 50%" and a four-pixel dotted border is
// a ring of circles 46 and 50 pixels about its centre. Every dot is centred on
// the ring's middle circle, is as wide as the border, and is the same distance
// round it from the next; and each side's first and last dot is on a diagonal,
// where §4.4's transition puts the meeting of two sides of one width.
func TestADottedCircleHasItsDotsOnItsMiddle(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 92px; height: 92px;
		border: 4px dotted blue; border-radius: 50% }`)
	sides := dotsOf(t, ops)
	if len(sides) != 4 {
		t.Fatalf("%d sides of dots, want 4: %v", len(sides), ops)
	}
	const cx, cy, mid = 50.0, 50.0, 48.0
	// A quarter of the middle circle, cut into periods of two widths.
	quarter := math.Pi * mid / 2
	n := math.Round(quarter / 8)
	step := quarter / n
	for i, side := range sides {
		if len(side) != int(n)+1 {
			t.Errorf("side %d has %d dots, want %v", i, len(side), n+1)
			continue
		}
		for j, d := range side {
			if r := math.Hypot(d.x-cx, d.y-cy); math.Abs(r-mid) > 0.05 {
				t.Errorf("side %d dot %d is %.3f from the centre, want %v", i, j, r, mid)
			}
			if math.Abs(d.r-2) > 0.02 {
				t.Errorf("side %d dot %d has radius %.3f, want 2", i, j, d.r)
			}
			if j > 0 {
				p := side[j-1]
				// The distance round the circle between two neighbours.
				a := math.Abs(math.Atan2(d.y-cy, d.x-cx) - math.Atan2(p.y-cy, p.x-cx))
				if a > math.Pi {
					a = 2*math.Pi - a
				}
				if got := a * mid; math.Abs(got-step) > 0.05 {
					t.Errorf("side %d dots %d and %d are %.3f apart round the circle, want %.3f", i, j-1, j, got, step)
				}
			}
		}
		for _, d := range []dot{side[0], side[len(side)-1]} {
			a := math.Atan2(d.y-cy, d.x-cx) * 180 / math.Pi
			if off := math.Mod(math.Abs(a)-45, 90); math.Abs(off) > 0.1 {
				t.Errorf("side %d ends on a dot at %.2f degrees, want a diagonal", i, a)
			}
		}
	}
	if hasRule(paintFindingsOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 92px; height: 92px;
		border: 4px dotted blue; border-radius: 50% }`), RuleUnsupportedValue) {
		t.Error("a dotted rounded border was reported")
	}
}

// paintFindingsOf lays out and paints a document and returns what was said.
func paintFindingsOf(t *testing.T, htmlSrc, cssSrc string) []Finding {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}})
	rec := NewRecorder(nil)
	PaintReporting(Layout(built.Root, Size{W: rpx(800), H: rpx(2000)}, nil, rec), rec)
	return rec.Findings()
}

// dashedAt reports whether a point is inside one of the dashed marks.
func dashedAt(ps []FillPath, x, y float64) bool {
	for _, p := range ps {
		if p.Path.Contains(ptPx(x, y)) {
			return true
		}
	}
	return false
}

// TestADashedCornerIsSymmetrical: a dash goes round each corner centred on the
// diagonal its two sides meet on, so the corner is its own mirror image about
// that diagonal — the symmetry §3.2 encourages — and the marks cover half the
// middle of the border, dash and gap alike.
func TestADashedCornerIsSymmetrical(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 192px; height: 92px;
		border: 4px dashed blue; border-radius: 40px }`)
	ps := pathsOf(ops)
	if len(ps) != 4 {
		t.Fatalf("%d marked paths, want one per side: %v", len(ps), ops)
	}
	// The top left corner's circles are centred on (40, 40), and the diagonal
	// through it is y = x. The corner's dash is twelve pixels along the middle
	// of the border, six either side of the diagonal: 6/38 of a radian each
	// way round the middle circle. The next marks on the two sides are where
	// each side's own spacing puts them, a period on, so the corner is sampled
	// within 25 degrees of the diagonal and mirrored about it.
	for y := 0.25; y < 40; y += 0.5 {
		for x := 0.25; x < 40; x += 0.5 {
			a := math.Atan2(y-40, x-40) * 180 / math.Pi
			if math.Abs(a+135) > 25 {
				continue
			}
			if dashedAt(ps, x, y) != dashedAt(ps, y, x) {
				t.Fatalf("(%v, %v) and its mirror (%v, %v) differ", x, y, y, x)
			}
		}
	}
	half := 6.0 / 38 * 180 / math.Pi
	for _, tc := range []struct {
		off  float64
		want bool
	}{{0, true}, {half - 0.3, true}, {-half + 0.3, true}, {half + 0.3, false}, {-half - 0.3, false}} {
		s, c := math.Sincos((225 + tc.off) * math.Pi / 180)
		if got := dashedAt(ps, 40+38*c, 40+38*s); got != tc.want {
			t.Errorf("%.2f degrees from the diagonal, in a dash is %v, want %v", tc.off, got, tc.want)
		}
	}
	// Half of the middle of the border is dashes: sampled round it.
	in, all := 0, 0
	for x := 40.0; x < 160; x += 0.1 {
		all++
		if dashedAt(ps, x, 2) {
			in++
		}
	}
	for a := 180.0; a < 270; a += 0.05 {
		all++
		s, c := math.Sincos(a * math.Pi / 180)
		if dashedAt(ps, 40+38*c, 40+38*s) {
			in++
		}
	}
	if f := float64(in) / float64(all); math.Abs(f-0.5) > 0.05 {
		t.Errorf("%.3f of the middle of the border is dashes, want about a half", f)
	}
}

// TestAMarkIsCentredOnASquareCornersMitre: where a rounded box has a square
// corner, the two sides meet on its mitre, and the dot there is centred on the
// middle of the mitre.
func TestAMarkIsCentredOnASquareCornersMitre(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 192px; height: 92px;
		border: 4px dotted blue; border-top-left-radius: 30px }`)
	found := false
	for _, side := range dotsOf(t, ops) {
		for _, d := range side {
			// The top right corner is square: its mitre runs from (200, 0)
			// to (196, 4), and its middle is (198, 2).
			if math.Abs(d.x-198) < 0.05 && math.Abs(d.y-2) < 0.05 {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no dot is centred on the square corner's mitre: %v", dotsOf(t, ops))
	}
}

// TestADottedSideIsDrawnInsideItsRegion: the dots at a side's two ends are
// halves, cut by the line the side meets the next on, and so every dot is drawn
// inside the side's region. A dashed side needs no clip: each dash is a piece
// of the band.
func TestADottedSideIsDrawnInsideItsRegion(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 192px; height: 92px;
		border: 6px dotted blue; border-radius: 30px 10px }`)
	f := radiusFragment(t, `<div id="a"></div>`, noDefaults+`#a { width: 192px; height: 92px;
		border: 6px dotted blue; border-radius: 30px 10px }`, "a")
	at := transitions(f.Border)
	gs := groupsOf(ops)
	if len(gs) != 4 {
		t.Fatalf("%d groups, want a clip per side", len(gs))
	}
	for i, g := range gs {
		want := sideRegion(f.BorderRect, f.radii, f.BorderRect.Inset(f.Border), f.paddingRadii(), side(i), at)
		if g.Path.String() != want.String() {
			t.Errorf("side %d's dots are clipped to %s, want its region %s", i, g.Path, want)
		}
	}
	ops = paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 192px; height: 92px;
		border: 6px dashed blue; border-radius: 30px 10px }`)
	if n := len(groupsOf(ops)); n != 0 {
		t.Errorf("a dashed side was clipped %d times", n)
	}
}

// TestDotsNeverOverlap: where a thin dotted side meets a much thicker one, the
// band widens round the corner between them, and the thin side's dots there are
// still apart.
func TestDotsNeverOverlap(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 200px; height: 100px;
		border-style: dotted; border-color: blue; border-width: 2px 2px 2px 30px; border-radius: 60px }`)
	for i, side := range dotsOf(t, ops) {
		for j := 1; j < len(side); j++ {
			a, b := side[j-1], side[j]
			if d := math.Hypot(a.x-b.x, a.y-b.y); d < a.r+b.r-0.05 {
				t.Errorf("side %d dots %d and %d overlap: %.3f apart with radii %.3f and %.3f", i, j-1, j, d, a.r, b.r)
			}
		}
	}
}

// TestARoundedSideTooShortForAGapIsSolid: a side whose track holds less than
// half a period is its region, solid.
func TestARoundedSideTooShortForAGapIsSolid(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 0; height: 0;
		border: 10px dashed blue; border-radius: 10px }`)
	f := radiusFragment(t, `<div id="a"></div>`, noDefaults+`#a { width: 0; height: 0;
		border: 10px dashed blue; border-radius: 10px }`, "a")
	at := transitions(f.Border)
	ps := pathsOf(ops)
	if len(ps) != 4 {
		t.Fatalf("%d paths, want four solid sides", len(ps))
	}
	for i, p := range ps {
		want := sideRegion(f.BorderRect, f.radii, f.BorderRect.Inset(f.Border), f.paddingRadii(), side(i), at)
		if p.Path.String() != want.String() {
			t.Errorf("side %d is %s, want its region solid", i, p.Path)
		}
	}
}

// TestRoundedMarksAreChargedToTheDocument: every mark is paid for before it is
// made, and past the budget the side is drawn solid and the budget says so.
func TestRoundedMarksAreChargedToTheDocument(t *testing.T) {
	lowWork(t, 400*costOp)
	built := Build(Input{HTML: `<div id="a"></div>`, CSS: []Stylesheet{{Source: noDefaults +
		`#a { width: 5000px; height: 20px; border: 1px dotted blue; border-radius: 8px }`}}})
	rec := NewRecorder(nil)
	ops := PaintReporting(Layout(built.Root, Size{W: rpx(6000), H: rpx(1000)}, nil, rec), rec)
	requireCut(t, rec.Findings(), "the dashes and dots of the borders past that point, drawn solid")
	if n := len(dotsOf(t, ops)); n > 2 {
		t.Errorf("%d sides were dotted past the budget", n)
	}
}

// TestRoundedMarksCostTheirLength: a rounded dotted or dashed border four times
// as long costs about four times as much to paint, whether its marks are on
// its corners, which are placed by searching the corner's table, or on its
// straight runs, where building the path is most of the work.
func TestRoundedMarksCostTheirLength(t *testing.T) {
	for _, tc := range []struct {
		what, style, radius string
		n                   int
	}{
		{"a dotted border round large corners", "dotted", "30%", 250},
		{"a dotted border along long sides", "dotted", "4px", 1000},
		{"a dashed border along long sides", "dashed", "4px", 1000},
	} {
		laid := func(n int) *Fragment {
			css := noDefaults + strings.NewReplacer("N", itoa(n), "S", tc.style, "R", tc.radius).Replace(
				`#a { width: Npx; height: Npx; border: 1px S blue; border-radius: R }`)
			built := Build(Input{HTML: `<div id="a"></div>`, CSS: []Stylesheet{{Source: css}}})
			return Layout(built.Root, Size{W: rpx(float64(5 * n)), H: rpx(float64(5 * n))}, nil, NewRecorder(nil))
		}
		small, large := laid(tc.n), laid(4*tc.n)
		c := costtest.Time(t, "painting "+tc.what, func() { Paint(small) }, func() { Paint(large) })
		if c.Ratio > 8 {
			t.Errorf("%s: four times the border cost %.1f times as much; want about four", tc.what, c.Ratio)
		}
	}
}

// TestADotIsNoWiderThanTheSpacing is roundedMarks's promise that no two dots
// overlap, asked of a band much thicker than the width the marks are spaced
// by: a one-pixel side's dots are two pixels apart, and a band ten pixels
// thick would make them ten wide.
func TestADotIsNoWiderThanTheSpacing(t *testing.T) {
	outer := Rect{X: 0, Y: 0, W: rpx(200), H: rpx(100)}
	oR := Radii{corner(30, 30), corner(30, 30), corner(30, 30), corner(30, 30)}
	e := Edges{Top: rpx(10), Right: rpx(10), Bottom: rpx(10), Left: rpx(10)}
	p := &painter{rec: NewRecorder(nil)}
	p.roundedMarks(outer, oR, outer.Inset(e), insetRadii(oR, e), sideTop, transitions(e), rpx(1), true, blue)
	sides := dotsOf(t, p.ops)
	if len(sides) != 1 || len(sides[0]) < 10 {
		t.Fatalf("the dots are %v", sides)
	}
	for j := 1; j < len(sides[0]); j++ {
		a, b := sides[0][j-1], sides[0][j]
		if d := math.Hypot(a.x-b.x, a.y-b.y); d < a.r+b.r-0.05 {
			t.Errorf("dots %d and %d overlap: %.3f apart with radii %.3f and %.3f", j-1, j, d, a.r, b.r)
		}
	}
}

// TestDotsAreEvenlySpacedRoundAnEllipse: round an elliptical corner a whole
// turn of direction is not a whole turn of length — a corner eighty pixels
// wide and twenty tall is long where it is flat and short where it bends — and
// the dots are spaced by length. Neighbouring dots are the same distance
// apart all the way round, to within what a chord loses on the curve.
func TestDotsAreEvenlySpacedRoundAnEllipse(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`, noDefaults+`#a { width: 300px; height: 100px;
		border: 2px dotted blue; border-radius: 120px / 40px }`)
	sides := dotsOf(t, ops)
	if len(sides) != 4 {
		t.Fatalf("%d sides of dots", len(sides))
	}
	for i, side := range sides {
		var lo, hi float64 = math.Inf(1), 0
		for j := 1; j < len(side); j++ {
			d := math.Hypot(side[j].x-side[j-1].x, side[j].y-side[j-1].y)
			lo, hi = math.Min(lo, d), math.Max(hi, d)
		}
		if hi-lo > 0.03*hi {
			t.Errorf("side %d's dots are from %.3f to %.3f apart, want one spacing", i, lo, hi)
		}
	}
}
