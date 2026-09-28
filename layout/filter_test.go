package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// Filter Effects 1's filter property: blur() and opacity(), as a group.

func filterGroupsOf(ops []Op) []FilterGroup {
	var out []FilterGroup
	for _, op := range ops {
		switch v := op.(type) {
		case FilterGroup:
			out = append(out, v)
			out = append(out, filterGroupsOf(v.Ops)...)
		case ClipPath:
			out = append(out, filterGroupsOf(v.Ops)...)
		}
	}
	return out
}

func filterFindings(t *testing.T, htmlSrc, cssSrc string) ([]Op, []Finding) {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults + cssSrc}}})
	rec := NewRecorder(nil)
	ops := PaintReporting(Layout(built.Root, Size{W: rpx(600), H: rpx(1000)}, nil, rec), rec)
	return ops, rec.Findings()
}

// TestAFilterChainIsOneBlurAndOneOpacity: blurs compose by adding variances and
// opacities by multiplying, and the two commute, so every chain of them is one
// of each at most, blur first.
func TestAFilterChainIsOneBlurAndOneOpacity(t *testing.T) {
	for _, tc := range []struct {
		filter string
		want   []FilterFunction
	}{
		{"blur(3px) blur(4px)", []FilterFunction{{Kind: FilterBlur, StdDev: rpx(5)}}},
		{"opacity(50%) opacity(0.5)", []FilterFunction{{Kind: FilterOpacity, Amount: 0.25}}},
		{"opacity(0.5) blur(2px)", []FilterFunction{{Kind: FilterBlur, StdDev: rpx(2)}, {Kind: FilterOpacity, Amount: 0.5}}},
		// "Values of amount over 100% are allowed but UAs must clamp the
		// values to 1."
		{"opacity(250%) blur(1px)", []FilterFunction{{Kind: FilterBlur, StdDev: rpx(1)}}},
		// Clamped before it is multiplied: 250% is 1, and 1 × 0.4 is 0.4.
		{"opacity(250%) opacity(0.4)", []FilterFunction{{Kind: FilterOpacity, Amount: 0.4}}},
		{"blur(0.5em)", []FilterFunction{{Kind: FilterBlur, StdDev: rpx(8)}}},
	} {
		ops := paintOf(t, `<div id="d">x</div>`, noDefaults+`#d { height: 20px; background: red; filter: `+tc.filter+` }`)
		gs := filterGroupsOf(ops)
		if len(gs) != 1 {
			t.Errorf("%s: %d groups", tc.filter, len(gs))
			continue
		}
		if len(gs[0].Filters) != len(tc.want) {
			t.Errorf("%s: %+v, want %+v", tc.filter, gs[0].Filters, tc.want)
			continue
		}
		for i := range tc.want {
			g, w := gs[0].Filters[i], tc.want[i]
			if g.Kind != w.Kind || g.StdDev != w.StdDev || math.Abs(g.Amount-w.Amount) > 1e-12 {
				t.Errorf("%s: %+v, want %+v", tc.filter, gs[0].Filters, tc.want)
			}
		}
	}
}

// TestAFilterThatFiltersNothingIsNoGroup, and one that leaves nothing paints
// nothing.
func TestAFilterThatFiltersNothingIsNoGroup(t *testing.T) {
	for _, f := range []string{"blur(0)", "opacity(1)", "blur()"} {
		ops := paintOf(t, `<div id="d">x</div>`, noDefaults+`#d { height: 20px; background: red; filter: `+f+` }`)
		if len(filterGroupsOf(ops)) != 0 {
			t.Errorf("%s made a group", f)
		}
		if len(fillsOf(ops, red)) != 1 {
			t.Errorf("%s: the background is not painted as it was", f)
		}
	}
	ops := paintOf(t, `<div id="d">x</div>`, noDefaults+`#d { height: 20px; background: red; filter: opacity(0) }`)
	if len(filterGroupsOf(ops)) != 0 || len(fillsOf(ops, red)) != 0 {
		t.Error("a box of opacity(0) painted something")
	}
}

// TestAFilterFunctionNotAppliedIsReported: a url() is reported by name, and the
// functions beside it, every one of which is applied (filtercolour.go), are
// not.
func TestAFilterFunctionNotAppliedIsReported(t *testing.T) {
	ops, findings := filterFindings(t, `<div id="d">x</div>`,
		`#d { height: 20px; background: red; filter: blur(2px) grayscale(1) url(#f) }`)
	if gs := filterGroupsOf(ops); len(gs) != 1 || gs[0].Filters[0].StdDev != rpx(2) {
		t.Errorf("the blur beside what was not applied was not applied: %+v", gs)
	}
	said := ""
	for _, f := range findings {
		if f.Rule == RuleUnsupportedValue && f.Property == "filter" {
			said = f.Message
		}
	}
	if strings.Contains(said, "grayscale()") || !strings.Contains(said, "url()") {
		t.Errorf("the report is %q, want it to name url() and not grayscale()", said)
	}
	_, findings = filterFindings(t, `<div id="d">x</div>`, `#d { filter: blur(2px) opacity(0.5) }`)
	if hasRule(findings, RuleUnsupportedValue) || hasRule(findings, RuleUnsupportedProperty) {
		t.Errorf("a filter this engine applies was reported: %v", findings)
	}
}

// TestAFilteredBoxIsAStackingContext: "the same way that CSS opacity does" — it
// is painted at z-index 0, above the in-flow blocks after it.
func TestAFilteredBoxIsAStackingContext(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div><div id="b"></div>`, noDefaults+`
		#a { height: 20px; background: red; filter: blur(1px) }
		#b { height: 20px; margin-top: -10px; background: blue }`)
	iBlue, iGroup := -1, -1
	for i, op := range ops {
		switch v := op.(type) {
		case FillRect:
			if v.Color == blue {
				iBlue = i
			}
		case FilterGroup:
			iGroup = i
		}
	}
	if iBlue < 0 || iGroup < 0 || iGroup < iBlue {
		t.Errorf("the filtered box is at %d and the later block at %d; it must be painted after", iGroup, iBlue)
	}
}

// TestAFilterIsClippedAfterItIsApplied: §5, "first any filter effect is
// applied, then any clipping". What clips the box clips the group, and nothing
// inside it is cut.
func TestAFilterIsClippedAfterItIsApplied(t *testing.T) {
	ops := paintOf(t, `<div id="o"><div id="d"></div></div>`, noDefaults+`
		#o { width: 50px; height: 50px; overflow: hidden }
		#d { width: 100px; height: 100px; background: red; filter: blur(2px) }`)
	gs := filterGroupsOf(ops)
	if len(gs) != 1 {
		t.Fatalf("%d groups", len(gs))
	}
	if !gs[0].Clip.Active || gs[0].Clip.Rect != (Rect{W: rpx(50), H: rpx(50)}) {
		t.Errorf("the group is clipped to %v, want the 50px box", gs[0].Clip)
	}
	if f := fillsOf(gs[0].Ops, red); len(f) != 1 || f[0] != (Rect{W: rpx(100), H: rpx(100)}) {
		t.Errorf("inside the group the background is %v, want it whole", f)
	}
	// And a curve around it goes round the group.
	ops = paintOf(t, `<div id="o"><div id="d"></div></div>`, noDefaults+`
		#o { width: 100px; height: 100px; overflow: hidden; border-radius: 40px }
		#d { width: 100px; height: 100px; background: red; filter: blur(2px) }`)
	cps := groupsOf(ops)
	if len(cps) != 1 || len(cps[0].Ops) != 1 {
		t.Fatalf("%d clip groups: %v", len(cps), ops)
	}
	if _, ok := cps[0].Ops[0].(FilterGroup); !ok {
		t.Errorf("the curve holds %T, want the filter group", cps[0].Ops[0])
	}
}

// TestAFilteredInlineBoxIsAGroup: a <span> with a filter, and the block lifted
// out of it (§9.2.1.1), which is not in the tree under it and is filtered with
// it all the same.
func TestAFilteredInlineBoxIsAGroup(t *testing.T) {
	ops := paintOf(t, `<p>a <span id="s">b</span> c</p>`, noDefaults+`#s { filter: blur(1px) }`)
	gs := filterGroupsOf(ops)
	if len(gs) != 1 || len(gs[0].Ops) != 1 {
		t.Fatalf("%d groups: %v", len(gs), ops)
	}
	if r, ok := gs[0].Ops[0].(DrawText); !ok || r.Text != "b" {
		t.Errorf("the group holds %v, want the span's run", gs[0].Ops)
	}
	ops = paintOf(t, `<span id="s"><div>float</div></span>`, noDefaults+`#s { filter: blur(1px) }`)
	gs = filterGroupsOf(ops)
	if len(gs) != 1 || len(gs[0].Ops) != 1 {
		t.Fatalf("the block lifted out of a filtered span: %d groups: %v", len(gs), ops)
	}
	// Cut before it is blurred, inside a box that clips, which is reported.
	_, findings := filterFindings(t, `<p id="o">a <span id="s">b</span> c</p>`,
		`#o { overflow: hidden; height: 50px } #s { filter: blur(1px) }`)
	found := false
	for _, f := range findings {
		found = found || strings.Contains(f.Message, "cut before it was filtered")
	}
	if !found {
		t.Errorf("a blurred inline box inside a clip was not reported: %v", findings)
	}
}

// TestAFilterIsAContainingBlockItReportsOnlyWhereItIsNot: §5's containing
// block is made (see filtercontainingblock_test.go), and what is reported is
// the one case that is not: a positioned box in a block lifted out of a
// filtered inline box, which is not above the block in the box tree. Nothing
// is reported where the filter is the containing block, where a positioned
// box between is, or at the root, which §5 exempts.
func TestAFilterIsAContainingBlockItReportsOnlyWhereItIsNot(t *testing.T) {
	for _, tc := range []struct {
		doc, css string
		want     bool
	}{
		{`<div id="f"><div id="a"></div></div>`, `#f { filter: blur(1px) } #a { position: absolute }`, false},
		{`<div id="f"><div id="a"></div></div>`, `#f { filter: blur(1px) } #a { position: fixed }`, false},
		{`<div id="f"><div id="p"><div id="a"></div></div></div>`,
			`#f { filter: blur(1px) } #p { position: relative } #a { position: absolute }`, false},
		{`<div id="f"><div id="a"></div></div>`, `#f { filter: blur(1px); position: relative } #a { position: absolute }`, false},
		{`<div id="a"></div>`, `html { filter: blur(1px) } #a { position: absolute }`, false},
		{`<span id="f"><div><div id="a"></div></div></span>`, `#f { filter: blur(1px) } #a { position: absolute }`, true},
		{`<span id="f"><div><div id="a"></div></div></span>`, `#f { filter: blur(1px) } #a { position: fixed }`, true},
		{`<span id="f"><div id="p"><div id="a"></div></div></span>`,
			`#f { filter: blur(1px) } #p { position: relative } #a { position: absolute }`, false},
	} {
		_, findings := filterFindings(t, tc.doc, tc.css)
		got := false
		for _, f := range findings {
			got = got || (f.Rule == RulePositionApproximated && f.Property == "filter")
		}
		if got != tc.want {
			t.Errorf("%s: reported=%v, want %v", tc.css, got, tc.want)
		}
	}
}

// TestALinkInAFilterIsOutsideTheGroup: a link is an area, a filter does nothing
// to one, and the group's clip cuts it as nothing inside the group was.
func TestALinkInAFilterIsOutsideTheGroup(t *testing.T) {
	ops := paintOf(t, `<div id="o"><a id="l" href="#x">x</a></div>`, noDefaults+`
		#o { width: 50px; height: 50px; overflow: hidden }
		#l { display: block; width: 100px; height: 20px; filter: blur(1px); background: red }`)
	var link *Link
	for _, op := range ops {
		if l, ok := op.(Link); ok {
			link = &l
		}
	}
	if link == nil {
		t.Fatalf("no link at the top of the list: %v", ops)
	}
	if len(link.Rects) != 1 || link.Rects[0].W != rpx(50) {
		t.Errorf("the link's area is %v, want it cut to the 50px clip", link.Rects)
	}
}

// TestAFilterGroupTakesAnOpacityAroundIt: dimOps folds an alpha into the
// chain's opacity, exactly, since the group is composited as one.
func TestAFilterGroupTakesAnOpacityAroundIt(t *testing.T) {
	g := FilterGroup{Filters: []FilterFunction{{Kind: FilterBlur, StdDev: rpx(1)}},
		Ops: []Op{FillRect{Rect: Rect{W: rpx(10), H: rpx(10)}, Color: red}}}
	ops, marks := dimOps([]Op{g}, 0, 0.5)
	got := ops[0].(FilterGroup)
	if len(got.Filters) != 2 || got.Filters[1].Kind != FilterOpacity || got.Filters[1].Amount != 0.5 {
		t.Errorf("the dimmed chain is %+v", got.Filters)
	}
	if len(marks) != 1 || marks[0].rect != g.Extent() {
		t.Errorf("the group's mark is %+v, want its extent %v", marks, g.Extent())
	}
	again, _ := dimOps(ops, 0, 0.5)
	if f := again[0].(FilterGroup).Filters; len(f) != 2 || f[1].Amount != 0.25 {
		t.Errorf("a second dimming is %+v, want one opacity of 0.25", f)
	}
	if ops, _ := dimOps([]Op{g}, 0, 0); len(ops) != 0 {
		t.Error("a group dimmed to nothing was kept")
	}
	if g.Filters[0].Kind != FilterBlur || len(g.Filters) != 1 {
		t.Error("dimming wrote into the chain it was given")
	}
}

// TestAFilterGroupsExtentIsItsBlur: three deviations past what it holds, and
// cut by its clip.
func TestAFilterGroupsExtentIsItsBlur(t *testing.T) {
	g := FilterGroup{Filters: []FilterFunction{{Kind: FilterBlur, StdDev: rpx(2)}},
		Ops: []Op{FillRect{Rect: Rect{X: rpx(10), Y: rpx(10), W: rpx(10), H: rpx(10)}, Color: red}}}
	if e := g.Extent(); e != (Rect{X: rpx(4), Y: rpx(4), W: rpx(22), H: rpx(22)}) {
		t.Errorf("the extent is %v", e)
	}
	g.Clip = Clip{Rect: Rect{W: rpx(15), H: rpx(100)}, Active: true}
	if e := g.Extent(); e != (Rect{X: rpx(4), Y: rpx(4), W: rpx(11), H: rpx(22)}) {
		t.Errorf("the clipped extent is %v", e)
	}
}

// TestTheFilterBoundFires lowers the bound on the functions a filter lists.
func TestTheFilterBoundFires(t *testing.T) {
	was := maxFilterFunctions
	defer func() { maxFilterFunctions = was }()
	maxFilterFunctions = 2
	ops, findings := filterFindings(t, `<div id="d">x</div>`,
		`#d { height: 20px; background: red; filter: blur(1px) blur(1px) blur(1px) }`)
	if len(filterGroupsOf(ops)) != 0 {
		t.Error("a filter past the bound was applied")
	}
	if !reportedLimit(findings, "filter of more than 2") {
		t.Errorf("the bound fired silently: %v", findings)
	}
}

// TestPictureRendersABlur: the comparison's blur of a flat group is the
// Gaussian's, by its closed form — checked here against a numerical
// convolution of the same square — and is itself, and differs from the square
// unblurred and blurred by another deviation.
func TestPictureRendersABlur(t *testing.T) {
	sq := FillRect{Rect: picRect(50, 50, 20, 20), Color: picRed}
	blur := func(s float64) FilterGroup {
		return FilterGroup{Filters: []FilterFunction{{Kind: FilterBlur, StdDev: picPx(s)}}, Ops: []Op{sq}}
	}
	m, ok := filteredFills(blur(4))
	if !ok || m.shade == nil {
		t.Fatalf("a blurred square rendered as %+v", m)
	}
	// Numerically: the square's alpha convolved with the Gaussian, at a few
	// points inside, on the edge and outside.
	for _, p := range [][2]float64{{60, 60}, {50, 60}, {45, 55}, {72, 72}, {40, 40}} {
		var sum float64
		const step = 0.05
		for y := 50.0 + step/2; y < 70; y += step {
			for x := 50.0 + step/2; x < 70; x += step {
				dx, dy := p[0]-x, p[1]-y
				sum += math.Exp(-(dx*dx+dy*dy)/(2*16)) / (2 * math.Pi * 16) * step * step
			}
		}
		got := m.shade.at(p[0], p[1])
		if math.Abs(got.A-sum) > 1e-3 || (got.A > 1e-9 && math.Abs(got.R-255) > 1e-6) {
			t.Errorf("at %v the blur is %v, want alpha %.5f of red", p, got, sum)
		}
	}
	on := func(ops ...Op) []Op { return append([]Op{picFill(0, 0, 200, 200, picGreen)}, ops...) }
	if !pictureEqual(on(blur(4)), on(blur(4)), picPage) {
		t.Error("a blurred square is not itself")
	}
	if pictureEqual(on(blur(4)), on(sq), picPage) {
		t.Error("a blurred square is the square")
	}
	if pictureEqual(on(blur(4)), on(blur(5)), picPage) {
		t.Error("two blurs of different deviations are one")
	}
	// An opacity alone is the square at that alpha.
	faded := FilterGroup{Filters: []FilterFunction{{Kind: FilterOpacity, Amount: 0.5}}, Ops: []Op{sq}}
	half := FillRect{Rect: sq.Rect, Color: style.RGBA{R: 255, A: 0.5}}
	if !pictureEqual(on(faded), on(half), picPage) {
		t.Error("a square at opacity(0.5) is not the square at half alpha")
	}
	// Text in a filtered group is keyed by the filter.
	run := picText("Test", 8, 29)
	if pictureEqual([]Op{FilterGroup{Filters: blur(1).Filters, Ops: []Op{run}}}, []Op{run}, picPage) {
		t.Error("a blurred run is the run")
	}
	if pictureEqual([]Op{FilterGroup{Filters: blur(1).Filters, Ops: []Op{run}}},
		[]Op{FilterGroup{Filters: blur(2).Filters, Ops: []Op{run}}}, picPage) {
		t.Error("a run blurred by two deviations is one mark")
	}
}

// TestARectangleClipGoesOnAFilterGroup: clipOps puts a clip on the group rather
// than into it, since the clip is applied after the filter; drops a group it
// hides; and carries none that it does not cut.
func TestARectangleClipGoesOnAFilterGroup(t *testing.T) {
	g := FilterGroup{Filters: []FilterFunction{{Kind: FilterBlur, StdDev: rpx(2)}},
		Ops: []Op{FillRect{Rect: Rect{X: rpx(10), Y: rpx(10), W: rpx(20), H: rpx(20)}, Color: red}}}
	cut := Clip{Rect: Rect{W: rpx(20), H: rpx(100)}, Active: true}
	got := clipOps([]Op{g}, 0, cut)
	if len(got) != 1 {
		t.Fatalf("%d ops", len(got))
	}
	fg := got[0].(FilterGroup)
	if fg.Clip != cut {
		t.Errorf("the group's clip is %v, want %v", fg.Clip, cut)
	}
	if r := fg.Ops[0].(FillRect).Rect; r.W != rpx(20) {
		t.Errorf("the fill inside was cut to %v", r)
	}
	if got := clipOps([]Op{g}, 0, Clip{Rect: Rect{X: rpx(500), W: rpx(10), H: rpx(10)}, Active: true}); len(got) != 0 {
		t.Error("a group wholly outside the clip was kept")
	}
	if got := clipOps([]Op{g}, 0, Clip{Rect: Rect{W: rpx(500), H: rpx(500)}, Active: true}); got[0].(FilterGroup).Clip.Active {
		t.Error("a clip that cuts nothing was carried")
	}
}

// TestNestedFiltersCostWhatTheyHold: a group inside a group is asked where it
// reaches by every group around it, and working that out afresh each time is
// the square of the nesting. Painting boxes nested n and 4n deep, each filtered
// and each holding a fill, is linear in the boxes.
func TestNestedFiltersCostWhatTheyHold(t *testing.T) {
	laid := func(n int) *Fragment {
		src := strings.Repeat(`<div class="f"><p>x</p>`, n) + strings.Repeat(`</div>`, n)
		built := Build(Input{HTML: src, CSS: []Stylesheet{{Source: noDefaults +
			`.f { filter: blur(1px); padding-left: 1px } p { height: 2px; background: red }`}}})
		return Layout(built.Root, Size{W: rpx(2000), H: rpx(10000)}, nil, NewRecorder(nil))
	}
	small, large := laid(50), laid(200)
	if got := len(filterGroupsOf(Paint(large))); got != 200 {
		t.Fatalf("%d groups at the larger size, want 200", got)
	}
	c := costtest.Time(t, "painting n nested filters", func() { Paint(small) }, func() { Paint(large) })
	if c.Ratio > 8 {
		t.Errorf("four times the nesting cost %.1f times as much; want about four", c.Ratio)
	}
}
