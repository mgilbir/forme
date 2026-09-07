package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// The four CSS-wide keywords, and the two places that knew about three of them.
//
// "revert-layer" is CSS Cascade 5's and was not recognised at all, so a
// declaration using it was read as a value of the property and dropped for not
// being one — "color: revert-layer" left the colour an *earlier* declaration had
// set, which is the opposite of what it asks for. It rolls back to the previous
// cascade layer, and this engine has none: no @layer rule reaches it, so every
// declaration is in the implicit outer layer, and the specification's own answer
// for that case is that it behaves as "revert".
//
// The other place is inertness, which is in inert_test.go: a property this
// engine does not implement is reported only where the document asked for
// something other than what the page already shows, and the comparison is
// against the declared value — so a keyword standing for a value has to be
// resolved first. Only "initial" was.

// styledColor is the computed colour of the paragraph, and the findings raised.
func styledColor(t *testing.T, src string) (string, []Finding) {
	t.Helper()
	rules, errs := css.ParseStylesheet(src)
	if len(errs) > 0 {
		t.Fatalf("the fixture does not parse: %v", errs)
	}
	doc, _, _ := html.Parse(`<p>x</p>`)
	out := Apply(doc, []Sheet{{Rules: rules, Name: "a.css"}})
	var got string
	doc.Walk(func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Name == "p" {
			got = out.Styles[n]["color"]
		}
		return true
	})
	return got, out.Findings
}

// TestRevertLayerIsRevert.
func TestRevertLayerIsRevert(t *testing.T) {
	plain, _ := styledColor(t, "p { color: red }")
	if plain != "red" {
		t.Fatalf("the control gave %q, so the fixture is not doing what it says", plain)
	}

	revert, _ := styledColor(t, "p { color: red } p { color: revert }")
	layer, findings := styledColor(t, "p { color: red } p { color: revert-layer }")
	if layer != revert {
		t.Errorf("\"revert-layer\" gave %q and \"revert\" gave %q; with no cascade "+
			"layer to roll back to they are the same keyword", layer, revert)
	}
	if layer == plain {
		t.Errorf("\"revert-layer\" left the colour the earlier declaration set, " +
			"which is what happens when it is read as a colour and dropped")
	}
	// And it is reported by name rather than as a value that did not parse.
	named := false
	for _, f := range findings {
		if strings.Contains(f.Message, "revert-layer") && f.Unsupported {
			named = true
		}
	}
	if !named {
		t.Errorf("no finding names \"revert-layer\" as unimplemented: %v", findings)
	}
}

// TestAnOffsetSaysWhichSheetItIsIn.
//
// A document is styled by several sheets — its own, every <link>, and everything
// those @import — and a byte offset means nothing without the one it is into.
// The stage that turns these into the caller's findings cannot recover it: by
// then the sheets have been prepared into one ordered list of rules.
//
// So Finding carries the name, and this is what says it carries the *right* one:
// two sheets with a fault each, at offsets that are equal in one and different
// in the other, so a finding attributed to the wrong sheet cannot pass by
// looking plausible.
func TestAnOffsetSaysWhichSheetItIsIn(t *testing.T) {
	const (
		first  = "p { resize: both }"
		second = "q { color: red } q { resize: both }"
	)
	firstRules, _ := css.ParseStylesheet(first)
	secondRules, _ := css.ParseStylesheet(second)
	doc, _, _ := html.Parse(`<p>x</p><q>y</q>`)
	out := Apply(doc, []Sheet{
		{Rules: firstRules, Name: "a.css"},
		{Rules: secondRules, Name: "b.css"},
	})

	got := map[string]int{}
	for _, f := range out.Findings {
		if strings.Contains(f.Message, "resize") {
			got[f.Sheet] = f.Offset
		}
	}
	if len(got) != 2 {
		t.Fatalf("the two sheets gave %d attributed findings: %v", len(got), got)
	}
	if at, ok := got["a.css"]; !ok || first[at:at+len("resize")] != "resize" {
		t.Errorf("a.css's finding is at %d, where the sheet reads %q",
			at, snippet(first, at))
	}
	if at, ok := got["b.css"]; !ok || second[at:at+len("resize")] != "resize" {
		t.Errorf("b.css's finding is at %d, where the sheet reads %q",
			at, snippet(second, at))
	}
}

// snippet is a few bytes of a sheet from an offset, for a message.
func snippet(src string, at int) string {
	if at < 0 || at > len(src) {
		return "<outside the sheet>"
	}
	end := at + 12
	if end > len(src) {
		end = len(src)
	}
	return src[at:end]
}
