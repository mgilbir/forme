package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// hanging-punctuation, CSS Text §8.4.
//
// A paragraph that begins with an opening quotation mark has a ragged left edge
// without it: the mark is narrow and light, and the letters after it start a
// character further in than the letters of every line below. This is the
// property that asks for the mark to sit in the margin instead.
//
// Courier at 20px is 12px a character throughout, so every number below is a
// count of characters.

const hangCSS = `#p { font-family: Courier; font-size: 20px; width: 240px }`

// firstRunX is where the first run of the first line begins, relative to the
// block's content edge — which is what "hangs into the margin" moves.
func firstRunX(t *testing.T, markup, css string) style.Unit {
	t.Helper()
	f := find(t, layoutOf(t, 600, `<div id="p">`+markup+`</div>`, hangCSS+css), "p")
	if len(f.Lines) == 0 || len(f.Lines[0].Runs) == 0 {
		t.Fatalf("%q laid out no runs", markup)
	}
	return f.Lines[0].Runs[0].X
}

// TestAnOpeningBracketHangsIntoTheMargin is the feature.
func TestAnOpeningBracketHangsIntoTheMargin(t *testing.T) {
	plain := firstRunX(t, "(one two", "")
	hung := firstRunX(t, "(one two", `#p { hanging-punctuation: first }`)
	want := plain.Sub(chars(1))
	if hung != want {
		t.Errorf("the line begins at %v with the bracket hanging and at %v without; "+
			"one Courier character at 20px is 12px", hung, want)
	}
}

// TestOnlyTheFirstFormattedLineHangs. §8.4 says "the first formatted line of an
// element", and the suite's own fixture is a block whose second line begins with
// a bracket too: "(This should hang.<br>(This should not."
func TestOnlyTheFirstFormattedLineHangs(t *testing.T) {
	f := find(t, layoutOf(t, 600, `<div id="p">(one<br>(two</div>`,
		hangCSS+`#p { hanging-punctuation: first }`), "p")
	if len(f.Lines) != 2 {
		t.Fatalf("%d lines, want 2", len(f.Lines))
	}
	first, second := f.Lines[0].Runs[0].X, f.Lines[1].Runs[0].X
	if first.Add(chars(1)) != second {
		t.Errorf("the two lines begin at %v and %v; only the first hangs, so the "+
			"second starts one character further in", first, second)
	}
}

// TestAClosingBracketHangsPastTheEnd, which is measured through the width the
// line is aligned to rather than through where it begins: a hanging character
// is one the alignment does not count.
func TestAClosingBracketHangsPastTheEnd(t *testing.T) {
	// Right-aligned, so what the line is measured as decides where it sits.
	right := func(css string) style.Unit {
		t.Helper()
		f := find(t, layoutOf(t, 600, `<div id="p">one)</div>`,
			hangCSS+`#p { text-align: right }`+css), "p")
		return f.Lines[0].Runs[0].X
	}
	plain := right("")
	hung := right(`#p { hanging-punctuation: last }`)
	if want := plain.Add(chars(1)); hung != want {
		t.Errorf("the right-aligned line begins at %v with the bracket hanging and "+
			"%v was wanted; the bracket sits past the edge and is not counted",
			hung, want)
	}
}

// TestABoxEdgeWithRoomInItStopsTheHang.
//
// "<span style='border-left:1em'>(</span>text" must not hang: there is a border
// between the margin and the bracket, so the bracket is not at the start of the
// line at all. The suite's reference for hanging-punctuation-first compensates
// that row with a margin instead of an indent, which is how it says so.
//
// A zero one is a boundary and nothing more, and the bracket still hangs.
func TestABoxEdgeWithRoomInItStopsTheHang(t *testing.T) {
	plain := firstRunX(t, "<span>(</span>one", "")
	for _, tc := range []struct {
		css   string
		hangs bool
		what  string
	}{
		{"span { border-left: 10px solid blue }", false, "a border"},
		{"span { margin-left: 10px }", false, "a margin"},
		{"span { padding-left: 10px }", false, "padding"},
		{"span { border-left: 0 }", true, "a border of zero"},
		{"span { margin-left: 0; padding-left: 0 }", true, "a margin and padding of zero"},
		{"span { color: red }", true, "nothing that takes room"},
	} {
		got := firstRunX(t, "<span>(</span>one", `#p { hanging-punctuation: first }`+tc.css)
		base := firstRunX(t, "<span>(</span>one", tc.css)
		hung := got != base
		if hung != tc.hangs {
			t.Errorf("%s: the line begins at %v against %v without the property; "+
				"hung=%v, want %v", tc.what, got, base, hung, tc.hangs)
		}
		_ = plain
	}
}

// TestCollapsibleWhiteSpaceIsNotRoom. §4.1.1 removes it at the edge of a line,
// so it stands between nothing — which is what
// hanging-punctuation-last-whitespace asks about.
//
// The trailing case is the one that reaches the rule. A collapsible space
// *before* the first character never becomes an item at all: §4.1.1's fourth
// rule drops it where a block begins, so the walk to the first run never meets
// one and the fixture below would pass with the rule deleted. A trailing one
// does become an item — it hangs — so the walk to the last run has to pass over
// it, and deleting the rule costs the suite a test.
func TestCollapsibleWhiteSpaceIsNotRoom(t *testing.T) {
	right := func(markup, css string) style.Unit {
		t.Helper()
		f := find(t, layoutOf(t, 600, `<div id="p">`+markup+`</div>`,
			hangCSS+`#p { text-align: right }`+css), "p")
		return f.Lines[0].Runs[0].X
	}
	plain := right("one)  ", "")
	hung := right("one)  ", `#p { hanging-punctuation: last }`)
	if want := plain.Add(chars(1)); hung != want {
		t.Errorf("the line begins at %v, want %v: the spaces after the bracket hang "+
			"past the end of the line and are not between anything", hung, want)
	}
	// And the leading case, which is the property doing the right thing for a
	// reason this rule is not responsible for.
	before := firstRunX(t, "   (one", "")
	if got := firstRunX(t, "   (one", `#p { hanging-punctuation: first }`); got != before.Sub(chars(1)) {
		t.Errorf("the line begins at %v, want %v", got, before.Sub(chars(1)))
	}
}

// TestOneCharacterHangsAndNotARun. §8.4 hangs "an opening bracket or quote",
// which is one of them: "((one" puts the second bracket inside the line.
func TestOneCharacterHangsAndNotARun(t *testing.T) {
	one := firstRunX(t, "(one", "").Sub(firstRunX(t, "(one",
		`#p { hanging-punctuation: first }`))
	two := firstRunX(t, "((one", "").Sub(firstRunX(t, "((one",
		`#p { hanging-punctuation: first }`))
	if one != two {
		t.Errorf("one bracket hangs by %v and two by %v; the property hangs a "+
			"character, not a run of them", one, two)
	}
}

// TestEveryOpeningAndClosingMarkHangs walks the categories §8.4 names: Ps and Pe
// for the brackets, Pi and Pf for the directional quotation marks, and the two
// ASCII quotes, which belong to no category of their own.
func TestEveryOpeningAndClosingMarkHangs(t *testing.T) {
	for _, tc := range []struct{ mark, what string }{
		{"(", "a parenthesis, Ps"},
		{"[", "a bracket, Ps"},
		{"«", "a guillemet, Pi"},
		{"“", "a left double quote, Pi"},
		{"”", "a right double quote, Pf — a language may open with one"},
		{`"`, "the ASCII double quote, which is Po"},
		{"'", "the ASCII apostrophe, likewise"},
	} {
		plain := firstRunX(t, tc.mark+"one", "")
		hung := firstRunX(t, tc.mark+"one", `#p { hanging-punctuation: first }`)
		if hung >= plain {
			t.Errorf("%s: the line begins at %v with the property and %v without; "+
				"it did not hang", tc.what, hung, plain)
		}
	}
	// And a character that is neither does not.
	for _, mark := range []string{"a", "1", ".", "-", "€"} {
		plain := firstRunX(t, mark+"one", "")
		hung := firstRunX(t, mark+"one", `#p { hanging-punctuation: first }`)
		if hung != plain {
			t.Errorf("%q hung; it is not a bracket or a quote", mark)
		}
	}
}

// TestFirstAndLastTogether, which the grammar allows and the suite writes.
func TestFirstAndLastTogether(t *testing.T) {
	both := firstRunX(t, "(one)", `#p { hanging-punctuation: first last }`)
	only := firstRunX(t, "(one)", `#p { hanging-punctuation: first }`)
	if both != only {
		t.Errorf("with both values the line begins at %v and with first alone at %v; "+
			"the closing bracket hangs at the other end and moves neither", both, only)
	}
	if none := firstRunX(t, "(one)", ""); both == none {
		t.Errorf("the line begins at %v either way, so this fixture cannot tell "+
			"whether anything hung", none)
	}
}

// TestAShrinkToFitBoxIsNotSizedForWhatHangs.
//
// Every one of the suite's fixtures for this property floats its boxes, which is
// what makes the intrinsic width the half that decides the page: a float sized
// as though the bracket were inside the line is a character too wide, with the
// bracket drawn in its margin anyway.
func TestAShrinkToFitBoxIsNotSizedForWhatHangs(t *testing.T) {
	width := func(css string) style.Unit {
		t.Helper()
		f := find(t, layoutOf(t, 600, `<div id="p">(one)</div>`,
			`#p { font-family: Courier; font-size: 20px; float: left }`+css), "p")
		return f.ContentRect().W
	}
	plain := width("")
	for _, tc := range []struct {
		css  string
		by   float64
		what string
	}{
		{`#p { hanging-punctuation: first }`, 12, "the opening bracket"},
		{`#p { hanging-punctuation: last }`, 12, "the closing one"},
		{`#p { hanging-punctuation: first last }`, 24, "both"},
	} {
		if got, want := width(tc.css), plain.Sub(chars(tc.by/12)); got != want {
			t.Errorf("%s: the float is %v wide, want %v — %gpx narrower than the %v "+
				"it is without the property", tc.what, got, want, tc.by, plain)
		}
	}
}

// TestNothingHangsWithoutTheProperty is the containment case: the initial value
// is none, which is what almost every document has.
func TestNothingHangsWithoutTheProperty(t *testing.T) {
	base := firstRunX(t, "(one)", "")
	for _, css := range []string{
		"",
		`#p { hanging-punctuation: none }`,
		`#p { hanging-punctuation: wibble }`,
		`#p { hanging-punctuation: first first }`,
		`#p { hanging-punctuation: none first }`,
	} {
		if got := firstRunX(t, "(one)", css); got != base {
			t.Errorf("%q moved the line to %v from %v", css, got, base)
		}
	}
}

// TestHangingPunctuationInherits, which lets a document ask for it once.
func TestHangingPunctuationInherits(t *testing.T) {
	f := find(t, layoutOf(t, 600, `<div id="outer"><div id="p">(one</div></div>`,
		hangCSS+`#outer { hanging-punctuation: first }`), "p")
	plain := firstRunX(t, "(one", "")
	if got := f.Lines[0].Runs[0].X; got != plain.Sub(chars(1)) {
		t.Errorf("the line begins at %v; the property was set on the box outside and "+
			"inherits", got)
	}
}

// TestTheEndValuesAreReported. force-end and allow-end are about a stop or a
// comma at the end of *any* line and neither is implemented — and what they
// change is where a line breaks, which shows as a word moved with nothing on the
// page to say why.
func TestTheEndValuesAreReported(t *testing.T) {
	for _, value := range []string{"force-end", "allow-end", "first allow-end"} {
		built := Build(Input{
			HTML: `<div id="p">one, two, three</div>`,
			CSS:  []Stylesheet{{Source: hangCSS + `#p { hanging-punctuation: ` + value + ` }`}},
		})
		rec := NewRecorder(nil)
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(10000)
		Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
		found := false
		for _, f := range rec.Findings() {
			if f.Property == "hanging-punctuation" && f.Unsupported() {
				found = true
			}
		}
		if !found {
			t.Errorf("%q was not reported: %v", value, rec.Findings())
		}
	}
	// And the two that are implemented are not reported.
	for _, value := range []string{"first", "last", "first last", "none"} {
		built := Build(Input{
			HTML: `<div id="p">(one)</div>`,
			CSS:  []Stylesheet{{Source: hangCSS + `#p { hanging-punctuation: ` + value + ` }`}},
		})
		rec := NewRecorder(nil)
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(10000)
		Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
		for _, f := range rec.Findings() {
			if f.Property == "hanging-punctuation" {
				t.Errorf("%q was reported: %s", value, f.Message)
			}
		}
	}
}

// chars is a number of Courier characters at 20px, which is what every
// measurement in this file is a count of.
func chars(n float64) style.Unit {
	u, _ := style.FromPx(n * 12)
	return u
}

// TestAnIdeographicSpaceHangs, which is the one entry in the set that has to be
// argued for rather than read off a Unicode category.
//
// U+3000 is not punctuation. It is what a Japanese paragraph is indented with —
// the language has no first-line indent of its own, so an author writes a
// full-width space and the indent is one character — and §8.4's whole subject is
// the ragged edge a mark at the start of a line leaves.
// hanging-punctuation-first-002 is that fixture: an arrow after the space, asked
// to line up with an arrow that has no space in front of it at all.
func TestAnIdeographicSpaceHangs(t *testing.T) {
	plain := firstRunX(t, "x", "")
	hung := firstRunX(t, "　x", `#p { hanging-punctuation: first }`)
	if hung.Add(chars(1)) != plain {
		t.Errorf("the line begins at %v with the space hanging and the same text "+
			"without a space begins at %v; the two should be one character apart",
			hung, plain)
	}
	// And without the property it takes its room, so the row above is measuring
	// the hang rather than a character that never counted.
	if got := firstRunX(t, "　x", ""); got != plain {
		t.Errorf("without the property the line begins at %v, want %v — the space "+
			"is in the line and the run starts where it always did", got, plain)
	}
}

// TestTheValueIsReadOffTheBoxTheCharacterIsIn.
//
// The property inherits, so a rule on the block reaches the text inside it
// whatever box that text is in — which is why reading the block's value was
// right for almost every document and wrong for one shape.
//
// hanging-punctuation-inline-001 is that shape: "字字字字<span>」</span>" with the
// value on the span and nowhere else. The block's value there is "none", and the
// bracket the author asked to hang sat inside the line pushing the text along.
func TestTheValueIsReadOffTheBoxTheCharacterIsIn(t *testing.T) {
	// A float is sized to what its content has to hold, and a character that
	// hangs is not part of that — which is what makes the hang visible as a
	// number. Measuring where the run ends would not: the run keeps its advance
	// and the hang is what sits outside the line, not outside the run.
	width := func(markup, css string) style.Unit {
		t.Helper()
		root := layoutOf(t, 600, `<div><div id="f">`+markup+`</div></div>`,
			noDefaults+`#f { float: left; font-family: Courier; font-size: 20px } `+css)
		return find(t, root, "f").BorderRect.W
	}
	onSpan := width(`one<span class=h>)</span>`, `.h { hanging-punctuation: last }`)
	onBlock := width(`one<span>)</span>`, `#f { hanging-punctuation: last }`)
	if onSpan != onBlock {
		t.Errorf("with the value on the span the box is %v and with it on the "+
			"block %v; the character is the same character either way",
			onSpan, onBlock)
	}
	// Three characters rather than four, and the plain box is four, so the two
	// agree on something rather than on nothing.
	if onSpan != chars(3) {
		t.Errorf("the box is %v, want %v — three Courier characters at 20px",
			onSpan, chars(3))
	}
	if plain := width(`one<span>)</span>`, ``); plain != chars(4) {
		t.Errorf("with nothing set the box is %v, want %v", plain, chars(4))
	}
}

// TestAnInnerValueDoesNotReachACharacterOutsideIt is the containment half of the
// rule above: the value belongs to the box the character is in, so a span that
// asks for a hang does not make the block's own text hang.
func TestAnInnerValueDoesNotReachACharacterOutsideIt(t *testing.T) {
	// The bracket is the block's, and the span holding the value is elsewhere
	// on the line.
	got := firstRunX(t, `(one <span class=h>two</span>`, `.h { hanging-punctuation: first }`)
	if want := firstRunX(t, `(one <span>two</span>`, ``); got != want {
		t.Errorf("the line begins at %v, want %v — the value is on a span that does "+
			"not hold the bracket", got, want)
	}
}
