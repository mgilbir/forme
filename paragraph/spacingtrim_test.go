package paragraph

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// TestSpacingTrimOfReadsTheValue.
func TestSpacingTrimOfReadsTheValue(t *testing.T) {
	for _, tc := range []struct {
		value     string
		trims     bool
		unhandled string
		what      string
	}{
		{"normal", true, "", "the initial value trims at the end of a line"},
		{"space-all", false, "", "space-all keeps every full-width form"},
		{"space-first", true, "space-first",
			"space-first is normal at the end of a line and differs at the start"},
		{"trim-start", true, "trim-start", "and so is trim-start"},
		{"NORMAL", true, "", "values are matched case-insensitively"},
		{"  space-all  ", false, "", "and with the space around them ignored"},
		{"wibble", true, "",
			"an invalid value is dropped by the cascade, so this is asked as the initial one"},
		{"", true, "", "and so is an empty one"},
	} {
		got, unhandled := SpacingTrimOf(tc.value)
		if got.TrimClosingAtEnd != tc.trims || unhandled != tc.unhandled {
			t.Errorf("%s: %q gave trims=%v unhandled=%q, want %v and %q",
				tc.what, tc.value, got.TrimClosingAtEnd, unhandled,
				tc.trims, tc.unhandled)
		}
	}
}

// TestWhichCharactersAreTrimmedAtTheEndOfALine.
//
// Pe and nothing else. The *full-width* half of §8.2's class is not tested here
// at all — the face states which of its glyphs have a blank half to give up, and
// a character the face says nothing about is trimmed by nothing however this
// answers.
func TestWhichCharactersAreTrimmedAtTheEndOfALine(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want bool
		what string
	}{
		{'）', true, "a full-width closing parenthesis"},
		{'」', true, "a full-width right corner bracket"},
		{')', true, "an ASCII one, which the face will decline"},
		{'（', false, "an opening bracket, which is the other end of the line"},
		{'国', false, "an ideograph"},
		{'。', false, "a full stop, which is §8.4's business and not this one"},
		{'”', false, "a closing quote, which is Pf and out of this set"},
	} {
		if got := TrimsAsClosingPunctuation(tc.r); got != tc.want {
			t.Errorf("%s (U+%04X): %v, want %v", tc.what, tc.r, got, tc.want)
		}
	}
	if got := TrailingClosingPunctuation("国国）"); got != len("）") {
		t.Errorf("the trailing bracket of %q is %d bytes, want %d", "国国）", got, len("）"))
	}
	if got := TrailingClosingPunctuation("国）国"); got != 0 {
		t.Errorf("%q ends in an ideograph and nothing is trimmed; got %d", "国）国", got)
	}
}

// TestATrimIsTakenOnlyByALineThatNeedsIt is §8.2's own clause: the character is
// trimmed "if it does not fit on the line before justification".
//
// Stated over items because it is a rule about a *line*, and the discriminating
// pair is one width apart: the same items in a box that holds them whole, and in
// a box one trim narrower. Courier at 20px advances 12px a character, and the
// trim below is one of them.
func TestATrimIsTakenOnlyByALineThatNeedsIt(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	items := func() []Item {
		out := words(t, br, face, "aaa b")
		last := &out[len(out)-1]
		last.TrimEnd = u(12)
		return out
	}

	// "aaa b" is five characters: 60px. The trim is worth 12.
	if got := breakAll(t, br, items(), 60); len(got) != 1 || got[0] != "aaa b" {
		t.Errorf("in 60px the line holds all of it untrimmed and came out %q", got)
	}
	// One pixel short of holding it whole, and the trim is exactly what closes
	// the gap: the line keeps the character rather than sending it down.
	if got := breakAll(t, br, items(), 59); len(got) != 1 || got[0] != "aaa b" {
		t.Errorf("in 59px the trim is what lets the line hold %q; it came out %q",
			"aaa b", got)
	}
	// And a line too short for even the trimmed form still breaks.
	if got := breakAll(t, br, items(), 47); len(got) != 2 {
		t.Errorf("in 47px not even the trimmed character fits; got %q", got)
	}
}

// TestATrimIsNotTakenByALineThatDoesNotNeedIt, which is the same rule from the
// other side and is what a plain "always trim" would fail.
//
// The measure is the line's own: a trimmed character is narrower, so a line that
// took a trim it did not need would report less used width than the characters
// on it come to.
func TestATrimIsNotTakenByALineThatDoesNotNeedIt(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	out := words(t, br, face, "aaa b")
	last := &out[len(out)-1]
	last.TrimEnd = u(12)

	line, _, _, _, _, _ := br.BreakOneLine(out, 0, 0, u(600), 0)
	var used style.Unit
	for _, it := range line {
		used = used.Add(it.Width)
	}
	if want := u(60); used != want {
		t.Errorf("the line measures %v in 600px of room, want %v — there was room "+
			"for the character whole, so §8.2 leaves it whole", used, want)
	}
}
