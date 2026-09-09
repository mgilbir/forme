package bidi

import "testing"

// Rule L1 at the end of a line, and the characters rule X9 removed.
//
// §5.2 says two things about such a character and they are not in conflict: it
// takes the level of the character before it, "so that the character does not
// interrupt a run", and it joins a sequence of white space that L1 resets to the
// paragraph's direction. A PDF in the middle of a run of spaces is part of that
// run; a PDF with a letter in front of it is part of the letter.
//
// Resetting one on sight made LineLevels contradict Levels, which the note on
// Levels says cannot happen: Resolve applies L1 with the whole paragraph taken
// as one line, so asking LineLevels for that same line has to give the same
// answer. What a caller sees when they disagree is one extra run, of a character
// that marks no paper, in the middle of a word.

// levelsOf resolves a string and returns both readings of it: the paragraph's
// own levels, and the levels of the line that is the whole paragraph.
func levelsOf(t *testing.T, s string, dir Direction) (para, line []int) {
	t.Helper()
	text := []rune(s)
	p := Resolve(text, dir)
	return p.Levels(), p.LineLevels(0, len(text))
}

// TestATrailingRemovedCharacterKeepsTheLevelBeforeIt.
//
// "a<LRO>b<RLO>c<PDF>": the PDF ends the paragraph with a strong character in
// front of it and no white space anywhere, so there is no sequence for L1 to
// reset and the PDF is part of the c beside it.
func TestATrailingRemovedCharacterKeepsTheLevelBeforeIt(t *testing.T) {
	const s = "a‭b‮c‬" // LRO, RLO, PDF

	para, line := levelsOf(t, s, LeftToRight)
	if len(para) != len([]rune(s)) {
		t.Fatalf("%d levels for %d characters", len(para), len([]rune(s)))
	}
	if para[5] == 0 {
		t.Fatalf("the trailing PDF is at the paragraph level already, so this "+
			"fixture cannot tell the two readings apart: %v", para)
	}
	if line[5] != para[5] {
		t.Errorf("the trailing PDF is level %d in the paragraph and %d on the "+
			"line that is the whole paragraph; there is no white space in this "+
			"string for L1 to reset, and §5.2 gives the character the level of "+
			"the one before it", para[5], line[5])
	}
}

// TestARemovedCharacterAmongTrailingSpacesIsResetWithThem is the other half, and
// it is what stops the rule above from being "never reset one".
//
// The line has to end *before* the paragraph does, which is the only place this
// is visible: at the end of the paragraph Resolve has already reset the spaces
// and fillRemoved has already carried that reset into the character among them,
// so resetting it again changes nothing. A shorter line is the case LineLevels
// exists for — the spaces are at the end of *it* and nothing has reset them yet.
func TestARemovedCharacterAmongTrailingSpacesIsResetWithThem(t *testing.T) {
	// Hebrew, a space, a PDF, then more Hebrew, in a left-to-right paragraph.
	// The line ends after the PDF, so the space and the PDF are its trailing
	// run — and both sit between right-to-left words, so they are level 1 until
	// L1 resets them to the paragraph's 0.
	const s = "אב ‬אב"
	const line = 4 // alef, bet, space, PDF

	p := Resolve([]rune(s), LeftToRight)
	got := p.LineLevels(0, line)
	if len(got) != line {
		t.Fatalf("%d levels for a line of %d characters", len(got), line)
	}
	if p.Levels()[3] == p.Level() {
		t.Fatalf("the PDF is at the paragraph level in the paragraph already, "+
			"so a reset on the line cannot be told from no reset: %v", p.Levels())
	}
	for i := 2; i < line; i++ {
		if got[i] != p.Level() {
			t.Errorf("character %d of the line's trailing run is level %d, want "+
				"the paragraph's %d: a removed character among the spaces at the "+
				"end of a line is part of the run they make", i, got[i], p.Level())
		}
	}
}

// TestARemovedCharacterBeforeTrailingSpacesKeepsItsOwn is the third case, and
// the one that shows the scan holds a removed character rather than resetting or
// stopping at it: the PDF here is between a letter and the spaces, so it belongs
// to the letter.
func TestARemovedCharacterBeforeTrailingSpacesKeepsItsOwn(t *testing.T) {
	// Right-to-left text so the letters are level 1, then a PDF, then spaces.
	const s = "אב‬  "

	para, line := levelsOf(t, s, LeftToRight)
	if para[2] == 0 {
		t.Fatalf("the PDF is at the paragraph level already, so this fixture "+
			"tells the two readings apart from nothing: %v", para)
	}
	if line[2] != para[2] {
		t.Errorf("the PDF between a letter and the trailing spaces is level %d "+
			"on the line and %d in the paragraph; it is the letter's, because "+
			"the letter is what precedes it", line[2], para[2])
	}
	for i := 3; i < len([]rune(s)); i++ {
		if line[i] != 0 {
			t.Errorf("trailing space %d is level %d, want the paragraph's 0", i, line[i])
		}
	}
}
