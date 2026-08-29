package layout

import (
	"strings"
	"testing"
)

// css-display-3 §3.1's "display: contents".
//
//	The element itself does not generate any boxes, but its children and
//	pseudo-elements still generate boxes and text runs as normal.
//
// It was read as "inline", which is the closest available answer and is wrong in
// a way that shows: an inline box takes part in layout, so its own padding,
// border and background were drawn, and its boundary broke shaping,
// letter-spacing and §8.1's ideograph spacing — every one of which the author
// asked against by writing the value.

// displayFindings returns what was said about display in a document.
func displayFindings(t *testing.T, htmlSrc string, cssSrc ...string) []Finding {
	t.Helper()
	var out []Finding
	for _, f := range build(t, htmlSrc, cssSrc...).Findings {
		if f.Rule == RuleUnsupportedValue && f.Property == "display" {
			out = append(out, f)
		}
	}
	return out
}

// TestAContentsElementGeneratesNoBox is the rule, stated over the tree.
//
// The content is inline on purpose. A block inside an inline box is cut out of
// it by §9.2.1.1 whatever this does, so the two answers agree there and the
// fixture would say nothing; an inline box around inline content stays, and is
// the box the author asked not to have.
func TestAContentsElementGeneratesNoBox(t *testing.T) {
	got := bodyBoxes(t, `<div id="d">a<span style="display: contents">b</span>c</div>`)
	if strings.Contains(got, "span") {
		t.Errorf("the box tree holds a box for the element:\n%s", got)
	}
	if !strings.Contains(got, `text "b"`) {
		t.Errorf("the element's text went with its box:\n%s", got)
	}
}

// TestAContentsElementsChildrenBelongToItsParent. "No box" is not "no place":
// the children stand where the element stood, so a <p> inside one is a child of
// whatever the element was a child of — which is what makes a contents element
// around a table row work, and what makes one around a block not generate an
// anonymous inline wrapper.
func TestAContentsElementsChildrenBelongToItsParent(t *testing.T) {
	built := build(t, `<div id="outer">a<span style="display: contents">b</span>c</div>`)
	outer := boxWithID(t, built.Root, "outer")
	var kids []string
	for _, c := range outer.Children {
		if c.IsText() {
			kids = append(kids, c.Text)
		}
	}
	if len(kids) != 3 || kids[0] != "a" || kids[1] != "b" || kids[2] != "c" {
		t.Errorf("the outer box's text children are %q, want the three runs in "+
			"order — the middle one belongs to this box now", kids)
	}

	// A list item is the case where it is most visible: the element is gone, so
	// the marker it would have generated is gone with it, and the text stands
	// in the list.
	got := bodyBoxes(t, `<ul><li style="display: contents">x</li></ul>`)
	if strings.Contains(got, "li") || strings.Contains(got, "list-item") {
		t.Errorf("a contents list item kept its box or its marker:\n%s", got)
	}
}

// TestAContentsElementStillStylesItsChildren, which is the half "no box" does
// not touch: the element is still in the element tree, still cascades, and is
// still what its children inherit from. An implementation that dropped the
// element rather than its box would lose all of that.
func TestAContentsElementStillStylesItsChildren(t *testing.T) {
	built := build(t, `<div id="outer"><div id="wrap"><p id="a">a</p></div></div>`,
		noDefaults+`#outer { font-size: 10px } `+
			`#wrap { display: contents; color: rgb(0,0,255); font-size: 40px }`)
	a := boxWithID(t, built.Root, "a")
	if got := a.Style["color"]; got != "rgb(0,0,255)" {
		t.Errorf("the child's colour is %q; an inherited property comes from the "+
			"element, whether or not it has a box", got)
	}
	// The font size the box carries is the element's own and not its parent's.
	// It is the number an em means inside it, and a build that stood the
	// children up against the grandparent would give them 10.
	if got := a.FontSize.Px(); got != 40 {
		t.Errorf("the child box's font size is %gpx, want 40 — the size the "+
			"element declared, which is what an em inside it means", got)
	}

	// And its text is processed under its own white space and text-transform,
	// which are the properties a text box takes from the style handed to it
	// rather than from the cascade.
	got := bodyBoxes(t, `<div id="d">a<span id="wrap">b c</span></div>`,
		noDefaults+`#wrap { display: contents; text-transform: uppercase }`)
	if !strings.Contains(got, `"B C"`) {
		t.Errorf("the text inside a contents element was not transformed by it:\n%s",
			got)
	}
}

// TestAContentsElementStillGeneratesItsPseudoElements. The specification says
// so in as many words, and it is the clause that makes "no box" a statement
// about this element rather than about anything it holds.
func TestAContentsElementStillGeneratesItsPseudoElements(t *testing.T) {
	got := bodyBoxes(t, `<div id="wrap">middle</div>`,
		noDefaults+`#wrap { display: contents } `+
			`#wrap::before { content: "[" } #wrap::after { content: "]" }`)
	if !strings.Contains(got, "[") || !strings.Contains(got, "]") {
		t.Errorf("a contents element's pseudo-elements did not reach the tree:\n%s", got)
	}
}

// TestNestedContentsElementsCollapseTogether: one such element inside another
// is two elements with no boxes, not a box holding a box.
func TestNestedContentsElementsCollapseTogether(t *testing.T) {
	got := bodyBoxes(t, `<div style="display: contents">`+
		`<div style="display: contents"><p>x</p></div></div>`)
	if strings.Contains(got, "div") {
		t.Errorf("the tree holds a box for one of the two:\n%s", got)
	}
	if !strings.Contains(got, "p block") {
		t.Errorf("the paragraph was lost:\n%s", got)
	}
}

// TestAContentsElementPaintsNothingOfItsOwn is the visible difference, and the
// reason this was worth implementing rather than approximating.
//
// Read as "inline", the element had a box, and a box with a background paints
// one. The author wrote "display: contents" to say the element is not there.
func TestAContentsElementPaintsNothingOfItsOwn(t *testing.T) {
	ops := paintOf(t, `<div id="d"><span id="wrap">x</span></div>`,
		noDefaults+`#wrap { display: contents; background-color: rgb(0,0,255); `+
			`padding: 20px; border: 5px solid rgb(0,0,255) }`)
	if got := fillsOf(ops, blue); len(got) != 0 {
		t.Errorf("a contents element painted %v; it has no box, so it has no "+
			"background and no border", got)
	}
}

// TestDisplayContentsIsSilentWhereItIsHonoured. The guardrail and the
// implementation are two halves of one claim, and a finding left behind on a
// declaration that *was* applied is a caller told their page is wrong when it is
// right.
func TestDisplayContentsIsSilentWhereItIsHonoured(t *testing.T) {
	for _, src := range []string{
		`<div style="display: contents"><p>x</p></div>`,
		`<span style="display: contents">x</span>`,
		`<div style="display: contents"></div>`,
		`<ul><li style="display: contents">x</li></ul>`,
	} {
		if got := displayFindings(t, src); len(got) != 0 {
			t.Errorf("%s was reported as %q; the value was applied", src, got[0].Message)
		}
	}
}

// TestDisplayContentsIsNotHonouredOnAnUnusualElement is the containment half.
//
// css-display-3's appendix on unusual elements: an element whose layout is not
// decided by CSS box generation has no contents to be replaced by. A replaced
// element's content is a picture and a form control's is a widget the engine
// draws, and neither is a subtree of boxes — so the declaration cannot be
// honoured, and going quiet about it would be the silent wrongness the finding
// exists to prevent.
func TestDisplayContentsIsNotHonouredOnAnUnusualElement(t *testing.T) {
	for _, src := range []string{
		`<img src="x.png" style="display: contents">`,
		`<input style="display: contents">`,
		`<textarea style="display: contents"></textarea>`,
		`<select style="display: contents"><option>a</option></select>`,
	} {
		if got := displayFindings(t, src); len(got) != 1 {
			t.Errorf("%s produced %d findings, want one — the value is not applied "+
				"to it and nothing else about the page says so", src, len(got))
		}
	}
	// The root is the other one, and is not an exception this engine chose:
	// §2.7 blockifies the root element, so the value never reaches it.
	if got := displayFindings(t, `<p>x</p>`, `html { display: contents }`); len(got) != 1 {
		t.Errorf("display:contents on the root produced %d findings, want one", len(got))
	}
}

// boxWithID finds the box an element with a given id generated. boxFor beside it
// searches by element name, which these cannot use: the fixtures below set the
// same element name twice on purpose.
func boxWithID(t *testing.T, root *Box, id string) *Box {
	t.Helper()
	var found *Box
	var walk func(*Box)
	walk = func(b *Box) {
		if found != nil || b == nil {
			return
		}
		if b.Element != nil {
			if got, _ := b.Element.Attr("id"); got == id {
				found = b
				return
			}
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("no box for #%s", id)
	}
	return found
}
