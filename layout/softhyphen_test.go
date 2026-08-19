package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// The soft hyphen, CSS Text §6.1, and the "hyphens: manual" that is every
// document's initial value.
//
// U+00AD is a character an author writes *inside* a word to say "break here if
// you must, and print a hyphen when you do". It is invisible everywhere else. It
// is what "&shy;" is for, and it is the whole of what "manual" asks for — so
// this is not an opt-in feature: a page with a soft hyphen in a long word was a
// page where the word did not break, and the initial value says it should have.
//
// This file used to hold the opposite. TestNoLineBreaksAtASoftHyphen asserted
// that no line was broken at one, and said in its own comment that the day
// someone implemented breaking there it would fail and point at the entry in
// style/inert.go that had to change with it. That is exactly what happened, and
// the entry now says "manual" where it said "none".
//
// Courier at 20px is 12px a character throughout.
//
// The character is named rather than written: a soft hyphen in a source file is
// invisible, survives a copy and paste as nothing at all, and would make every
// fixture below unreadable in a diff.
const shy = "\u00ad"

// drawnText joins the text of everything painted, in painting order.
func drawnText(ops []Op) string {
	var b strings.Builder
	for _, op := range ops {
		if d, ok := op.(DrawText); ok {
			b.WriteString(d.Text)
		}
	}
	return b.String()
}

// TestASoftHyphenBreaksTheLine is the property, and it is named in style/inert.go
// as what holds the claim that this engine produces "hyphens: manual".
func TestASoftHyphenBreaksTheLine(t *testing.T) {
	const soft = "" + shy
	// Eight characters in a box that holds five, with the opportunity in the
	// middle: it must break, and it cannot break anywhere else.
	root := layoutOf(t, 600, `<div id="p">aaaa`+soft+`bbbb</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 60px }`)
	f := find(t, root, "p")
	if len(f.Lines) != 2 {
		t.Fatalf("%d lines; the word has a soft hyphen in it and does not fit on one",
			len(f.Lines))
	}
	// And the same word without one does not break, so the two lines above are
	// the soft hyphen's doing and not the box's width.
	solid := layoutOf(t, 600, `<div id="p">aaaabbbb</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 60px }`)
	if got := len(find(t, solid, "p").Lines); got != 1 {
		t.Fatalf("the control broke into %d lines, want 1: a word with nothing to "+
			"break at is set past the edge, so this fixture would pass either way", got)
	}
}

// TestABrokenWordPrintsAHyphen. The break is half of what the character asks
// for; the hyphen is the other half, and without it the reader is shown a word
// split in two with nothing to say so.
func TestABrokenWordPrintsAHyphen(t *testing.T) {
	ops := paintOf(t, "<div id=\"p\">aaaa"+shy+"bbbb</div>",
		`#p { font-family: Courier; font-size: 20px; width: 60px }`)
	got := drawnText(ops)
	if !strings.ContainsAny(got, "‐-") {
		t.Errorf("the page reads %q; a word broken at a soft hyphen is printed with "+
			"a hyphen", got)
	}
	// After the first half and not somewhere else on the line.
	if i := strings.IndexAny(got, "‐-"); i >= 0 && !strings.HasPrefix(got[i:], "‐") &&
		!strings.HasPrefix(got[i:], "-") {
		t.Errorf("the page reads %q", got)
	}
	if !strings.HasPrefix(got, "aaaa") {
		t.Errorf("the page reads %q, and the first line is the part before the "+
			"soft hyphen", got)
	}
}

// TestASoftHyphenThatIsNotAtABreakIsInvisible is the other half and the one that
// is true of almost every soft hyphen ever written: the character is a mark for
// the line breaking, not a mark on the page.
func TestASoftHyphenThatIsNotAtABreakIsInvisible(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 400px }`
	with := paintOf(t, "<div id=\"p\">aaaa"+shy+"bbbb</div>", css)
	without := paintOf(t, `<div id="p">aaaabbbb</div>`, css)
	if got := drawnText(with); strings.ContainsAny(got, "‐-") {
		t.Errorf("the page reads %q; the line did not break, so nothing is hyphenated",
			got)
	}
	if len(fillsOfAny(with)) != len(fillsOfAny(without)) {
		t.Errorf("a soft hyphen that is not at a break changed what is painted")
	}
}

// TestTheHyphenIsPaidForBeforeTheLineIsBroken, and what happens when it cannot
// be paid for.
//
// This is the arithmetic the feature turns on, and the answer is not the obvious
// one. The suite's hyphens-overflow-001 states it case by case in a box five
// characters wide, and the two halves are these:
//
//   - "12 45&shy;xx" goes back to the space and sets "12" then "45xx", with no
//     hyphen printed. The text before the soft hyphen fits on its own; the
//     hyphen after it does not, and there is an earlier opportunity to use
//     instead — so the word is not hyphenated at all.
//   - "12345&shy;xx" has no earlier opportunity and breaks at the soft hyphen
//     anyway, printing the hyphen past the edge of the box.
//
// So the hyphen is charged where the charge can be avoided and hangs where it
// cannot. Reading only the first case gives a line that never overflows and
// loses half the suite's fixtures; reading only the second gives a line that
// overflows whenever a word could have been left whole.
//
// Courier at 20px is 12px a character, so 61.2px is the same 5.1 characters
// those fixtures use.
func TestTheHyphenIsPaidForBeforeTheLineIsBroken(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 61.2px }`
	lines := func(markup string) (*Fragment, string) {
		t.Helper()
		doc := `<div id="p">` + markup + `</div>`
		return find(t, layoutOf(t, 600, doc, css), "p"), drawnText(paintOf(t, doc, css))
	}

	// The hyphen does not fit and there is a space to go back to.
	f, text := lines("12 45\u00adxx")
	if len(f.Lines) != 2 {
		t.Fatalf("%d lines for \"12 45<shy>xx\"", len(f.Lines))
	}
	if strings.ContainsAny(text, "‐-") {
		t.Errorf("the page reads %q; the line went back to the space, so the word "+
			"was never broken and nothing is hyphenated", text)
	}
	if got := f.Lines[0].Runs[0].Width.Px(); got > 24.5 {
		t.Errorf("the first line is %gpx; it holds \"12\", which is 24", got)
	}

	// The same sum with no space in it: the hyphen hangs past the edge.
	f, text = lines("12345\u00adxx")
	if len(f.Lines) != 2 {
		t.Fatalf("%d lines for \"12345<shy>xx\"", len(f.Lines))
	}
	if !strings.ContainsAny(text, "‐-") {
		t.Errorf("the page reads %q; there is nowhere else for this line to end, so "+
			"it breaks at the soft hyphen and the hyphen overflows", text)
	}
	var w float64
	for _, r := range f.Lines[0].Runs {
		w += r.Width.Px()
	}
	if w < 71.5 || w > 72.5 {
		t.Errorf("the broken line is %gpx, want 72 — five characters and the hyphen, "+
			"in a box of 61.2", w)
	}

	// And the exact case, where the hyphen is the last thing that fits.
	f, text = lines("1234\u00adxx")
	if len(f.Lines) != 2 || !strings.ContainsAny(text, "‐-") {
		t.Errorf("%d lines reading %q; four characters and a hyphen is 60 in a box "+
			"of 61.2, so this one breaks", len(f.Lines), text)
	}
}

// TestHyphensNoneBreaksNothing. The value exists to turn the character off, and
// it is the one of the three that now asks for a page this engine would not
// otherwise produce.
func TestHyphensNoneBreaksNothing(t *testing.T) {
	root := layoutOf(t, 600, "<div id=\"p\">aaaa"+shy+"bbbb</div>",
		`#p { font-family: Courier; font-size: 20px; width: 60px; hyphens: none }`)
	if got := len(find(t, root, "p").Lines); got != 1 {
		t.Errorf("%d lines under hyphens: none; the value says the word may not be "+
			"broken at all", got)
	}
	ops := paintOf(t, "<div id=\"p\">aaaa"+shy+"bbbb</div>",
		`#p { font-family: Courier; font-size: 20px; width: 60px; hyphens: none }`)
	if got := drawnText(ops); strings.ContainsAny(got, "‐-") {
		t.Errorf("the page reads %q under hyphens: none", got)
	}

	// And it reaches the text from an ancestor, which is the whole point of the
	// property inheriting: a document turns hyphenation off once, on a wrapper,
	// and means it for everything inside. Declared on the box that holds the
	// text, this would pass whether or not the property inherited at all.
	nested := layoutOf(t, 600,
		"<div id=\"outer\"><div id=\"p\"><span>aaaa"+shy+"bbbb</span></div></div>",
		`#outer { hyphens: none }
		 #p { font-family: Courier; font-size: 20px; width: 60px }`)
	if got := len(find(t, nested, "p").Lines); got != 1 {
		t.Errorf("%d lines: hyphens: none was set two boxes up and the property "+
			"inherits", got)
	}
}

// TestASoftHyphenBeforeASpaceIsNotABreak. There would be nothing after it to
// move to the next line: the word has already ended.
//
// This asserts the behaviour and does not pin the guard, and the difference is
// worth stating rather than leaving for the next reader to discover. Deleting
// the guard in breaks.go leaves this test passing and moves nothing on the
// suite: what it withdraws is a break opportunity in front of a space, which
// UAX #14's LB7 forbids and which every path that would take one already
// declines for its own reasons. So the property below is protected several times
// over and this is the statement of it, not the thing holding it up. See the
// comment beside the guard for the measurement.
func TestASoftHyphenBeforeASpaceIsNotABreak(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 48px;
		white-space: pre-wrap }`
	doc := "<div id=\"p\">aaa" + shy + " bbb</div>"
	if got := drawnText(paintOf(t, doc, css)); strings.ContainsAny(got, "\u2010-") {
		t.Errorf("the page reads %q; the soft hyphen is the last thing in its word",
			got)
	}
	f := find(t, layoutOf(t, 600, doc, css), "p")
	if len(f.Lines) != 2 {
		t.Fatalf("%d lines, want 2", len(f.Lines))
	}
	if second := f.Lines[1].Runs[0].Text; strings.HasPrefix(second, " ") {
		t.Errorf("the second line begins %q; a line may not end in front of a "+
			"space, so the space belongs to the line before it", second)
	}
}

// TestTheSoftHyphenIsFoundAcrossABoxBoundary.
//
// The suite's hyphens-span-001 writes one word nine ways — the soft hyphen
// before a </span>, after one, alone inside one, with the halves in separate
// spans — and asks for the same hyphen in the same place from all of them. So
// the opportunity a soft hyphen offers cannot stop at the end of the text node
// it is in, and neither can the hyphen: a </span> between the character and the
// word after it is not something a reader sees.
//
// hyphens-out-of-flow-001 asks the same question with an absolutely positioned
// box in each of those places, which is the other thing that can be written
// between the two without being on the line at all.
func TestTheSoftHyphenIsFoundAcrossABoxBoundary(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 72px }
		span.abs { position: absolute; color: transparent }`
	// Six characters of room and a seven-character word: it must break, and the
	// only place it may is the soft hyphen.
	for _, markup := range []string{
		"high" + shy + "way",
		"<span>high</span>" + shy + "way",
		"<span>high" + shy + "</span>way",
		"high<span>" + shy + "</span>way",
		"high<span>" + shy + "way</span>",
		"high" + shy + "<span>way</span>",
		"<span>high</span>" + shy + "<span>way</span>",
		"<span>high" + shy + "wa</span>y",
		"<span>hi</span>gh" + shy + "way",
		// And the same with a box that is not on the line at all.
		"high<span class=abs>x</span>" + shy + "way",
		"high" + shy + "<span class=abs>x</span>way",
	} {
		doc := `<div id="p">` + markup + `</div>`
		f := find(t, layoutOf(t, 600, doc, css), "p")
		if len(f.Lines) != 2 {
			t.Errorf("%s: %d lines, want 2", markup, len(f.Lines))
			continue
		}
		if got := drawnText(paintOf(t, doc, css)); !strings.ContainsAny(got, "\u2010-") {
			t.Errorf("%s: the page reads %q and no hyphen was printed", markup, got)
		}
	}
}

// TestTheHyphenIsACharacterTheFaceCanSet.
//
// U+2010 HYPHEN is the typographically right character and CSS Text §6.1 allows
// it, but a face that has no glyph for it does not say so by refusing: the
// standard PDF faces substitute a space for a character outside WinAnsi, which
// is what a reader shows for an undefined code. So "did asking for it produce a
// glyph" is a question that says yes for every character there is, and asking it
// put a space where the hyphen belonged — a word broken in two with nothing
// between the halves.
//
// Courier has no U+2010 and a real TrueType face does, which is what makes this
// two rows rather than an assertion about one.
func TestTheHyphenIsACharacterTheFaceCanSet(t *testing.T) {
	courier, err := shape.Standard("Courier")
	if err != nil {
		t.Fatalf("no Courier: %v", err)
	}
	if got := hyphenTextFor(courier); got != "-" {
		t.Errorf("Courier was given %q; U+2010 is outside WinAnsi and this face "+
			"draws a space for it", got)
	}
	face, err := shape.Load(realFont())
	if err != nil {
		t.Fatalf("loading the bundled face: %v", err)
	}
	if got := hyphenTextFor(face); got != "\u2010" {
		t.Errorf("a face with a glyph for U+2010 was given %q", got)
	}
	// And nothing at all still answers, because a nil face is what a run that
	// could not be set carries.
	if got := hyphenTextFor(nil); got != "-" {
		t.Errorf("no face was given %q", got)
	}
}

// fillsOfAny counts the fills in a display list, whatever their colour.
func fillsOfAny(ops []Op) []FillRect {
	var out []FillRect
	for _, op := range ops {
		if r, ok := op.(FillRect); ok {
			out = append(out, r)
		}
	}
	return out
}
