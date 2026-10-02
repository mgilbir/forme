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
