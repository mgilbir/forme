package layout

import "testing"

// text-align-all, CSS Text 4 §7.1, and text-align as the shorthand for it.
//
// The split matters for two things and neither is spelling. The first is the
// cascade: two ways of saying the same thing have to set the same longhand, or
// the later declaration does not win. The second is the last line, which
// text-align sets as well as the rest — differently for two of its values, and
// that is where the suite pushes.
//
// Positions are arithmetic rather than recorded numbers, as in textalign_test.go
// beside this: Courier is 600/1000, so "abcdef" at 20px is 72px, and in a 150px
// line left is 0, right is 78 and centre is 39. Two lines are needed wherever the
// last line's own alignment is in question, so the fixtures below say "abcdef
// abcdef" in a width that holds one of them.

const alignAllCSS = `#p { font-family: Courier; font-size: 20px; width: 150px }`

// lineXs is every line's first run, which is what a test about the last line
// needs and lineX cannot give.
func lineXs(t *testing.T, root *Fragment, id string) []float64 {
	t.Helper()
	f := find(t, root, id)
	var out []float64
	for _, ln := range f.Lines {
		if len(ln.Runs) == 0 {
			t.Fatalf("#%s has a line with no runs", id)
		}
		out = append(out, ln.Runs[0].X.Px())
	}
	return out
}

// TestTextAlignAndTextAlignAllCascadeAgainstEachOther is the reason for the
// split, stated as the thing that would be wrong without it.
//
// Two declarations of the same property are decided by order. If layout read
// "text-align" in one place and "text-align-all" in another, the two would not
// be the same property and the answer would come from which of the two readers
// ran — so the *first* declaration would win in one direction and the second in
// the other, whichever way the code happened to be written.
func TestTextAlignAndTextAlignAllCascadeAgainstEachOther(t *testing.T) {
	for _, tc := range []struct {
		css  string
		want float64
		why  string
	}{
		{`#p { text-align: center; text-align-all: right }`, 78,
			"text-align-all is later, so it wins"},
		{`#p { text-align-all: right; text-align: center }`, 39,
			"text-align is later, so it wins"},
	} {
		root := layoutOf(t, 600, `<div id="p">abcdef</div>`, alignAllCSS+tc.css)
		if got := lineX(t, root, "p"); got != tc.want {
			t.Errorf("%s: the line is at %gpx, want %g (%s)", tc.css, got, tc.want, tc.why)
		}
	}
}

// TestTextAlignLeavesTheLastLineAlone is the ordinary row of §7.1's table: any
// value but two sets text-align-last back to auto, so the last line follows the
// rest and an earlier text-align-last does not survive.
func TestTextAlignLeavesTheLastLineAlone(t *testing.T) {
	root := layoutOf(t, 600, `<div id="p">abcdef abcdef</div>`,
		alignAllCSS+`#p { text-align-last: right; text-align: center }`)
	got := lineXs(t, root, "p")
	if len(got) != 2 || got[0] != 39 || got[1] != 39 {
		t.Errorf("lines at %v, want [39 39]: text-align: center resets "+
			"text-align-last to auto, so the last line is centred too", got)
	}
	// And the other order, where the last line keeps what it was given.
	root = layoutOf(t, 600, `<div id="p">abcdef abcdef</div>`,
		alignAllCSS+`#p { text-align: center; text-align-last: right }`)
	got = lineXs(t, root, "p")
	if len(got) != 2 || got[0] != 39 || got[1] != 78 {
		t.Errorf("lines at %v, want [39 78]", got)
	}
}

// TestJustifyAllJustifiesTheLastLineToo is §7.1's second row, and the whole
// reason that value has a name of its own.
//
// A justified paragraph leaves its last line short, because stretching a handful
// of words across the measure looks like damage. "justify-all" is how an author
// asks for it anyway, and it is now the shorthand setting both longhands rather
// than a keyword a second reader had to know about.
func TestJustifyAllJustifiesTheLastLineToo(t *testing.T) {
	if !lastLineIsStretched(t, `#p { text-align: justify-all }`) {
		t.Errorf("justify-all left the last line short")
	}
	if lastLineIsStretched(t, `#p { text-align: justify }`) {
		t.Errorf("justify stretched the last line")
	}
	// And it is the shorthand doing it: the same thing written as the two
	// longhands has to give the same page.
	if !lastLineIsStretched(t, `#p { text-align-all: justify; text-align-last: justify }`) {
		t.Errorf("the two longhands did not do what justify-all does")
	}
}

// lastLineIsStretched reports whether the last line's final run reaches the end
// of the measure, which is what justification does and what leaving the line
// short does not.
//
// "abc abc abc abc abc" in 150px of Courier at 20px is two lines: three words
// and a pair of spaces come to 132px and the fourth does not fit, so the last
// line holds two words of 36px with a space between them — 84px, and 66px of
// slack to stretch into or to leave alone. Two words and not one, because
// justification spreads the spaces *between* words and a line with one word has
// none: such a line cannot be stretched by any value and would report "short"
// for both of them.
func lastLineIsStretched(t *testing.T, css string) bool {
	t.Helper()
	root := layoutOf(t, 600, `<div id="p">abc abc abc abc abc</div>`, alignAllCSS+css)
	f := find(t, root, "p")
	if len(f.Lines) != 2 {
		t.Fatalf("the fixture made %d lines, want 2", len(f.Lines))
	}
	last := f.Lines[1]
	end := last.Runs[len(last.Runs)-1]
	return end.X.Add(end.Width).Px() > 149
}

// TestMatchParentReachesTheLastLineThroughAnOverriddenTextAlignAll is §7.1's
// third row, and the one a table alone would get wrong.
//
// "text-align: match-parent" cannot set text-align-last to auto, because auto
// means "follow text-align-all" and text-align-all is a property an author may
// set separately on the same element. Doing so would then take the match off the
// last line, which is not what was asked for. So the shorthand sets both to
// match-parent, and the last line stays matched.
//
// This is writing-system-independent arithmetic of the suite's own fixture,
// text-align-match-parent-05: a right-to-left section holding a left-to-right
// div whose text-align is match-parent and whose text-align-all is left. Every
// line but the last is flush left; the last is flush right, because the parent's
// start is right.
func TestMatchParentReachesTheLastLineThroughAnOverriddenTextAlignAll(t *testing.T) {
	const src = `<div id="outer" dir="rtl"><div id="p" dir="ltr">abcdef abcdef</div></div>`
	root := layoutOf(t, 600, src, alignAllCSS+
		`#outer { width: 150px }
		 #p { text-align: match-parent; text-align-all: left }`)
	got := lineXs(t, root, "p")
	if len(got) != 2 || got[0] != 0 || got[1] != 78 {
		t.Errorf("lines at %v, want [0 78]: text-align-all was overridden to "+
			"left, and the last line still matches the parent, whose start is "+
			"the right edge", got)
	}
	// Without the override both lines match the parent, which is what says the
	// first line above moved because of text-align-all and not by accident.
	root = layoutOf(t, 600, src, alignAllCSS+
		`#outer { width: 150px }
		 #p { text-align: match-parent }`)
	if got := lineXs(t, root, "p"); len(got) != 2 || got[0] != 78 || got[1] != 78 {
		t.Errorf("without the override the lines are at %v, want [78 78]", got)
	}
}

// TestTextAlignLastMatchParentIsTheParentsAlignment is the value read on the
// longhand an author can write directly, which is the same walk.
func TestTextAlignLastMatchParentIsTheParentsAlignment(t *testing.T) {
	for _, tc := range []struct {
		outer string
		want  float64
	}{
		{"text-align: center", 39},
		{"text-align: right", 78},
		{"direction: rtl", 78},
		{"direction: ltr", 0},
	} {
		root := layoutOf(t, 600,
			`<div id="outer"><div id="p">abcdef abcdef</div></div>`,
			alignAllCSS+`#outer { width: 150px; `+tc.outer+` }
			 #p { text-align-all: left; text-align-last: match-parent }`)
		got := lineXs(t, root, "p")
		if len(got) != 2 || got[0] != 0 || got[1] != tc.want {
			t.Errorf("under %q the lines are at %v, want [0 %g]", tc.outer, got, tc.want)
		}
	}
}

// TestMatchParentOnTheLastLineOfTheRootStaysLogical is the case that runs off
// the top of the tree, and it is the same one text-align has: the specification
// resolves match-parent against the parent's value, the root has no parent, and
// there is nothing there to make "start" physical against.
//
// What it must fall back to is the direction the *line* was set in, not the
// direction of whatever box the walk ended on. The fixture is
// text-align-match-parent-root-logical's: a right-to-left root holding a
// left-to-right block, with match-parent all the way up. The line is
// left-to-right and must come out flush left; resolving the root's own direction
// there answers right, which is the reading this is built to catch.
func TestMatchParentOnTheLastLineOfTheRootStaysLogical(t *testing.T) {
	const css = `html { direction: rtl; text-align-all: match-parent }
		 #p { direction: ltr; text-align-last: match-parent }`
	got := lineXs(t, layoutOf(t, 600,
		`<div id="p">abcdef abcdef</div>`, alignAllCSS+css), "p")
	if len(got) != 2 || got[1] != 0 {
		t.Errorf("the lines are at %v, want the last at 0: the walk ran off the "+
			"top with nothing to make start physical against, so it stays the "+
			"direction the line was set in", got)
	}
	// The same document with the block right-to-left, which is what says the
	// answer above came from the line's direction rather than from a constant.
	got = lineXs(t, layoutOf(t, 600, `<div id="p">abcdef abcdef</div>`,
		alignAllCSS+`html { direction: rtl; text-align-all: match-parent }
		 #p { direction: rtl; text-align-last: match-parent }`), "p")
	if len(got) != 2 || got[1] != 78 {
		t.Errorf("with a right-to-left block the lines are at %v, want the last "+
			"at 78", got)
	}
}
