package style

import (
	"runtime"
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// A nested rule's selectors are relative, one at a time, and its "&" is the
// parent as a unit.
//
// Both halves used to be done by rewriting text: the parent's component values
// were pasted in wherever "&" stood, or once in front of the whole prelude when
// none did. Pasting in front of a list scopes its first selector only, and
// pasting into every "&" copies the parent once per use, at every level. The
// tests below are the first as behaviour and the second as cost.

// nestingListDoc has a paragraph and a heading inside the card and one of each
// outside it, which is what separates a scoped selector from a document-wide one.
const nestingListDoc = `<div class="card"><h2 id="in-h">h</h2><p id="in-p">p</p></div>` +
	`<h2 id="out-h">h</h2><p id="out-p">p</p>`

// TestEverySelectorOfANestedListIsScoped is audit C24: ".card { h2, p { } }" is
// "& h2, & p", so neither selector reaches outside the card.
func TestEverySelectorOfANestedListIsScoped(t *testing.T) {
	doc := parseDoc(t, nestingListDoc)
	for _, src := range []string{
		`.card { h2, p { font-family: scoped } }`,
		// The mixed form: one selector names "&" and the other does not. Each
		// is read on its own, so the second still gets its own "& ".
		`.card { & h2, p { font-family: scoped } }`,
		`.card { h2, & p { font-family: scoped } }`,
		// And the relative form, where one begins with a combinator.
		`.card { > h2, p { font-family: scoped } }`,
	} {
		styled := Apply(doc, []Sheet{author(t, src)})
		for id, want := range map[string]string{
			"#in-h": "scoped", "#in-p": "scoped", "#out-h": "", "#out-p": "",
		} {
			got := styled.Styles[elementFor(t, doc, id)]["font-family"]
			if (want == "") == (got == "scoped") {
				t.Errorf("%s: %s has font-family %q; every selector of a nested list "+
					"is relative to the parent, so only the card's own children "+
					"are styled", src, id, got)
			}
		}
	}
}

// TestARelativeSelectorIsRelativeThroughItsCombinator. A nested selector that
// begins with a combinator is joined to the parent by it — even when it names
// "&" further on, as "> & .bar" does, which is "& > & .bar". The cases are the
// WPT test css-nesting/parsing's, read for which elements they select rather
// than how they serialise.
func TestARelativeSelectorIsRelativeThroughItsCombinator(t *testing.T) {
	doc := parseDoc(t, `<div class="p" id="top"><div class="p" id="mid">`+
		`<i id="deep"><b class="bar" id="b1">x</b></i><b class="bar" id="b2">y</b>`+
		`</div></div><b class="bar" id="b3">z</b>`)
	for _, tc := range []struct{ src, want string }{
		{`.p { > .bar { font-family: hit } }`, "#b2"},
		{`.p { & > .bar { font-family: hit } }`, "#b2"},
		{`.p { .bar { font-family: hit } }`, "#b1 #b2"},
		{`.p { > & .bar { font-family: hit } }`, "#b1 #b2"},
		{`.p { + .bar { font-family: hit } }`, "#b3"},
		{`.p { > .bar, + .bar { font-family: hit } }`, "#b2 #b3"},
	} {
		styled := Apply(doc, []Sheet{author(t, tc.src)})
		var got []string
		for _, id := range []string{"#top", "#mid", "#deep", "#b1", "#b2", "#b3"} {
			if styled.Styles[elementFor(t, doc, id)]["font-family"] == "hit" {
				got = append(got, id)
			}
		}
		if strings.Join(got, " ") != tc.want {
			t.Errorf("%s selected %v, want %s", tc.src, got, tc.want)
		}
	}
}

// TestTheNestingSelectorHasTheSpecificityOfIs. "&" is ":is(<the parent's
// list>)", and ":is()" counts its most specific argument whichever one matched.
// ".c, #nomatch" matches by its class and still counts an id, so the nested
// rule beats three classes written after it.
func TestTheNestingSelectorHasTheSpecificityOfIs(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	for _, src := range []string{
		`.c, #nomatch { p { font-family: wins } } .c.c.c p { font-family: loses }`,
		`.c, #nomatch { & p { font-family: wins } } .c.c.c p { font-family: loses }`,
		`.c, #nomatch { :is(&) p { font-family: wins } } .c.c.c p { font-family: loses }`,
		// And the implicit "&" counts exactly as a written one does.
		`#nomatch, .c { > p { font-family: wins } } .c.c.c p { font-family: loses }`,
	} {
		if got := styleOf(t, doc, []Sheet{author(t, src)}, "#target", "font-family"); got != "wins" {
			t.Errorf("%s gave %q; the nested rule carries the id of its parent's "+
				"list and beats (0,3,1)", src, got)
		}
	}
	// ":where(&)" contributes nothing, like any :where().
	if got := styleOf(t, doc, []Sheet{author(t,
		`#outer { :where(&) p { font-family: loses } } div p { font-family: wins }`)},
		"#target", "font-family"); got != "wins" {
		t.Errorf("\":where(&) p\" gave %q; it has the specificity of p alone", got)
	}
}

// TestNestedDeclarationsKeepTheParentsOwnSpecificity is the other half of the
// last test and the reason it is not "every nested thing counts as :is()": a
// declaration written directly in a nested @media belongs to the rule it is in,
// selector by selector — the WPT test css-nesting/nested-declarations-matching,
// "nested group rules have top-level specificity behavior".
func TestNestedDeclarationsKeepTheParentsOwnSpecificity(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	got := styleOf(t, doc, []Sheet{author(t, `
		.c.c { font-family: wins }
		#nomatch, p.c { @media print { font-family: loses } }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("gave %q; the paragraph matches the rule by \"p.c\", (0,1,1), "+
			"which loses to \".c.c\" — the #nomatch beside it is not counted", got)
	}
}

// TestTheNestingSelectorCannotBeAPseudoElement. "&" is ":is()", which cannot
// represent a pseudo-element, so a parent's pseudo-element selectors are simply
// not part of it: "div::before { & { } }" styles nothing, and not the div, and
// "*, ::before { & * { } }" has the specificity of "* *". These are the WPT tests
// css-nesting/contextually-invalid-selectors-003 and -001.
func TestTheNestingSelectorCannotBeAPseudoElement(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	if got := styleOf(t, doc, []Sheet{author(t, `
		p { font-family: wins }
		p::before { & { font-family: loses } }`)},
		"#target", "font-family"); got != "wins" {
		t.Errorf("\"p::before { & { } }\" set the paragraph's own font-family to %q; "+
			"& cannot stand for a ::before, so it matches nothing", got)
	}
	if got := styleOf(t, doc, []Sheet{author(t, `
		p { font-family: wins }
		*, ::before { & * { font-family: loses } }`)},
		"#target", "font-family"); got != "wins" {
		t.Errorf("gave %q; \"& *\" under \"*, ::before\" has no specificity, and "+
			"loses to \"p\" written before it", got)
	}
	// While the parent's own pseudo-element declarations still apply to it, and
	// a nested @media inside it lands on the pseudo-element too.
	styled := Apply(doc, []Sheet{author(t, `
		p::before { content: "a"; @media print { font-family: nested } }`)})
	key := PseudoKey{Node: elementFor(t, doc, "#target"), Name: "before"}
	if got := styled.Pseudo[key]["font-family"]; got != "nested" {
		t.Errorf("the nested @media under p::before gave the ::before %q", got)
	}
}

// TestAnAmpersandInADroppedArgumentStillCounts is the WPT test
// css-nesting/nest-containing-forgiving. Whether a nested selector "contains &"
// is decided on what was written, so ":is(#target, !&)" is taken as written —
// no "& " in front — even though ":is()" then drops the argument holding the
// "&". It selects #target wherever it is.
func TestAnAmpersandInADroppedArgumentStillCounts(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	got := styleOf(t, doc, []Sheet{author(t,
		`.does-not-exist { :is(#target, !&) { font-family: wins } }`)},
		"#target", "font-family")
	if got != "wins" {
		t.Errorf("gave %q; the selector contains \"&\" as written, so it is not "+
			"made relative to .does-not-exist", got)
	}
}

// TestAmpersandAtTheTopLevelIsTheRoot. With no parent, "&" is ":scope", which
// is the root element, and it has no specificity: the WPT tests
// css-nesting/top-level-is-scope and top-level-parent-pseudo-specificity.
func TestAmpersandAtTheTopLevelIsTheRoot(t *testing.T) {
	doc := parseDoc(t, `<div id="d"><p id="deep">x</p></div>`)
	styled := Apply(doc, []Sheet{author(t, `
		& { font-family: loses }
		:where(&) { font-family: wins }
		& p { font-style: italic }
		& > p { font-weight: 700 }`)})
	root := styled.Styles[elementFor(t, doc, ":root")]
	if got := root["font-family"]; got != "wins" {
		t.Errorf("the root's font-family is %q; a top-level \"&\" has no "+
			"specificity, so the later \":where(&)\" wins", got)
	}
	p := styled.Styles[elementFor(t, doc, "#deep")]
	if p["font-style"] != "italic" {
		t.Errorf("\"& p\" did not select a paragraph inside the root: %q", p["font-style"])
	}
	if p["font-weight"] == "700" {
		t.Error("\"& > p\" selected a paragraph that is not a child of the root")
	}
}

// TestTheWPTNestingBasicCases ports the rules of the WPT reftest
// css-nesting/nesting-basic and implicit-nesting, whose every block is green
// when nesting is right. The values name the outcome, as everywhere here.
func TestTheWPTNestingBasicCases(t *testing.T) {
	doc := parseDoc(t, `
		<div class="test test-1"><div id="t1"></div></div>
		<div class="test test-3"><div class="test-3-child" id="t3"></div></div>
		<div class="test test-4"><section><span><b id="t4"></b></span></section></div>
		<div class="test test-6" id="t6"></div>
		<div class="test t7- t7--" id="t7"><div class="test-7-child"></div></div>
		<div class="test test-8" id="t8"></div>
		<div class="test test-9 t9-- t9-" id="t9"></div>
		<div class="test test-10" id="t10"></div>
		<div class="test test-14" id="t14"></div>
		<div class="test test-i2"><div class="test-i2-child" id="i2"></div></div>
		<div class="test test-i3"><div class="test-i3-child" id="i3"></div></div>
		<div class="test test-i4" id="i4"></div>
		<div class="test test-i5"><div class="test-i5" id="i5"></div></div>
		<div class="test test-i6"><div class="test-i6-child" id="i6"></div></div>`)
	styled := Apply(doc, []Sheet{author(t, `
		.test { font-family: red }
		.test-1 { & > div { font-family: green } }
		.test-3 { & .test-3-child { font-family: green } }
		span > b {
		  .test-4 section & { font-family: green }
		  .test-4 section > & { font-family: red }
		}
		.test-6 { &.test { font-family: green } }
		.test-7, .t7- { & + .test-7-child, &.t7-- { font-family: green } }
		.test-8 { & { font-family: green } }
		.test-9 { &:is(.t9-, &.t9--) { font-family: green } }
		.test-10 { & { font-family: red } font-family: green }
		div.test-14 { div& { font-family: green } }
		.test-i2 { .test-i2-child { font-family: green } }
		.test-i2-child { font-family: red }
		.test-i3-child { font-family: red }
		.test-i3-child { .test-i3 & { font-family: green } }
		.test-i4 { :is(&) { font-family: green } }
		.test-i5 { :is(.test-i5, &.does-not-exist) { font-family: green } }
		.test-i6 { > .foo,.test-i6-child,+ .bar { font-family: green } }`)})
	for _, id := range []string{"#t1", "#t3", "#t4", "#t6", "#t7", "#t8", "#t9",
		"#t10", "#t14", "#i2", "#i3", "#i4", "#i5", "#i6"} {
		if got := styled.Styles[elementFor(t, doc, id)]["font-family"]; got != "green" {
			t.Errorf("%s is %q, want green", id, got)
		}
	}
	for _, f := range styled.Findings {
		t.Errorf("valid nested CSS was reported: %s", f.Message)
	}
}

// TestAnAmpersandBeforeATypeIsRefused. "&div" puts a type selector after a
// simple selector, which a compound does not allow — "div&" is the spelling —
// so the rule is dropped and said to be, rather than read as something else.
func TestAnAmpersandBeforeATypeIsRefused(t *testing.T) {
	doc := parseDoc(t, nestingDoc)
	styled := Apply(doc, []Sheet{author(t,
		`p { font-family: wins } .c { &p { font-family: loses } }`)})
	if got := styled.Styles[elementFor(t, doc, "#target")]["font-family"]; got != "wins" {
		t.Errorf("\"&p\" was applied: %q", got)
	}
	if found, _ := says(styled.Findings, "element name must come first"); !found {
		t.Errorf("\"&p\" was dropped without a word: %v", styled.Findings)
	}
}

// nestedSource is "div { & … & { & … & { … b { font-family: deep } } } }":
// depth levels of width ampersands each, which the old rewrite expanded to width
// to the power depth copies of "div".
func nestedSource(width, depth int) string {
	amps := strings.TrimSpace(strings.Repeat("& ", width))
	var b strings.Builder
	b.WriteString("div {")
	for i := 0; i < depth; i++ {
		b.WriteString(amps + " {")
	}
	b.WriteString(" b { font-family: deep } ")
	for i := 0; i <= depth; i++ {
		b.WriteString("}")
	}
	return b.String()
}

// nestingBytes styles a document with a nested stylesheet and says how many
// bytes it allocated, the least of three.
//
// Bytes rather than time because they are what the old shape spent and what
// the measure needs to be free of: a copy of the parent per "&" is an
// allocation per copy, and an allocation count does not move with the load on
// the machine running the test. (It was timed first, and a loaded machine put
// a linear curve at nine times for four.)
func nestingBytes(t *testing.T, doc string, src string) uint64 {
	t.Helper()
	rules, errs := css.ParseStylesheet(src)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	d := parseDoc(t, doc)
	var best uint64
	for i := 0; i < 3; i++ {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		Apply(d, []Sheet{{Origin: OriginAuthor, Rules: rules}})
		runtime.ReadMemStats(&after)
		if got := after.TotalAlloc - before.TotalAlloc; i == 0 || got < best {
			best = got
		}
	}
	return best
}

// TestNestingCostIsLinearInWidth is audit C2 in its first dimension. Each "&"
// was a copy of the parent, so width w at two levels was w² copies of "div":
// four times the ampersands was sixteen times the selector, and at three levels
// sixty-four. Now each "&" is a pointer, and four times the ampersands is four
// times the source.
//
// Two levels and not more so that the old shape finishes and fails rather than
// running out of memory.
func TestNestingCostIsLinearInWidth(t *testing.T) {
	doc := `<div><div><b></b></div></div>`
	const small, large = 300, 1200
	a := nestingBytes(t, doc, nestedSource(small, 2))
	b := nestingBytes(t, doc, nestedSource(large, 2))
	if ratio := float64(b) / float64(a); ratio > 8 {
		t.Errorf("width %d allocated %d bytes and width %d allocated %d, %.1f times "+
			"for four times the source; linear is four, and a parent copied into "+
			"every \"&\" is sixteen", small, a, large, b, ratio)
	}
}

// TestNestingCostIsLinearInDepth is audit C2 in the other: "& &" nested d deep
// was 2^d copies, and matching it asked about the outermost rule once per way
// of placing every level.
//
// What is counted is the matcher's own steps, which are the work and have no
// noise. Matching is now a table of "does level k match element x", each entry
// worked out once, so its cost is at most the levels times the depth of the
// tree: linear in the source for a given document, which is what this holds
// still. The tree is shallower than the nesting, so the innermost rule cannot
// match and every level has to be asked about — and "no" has to be the real
// answer rather than a budget running out, which is what an exponential looks
// like from here.
func TestNestingCostIsLinearInDepth(t *testing.T) {
	steps := func(depth, tree int) (int, bool) {
		t.Helper()
		doc := parseDoc(t, strings.Repeat("<div>", tree)+`<b id="leaf"></b>`+
			strings.Repeat("</div>", tree))
		s := &Styler{seen: map[string]bool{}, attrOffset: -1}
		var innermost []css.Selector
		for _, r := range s.prepare([]Sheet{author(t, nestedSource(2, depth))}) {
			if len(r.decls) == 1 && serialize(r.decls[0].value) == "deep" {
				innermost = r.selectors
			}
		}
		if len(innermost) != 1 {
			t.Fatalf("depth %d: no single innermost rule among the prepared ones", depth)
		}
		m := NewMatcher(doc)
		matched := m.Match(innermost[0], elementFor(t, doc, "#leaf"))
		if m.Tripped() {
			t.Fatalf("nesting %d deep on a tree %d deep tripped the matching "+
				"budget; the answer is a table of levels by elements, and it is "+
				"being worked out by trying every placement instead", depth, tree)
		}
		return m.steps, matched
	}

	// The answer first. Level k of "& &" needs k nested <div>, so twenty-four
	// levels match under a hundred and do not under sixteen.
	if _, matched := steps(24, 100); !matched {
		t.Error("nesting 24 deep did not match a leaf under 100 <div>")
	}
	const small, large, tree = 24, 96, 16
	a, matchedA := steps(small, tree)
	b, matchedB := steps(large, tree)
	if matchedA || matchedB {
		t.Errorf("nesting %d and %d deep matched under %d <div>: %v %v",
			small, large, tree, matchedA, matchedB)
	}
	if ratio := float64(b) / float64(a); ratio > 8 {
		t.Errorf("depth %d took %d steps and depth %d took %d, %.1f times for "+
			"four times the source; linear is four", small, a, large, b, ratio)
	}
}
