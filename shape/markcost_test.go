package shape

import (
	"math/rand"
	"strings"
	"testing"
	"time"
)

// referenceCanonicalOrder is UAX #15's canonical ordering written the way the
// standard states it: one insertion sort over the whole run. It is what
// orderCanonically replaced, and it is kept here as the thing the fast one has
// to agree with.
func referenceCanonicalOrder(runes []rune, from []int) {
	for i := 1; i < len(runes); i++ {
		cc := CombiningClass(runes[i])
		if cc == 0 {
			continue
		}
		for j := i; j > 0 && CombiningClass(runes[j-1]) > cc; j-- {
			runes[j-1], runes[j] = runes[j], runes[j-1]
			from[j-1], from[j] = from[j], from[j-1]
		}
	}
}

func sameOrder(t *testing.T, runes []rune) {
	t.Helper()
	fast, fastFrom := append([]rune(nil), runes...), make([]int, len(runes))
	slow, slowFrom := append([]rune(nil), runes...), make([]int, len(runes))
	for i := range runes {
		fastFrom[i], slowFrom[i] = i, i
	}
	// orderCanonically is given one run of marks at a time, which is how
	// ComposeCanonically calls it; the reference skips starters itself.
	for start := 0; start < len(fast); {
		if CombiningClass(fast[start]) == 0 {
			start++
			continue
		}
		end := start
		for end < len(fast) && CombiningClass(fast[end]) != 0 {
			end++
		}
		orderCanonically(fast[start:end], fastFrom[start:end])
		start = end
	}
	referenceCanonicalOrder(slow, slowFrom)

	if string(fast) != string(slow) {
		t.Errorf("%q ordered as %q, and the standard's own insertion sort gives %q",
			string(runes), string(fast), string(slow))
		return
	}
	for i := range fastFrom {
		if fastFrom[i] != slowFrom[i] {
			t.Errorf("%q: the offsets came out %v and the insertion sort gives %v — "+
				"the sort is not stable", string(runes), fastFrom, slowFrom)
			return
		}
	}
}

// The fast order must be the same order, including which of two equal marks
// comes first: a stable sort is what UAX #15 asks for, and two marks of one
// class that swap places change what is drawn.
func TestCanonicalOrderIsAStableSortByClass(t *testing.T) {
	marks := []rune{0x0301, 0x0316, 0x0323, 0x0302, 0x0327, 0x0334, 0x05B0, 0x0E38}
	for _, text := range []string{
		"", "a", "á", "á̖", "á̖",
		"a" + strings.Repeat("̖̣́̂", 30),
		"a" + strings.Repeat("́", 40),
		strings.Repeat("á̖", 20),
	} {
		sameOrder(t, []rune(text))
	}
	// Past canonInsertionMax in both directions, and randomly, because the
	// threshold is where the two implementations part company.
	r := rand.New(rand.NewSource(1))
	for n := 1; n < 80; n++ {
		for trial := 0; trial < 20; trial++ {
			runes := []rune{'a'}
			for i := 0; i < n; i++ {
				runes = append(runes, marks[r.Intn(len(marks))])
			}
			sameOrder(t, runes)
		}
	}
}

func FuzzCanonicalOrderIsAStableSortByClass(f *testing.F) {
	for _, s := range []string{
		"á̖", "a" + strings.Repeat("̖̣́̂", 20),
		"á", "", "abc", "́́",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 4096 {
			return
		}
		sameOrder(t, []rune(text))
	})
}

// TestCanonicalOrderingIsNotQuadratic is the cost of the ordering on the input
// it is reached for.
//
// A text node is untrusted and one base may carry any number of marks. Written
// as the standard's insertion sort, a run whose classes alternate makes every
// mark walk back over all the ones before it: sixty-four thousand marks written
// 220, 230, 220, 230 took 4.9 seconds and climbed by four per doubling. The same
// marks all of one class took 716 microseconds, because they are already in
// order — so what finds this is a long *unsorted* run, not a long one.
//
// The bound is on the clock rather than on allocation, which is the other way
// round from the tests in paragraph, and for a stated reason: this ordering
// allocates nothing that depends on the order, so allocation is flat across the
// fault and cannot see it. The margin is what makes a clock bound honest here —
// 9ms against a second — where those tests had only a factor of thirty-seven.
func TestCanonicalOrderingIsNotQuadratic(t *testing.T) {
	runes := []rune("a" + strings.Repeat("̖̣́̂", 16000))
	start := time.Now()
	ComposeCanonically(runes)
	if el := time.Since(start); el > time.Second {
		t.Errorf("ordering %d marks took %v; canonical ordering is a stable sort "+
			"and must not be quadratic in one combining sequence", len(runes)-1, el)
	}
}
