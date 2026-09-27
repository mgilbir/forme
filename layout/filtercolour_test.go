package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// Filter Effects 1's colour functions and drop-shadow(): see filtercolour.go.

// hexColour is a colour from six hex digits.
func hexColour(t *testing.T, s string) style.RGBA {
	t.Helper()
	c, ok := parseColorValue("#" + s)
	if !ok {
		t.Fatalf("#%s is not a colour", s)
	}
	return c
}

// filteredFill is the one fill a filtered box of one background paints.
func filteredFill(t *testing.T, bg, filter string) (style.RGBA, []Op) {
	t.Helper()
	ops := paintOf(t, `<div id="d"></div>`, noDefaults+`#d { width: 100px; height: 50px; background: `+bg+`; filter: `+filter+` }`)
	var fills []FillRect
	for _, op := range ops {
		if f, ok := op.(FillRect); ok {
			fills = append(fills, f)
		}
	}
	if len(fills) != 1 || len(filterGroupsOf(ops)) != 0 {
		t.Fatalf("%s on %s painted %v, want one fill and no group", filter, bg, ops)
	}
	return fills[0].Color, ops
}

// within8Bit reports whether two colours are the same 8-bit colour, to the
// rounding a device does.
func within8Bit(a, b style.RGBA) bool {
	return math.Abs(a.R-b.R) <= 0.5 && math.Abs(a.G-b.G) <= 0.5 && math.Abs(a.B-b.B) <= 0.5 &&
		math.Abs(a.A-b.A) <= 0.5/255
}

// TestTheColourFunctionsAreTheSuitesReferences is the suite's filter-effects
// tests, each a box of one colour filtered and a reference of the colour that
// makes, which Chrome, Firefox and Safari pass: grayscale-001 and -004 (blue
// to #121212) and -005 (300% clamped to the same), contrast-002 (0% is middle
// gray) and -003 (#3f0000 at 200% is black, clamped), invert-002 (yellow to
// blue), hue_rotate-002 (red turned 120deg is #007100, clamped), and
// saturate-001 (yellow at 0% is #ededed). And the identities: -001's
// contrast(100%), hue-rotate(0deg) and invert(0%), and grayscale's 0%.
func TestTheColourFunctionsAreTheSuitesReferences(t *testing.T) {
	for _, tc := range []struct{ bg, filter, want string }{
		{"blue", "grayscale(100%)", "121212"},
		{"blue", "grayscale(1)", "121212"},
		{"blue", "grayscale(300%)", "121212"},
		{"red", "contrast(0%)", "808080"},
		{"#3f0000", "contrast(200%)", "000000"},
		{"yellow", "invert(100%)", "0000ff"},
		{"red", "hue-rotate(120deg)", "007100"},
		{"yellow", "saturate( 0% )", "ededed"},
		{"green", "contrast(100%)", "008000"},
		{"green", "hue-rotate(0deg)", "008000"},
		{"yellow", "invert(0%)", "ffff00"},
		{"#121212", "grayscale(0%)", "121212"},
		// And the rest of §13.1 at a value whose answer is plain: sepia(1)
		// of blue is its matrix's third column, brightness(0.5) of white is half, and
		// invert(0.5) of anything is middle gray.
		{"blue", "sepia(1)", "302b21"},
		{"white", "brightness(0.5)", "808080"},
		{"red", "invert(0.5)", "808080"},
		// calc() is read where the grammar allows it.
		{"red", "hue-rotate(calc(60deg * 2))", "007100"},
		{"red", "contrast(calc(50% - 0.5 * 100%))", "808080"},
	} {
		got, _ := filteredFill(t, tc.bg, tc.filter)
		if want := hexColour(t, tc.want); !within8Bit(got, want) {
			t.Errorf("%s on %s is %v, want #%s", tc.filter, tc.bg, got, tc.want)
		}
	}
}

// TestTheMatricesAreSection13s holds the matrices to §13.1's formulas where they
// are not the identity or a clamp: every row of hue-rotate() and saturate() sums
// to one, so a gray stays itself at any angle or amount, and §9.6's own worked
// term a00 = 0.213 + cos θ × 0.787 − sin θ × 0.213.
func TestTheMatricesAreSection13s(t *testing.T) {
	for _, deg := range []float64{0, 37, 90, 180, 300, -45} {
		m := hueRotateMatrix(deg)
		for row := 0; row < 3; row++ {
			if sum := m[row*5] + m[row*5+1] + m[row*5+2]; math.Abs(sum-1) > 1e-12 {
				t.Errorf("hue-rotate(%vdeg) row %d sums to %v", deg, row, sum)
			}
		}
		s, c := math.Sincos(deg * math.Pi / 180)
		if want := 0.213 + c*0.787 - s*0.213; math.Abs(m[0]-want) > 1e-12 {
			t.Errorf("hue-rotate(%vdeg): a00 is %v, want %v", deg, m[0], want)
		}
	}
	for _, a := range []float64{0, 0.3, 1, 2.5} {
		m := saturateMatrix(a)
		for row := 0; row < 3; row++ {
			if sum := m[row*5] + m[row*5+1] + m[row*5+2]; math.Abs(sum-1) > 1e-12 {
				t.Errorf("saturate(%v) row %d sums to %v", a, row, sum)
			}
		}
	}
	for _, m := range [][20]float64{grayscaleMatrix(0), sepiaMatrix(0), saturateMatrix(1), hueRotateMatrix(0),
		invertMatrix(0), brightnessMatrix(1), contrastMatrix(1)} {
		for i := range m {
			if math.Abs(m[i]-identityMatrix[i]) > 1e-12 {
				t.Errorf("a function at its identity amount is %v", m)
				break
			}
		}
	}
	// And none of them touches alpha.
	for _, m := range [][20]float64{grayscaleMatrix(0.4), sepiaMatrix(0.7), saturateMatrix(3),
		hueRotateMatrix(200), invertMatrix(0.2), brightnessMatrix(2), contrastMatrix(0.3)} {
		if m[3] != 0 || m[8] != 0 || m[13] != 0 || m[15] != 0 || m[16] != 0 || m[17] != 0 || m[18] != 1 || m[19] != 0 {
			t.Errorf("a colour function reads or writes alpha: %v", m)
		}
	}
}

// TestAColourFunctionIsAppliedInOrder: the functions are applied in the order
// written, each clamping (§9.1), so brightness(2) then brightness(0.5) is not
// the identity on a light colour: #c0c0c0 doubled is white, and white halved is
// #808080. The other way round it is #c0c0c0 again.
func TestAColourFunctionIsAppliedInOrder(t *testing.T) {
	got, _ := filteredFill(t, "#c0c0c0", "brightness(2) brightness(0.5)")
	if !within8Bit(got, hexColour(t, "808080")) {
		t.Errorf("brightness(2) brightness(0.5) of #c0c0c0 is %v, want #808080", got)
	}
	got, _ = filteredFill(t, "#c0c0c0", "brightness(0.5) brightness(2)")
	if !within8Bit(got, hexColour(t, "c0c0c0")) {
		t.Errorf("brightness(0.5) brightness(2) of #c0c0c0 is %v, want #c0c0c0", got)
	}
}

// TestAMatrixIsFoldedWhereItIsExact: over marks whose colours it keeps inside
// [0, 1], a matrix is each mark's colour mapped, text and gradients included,
// and the blur beside it is still the group; where it clamps a colour a
// translucent mark over another mixes, and it stays in the group.
func TestAMatrixIsFoldedWhereItIsExact(t *testing.T) {
	ops := paintOf(t, `<div id="d"><div id="i"></div>x</div>`, noDefaults+`
		#d { width: 100px; height: 50px; background: linear-gradient(red, blue); color: #336699;
			filter: blur(2px) grayscale(1) }
		#i { height: 10px; background: rgba(0, 128, 0, 0.5) }`)
	gs := filterGroupsOf(ops)
	if len(gs) != 1 || len(gs[0].Filters) != 1 || gs[0].Filters[0].Kind != FilterBlur {
		t.Fatalf("the groups are %+v, want one blur", gs)
	}
	gray := func(c style.RGBA) bool { return math.Abs(c.R-c.G) < 1e-9 && math.Abs(c.G-c.B) < 1e-9 }
	sawText, sawStop := false, false
	eachColour(gs[0].Ops, func(c style.RGBA) bool {
		if !gray(c) {
			t.Errorf("a colour in the group is %v, not gray", c)
		}
		return true
	})
	for _, op := range gs[0].Ops {
		switch v := op.(type) {
		case DrawText:
			sawText = true
			// 0.2126 × 0x33 + 0.7152 × 0x66 + 0.0722 × 0x99
			if want := 0.2126*0x33 + 0.7152*0x66 + 0.0722*0x99; math.Abs(v.Color.R-want) > 1e-9 {
				t.Errorf("the text is %v, want gray %v", v.Color, want)
			}
		case FillGradient:
			sawStop = true
		}
	}
	if !sawText || !sawStop {
		t.Errorf("the group holds %v, want the text and the gradient recoloured", gs[0].Ops)
	}
	// Clamped, with a translucent mark over another: the matrix stays.
	ops = paintOf(t, `<div id="d"><div id="i"></div></div>`, noDefaults+`
		#d { width: 100px; height: 50px; background: #c0c0c0; filter: brightness(2) }
		#i { height: 10px; background: rgba(0, 0, 255, 0.5) }`)
	gs = filterGroupsOf(ops)
	if len(gs) != 1 || len(gs[0].Filters) != 1 || gs[0].Filters[0].Kind != FilterColorMatrix ||
		gs[0].Filters[0].Matrix != brightnessMatrix(2) {
		t.Fatalf("the groups are %+v, want the brightness left in one", gs)
	}
	// And a matrix after one left in the group is not folded into what the
	// group holds, which the first has not yet been applied to.
	ops = paintOf(t, `<div id="d"><div id="i"></div></div>`, noDefaults+`
		#d { width: 100px; height: 50px; background: #c0c0c0; filter: brightness(2) grayscale(1) }
		#i { height: 10px; background: rgba(0, 0, 255, 0.5) }`)
	gs = filterGroupsOf(ops)
	if len(gs) != 1 || len(gs[0].Filters) != 2 || gs[0].Filters[1].Matrix != grayscaleMatrix(1) {
		t.Fatalf("the groups are %+v, want the brightness and then the grayscale in one", gs)
	}
	eachColour(gs[0].Ops, func(c style.RGBA) bool {
		if c == (style.RGBA{B: 255, A: 0.5}) || c == hexColour(t, "c0c0c0") {
			return true
		}
		t.Errorf("a colour inside the group is %v, recoloured before the brightness", c)
		return true
	})
	// Clamped, and nothing mixes: folded, each colour clamped.
	ops = paintOf(t, `<div id="d"><div id="i"></div></div>`, noDefaults+`
		#d { width: 100px; height: 50px; background: #c0c0c0; filter: brightness(2) }
		#i { height: 10px; background: #000080 }`)
	if gs := filterGroupsOf(ops); len(gs) != 0 {
		t.Errorf("an opaque mark over another made a group: %+v", gs)
	}
	if len(fillsOf(ops, style.RGBA{R: 255, G: 255, B: 255, A: 1})) != 1 ||
		len(fillsOf(ops, style.RGBA{B: 255, A: 1})) != 1 {
		t.Errorf("the fills are %v, want white and blue", ops)
	}
}

// TestAMatrixOverAPictureIsTheBackends: a picture's pixels are not rewritten,
// so a colour function over one stays in the group.
func TestAMatrixOverAPictureIsTheBackends(t *testing.T) {
	ops := []Op{DrawImage{Rect: Rect{W: rpx(10), H: rpx(10)}}, FillRect{Rect: Rect{W: rpx(5), H: rpx(5)}, Color: blue}}
	if _, ok := foldMatrix(ops, grayscaleMatrix(1)); ok {
		t.Error("a matrix was folded over a picture")
	}
	out, _, _ := testPainter().applyFilters([]FilterFunction{{Kind: FilterColorMatrix, Matrix: grayscaleMatrix(1)}}, ops)
	if len(out) != 1 {
		t.Fatalf("the filtered picture is %v", out)
	}
	if g, ok := out[0].(FilterGroup); !ok || g.Filters[0].Kind != FilterColorMatrix {
		t.Errorf("the filtered picture is %v, want a group with the matrix", out)
	}
}

// TestADropShadowIsAShadowOfTheGroup is §13.1.10: the group's alpha, moved by
// the offset, flooded with the colour and blurred by the deviation (the third
// length itself, not half of it), drawn under the group. Two marks that overlap
// cast one shadow, not two, so the shadow's colour's alpha is the group's and
// not each mark's.
func TestADropShadowIsAShadowOfTheGroup(t *testing.T) {
	ops := paintOf(t, `<div id="d"><div id="i"></div></div>`, noDefaults+`
		#d { width: 100px; height: 50px; background: red; filter: drop-shadow(rgba(0, 0, 255, 0.5) 5px 7px 3px) }
		#i { width: 20px; height: 10px; background: rgba(0, 255, 0, 0.25) }`)
	gs := filterGroupsOf(ops)
	if len(gs) != 1 {
		t.Fatalf("the groups are %+v, want the shadow", gs)
	}
	g := gs[0]
	if len(g.Filters) != 2 || g.Filters[0].Kind != FilterBlur || g.Filters[0].StdDev != rpx(3) ||
		g.Filters[1].Kind != FilterOpacity || g.Filters[1].Amount != 0.5 {
		t.Errorf("the shadow's chain is %+v, want a 3px deviation and half its alpha", g.Filters)
	}
	var shadow []FillRect
	for _, op := range g.Ops {
		if f, ok := op.(FillRect); ok {
			shadow = append(shadow, f)
		}
	}
	if len(shadow) != 2 || shadow[0].Rect != (Rect{X: rpx(5), Y: rpx(7), W: rpx(100), H: rpx(50)}) ||
		shadow[1].Rect != (Rect{X: rpx(5), Y: rpx(7), W: rpx(20), H: rpx(10)}) {
		t.Errorf("the shadow's marks are %v, want both moved by (5, 7)", shadow)
	}
	for i, want := range []float64{1, 0.25} {
		if i < len(shadow) && shadow[i].Color != (style.RGBA{B: 255, A: want}) {
			t.Errorf("shadow mark %d is %v, want blue at the mark's own alpha, %v", i, shadow[i].Color, want)
		}
	}
	// Under the group: the shadow first, and then the marks as they were.
	iShadow, iRed := -1, -1
	for i, op := range ops {
		switch v := op.(type) {
		case FilterGroup:
			iShadow = i
		case FillRect:
			if v.Color == red && iRed < 0 {
				iRed = i
			}
		}
	}
	if iShadow < 0 || iRed < iShadow {
		t.Errorf("the shadow is at %d and the marks at %d; it goes under them", iShadow, iRed)
	}
	// A sharp opaque shadow is its marks, and a colour left out is the text's.
	ops = paintOf(t, `<div id="d">x</div>`, noDefaults+`
		#d { width: 100px; height: 50px; color: #123456; background: red; filter: drop-shadow(2px 2px) }`)
	if len(filterGroupsOf(ops)) != 0 {
		t.Errorf("a sharp opaque shadow made a group: %v", ops)
	}
	c := hexColour(t, "123456")
	sawRect, sawText, texts := false, false, 0
	for _, op := range ops {
		switch v := op.(type) {
		case FillRect:
			sawRect = sawRect || (v.Color == c && v.Rect.X == rpx(2) && v.Rect.Y == rpx(2))
		case DrawTextShadow:
			sawText = sawText || (v.Run.Color == c && v.StdDev == 0)
		case DrawText:
			texts++
		}
	}
	if texts != 1 {
		t.Errorf("%d runs of text, want the one the box holds: the shadow of text is not text", texts)
	}
	if !sawRect || !sawText {
		t.Errorf("the shadow is %v, want the fill and the text moved and in the text's colour", ops)
	}
}

// TestAShadowIsFilteredByWhatFollowsIt: a function after drop-shadow() applies
// to the shadow as well — grayscale after a blue shadow is a gray one.
func TestAShadowIsFilteredByWhatFollowsIt(t *testing.T) {
	ops := paintOf(t, `<div id="d"></div>`, noDefaults+`
		#d { width: 100px; height: 50px; background: red; filter: drop-shadow(blue 4px 4px) grayscale(1) }`)
	for _, op := range ops {
		if f, ok := op.(FillRect); ok && !(math.Abs(f.Color.R-f.Color.G) < 1e-9 && math.Abs(f.Color.G-f.Color.B) < 1e-9) {
			t.Errorf("a mark is %v after the grayscale", f.Color)
		}
	}
	if len(fillsOf(ops, style.RGBA{R: 0.0722 * 255, G: 0.0722 * 255, B: 0.0722 * 255, A: 1})) != 1 {
		t.Errorf("the shadow is not blue gone gray: %v", ops)
	}
}

// TestADropShadowOverAPictureIsTheBackends: a picture's alpha is not marks, so
// its shadow stays in the group, which reaches as far as the shadow does.
func TestADropShadowOverAPictureIsTheBackends(t *testing.T) {
	ops := []Op{DrawImage{Rect: Rect{W: rpx(10), H: rpx(10)}}}
	f := FilterFunction{Kind: FilterDropShadow, Offset: Point{X: rpx(20), Y: rpx(5)}, StdDev: rpx(2), Color: blue}
	out, _, _ := testPainter().applyFilters([]FilterFunction{f}, ops)
	g, ok := out[0].(FilterGroup)
	if len(out) != 1 || !ok || g.Filters[0].Kind != FilterDropShadow {
		t.Fatalf("the shadow of a picture is %v, want a group with it", out)
	}
	// The picture, and the shadow 20 across and 5 down, grown by three
	// deviations, 6, on every side.
	if want := (Rect{X: 0, Y: rpx(-1), W: rpx(36), H: rpx(22)}); g.Extent() != want {
		t.Errorf("the group reaches %v, want %v", g.Extent(), want)
	}
}

// TestTheColourFunctionsAreNotReported: every function is applied, and a url()
// is still reported by name, with what is beside it applied.
func TestTheColourFunctionsAreNotReported(t *testing.T) {
	for _, f := range []string{"grayscale(1)", "sepia(0.5)", "saturate(3)", "hue-rotate(1turn)", "invert(0.2)",
		"brightness(150%)", "contrast(0.8)", "drop-shadow(1px 1px 2px red)", "blur(2px) grayscale(1) opacity(0.5)",
		"contrast(calc(0.5))", "saturate(calc(50% + 10%))", "hue-rotate(calc(10deg * 2))"} {
		_, findings := filterFindings(t, `<div id="d">x</div>`, `#d { height: 20px; background: red; filter: `+f+` }`)
		if hasRule(findings, RuleUnsupportedValue) || hasRule(findings, RuleUnsupportedProperty) {
			t.Errorf("%s was reported: %v", f, findings)
		}
	}
	ops, findings := filterFindings(t, `<div id="d">x</div>`,
		`#d { height: 20px; background: red; filter: blur(2px) url(#f) }`)
	if gs := filterGroupsOf(ops); len(gs) != 1 || gs[0].Filters[0].StdDev != rpx(2) {
		t.Errorf("the blur beside a url() was not applied: %+v", gs)
	}
	said := ""
	for _, f := range findings {
		if f.Rule == RuleUnsupportedValue && f.Property == "filter" {
			said = f.Message
		}
	}
	if !strings.Contains(said, "url()") || strings.Contains(said, "blur") {
		t.Errorf("the report is %q, want it to name url() alone", said)
	}
}

// TestAChainKeepsItsOrder: a group's chain is applied in order, and adding a
// blur or an opacity to one merges it into the run it ends only where they
// commute: a blur goes ahead of a trailing opacity, not ahead of a matrix.
func TestAChainKeepsItsOrder(t *testing.T) {
	m := FilterFunction{Kind: FilterColorMatrix, Matrix: brightnessMatrix(3)}
	blur := func(px float64) FilterFunction { return FilterFunction{Kind: FilterBlur, StdDev: rpx(px)} }
	op := func(a float64) FilterFunction { return FilterFunction{Kind: FilterOpacity, Amount: a} }
	for _, tc := range []struct {
		chain []FilterFunction
		add   FilterFunction
		want  []FilterFunction
	}{
		{[]FilterFunction{blur(3)}, blur(4), []FilterFunction{blur(5)}},
		{[]FilterFunction{blur(3), op(0.5)}, blur(4), []FilterFunction{blur(5), op(0.5)}},
		{[]FilterFunction{op(0.5)}, blur(2), []FilterFunction{blur(2), op(0.5)}},
		{[]FilterFunction{op(0.5)}, op(0.5), []FilterFunction{op(0.25)}},
		{[]FilterFunction{blur(1), m}, blur(2), []FilterFunction{blur(1), m, blur(2)}},
		{[]FilterFunction{m, op(0.5)}, blur(2), []FilterFunction{m, blur(2), op(0.5)}},
		{[]FilterFunction{op(0.5)}, m, []FilterFunction{op(0.5), m}},
	} {
		got := appendFilter(tc.chain, tc.add)
		if len(got) != len(tc.want) {
			t.Errorf("%+v then %+v is %+v, want %+v", tc.chain, tc.add, got, tc.want)
			continue
		}
		for i := range got {
			if got[i].Kind != tc.want[i].Kind || got[i].StdDev != tc.want[i].StdDev ||
				math.Abs(got[i].Amount-tc.want[i].Amount) > 1e-12 {
				t.Errorf("%+v then %+v is %+v, want %+v", tc.chain, tc.add, got, tc.want)
				break
			}
		}
	}
}

// TestAFilterChainOfManyMarksCostsTheirNumber: folding a matrix and casting a
// shadow over four times the marks costs about four times as much.
func TestAFilterChainOfManyMarksCostsTheirNumber(t *testing.T) {
	laid := func(n int) *Fragment {
		built := Build(Input{HTML: `<div id="d">` + strings.Repeat(`<p></p>`, n) + `</div>`, CSS: []Stylesheet{{Source: noDefaults +
			`#d { filter: grayscale(1) drop-shadow(red 2px 2px 1px) sepia(0.5) } p { height: 2px; background: #336699; margin: 0 0 1px }`}}})
		return Layout(built.Root, Size{W: rpx(600), H: rpx(100000)}, nil, NewRecorder(nil))
	}
	small, large := laid(500), laid(2000)
	c := costtest.Time(t, "filtering n marks", func() { Paint(small) }, func() { Paint(large) })
	if c.Ratio > 8 {
		t.Errorf("four times the marks cost %.1f times as much; want about four", c.Ratio)
	}
}

// TestAMathFunctionIsClampedNotRefused: CSS Values 4 enforces a math
// function's range by clamping, so a calc() that comes to a negative amount or
// deviation is nothing: contrast(calc(-1)) is contrast(0), middle gray, and
// blur(calc(1px - 3px)) blurs nothing.
func TestAMathFunctionIsClampedNotRefused(t *testing.T) {
	got, _ := filteredFill(t, "red", "contrast(calc(-1))")
	if !within8Bit(got, hexColour(t, "808080")) {
		t.Errorf("contrast(calc(-1)) of red is %v, want middle gray", got)
	}
	ops := paintOf(t, `<div id="d"></div>`, noDefaults+`#d { height: 20px; background: red; filter: blur(calc(1px - 3px)) }`)
	if len(filterGroupsOf(ops)) != 0 || len(fillsOf(ops, red)) != 1 {
		t.Errorf("blur(calc(1px - 3px)) is %v, want the box unblurred", ops)
	}
}

// TestAShadowIsChargedToTheDocument: a drop shadow is as many operations again
// as what it is a shadow of, and they are charged as painting charges a mark.
func TestAShadowIsChargedToTheDocument(t *testing.T) {
	spent := func(filter string) int64 {
		built := Build(Input{HTML: `<div id="d">` + strings.Repeat(`<p></p>`, 100) + `</div>`, CSS: []Stylesheet{{Source: noDefaults +
			`#d { filter: ` + filter + ` } p { height: 2px; background: #336699; margin: 0 0 1px }`}}})
		rec := NewRecorder(nil)
		before := rec.work.left
		PaintReporting(Layout(built.Root, Size{W: rpx(600), H: rpx(10000)}, nil, rec), rec)
		return before - rec.work.left
	}
	plain, sharp, blurred := spent("none"), spent("drop-shadow(red 2px 2px)"), spent("drop-shadow(red 2px 2px 1px)")
	if sharp-plain < 100*costOp {
		t.Errorf("a sharp shadow of 100 marks cost %d steps more, want at least 100 marks' worth (%d)", sharp-plain, 100*costOp)
	}
	if blurred-plain < 101*costOp {
		t.Errorf("a blurred shadow of 100 marks cost %d steps more, want the marks and their group (%d)", blurred-plain, 101*costOp)
	}
}

// TestADropShadowOnAnInlineBoxInsideAClipIsReported: like a blur, a shadow
// reaches past the marks, and an inline box's marks are cut by its block's
// clip before the group is formed.
func TestADropShadowOnAnInlineBoxInsideAClipIsReported(t *testing.T) {
	_, findings := filterFindings(t, `<p id="o">a <span id="s">b</span> c</p>`,
		`#o { overflow: hidden; height: 50px } #s { filter: drop-shadow(2px 2px) }`)
	found := false
	for _, f := range findings {
		found = found || strings.Contains(f.Message, "cut before it was filtered")
	}
	if !found {
		t.Errorf("a shadowed inline box inside a clip was not reported: %v", findings)
	}
}

// TestTheComparisonKeysWhatItCannotRender: the reftest comparison renders a
// blur and an opacity, and a colour matrix or a drop shadow left in a group it
// does not: such a group is one mark keyed by its chain and what it holds, so
// it is never taken for the same marks unfiltered, nor for another matrix.
func TestTheComparisonKeysWhatItCannotRender(t *testing.T) {
	sq := FillRect{Rect: Rect{W: rpx(20), H: rpx(20)}, Color: blue}
	gray := FilterGroup{Filters: []FilterFunction{{Kind: FilterColorMatrix, Matrix: grayscaleMatrix(1)}}, Ops: []Op{sq}}
	sepia := FilterGroup{Filters: []FilterFunction{{Kind: FilterColorMatrix, Matrix: sepiaMatrix(1)}}, Ops: []Op{sq}}
	if pictureEqual([]Op{gray}, []Op{sq}, picPage) {
		t.Error("a grayscale group compared equal to the square unfiltered")
	}
	if pictureEqual([]Op{gray}, []Op{sepia}, picPage) {
		t.Error("a grayscale group compared equal to a sepia one")
	}
	if !pictureEqual([]Op{gray}, []Op{gray}, picPage) {
		t.Error("a group did not compare equal to itself")
	}
}

// testPainter is a painter with nothing painted yet, for a test that applies
// a chain to operations it made itself.
func testPainter() *painter {
	return &painter{rec: NewRecorder(nil), colors: map[string]style.RGBA{}}
}

// TestEachFunctionIsItsPrimitive holds each function to §13.1's primitive
// written out another way, on rgb(20% 40% 60%), which none of them clamps:
// brightness() and contrast() are feFuncs of type linear (C' = slope × C +
// intercept), invert() one of type table with two values (C' = v0 + C × (v1 −
// v0)), grayscale() and sepia() are their full matrix mixed with the identity
// by the amount, saturate() the identity mixed with §9.6's luminance by it, and
// hue-rotate(180deg) is §9.6's luminance matrix twice less the identity, since
// cos is −1 and sin nothing.
func TestEachFunctionIsItsPrimitive(t *testing.T) {
	c := [3]float64{0.2, 0.4, 0.6}
	in := style.RGBA{R: 0.2 * 255, G: 0.4 * 255, B: 0.6 * 255, A: 0.7}
	each := func(f func(v float64) float64) [3]float64 { return [3]float64{f(c[0]), f(c[1]), f(c[2])} }
	mix := func(m [3][3]float64, a float64) [3]float64 {
		var out [3]float64
		for i := range out {
			full := m[i][0]*c[0] + m[i][1]*c[1] + m[i][2]*c[2]
			out[i] = a*full + (1-a)*c[i]
		}
		return out
	}
	lum := func(w [3]float64) float64 { return w[0]*c[0] + w[1]*c[1] + w[2]*c[2] }
	rec709 := [3]float64{0.2126, 0.7152, 0.0722}
	svg := [3]float64{0.213, 0.715, 0.072}
	for _, tc := range []struct {
		name string
		m    [20]float64
		want [3]float64
	}{
		{"brightness(1.5)", brightnessMatrix(1.5), each(func(v float64) float64 { return 1.5 * v })},
		{"contrast(0.3)", contrastMatrix(0.3), each(func(v float64) float64 { return 0.3*v - 0.5*0.3 + 0.5 })},
		{"invert(0.2)", invertMatrix(0.2), each(func(v float64) float64 { return 0.2 + v*(0.8-0.2) })},
		{"grayscale(0.4)", grayscaleMatrix(0.4), mix([3][3]float64{rec709, rec709, rec709}, 0.4)},
		{"sepia(0.7)", sepiaMatrix(0.7), mix([3][3]float64{{0.393, 0.769, 0.189}, {0.349, 0.686, 0.168}, {0.272, 0.534, 0.131}}, 0.7)},
		{"saturate(0.25)", saturateMatrix(0.25), each(func(v float64) float64 { return 0.25*v + 0.75*lum(svg) })},
		{"hue-rotate(180deg)", hueRotateMatrix(180), each(func(v float64) float64 { return 2*lum(svg) - v })},
	} {
		r, g, b, a := applyMatrix(tc.m, in)
		got := [3]float64{r, g, b}
		for i := range got {
			if math.Abs(got[i]-tc.want[i]) > 1e-12 {
				t.Errorf("%s of rgb(20%% 40%% 60%%) is %v, want %v", tc.name, got, tc.want)
				break
			}
		}
		if a != in.A {
			t.Errorf("%s changed alpha from %v to %v", tc.name, in.A, a)
		}
	}
}

// TestTheSuitesDropShadows are three of the suite's filter-effects reftests,
// which Chrome, Firefox and Safari pass, at the positions the page gives them:
// filters-drop-shadow-001 (a box's sharp shadow of its own colour is a second
// box), -003 (a box off the page casts its shadow onto it) and
// drop-shadow-clipped-001 (a box cut away by the clip around the filtered one
// casts a shadow inside it: the clip is applied after the filter).
func TestTheSuitesDropShadows(t *testing.T) {
	for _, tc := range []struct{ name, test, testCSS, ref, refCSS string }{
		{"filters-drop-shadow-001",
			`<div id="ng"></div><div id="ok"></div>`,
			`#ok { position: absolute; top: 100px; left: 0; width: 200px; height: 200px;
				background-color: rgb(0, 255, 0); filter: drop-shadow(20px 10px rgb(0, 255, 0)) }
			#ng { position: absolute; top: 110px; left: 20px; width: 200px; height: 200px;
				background-color: rgb(255, 0, 0) }`,
			`<div id="s"></div><div id="b"></div>`,
			`#b { position: absolute; top: 100px; left: 0; width: 200px; height: 200px; background-color: rgb(0, 255, 0) }
			#s { position: absolute; top: 110px; left: 20px; width: 200px; height: 200px; background-color: rgb(0, 255, 0) }`},
		{"filters-drop-shadow-003",
			`<div></div>`,
			`div { width: 300px; height: 300px; top: -1000px; left: -1000px; background-color: red;
				position: relative; filter: drop-shadow(1000px 1000px 0 green) }`,
			`<div></div>`,
			`div { width: 300px; height: 300px; background-color: green }`},
		{"drop-shadow-clipped-001",
			`<div id="o"><div id="f"><div id="r"></div></div></div>`,
			`#o { overflow: hidden; width: 100px; height: 100px }
			#f { filter: drop-shadow(-105px 0 0 green) }
			#r { width: 50px; height: 50px; position: relative; left: 105px; background: red }`,
			`<div></div>`,
			`div { width: 50px; height: 50px; background: green }`},
	} {
		test := paintOf(t, tc.test, noDefaults+tc.testCSS)
		ref := paintOf(t, tc.ref, noDefaults+tc.refCSS)
		if !pictureEqual(test, ref, picPage) {
			t.Errorf("%s does not match its reference:\n%v\n%v", tc.name, test, ref)
		}
	}
}

// TestATransparentShadowIsNothing: flooded with transparency, a drop shadow
// adds nothing under the group, and the box is painted as it is without it.
func TestATransparentShadowIsNothing(t *testing.T) {
	const box = `#d { width: 100px; height: 50px; background: red`
	plain := paintOf(t, `<div id="d"></div>`, noDefaults+box+` }`)
	for _, f := range []string{"drop-shadow(transparent 5px 5px 2px)", "drop-shadow(5px 5px rgba(0, 0, 255, 0))"} {
		got := paintOf(t, `<div id="d"></div>`, noDefaults+box+`; filter: `+f+` }`)
		if len(got) != len(plain) || len(filterGroupsOf(got)) != 0 {
			t.Errorf("%s painted %v, want %v", f, got, plain)
		}
	}
}

// TestShadowsOfShadowsCostTheirAllowance: each drop shadow is a shadow of
// everything before it, the shadows before it included, so a chain of them
// doubles what the group holds at each step. The passes are paid from the
// allowance filterPass keeps, and past it a shadow is left in its group:
// sixteen of them on one box, which would be 65,536 marks, make at most the
// allowance in marks, the rest are the backend's, and the refusal is reported.
func TestShadowsOfShadowsCostTheirAllowance(t *testing.T) {
	chain := strings.TrimSpace(strings.Repeat("drop-shadow(red 1px 1px) ", 16))
	ops, findings := filterFindings(t, `<div id="d"></div>`, `#d { width: 100px; height: 50px; background: blue; filter: `+chain+` }`)
	n, _ := countOpsUpTo(ops, 1<<40)
	if n > 2*filterRewriteFloor {
		t.Errorf("sixteen shadows made %d operations, want at most about the allowance, %d", n, filterRewriteFloor)
	}
	left := 0
	for _, g := range filterGroupsOf(ops) {
		for _, f := range g.Filters {
			if f.Kind == FilterDropShadow {
				left++
			}
		}
	}
	if left == 0 {
		t.Errorf("no shadow was left for the backend: %d operations", n)
	}
	requireCut(t, findings, "the filters folded into the marks past that point, which were left in their groups")
}

// TestNestedColourFunctionsCostWhatTheyHold: a matrix is folded into
// everything the filtered box holds, a filtered box inside it included, so
// boxes nested n deep, each filtered with k functions, are k·n²/2 passes over
// a mark unless something bounds them. The parser bounds n, at a few hundred,
// and maxFilterFunctions bounds k, and their product is still a constant a
// document can make every mark pay tens of thousands of times over. Four times
// the nesting costs about four times as much, and the inner boxes' colours are
// still folded rather than all left to the backend.
func TestNestedColourFunctionsCostWhatTheyHold(t *testing.T) {
	filter := strings.TrimSpace(strings.Repeat("sepia(0.5) ", 16))
	laid := func(n int) *Fragment {
		src := strings.Repeat(`<div class="f"><p></p>`, n) + strings.Repeat(`</div>`, n)
		built := Build(Input{HTML: src, CSS: []Stylesheet{{Source: noDefaults +
			`.f { filter: ` + filter + `; padding-left: 1px } p { height: 2px; background: #336699 }`}}})
		return Layout(built.Root, Size{W: rpx(2000), H: rpx(10000)}, nil, NewRecorder(nil))
	}
	small, large := laid(50), laid(200)
	ops := Paint(large)
	if got, _ := countOpsUpTo(ops, 1<<40); got < 200 {
		t.Fatalf("%d operations painted, want the 200 boxes' at least", got)
	}
	if len(filterGroupsOf(ops)) == 200 {
		t.Fatal("nothing was folded: every box is a group")
	}
	c := costtest.Time(t, "painting n nested colour functions", func() { Paint(small) }, func() { Paint(large) })
	if c.Ratio > 8 {
		t.Errorf("four times the nesting cost %.1f times as much; want about four", c.Ratio)
	}
}

// TestAShadowUnderAnOpacityIsCheckedWithIt: an opacity is folded into each
// mark as it is painted, before the filter sees it, and a shadow cast of those
// marks is dimmed with them. Where the shadow lies over the marks the two are
// drawn one through the other rather than dimmed as one, and the opacity's
// report says so — for the box's own opacity, one around it, and an inline
// box's. Apart, nothing is reported, and a colour function, which reads no
// alpha, is exact under any opacity.
func TestAShadowUnderAnOpacityIsCheckedWithIt(t *testing.T) {
	for _, tc := range []struct {
		doc, css string
		want     bool
	}{
		{`<div id="d"></div>`, `#d { width: 100px; height: 50px; opacity: .5; background: red; filter: drop-shadow(blue 5px 5px) }`, true},
		{`<div id="d"></div>`, `#d { width: 100px; height: 50px; opacity: .5; background: red; filter: drop-shadow(blue 5px 5px 2px) }`, true},
		{`<div id="o"><div id="d"></div></div>`, `#o { opacity: .5 } #d { width: 100px; height: 50px; background: red; filter: drop-shadow(blue 5px 5px) }`, true},
		{`<p><span id="d">xx</span></p>`, `#d { opacity: .5; filter: drop-shadow(blue 1px 1px) }`, true},
		{`<div id="d"></div>`, `#d { width: 100px; height: 50px; opacity: .5; background: red; filter: drop-shadow(blue 200px 5px) }`, false},
		{`<div id="d"></div>`, `#d { width: 100px; height: 50px; opacity: .5; background: red; filter: grayscale(1) }`, false},
	} {
		_, findings := filterFindings(t, tc.doc, tc.css)
		got := false
		for _, f := range findings {
			got = got || (f.Rule == RuleUnsupportedValue && strings.Contains(f.Message, "lie over each other"))
		}
		if got != tc.want {
			t.Errorf("%s: reported=%v, want %v: %v", tc.css, got, tc.want, findings)
		}
	}
}

// TestAShadowOfAGroupKeepsItsAlpha: the shadow of a group inside the filtered
// box is a group of the same marks with the same effect on alpha — its colour
// matrices, which change none, left out, and the blurs either side of one
// merged into the one blur they come to.
func TestAShadowOfAGroupKeepsItsAlpha(t *testing.T) {
	inner := FilterGroup{Filters: []FilterFunction{
		{Kind: FilterBlur, StdDev: rpx(3)},
		{Kind: FilterColorMatrix, Matrix: grayscaleMatrix(1)},
		{Kind: FilterBlur, StdDev: rpx(4)},
		{Kind: FilterOpacity, Amount: 0.5},
	}, Ops: []Op{FillRect{Rect: Rect{W: rpx(10), H: rpx(10)}, Color: red}}}
	out := shadowMarks([]Op{inner}, Point{X: rpx(2)}, blue)
	g, ok := out[0].(FilterGroup)
	if len(out) != 1 || !ok {
		t.Fatalf("the shadow of a group is %v", out)
	}
	if len(g.Filters) != 2 || g.Filters[0].Kind != FilterBlur || g.Filters[0].StdDev != rpx(5) ||
		g.Filters[1].Kind != FilterOpacity || g.Filters[1].Amount != 0.5 {
		t.Errorf("the shadow's chain is %+v, want one 5px blur and the opacity", g.Filters)
	}
	if f, ok := g.Ops[0].(FillRect); !ok || f.Color != blue || f.Rect.X != rpx(2) {
		t.Errorf("the shadow's mark is %v, want the fill in blue, moved", g.Ops)
	}
}

// TestAShadowPastThePageIsAnOverhang: a drop shadow is ink the filter's offset
// put where no layout decision placed anything, so a shadow reaching past the
// page's edge is not read by the overflow-page guardrail as a box that left
// it — which would refuse the document — sharp or blurred, a fill, a path or
// a gradient. The box itself past the edge still is.
func TestAShadowPastThePageIsAnOverhang(t *testing.T) {
	avail := Size{W: rpx(1000), H: rpx(400)}
	for _, tc := range []struct {
		css  string
		want bool
	}{
		{`#d { height: 50px; background: red; filter: drop-shadow(0 500px) }`, false},
		{`#d { height: 50px; background: red; filter: drop-shadow(0 500px 4px) }`, false},
		{`#d { height: 50px; background: red; border-radius: 10px; filter: drop-shadow(0 500px) }`, false},
		{`#d { height: 50px; background: linear-gradient(red, blue); filter: drop-shadow(0 500px) }`, false},
		{`#d { height: 50px; background: red; filter: drop-shadow(0 500px); margin-top: 500px }`, true},
	} {
		ops := paintOf(t, `<div id="d"></div>`, noDefaults+tc.css)
		rec := NewRecorder(nil)
		checkPageOverflow(rec, ops, avail, 1)
		if got := hasRule(rec.Findings(), RuleOverflowPage); got != tc.want {
			t.Errorf("%s: the guardrail fired=%v, want %v: %v", tc.css, got, tc.want, ops)
		}
	}
}
