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
			// outside its parent it is ordered correctly. Where it is *not* is
			// within the parent — see TestANestedLayerIsOrderedFlatlyAndSaysSo.
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

// TestANestedLayerIsOrderedFlatlyAndSaysSo is the narrowing, stated as a test so
// that it is a decision rather than a gap somebody finds.
//
// A layer inside a layer is given its own place in the order at the point it is
// first seen, rather than a place inside its parent. The two agree for the way
// layers are usually written; they disagree for a document that fixes the order
// up front and fills it in afterwards, which is the fixture here — sorted within
// its parent, "framework.base" would sit under "framework" and lose to "app".
//
// The test asserts what this engine does *and* that it says so. If the ordering
// is ever implemented properly, this fails twice over and names what to change.
func TestANestedLayerIsOrderedFlatlyAndSaysSo(t *testing.T) {
	const sheet = `@layer framework, app;
		@layer app { #target { color: green } }
		@layer framework { @layer base { #target { color: red } } }`

	got, findings := layeredColour(t, sheet)
	if got != "red" {
		t.Errorf("the colour came out %q; this engine orders a nested layer as one "+
			"of its own, which puts framework.base last — if that has changed, "+
			"the note in layer.go and the finding below have changed with it", got)
	}
	found := false
	for _, f := range findings {
		if f.Property == "@layer" && f.Unsupported {
			found = true
		}
	}
	if !found {
		t.Error("nothing said the nesting was ordered flatly; a rule winning that " +
			"the author ordered to lose is not something to pass over in silence")
	}
}

// TestALayerThatIsNotNestedSaysNothingAboutNesting keeps the note above from
// being raised on every stylesheet that uses a layer at all.
func TestALayerThatIsNotNestedSaysNothingAboutNesting(t *testing.T) {
	_, findings := layeredColour(t,
		`@layer base, theme; @layer base { #target { color: red } } @layer theme { #target { color: green } }`)
	for _, f := range findings {
		if f.Property == "@layer" {
			t.Errorf("a stylesheet with no nested layer reported %q", f.Message)
		}
	}
}
