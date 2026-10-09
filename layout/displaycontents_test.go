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
	if got := a.Style.Get("color"); got != "rgb(0,0,255)" {
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

// TestDisplayContentsOnAnUnusualElementIsNone is css-display-3's Appendix B:
// on a replaced element or a form control, "display: contents computes to
// display: none". The cascade gives the element that value (see
// style.unusualDisplayContents), and what this pins is what layout makes of
// it: no box for the element, nothing of what it holds, and nothing reported,
// since the value was applied as the specification says it applies.
//
// The replaced elements and the controls used to keep an inline box and a
// finding that the value was not implemented — the picture or the widget drawn
// where the author had said there was no box. And a MathML element was the
// other way about: neither replaced nor a control to the predicate that
// decided, so the value was honoured on it and its <mi> stood on the page as a
// block of its own, outside any formula. The Appendix's answer is none for
// both: "For all MathML elements, display: contents computes to display:
// none."
func TestDisplayContentsOnAnUnusualElementIsNone(t *testing.T) {
	for _, src := range []string{
		`<img id="e" src="x.png" style="display: contents">`,
		`<input id="e" value="gone" style="display: contents">`,
		`<input id="e" type="submit" value="gone" style="display: contents">`,
		`<textarea id="e" style="display: contents">gone</textarea>`,
		`<select id="e" style="display: contents"><option>gone</option></select>`,
		`<canvas id="e" style="display: contents"><p>gone</p></canvas>`,
		`<video id="e" style="display: contents">gone</video>`,
		`<object id="e" style="display: contents">gone</object>`,
		`<iframe id="e" style="display: contents"></iframe>`,
		`<svg id="e" style="display: contents"><text>gone</text></svg>`,
		`<math id="e" style="display: contents"><mi>gone</mi></math>`,
		`<math><mrow id="e" style="display: contents"><mi>gone</mi></mrow></math>`,
		`kept<br id="e" style="display: contents">kept`,
	} {
		got := bodyBoxes(t, `<div>kept`+src+`</div>`)
		if strings.Contains(got, "gone") {
			t.Errorf("%s: what the element holds reached the page:\n%s", src, got)
		}
		if !strings.Contains(got, "kept") {
			t.Errorf("%s: the element's siblings were lost:\n%s", src, got)
		}
		built := build(t, `<div>kept`+src+`</div>`)
		var walk func(*Box)
		walk = func(b *Box) {
			if b.Element != nil {
				if id, _ := b.Element.Attr("id"); id == "e" {
					t.Errorf("%s: the element has a box:\n%s", src, sketchBox(built.Root))
				}
			}
			for _, c := range b.Children {
				walk(c)
			}
		}
		walk(built.Root)
		if got := displayFindings(t, src); len(got) != 0 {
			t.Errorf("%s was reported as %q; the value was applied", src, got[0].Message)
		}
	}

	// A <br> is the one whose absence shows without content: with no box there
	// is no forced break, so the two words share a line as they would under
	// "display: none".
	got := bodyBoxes(t, `<div>a<br style="display: contents">b</div>`)
	if strings.Contains(got, "br ") {
		t.Errorf("the <br> has a box, which is a forced break:\n%s", got)
	}
}

// TestDisplayContentsOnAButtonIsHonoured. The Appendix names <button>,
// <details> and <fieldset> to say they are not unusual: "display: contents
// simply removes their principal box, and their contents render as normal".
// And a <legend> "reacts to display: contents normally". A <button> was
// refused with the form controls before, because it is one to the engine.
func TestDisplayContentsOnAButtonIsHonoured(t *testing.T) {
	for _, src := range []string{
		`<button id="e" style="display: contents">kept</button>`,
		`<fieldset id="e" style="display: contents"><legend>kept</legend></fieldset>`,
		`<fieldset><legend id="e" style="display: contents">kept</legend></fieldset>`,
	} {
		built := build(t, `<div>`+src+`</div>`)
		if !strings.Contains(textOfTree(built.Root), "kept") {
			t.Errorf("%s: the contents were lost:\n%s", src, sketchBox(built.Root))
		}
		var walk func(*Box)
		walk = func(b *Box) {
			if b.Element != nil {
				if id, _ := b.Element.Attr("id"); id == "e" {
					t.Errorf("%s: the element kept its box:\n%s", src, sketchBox(built.Root))
				}
			}
			for _, c := range b.Children {
				walk(c)
			}
		}
		walk(built.Root)
		if got := displayFindings(t, src); len(got) != 0 {
			t.Errorf("%s was reported as %q; the value was applied", src, got[0].Message)
		}
	}
	// No control is drawn for a button that has no box: its text is the
	// enclosing block's.
	if b := boxFor(build(t, `<button style="display: contents">x</button>`).Root,
		"button"); b != nil {
		t.Errorf("a contents button has a box")
	}
}

// TestDisplayContentsOnTheRootIsBlock is §2.8: "a display of contents
// computes to block on the root element". The root used to keep the inline
// box the value read as everywhere, and a finding.
func TestDisplayContentsOnTheRootIsBlock(t *testing.T) {
	built := build(t, `<p>x</p>`, `html { display: contents }`)
	if built.Root == nil || built.Root.Element == nil || built.Root.Element.Name != "html" {
		t.Fatalf("the root has no box of its own:\n%s", sketchBox(built.Root))
	}
	if built.Root.Outer != OuterBlock || built.Root.Inner != InnerFlow {
		t.Errorf("the root is %v/%v, want block/flow", built.Root.Outer, built.Root.Inner)
	}
	if got := displayFindings(t, `<p>x</p>`, `html { display: contents }`); len(got) != 0 {
		t.Errorf("the root was reported as %q; the value was applied", got[0].Message)
	}
}

// TestContentsAroundAndInsideUnusualElements: the value is the element's, and
// changes nothing about the elements around it or inside it. A contents list
// item is still none of its own; an <img> inside a contents element still has
// its box; and a table part inside a contents element still finds its table,
// because the contents element is not there for §17.2.1 to wrap.
func TestContentsAroundAndInsideUnusualElements(t *testing.T) {
	built := build(t, `<div style="display: contents"><img id="i" src="x.png"></div>`)
	if b := boxWithID(t, built.Root, "i"); b == nil {
		t.Errorf("an <img> inside a contents element lost its box")
	}

	got := bodyBoxes(t, `<table><tbody><tr id="r"><td>a</td>`+
		`<td style="display: contents">b</td></tr></tbody></table>`)
	// The cell is gone and its text is a run inside a row, which §17.2.1
	// wraps in an anonymous cell of its own.
	if strings.Count(got, "td block/table-cell") != 1 ||
		!strings.Contains(got, "anonymous block/table-cell") || !strings.Contains(got, `"b"`) {
		t.Errorf("the contents cell kept its box, or its text no anonymous cell:\n%s", got)
	}
	got = bodyBoxes(t, `<table><tbody style="display: contents"><tr><td>a</td></tr>`+
		`</tbody></table>`)
	if strings.Contains(got, "tbody") || !strings.Contains(got, "tr block/table-row") {
		t.Errorf("a contents row group kept its box or lost its row:\n%s", got)
	}

	// A list whose items are images is the list-item case of the rule: the
	// image is none, the item around it is untouched.
	got = bodyBoxes(t, `<ul><li>a<img src="x.png" style="display: contents"></li></ul>`)
	if !strings.Contains(got, "li block list-item") {
		t.Errorf("the list item around a contents image changed:\n%s", got)
	}
	if strings.Contains(got, "img") {
		t.Errorf("the contents image has a box:\n%s", got)
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
