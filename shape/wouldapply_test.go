package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestWouldApplyTakesExactlyTheSequence holds wouldApply to HarfBuzz's
// would_apply, one subtable form at a time, for the probe the Indic model
// sends: two glyphs and nothing around them. A lookup would apply only if it
// takes exactly those two — no fewer, no more — and, with no context allowed,
// needs nothing before or after them.
func TestWouldApplyTakesExactlyTheSequence(t *testing.T) {
	const a, b, c, before = 7, 9, 11, 5
	pair := []int{a, b}
	seq := func(first int, input []int) map[int][]fonttest.ContextRule {
		return map[int][]fonttest.ContextRule{first: {{Input: input}}}
	}
	chain := func(first int, back, input, ahead []int) map[int][]fonttest.ChainRule {
		return map[int][]fonttest.ChainRule{first: {{Backtrack: back, Input: input, Lookahead: ahead}}}
	}
	for _, tc := range []struct {
		what        string
		kind        int
		sub         []byte
		gids        []int
		zeroContext bool
		want        bool
	}{
		{"a single substitution, asked about two", 1, fonttest.SingleSubst([]int{a}, []int{c}), pair, true, false},
		{"a single substitution, asked about one", 1, fonttest.SingleSubst([]int{a}, []int{c}), []int{a}, true, true},
		{"a multiple substitution, asked about two", 2, fonttest.MultipleSubst([]int{a}, [][]int{{b, c}}), pair, true, false},
		{"a ligature of the two", 4, fonttest.LigatureSubst([]fonttest.Ligature{{Components: pair, Glyph: c}}), pair, true, true},
		{"a ligature of three", 4, fonttest.LigatureSubst([]fonttest.Ligature{{Components: []int{a, b, c}, Glyph: c}}), pair, true, false},
		{"a ligature of the reverse", 4, fonttest.LigatureSubst([]fonttest.Ligature{{Components: []int{b, a}, Glyph: c}}), pair, true, false},
		{"a context rule by glyph", 5, fonttest.SequenceContext1(seq(a, pair)), pair, true, true},
		{"a longer context rule by glyph", 5, fonttest.SequenceContext1(seq(a, []int{a, b, c})), pair, true, false},
		{"a context rule by class", 5, fonttest.SequenceContext2([]int{a}, map[int]int{a: 1, b: 2},
			map[int][]fonttest.ContextRule{1: {{Input: []int{1, 2}}}}), pair, true, true},
		{"a context rule by another class", 5, fonttest.SequenceContext2([]int{a}, map[int]int{a: 1, b: 2},
			map[int][]fonttest.ContextRule{1: {{Input: []int{1, 3}}}}), pair, true, false},
		{"a context rule by coverage", 5, fonttest.SequenceContext3([][]int{{a}, {b}}, nil), pair, true, true},
		{"a context rule by other coverage", 5, fonttest.SequenceContext3([][]int{{a}, {c}}, nil), pair, true, false},
		{"a chained rule with backtrack, no context allowed", 6, fonttest.ChainedContext1(chain(a, []int{before}, pair, nil)), pair, true, false},
		{"a chained rule with backtrack, context ignored", 6, fonttest.ChainedContext1(chain(a, []int{before}, pair, nil)), pair, false, true},
		{"a chained rule by class with lookahead, no context allowed", 6, fonttest.ChainedContext2([]int{a}, nil,
			map[int]int{a: 1, b: 2}, map[int]int{c: 1},
			map[int][]fonttest.ChainRule{1: {{Input: []int{1, 2}, Lookahead: []int{1}}}}), pair, true, false},
		{"a chained rule by class, no context", 6, fonttest.ChainedContext2([]int{a}, nil,
			map[int]int{a: 1, b: 2}, nil,
			map[int][]fonttest.ChainRule{1: {{Input: []int{1, 2}}}}), pair, true, true},
		{"a chained rule by coverage, no context", 6, fonttest.ChainedContext3(nil, [][]int{{a}, {b}}, nil, nil), pair, true, true},
		{"a chained rule by coverage with backtrack", 6, fonttest.ChainedContext3([][]int{{before}}, [][]int{{a}, {b}}, nil, nil), pair, true, false},
		{"a reverse chaining substitution", 8, fonttest.ReverseChainSubst([]int{a}, []int{c}, nil, nil), pair, true, false},
	} {
		if got := wouldApply(tc.kind, tc.sub, tc.gids, tc.zeroContext); got != tc.want {
			t.Errorf("%s: would apply %t, want %t", tc.what, got, tc.want)
		}
	}
}
