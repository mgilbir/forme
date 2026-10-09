package style

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/costtest"
)

// CSS Cascade 5 §7.3.3 "revert" and §7.3.4 "revert-layer". See revert.go.
//
// Most of these ride on font-family, for the reason cascade_test.go gives: a
// family is a free ident, so a value can name the declaration it came from, and
// a test cannot pass because the wrong declaration happened to say the same
// thing. font-family inherits, and the parent sets "parent", so "unset" — what
// both keywords were read as — is a value of its own too.

// revertDoc is the document every case styles: #a inside #p.
const revertDoc = `<div id="p"><p id="a">x</p></div>`

// reverted is #a's computed value for a property under a user-agent, a user and
// an author sheet, any of which may be empty, and the findings raised. The
// author sheet always sets the parent's family to "parent".
func reverted(t *testing.T, markup, ua, user, au, property string) (string, []Finding) {
	t.Helper()
	var sheets []Sheet
	for _, s := range []struct {
		origin Origin
		src    string
	}{
		{OriginUserAgent, ua},
		{OriginUser, user},
		{OriginAuthor, "#p { font-family: parent } " + au},
	} {
		if s.src == "" {
			continue
		}
		sheets = append(sheets, sheet(t, s.origin, s.src))
	}
	doc := parseDoc(t, markup)
	got := Apply(doc, sheets)
	return got.Styles[elementFor(t, doc, "#a")].Get(property), got.Findings
}

// noRollbackFinding fails if any finding says either keyword is not
// implemented, which every case here now is.
func noRollbackFinding(t *testing.T, what string, findings []Finding) {
	t.Helper()
	for _, f := range findings {
		if strings.Contains(f.Message, "revert") {
			t.Errorf("%s: a finding still names the keyword: %q", what, f.Message)
		}
	}
}

type revertCase struct {
	what                       string
	markup                     string // revertDoc where empty
	ua, user, author, property string // font-family where empty
	want                       string
}

func runRevertCases(t *testing.T, cases []revertCase) {
	t.Helper()
	for _, c := range cases {
		markup, property := c.markup, c.property
		if markup == "" {
			markup = revertDoc
		}
		if property == "" {
			property = "font-family"
		}
		got, findings := reverted(t, markup, c.ua, c.user, c.author, property)
		if got != c.want {
			t.Errorf("%s: %s is %q, want %q", c.what, property, got, c.want)
		}
		noRollbackFinding(t, c.what, findings)
	}
}

// TestRevertRollsBackToThePreviousOrigin is §7.3.3: the origin, and every
// origin above it, as if it had written nothing.
func TestRevertRollsBackToThePreviousOrigin(t *testing.T) {
	runRevertCases(t, []revertCase{
		{
			what: "an author revert falls back to the user agent",
			ua:   `p { font-family: ua }`,
			// The more specific author rule is removed too: the whole origin
			// goes, not the declaration that said revert.
			author: `p#a { font-family: author } p { font-family: revert !important }`,
			want:   "ua",
		},
		{
			what:   "an author revert falls back to the user's sheet first",
			ua:     `p { font-family: ua }`,
			user:   `p { font-family: user }`,
			author: `#a { font-family: author } #a { font-family: revert }`,
			want:   "user",
		},
		{
			// A user revert removes the author origin as well, which is above
			// it, so the author's ordinary rule does not come back.
			what:   "a user revert skips the author and lands on the user agent",
			ua:     `p { font-family: ua }`,
			user:   `p { font-family: revert !important }`,
			author: `#a { font-family: author }`,
			want:   "ua",
		},
		{
			what: "a user-agent revert is unset, which inherits a family",
			ua:   `p { font-family: revert }`,
			want: "parent",
		},
		{
			what:   "an author revert with nothing beneath it is unset",
			author: `#a { font-family: author } #a { font-family: revert }`,
			want:   "parent",
		},
		{
			// unset of a property that does not inherit is its initial value.
			what:     "nothing beneath a revert of display is display's initial value",
			markup:   `<div id="p"><div id="a">x</div></div>`,
			author:   `#a { display: flex } #a { display: revert }`,
			property: "display",
			want:     "inline",
		},
		{
			what:     "display: revert on a div is the user agent's block",
			markup:   `<div id="p"><div id="a">x</div></div>`,
			ua:       `div { display: block }`,
			author:   `#a { display: flex } #a { display: revert }`,
			property: "display",
			want:     "block",
		},
		{
			// An important author revert does not stop at the author's own
			// ordinary declarations: they are in the origin it removes.
			what:   "an important author revert removes the author's normal rules too",
			user:   `p { font-family: user }`,
			author: `#a { font-family: author } p { font-family: revert !important }`,
			want:   "user",
		},
		{
			what:   "a winner rolled back to is itself rolled back",
			ua:     `p { font-family: ua }`,
			user:   `p { font-family: revert }`,
			author: `#a { font-family: revert }`,
			want:   "ua",
		},
	})
}

// TestAHeadingsSizeReverts is the commonest use: an author's reset undone for
// one element, back to the user agent's 2em of the parent's 10px.
func TestAHeadingsSizeReverts(t *testing.T) {
	got, _ := reverted(t, `<div id="p"><h1 id="a">x</h1></div>`,
		`h1 { font-size: 2em }`, "",
		`#p { font-size: 10px } h1 { font-size: 3em } h1 { font-size: revert }`,
		"font-size")
	if got != "20px" {
		t.Errorf("font-size is %q; a heading's reverted size is the user agent's "+
			"2em of 10px", got)
	}
}

// TestRevertLayerRollsBackToThePreviousLayer is §7.3.4.
func TestRevertLayerRollsBackToThePreviousLayer(t *testing.T) {
	runRevertCases(t, []revertCase{
		{
			what:   "to the layer below",
			author: `@layer base, top; @layer base { #a { font-family: base } } @layer top { p { font-family: revert-layer } }`,
			want:   "base",
		},
		{
			// The whole layer goes, including a rule in it more specific than
			// the one that said revert-layer — here the later one wins in the
			// layer, so the earlier and more specific is what would be left if
			// only the declaration were removed.
			what: "the whole of its own layer, not only the declaration",
			author: `@layer base { #a { font-family: base } }
				@layer top { p#a { font-family: same } p#a { font-family: revert-layer } }`,
			want: "base",
		},
		{
			// Unlayered declarations are their own layer, above every other:
			// revert-layer in one rolls back into the layers, not to the
			// previous origin.
			what:   "from unlayered into the layers",
			ua:     `p { font-family: ua }`,
			author: `@layer base { #a { font-family: base } } #a { font-family: revert-layer }`,
			want:   "base",
		},
		{
			// §7.3.4: with nothing lower in the origin, it is revert.
			what:   "to the previous origin where no lower layer set it",
			ua:     `p { font-family: ua }`,
			author: `@layer a { #a { font-family: revert-layer } }`,
			want:   "ua",
		},
		{
			what:   "with no layers at all, it is revert",
			ua:     `p { font-family: ua }`,
			author: `#a { font-family: author } #a { font-family: revert-layer }`,
			want:   "ua",
		},
		{
			what: "in the user agent's sheet with nothing lower, unset",
			ua:   `p { font-family: revert-layer }`,
			want: "parent",
		},
		{
			what: "a chain of layers each rolling back",
			author: `@layer a, b, c; @layer a { #a { font-family: a } }
				@layer b { #a { font-family: revert-layer } }
				@layer c { #a { font-family: revert-layer } }`,
			want: "a",
		},
		{
			what: "between sublayers of one layer",
			author: `@layer x { @layer b { #a { font-family: inner } }
				@layer c { #a { font-family: revert-layer } } }`,
			want: "inner",
		},
		{
			// A layer's own rules are the implicit last of its sublayers, so a
			// revert-layer there removes those and not the sublayers'.
			what: "from a layer's own rules into its sublayers",
			author: `@layer x { @layer b { #a { font-family: inner } }
				#a { font-family: revert-layer } }`,
			want: "inner",
		},
		{
			what: "from a sublayer to an earlier top-level layer",
			author: `@layer early { #a { font-family: early } }
				@layer x.b { #a { font-family: revert-layer } }`,
			want: "early",
		},
		{
			what: "between anonymous layers",
			author: `@layer { #a { font-family: first } }
				@layer { #a { font-family: revert-layer } }`,
			want: "first",
		},
		{
			// Important reverses the layers: an important declaration in an
			// earlier layer beats one in a later. Rolling the earlier back
			// leaves the later one's important declaration.
			what: "an important revert-layer to a later layer's important rule",
			author: `@layer a, b; @layer a { #a { font-family: revert-layer !important } }
				@layer b { #a { font-family: bimportant !important } #a { font-family: bnormal } }`,
			want: "bimportant",
		},
		{
			// And removes the normal declarations of its own layer with it.
			what: "an important revert-layer removes its layer's normal rules",
			author: `@layer a, b; @layer a { #a { font-family: a } }
				@layer b { #a { font-family: bnormal } #a { font-family: revert-layer !important } }`,
			want: "a",
		},
		{
			what: "in the user's sheet",
			ua:   `p { font-family: ua }`,
			user: `@layer x { p { font-family: userx } } p { font-family: revert-layer }`,
			want: "userx",
		},
		{
			// A layer of one origin is not the layer of the same name in
			// another: the user agent's "x" is not removed with the author's.
			what:   "one origin's layer is not another's of the same name",
			ua:     `@layer x { p { font-family: uax } }`,
			author: `@layer x { #a { font-family: revert-layer } }`,
			want:   "uax",
		},
	})
}

// TestARollbackInAStyleAttribute: an element-attached declaration is in the
// author origin for revert, and in a step of its own for revert-layer.
func TestARollbackInAStyleAttribute(t *testing.T) {
	runRevertCases(t, []revertCase{
		{
			what:   "an inline revert removes the author's rules",
			markup: `<div id="p"><p id="a" style="font-family: revert">x</p></div>`,
			ua:     `p { font-family: ua }`,
			author: `#a { font-family: author }`,
			want:   "ua",
		},
		{
			what:   "an inline revert-layer removes the attribute only",
			markup: `<div id="p"><p id="a" style="font-family: revert-layer">x</p></div>`,
			ua:     `p { font-family: ua }`,
			author: `@layer l { #a { font-family: layered } } #a { font-family: author }`,
			want:   "author",
		},
		{
			// §7.3.4, in as many words: not the intervening author important
			// rules.
			what:   "an important inline revert-layer leaves the author's important rules",
			markup: `<div id="p"><p id="a" style="font-family: revert-layer !important">x</p></div>`,
			ua:     `p { font-family: ua }`,
			author: `#a { font-family: authorimportant !important } #a { font-family: author }`,
			want:   "authorimportant",
		},
		{
			// An important author revert beats a normal attribute and removes
			// it with the rest of its origin.
			what:   "an important author revert removes a normal attribute",
			markup: `<div id="p"><p id="a" style="font-family: inline">x</p></div>`,
			ua:     `p { font-family: ua }`,
			author: `#a { font-family: revert !important }`,
			want:   "ua",
		},
		{
			// And revert-layer there leaves it: the attribute is not in the
			// unlayered author layer the rule is.
			what:   "an important unlayered revert-layer leaves a normal attribute",
			markup: `<div id="p"><p id="a" style="font-family: inline">x</p></div>`,
			ua:     `p { font-family: ua }`,
			author: `#a { font-family: revert-layer !important }`,
			want:   "inline",
		},
		{
			// A user important revert removes the author origin, attribute and
			// all.
			what:   "a user important revert removes the attribute",
			markup: `<div id="p"><p id="a" style="font-family: inline">x</p></div>`,
			ua:     `p { font-family: ua }`,
			user:   `p { font-family: revert !important }`,
			want:   "ua",
		},
	})
}

// TestARollbackPastAPresentationalHint. §6.1 gives the hints an origin between
// the user's and the author's that is part of the author origin for revert and
// not for revert-layer.
func TestARollbackPastAPresentationalHint(t *testing.T) {
	runRevertCases(t, []revertCase{
		{
			what:     "revert removes a hint with the author origin",
			markup:   `<div id="p"><img id="a" width="5"></div>`,
			ua:       `img { width: 3px }`,
			author:   `img { width: revert }`,
			property: "width",
			want:     "3px",
		},
		{
			what:     "revert-layer from the lowest layer lands on the hint",
			markup:   `<div id="p"><img id="a" width="5"></div>`,
			ua:       `img { width: 3px }`,
			author:   `@layer base { img { width: revert-layer } }`,
			property: "width",
			want:     "5px",
		},
		{
			what:     "revert-layer from unlayered lands on the hint",
			markup:   `<div id="p"><img id="a" width="5"></div>`,
			author:   `img { width: 9px } img { width: revert-layer }`,
			property: "width",
			want:     "5px",
		},
	})
}

// TestARollbackOnAShorthandRollsBackEveryLonghand. A CSS-wide keyword on a
// shorthand is that keyword on each longhand, and each rolls back on its own.
func TestARollbackOnAShorthandRollsBackEveryLonghand(t *testing.T) {
	for _, kw := range []string{"revert", "revert-layer"} {
		for property, want := range map[string]string{
			"margin-top": "3px", "margin-right": "4px",
			"margin-bottom": "3px", "margin-left": "4px",
		} {
			got, findings := reverted(t, revertDoc, `p { margin: 3px 4px }`, "",
				`#a { margin: 9px } #a { margin: `+kw+` }`, property)
			if got != want {
				t.Errorf("margin: %s left %s %q, want the user agent's %q",
					kw, property, got, want)
			}
			noRollbackFinding(t, "margin: "+kw, findings)
		}
	}
	// And a longhand reverted to a value the user agent wrote as a shorthand.
	got, _ := reverted(t, revertDoc, `p { margin: 3px 4px }`, "",
		`#a { margin: 9px } #a { margin-left: revert }`, "margin-left")
	if got != "4px" {
		t.Errorf("margin-left: revert is %q, want the user agent's 4px", got)
	}
}

// TestARollbackOfALogicalProperty. css-logical makes a logical property and the
// physical one it sets one property, so a revert of either removes both, and a
// revert of the logical spelling rolls back to whatever set the physical side.
func TestARollbackOfALogicalProperty(t *testing.T) {
	runRevertCases(t, []revertCase{
		{
			what:     "a physical revert removes the logical declaration",
			ua:       `p { margin-left: 3px }`,
			author:   `#a { margin-inline-start: 9px } #a { margin-left: revert }`,
			property: "margin-left",
			want:     "3px",
		},
		{
			what:     "a logical revert removes the physical declaration",
			ua:       `p { margin-left: 3px }`,
			author:   `#a { margin-left: 9px } #a { margin-inline-start: revert }`,
			property: "margin-left",
			want:     "3px",
		},
		{
			what:     "a logical revert in right-to-left text is the right margin",
			ua:       `p { margin-right: 3px }`,
			author:   `#a { direction: rtl; margin-right: 9px } #a { margin-inline-start: revert }`,
			property: "margin-right",
			want:     "3px",
		},
		{
			// direction is read before the rename, so it is rolled back there
			// too: the user agent's rtl stands and the logical margin is on the
			// right.
			what:     "a reverted direction decides the logical side",
			ua:       `p { direction: rtl }`,
			author:   `#a { direction: ltr } #a { direction: revert; margin-inline-start: 5px }`,
			property: "margin-right",
			want:     "5px",
		},
		{
			// Both at once: the direction's roll-back is asked before the
			// rename and the margin's after it, under the renamed name.
			what:     "a reverted direction and a reverted logical margin together",
			ua:       `p { direction: rtl; margin-right: 3px }`,
			author:   `#a { direction: ltr; margin-right: 9px } #a { direction: revert; margin-inline-start: revert }`,
			property: "margin-right",
			want:     "3px",
		},
		{
			what:     "an inline revert of a logical property",
			markup:   `<div id="p"><p id="a" style="margin-inline-start: revert">x</p></div>`,
			ua:       `p { margin-left: 3px }`,
			author:   `#a { margin-left: 9px }`,
			property: "margin-left",
			want:     "3px",
		},
	})
}

// TestARollbackOnAPseudoElement: a pseudo-element's declarations are cascaded
// the same way as an element's.
func TestARollbackOnAPseudoElement(t *testing.T) {
	ua := sheet(t, OriginUserAgent, `p::before { font-family: ua }`)
	au := author(t, `p::before { content: "x"; font-family: author } p::before { font-family: revert }`)
	doc := parseDoc(t, revertDoc)
	got := Apply(doc, []Sheet{ua, au})
	cs, ok := got.Pseudo[PseudoKey{Node: elementFor(t, doc, "#a"), Name: "before"}]
	if !ok {
		t.Fatal("the ::before has no style")
	}
	if f := cs.Get("font-family"); f != "ua" {
		t.Errorf("::before's font-family is %q, want the user agent's", f)
	}
}

// TestARollbackThroughACustomPropertyIsNotSubstituted. CSS Variables makes
// "var(--k)" with "--k: revert" the keyword, and this engine substitutes no
// custom property: the declaration computes to "unset", and the finding that
// says so stands.
func TestARollbackThroughACustomPropertyIsNotSubstituted(t *testing.T) {
	got, findings := reverted(t, revertDoc, `p { font-family: ua }`, "",
		`#a { --k: revert; font-family: var(--k) }`, "font-family")
	if got != "parent" {
		t.Errorf("font-family is %q; a var() computes to unset here", got)
	}
	said := false
	for _, f := range findings {
		if f.Unsupported && strings.Contains(f.Message, "custom property") {
			said = true
		}
	}
	if !said {
		t.Errorf("nothing says the var() was not substituted: %v", findings)
	}
}

// TestARollbackIsLinearInTheLayersItCrosses. Each revert-layer removes one
// layer and the walk goes on from where it was, so k layers rolling back in
// turn cost one pass over them. Picking the winner again from the top after
// each removal, which is the obvious way to write it, is k passes.
//
// Timed, because the walk allocates nothing per step that a quadratic one would
// allocate more of. The list is built outside the measurement and kept to a few
// hundred kilobytes; see costtest.
func TestARollbackIsLinearInTheLayersItCrosses(t *testing.T) {
	list := func(k int) []candidate {
		out := make([]candidate, 0, k)
		for i := k; i >= 1; i-- {
			text := kwRevertLayer
			if i == 1 {
				text = "bottom"
			}
			out = append(out, candidate{property: "font-family", text: text,
				origin: OriginAuthor, layer: i, order: i})
		}
		return out
	}
	walk := func(k int) func() {
		l := list(k)
		return func() {
			if got, _ := rollBack(l, preparedDecl{}, false); got != "bottom" {
				panic("the roll-back did not reach the bottom layer: " + got)
			}
		}
	}
	c := costtest.Time(t, "revert-layer through every layer", walk(500), walk(2000))
	if c.Ratio > 8 {
		t.Errorf("rolling back through 500 layers took %v and 2000 took %v, a "+
			"factor of %.1f: one walk is four, and a pick per layer sixteen",
			c.Small, c.Large, c.Ratio)
	}
}

// TestARollbackThroughEveryLayerOfADocument is the same at the size a document
// reaches it, through Apply: every layer is still crossed, and the answer is
// the first.
func TestARollbackThroughEveryLayerOfADocument(t *testing.T) {
	var b strings.Builder
	b.WriteString(`@layer l0 { #a { font-family: bottom } }`)
	for i := 1; i < 300; i++ {
		b.WriteString(` @layer l` + strconv.Itoa(i) + ` { #a { font-family: revert-layer } }`)
	}
	rules, errs := css.ParseStylesheet(b.String())
	if len(errs) != 0 {
		t.Fatalf("the sheet reported %v", errs)
	}
	doc := parseDoc(t, revertDoc)
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	if f := got.Styles[elementFor(t, doc, "#a")].Get("font-family"); f != "bottom" {
		t.Errorf("font-family is %q, want the bottom layer's", f)
	}
}
