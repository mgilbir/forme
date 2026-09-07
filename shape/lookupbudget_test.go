package shape

import (
	"testing"
	"time"

	"github.com/mgilbir/forme/fonttest"
)

// A matched rule names a list of lookup records — "apply lookup N at position
// P" — and nothing in the format stops those records from naming the rule's own
// lookup. Depth bounds how tall that tree gets; the record count decides how
// wide, and nothing bounded that. Forty records eight levels deep is forty to
// the eighth applications from a few hundred bytes of GSUB, on one letter.
//
// A font is untrusted input on this engine's own terms — an @font-face URL
// reaches this parser — so the tests below are all the same shape: build the
// smallest font that asks for the fan-out, shape one letter, and require an
// answer within a few seconds. The existing recursion tests use one record per
// rule, which is linear and was never the problem.

// fanOutRule is a rule on gidB whose records all name lookup number self.
func fanOutRule(records, self int) []byte {
	recs := make([]fonttest.SeqLookup, records)
	for i := range recs {
		recs[i] = fonttest.SeqLookup{At: 0, Lookup: self}
	}
	return fonttest.SequenceContext1(map[int][]fonttest.ContextRule{
		gidB: {{Input: []int{gidB}, Lookups: recs}},
	})
}

// shapeWithin shapes s and fails if it has not finished within d.
func shapeWithin(t *testing.T, f *Face, s string, d time.Duration) []int {
	t.Helper()
	done := make(chan []int, 1)
	go func() {
		glyphs, _ := f.ShapeGlyphs(s)
		out := make([]int, len(glyphs))
		for i, g := range glyphs {
			out[i] = g.GID
		}
		done <- out
	}()
	select {
	case got := <-done:
		return got
	case <-time.After(d):
		t.Fatalf("shaping %q did not finish within %v", s, d)
		return nil
	}
}

// TestAFanningContextualRuleTerminates is the substitution half.
func TestAFanningContextualRuleTerminates(t *testing.T) {
	f := contextFace(t, []fonttest.Lookup{{
		Type: 5, Subtables: [][]byte{fanOutRule(40, 0)},
	}}, nil)
	// Nothing substitutes anything, so the answer is the letters themselves —
	// the point is that there is an answer at all.
	wantGIDs(t, shapeWithin(t, f, "bbb", 10*time.Second), []int{gidB, gidB, gidB}, "bbb")
}

// TestAFanningRuleThroughASecondLookupTerminates is the mutual-recursion form:
// the tree is built by two lookups naming each other, so a bound that looked
// only at whether a lookup names itself would not see it.
func TestAFanningRuleThroughASecondLookupTerminates(t *testing.T) {
	f := contextFace(t, []fonttest.Lookup{
		{Type: 5, Subtables: [][]byte{fanOutRule(20, 1)}},
		{Type: 5, Subtables: [][]byte{fanOutRule(20, 0)}},
	}, nil)
	wantGIDs(t, shapeWithin(t, f, "bbb", 10*time.Second), []int{gidB, gidB, gidB}, "bbb")
}

// TestAFanningPositioningRuleTerminates is the GPOS twin. The matching code is
// shared with substitution and only what a matched rule then does differs,
// which is exactly why one bound has to cover both.
func TestAFanningPositioningRuleTerminates(t *testing.T) {
	// contextPosFace puts its own nudge at index 0, so the rule is lookup 1 and
	// names itself.
	f := contextPosFace(t, []fonttest.Lookup{{
		Type: 7, Subtables: [][]byte{fanOutRule(40, 1)},
	}}, []int{1})
	done := make(chan bool, 1)
	go func() {
		f.ShapeGlyphs("bbb")
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("positioning a fanning rule did not finish")
	}
}

// TestTheLookupAllowanceIsSpentOnceEach says what the bound counts, since a
// bound that is never reached and a bound that is reached at once look the same
// from outside.
func TestTheLookupAllowanceIsSpentOnceEach(t *testing.T) {
	budget := lookupBudget(3)
	sh := shaper{ops: budget}
	start := *budget
	for i := 0; i < 100; i++ {
		if !sh.recurse() {
			t.Fatalf("the allowance ran out after %d of 100 recursions", i)
		}
	}
	if got, want := start-*budget, 100; got != want {
		t.Errorf("100 recursions spent %d units of the allowance, want %d", got, want)
	}

	// A copy of the shaper draws from the same allowance, because the allowance
	// belongs to the run and a shaper is copied per lookup.
	nested := sh
	nested.recurse()
	if got, want := start-*budget, 101; got != want {
		t.Errorf("a nested shaper spent %d units in total, want %d", got, want)
	}

	// And it runs out.
	for *budget > 0 {
		sh.recurse()
	}
	if sh.recurse() {
		t.Error("the allowance kept paying out after it was empty")
	}
}

// TestAnUnbudgetedShaperCannotRecurse pins the fail-closed answer. A shaper
// assembled by hand rather than by the one place that builds one is the state
// this bound exists to make impossible, so it is treated as one that has
// already spent everything rather than as one with no bound at all.
func TestAnUnbudgetedShaperCannotRecurse(t *testing.T) {
	var sh shaper
	if sh.recurse() {
		t.Error("a shaper with no allowance recursed")
	}
}

// TestTheAllowanceIsNotReachedByOrdinaryShaping is the other side of the bound:
// a limit that a real font trips is a correctness bug, not a safety property.
// One lookup naming another, applied over a paragraph's worth of text, must
// spend a small fraction of what a run is given.
func TestTheAllowanceIsNotReachedByOrdinaryShaping(t *testing.T) {
	rule := fonttest.SequenceContext1(map[int][]fonttest.ContextRule{
		gidB: {{Input: []int{gidB}, Lookups: []fonttest.SeqLookup{{At: 0, Lookup: 0}}}},
	})
	f := contextFace(t, []fonttest.Lookup{substB(), {Type: 5, Subtables: [][]byte{rule}}}, nil)

	text := ""
	for i := 0; i < 500; i++ {
		text += "b"
	}
	got := shapeWithin(t, f, text, 10*time.Second)
	if len(got) != 500 {
		t.Fatalf("shaping %d letters gave %d glyphs", 500, len(got))
	}
	for i, gid := range got {
		if gid != gidBalt {
			t.Fatalf("letter %d is glyph %d, want %d: the rule stopped applying part way "+
				"through, so the allowance is too small for ordinary text", i, gid, gidBalt)
		}
	}
}
