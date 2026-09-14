package shape_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/forme/fonts/notosans"
)

// TestAttachingALongMarkRunIsNotQuadratic is the cost of mark attachment on the
// input it is reached for.
//
// One base may carry any number of marks, and a text node is untrusted. Three
// separate walks over the run made that quadratic, each of them reading the
// marks already dealt with once more for every new one:
//
//   - the nearest non-mark before a mark was searched for backwards, so the
//     k-th mark walked back over k of them;
//   - the attachments gathered for the run were put in lookup order by an
//     insertion sort;
//   - a mark is moved back over everything between it and its base, and that
//     stretch was summed again for each mark.
//
// Sixteen thousand U+0301 on one "a" took 1.8 seconds and climbed by four per
// doubling; it is 24ms and climbs by two. The first two were fixed and the
// shape was still quadratic, which is why the guard is on the whole path rather
// than on any one of them — any single walk coming back is enough to fail it.
//
// The bound is on the clock rather than on allocation because the fault does
// not allocate: the allocation ratio was flat at 2.0 across all three of these
// while the time ratio was 4.0. The margin is what makes that honest — 24ms
// against two seconds.
func TestAttachingALongMarkRunIsNotQuadratic(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}

	// The shape of the curve rather than a time, because a time cannot be both
	// tight enough to catch this and loose enough for the race detector's job,
	// which runs ten times slower and would need a bound ten times looser than
	// the fault itself. A ratio is the same on any machine: four times the input
	// is four times the work when this is linear and sixteen when it is not.
	//
	// Measured as the best of three, because the clock is noisy upward only.
	shapeOf := func(marks int) time.Duration {
		text := "a" + strings.Repeat("\u0301", marks)
		best := time.Duration(1 << 62)
		for i := 0; i < 3; i++ {
			start := time.Now()
			glyphs, _ := face.ShapeGlyphs(text)
			if el := time.Since(start); el < best {
				best = el
			}
			if len(glyphs) < marks/2 {
				t.Fatalf("shaping %d marks gave %d glyphs; the fixture is not "+
					"reaching mark attachment", marks, len(glyphs))
			}
		}
		return best
	}

	const small, large = 8000, 32000
	lo, hi := shapeOf(small), shapeOf(large)
	if lo <= 0 {
		t.Fatalf("%d marks measured as %v; there is nothing to compare", small, lo)
	}
	// Four times the input. Linear is 4, quadratic is 16, and 8 is between them
	// with room on both sides rather than against either.
	if ratio := float64(hi) / float64(lo); ratio > 8 {
		t.Errorf("shaping %d marks took %v and %d took %v, a factor of %.1f for four "+
			"times the input; attaching a mark must not read the marks before it",
			small, lo, large, hi, ratio)
	}
}
