package style

import "testing"

// layeredColour applies just the sheet given, with no rule of its own.
//
// styledBy cannot be used here: it writes an unlayered "#target { color: blue }"
// as its baseline, and an unlayered declaration beats every layered one — so
// every case below would come out blue and the ones that pass would pass for
// the wrong reason. The baseline has to stay out of the cascade the test is
// about.
func layeredColour(t *testing.T, src string) (string, []Finding) {
	t.Helper()
	doc := parseDoc(t, `<p id="target">x</p>`)
	got := Apply(doc, []Sheet{author(t, src)})
	return got.Styles[doc.Element("p")].Get("color"), got.Findings
}

// TestALayerIsAppliedAndOrdered is the whole of CSS Cascade 5 §6.4 this engine
// can act on.
//
// Every rule inside an @layer used to be dropped. That is the worst of the ways
// to be wrong about a cascade feature: a stylesheet written in layers — which is
// how a framework ships one — lost not some of its precedence but all of its
// rules.
func TestALayerIsAppliedAndOrdered(t *testing.T) {
	for _, c := range []struct {
		what, sheet, want string
	}{
		{
			"a layered rule applies at all",
			`@layer base { #target { color: red } }`,
			"red",
		},
		{
			// The point of the feature: the later layer wins whatever the
			// selectors say, and here the earlier one is the more specific.
			"a later layer beats an earlier one, against specificity",
			`@layer base { p#target { color: red } } @layer theme { #target { color: green } }`,
			"green",
		},
		{
			// The statement form fixes the order before either block exists,
			// which is the reason it is worth having.
			"the statement form fixes the order",
			`@layer theme, base;
			 @layer base { #target { color: red } }
			 @layer theme { p#target { color: green } }`,
			"red",
		},
		{
			// Unlayered beats layered, which is the promise an author is making
			// when they put something outside the layers.
			"an unlayered rule beats a layered one",
			`@layer base { p#target { color: red } } #target { color: green }`,
			"green",
		},
		{
			// And it beats it from above as well as below: order does not come
			// into it, because the layer term is consulted first.
			"an unlayered rule written first still beats a layered one",
			`#target { color: green } @layer base { p#target { color: red } }`,
			"green",
		},
		{
			// A name reopened adds to the layer it already named rather than
			// making a new one, so the first mention fixes where it sits.
			"a layer reopened keeps its place",
			`@layer base { #target { color: red } }
			 @layer theme { #target { color: green } }
			 @layer base { #target { color: blue } }`,
			"green",
		},
		{
			// A sublayer's name is its parent's and its own, so it is a
			// different layer from a top-level one that happens to share the
			// last part. Without the path they collide: the inner block would
			// join the outer layer, and the more specific selector there would
			// decide instead of the layer order.
			"a sublayer does not collide with a top-level layer of the same name",
			`@layer x { p#target { color: green } }
			 @layer a { @layer x { #target { color: red } } }`,
			"red",
		},
		{
			// A layer inside a layer still applies, and against everything
			// outside its parent it is ordered with its parent. The order
			// within the parent is TestANestedLayerIsOrderedWithinItsParent.
			"a nested layer applies and is ordered after its parent",
			`@layer a { @layer x { #target { color: red } } }
			 @layer b { #target { color: green } }`,
			"green",
		},
		{
			// Importance reverses the layer order, exactly as it reverses the
			// origin order: the *earlier* layer wins.
			"important reverses the layer order",
			`@layer base { #target { color: red !important } }
			 @layer theme { #target { color: green !important } }`,
			"red",
		},
		{
			// And an important layered declaration beats an important unlayered
			// one, which is the other half of the same reversal.
			"important layered beats important unlayered",
			`#target { color: green !important } @layer base { #target { color: red !important } }`,
			"red",
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			got, _ := layeredColour(t, c.sheet)
			if got != c.want {
				t.Errorf("the colour came out %q, want %q\nsheet: %s", got, c.want, c.sheet)
			}
		})
	}
}

// TestALayerSaysNothingWhenItIsApplied.
//
// It was reported as an at-rule that is not applied. It is applied now, and a
// stylesheet that uses one is not carrying something unsupported.
func TestALayerSaysNothingWhenItIsApplied(t *testing.T) {
	for _, sheet := range []string{
		`@layer base { #target { color: red } }`,
		`@layer a, b;`,
		`@layer { #target { color: red } }`,
	} {
		_, findings := layeredColour(t, sheet)
		for _, f := range findings {
			if f.Property == "@layer" {
				t.Errorf("%s reported %q", sheet, f.Message)
			}
		}
	}
}

// TestAnAnonymousLayerIsItsOwnLayer.
//
// "@layer { }" has no name, so nothing can add to it later — two of them are
// two layers, and the second wins.
func TestAnAnonymousLayerIsItsOwnLayer(t *testing.T) {
	got, _ := layeredColour(t,
		`@layer { p#target { color: red } } @layer { #target { color: green } }`)
	if got != "green" {
		t.Errorf("the colour came out %q, want %q; two anonymous layers are two "+
			"layers and the later one wins", got, "green")
	}
}

// TestALayerBlockNamingTwoLayersIsDropped.
//
// A block belongs to one layer. Naming two is a parse error, and guessing which
// was meant would put the rules somewhere the author did not ask for.
func TestALayerBlockNamingTwoLayersIsDropped(t *testing.T) {
	got, findings := layeredColour(t, `@layer a, b { #target { color: red } }`)
	if got == "red" {
		t.Error("the rule was applied; a block naming two layers is dropped rather " +
			"than guessed at")
	}
	found := false
	for _, f := range findings {
		if f.Property == "@layer" {
			found = true
		}
	}
	if !found {
		t.Error("it was dropped without a word")
	}
}

// TestANestedLayerIsOrderedWithinItsParent is audit C114, and replaces the test
// that stated the flat ordering as a decision. Layers are a tree (Cascade 5
// §6.4.3): a sublayer is ordered among its siblings inside its parent, the
// parent's own rules come after all of its sublayers, and "a.b" is the same
// layer whether it is written dotted or nested.
func TestANestedLayerIsOrderedWithinItsParent(t *testing.T) {
	for _, c := range []struct{ what, sheet, want string }{
		{"the order fixed up front, nested",
			`@layer framework, app;
			 @layer app { #target { color: green } }
			 @layer framework { @layer base { #target { color: red } } }`, "green"},
		{"the order fixed up front, dotted",
			`@layer framework, app;
			 @layer framework.base { #target { color: red } }
			 @layer app { #target { color: green } }`, "green"},
		{"a statement naming a dotted layer",
			`@layer framework.base, app;
			 @layer app { #target { color: green } }
			 @layer framework.base { #target { color: red } }`, "green"},
		{"the parent's own rules beat its sublayers'",
			`@layer a { #target { color: green } @layer b { p#target { color: red } } }`, "green"},
		{"and the same written dotted",
			`@layer a.b { p#target { color: red } } @layer a { #target { color: green } }`, "green"},
		{"siblings in the order they were first named in their parent",
			`@layer a.y, a.x; @layer a.x { #target { color: green } } @layer a.y { #target { color: red } }`,
			"green"},
		{"important reverses it: the sublayer's important beats the parent's",
			`@layer a { #target { color: red !important } @layer b { #target { color: green !important } } }`,
			"green"},
		{"a layer given a sublayer after its rules were read",
			`@layer a { #target { color: green } } @layer b { #target { color: red } } @layer a.x;`,
			"red"},
		{"an anonymous sublayer is under its parent's own rules",
			`@layer a { @layer { p#target { color: red } } #target { color: green } }`, "green"},
		{"one name in two parents is two layers",
			`@layer a { @layer x { #target { color: red } } } @layer x { #target { color: green } }`,
			"green"},
	} {
		t.Run(c.what, func(t *testing.T) {
			got, findings := layeredColour(t, c.sheet)
			if got != c.want {
				t.Errorf("the colour came out %q, want %q\nsheet: %s", got, c.want, c.sheet)
			}
			for _, f := range findings {
				if f.Property == "@layer" {
					t.Errorf("a nested layer is ordered properly and said so: %q", f.Message)
				}
			}
		})
	}
}

// TestALayerThatIsNotNestedSaysNothingAboutNesting. A nested layer used to be
// reported as ordered flatly, and this kept that report off every stylesheet
// that used a layer at all. Nothing is reported for either now; it stays so
// that a report about layers, if one returns, is not raised on a sheet that
// has nothing to report.
func TestALayerThatIsNotNestedSaysNothingAboutNesting(t *testing.T) {
	_, findings := layeredColour(t,
		`@layer base, theme; @layer base { #target { color: red } } @layer theme { #target { color: green } }`)
	for _, f := range findings {
		if f.Property == "@layer" {
			t.Errorf("a stylesheet with no nested layer reported %q", f.Message)
		}
	}
}

// TestTheLayerTreeIsATree checks the shape finishLayers walks rather than the
// colours it produces, because the failure it guards against is not a wrong
// colour: the root is made on first use, and a first layer numbered before the
// root existed became the root itself, a child of its own. The flattening walk
// then followed that edge for ever and the process ran out of memory — on
// "@layer { }", the anonymous form, which is the one sheet that makes a layer
// before naming anything. Every layer must have exactly one parent and the root
// none, whichever form of @layer comes first.
func TestTheLayerTreeIsATree(t *testing.T) {
	for _, src := range []string{
		`@layer { #target { color: red } }`,
		`@layer { @layer { #target { color: red } } } @layer { }`,
		`@layer a { #target { color: red } }`,
		`@layer a.b.c, d;`,
		`@layer a { @layer { } @layer b.c { } } @layer a.b.d;`,
	} {
		s := &Styler{matcher: NewMatcher(parseDoc(t, `<p id="target">x</p>`)),
			seen: map[string]bool{}, attrOffset: -1}
		s.prepare([]Sheet{author(t, src)})
		if len(s.layers) < 2 {
			t.Errorf("%s: made %d layer nodes; it declares at least one layer", src, len(s.layers))
			continue
		}
		parents := make([]int, len(s.layers))
		for id, node := range s.layers {
			for _, child := range node.children {
				if child == 0 {
					t.Errorf("%s: layer %d has the root as a child", src, id)
				}
				parents[child]++
			}
		}
		for id, n := range parents {
			if id != 0 && n != 1 {
				t.Errorf("%s: layer %d has %d parents, want 1", src, id, n)
			}
		}
	}
}
