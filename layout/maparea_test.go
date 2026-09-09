package layout

import (
	"strings"
	"testing"
)

// <map> and <area>, which are markup about where a reader may click.
//
// The image map itself is refused and always will be: nothing here turns a
// rectangle into a link, and this engine's pages are not clicked. What was
// refused with it was the *box*, and that is a different question — the same one
// an <iframe>, a form control and a <canvas> were each wrongly answered on.
//
// A <map> is an ordinary inline box holding whatever the author put in it. An
// <area> is hidden by HTML's own rendering section rather than by anything this
// engine decided, which makes it a rule a stylesheet may overrule.

// TestAnAreaIsHiddenByARuleAndNotByARefusal.
//
// The default is nothing on the page, which is what HTML asks for. The
// difference from a refusal is that a stylesheet can take it back — and when it
// does, everything an ordinary box has comes with it, generated content
// included. CSS2/generated-content/content-100 is that document: an <area>
// blockified by the author with a ":before" that prints one of its attributes.
func TestAnAreaIsHiddenByARuleAndNotByARefusal(t *testing.T) {
	hidden := Build(Input{HTML: `<map><area alt="" nohref="nohref"></map>`})
	if hidden.Root == nil {
		t.Fatal("the document produced no boxes at all")
	}
	if boxFor(hidden.Root, "area") != nil {
		t.Error("an <area> with nothing said about it generated a box; HTML's " +
			"rendering section hides one")
	}

	shown := Build(Input{HTML: `<map><area id="a" alt="" nohref="nohref"></map>`,
		CSS: []Stylesheet{{Source: `area { display: block } area:before { content: attr(nohref) }`}}})
	b := boxFor(shown.Root, "area")
	if b == nil {
		t.Fatal("\"area { display: block }\" generated no box; the element is " +
			"hidden by a rule, and a rule is something a stylesheet may overrule")
	}
	if got := textOfTree(shown.Root); !strings.Contains(got, "nohref") {
		t.Errorf("the generated content is not on the page: %q", got)
	}
}

// TestAMapIsAnOrdinaryInlineBox. Nothing about a map is special to layout: it is
// a container, and what it contains belongs on the page whether or not any of it
// can be clicked.
func TestAMapIsAnOrdinaryInlineBox(t *testing.T) {
	built := Build(Input{HTML: `<p>a<map id="m">inside</map>b</p>`})
	if boxFor(built.Root, "map") == nil {
		t.Fatal("a <map> generated no box")
	}
	if got := textOfTree(built.Root); !strings.Contains(got, "inside") {
		t.Errorf("what the map contained is not on the page: %q", got)
	}
}

// TestNeitherIsReportedAsUnsupported is the other half of the boundary. A link
// is not something paper has, and this engine says nothing about an <a href>
// either — so an image map is not a page missing something a reader would have
// seen, and a finding on one would be noise in the list that is meant to name
// real losses.
func TestNeitherIsReportedAsUnsupported(t *testing.T) {
	built := Build(Input{HTML: `<map name="m"><area shape="rect" coords="0,0,1,1" href="x"></map>`})
	for _, f := range built.Findings {
		if f.Unsupported() {
			t.Errorf("an image map was reported as unsupported: %s", f.Error())
		}
	}
}
