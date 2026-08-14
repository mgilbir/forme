package bidi

import "testing"

// The paragraph entry points: rule L1 per line, and the base direction.
//
// Unicode's conformance suites exercise L1 only with the whole text as one
// line, because the files have no notion of line breaking. A layout engine
// applies it per line, and every case below is one the suites cannot state.
//
// These came from pdf0, which had a second implementation of this algorithm
// before this package was exported. The implementation is gone; the tests are
// what it is worth keeping.

// TestLineLevelsResetTrailingWhitespace is rule L1, which is the half of the
// algorithm the conformance suites do not reach.
func TestLineLevelsResetTrailingWhitespace(t *testing.T) {
	// A space between two Hebrew words is right-to-left: N1 gives it the
	// direction both sides agree on, and it stays there inside a line.
	text := []rune{alefHeb, ' ', betHeb}
	line := Resolve(text, LeftToRight).LineLevels(0, 3)
	if line[1] != 1 {
		t.Errorf("the space between two Hebrew words is at level %d, want 1 — L1 "+
			"applies at a line's edges and not inside it (line %v)", line[1], line)
	}

	// A tab is a segment separator, so L1 resets it wherever it falls, and the
	// white space before it with it. Without clause 3 the space would stay
	// right-to-left and sit on the wrong side of the tab stop.
	text = []rune{alefHeb, ' ', '\t', betHeb}
	p := Resolve(text, LeftToRight)
	if p.Levels()[0] != 1 || p.Levels()[3] != 1 {
		t.Fatalf("the letters either side of the tab are at %v, want both at level "+
			"1 — the tab has to sit between two right-to-left letters or its reset "+
			"is the context's doing and not L1's", p.Levels())
	}
	line = p.LineLevels(0, 4)
	if line[2] != 0 {
		t.Errorf("the tab is at level %d, want the paragraph's 0", line[2])
	}
	if line[1] != 0 {
		t.Errorf("the space before the tab is at level %d, want 0 — L1 clause 3 "+
			"resets white space preceding a separator", line[1])
	}
	if line[0] != 1 || line[3] != 1 {
		t.Errorf("L1 moved the letters too: %v", line)
	}
}

// TestLineLevelsAreIndependentPerLine is why the paragraph is kept rather than
// resolved and thrown away.
//
// The same paragraph broken in two places gives the same character different
// levels, because what is at the end of a line depends on where the line ended.
func TestLineLevelsAreIndependentPerLine(t *testing.T) {
	text := []rune{alefHeb, ' ', betHeb, ' ', gimel}
	p := Resolve(text, LeftToRight)

	whole := p.LineLevels(0, 5)
	if whole[1] != 1 || whole[3] != 1 {
		t.Fatalf("as one line, the interior spaces are at %v", whole)
	}
	if first := p.LineLevels(0, 2); first[1] != 0 {
		t.Errorf("as the end of a line, the space is at level %d, want 0", first[1])
	}
}

// TestResolveTakesTheBaseDirectionItIsGiven, and works it out when it is not.
func TestResolveTakesTheBaseDirectionItIsGiven(t *testing.T) {
	hebrew := []rune{alefHeb, betHeb}
	latin := []rune{'a', 'b'}

	if got := Resolve(hebrew, LeftToRight).Level(); got != 0 {
		t.Errorf("Hebrew in a left-to-right paragraph is at base level %d, want 0", got)
	}
	if got := Resolve(latin, RightToLeft).Level(); got != 1 {
		t.Errorf("Latin in a right-to-left paragraph is at base level %d, want 1", got)
	}
	// Auto is P2/P3: the first strong character decides, and nothing strong
	// means left to right.
	if got := Resolve(hebrew, Auto).Level(); got != 1 {
		t.Errorf("Auto over Hebrew gave base level %d, want 1", got)
	}
	if got := Resolve(latin, Auto).Level(); got != 0 {
		t.Errorf("Auto over Latin gave base level %d, want 0", got)
	}
	if got := Resolve([]rune("123 "), Auto).Level(); got != 0 {
		t.Errorf("Auto over text with nothing strong gave base level %d, want 0", got)
	}
}

// TestLineLevelsClampsItsRange, since a caller indexes it with line boundaries
// it worked out itself and an off-by-one there should not be a panic.
func TestLineLevelsClampsItsRange(t *testing.T) {
	p := Resolve([]rune{alefHeb, ' ', betHeb}, LeftToRight)
	if got := p.LineLevels(-5, 99); len(got) != 3 {
		t.Errorf("a range wider than the paragraph gave %d levels, want 3", len(got))
	}
	if got := p.LineLevels(2, 2); got != nil {
		t.Errorf("an empty range gave %v, want nil", got)
	}
	if got := p.LineLevels(3, 1); got != nil {
		t.Errorf("a reversed range gave %v, want nil", got)
	}
}
