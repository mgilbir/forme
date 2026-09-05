package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// CSS Nesting: a style rule inside a style rule.
//
// Every test here asserts *which declaration won*, for the reason the cascade
// tests give: a nested rule that never applied and a nested rule that applied
// with the wrong specificity both look like "the page is not what I wrote", and
// only the winner names which. The values say what they are — "wins", "loses" —
// so a test cannot pass on a plausible value it did not mean.

const nestingDoc = `<div id="outer" class="c"><p id="target" class="c">x</p></div>`

// TestANestedSelectorWithNoAmpersandIsADescendant is the relaxed syntax every
// browser ships: a nested selector that says nothing about where it goes is
// relative to its parent, with a descendant combinator between them.
//
// It is the case the suite writes, and it is the one that was read as a
// malformed declaration until the parser learnt to look ahead for the "{".
func TestANestedSelectorWithNoAmpersandIsADescendant(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	got := styleOf(t, doc, []Sheet{author(t, `
		#outer { font-family: loses; p { font-family: wins } }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("a nested \"p\" inside \"#outer\" gave %q, want the nested rule to "+
			"have matched the paragraph inside it", got)
	}
	// And it really is a descendant rather than the same element: a nested rule
	// that had been read as applying to the parent would set the div too.
	if got := styleOf(t, doc, []Sheet{author(t, `
		#outer { font-family: outer; p { font-family: inner } }`)},
		"#outer", "font-family"); got != "outer" {
		t.Errorf("the parent's own font-family is %q; a nested rule is about what "+
			"the parent contains and not about the parent", got)
	}
}

// TestTheParentGoesWhereTheAmpersandIs. "&" is the parent, wherever it stands —
// including after the nested selector, which is the whole reason the syntax has
// a symbol for it rather than an implicit position.
func TestTheParentGoesWhereTheAmpersandIs(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	for _, tc := range []struct{ src, want, what string }{
		{`#outer { & p { font-family: wins } }`, "wins", "an explicit descendant"},
		{`#outer { & > p { font-family: wins } }`, "wins", "a child combinator"},
		{`p { &.c { font-family: wins } }`, "wins", "a compound with the parent"},
		{`.c { #target& { font-family: wins } }`, "wins", "the parent second"},
	} {
		if got := styleOf(t, doc, []Sheet{author(t, tc.src)}, "#target", "font-family"); got != tc.want {
			t.Errorf("%s: %q gave %q, want %q", tc.what, tc.src, got, tc.want)
		}
	}
}

// TestAmpersandInsideAFunctionIsStillTheParent. ":not(&)" excludes the parent,
// which means the substitution has to reach inside the function's arguments —
// a rewrite that only looked at the top level would leave a bare "&" for the
// selector parser to reject, and the rule would be dropped rather than applied.
func TestAmpersandInsideAFunctionIsStillTheParent(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	got := styleOf(t, doc, []Sheet{author(t, `
		p { font-family: loses }
		div { :not(&) { font-family: wins } }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("\":not(&)\" inside \"div\" gave %q; the paragraph is not a div, so "+
			"the nested rule should have matched it", got)
	}
}

// TestANestedRuleTakesItsParentsSpecificity.
//
// §2: the parent stands in for ":is(<its selector list>)", and ":is()" takes the
// specificity of its most specific argument. So a rule nested inside "#outer"
// carries an id, and beats a rule with the same compound written out flat.
//
// It is the half of nesting that cannot be seen by looking at which elements
// match: both rules below match the paragraph, and only the specificity decides.
func TestANestedRuleTakesItsParentsSpecificity(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	got := styleOf(t, doc, []Sheet{author(t, `
		#outer { p { font-family: wins } }
		div p { font-family: loses }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("gave %q; a rule nested in \"#outer\" carries that id's "+
			"specificity and beats \"div p\", whichever order they are written in", got)
	}
	// The other way round, so the answer is not simply "the first rule wins".
	got = styleOf(t, doc, []Sheet{author(t, `
		div p { font-family: loses }
		#outer { p { font-family: wins } }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("gave %q with the rules swapped", got)
	}
}

// TestANestedRuleCascadesWhereItIsWritten. Order of appearance is the last term
// of the cascade, and a nested rule's place in it is where it stands in the
// source — after the declarations of the rule holding it, and before whatever
// follows that rule.
func TestANestedRuleCascadesWhereItIsWritten(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	got := styleOf(t, doc, []Sheet{author(t, `
		p { font-family: loses; & { font-family: wins } }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("gave %q; a nested \"&\" has the same specificity as its parent, "+
			"so the later of the two declarations wins and it is the nested one", got)
	}
	got = styleOf(t, doc, []Sheet{author(t, `
		p { & { font-family: loses } }
		p { font-family: wins }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("gave %q; a rule after the one holding the nested rule is later "+
			"than both", got)
	}
}

// TestNestingGoesMoreThanOneDeep. Each level is written against the level above,
// so the second level has to be desugared against the *desugared* first level
// and not against the source the author wrote.
func TestNestingGoesMoreThanOneDeep(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	got := styleOf(t, doc, []Sheet{author(t, `
		div { font-family: one;
		  & p { font-family: two;
		    &.c { font-family: wins } } }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("gave %q; the innermost rule is \"div p.c\", which the paragraph "+
			"matches", got)
	}
}

// TestAnUnusableParentTakesItsNestedRulesWithIt.
//
// A rule whose selector list cannot be used is dropped, which the specification
// requires — and everything nested inside it is written *against* that selector,
// so there is nothing left for it to be relative to. Applying it anyway would
// take the ":is()" of an unusable list, which ":is()" is forgiving about, and
// the nested rule would quietly match far more than the author asked for.
func TestAnUnusableParentTakesItsNestedRulesWithIt(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	got := styleOf(t, doc, []Sheet{author(t, `
		p { font-family: wins }
		::not-a-pseudo-element { p { font-family: loses } }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("gave %q; the outer rule's selector is unusable, so the rule "+
			"nested in it has no parent and cannot apply either", got)
	}

	// A list where only *part* is unusable, which is the case that separates
	// dropping the nested rule from letting ":is()" forgive its way through.
	// ":is()" drops an argument it cannot use and keeps the rest, so a nested
	// rule desugared against this list and then applied would match every
	// paragraph in a div — while the rule that was actually written matches
	// nothing at all, because one bad selector invalidates the whole list.
	got = styleOf(t, doc, []Sheet{author(t, `
		p { font-family: wins }
		div, ::not-a-pseudo-element { p { font-family: loses } }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("gave %q; one unusable selector invalidates the list, and the "+
			"rule nested inside it goes with it rather than applying to the "+
			"half that parsed", got)
	}
}

// TestANestedRuleIsNotAMalformedDeclaration is the parser's half, asserted where
// the mistake was: a style rule inside a block used to be read as a property
// whose colon had gone missing, and reported as invalid CSS. A stylesheet the
// author wrote correctly must not be called wrong.
func TestANestedRuleIsNotAMalformedDeclaration(t *testing.T) {
	decls, rules, errs := css.ParseDeclarations(`color: red; span { color: blue }`)
	for _, e := range errs {
		t.Errorf("valid nested CSS was reported: %s", e.Message)
	}
	if len(decls) != 1 || strings.ToLower(decls[0].Name) != "color" {
		t.Errorf("%d declarations, want the one \"color\": %v", len(decls), decls)
	}
	if len(rules) != 1 {
		t.Fatalf("%d nested rules, want 1", len(rules))
	}
	if rules[0].At || !rules[0].HasBlock {
		t.Errorf("the nested rule came out at=%v hasBlock=%v, want a qualified rule "+
			"with a block", rules[0].At, rules[0].HasBlock)
	}

	// Two rules in a row, which is the case a try-and-see would get wrong: a
	// nested rule is not terminated by a semicolon, so consuming to the next one
	// would swallow both.
	_, rules, errs = css.ParseDeclarations(`a { color: red } b { color: blue }`)
	for _, e := range errs {
		t.Errorf("two nested rules in a row were reported: %s", e.Message)
	}
	if len(rules) != 2 {
		t.Errorf("%d nested rules, want 2 — a rule does not end at a semicolon", len(rules))
	}
}

// TestAMalformedDeclarationIsStillReported is the containment argument. The
// look-ahead must not turn every unreadable declaration into a rule and go
// quiet about it: a declaration with no colon and no block is still wrong.
func TestAMalformedDeclarationIsStillReported(t *testing.T) {
	for _, src := range []string{
		`color red`,
		`color: red; font-family`,
		`; ! ;`,
	} {
		_, _, errs := css.ParseDeclarations(src)
		if len(errs) == 0 {
			t.Errorf("%q was accepted, and it is not a declaration or a rule", src)
		}
	}
}
