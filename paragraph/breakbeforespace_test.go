package paragraph

import "testing"

// No line break before white space, when the opportunity came from the
// character before it.
//
// UAX #14 says it twice for two reasons. LB7 is "× SP" and "× ZW": a line may
// not end between a word and the space after it, so the space stays on its
// word's line and the break falls after it. LB21 is "× BA", and most of the
// other space separators are class BA — the ogham space mark among them, which
// is a *visible* stemline and belongs to the word it follows.
//
// The rule was already applied to the opportunities word-break: break-all adds,
// and the comment there names LB7 as the reason. It was not applied to the one
// an ideograph defers to the character after it, and that is the case the suite
// tests: "ああ" then an ogham space mark, in a box two ems wide under
// white-space: break-spaces. A break between the second ideograph and the
// stemline is one UAX #14 forbids, and taking it fitted both ideographs on a
// line the specification puts one on.

// opportunities returns the index of every piece that may begin a line.
func opportunities(t *testing.T, text string, ws WhiteSpace) []int {
	t.Helper()
	pieces, _ := SplitAtBreaks(text, ws, WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
	var out []int
	for i, p := range pieces {
		if p.BreakBefore {
			out = append(out, i)
		}
	}
	return out
}

// TestNoBreakBeforeASpaceSeparatorAfterAnIdeograph is the bug.
func TestNoBreakBeforeASpaceSeparatorAfterAnIdeograph(t *testing.T) {
	ws := WhiteSpace{PreserveBreaks: true, Wrap: true, BreakSpaces: true}
	// Two ideographs and an ogham space mark. The break between the ideographs
	// stands; the one before the stemline does not.
	pieces, _ := SplitAtBreaks("ああ\u1680", ws, WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
	for i, p := range pieces {
		if p.Space && p.BreakBefore {
			t.Errorf("piece %d (%q) may begin a line, and it is white space that "+
				"follows an ideograph; UAX #14 forbids a break before it", i, p.Text)
		}
	}
	// The control: an ideograph followed by another ideograph still offers the
	// opportunity, so this is about what follows and not about ideographs.
	if got := opportunities(t, "あああ", ws); len(got) == 0 {
		t.Errorf("a run of ideographs offers no break at all")
	}
}

// TestTheSameHoldsForAnOrdinarySpace, which is LB7 rather than LB21 and is the
// commoner half by far: a space after an ideograph belongs to its line.
func TestTheSameHoldsForAnOrdinarySpace(t *testing.T) {
	for _, ws := range []WhiteSpace{
		{Collapse: true, Wrap: true},
		{PreserveBreaks: true, Wrap: true},
		{PreserveBreaks: true, Wrap: true, BreakSpaces: true},
	} {
		pieces, _ := SplitAtBreaks("あ あ", ws, WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
		for i, p := range pieces {
			if p.Space && p.BreakBefore {
				t.Errorf("%+v: piece %d (%q) may begin a line", ws, i, p.Text)
			}
		}
	}
}

// TestABreakIsStillOfferedAfterTheSpace is the containment case: suppressing the
// opportunity before white space must not lose the one after it, or an
// ideograph followed by a space would join the two lines into one.
func TestABreakIsStillOfferedAfterTheSpace(t *testing.T) {
	ws := WhiteSpace{PreserveBreaks: true, Wrap: true, BreakSpaces: true}
	pieces, _ := SplitAtBreaks("あ\u1680あ", ws, WordBreak{}, LineBreak{}, Hyphens{}, WritingSystemOther)
	found := false
	for i, p := range pieces {
		if i > 0 && p.BreakBefore && !pieces[i-1].BreakBefore && pieces[i-1].Space {
			found = true
		}
	}
	if !found {
		t.Errorf("nothing may begin a line after the separator; the opportunity "+
			"moves to the far side of it rather than being lost: %v", pieces)
	}
}

// TestLineBreakAnywhereStillBreaksBeforeSpace. §5.3 puts an opportunity around
// *every* typographic character unit, and says so in those words — so it is the
// one value that is not subject to the rule above, and the code path it takes
// is deliberately separate.
func TestLineBreakAnywhereStillBreaksBeforeSpace(t *testing.T) {
	ws := WhiteSpace{PreserveBreaks: true, Wrap: true, BreakSpaces: true}
	pieces, _ := SplitAtBreaks("ああ\u1680", ws, WordBreak{}, LineBreak{Anywhere: true}, Hyphens{}, WritingSystemOther)
	if len(pieces) == 0 {
		t.Fatal("no pieces")
	}
	any := false
	for _, p := range pieces {
		if p.Space && p.BreakBefore {
			any = true
		}
	}
	if !any {
		t.Errorf("line-break: anywhere offered no opportunity before the separator; "+
			"§5.3 puts one around every unit: %v", pieces)
	}
}
