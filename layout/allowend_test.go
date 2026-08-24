package layout

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// hanging-punctuation: allow-end, CSS Text §8.4.
//
// A stop or a comma at the end of a line hangs past it — but only where hanging
// it is what lets the line have it. That is the whole of the difference from
// force-end, which hangs one whether or not it fits, and it is why the decision
// belongs to the line filling rather than to a pass over the items: whether the
// character fits is a question about a line, and there is no line until the fill
// has made one.
//
// The characters are not the ones "first" and "last" hang. Those two are about a
// bracket or a quote; these two are about a full stop or a comma, and §8.4 gives
// the set as a list of thirteen characters rather than as a Unicode category —
// see HangsAsStopOrComma.

// allowEndLines returns what each line of a box reads, in a box of a stated
// number of characters.
//
// Courier at 16px advances 9.59375, so four characters are 38.4px and five are
// 48. The width is set a little above the count so that no assertion here sits
// on a rounding edge: a run of four measured whole and four characters measured
// one at a time differ by a fraction of a layout unit, and a box exactly four
// wide would answer one question about the value and another about that.
//
// The suite writes the same fixtures in Ahem at 16px, where a character is 16
// and its 65px box holds four.
func allowEndLines(t *testing.T, chars int, body, extra string, rules ...string) []string {
	t.Helper()
	width := float64(chars)*9.59375 + 1.5
	root := layoutOf(t, 600, `<div id="d">`+body+`</div>`, noDefaults+
		`#d { font-family: Courier; font-size: 16px; width: `+strconv.FormatFloat(width, 'f', -1, 64)+`px;
		      hanging-punctuation: allow-end; `+extra+` } `+strings.Join(rules, " "))
	var out []string
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "d" {
				for _, ln := range f.Lines {
					var b strings.Builder
					for _, r := range ln.Runs {
						b.WriteString(r.Text)
					}
					out = append(out, b.String())
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// TestACommaHangsOnlyWhenItIsWhatDoesNotFit is §8.4's own sentence, and the
// suite's hanging-punctuation-allow-end-basic is these four rows with its own
// comments on them.
//
// Five characters fit. The question each row asks is *which* character was the
// one that did not, because the value hangs one character and only rescues a
// line that one character's worth of room would rescue.
func TestACommaHangsOnlyWhenItIsWhatDoesNotFit(t *testing.T) {
	for _, tc := range []struct {
		what string
		body string
		want []string
	}{
		// The comma is the sixth character: hanging it leaves five, which fit.
		{"the overflow is the comma", "ab c,", []string{"ab c,"}},
		// The overflow is "d", the fifth. Hanging the comma would not help, so
		// the line breaks where it would have without the value.
		// The trailing space is not on the line at all: §4.1.2's third rule
		// removes a collapsible space at the end of one.
		{"the overflow is a letter", "ab cd,", []string{"ab", "cd,"}},
		// Two commas, and the *first* is the sixth character. One hanging
		// character is not enough, so nothing hangs.
		{"the overflow is the first of two", "ab c,,", []string{"ab", "c,,"}},
		// Two commas, and the *second* is the sixth. Hanging it leaves five.
		{"the overflow is the second of two", "a b,,", []string{"a b,,"}},
		// Nothing to hang, and nothing that needs it.
		{"no comma at all", "ab cd", []string{"ab", "cd"}},
		{"a comma that fits", "ab,", []string{"ab,"}},
	} {
		got := allowEndLines(t, 4, tc.body, "")
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: %q set %q, want %q", tc.what, tc.body, got, tc.want)
		}
	}
}

// TestOnlyTheLastCharacterOfARunIsACandidate is what makes the rule above
// decidable, and it is the third row of the table restated as a property.
//
// §8.4 hangs "a stop or comma", not a run of them. So "ab c,," cannot be
// rescued: the character that overflowed is the first comma, and the value has
// nothing to say about a line that is two characters too long.
func TestOnlyTheLastCharacterOfARunIsACandidate(t *testing.T) {
	for _, body := range []string{"ab c,,", "ab c,,,"} {
		got := allowEndLines(t, 4, body, "")
		if len(got) < 2 {
			t.Errorf("%q set %q on one line; only one character may hang, and this "+
				"needs more than one", body, got)
		}
	}
}

// TestAForcedBreakAfterTheCommaDoesNotUndoTheHang.
//
// The hang is provisional: a character is at the end of a line only until
// something lands after it. A <br> lands after it and takes no room, so the line
// ends there and the comma stays hung — which is the last row of the suite's
// basic fixture, and the one that says the fill has to go on after hanging
// rather than return.
//
// Two lines and not three: a trailing forced break ends the line it is on.
func TestAForcedBreakAfterTheCommaDoesNotUndoTheHang(t *testing.T) {
	got := allowEndLines(t, 4, "ab c,<br>", "")
	if want := []string{"ab c,"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%q, want %q: the comma hangs and the break ends the line it is on",
			got, want)
	}
	// And with something after the break, the break's own line is the second.
	got = allowEndLines(t, 4, "ab c,<br>12345", "")
	if want := []string{"ab c,", "12345"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%q, want %q", got, want)
	}
}

// TestSomethingAfterTheCommaTakesTheHangBack is the other half of "provisional",
// and it is the case that made the fill continue rather than return.
//
// A character that hangs is outside the line, so the line has room it did not
// count. If the next thing on the line takes room, the comma was not at the end
// of the line after all: it goes back into the measure, and the line breaks
// where it would have.
func TestSomethingAfterTheCommaTakesTheHangBack(t *testing.T) {
	// "ab c,d" — the comma is the sixth character and would hang, and then "d"
	// arrives. Hanging is taken back and the line breaks at the space, exactly
	// as it does without the value.
	got := allowEndLines(t, 4, "ab c,d", "")
	want := allowEndLines(t, 4, "ab c,d", "hanging-punctuation: none")
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("with allow-end %q and without it %q; nothing hangs when the "+
			"comma is not at the end of the line", got, want)
	}
}

// TestADocumentThatDoesNotAskForItIsUnchanged is the containment case.
//
// The pass that cuts stops and commas out of the runs walks every item of every
// block, so it must do nothing at all to a document that did not ask — which is
// nearly every document, and every one of the suite's other five thousand.
func TestADocumentThatDoesNotAskForItIsUnchanged(t *testing.T) {
	for _, body := range []string{"ab c,", "ab cd,", "a b,,", "one, two, three."} {
		for _, value := range []string{"none", "first", "last", "first last"} {
			got := allowEndLines(t, 4, body, "hanging-punctuation: "+value)
			want := allowEndLines(t, 4, body, "hanging-punctuation: none")
			if value == "first" || value == "last" || value == "first last" {
				// Those two hang a bracket or a quote, and there is none here,
				// so the lines are the plain ones.
				if strings.Join(got, "|") != strings.Join(want, "|") {
					t.Errorf("%q with %q set %q, want %q", body, value, got, want)
				}
			}
		}
	}
}

// TestTheOtherValuesStillHangTheirOwnCharacters, which is what says the two sets
// are separate: a bracket is not a stop, and allow-end does not hang one.
func TestTheOtherValuesStillHangTheirOwnCharacters(t *testing.T) {
	// "ab c)" — the bracket is the sixth character. allow-end hangs stops and
	// commas, so it does not rescue this line.
	if got := allowEndLines(t, 4, "ab c)", ""); len(got) < 2 {
		t.Errorf("allow-end set %q on one line; a closing bracket is not a stop "+
			"or a comma and the value does not hang one", got)
	}
	// And "last" does hang it, which is the control: the character really is
	// one §8.4 hangs, under the other value.
	if got := allowEndLines(t, 4, "ab c)", "hanging-punctuation: last"); len(got) != 1 {
		t.Errorf("hanging-punctuation: last set %q, want one line", got)
	}
}

// allowEndRightEdges is where each line *begins* when the block is set flush
// right, which is the box's width less the line's measure.
//
// It is the one thing a hang can be seen by. Hanging a character does not move
// it and does not change what the line reads — what it changes is the line's
// measure, and an alignment is the reader's view of that. Every assertion below
// would hold with the value doing nothing at all if it asked about the text.
func allowEndRightEdges(t *testing.T, chars int, body, extra string, rules ...string) []style.Unit {
	t.Helper()
	width := float64(chars)*9.59375 + 1.5
	root := layoutOf(t, 600, `<div id="d">`+body+`</div>`, noDefaults+
		`#d { font-family: Courier; font-size: 16px; text-align: right;
		      width: `+strconv.FormatFloat(width, 'f', -1, 64)+`px;
		      hanging-punctuation: allow-end; `+extra+` } `+strings.Join(rules, " "))
	var out []style.Unit
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "d" {
				for _, ln := range f.Lines {
					if len(ln.Runs) == 0 {
						continue
					}
					out = append(out, ln.Runs[0].X)
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// TestAHangIsOnlyTakenWhereItIsNeeded, measured rather than read.
//
// A comma that fits is inside the line and counts towards its measure. Hanging
// one that fits would be force-end, which is a different value and not this one —
// and the difference is invisible in what the line says, because hanging a
// character does not move it. It shows in the alignment: a line measured one
// character shorter is flush-right one character further out.
func TestAHangIsOnlyTakenWhereItIsNeeded(t *testing.T) {
	// "ab," is three characters in a box of four: nothing overflows, so nothing
	// hangs and the line is measured at three.
	fits := allowEndRightEdges(t, 4, "ab,", "")
	// "ab c," is five in the same box: the comma overflows and hangs, so the
	// line is measured at four and is flush with the right edge.
	hangs := allowEndRightEdges(t, 4, "ab c,", "")
	if len(fits) != 1 || len(hangs) != 1 {
		t.Fatalf("the fixtures made %d and %d lines, want one each", len(fits), len(hangs))
	}
	if !(hangs[0] < fits[0]) {
		t.Errorf("the line whose comma hangs begins at %v and the line whose comma "+
			"fits at %v; the first is measured a character shorter and so begins "+
			"further left, and a comma that fits must not hang", hangs[0], fits[0])
	}
	// And the gap between them is exactly one character: three characters
	// measured against four. Stated as a difference rather than as two numbers,
	// because the box is a little wider than four characters on purpose — see
	// allowEndLines — and the slack cancels.
	if got, want := fits[0].Sub(hangs[0]), courierChar(t); got != want {
		t.Errorf("the two lines begin %v apart, want %v: one is measured at three "+
			"characters and the other at four", got, want)
	}
}

// TestOnlyOneCharacterHangsPerLine. §8.4 hangs "a stop or comma", and the second
// candidate on a line is an ordinary character that does not fit.
//
// Two spans so that the two commas are two items: the pass cuts one character
// out of each run, so a run ending in two commas offers one candidate and this
// needs two.
func TestOnlyOneCharacterHangsPerLine(t *testing.T) {
	got := allowEndRightEdges(t, 2, `<span>ab,</span><span>,</span>`, "")
	if len(got) == 0 {
		t.Fatal("no lines")
	}
	// One comma hangs, so the line is measured at three characters in a box of
	// two — it begins one character *before* the box does. Two hanging would
	// measure it at two and begin it at the box's own edge.
	if !(got[0] < 0) {
		t.Errorf("the line begins at %v; with one of the two commas hanging it is "+
			"three characters wide in a box of two and overhangs the left edge", got[0])
	}
}

// TestTheHangIsGivenBackToTheMeasureNotJustToTheBreak.
//
// The character that follows a hung comma is usually as wide as the comma, so
// the line breaks in the same place whether the hang was taken back or not, and
// only the *measure* differs. This fixture puts something narrower after it —
// an inline-block one pixel wide — so that the two answers differ in the count
// of lines as well.
//
// Without the hang given back, the line still thinks it has the comma's width in
// hand and takes the box onto the line it does not have room for.
func TestTheHangIsGivenBackToTheMeasureNotJustToTheBreak(t *testing.T) {
	const body = `ab c,<span class="tiny"></span>`
	const tiny = `.tiny { display: inline-block; width: 1px; height: 1px }`
	got := allowEndLines(t, 4, body, "", tiny)
	if len(got) != 2 {
		t.Errorf("%q made %d lines, want 2: the comma goes back into the measure "+
			"when the box after it arrives, and there is no room for the box",
			got, len(got))
	}
}

// TestOnlyOneCharacterHangsAcrossSomethingThatTakesNoRoom.
//
// §8.4 hangs "a stop or comma" and not a run of them, and that falls out of the
// restore rather than needing a rule of its own: a comma takes room, so the
// restore fires before a second candidate is ever tested. This is the hardest
// case it has to hold for — two commas with a float between them, which is the
// only thing that stands between two characters and takes no room at all.
func TestOnlyOneCharacterHangsAcrossSomethingThatTakesNoRoom(t *testing.T) {
	const body = `ab,<span class="f"></span>,`
	const float = `.f { float: left; width: 1px; height: 1px }`
	// Three characters in a box of two: one comma hangs and the other does not,
	// so the line is measured at three and overhangs the left edge by one.
	// Two hanging would measure it at two and leave it flush.
	got := allowEndRightEdges(t, 2, body, "", float)
	if len(got) == 0 {
		t.Fatal("no lines")
	}
	if !(got[0] < 0) {
		t.Errorf("the line begins at %v; with one of the two commas hanging it is "+
			"three characters wide in a box of two", got[0])
	}
}

// TestTheHangIsGivenBackWhenSomethingFollowsIt, measured.
//
// A character hung by allow-end is at the end of the line only until something
// lands after it. "ab c,d" has a "d" after the comma, so the comma is inside the
// line and counts: the second line is three characters and not two.
func TestTheHangIsGivenBackWhenSomethingFollowsIt(t *testing.T) {
	got := allowEndRightEdges(t, 4, "ab c,d", "")
	want := allowEndRightEdges(t, 4, "ab c,d", "hanging-punctuation: none")
	if len(got) != 2 || len(want) != 2 {
		t.Fatalf("%d lines with the value and %d without, want 2 each", len(got), len(want))
	}
	if got[1] != want[1] {
		t.Errorf("the second line begins at %v with the value and %v without it; the "+
			"comma has a \"d\" after it, so it is not at the end of the line and "+
			"nothing hangs", got[1], want[1])
	}
}

// TestAForcedBreakDoesNotGiveTheHangBack is the other side of the same rule: a
// <br> takes no room, so the character before it really is at the end of the
// line and keeps its hang.
func TestAForcedBreakDoesNotGiveTheHangBack(t *testing.T) {
	got := allowEndRightEdges(t, 4, "ab c,<br>", "")
	if len(got) != 1 {
		t.Fatalf("%d lines, want 1: the comma hangs, so \"ab c\" fills the box and "+
			"the forced break ends the line it is on", len(got))
	}
	// The line is measured at four characters in a box four characters wide, so
	// it fits: flush right leaves it at or just inside the left edge. With the
	// comma counted it would be five and would begin outside the box.
	//
	// Asked as "inside the box" rather than as a number, because four characters
	// measured one at a time and a box four characters wide differ by a layout
	// unit — a sixty-fourth of a pixel — and that is a fact about rounding
	// rather than about this value.
	if got[0] < 0 || got[0] >= courierChar(t) {
		t.Errorf("the line begins at %v, which is not inside a box of four "+
			"characters: the comma is being counted", got[0])
	}
	if plain := allowEndRightEdges(t, 4, "ab c,<br>", "hanging-punctuation: none"); len(plain) == 1 {
		t.Errorf("the plain document made one line too; the fixture cannot tell the " +
			"value from nothing")
	}
}

// courierChar is one character of the face these fixtures are set in.
func courierChar(t *testing.T) style.Unit {
	t.Helper()
	u, ok := style.FromPx(9.59375)
	if !ok {
		t.Fatal("9.59375px is not representable")
	}
	return u
}

// TestACommaInsideANowrapSpanDoesNotHang.
//
// A cut is not free: the run the character comes out of was one thing to the
// fill — a unit that fits or does not — and two items are two units, so a run
// that does not fit whole can be placed in part. That is right where the line
// may end between them and wrong where it may not.
//
// "white-space: nowrap" on a span is the case where it may not. With the comma
// cut out, the line ended inside the span and left the box after it stranded on
// a line of its own, which is the one thing the span was written to prevent. The
// suite's hanging-punctuation-allow-end asserts it by name: punctuation does not
// hang "when a nowrap span prevents breaking before the punctuation".
func TestACommaInsideANowrapSpanDoesNotHang(t *testing.T) {
	const inner = `12 <span class="nw">34,<span class="ib"></span></span> 1234`
	const rules = `.nw { white-space: nowrap } ` +
		`.ib { display: inline-block; width: 19.1875px; height: 1px }`
	got := allowEndLines(t, 5, inner, "", rules)
	want := allowEndLines(t, 5, inner, "hanging-punctuation: none", rules)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("with allow-end %q and without it %q; the comma has more of the "+
			"nowrap span after it, so no line can end there and nothing hangs",
			got, want)
	}
}

// TestACommaAtTheEndOfANowrapSpanStillHangs is the other side of that rule, and
// the reason it is asked about the *boundary* rather than about the box.
//
// A span that does not wrap says nothing about the edge after its own content.
// The comma below is the last thing in the span, and the space after it is the
// div's — so a line may end there, the character may hang, and it does.
func TestACommaAtTheEndOfANowrapSpanStillHangs(t *testing.T) {
	const inner = `ab <span class="nw">c,</span>`
	const rules = `.nw { white-space: nowrap }`
	// Five characters in a box of four. With the comma hanging the line holds
	// them all; without, "c," goes below as a unit the span will not divide.
	got := allowEndLines(t, 4, inner, "", rules)
	if len(got) != 1 || got[0] != "ab c," {
		t.Errorf("the content set %q, want one line of %q: the comma ends the span, "+
			"and nothing follows it for a line to be unable to end before",
			got, "ab c,")
	}
	// And it is the value doing it: without allow-end the same content breaks.
	if plain := allowEndLines(t, 4, inner, "hanging-punctuation: none", rules); len(plain) == 1 {
		t.Errorf("the plain document set one line too; the fixture cannot tell the " +
			"value from nothing")
	}
}
