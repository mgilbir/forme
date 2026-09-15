package brotli

import (
	"strings"
	"testing"
)

// TestADistanceThatWindsBackPastTheStartIsRefused is the bound on the distance
// a stream may name.
//
// Twelve of the sixteen short distance codes are a remembered distance give or
// take one, two or three. At the start of a stream the remembered distances are
// small, so a code asking for "the most recent, minus three" computes a distance
// at or below zero: a reference to before the beginning of the output.
//
// Naming an impossible distance rather than failing on the spot is deliberate,
// and the comment says so: "Past the start of the stream. Naming an impossible
// distance rather than failing here keeps the check in one place, below." The
// impossible distance is maxAllowedDistance+1, and the check below is what turns
// it into a refusal — so the constant is doing two jobs, and the one that
// matters is being past what any real distance can be.
//
// Nothing had reached it: every stream in the corpus names distances that exist,
// so the brotli suite passes with maxAllowedDistance raised.
func TestADistanceThatWindsBackPastTheStartIsRefused(t *testing.T) {
	// A decoder whose remembered distances are small. The format begins with
	// {16, 15, 11, 4}, and minus three of any of those is still a distance — so
	// the state that reaches this is one a stream arrives at rather than starts
	// from: a copy at distance one is legal, and remembering it puts a 1 in the
	// ring buffer. After that, "that distance, minus three" is -2.
	d := &decoder{distRB: [4]int{1, 2, 3, 1}, distIdx: 0}

	// Twelve of the sixteen codes are a remembered distance give or take one,
	// two or three; with the buffer above, at least one of them winds back past
	// the beginning of the output.
	var impossible int
	for code := 4; code < 16; code++ {
		dd := &decoder{distRB: d.distRB, distIdx: d.distIdx}
		got, _ := dd.recentDistance(code)
		if got > maxAllowedDistance {
			impossible = got
			break
		}
	}
	if impossible == 0 {
		t.Fatal("no short code wound back past the start of the stream; the " +
			"fixture does not reach what the bound is being asked about")
	}
	if impossible != maxAllowedDistance+1 {
		t.Errorf("a distance past the start came out as %d, not %d; it is named as "+
			"one past the bound so that the single check below refuses it",
			impossible, maxAllowedDistance+1)
	}

	// And that check refuses it, rather than reading a dictionary word from it.
	dd := &decoder{distRB: d.distRB, distIdx: d.distIdx}
	if _, err := dd.dictionaryWord(impossible, 0, 4, 0); err == nil {
		t.Errorf("a distance of %d was read as a dictionary word; past the bound it "+
			"is refused", impossible)
	} else if !strings.Contains(err.Error(), "before the start of the stream") {
		t.Errorf("it was refused with %q; the refusal that belongs here is the one "+
			"the bound produces, not something else the walk noticed", err)
	}

	// The other half, and the half that can fail. Everything above is written
	// in terms of maxAllowedDistance — the impossible distance *is* the bound
	// plus one — so raising the bound scales the sentinel, the comparison and
	// the refusal together and changes nothing. That is not a weak test so much
	// as a true fact about the constant: its whole job is to be larger than any
	// distance a stream can name, and making it larger still does that job.
	//
	// What it must not be is *small*. A bound below a distance a real stream
	// names turns an ordinary back-reference into "before the start of the
	// stream", and the stream is refused for naming something it was entitled
	// to name. These are the ordinary distances, and they have to survive.
	for _, distance := range []int{1, 2, 3, 4, 16, 1000, 1 << 20, 1 << 24} {
		od := &decoder{distRB: d.distRB, distIdx: d.distIdx}
		_, err := od.dictionaryWord(distance, 0, 4, 0)
		if err != nil && strings.Contains(err.Error(), "before the start of the stream") {
			t.Errorf("an ordinary distance of %d was refused as a reference to "+
				"before the start; the bound has to sit above every distance a "+
				"stream may name", distance)
		}
	}
}
