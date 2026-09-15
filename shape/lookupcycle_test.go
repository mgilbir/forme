package shape

import (
	"testing"
	"time"

	"github.com/mgilbir/forme/fonttest"
)

// TestAContextualLookupThatCallsItselfTerminates is the bound between a font a
// page links and a stack that runs out.
//
// applyGSUBAt's own comment states the threat: "A font can describe a cycle —
// rule A applying lookup B which applies A — and nothing in the format forbids
// it, so the depth is what stops it." Nothing had ever made one: every
// contextual fixture in this package describes a finite chain.
//
// Two bounds stand here, and what each is worth is worth stating. Raising
// maxLookupRecursion alone does not fail this, and should not: the ops
// allowance catches the cycle first, which is what the code says it is for —
// "depth alone does not bound this work, and for a while it was all that did".
// Raising the allowance alone is caught by the width test at the bottom of this
// file. Raising both is caught here, as a stack overflow, which is the fault
// itself rather than a report of it.
//
// The shortest cycle is a lookup that names itself. It is one rule: on this
// glyph, apply lookup 0 at position 0 — and lookup 0 is the rule. A font may
// say that, and OpenType has nothing to say against it.
func TestAContextualLookupThatCallsItselfTerminates(t *testing.T) {
	rule := fonttest.SequenceContext1(map[int][]fonttest.ContextRule{
		gidB: {{
			Input:   []int{gidB},
			Lookups: []fonttest.SeqLookup{{At: 0, Lookup: 0}},
		}},
	})
	f := contextFace(t, []fonttest.Lookup{{Type: 5, Subtables: [][]byte{rule}}}, nil)

	// In a goroutine with a deadline, which catches the fault that is bounded
	// but enormous — a tree of applications the ops allowance below is there
	// for. It does not catch a stack overflow: that is fatal to the process
	// whichever goroutine it happens on, and shows up as a crashed run rather
	// than as this test saying what went wrong. Both are failures and only one
	// of them is legible, which is why the deadline is here at all.
	done := make(chan []Glyph, 1)
	go func() {
		glyphs, _ := f.ShapeGlyphs("b")
		done <- glyphs
	}()
	select {
	case glyphs := <-done:
		if len(glyphs) != 1 {
			t.Errorf("shaping one glyph gave %d; the cycle must be stopped, not "+
				"the run rewritten", len(glyphs))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shaping one glyph through a lookup that applies itself did not " +
			"finish; a font may describe a cycle and the depth is what stops it")
	}
}

// TestALookupCycleOfTwoTerminates is the same threat written the way the
// comment states it, because a font need not name itself to make a cycle and a
// bound that only caught the self-naming case would be worth less than it looks.
func TestALookupCycleOfTwoTerminates(t *testing.T) {
	// Lookup 0 applies lookup 1; lookup 1 applies lookup 0.
	ruleA := fonttest.SequenceContext1(map[int][]fonttest.ContextRule{
		gidB: {{Input: []int{gidB}, Lookups: []fonttest.SeqLookup{{At: 0, Lookup: 1}}}},
	})
	ruleB := fonttest.SequenceContext1(map[int][]fonttest.ContextRule{
		gidB: {{Input: []int{gidB}, Lookups: []fonttest.SeqLookup{{At: 0, Lookup: 0}}}},
	})
	f := contextFace(t, []fonttest.Lookup{
		{Type: 5, Subtables: [][]byte{ruleA}},
		{Type: 5, Subtables: [][]byte{ruleB}},
	}, nil)

	done := make(chan []Glyph, 1)
	go func() {
		glyphs, _ := f.ShapeGlyphs("b")
		done <- glyphs
	}()
	select {
	case glyphs := <-done:
		if len(glyphs) != 1 {
			t.Errorf("shaping one glyph gave %d", len(glyphs))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("two lookups that apply each other did not finish")
	}
}

// TestARuleThatNamesItsOwnLookupFortyTimesTerminates is the other half of the
// same threat, and the half the depth bound cannot see.
//
// applyGSUBAt's comment sets it out: a matched rule names a list of lookup
// records, nothing stops those records from naming the rule's own lookup, and
// "forty of them, forty times over, eight levels deep, is forty to the eighth
// applications from a few hundred bytes of GSUB. It is not a cycle the depth
// catches; it is a tree, and the depth bounds its height while the records
// decide its width."
//
// The allowance that bounds the width was written for exactly this and **the
// whole shape suite passes with it raised past anything a run could reach**.
// Every contextual fixture in the package names one or two lookups from a rule,
// so the width has never been the thing under test. This is the font that makes
// it the thing.
//
// The allowance is the clamp of three numbers and all three have to be raised to
// plant against it: leaving the ceiling at 1<<29 still bounds the tree, and a
// plant that raises only the other two passes — which reads as a weak test and
// is a weak plant.
func TestARuleThatNamesItsOwnLookupFortyTimesTerminates(t *testing.T) {
	const records = 40
	seq := make([]fonttest.SeqLookup, records)
	for i := range seq {
		seq[i] = fonttest.SeqLookup{At: 0, Lookup: 0}
	}
	rule := fonttest.SequenceContext1(map[int][]fonttest.ContextRule{
		gidB: {{Input: []int{gidB}, Lookups: seq}},
	})
	f := contextFace(t, []fonttest.Lookup{{Type: 5, Subtables: [][]byte{rule}}}, nil)

	done := make(chan []Glyph, 1)
	go func() {
		glyphs, _ := f.ShapeGlyphs("b")
		done <- glyphs
	}()
	select {
	case glyphs := <-done:
		if len(glyphs) != 1 {
			t.Errorf("shaping one glyph gave %d", len(glyphs))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a rule naming its own lookup forty times did not finish; the " +
			"depth bounds the height of that tree and only the allowance bounds " +
			"its width")
	}
}
