package shape

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

func TestBoundedRunPreservesShapingAndOwnership(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	in := RunInput{Text: "office אבג", Before: "a", After: "b", Kerns: true}
	want, m := f.Clone().ShapeGlyphsMerged(in.Text, in.Before, in.After, "", "", true, in.Features)
	result, err := f.ShapeGlyphsContext(context.Background(), in, RunLimits{})
	got, n := result.Glyphs, result.Missing
	if err != nil || n != m || !reflect.DeepEqual(got, want) {
		t.Fatalf("shaping differs: %v, %d/%d", err, n, m)
	}
	if len(f.Used()) != 0 {
		t.Fatal("receiver glyph usage changed")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, e := f.ShapeGlyphsContext(context.Background(), in, RunLimits{})
			if e != nil || !reflect.DeepEqual(result.Glyphs, want) {
				t.Errorf("concurrent shaping: %v", e)
			}
		}()
	}
	wg.Wait()
}
func TestBoundedRunFailureHasNoPartialGlyphs(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		ctx  context.Context
		in   RunInput
		lim  RunLimits
		want error
	}{
		{ctx, RunInput{Text: "abc"}, RunLimits{}, context.Canceled},
		{context.Background(), RunInput{Text: "abc"}, RunLimits{MaxWork: 1}, ErrRunLimit},
		{context.Background(), RunInput{Text: "abc"}, RunLimits{MaxGlyphs: 1}, ErrRunLimit},
		{context.Background(), RunInput{Text: "a", MergeBefore: strings.Repeat("x", 20)}, RunLimits{MaxInputBytes: 10}, ErrRunLimit},
		{context.Background(), RunInput{Text: "a", Features: Features{Tags: strings.Repeat("liga,", 20)}}, RunLimits{MaxInputBytes: 10}, ErrRunLimit},
	}
	for _, tt := range tests {
		result, e := f.ShapeGlyphsContext(tt.ctx, tt.in, tt.lim)
		if !errors.Is(e, tt.want) || result.Glyphs != nil || result.Missing != 0 {
			t.Fatalf("partial/error: %v %d %v", result.Glyphs, result.Missing, e)
		}
	}
}
func TestBoundedRunStopsRepeatedExpansion(t *testing.T) {
	sub := fonttest.Lookup{Type: 2, Subtables: [][]byte{fonttest.MultipleSubst([]int{1}, [][]int{{1, 1}})}}
	lookups := make([]fonttest.Lookup, 16)
	idx := make([]int, len(lookups))
	for i := range lookups {
		lookups[i] = sub
		idx[i] = i
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{Name: "Expanding", Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}}, Extra: map[string][]byte{"GSUB": fonttest.GSUBLookups(lookups, map[string][]int{"ccmp": idx})}})
	f, e := Load(data)
	if e != nil {
		t.Fatal(e)
	}
	result, e := f.ShapeGlyphsContext(context.Background(), RunInput{Text: "a"}, RunLimits{MaxGlyphs: 32})
	if !errors.Is(e, ErrRunLimit) || result.Glyphs != nil || result.Missing != 0 {
		t.Fatalf("expansion accepted: %d glyphs, %v", len(result.Glyphs), e)
	}
}

func TestBoundedRunReportsRecursiveLookupExhaustion(t *testing.T) {
	rule := fonttest.SequenceContext3([][]int{{gidB}}, []fonttest.SeqLookup{{At: 0, Lookup: 0}})
	f := contextFace(t, []fonttest.Lookup{{Type: 5, Subtables: [][]byte{rule}}}, nil)
	result, e := f.ShapeGlyphsContext(context.Background(), RunInput{Text: "b"}, RunLimits{})
	if !errors.Is(e, ErrRunLimit) || result.Glyphs != nil || result.Missing != 0 {
		t.Fatalf("recursive rule: %v %d %v", result.Glyphs, result.Missing, e)
	}
}

type cancelAfterChecks struct {
	context.Context
	checks int
}

func (c *cancelAfterChecks) Err() error {
	c.checks--
	if c.checks <= 0 {
		return context.Canceled
	}
	return nil
}
func TestBoundedRunChecksCancellationDuringExpansion(t *testing.T) {
	f, e := NotoSans()
	if e != nil {
		t.Fatal(e)
	}
	ctx := &cancelAfterChecks{Context: context.Background(), checks: 20}
	result, e := f.ShapeGlyphsContext(ctx, RunInput{Text: strings.Repeat("office", 40)}, RunLimits{})
	if !errors.Is(e, context.Canceled) || result.Glyphs != nil || result.Missing != 0 {
		t.Fatalf("cancellation during work: %v %d %v", result.Glyphs, result.Missing, e)
	}
}

// A bound one glyph ran into, measured before and for some other run, is the
// face's history and not this run's: a run that draws none of it is accepted.
func TestBoundedRunIgnoresAnotherRunsInkBound(t *testing.T) {
	spin := []byte{139, 139, 21}
	line := []byte{139, 139, 21, 239, 239, 5, 14}
	cff := fonttest.CFF(fonttest.CFFOptions{Glyphs: 3, Charstrings: [][]byte{{14}, spin, line}})
	f, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true}, {Rune: 'Z', Advance: 500, HasShape: true}}}))
	if err != nil {
		t.Fatal(err)
	}
	f.glyphExtents(1)
	if len(f.LayoutLimits()) == 0 {
		t.Fatal("the spinning glyph reports no bound: the test does not reach what it is about")
	}
	result, err := f.ShapeGlyphsContext(context.Background(), RunInput{Text: "Z"}, RunLimits{})
	if err != nil || len(result.Glyphs) != 1 {
		t.Fatalf("a run drawing none of the capped glyph: %v, %v", result.Glyphs, err)
	}
	if _, err := f.WithShapingLimits(context.Background(), RunLimits{}, func(f *Face) error {
		f.ShapeGlyphs("Z")
		return nil
	}); err != nil {
		t.Fatalf("a scope drawing none of the capped glyph: %v", err)
	}
}

// A bound reading the layout a run is shaped with is the run's own, and refuses
// it.
func TestBoundedRunRefusesItsOwnLayoutsBound(t *testing.T) {
	data, err := os.ReadFile("../fonts/notosans/NotoSans-Variable.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	probe := f.Clone()
	probe.runWork = &runWork{ctx: context.Background(), glyphs: 1 << 20, left: 1 << 40}
	probe.ShapeGlyphs("office")
	if len(probe.runWork.layouts) == 0 {
		t.Fatal("shaping records no layout it was shaped with")
	}
	for _, l := range probe.runWork.layouts {
		l.limits = append(l.limits, "a planted limit")
	}
	if _, err := f.ShapeGlyphsContext(context.Background(), RunInput{Text: "office"}, RunLimits{}); !errors.Is(err, ErrRunLimit) {
		t.Fatalf("a run shaped with a layout that ran into a limit: %v", err)
	}
}

func TestBoundedShapingScopeMeasurementsAndOwnership(t *testing.T) {
	f, e := NotoSans()
	if e != nil {
		t.Fatal(e)
	}
	want := f.Clone().MeasureShaped("office", 12)
	work, e := f.WithShapingLimits(context.Background(), RunLimits{}, func(clone *Face) error {
		if got := clone.MeasureShaped("office", 12); got != want {
			t.Fatalf("measurement %v want %v", got, want)
		}
		clone.ShapeGlyphs("אבג")
		return nil
	})
	if e != nil || work <= 0 || len(f.Used()) != 0 {
		t.Fatalf("scope work/ownership: %d %v %v", work, e, f.Used())
	}
	for _, limits := range []RunLimits{{MaxWork: 1}, {MaxInputBytes: 1}, {MaxGlyphs: 1}} {
		work, e = f.WithShapingLimits(context.Background(), limits, func(clone *Face) error { clone.MeasureShaped("office", 12); return nil })
		if work != 0 || !errors.Is(e, ErrRunLimit) {
			t.Fatalf("scope failure: %d %v", work, e)
		}
	}
	custom := errors.New("callback")
	work, e = f.WithShapingLimits(context.Background(), RunLimits{}, func(*Face) error { return custom })
	if work != 0 || !errors.Is(e, custom) {
		t.Fatalf("callback error: %d %v", work, e)
	}
}

// A face kept past its scope, whose context is then done, shapes rather than
// panicking where nothing recovers.
func TestAFaceKeptPastItsScopeIsUnbounded(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var kept *Face
	if _, err := f.WithShapingLimits(ctx, RunLimits{}, func(f *Face) error { kept = f; return nil }); err != nil {
		t.Fatal(err)
	}
	cancel()
	if glyphs, _ := kept.ShapeGlyphs("office"); len(glyphs) == 0 {
		t.Fatal("the kept face shaped nothing")
	}
}

// A face the caller owns shapes a bounded run as it shapes an unbounded one,
// records its glyphs as ShapeGlyphs does, and leaves no budget on the face.
func TestABoundedRunOnAnOwnedFace(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	in := RunInput{Text: "office אבג", Before: "a", After: "b", Kerns: true}
	plain := f.Clone()
	want, m := plain.ShapeGlyphsMerged(in.Text, in.Before, in.After, "", "", true, in.Features)
	owned := f.Clone()
	result, err := owned.ShapeGlyphsBounded(context.Background(), in, RunLimits{})
	if err != nil || result.Missing != m || !reflect.DeepEqual(result.Glyphs, want) || result.Work <= 0 {
		t.Fatalf("shaping differs: %v, %d/%d, work %d", err, result.Missing, m, result.Work)
	}
	if !reflect.DeepEqual(owned.Used(), plain.Used()) {
		t.Fatalf("recorded %v, ShapeGlyphs records %v", owned.Used(), plain.Used())
	}
	if owned.runWork != nil || owned.spareWork.ctx != nil {
		t.Fatal("the budget, or the caller's context, is left on the face")
	}

	// A run that fails leaves the next one a whole budget.
	if _, err := owned.ShapeGlyphsBounded(context.Background(), in, RunLimits{MaxWork: 1}); !errors.Is(err, ErrRunLimit) {
		t.Fatalf("a run over its work limit: %v", err)
	}
	if owned.runWork != nil {
		t.Fatal("a failed run leaves its budget on the face")
	}
	again, err := owned.ShapeGlyphsBounded(context.Background(), in, RunLimits{})
	if err != nil || !reflect.DeepEqual(again.Glyphs, want) || again.Work != result.Work {
		t.Fatalf("the run after a failure: %v, work %d, want %d", err, again.Work, result.Work)
	}
}

// Bounding a run on an owned face costs no allocation ShapeGlyphs does not
// make: no clone, no budget and no list of layouts.
func TestABoundedRunOnAnOwnedFaceAllocatesAsAnUnboundedOne(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	in := RunInput{Text: "Quarterly revenue, office 001234", Kerns: true}
	plain, owned := f.Clone(), f.Clone()
	ctx := context.Background()
	unbounded := testing.AllocsPerRun(50, func() {
		plain.ShapeGlyphsMerged(in.Text, "", "", "", "", true, in.Features)
	})
	bounded := testing.AllocsPerRun(50, func() {
		if _, err := owned.ShapeGlyphsBounded(ctx, in, RunLimits{}); err != nil {
			t.Fatal(err)
		}
	})
	if bounded > unbounded {
		t.Fatalf("a bounded run allocates %v times, an unbounded one %v", bounded, unbounded)
	}
}

// The face a scope hands out is already under the scope's budget, which a run
// of its own would replace.
func TestABoundedRunIsRefusedInsideAScope(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WithShapingLimits(context.Background(), RunLimits{}, func(f *Face) error {
		_, err := f.ShapeGlyphsBounded(context.Background(), RunInput{Text: "a"}, RunLimits{})
		return err
	})
	if err == nil {
		t.Fatal("a bounded run inside a scope was accepted")
	}
}

// The context is asked at every boundary between phases, however few charges
// came before: a scope cancelled after its last run is refused.
func TestAScopeCancelledAfterItsLastRunIsRefused(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = f.WithShapingLimits(ctx, RunLimits{}, func(f *Face) error {
		f.ShapeGlyphs("a")
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a scope cancelled after its last run: %v", err)
	}
}

// The default limits admit a run of real text as long as MaxInputBytes allows,
// in the scripts whose fonts do the most per character, with room to spare
// (issue 916). A subtable used to be charged its whole size at every position
// it was tried at, whether or not it covered the glyph, which put a byte of
// Latin at about 460,000 units and of Devanagari at eight million: the 64M
// default admitted 140 bytes of the one and 8 of the other. What is charged
// now is what is read — see RunLimits — and the costliest of these, Devanagari,
// is about 300 units a byte.
func TestTheDefaultLimitsAdmitOrdinaryText(t *testing.T) {
	noto, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	load := func(path string) *Face {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Load(data)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	arabic := load("../testdata/harfbuzz/fonts/NotoSansArabic.ttf")
	khmer := load("../testdata/harfbuzz/fonts/NotoSansKhmer.ttf")
	const maxInput = 4096 // RunLimits' default MaxInputBytes
	for _, c := range []struct {
		name string
		face *Face
		text string
	}{
		{"Latin", noto, "office affluent waffle "},
		{"Devanagari", noto, "क्षत्रिय नमस्ते हिन्दी "},
		{"Arabic", arabic, "مرحبا بالعالم "},
		{"Khmer", khmer, "ភាសាខ្មែរ "},
	} {
		text := strings.Repeat(c.text, maxInput/len(c.text))
		if _, missing := c.face.ShapeGlyphs(text); missing != 0 {
			t.Fatalf("%s: the face cannot set the text, so this tests nothing", c.name)
		}
		result, err := c.face.ShapeGlyphsContext(context.Background(), RunInput{Text: text, Kerns: true}, RunLimits{})
		if err != nil {
			t.Errorf("%d bytes of %s refused at the default limits: %v", len(text), c.name, err)
			continue
		}
		// Room to spare: a sixteenth of the default.
		if result.Work > (64<<20)/16 {
			t.Errorf("%d bytes of %s cost %d units, more than a sixteenth of the default", len(text), c.name, result.Work)
		}
	}
}

// What a rule set makes the shaper try is charged, however little of it
// matches: a font that lists a thousand rules for a glyph and matches none
// pays for the thousand at every position it is tried at. It is the work the
// budget is for, and what charging only what is read must not stop counting.
func TestTheRulesALookupTriesAreCharged(t *testing.T) {
	const rules, run = 1000, 64
	set := make([]fonttest.ContextRule, rules)
	for i := range set {
		// b followed by d, which the run never has.
		set[i] = fonttest.ContextRule{Input: []int{gidB, gidD}, Lookups: []fonttest.SeqLookup{{At: 0, Lookup: 0}}}
	}
	f := contextFace(t, []fonttest.Lookup{substB(), {Type: 5, Subtables: [][]byte{
		fonttest.SequenceContext1(map[int][]fonttest.ContextRule{gidB: set}),
	}}}, nil)
	text := strings.Repeat("b", run)
	result, err := f.ShapeGlyphsContext(context.Background(), RunInput{Text: text}, RunLimits{MaxWork: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	if result.Work < rules*run {
		t.Errorf("%d positions each trying %d rules were charged %d units, fewer than the rules tried",
			run, rules, result.Work)
	}
	if _, err := f.ShapeGlyphsContext(context.Background(), RunInput{Text: text}, RunLimits{MaxWork: rules * run / 2}); !errors.Is(err, ErrRunLimit) {
		t.Errorf("a budget of half the rules tried was not exhausted: %v", err)
	}
}

// A ShapingBudget charges everything shaped on the faces put under it to one
// budget, takes them off it when Run returns however Run ended, and is spent
// by one Run.
func TestAShapingBudgetIsSharedByItsFacesAndSpentByOneRun(t *testing.T) {
	noto, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	a, b := noto.Clone(), noto.Clone()
	single, err := a.Clone().ShapeGlyphsContext(context.Background(), RunInput{Text: "office"}, RunLimits{})
	if err != nil {
		t.Fatal(err)
	}

	budget, err := NewShapingBudget(context.Background(), RunLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if err := budget.Bound(a); err != nil {
		t.Fatal(err)
	}
	if err := budget.Bound(a); err != nil {
		t.Errorf("a face put under the budget twice: %v", err)
	}
	work, err := budget.Run(func() error {
		a.ShapeGlyphs("office")
		// A face put under it while it runs.
		if err := budget.Bound(b); err != nil {
			return err
		}
		b.ShapeGlyphs("office")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if work != 2*single.Work {
		t.Errorf("two runs on two faces were charged %d, want twice the %d one costs", work, single.Work)
	}
	for _, f := range []*Face{a, b} {
		if _, err := f.ShapeGlyphsBounded(context.Background(), RunInput{Text: "x"}, RunLimits{}); err != nil {
			t.Errorf("a face is still under the budget once Run returned: %v", err)
		}
	}
	if _, err := budget.Run(func() error { return nil }); err == nil {
		t.Error("a spent budget ran again")
	}
	if err := budget.Bound(noto.Clone()); err == nil {
		t.Error("a face was put under a spent budget")
	}
}

// A budget that runs out stops the shaping where it is, and Run reports it;
// so does a context done, and a face under another budget is not taken.
func TestAShapingBudgetStopsWhatItCannotAfford(t *testing.T) {
	noto, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	f := noto.Clone()
	budget, _ := NewShapingBudget(context.Background(), RunLimits{MaxWork: 1})
	_ = budget.Bound(f)
	reached := false
	if _, err := budget.Run(func() error {
		f.ShapeGlyphs("office")
		reached = true
		return nil
	}); !errors.Is(err, ErrRunLimit) {
		t.Errorf("a budget of one unit: %v", err)
	}
	if reached {
		t.Error("shaping went on past the budget")
	}
	if _, err := f.ShapeGlyphsBounded(context.Background(), RunInput{Text: "x"}, RunLimits{}); err != nil {
		t.Errorf("a face is still under a budget that stopped: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	budget, _ = NewShapingBudget(ctx, RunLimits{})
	_ = budget.Bound(f)
	cancel()
	if _, err := budget.Run(func() error { f.ShapeGlyphs("office"); return nil }); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context: %v", err)
	}

	first, _ := NewShapingBudget(context.Background(), RunLimits{})
	second, _ := NewShapingBudget(context.Background(), RunLimits{})
	g := noto.Clone()
	_ = first.Bound(g)
	if err := second.Bound(g); err == nil {
		t.Error("a face under one budget was put under another")
	}
	if _, err := NewShapingBudget(context.Background(), RunLimits{MaxWork: -1}); err == nil {
		t.Error("a negative limit was accepted")
	}
}

// A budget's defaults grow with what it shapes (issue 923): no bound on how
// long a run is, glyphs of 64 a byte of the longest run, and 1,024 units of
// work a byte on top of the 64 million — which a font doing several times
// what real text does still runs out of.
func TestAShapingBudgetsDefaultsGrowWithItsText(t *testing.T) {
	noto, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	f := noto.Clone()
	budget, err := NewShapingBudget(context.Background(), RunLimits{})
	if err != nil {
		t.Fatal(err)
	}
	_ = budget.Bound(f)
	text := strings.Repeat("revenue ", 2000) // 16,000 bytes, past a run's 4,096
	if _, err := budget.Run(func() error { f.ShapeGlyphs(text); return nil }); err != nil {
		t.Fatalf("a run of %d bytes under a budget's defaults: %v", len(text), err)
	}
	if w := budget.work; w.glyphs != 64*len(text) || w.granted < 64<<20+1024*int64(len(text)) {
		t.Errorf("after a run of %d bytes the budget allows %d glyphs and %d units, want %d and at least %d",
			len(text), w.glyphs, w.granted, 64*len(text), 64<<20+1024*len(text))
	}

	// The allowance a byte brings, with the 64 million taken away so that
	// what is tested is what a byte is given: real text fits in it, and a
	// font trying 5,000 rules at every glyph, matching none, does not.
	allowanceOnly := func() *ShapingBudget {
		b, _ := NewShapingBudget(context.Background(), RunLimits{})
		b.work.left, b.work.granted = 0, 0
		return b
	}
	budget = allowanceOnly()
	plain := noto.Clone()
	_ = budget.Bound(plain)
	if _, err := budget.Run(func() error { plain.ShapeGlyphs(text); return nil }); err != nil {
		t.Errorf("real text does not fit in what its bytes are given: %v", err)
	}
	const rules = 5000
	set := make([]fonttest.ContextRule, rules)
	for i := range set {
		set[i] = fonttest.ContextRule{Input: []int{gidB, gidD}, Lookups: []fonttest.SeqLookup{{At: 0, Lookup: 0}}}
	}
	hostile := contextFace(t, []fonttest.Lookup{substB(), {Type: 5, Subtables: [][]byte{
		fonttest.SequenceContext1(map[int][]fonttest.ContextRule{gidB: set}),
	}}}, nil)
	budget = allowanceOnly()
	_ = budget.Bound(hostile)
	if _, err := budget.Run(func() error { hostile.ShapeGlyphs(strings.Repeat("b", 300)); return nil }); !errors.Is(err, ErrRunLimit) {
		t.Errorf("a font doing %d units a byte fitted in what its bytes are given: %v", rules, err)
	}
}
