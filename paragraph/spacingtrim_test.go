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
		start     OpeningTrim
		unhandled string
		what      string
	}{
		{"normal", true, OpeningTrimNone, "", "the initial value trims at the end of a line"},
		{"space-all", false, OpeningTrimNone, "", "space-all keeps every full-width form"},
		{"space-first", true, OpeningTrimAfterSoftWrap, "",
			"space-first trims at the start of a line a soft wrap began"},
		{"trim-start", true, OpeningTrimEveryLine, "", "trim-start at the start of every line"},
		{"trim-both", true, OpeningTrimEveryLine, "trim-both",
			"trim-both at the start of every line, and its end clause is named"},
		{"trim-all", true, OpeningTrimNone, "trim-all", "trim-all is named and set as normal"},
		{"auto", true, OpeningTrimNone, "", "auto is the user agent's choice, and it chooses normal"},
		{"NORMAL", true, OpeningTrimNone, "", "values are matched case-insensitively"},
		{"  space-all  ", false, OpeningTrimNone, "", "and with the space around them ignored"},
		{"wibble", true, OpeningTrimNone, "",
			"an invalid value is dropped by the cascade, so this is asked as the initial one"},
		{"", true, OpeningTrimNone, "", "and so is an empty one"},
	} {
		got, unhandled := SpacingTrimOf(tc.value)
		if got.TrimClosingAtEnd != tc.trims || got.TrimOpeningAtStart != tc.start ||
			unhandled != tc.unhandled {
			t.Errorf("%s: %q gave trims=%v start=%v unhandled=%q, want %v, %v and %q",
				tc.what, tc.value, got.TrimClosingAtEnd, got.TrimOpeningAtStart, unhandled,
				tc.trims, tc.start, tc.unhandled)
		}
	}
}

// TestWhichLineStartsAreTrimmed is OpeningTrim.Trims over the three kinds of
// line start, which is the whole of what separates space-first from
// trim-start.
func TestWhichLineStartsAreTrimmed(t *testing.T) {
	for _, tc := range []struct {
		o                   OpeningTrim
		first, forced, soft bool
	}{
		{OpeningTrimNone, false, false, false},
		{OpeningTrimEveryLine, true, true, true},
		{OpeningTrimAfterSoftWrap, false, false, true},
	} {
		if got := tc.o.Trims(true, false); got != tc.first {
			t.Errorf("%v on the first line: %v, want %v", tc.o, got, tc.first)
		}
		if got := tc.o.Trims(false, true); got != tc.forced {
			t.Errorf("%v after a forced break: %v, want %v", tc.o, got, tc.forced)
		}
		if got := tc.o.Trims(false, false); got != tc.soft {
			t.Errorf("%v after a soft wrap: %v, want %v", tc.o, got, tc.soft)
		}
	}
}

// TestWhichCharactersAreTrimmedAtTheStartOfALine is §8.2's fullwidth
// opening punctuation: Ps in the CJK Symbols and Punctuation block or of East
// Asian Width F, and the two opening quotation marks.
func TestWhichCharactersAreTrimmedAtTheStartOfALine(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want bool
		what string
	}{
		{'（', true, "a fullwidth left parenthesis, East Asian Width F"},
		{'「', true, "a left corner bracket, in the CJK block"},
		{'〔', true, "a left tortoise shell bracket, in the CJK block"},
		{'“', true, "a left double quotation mark, named by the class"},
		{'‘', true, "a left single quotation mark, named by the class"},
		{'(', false, "an ASCII parenthesis, which is Ps and neither F nor in the block"},
		{'［', true, "a fullwidth left square bracket"},
		{'）', false, "a closing bracket, which is the other end of the line"},
		{'”', false, "a closing quotation mark"},
		{'国', false, "an ideograph"},
		{'〝', true, "a reversed double prime quotation mark, Ps in the block"},
	} {
		if got := TrimsAsOpeningPunctuation(tc.r); got != tc.want {
			t.Errorf("%s (U+%04X): %v, want %v", tc.what, tc.r, got, tc.want)
		}
	}
	if got := LeadingOpeningPunctuation("（国国"); got != len("（") {
		t.Errorf("the leading bracket of %q is %d bytes, want %d", "（国国", got, len("（"))
	}
	if got := LeadingOpeningPunctuation("国（国"); got != 0 {
		t.Errorf("%q begins with an ideograph and nothing is trimmed; got %d", "国（国", got)
	}
	if got := LeadingOpeningPunctuation(""); got != 0 {
		t.Errorf("an empty run has nothing to trim; got %d", got)
	}
}

// TestAnOpeningBracketIsTrimmedWhereItBeginsALine is the fill's half of §8.2's
// line-start clause, over items built directly: "(aa (aa (aa" in Courier at
// 20px, where every character is 12px and the brackets carry a 6px trim.
//
// Courier's "(" is not a fullwidth bracket, and that is not what is under
// test: the fill takes a trim an item carries, and which items carry one is
// the layout package's question. What is under test is which lines take it —
// only the one that begins with the bracket, on the lines the value names —
// and that the line is narrower by exactly the trim when it does.
func TestAnOpeningBracketIsTrimmedWhereItBeginsALine(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	build := func(on OpeningTrim, forced bool) []Item {
		var out []Item
		for i := 0; i < 3; i++ {
			if i > 0 {
				if forced && i == 2 {
					out = append(out, Item{Face: face, Size: u(size20), Forced: true})
				} else {
					out = append(out, Item{
						Text: " ", Face: face, Size: u(size20), Space: true, Collapsible: true,
						Width: br.MeasureSpaced(face, " ", u(size20), TextSpacing{}),
					})
				}
			}
			out = append(out, Item{
				Text: "(", Face: face, Size: u(size20), BreakBefore: i > 0,
				Width: u(12), TrimStart: u(6), TrimStartOn: on,
			}, Item{
				Text: "aa", Face: face, Size: u(size20),
				Width: br.MeasureSpaced(face, "aa", u(size20), TextSpacing{}),
			})
		}
		return out
	}
	// lines breaks into lines of 30px, one "(aa" each: 36px whole and 30
	// trimmed, so a line that does not take the trim does not hold its own
	// bracket group and overflows. What each line measures says which did.
	lines := func(items []Item, width float64) []style.Unit {
		var out []style.Unit
		i, iByte := 0, 0
		for n := 0; i < len(items) && n < 10; n++ {
			line, next, nextByte, _, _, _ := br.BreakOneLine(items, i, iByte, u(width), 0)
			var used style.Unit
			for _, it := range line {
				if !it.Collapsible && !it.Forced {
					used = used.Add(it.Width)
				}
			}
			out = append(out, used)
			i, iByte = next, nextByte
		}
		return out
	}
	for _, tc := range []struct {
		what   string
		on     OpeningTrim
		forced bool
		want   []float64
	}{
		{"trim-start trims every line", OpeningTrimEveryLine, false, []float64{30, 30, 30}},
		{"and every line after a forced break", OpeningTrimEveryLine, true, []float64{30, 30, 30}},
		{"space-first spares the first line", OpeningTrimAfterSoftWrap, false, []float64{36, 30, 30}},
		{"and the line after a forced break", OpeningTrimAfterSoftWrap, true, []float64{36, 30, 36}},
		{"normal trims none", OpeningTrimNone, false, []float64{36, 36, 36}},
	} {
		got := lines(build(tc.on, tc.forced), 30)
		var want []style.Unit
		for _, w := range tc.want {
			want = append(want, u(w))
		}
		if len(got) != len(want) {
			t.Errorf("%s: %d lines %v, want %v", tc.what, len(got), got, want)
			continue
		}
		for k := range got {
			if got[k] != want[k] {
				t.Errorf("%s: line %d measures %v, want %v", tc.what, k+1, got[k], want[k])
			}
		}
	}
}

// TestAnOpeningBracketInsideALineIsNotTrimmed: "a (aa" on one line has the
// bracket after a word, and §8.2 trims it only at the start of a line. A fill
// that trimmed every candidate would measure the line 6px short.
func TestAnOpeningBracketInsideALineIsNotTrimmed(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	items := words(t, br, face, "a (aa")
	// words gives "(aa" as one item; the bracket is cut out as layout cuts it.
	head, tail := br.SplitItem(items[2], 1)
	head.TrimStart, head.TrimStartOn = u(6), OpeningTrimEveryLine
	items = append(items[:2], head, tail)

	line, _, _, _, _, _ := br.BreakOneLine(items, 0, 0, u(600), 0)
	var used style.Unit
	for _, it := range line {
		used = used.Add(it.Width)
		if it.StartTrimmed {
			t.Errorf("%q was set trimmed in the middle of a line", it.Text)
		}
	}
	if want := u(60); used != want {
		t.Errorf("the line measures %v, want %v — the bracket is not at its start", used, want)
	}
}

// TestASplitKeepsATrimOnTheEndThatHasTheCharacter: a cut at an item's edge
// leaves one half empty, and a trim copied onto the empty half would be a trim
// of a character that is not there.
func TestASplitKeepsATrimOnTheEndThatHasTheCharacter(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	item := Item{Text: "(", Face: face, Size: u(size20), Width: u(12),
		TrimStart: u(6), TrimEnd: u(3)}
	head, tail := br.SplitItem(item, 0)
	if head.TrimStart != 0 || tail.TrimStart != u(6) {
		t.Errorf("cut at its start: head trims %v and tail %v at the start, want 0 and 6px",
			head.TrimStart, tail.TrimStart)
	}
	if head.TrimEnd != 0 || tail.TrimEnd != u(3) {
		t.Errorf("cut at its start: head trims %v and tail %v at the end, want 0 and 3px",
			head.TrimEnd, tail.TrimEnd)
	}
	head, tail = br.SplitItem(item, len(item.Text))
	if head.TrimEnd != u(3) || tail.TrimEnd != 0 {
		t.Errorf("cut at its end: head trims %v and the empty tail %v at the end, "+
			"want 3px and 0", head.TrimEnd, tail.TrimEnd)
	}
	if head.TrimStart != u(6) {
		t.Errorf("cut at its end: the head, which is the whole of it, lost its start trim")
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

// TestALineThatTookTheTrimIsNarrowerForIt, which is the third corner of the
// same rule and the one the other two leave open.
//
// The test above it checks what a line that needs the trim *holds*, and the one
// below it checks what a line that does not need it *measures*. Neither checks
// what a line that took the trim measures — so a fill that decided to trim and
// then kept the character's full width satisfies both: the line reads "aaa b"
// either way, and the width that is wrong is never looked at.
//
// It matters because the trimmed advance is what reaches the display list. §8.2
// narrows the character to its half-width form, so the line is narrower by
// exactly the trim; a line that reported the untrimmed width would centre and
// right-align its own text a half em off, and would claim room it did not use.
//
// Found by planting: taking the narrowing out of breakOneLine — the whole of
// what the trim does to the width — left every test in this package, every
// other test in the engine, and all 6253 reftest documents passing.
func TestALineThatTookTheTrimIsNarrowerForIt(t *testing.T) {
	br := NewBreaker(nil)
	face := courier(t)
	out := words(t, br, face, "aaa b")
	last := &out[len(out)-1]
	last.TrimEnd = u(12)

	// 59px: one short of holding the character whole, which is the width that
	// makes the fill take the trim. See the test above.
	line, _, _, _, _, _ := br.BreakOneLine(out, 0, 0, u(59), 0)
	var used style.Unit
	for _, it := range line {
		used = used.Add(it.Width)
	}
	if want := u(48); used != want {
		t.Errorf("the line measures %v, want %v — five characters at 12px is 60 "+
			"and the trim is worth 12. %v is the untrimmed width, which is a "+
			"line that decided to trim and then did not", used, want, u(60))
	}

	// And the character is still on it, so this is a trimmed line rather than a
	// broken one.
	if len(line) != len(out) {
		t.Errorf("the line holds %d items and the text has %d; the trim is what "+
			"keeps the last one on it", len(line), len(out))
	}
}
