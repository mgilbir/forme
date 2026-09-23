package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
)

// TestTheWritingModeIsResolvedAsTheDirectionIs: the element's own, from the
// cascade and its inline style and inheritance, the way every property is — so
// a logical margin inside a vertical container lands where the container's
// lines put it (audit C40).
func TestTheWritingModeIsResolvedAsTheDirectionIs(t *testing.T) {
	rules, errs := css.ParseStylesheet(`#outer { writing-mode: vertical-rl } ` +
		`#t { margin-inline-start: 5px; margin-block-start: 6px }`)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	doc := parseDoc(t, `<div id="outer"><p id="t">x</p>`+
		`<p id="own" style="writing-mode: horizontal-tb; margin-inline-start: 7px">y</p></div>`)
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	inherited := got.Styles[elementFor(t, doc, "#t")]
	if inherited.Get("margin-top") != "5px" || inherited.Get("margin-right") != "6px" {
		t.Errorf("inside vertical-rl the inline start is the top and the block start "+
			"the right: top=%q right=%q left=%q", inherited.Get("margin-top"),
			inherited.Get("margin-right"), inherited.Get("margin-left"))
	}
	own := got.Styles[elementFor(t, doc, "#own")]
	if own.Get("margin-left") != "7px" {
		t.Errorf("an element that sets its own writing mode is mapped by it: left=%q",
			own.Get("margin-left"))
	}
}

// TestTheEarlyDirectionIsTheCascadesDirection is audit C108. An important
// inline direction beats an important author one — the cascade's own rule,
// which the computed direction follows — and the logical mapping has to see the
// same answer.
func TestTheEarlyDirectionIsTheCascadesDirection(t *testing.T) {
	rules, errs := css.ParseStylesheet(`div { direction: ltr !important }`)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	doc := parseDoc(t, `<div id="t" style="direction: rtl !important; margin-inline-start: 10px">x</div>`)
	cs := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}}).Styles[elementFor(t, doc, "#t")]
	if cs.Get("direction") != "rtl" {
		t.Fatalf("the direction computed to %q, want rtl", cs.Get("direction"))
	}
	if cs.Get("margin-right") != "10px" || cs.Get("margin-left") == "10px" {
		t.Errorf("direction is rtl and the start margin went left: left=%q right=%q",
			cs.Get("margin-left"), cs.Get("margin-right"))
	}
}

// TestImportanceDecidesBetweenALogicalAndAPhysicalInlineDeclaration is audit
// C155: in one style attribute, importance before order, as between any two
// declarations there.
func TestImportanceDecidesBetweenALogicalAndAPhysicalInlineDeclaration(t *testing.T) {
	for _, tc := range []struct{ style, want string }{
		{"margin-left: 1px !important; margin-inline-start: 2px", "1px"},
		{"margin-inline-start: 2px !important; margin-left: 1px", "2px"},
		{"margin-left: 1px; margin-inline-start: 2px", "2px"},
		{"margin-inline-start: 2px; margin-left: 1px", "1px"},
	} {
		doc := parseDoc(t, `<div id="t" style="`+tc.style+`">x</div>`)
		cs := Apply(doc, nil).Styles[elementFor(t, doc, "#t")]
		if got := cs.Get("margin-left"); got != tc.want {
			t.Errorf("style=%q: margin-left %q, want %q", tc.style, got, tc.want)
		}
	}
}
