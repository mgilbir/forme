package css

import "testing"

// The nesting selector, parsed. What a nested rule selects is tested where it
// is matched, in the style package; what is here is the shape the parser gives
// it, which is the half that decides whether a nested rule is one selector or
// a copy of its parent per "&".

// nested parses a nested rule's prelude against a parent.
func nested(t *testing.T, parent *Nesting, input string) ([]Selector, bool) {
	t.Helper()
	vals, _ := ParseComponentValues(input)
	sels, _, ok := ParseNestedSelectorList(vals, parent)
	return sels, ok
}

// TestTheWPTNestingSelectorsParse is the list of the WPT test
// css-nesting/parsing: each is valid, and the ones that name no "&" or begin
// with a combinator gain a leading "&" compound — the serialisation the test
// expects, read as structure. (Only that direction is asserted: a leading "&"
// the author wrote has the same shape as one that was added.)
func TestTheWPTNestingSelectorsParse(t *testing.T) {
	parent := NewNesting(mustParse(t, ".foo"))
	for input, implicit := range map[string]bool{
		"&": false, "&.bar": false, "& .bar": false, "& > .bar": false,
		"> .bar": true, "> & .bar": true, "+ .bar &": true, ".foo": true,
		".test > & .bar": false, ":is(.bar, .baz)": true, "&:is(.bar, .baz)": false,
		":is(.bar, &.baz)": false, "&:is(.bar, &.baz)": false, "div&": false,
		".class&": false, "&.class": false, "[attr]&": false, "&[attr]": false,
		"#id&": false, "&#id": false, ":is(div)&": false, "&:is(div)": false,
		"& .bar & .baz & .qux": false, "&&": false,
	} {
		sels, ok := nested(t, parent, input)
		if !ok || len(sels) != 1 {
			t.Errorf("%q was refused or split: ok=%v, %d selectors", input, ok, len(sels))
			continue
		}
		first := sels[0].Compounds[0]
		gained := len(first.Pseudos) == 1 && first.Type == "" && len(first.Classes) == 0 &&
			len(first.IDs) == 0 && len(first.Attrs) == 0 &&
			first.Pseudos[0].Kind == PseudoNesting && len(sels[0].Compounds) > 1
		if implicit && !gained {
			t.Errorf("%q did not gain a leading \"&\" compound: %+v", input, sels[0].Compounds)
		}
	}
	// "&div" puts a type after a simple selector, which no compound allows.
	if _, ok := nested(t, parent, "&div"); ok {
		t.Error("\"&div\" was accepted; the type selector must come first")
	}
	// A list is split and each selector is relative on its own.
	sels, ok := nested(t, parent, "+ .bar, .foo, > .baz")
	if !ok || len(sels) != 3 {
		t.Fatalf("the list came to ok=%v, %d selectors", ok, len(sels))
	}
	for i, want := range []Combinator{NextSibling, Descendant, Child} {
		c := sels[i].Compounds
		if len(c) != 2 || c[0].Pseudos[0].Kind != PseudoNesting || c[1].Combinator != want {
			t.Errorf("selector %d is %+v, want \"&\" then %q", i, c, want.String())
		}
	}
}

// TestEveryAmpersandIsTheSameParent is the cost argument as structure: the
// parent is parsed once, and every "&" in the rules under it holds that one
// value rather than a copy of it.
func TestEveryAmpersandIsTheSameParent(t *testing.T) {
	parent := NewNesting(mustParse(t, ".a, #b"))
	sels, ok := nested(t, parent, "& & &, :not(&)")
	if !ok {
		t.Fatal("refused")
	}
	var seen int
	for _, s := range sels {
		for _, c := range s.Compounds {
			for _, p := range c.Pseudos {
				if p.Kind == PseudoNesting {
					seen++
					if p.Nest != parent {
						t.Error("an \"&\" holds a parent other than the one it was parsed against")
					}
				}
				for _, a := range p.Args {
					for _, ac := range a.Compounds {
						for _, ap := range ac.Pseudos {
							if ap.Kind == PseudoNesting && ap.Nest == parent {
								seen++
							}
						}
					}
				}
			}
		}
	}
	if seen != 4 {
		t.Errorf("found %d references to the parent, want 4", seen)
	}
	// And its specificity is :is()'s, the most specific of the list.
	if got := sels[0].Specificity; got != (Specificity{3, 0, 0}) {
		t.Errorf("\"& & &\" under \".a, #b\" is %v, want (3,0,0)", got)
	}
}

// TestAnAmpersandAtTheTopLevelIsTheRootWithNoWeight. Outside any rule "&" is
// ":scope", the root, and counts nothing towards specificity.
func TestAnAmpersandAtTheTopLevelIsTheRootWithNoWeight(t *testing.T) {
	sels := mustParse(t, "& .x")
	if got := sels[0].Specificity; got != (Specificity{0, 1, 0}) {
		t.Errorf("\"& .x\" at the top level is %v, want (0,1,0)", got)
	}
	p := sels[0].Compounds[0].Pseudos[0]
	if p.Kind != PseudoWhere || len(p.Args) != 1 ||
		p.Args[0].Compounds[0].Pseudos[0].Kind != PseudoRoot {
		t.Errorf("a top-level \"&\" is %+v, want :where(:root)", p)
	}
	// A leading combinator is still an error at the top level: there is nothing
	// for it to be relative to.
	if _, _, ok := parseSel(t, "> .x"); ok {
		t.Error("\"> .x\" was accepted at the top of a stylesheet")
	}
}
