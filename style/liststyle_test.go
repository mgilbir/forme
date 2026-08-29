package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
)

// The "list-style" shorthand, and the two slots "none" can fill.
//
// "none" is a legal value of both list-style-type and list-style-image, and the
// grammar gives no way to say which is meant. The answer comes from the rest of
// the declaration, and where there is nothing left for it to be, the declaration
// is not a list-style at all and §4.2 drops it whole.
//
// CSS2/lists/list-style-020 is the fixture. It declares nine unresolvable ones
// in a row and asks for the inherited marker to survive every one.

// listStyleOf computes the three longhands of one declaration.
func listStyleOf(t *testing.T, decl string) (kind, position, image string) {
	t.Helper()
	rules, errs := css.ParseStylesheet(
		`#target { list-style-type: circle } #target { ` + decl + ` }`)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet %q reported %v", decl, errs)
	}
	doc := parseDoc(t, `<ul><li id="target">x</li></ul>`)
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	cs := got.Styles[elementFor(t, doc, "#target")]
	return cs["list-style-type"], cs["list-style-position"], cs["list-style-image"]
}

// TestWhichSlotTheNoneFills is list-style-020's first six rows.
func TestWhichSlotTheNoneFills(t *testing.T) {
	for _, tc := range []struct{ decl, kind, image string }{
		// The one an author writes: it suppresses the marker, and it takes both
		// slots to do that — leaving the type at "disc" would draw one.
		{"list-style: none", "none", "none"},
		{"list-style: none none", "none", "none"},
		// A type is given, so the none is the image.
		{"list-style: none square", "square", "none"},
		{"list-style: square none", "square", "none"},
		// An image is given, so the none is the type.
		{"list-style: url(d.png) none", "none", "url(d.png)"},
		{"list-style: none url(d.png)", "none", "url(d.png)"},
	} {
		kind, _, image := listStyleOf(t, tc.decl)
		if kind != tc.kind || image != tc.image {
			t.Errorf("%q gave type %q and image %q, want %q and %q",
				tc.decl, kind, image, tc.kind, tc.image)
		}
	}
}

// TestANoneWithNowhereToGoDropsTheDeclaration is list-style-020's seventh row,
// all nine of it. Each leaves the inherited "circle" standing.
func TestANoneWithNowhereToGoDropsTheDeclaration(t *testing.T) {
	for _, decl := range []string{
		"list-style: none url(r.png) none",
		"list-style: url(r.png) none none",
		"list-style: none none url(r.png)",
		"list-style: none square none",
		"list-style: square none none",
		"list-style: none none square",
		"list-style: square url(r.png) none",
		"list-style: url(r.png) none square",
		"list-style: none square url(r.png)",
	} {
		kind, _, image := listStyleOf(t, decl)
		if kind != "circle" || image != "none" {
			t.Errorf("%q gave type %q and image %q; it is not a list-style and the "+
				"earlier declaration should stand", decl, kind, image)
		}
	}
}

// TestTheOrdinaryFormsStillWork is the containment argument: the shorthand is
// written far more often without a "none" in it at all, and it still sets what
// it names and resets what it does not.
func TestTheOrdinaryFormsStillWork(t *testing.T) {
	for _, tc := range []struct{ decl, kind, position, image string }{
		{"list-style: square", "square", "outside", "none"},
		{"list-style: inside", "disc", "inside", "none"},
		{"list-style: square inside", "square", "inside", "none"},
		{"list-style: inside square", "square", "inside", "none"},
		{"list-style: url(d.png)", "disc", "outside", "url(d.png)"},
		{"list-style: square inside url(d.png)", "square", "inside", "url(d.png)"},
		{"list-style: decimal", "decimal", "outside", "none"},
	} {
		kind, position, image := listStyleOf(t, tc.decl)
		if kind != tc.kind || position != tc.position || image != tc.image {
			t.Errorf("%q gave %q/%q/%q, want %q/%q/%q", tc.decl,
				kind, position, image, tc.kind, tc.position, tc.image)
		}
	}
}

// TestARepeatedComponentIsNotAListStyle. Each of the three appears at most once,
// which is what "||" in the grammar means.
func TestARepeatedComponentIsNotAListStyle(t *testing.T) {
	for _, decl := range []string{
		"list-style: square circle",
		"list-style: inside outside",
		"list-style: url(a.png) url(b.png)",
	} {
		if kind, _, _ := listStyleOf(t, decl); kind != "circle" {
			t.Errorf("%q gave type %q; a component written twice is not a value",
				decl, kind)
		}
	}
}
