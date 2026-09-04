package layout

import "testing"

// CSS Text 3 §7.3's other justification method: the slack between every pair of
// typographic character units rather than at the word spaces.
//
// It is what "inter-character" asks for, and "distribute" is §7.3's older name
// for the same thing. Thai and Chinese are justified this way — and so is a line
// with no space in it at all, which is what makes the difference easy to see:
// "XX" in a box twice its width is one X at each edge, and the word method
// leaves both at the left because there is no space to stretch.
//
// css-text/text-justify/text-justify-inter-character-001 draws that answer with
// a float, and text-justify-distribute-001 asks for the same page from the other
// spelling.

// charStarts is where each run of the first line of #p begins.
func charStarts(t *testing.T, cssSrc, htmlSrc string) []float64 {
	t.Helper()
	root := layoutOf(t, 600, htmlSrc, cssSrc)
	var out []float64
	for _, r := range find(t, root, "p").Lines[0].Runs {
		out = append(out, r.X.Px())
	}
	return out
}

const interCSS = `#p { font-family: Courier; font-size: 20px; width: 200px;
	text-align-last: justify }`

// TestTheSlackGoesBetweenTheCharacters is the bug, in the shape the suite draws
// it: two characters and no space between them.
func TestTheSlackGoesBetweenTheCharacters(t *testing.T) {
	// Courier at 20px is 12px a character, so "XX" is 24 of the 200 available
	// and the 176 left goes into the one gap between them.
	root := layoutOf(t, 600, `<p id="p">XX</p>`,
		interCSS+` #p { text-justify: inter-character }`)
	line := find(t, root, "p").Lines[0]
	if len(line.Runs) != 1 {
		t.Fatalf("%d runs, want one", len(line.Runs))
	}
	r := line.Runs[0]
	if r.X.Px() != 0 {
		t.Errorf("the run starts at %gpx, want 0", r.X.Px())
	}
	if got := r.LetterSpacing.Px(); got != 176 {
		t.Errorf("the extra after each character is %gpx, want 176 — one gap and "+
			"176 to put in it", got)
	}
	// The second character therefore lands at the far edge, 12px of its own
	// width short of it.
	if got := r.X.Px() + 12 + r.LetterSpacing.Px(); got != 188 {
		t.Errorf("the second character is at %gpx, want 188", got)
	}
}

// TestDistributeIsTheSameMethod, which §7.3 says outright.
func TestDistributeIsTheSameMethod(t *testing.T) {
	inter := charStarts(t, interCSS+` #p { text-justify: inter-character }`, `<p id="p">XX</p>`)
	dist := charStarts(t, interCSS+` #p { text-justify: distribute }`, `<p id="p">XX</p>`)
	if len(inter) != len(dist) || (len(inter) > 0 && inter[0] != dist[0]) {
		t.Errorf("inter-character gave %v and distribute gave %v", inter, dist)
	}
}

// TestTheWordMethodLeavesALineWithNoSpaceAlone is the contrast, and the reason
// the two are different methods rather than two names: "auto" has nowhere to put
// the slack on this line, so §7.3 aligns it as start.
func TestTheWordMethodLeavesALineWithNoSpaceAlone(t *testing.T) {
	root := layoutOf(t, 600, `<p id="p">XX</p>`, interCSS)
	r := find(t, root, "p").Lines[0].Runs[0]
	if r.LetterSpacing != 0 || r.X != 0 {
		t.Errorf("the run is at %v with %v of spacing; the word method has no space "+
			"to stretch here", r.X, r.LetterSpacing)
	}
}

// TestTheSlackIsSharedOverEveryPair. n units offer n-1 opportunities, which is
// what "between each pair" means — and getting it wrong by one is the mistake
// that leaves the last character a gap short of the edge.
func TestTheSlackIsSharedOverEveryPair(t *testing.T) {
	// Four characters of 12px are 48 of 200, so 152 goes into three gaps.
	root := layoutOf(t, 600, `<p id="p">XXXX</p>`,
		interCSS+` #p { text-justify: inter-character }`)
	r := find(t, root, "p").Lines[0].Runs[0]
	// A layout unit is a 64th of a pixel, so the third is rounded — what the
	// fixture pins is the *count*, which is what a mistake here gets wrong.
	if got := r.LetterSpacing.Mul(3).Px(); got < 151.9 || got > 152.1 {
		t.Errorf("three of the extra come to %gpx, want 152", got)
	}
	if got := r.LetterSpacing.Mul(4).Px(); got > 152.1 {
		return
	} else {
		t.Errorf("four of the extra come to %gpx, so the slack was shared over "+
			"four gaps where three characters' worth of pairs exist", got)
	}
}

// TestSpacesAreCharactersToo: the method is about *units*, so a line with words
// in it stretches between its letters as well as at its spaces, rather than only
// at the spaces.
func TestSpacesAreCharactersToo(t *testing.T) {
	root := layoutOf(t, 600, `<p id="p">ab cd</p>`,
		interCSS+` #p { text-justify: inter-character }`)
	line := find(t, root, "p").Lines[0]
	for _, r := range line.Runs {
		if r.Text == "ab" && r.LetterSpacing == 0 {
			t.Errorf("the first word got no extra; the slack goes between its " +
				"letters as well as at the space")
		}
	}
}

// TestNoneStillTurnsItOff, so that adding a method cannot quietly turn the one
// value that means "do not" into one that does.
func TestNoneStillTurnsItOff(t *testing.T) {
	root := layoutOf(t, 600, `<p id="p">ab cd</p>`,
		interCSS+` #p { text-justify: none }`)
	for _, r := range find(t, root, "p").Lines[0].Runs {
		if r.LetterSpacing != 0 {
			t.Errorf("a run got %v of extra under \"text-justify: none\"", r.LetterSpacing)
		}
	}
}

// TestAHangingSpaceIsNotAUnit. §4.1.2's white space at the end of a line is not
// on the page, so it is not one of the units the slack is shared between — the
// same exclusion the word method makes, and for the same reason.
//
// "pre-wrap" is what puts it in reach: it keeps the trailing space as a run, and
// a soft wrap hangs it unconditionally.
func TestAHangingSpaceIsNotAUnit(t *testing.T) {
	root := layoutOf(t, 600, `<p id="p">aa bb cc</p>`,
		`#p { font-family: Courier; font-size: 20px; width: 80px; white-space: pre-wrap;
			text-align: justify; text-justify: inter-character }`)
	line := find(t, root, "p").Lines[0]
	if len(line.Runs) < 4 {
		t.Fatalf("%d runs on the first line, want the trailing space among them",
			len(line.Runs))
	}
	// "aa bb" is five units of 12px in 80px, so 20px goes into four gaps. The
	// space hanging past the edge is a sixth run and not a sixth unit.
	if got := line.Runs[0].LetterSpacing.Px(); got != 5 {
		t.Errorf("the extra is %gpx, want 5 — four gaps, not the five the hanging "+
			"space would make", got)
	}
}

// A row of pictures is one typographic character unit here too.
//
// §8.2 says it of letter-spacing — "each consecutive run of atomic inlines
// (such as images and inline blocks) is treated as a single typographic
// character unit" — and it is the same notion this method distributes between:
// two pictures side by side offer one opportunity, the one between the pair and
// whatever is beside it, and not three.
//
// The suite writes it as text-justify-inter-character-atomic-inline, whose
// reference puts the padding on the pictures by hand.
func TestARowOfPicturesIsOneUnitToJustify(t *testing.T) {
	// Courier at 20px is 12px a character and the boxes below are 12px too, so
	// four things of 12 in 200px leave 152 of slack. Three units make two gaps
	// of 76; four would make three of 50⅔.
	const css = interCSS + ` #p { text-justify: inter-character }
		.b { display: inline-block; width: 12px; height: 12px }`
	// Measured from the paragraph's own edge, since the body has a margin.
	at := func(id string, markup string) float64 {
		t.Helper()
		root := layoutOf(t, 600, `<p id="p">`+markup+`</p>`, css)
		return find(t, root, id).BorderRect.X.Px() - find(t, root, "p").BorderRect.X.Px()
	}
	const row = `X<span class="b" id="b1"></span><span class="b" id="b2"></span>X`
	if got := at("b1", row); got != 88 {
		t.Errorf("the first picture starts at %gpx, want 88 — one gap of 76 "+
			"after the X", got)
	}
	if got := at("b2", row); got != 100 {
		t.Errorf("the second picture starts at %gpx, want 100 — hard against the "+
			"first, because the two are one unit", got)
	}
	// A space between them makes two units of what was one, which is what says
	// the rule above is about *consecutive* pictures and not about pictures.
	// Five units of 12 in 200 leave 140 over four gaps of 35, so the first
	// picture sits at 47. Merged into one unit they would be four units and
	// three gaps, and it would sit at 58⅔.
	const spaced = `X<span class="b" id="b1"></span> <span class="b" id="b2"></span>X`
	if got := at("b1", spaced); got != 47 {
		t.Errorf("with a space between them the first picture starts at %gpx, "+
			"want 47 — the space is a unit of its own and ends the run", got)
	}
}
