package css

import (
	"strings"
	"testing"
)

// A selector this engine does not match is not a selector that is invalid. The
// difference is the rest of the rule: Selectors 4 §3.3 drops a whole list for one
// invalid selector, and keeps it for one that is valid and matches nothing.
// These tests are about the list, which is where the difference shows.

// subject is the pseudo-classes of a selector's last compound.
func subject(s Selector) []Pseudo { return s.Compounds[len(s.Compounds)-1].Pseudos }

// TestAnInapplicableSelectorLeavesTheRestOfItsListStanding is audit C25.
//
// "a:hover, .active" is the standard shape of a framework's stylesheet —
// Bootstrap writes ".btn:hover, .btn.active, .btn.show" — and every browser
// printing the page styles .active. It was refused whole here, because :hover
// was a parse failure, and the finding said nothing was missing.
func TestAnInapplicableSelectorLeavesTheRestOfItsListStanding(t *testing.T) {
	for _, tc := range []struct {
		input       string
		want        int
		unsupported bool
	}{
		{"a:hover, .active", 2, false},
		{".btn:is(:hover, :focus), .btn.active", 2, false},
		{"a::selection, .x", 2, false},
		{"a::before:hover, .x", 2, false},
		// The markup answers these and this engine does not: kept, matching
		// nothing, and claimed as a gap.
		{"input:checked, .on", 2, true},
		{"p:has(img), .figure", 2, true},
		{"input::placeholder, .x", 2, true},
	} {
		sels, errs, ok := parseSel(t, tc.input)
		if !ok || len(sels) != tc.want {
			t.Errorf("%q gave ok=%v and %d selectors, want %d: %v", tc.input, ok,
				len(sels), tc.want, errs)
			continue
		}
		if len(errs) == 0 {
			t.Errorf("%q said nothing about the selector that matches nothing", tc.input)
			continue
		}
		if errs[0].Unsupported != tc.unsupported {
			t.Errorf("%q reported %q with Unsupported=%v, want %v", tc.input,
				errs[0].Message, errs[0].Unsupported, tc.unsupported)
		}
	}

	// A name no specification defines is still invalid, as it is in a browser,
	// and takes its list with it.
	for _, input := range []string{"a:nonesuch, .x", "a::nonesuch, .x", "a:hover(x), .x"} {
		if sels, _, ok := parseSel(t, input); ok || len(sels) != 0 {
			t.Errorf("%q was accepted; a browser refuses the whole list", input)
		}
	}
}

// TestAWrongNoNeverWidensARule is the containment argument for keeping a
// selector this engine cannot answer. "Matches nothing" is a safe wrong answer
// only where it narrows a rule; under a negation it widens one, so the negation
// is unanswered too.
func TestAWrongNoNeverWidensARule(t *testing.T) {
	for _, input := range []string{
		"a:not(:checked)",
		"a:not(.x, :disabled)",
		"a:not(:is(:checked, .x))",
		"a:not(:not(:checked))",
		"li:nth-child(2 of :checked)",
		"li:nth-last-child(odd of .x:has(p))",
	} {
		sels, errs, ok := parseSel(t, input)
		if !ok || len(sels) != 1 {
			t.Errorf("%q was refused: %v", input, errs)
			continue
		}
		if ps := subject(sels[0]); len(ps) != 1 || ps[0].Kind != PseudoUnanswered {
			t.Errorf("%q was read as %+v; a negation of something this engine "+
				"cannot answer must match nothing, not everything", input, ps)
		}
	}

	// A negation of what the medium answers is answered: nothing is hovered,
	// so ":not(:hover)" is every element — and ":is()" of an unanswered one is
	// only narrower, so it is kept as the :is() it is.
	for input, want := range map[string]PseudoKind{
		"a:not(:hover)":         PseudoNot,
		"a:is(:checked, .x)":    PseudoIs,
		"li:nth-child(2 of .x)": PseudoNthChild,
	} {
		sels, _, ok := parseSel(t, input)
		if !ok {
			t.Errorf("%q was refused", input)
			continue
		}
		if ps := subject(sels[0]); len(ps) != 1 || ps[0].Kind != want {
			t.Errorf("%q was read as %+v, want kind %d", input, ps, want)
		}
	}

	// "&" is its parent, and a parent that cannot be answered cannot be
	// negated either.
	parent := NewNesting(mustParseAllowing(t, "input:checked"))
	vals, _ := ParseComponentValues(":not(&)")
	sels, _, ok := ParseNestedSelectorList(vals, parent)
	if !ok {
		t.Fatal(":not(&) was refused")
	}
	if ps := subject(sels[0]); len(ps) != 1 || ps[0].Kind != PseudoUnanswered {
		t.Errorf(":not(&) under input:checked was read as %+v; it must match nothing", ps)
	}
}

// mustParseAllowing parses a selector list that is expected to be usable, and
// may report something about it.
func mustParseAllowing(t *testing.T, input string) []Selector {
	t.Helper()
	sels, errs, ok := parseSel(t, input)
	if !ok {
		t.Fatalf("%q was refused: %v", input, errs)
	}
	return sels
}

// TestAPseudoElementIsNotAnArgument is audit C163's first half. Selectors 4
// §4.2: :is() "cannot represent pseudo-elements", and nor can :where(), :not()
// or "of S". Accepted, "div:is(::before)" had an argument whose compound was
// empty and whose pseudo-element the matcher never looks at, so it selected
// every div; ":is(*, ::before)" counted the pseudo-element's specificity.
func TestAPseudoElementIsNotAnArgument(t *testing.T) {
	// In a forgiving list the argument is dropped and the rest stand.
	for input, left := range map[string]int{
		"div:is(::before)":        0,
		"div:where(::after, .x)":  1,
		":is(*, ::before)":        1,
		"div:is(p::first-line)":   0,
		"div:is(::selection, .y)": 1,
	} {
		sels, errs, ok := parseSel(t, input)
		if !ok || len(sels) != 1 {
			t.Errorf("%q was refused: %v", input, errs)
			continue
		}
		args := subject(sels[0])[0].Args
		if len(args) != left {
			t.Errorf("%q kept %d arguments, want %d", input, len(args), left)
		}
		for _, a := range args {
			if a.PseudoElement != "" {
				t.Errorf("%q kept the argument %q with its pseudo-element", input, sketch(a))
			}
		}
	}
	if got := mustParseAllowing(t, ":is(*, ::before)")[0].Specificity; got != (Specificity{}) {
		t.Errorf(":is(*, ::before) has specificity %v, want that of * alone, %v",
			got, Specificity{})
	}

	// Elsewhere the argument is invalid, and so is the selector.
	for _, input := range []string{
		"p:not(p::after)", "p:not(::before)", "li:nth-child(2 of ::marker)",
	} {
		if sels, _, ok := parseSel(t, input); ok || len(sels) != 0 {
			t.Errorf("%q was accepted", input)
		}
	}
}

// TestOnlyAUserActionPseudoClassFollowsAPseudoElement is audit C163's second
// half. "a::before:first-child" was collected into the compound the
// pseudo-element ends and matched against the element — the reordering the
// check for ".foo" after a pseudo-element was written to stop, still open for
// pseudo-classes. Selectors 4 §3.6.3 allows the user-action ones after a
// pseudo-element and nothing else.
func TestOnlyAUserActionPseudoClassFollowsAPseudoElement(t *testing.T) {
	for _, input := range []string{
		"a::before:not(.x)", "a::before:first-child", "p::first-line:lang(en)",
		"a::before:checked", "a::selection:first-child",
	} {
		if sels, _, ok := parseSel(t, input); ok || len(sels) != 0 {
			t.Errorf("%q was accepted; only a user-action pseudo-class may "+
				"follow a pseudo-element", input)
		}
	}
	for _, input := range []string{"a::before:hover", "a::after:focus", "p::marker:active"} {
		sels, errs, ok := parseSel(t, input)
		if !ok || len(sels) != 1 {
			t.Errorf("%q was refused: %v", input, errs)
			continue
		}
		if sels[0].PseudoElement == "" {
			t.Errorf("%q lost its pseudo-element", input)
		}
		if ps := subject(sels[0]); len(ps) != 1 || ps[0].Kind != PseudoNever {
			t.Errorf("%q was read as %+v; the ::before is never hovered on a page", input, ps)
		}
	}
}

// TestTheDepthBoundIsNotForgiven. A forgiving :is() drops an argument it cannot
// use, and an argument this engine stopped reading because it was nested too
// deeply is not one the author got wrong: the bound refuses the whole list, as
// it did before an :is() with every argument dropped was a valid selector.
func TestTheDepthBoundIsNotForgiven(t *testing.T) {
	deep := maxSelectorDepth + 2
	input := "p, " + strings.Repeat(":is(", deep) + "a" + strings.Repeat(")", deep)
	sels, errs, ok := parseSel(t, input)
	if ok || len(sels) != 0 {
		t.Errorf("a list with a selector nested %d deep was accepted", deep)
	}
	if len(errs) != 1 {
		t.Errorf("the bound was reported %d times, want once: %v", len(errs), errs)
	}
}

// TestWhatMatchesNothingKeepsItsSpecificity. A selector that matches nothing
// never wins by its own specificity, but an :is() around one takes the
// specificity of its most specific argument whichever one matched, and the
// exported Specificity is what was written.
func TestWhatMatchesNothingKeepsItsSpecificity(t *testing.T) {
	for input, want := range map[string]Specificity{
		"a::selection":                       {0, 0, 2},
		"a:hover":                            {0, 1, 1},
		"p:is(:not(#x:checked), .y)":         {1, 1, 1},
		"li:is(:nth-child(2 of #x:checked))": {1, 2, 1},
	} {
		if got := mustParseAllowing(t, input)[0].Specificity; got != want {
			t.Errorf("%q has specificity %v, want %v", input, got, want)
		}
	}
}
