package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
)

// Computed values: the em is gone by the time a value is stored.
//
// CSS says a length's computed value is an absolute length, and it matters
// because computed values are what inheritance passes on. This engine stored
// what the author wrote and resolved it in layout, against the box's own
// font-size — the right answer for the element that made the declaration and
// the wrong one for every element that inherited it.
//
// It is wrong in the direction that hides. A document where every size is set
// in em looks plausible at any scale, and only a fixture that nests to two
// different depths shows the difference at all; the suite's own
// margin-em-inherit-001 is such a fixture and says so in its comment.

// nested is a document three elements deep, so a value can be asked for at the
// element that declared it and at two elements that did not.
const nested = `<div id="p"><div id="c"><span id="g">x</span></div></div>`

// computedOf returns one element's computed value for a property.
func computedOf(t *testing.T, src, selector, property string) string {
	t.Helper()
	doc := parseDoc(t, nested)
	return styleOf(t, doc, []Sheet{author(t, src)}, selector, property)
}

// TestAnEmComputesToAnAbsoluteLength, on the element that wrote it.
func TestAnEmComputesToAnAbsoluteLength(t *testing.T) {
	for _, tc := range []struct{ property, want string }{
		{"margin-left", "112px"},   // 4em at 28px
		{"text-indent", "56px"},    // 2em
		{"line-height", "42px"},    // 1.5em
		{"letter-spacing", "-7px"}, // -0.25em, and a negative one is legal
	} {
		got := computedOf(t, `#p { font-size: 28px; margin-left: 4em; text-indent: 2em;
			line-height: 1.5em; letter-spacing: -0.25em }`, "#p", tc.property)
		if got != tc.want {
			t.Errorf("%s computed to %q, want %q", tc.property, got, tc.want)
		}
	}
}

// TestAnInheritedLengthKeepsTheSizeItWasComputedAt is the bug, and the reason
// the resolution had to move out of layout.
//
// #p is set in 28px and #c in 40px. "margin: inherit" on #c takes #p's computed
// value, which is 112px; resolving the author's "4em" against #c's own size
// gives 160px, which is a margin in a plausible wrong place.
func TestAnInheritedLengthKeepsTheSizeItWasComputedAt(t *testing.T) {
	src := `#p { font-size: 28px; margin-left: 4em; text-indent: 2em }
	        #c { font-size: 40px; margin-left: inherit }
	        #g { font-size: 10px }`
	// Explicitly, through the inherit keyword.
	if got := computedOf(t, src, "#c", "margin-left"); got != "112px" {
		t.Errorf("margin-left: inherit gave %q, want 112px — the parent's computed "+
			"value. 160px is its 4em resolved again at 40px", got)
	}
	// And implicitly, through an inherited property, which is the commoner half
	// by far and needs no keyword at all.
	for _, sel := range []string{"#c", "#g"} {
		if got := computedOf(t, src, sel, "text-indent"); got != "56px" {
			t.Errorf("%s inherited text-indent as %q, want 56px", sel, got)
		}
	}
}

// TestARelativeFontSizeIsAbsoluteOnceComputed, and does not compound.
//
// "font-size: 2em" on #c is twice #p's 28px. What is stored is the answer, so
// #g inheriting it inherits 56px rather than an instruction to double again.
func TestARelativeFontSizeIsAbsoluteOnceComputed(t *testing.T) {
	src := `#p { font-size: 28px } #c { font-size: 2em }`
	if got := computedOf(t, src, "#c", "font-size"); got != "56px" {
		t.Errorf("font-size: 2em computed to %q, want 56px", got)
	}
	if got := computedOf(t, src, "#g", "font-size"); got != "56px" {
		t.Errorf("the inherited font-size is %q, want 56px — 112px is the value "+
			"doubling a second time", got)
	}
}

// TestRemResolvesAgainstTheRoot, wherever it is written and however deep.
func TestRemResolvesAgainstTheRoot(t *testing.T) {
	src := `html { font-size: 20px } #p { font-size: 28px; margin-left: 2rem }`
	if got := computedOf(t, src, "#p", "margin-left"); got != "40px" {
		t.Errorf("2rem computed to %q, want 40px — twice the root's 20px, not twice "+
			"the element's 28px", got)
	}
}

// TestARemOnTheRootsOwnFontSizeIsTheInitialValue. CSS Values §5.1.1 carves it
// out by name, and it has to be carved out: the value a rem would otherwise
// refer to is the one being computed.
//
// Without the exception the answer depends on what the root's size happened to
// be when the question was asked, which is the sort of bug that comes out
// differently depending on the order of a walk.
func TestARemOnTheRootsOwnFontSizeIsTheInitialValue(t *testing.T) {
	doc := parseDoc(t, nested)
	got := styleOf(t, doc, []Sheet{author(t, `html { font-size: 2rem }`)},
		"html", "font-size")
	if got != "32px" {
		t.Errorf("font-size: 2rem on the root computed to %q, want 32px — twice the "+
			"initial 16px", got)
	}
	// And a rem in any *other* property on the root is the root's own computed
	// size, which is the answer the exception does not apply to.
	got = styleOf(t, doc, []Sheet{author(t, `html { font-size: 2rem; margin-left: 1rem }`)},
		"html", "margin-left")
	if got != "32px" {
		t.Errorf("margin-left: 1rem on the root computed to %q, want 32px — the "+
			"root's own computed size", got)
	}
}

// TestAPercentageIsNotAbsolutised is the first containment case, and it is a
// rule rather than an omission: a percentage margin is a fraction of the
// containing block, which nothing here knows, and CSS says its computed value
// is the percentage itself. The suite's margin-percentage-inherit-001 asserts
// that an inherited one is resolved again against the *child's* containing
// block, which is only possible if the percentage survives.
func TestAPercentageIsNotAbsolutised(t *testing.T) {
	src := `#p { font-size: 28px; margin-left: 10%; width: 50%;
	             background-size: 50% 1em }
	        #c { margin-left: inherit }`
	for _, tc := range []struct{ sel, property, want string }{
		{"#p", "margin-left", "10%"},
		{"#p", "width", "50%"},
		{"#c", "margin-left", "10%"},
		// A percentage *beside* an em, which is the case that reaches the walk
		// at all: a value with no em in it is never parsed, so a fixture of one
		// percentage alone would be held by the pre-filter rather than by the
		// rule under test.
		{"#p", "background-size", "50% 28px"},
	} {
		if got := computedOf(t, src, tc.sel, tc.property); got != tc.want {
			t.Errorf("%s %s computed to %q, want %q", tc.sel, tc.property, got, tc.want)
		}
	}
}

// TestTheUnitsThatNeedAFaceAreLeftAlone is the honest half.
//
// ex is the x-height and ch is the advance of "0", and both need the face that
// will set the element — chosen in layout, after the font-family computed here
// has been read. The cascade has no face, so it cannot turn those into a number,
// and the viewport units need a page box settled later still.
//
// They keep the old behaviour: right on the element that wrote them, and
// inherited as though the declaration had been made again lower down. That is a
// gap, and this test is what says it is a known one rather than an accident —
// the day the cascade is given faces, it is the test that has to change.
func TestTheUnitsThatNeedAFaceAreLeftAlone(t *testing.T) {
	for _, tc := range []struct{ property, value, want string }{
		{"width", "3ex", "3ex"},
		{"width", "3ch", "3ch"},
		{"width", "3vw", "3vw"},
		{"margin-left", "2lh", "2lh"},
	} {
		got := computedOf(t, `#p { font-size: 28px; `+tc.property+`: `+tc.value+` }`,
			"#p", tc.property)
		if got != tc.want {
			t.Errorf("%s: %s computed to %q, want it left as it was", tc.property, tc.value, got)
		}
	}

	// And each of them beside an em, which is what makes this a test of the
	// units rather than of the pre-filter. "3ex" alone holds none of the two
	// letters the filter looks for, so it is never parsed and would come back
	// unchanged whatever the walk did with it; "1em 3ex" is parsed, and the
	// second half has to survive the same pass that rewrote the first.
	for _, tc := range []struct{ value, want string }{
		{"1em 3ex", "28px 3ex"},
		{"1em 3ch", "28px 3ch"},
		{"1em 3vw", "28px 3vw"},
	} {
		got := computedOf(t, `#p { font-size: 28px; background-size: `+tc.value+` }`,
			"#p", "background-size")
		if got != tc.want {
			t.Errorf("background-size: %s computed to %q, want %q", tc.value, got, tc.want)
		}
	}
}

// TestTheLettersEMInSomethingThatIsNotALength are left alone, which is the
// containment case for the cheap pre-filter this walks values with.
//
// A font family called Emblem, a counter called em, a string holding "2em": all
// three contain the two letters the filter looks for and none is a dimension.
// Rewriting any of them would be a value the author did not write.
func TestTheLettersEMInSomethingThatIsNotALength(t *testing.T) {
	for _, tc := range []struct{ property, value string }{
		{"font-family", "Emblem"},
		{"content", `"2em"`},
		{"background-image", "url(2em.png)"},
	} {
		got := computedOf(t, `#p { font-size: 28px; `+tc.property+`: `+tc.value+` }`,
			"#p", tc.property)
		if got != tc.value {
			t.Errorf("%s: %s computed to %q, want it untouched", tc.property, tc.value, got)
		}
	}
}

// TestAFontSizeThatCannotBeResolvedIsLeftAsWritten, and the element is still
// marked as having declared one.
//
// The cascade has no answer for "3ic" and must not invent one. What it leaves
// behind is the declaration and the mark, which together are exactly what
// layout needs: an element that declared a font-size it could not resolve, to
// report against and to fall back to the inherited size for. A descendant that
// merely inherited the same string must not be reported a second time, and
// must not resolve it either.
func TestAFontSizeThatCannotBeResolvedIsLeftAsWritten(t *testing.T) {
	doc := parseDoc(t, nested)
	got := Apply(doc, []Sheet{author(t, `#p { font-size: 3ic }`)})
	p := elementFor(t, doc, "#p")
	if v := got.Styles[p]["font-size"]; v != "3ic" {
		t.Errorf("an unresolvable font-size computed to %q; the cascade has no answer "+
			"for it and must not write one", v)
	}
	if !got.OwnFontSize[p] {
		t.Error("the element that declared the unresolvable size was not marked as " +
			"having declared one; that mark is how layout tells it from a descendant " +
			"that inherited the same string")
	}
	c := elementFor(t, doc, "#c")
	if got.OwnFontSize[c] {
		t.Error("a descendant that inherited the string was marked as declaring it")
	}
	// The descendant does get a computed value, and it is the size it is really
	// set in: the one it inherited. Resolving the string a second time is what
	// the mark exists to prevent, and leaving the string in place would hand
	// the same trap to whatever reads it next.
	if v := got.Styles[c]["font-size"]; v != "16px" {
		t.Errorf("the descendant's font-size is %q, want 16px — the size it "+
			"inherited, not the string its parent could not resolve", v)
	}
}

// TestEveryElementGetsAFontSizeEvenWithNoStylesheet: the initial 16px, which is
// what every em in a document that says nothing about font-size is relative to.
func TestEveryElementGetsAFontSizeEvenWithNoStylesheet(t *testing.T) {
	if got := computedOf(t, `#p { margin-left: 2em }`, "#p", "margin-left"); got != "32px" {
		t.Errorf("2em with no font-size anywhere computed to %q, want 32px", got)
	}
}

// TestAPseudoElementResolvesAgainstItsOwnSize, which is its originating
// element's unless a rule gave it one — the same inheritance every other
// property of a pseudo-element follows.
func TestAPseudoElementResolvesAgainstItsOwnSize(t *testing.T) {
	doc := parseDoc(t, nested)
	got := Apply(doc, []Sheet{author(t,
		`#p { font-size: 28px } #p::before { content: "x"; margin-left: 2em }
		 #c { font-size: 28px } #c::before { content: "x"; font-size: 10px; margin-left: 2em }`)})
	for _, tc := range []struct{ sel, want string }{
		{"#p", "56px"}, // inherited 28px
		{"#c", "20px"}, // its own 10px
	} {
		n := elementFor(t, doc, tc.sel)
		cs, ok := got.Pseudo[PseudoKey{Node: n, Name: "before"}]
		if !ok {
			t.Fatalf("%s::before produced no style", tc.sel)
		}
		if cs["margin-left"] != tc.want {
			t.Errorf("%s::before margin-left computed to %q, want %q",
				tc.sel, cs["margin-left"], tc.want)
		}
	}
}

// TestALengthInsideAFunctionIsRewrittenToo asks the walk directly, and says why
// it has to.
//
// No property this engine implements takes a function holding a length, so
// there is no declaration that can reach the recursion — a test written through
// the cascade would pass with the recursion deleted, which is a test that proves
// nothing. Asking the walk is asking the thing that would be wrong.
//
// It recurses because a length inside a function is still a length. Leaving it
// out would be a rule that held right up until the day something used one, and
// then produced a page measured in the wrong units with nothing to say so.
func TestALengthInsideAFunctionIsRewrittenToo(t *testing.T) {
	size, _ := FromPx(28)
	root, _ := FromPx(16)
	for _, tc := range []struct{ in, want string }{
		{"foo(2em)", "foo(56px)"},
		{"foo(2em, 3rem)", "foo(56px, 48px)"},
		{"foo(bar(0.5em))", "foo(bar(14px))"},
		// And what it must leave alone, inside a function as much as outside.
		{"foo(2ex)", "foo(2ex)"},
		{"foo(50%)", "foo(50%)"},
		{"foo(2)", "foo(2)"},
	} {
		vals, errs := css.ParseComponentValues(tc.in)
		if len(errs) != 0 {
			t.Fatalf("%q did not parse: %v", tc.in, errs)
		}
		absolutiseValues(vals, size, root)
		if got := serialize(vals); got != tc.want {
			t.Errorf("%q became %q, want %q", tc.in, got, tc.want)
		}
	}
}
