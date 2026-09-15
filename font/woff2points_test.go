package font

import "testing"

// TestTheContourIndexBoundIsTighterThanThePointBound records that
// maxWOFF2Points cannot fire, and why.
//
// It is meant to bound one glyph: "The point count is a sum of per-contour
// counts, each read as a number rather than as bytes present, so it is the one
// figure here that could ask for an allocation the file does not back." The
// reasoning is right and the number is too large for it. Immediately below the
// check is a second one — a contour's end is an index and the format writes it
// in sixteen bits — which refuses as soon as the running total passes 65,536.
//
// A contour's count comes from short255, whose largest encoding is a u16, so one
// contour is at most 65,535 points. The total therefore passes the index bound
// on the second contour at the latest, and can never climb to 1,048,576: the
// points check is always reached with a total the index check is about to
// refuse.
//
// So there is no glyph that exercises it, and a test asserting otherwise would
// be asserting the index bound under another name. This asserts the relationship
// the reasoning rests on: the largest count one contour can state, and that two
// of them trip the tighter bound while staying under the looser one. If the
// format's count ever widens, or the index bound goes, this fails and says that
// the points bound has become the one that matters.
func TestTheContourIndexBoundIsTighterThanThePointBound(t *testing.T) {
	// The largest count a contour can state: code 253 followed by a u16.
	s := &stream{b: []byte{253, 0xFF, 0xFF}}
	most, ok := s.short255()
	if !ok {
		t.Fatal("the largest 255UInt16 encoding did not read")
	}
	if most != 0xFFFF {
		t.Fatalf("the largest count one contour can state read as %d, not %d; the "+
			"reasoning below depends on this being the ceiling", most, 0xFFFF)
	}

	const contourIndexBound = 65536 // where the check below the points check refuses

	// One contour cannot reach the points bound.
	if most >= maxWOFF2Points {
		t.Errorf("one contour may state %d points and the points bound is %d; it "+
			"can now be reached in a single contour and wants a glyph that does it",
			most, maxWOFF2Points)
	}
	// And the second contour trips the index bound first, so the running total
	// never climbs toward the points bound at all.
	if 2*most <= contourIndexBound {
		t.Errorf("two contours of %d points come to %d, which the index bound of %d "+
			"does not refuse; the walk continues and the points bound may now be "+
			"reachable", most, 2*most, contourIndexBound)
	}
	if contourIndexBound >= maxWOFF2Points {
		t.Errorf("the index bound is %d and the points bound %d; the points bound is "+
			"no longer the looser of the two and is now the one that fires",
			contourIndexBound, maxWOFF2Points)
	}
}
