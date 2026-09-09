package bidi

import (
	"testing"
	"unicode/utf8"
)

// The algorithm over arbitrary text.
//
// The conformance suite next door runs UAX #9's own 861,948 cases and settles
// what the answers *are*. What it cannot settle is what happens to text nobody
// tabulated: the algorithm is index arithmetic over a stack of embedding levels
// and a stack of bracket pairs, and every one of its rules is written about
// positions in a slice. A caller hands it a document's text, which is untrusted,
// and a panic here is the embedder's process.
//
// So this checks the contract rather than the answers:
//
//   - Totality. Resolve never panics and always terminates.
//   - Shape. There is a level for every character, and every level is inside
//     UAX #9's own maximum depth.
//   - Order. VisualOrder returns a permutation — every index once, none twice,
//     none out of range — which is what makes it safe to index a run with.
//   - Agreement. LineLevels over the whole paragraph agrees with Levels, since
//     rule L1 has already been applied with the paragraph taken as one line.

func FuzzResolve(f *testing.F) {
	seeds := []string{
		"", "abc", "אבג", "abc אבג",
		// The controls, which are the part with a stack behind it.
		"a‪b‬c",  // LRE, PDF
		"a‫b‬c",  // RLE, PDF
		"a‭b‮c‬", // LRO, RLO
		"a⁦b⁩c",  // LRI, PDI
		"a⁧b⁨c",  // RLI, FSI
		"‬⁩‬",    // pops with nothing pushed
		// Brackets, which are the other stack.
		"a(b)c", "א(ב)ג", "a(א)b", "((((((((((((((((((((a",
		// Numbers and separators, which rules W1 to W7 rewrite.
		"1,234.5", "א1,234.5", "a٠١b",
		// White space at the end, which is rule L1's fourth clause.
		"abc אבג   ", "\t\n ",
		// Not text at all.
		"�", "\x00\x01\x02",
	}
	for _, s := range seeds {
		for _, d := range []int{0, 1, 2} {
			f.Add(s, d)
		}
	}

	f.Fuzz(func(t *testing.T, s string, dirN int) {
		// Bounded: a fuzzer is looking for logic faults, and a very long string
		// only finds the time it takes to walk one.
		if len(s) > 1<<14 {
			return
		}
		dir := Direction(dirN % 3)
		text := []rune(s)

		p := Resolve(text, dir)
		if p == nil {
			t.Fatal("Resolve returned nothing")
		}
		if got := p.Len(); got != len(text) {
			t.Fatalf("Resolve answered about %d characters and was given %d", got, len(text))
		}
		levels := p.Levels()
		if len(levels) != len(text) {
			t.Fatalf("%d levels for %d characters", len(levels), len(text))
		}
		if lv := p.Level(); lv != 0 && lv != 1 {
			t.Fatalf("the paragraph level is %d, want 0 or 1", lv)
		}
		for i, lv := range levels {
			// UAX #9 caps the explicit depth at 125, and an implicit level adds
			// at most two to it. A level outside that is a stack that ran away,
			// and it is the number every caller indexes an array of runs by.
			if lv < 0 || lv > MaxDepth+2 {
				t.Fatalf("character %d has level %d, outside 0..%d", i, lv, MaxDepth+2)
			}
		}

		// VisualOrder has to be a permutation: a caller reorders a line's runs
		// with it, so a repeated index draws a run twice and a missing one drops
		// it off the page.
		order := VisualOrder(levels)
		if len(order) != len(levels) {
			t.Fatalf("VisualOrder returned %d positions for %d levels", len(order), len(levels))
		}
		seen := make([]bool, len(levels))
		for _, i := range order {
			if i < 0 || i >= len(levels) {
				t.Fatalf("VisualOrder returned index %d, outside 0..%d", i, len(levels)-1)
			}
			if seen[i] {
				t.Fatalf("VisualOrder returned index %d twice", i)
			}
			seen[i] = true
		}

		// The whole paragraph as one line is the state Resolve already left the
		// levels in, so the two have to agree. A disagreement means L1 was
		// applied twice or not at all.
		if line := p.LineLevels(0, len(text)); len(text) > 0 {
			if len(line) != len(levels) {
				t.Fatalf("LineLevels over the whole paragraph gave %d levels for %d characters",
					len(line), len(levels))
			}
			for i := range line {
				if line[i] != levels[i] {
					t.Fatalf("character %d is level %d in the paragraph and %d on "+
						"the line that is the whole paragraph", i, levels[i], line[i])
				}
			}
		}

		// A range nobody could mean is a range that must not panic either.
		p.LineLevels(-5, len(text)+5)
		p.LineLevels(len(text), 0)

		// The run views over the same text, which every caller of this package
		// uses rather than the levels directly.
		for _, r := range VisualRuns(s) {
			if r.Start < 0 || r.End > len(s) || r.Start > r.End {
				t.Fatalf("a visual run spans %d..%d of a %d-byte string", r.Start, r.End, len(s))
			}
			if !utf8.ValidString(s) {
				continue
			}
			if !utf8.RuneStart(s[r.Start]) && r.Start < len(s) {
				t.Fatalf("a visual run starts at %d, which is inside a character", r.Start)
			}
		}
	})
}
