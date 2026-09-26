package layout

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// The shaping context, found in one pass and bounded to what a shaper uses.
// Audit C22 and C102.

// TestTheTableAgreesWithTheWalk is contextNeighbours checked against the walk
// it replaced, kept below as it was, over random paragraphs of runs, invisible
// runs, insets of either width facing either way, out-of-flow records, tabs,
// room, and both directions.
func TestTheTableAgreesWithTheWalk(t *testing.T) {
	face := joiningFace(t)
	other := style.Unit(7)
	r := rand.New(rand.NewPCG(3, 4))
	for trial := 0; trial < 3000; trial++ {
		items := make([]inlineItem, 1+r.IntN(10))
		for k := range items {
			var it inlineItem
			switch r.IntN(8) {
			case 0, 1, 2:
				it = inlineItem{Text: "د", Face: face, Width: 100}
				if r.IntN(4) == 0 {
					it.Size = other
				}
				if r.IntN(6) == 0 {
					it.Spacing.Letter = other
				}
			case 3:
				it = inlineItem{Text: "­", Face: face}
			case 4:
				it = inlineItem{Inset: true, InsetLead: r.IntN(2) == 0}
				if r.IntN(2) == 0 {
					it.InsetLeft = 100
				}
				if r.IntN(2) == 0 {
					it.InsetRight = 100
				}
			case 5:
				it = inlineItem{Abs: &Box{}}
			case 6:
				it = inlineItem{Tab: true, Text: "\t", Face: face}
			case 7:
				it = inlineItem{Text: "‍", Face: face}
			}
			it.Level = r.IntN(2)
			if r.IntN(10) == 0 {
				it.Room = 5
			}
			items[k] = it
		}
		nb := contextNeighbours(items)
		for i := range items {
			if !isShapedRun(items[i]) {
				continue
			}
			for _, step := range []int{-1, +1} {
				wj, wok := neighbourByWalk(items, i, step)
				got := nb.after[i]
				if step < 0 {
					got = nb.before[i]
				}
				if got.j != wj || got.ok != wok {
					t.Fatalf("trial %d, item %d, step %d: the table says (%d, %v) and "+
						"the walk (%d, %v)\n%+v", trial, i, step, got.j, got.ok, wj, wok, items)
				}
			}
		}
	}
}

// TestAContextIsAtMostWhatAShaperUses: the text either side of a run is cut to
// the characters nearest it, and cutting into the invisible characters between
// two runs is said to be — but cutting only into the neighbour's own word is
// not, since a shaper cannot use a word's far end.
func TestAContextIsAtMostWhatAShaperUses(t *testing.T) {
	face := joiningFace(t)
	word := inlineItem{Text: strings.Repeat("د", 200), Face: face, Width: 100}
	shy := inlineItem{Text: "­", Face: face}
	items := []inlineItem{word, word}
	text := newRunText(items)
	before, lost := text.before(0, 1, false)
	if n := len([]rune(before)); n != maxContextRunes || lost {
		t.Errorf("a long word before is %d characters of context, lost %v; want %d, false",
			n, lost, maxContextRunes)
	}
	after, lost := text.after(0, 1, false)
	if n := len([]rune(after)); n != maxContextRunes || lost {
		t.Errorf("a long word after is %d characters of context, lost %v", n, lost)
	}

	items = []inlineItem{word}
	for k := 0; k < 2*maxContextRunes; k++ {
		items = append(items, shy)
	}
	items = append(items, word)
	last := len(items) - 1
	text = newRunText(items)
	if _, lost := text.before(0, last, false); !lost {
		t.Error("cutting into the invisible characters before a run was not reported")
	}
	if _, lost := text.after(0, last, false); !lost {
		t.Error("cutting into the invisible characters after a run was not reported")
	}
	// A short one is kept whole, neighbour and all.
	items = []inlineItem{word, shy, shy, {Text: "د", Face: face, Width: 100}}
	text = newRunText(items)
	if got, _ := text.before(0, 3, false); !strings.HasSuffix(got, "د­­") {
		t.Errorf("the context before is %q", got)
	}
}

// contextFixture lays out a paragraph whose inline items are the shape named,
// in a face that joins, so that every run is given a context.
func contextFixture(t *testing.T, body string) {
	t.Helper()
	set, _ := notoNamed(t)
	built := Build(Input{HTML: `<div id="d">` + body + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#d { font-family: T; font-size: 16px }`}}})
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(1000000)
	Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
}

// TestAChainOfInvisibleItemsIsLinear is C22 in its three spellings: soft
// hyphens inside a word, zero width joiners each in a span, and a run of <b>s.
// Each run walked past every invisible item beside it and built its context
// from all of them, so the paragraph was quadratic in time and memory.
func TestAChainOfInvisibleItemsIsLinear(t *testing.T) {
	for _, tc := range []struct {
		name string
		body func(n int) string
	}{
		{"soft hyphens", func(n int) string { return "a" + strings.Repeat("­", n) + "b" }},
		{"joiners in spans", func(n int) string {
			return "a" + strings.Repeat("<span>‍</span>", n) + "b"
		}},
		{"many <b>s", func(n int) string { return strings.Repeat("<b>a</b>", n) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireLinear(t, tc.name, 500, func(n int) { contextFixture(t, tc.body(n)) })
		})
	}
}

// TestTheSeparatorPassIsLinear is C102: the phrase separator pass built its
// stretch's text a leaf at a time with +=, which copies the whole of it for
// every leaf.
//
// The pass is measured on its own, over a paragraph built beforehand, in a
// language with no phrase model: the stretch is still gathered — auto-phrase
// asks for it — and nothing else the pass does is large enough to hide it.
//
// What is measured is the bytes the pass allocates, because the defect is a
// copy: the += allocated the stretch again for every leaf, n²/2 bytes, where
// the pass otherwise allocates a list of the leaves and the stretch's text once.
// It was timed, over the same twenty thousand leaves and eighty thousand, and
// read 4.4 on a workstation and 8.0 to 9.9 on every GitHub runner. The work
// was linear; the tree of eighty thousand boxes is twenty-five megabytes, and
// the runner's cache held the smaller tree and not the larger, so each leaf
// cost twice as much at the larger size. The same timing reads 8.3 to 11.1 on
// a workstation at five thousand leaves and twenty thousand, which is where
// its cache sits. The bytes are the same number on every machine.
//
// The sizes are the large ones on purpose. A list grown by append allocates
// about twice its length while it is small and about five times once the
// runtime grows it by a quarter at a time, so below some tens of thousands of
// leaves the bytes of linear work grow faster than the leaves: five thousand
// against twenty thousand reads 6.5. Here it reads about four and a half, and
// the += planted back reads sixteen.
func TestTheSeparatorPassIsLinear(t *testing.T) {
	pass := func(n int) func() {
		b := Build(Input{
			HTML: `<p id="p" lang="en">` + strings.Repeat("<b>a</b>", n) + `</p>`,
			CSS:  []Stylesheet{{Source: `p { word-space-transform: ideographic-space auto-phrase }`}},
		})
		p := findBox(t, b.Root, "p")
		return func() { (&boxBuilder{rec: NewRecorder(nil)}).phraseSeparatorsAtABoxEdge(p) }
	}
	const n = 20000
	if r := costtest.Allocated(t, "the phrase separator pass", pass(n), pass(4*n)); r > 8 {
		t.Errorf("the phrase separator pass over %d leaves allocated %.1f times what it "+
			"did over %d; linear is about four, and a copy of the stretch per leaf "+
			"is sixteen", 4*n, r, n)
	}
}

// TestTheCommonBoxOfNeighboursIsNear: two boxes side by side deep in a tree
// share the box just above them, and finding it walked the whole of one's
// chain to the root first.
func TestTheCommonBoxOfNeighboursIsNear(t *testing.T) {
	requireLinear(t, "the box two neighbours share", 1, func(n int) {
		root := &Box{}
		cur := root
		for d := 0; d < 200*n; d++ {
			next := &Box{Parent: cur}
			cur.Children = append(cur.Children, next)
			cur = next
		}
		leaves := make([]*Box, 200*n)
		for k := range leaves {
			leaves[k] = &Box{Parent: cur}
		}
		for k := 1; k < len(leaves); k++ {
			if commonAncestor(leaves[k-1], leaves[k]) != cur {
				t.Fatal("the neighbours' common box is not their parent")
			}
		}
	})
	// And every shape of the question still has its answer.
	a := &Box{}
	b := &Box{Parent: a}
	c := &Box{Parent: b}
	d := &Box{Parent: a}
	for _, tc := range []struct{ x, y, want *Box }{
		{c, d, a}, {d, c, a}, {c, b, b}, {b, c, b}, {c, c, c}, {c, &Box{}, nil},
	} {
		if got := commonAncestor(tc.x, tc.y); got != tc.want {
			t.Errorf("commonAncestor = %p, want %p", got, tc.want)
		}
	}
}

// TestABreakAtTheFarEndOfAChainIsFoundOnce: whether two runs may share a glyph
// asks whether a break opportunity lies between them, and it walked every item
// between them to find out — for every run of a chain of invisible ones, each
// of which has the same neighbour at the far end, where the break is. Planted,
// the walk reads as 14.4.
//
// Timed through costtest.TimeCopies. An item is half a kilobyte, so the chain
// of eight thousand is four megabytes and the chain of two thousand one, and
// with memory being streamed on the machine's other cores — which is what a
// shared runner is — the smaller chain stayed in the cache and the larger did
// not, and the linear walk read as 7.5 to 12.3 at this size and smaller ones
// alike. Four chains of the smaller size are the larger's memory.
func TestABreakAtTheFarEndOfAChainIsFoundOnce(t *testing.T) {
	face := joiningFace(t)
	chain := func(n int) func() {
		var items []inlineItem
		items = append(items, inlineItem{Text: "a", Face: face, Width: 100})
		for k := 0; k < n; k++ {
			items = append(items, inlineItem{Text: "⁠", Face: face})
		}
		items = append(items, inlineItem{Text: "b", Face: face, Width: 100, BreakBefore: true})
		return func() {
			g := mergeGroupTexts(items, contextNeighbours(items), func() runText { return newRunText(items) })
			if !g.empty() {
				t.Fatal("runs with a break between them were grouped")
			}
		}
	}
	const n = 2000
	c := costtest.TimeCopies(t, "a break at the end of a chain",
		func(int) func() { return chain(n) }, chain(4*n))
	if c.Ratio > 8 {
		t.Errorf("a break at the end of a chain: four times the input took %v; linear "+
			"work is about four, and quadratic is about sixteen", c)
	}
}

// neighbourByWalk is the neighbour walk as it was, one run at a time: the
// oracle contextNeighbours is checked against.
func neighbourByWalk(items []inlineItem, i, step int) (int, bool) {
	// The last item that draws nothing, which is the answer where there is no
	// run beyond it: a zero width joiner written at the edge of a box is the
	// whole of the context, and its characters are what says which form the
	// letter beside it takes. The suite's shaping-join-002 is a table cell
	// holding "&zwj;&#x0627;&zwj;" and nothing else.
	last, blank := 0, false
	// Room between the two, declared by the item that spends it. §8.1 breaks
	// shaping where there is room between the characters, which is the same rule
	// the insets below are read by — an inside list marker's half-em is that
	// room, and the marker and the item's text are two strings and not one.
	//
	// It is also what keeps such an item's width right, and that is worth saying
	// because it looks like two rules. A run inside a group is measured again
	// from its own text, and an item whose width is more than its text would
	// lose the difference — so the width could be repaired instead, by adding
	// Room back after the measure. Both were written and each was measured to
	// cover the other exactly: with either one in place the marker keeps its
	// gap, and only removing both loses it. The repair is the one that went,
	// because it answers a case this rule does not allow to arise, and a guard
	// that has never been seen to fire is not a guard.
	if step > 0 && items[i].Room != 0 {
		return last, blank
	}
	for j := i + step; j >= 0 && j < len(items); j += step {
		switch {
		case items[j].Abs != nil || items[j].Float != nil:
			// Out of flow: written between the two runs and drawn somewhere
			// else entirely, so it stands between nothing.
			continue
		case drawsNothing(items[j]):
			last, blank = j, true
			// A character that sets no paper and takes no room, which between
			// two runs is the same nothing an out-of-flow box is. The soft
			// hyphen is the one a document writes inside a word: SplitAtBreaks
			// keeps it, so it is an item of its own, and treating it as a run
			// gave the run on each side a context consisting of one invisible
			// character and nothing else. Arabic either side of a "&shy;" came
			// out in isolated forms — a joined word broken into letters by a
			// mark that is not drawn at all.
			//
			// The text is *kept* rather than skipped over: textBetween gathers
			// it back, because a shaper reads the ignorable characters as
			// transparent and a context with a hole in it is a different
			// context.
			continue
		case items[j].Inset:
			// An inline box's own margin, border and padding. A zero one is a
			// boundary and nothing more — which is what shaping-004 through
			// -006 are — and one with width is room between the letters, which
			// is what -009 through -011 are.
			if facingInset(items[j], items[i].Level&1 == 1) == 0 {
				continue
			}
			return last, blank
		case !isShapedRun(items[j]):
			return last, blank
		}
		if !sameShaping(items[i], items[j]) {
			return last, blank
		}
		if step < 0 && items[j].Room != 0 {
			// The same room, reached from the other side. The boundary is what
			// the rule is about, not which of the two runs asks about it.
			//
			// No document reaches this and a planted defect removing it changes
			// nothing: the only thing that spends room is an inside list
			// marker, which leads its line, so nothing is ever looking back
			// past one. It stays because the half above would otherwise be a
			// rule about markers rather than about room, and the next item that
			// spends some need not lead anything.
			return last, blank
		}
		return j, true
	}
	return last, blank
}
