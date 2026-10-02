package shape

import (
	"context"
	"errors"
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
