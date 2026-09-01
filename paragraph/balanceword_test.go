package paragraph

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A balanced measure may not be one that breaks a word open.
//
// §5.5 gives "overflow-wrap: break-word" its opportunities as a last resort —
// "if there are no otherwise-acceptable break points in the line" — so they are
// not something a balancer may reach for to even two lines out. The suite's
// text-wrap-balance-overflow-001 is "CONTROLLING YOUR BU" in fifteen characters
// with break-word set: the narrowest measure that still makes two lines is ten,
// where it reads "CONTROLLIN" and "G YOUR BU", and the reference is
// "CONTROLLING" and "YOUR BU".

// linesAt is the paragraph broken greedily at a width, as text.
func linesAt(t *testing.T, br *Breaker, items []Item, width style.Unit) []string {
	t.Helper()
	var out []string
	iByte := 0
	for i := 0; i < len(items) && len(out) < 10; {
		runs, next, nextByte, _, _ := br.BreakOneLine(items, i, iByte, width, 0)
		var b strings.Builder
		for _, r := range runs {
			b.WriteString(r.Text)
		}
		out = append(out, b.String())
		if !CursorAdvanced(i, iByte, next, nextByte) {
			break
		}
		i, iByte = next, nextByte
	}
	return out
}

// balanceWordItems is the suite's paragraph, at its own width.
func balanceWordItems(t *testing.T, br *Breaker, ow OverflowWrap) ([]Item, style.Unit) {
	t.Helper()
	// Courier at 20px is 12px a character, and the box is fifteen of them.
	return itemsOf(t, br, courier(t), "CONTROLLING YOUR BU",
		WhiteSpaceOf("collapse"), ow), u(15 * 12)
}

// TestBalancingDoesNotBreakAWordOpen.
func TestBalancingDoesNotBreakAWordOpen(t *testing.T) {
	br := NewBreaker(nil)
	items, width := balanceWordItems(t, br, OverflowWrap{BreakWord: true})
	cap := br.BalanceWidth(items, width, 0)
	got := linesAt(t, br, items, style.Min(cap, width))
	want := []string{"CONTROLLING", "YOUR BU"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("balanced to %v, want %v — break-word's opportunities are a "+
			"last resort and not something a balancer may reach for", got, want)
	}
}

// TestBalancingStillNarrowsAsFarAsTheWordsAllow, so the rule above is a floor
// under the search and not the search turned off.
func TestBalancingStillNarrowsAsFarAsTheWordsAllow(t *testing.T) {
	br := NewBreaker(nil)
	items, width := balanceWordItems(t, br, OverflowWrap{BreakWord: true})
	cap := br.BalanceWidth(items, width, 0)
	if cap >= width {
		t.Errorf("the balanced cap is %v against a width of %v; nothing was "+
			"balanced at all", cap, width)
	}
	// Eleven characters is the longest word, and the cap may not go below it.
	if cap < u(11*12) {
		t.Errorf("the balanced cap is %v, narrower than the %v the longest word "+
			"needs", cap, u(11*12))
	}
}

// TestAWordWiderThanTheRoomIsStillBroken. The rule has to give way where the
// word does not fit at the full width either: refusing then would leave the
// search with nothing to return, and the paragraph unbalanced for a reason the
// author cannot see.
func TestAWordWiderThanTheRoomIsStillBroken(t *testing.T) {
	br := NewBreaker(nil)
	items := itemsOf(t, br, courier(t), "abcdefghijkl xx yy zz",
		WhiteSpaceOf("collapse"), OverflowWrap{BreakWord: true})
	// Ten characters, against a twelve-character word.
	const width = 10 * 12
	full, _ := br.countLines(items, u(width), 0, 99)
	cap := br.BalanceWidth(items, u(width), 0)
	n, _ := br.countLines(items, style.Min(cap, u(width)), 0, 99)
	if n != full {
		t.Errorf("balancing turned %d lines into %d; the count is the one thing "+
			"it may not change", full, n)
	}
	// And it balanced: a rule that refused every measure that opens a word
	// would leave this paragraph at its full width, because every measure
	// narrower than the word opens one.
	if cap >= u(width) {
		t.Errorf("the balanced cap is %v against a width of %v; the word is "+
			"wider than the room either way, so refusing to open one leaves "+
			"nothing to choose", cap, u(width))
	}
}

// TestOverflowWrapAnywhereMayStillBeBalancedInto, which is the value's whole
// difference from break-word: §5.5 gives its opportunities to the intrinsic
// sizes as well, so they are real ones.
func TestOverflowWrapAnywhereMayStillBeBalancedInto(t *testing.T) {
	br := NewBreaker(nil)
	// The property sets both flags — see OverflowWrapOf — and Anywhere is the
	// one that says the opportunities are real.
	items, width := balanceWordItems(t, br,
		OverflowWrap{BreakWord: true, Anywhere: true})
	cap := br.BalanceWidth(items, width, 0)
	if cap >= u(11*12) {
		t.Errorf("with overflow-wrap: anywhere the balanced cap is %v; the "+
			"value's opportunities are real ones and the search may use them",
			cap)
	}
}

// TestTheScoredSearchDoesNotBreakAWordOpenEither. It is the search that runs
// where a float has made the lines different lengths, and it chooses between
// break sets rather than narrowing a measure — so it needs the rule said again
// in its own terms.
func TestTheScoredSearchDoesNotBreakAWordOpenEither(t *testing.T) {
	br := NewBreaker(nil)
	items, width := balanceWordItems(t, br, OverflowWrap{BreakWord: true})
	caps := br.BalanceScoredCaps(items, []style.Unit{width, width}, 0, 2)
	if len(caps) != 2 {
		t.Fatalf("the scored search returned %d caps, want 2", len(caps))
	}
	runs, _, nextByte, _, _ := br.BreakOneLine(items, 0, 0, style.Min(caps[0], width), 0)
	var first string
	for _, r := range runs {
		first += r.Text
	}
	if nextByte != 0 {
		t.Errorf("the scored search broke the first line inside a word, at %q",
			first)
	}
	if first != "CONTROLLING" {
		t.Errorf("the scored search's first line is %q, want %q", first,
			"CONTROLLING")
	}
}
