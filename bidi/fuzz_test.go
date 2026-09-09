package bidi

import (
	"testing"
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
//   - Runs. The run views are what every caller of this package actually uses,
//     and a run is a pair of byte offsets someone slices a string with. They
//     have to cover the string exactly once, cut it only where a character
//     begins, and carry the level Resolve gave the characters inside them;
//     VisualRuns has to be a permutation of LogicalRuns, and RunCharacters has
//     to return one offset per character of the run it was handed. The shortcut
//     that skips the algorithm entirely is checked against the algorithm here,
//     because its soundness is an argument about a table and a table changes.

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
		// uses rather than the levels directly. A run is a pair of byte offsets
		// a caller slices the string with, so what has to hold of them is
		// stronger than "in range": they have to cover the string exactly once,
		// and cut it only where a character begins.
		//
		// The offsets a character begins at are taken from range rather than
		// from utf8.RuneStart, so that the check means the same thing for text
		// that is not UTF-8 — a byte that is not a character is read as one
		// U+FFFD by both the run walk and this, and a run may begin there.
		starts := map[int]bool{len(s): true}
		for i := range s {
			starts[i] = true
		}

		logical := LogicalRuns(s)
		at := 0
		for k, r := range logical {
			if r.Start != at {
				t.Fatalf("run %d begins at %d and the one before it ended at %d; "+
					"the runs do not cover %q", k, r.Start, at, s)
			}
			if r.End <= r.Start || r.End > len(s) {
				t.Fatalf("run %d spans %d..%d of a %d-byte string", k, r.Start, r.End, len(s))
			}
			if !starts[r.Start] || !starts[r.End] {
				t.Fatalf("run %d spans %d..%d, which is not where characters begin in %q",
					k, r.Start, r.End, s)
			}
			if r.Level < 0 || r.Level > MaxDepth+2 {
				t.Fatalf("run %d has level %d, outside 0..%d", k, r.Level, MaxDepth+2)
			}
			at = r.End
		}
		if at != len(s) {
			t.Fatalf("the runs of %q cover %d of its %d bytes", s, at, len(s))
		}

		// The shortcut against the algorithm it is short-cutting. LogicalRuns
		// answers without running anything when NeedsAlgorithm says no, and the
		// argument for that is a claim about which characters can lift a level —
		// which is a claim about a table, and a table is a thing that changes.
		full := ResolveRuns(s)
		if s != "" && !runsEqual(logical, full) {
			t.Fatalf("LogicalRuns says %v for %q and the algorithm says %v",
				logical, s, full)
		}

		// And against the levels, which is the other half: the runs are a view
		// of Resolve's answer, so a character's run has to carry a character's
		// level. Auto is the direction, because that is what the run walk uses.
		auto := Resolve([]rune(s), Auto).Levels()
		k, chars := 0, 0
		for i := range s {
			for k < len(logical) && i >= logical[k].End {
				k++
			}
			if k >= len(logical) {
				t.Fatalf("byte %d of %q is in no run", i, s)
			}
			if chars >= len(auto) {
				t.Fatalf("%q has more characters than Resolve gave levels for", s)
			}
			if logical[k].Level != auto[chars] {
				t.Fatalf("character %d of %q is level %d by Resolve and %d by its run",
					chars, s, auto[chars], logical[k].Level)
			}
			chars++
		}
		if chars != len(auto) {
			t.Fatalf("%q walks as %d characters and Resolve gave %d levels", s, chars, len(auto))
		}

		// VisualRuns is the same runs in another order, so it is a permutation
		// of them: a run drawn twice paints over the page and one left out drops
		// text off it.
		visual := VisualRuns(s)
		if len(visual) != len(logical) {
			t.Fatalf("%d visual runs and %d logical ones in %q",
				len(visual), len(logical), s)
		}
		placed := make([]bool, len(logical))
		for _, r := range visual {
			found := false
			for k := range logical {
				if !placed[k] && logical[k] == r {
					placed[k], found = true, true
					break
				}
			}
			if !found {
				t.Fatalf("visual run %v is not one of the logical runs %v of %q",
					r, logical, s)
			}
		}

		// RunCharacters is what a caller sets a run with, and the offsets it
		// returns are what maps a glyph back to the text. Rule L4 replaces a
		// character with its mirror and must not change how many there are or
		// where they came from.
		for _, rtl := range []bool{false, true} {
			for _, r := range logical {
				runes, offsets := RunCharacters(s[r.Start:r.End], rtl)
				if len(runes) != len(offsets) {
					t.Fatalf("%d characters and %d offsets in the run %d..%d of %q",
						len(runes), len(offsets), r.Start, r.End, s)
				}
				want := 0
				for i := range s[r.Start:r.End] {
					if want >= len(offsets) {
						t.Fatalf("the run %d..%d of %q walks as more characters "+
							"than RunCharacters returned", r.Start, r.End, s)
					}
					if offsets[want] != i {
						t.Fatalf("character %d of the run %d..%d of %q came from "+
							"offset %d and the string says %d",
							want, r.Start, r.End, s, offsets[want], i)
					}
					want++
				}
				if want != len(offsets) {
					t.Fatalf("the run %d..%d of %q is %d characters and "+
						"RunCharacters returned %d", r.Start, r.End, s, want, len(offsets))
				}
			}
		}
	})
}

// runsEqual reports whether two run lists are the same.
func runsEqual(a, b []Run) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
