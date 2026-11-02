package layout

import (
	"testing"
)

// Options.Reported: elements the backend asked to be told about. Each match
// is laid out like any box and reported as a Component — its border box, its
// id — and nothing is drawn for it. See Component and Options.Reported.

// componentsIn is the Components of a display list, in order.
func componentsIn(ops []Op) []Component {
	var out []Component
	for _, op := range ops {
		if c, ok := op.(Component); ok {
			out = append(out, c)
		}
	}
	return out
}

func TestAReportedElementIsItsBorderBox(t *testing.T) {
	out := Compose(Input{HTML: `<body><p>before</p>
		<div id="clock" style="display:block; margin: 30px 20px; width: 100px; height: 50px"></div>
		<p>after</p></body>`}, Options{Reported: "#clock"})
	cs := componentsIn(out.Ops)
	if len(cs) != 1 {
		t.Fatalf("%d components in the display list, want 1: %+v", len(cs), cs)
	}
	c := cs[0]
	if c.Name != "clock" {
		t.Errorf("the component is named %q, want the element's id", c.Name)
	}
	if w, h := c.Rect.W.Px(), c.Rect.H.Px(); w != 100 || h != 50 {
		t.Errorf("the component's box is %vx%v, want 100x50", w, h)
	}
}

func TestNothingIsReportedWithoutASelector(t *testing.T) {
	html := `<div id="clock" style="display:block; width: 100px; height: 50px"></div>`
	if cs := componentsIn(Compose(Input{HTML: html}, Options{}).Ops); len(cs) != 0 {
		t.Errorf("%d components with no Reported selector, want 0", len(cs))
	}
	if cs := componentsIn(Compose(Input{HTML: html}, Options{Reported: ""}).Ops); len(cs) != 0 {
		t.Errorf("%d components with an empty Reported selector, want 0", len(cs))
	}
}

func TestAnElementTheSelectorDoesNotSelectIsNotReported(t *testing.T) {
	out := Compose(Input{HTML: `<div id="clock" style="display:block; width: 100px; height: 50px"></div>
		<p>words</p>`}, Options{Reported: "#clock, .notthere"})
	cs := componentsIn(out.Ops)
	if len(cs) != 1 || cs[0].Name != "clock" {
		t.Errorf("got %+v, want only the #clock div", cs)
	}
	if cs := componentsIn(Compose(Input{HTML: `<p>words</p>`}, Options{Reported: "#clock"}).Ops); len(cs) != 0 {
		t.Errorf("%d components for a selector that matches nothing, want 0", len(cs))
	}
}

func TestAReportedElementKeepsItsNameWhenItHasNoId(t *testing.T) {
	out := Compose(Input{HTML: `<div name="gauge" style="display:block; width: 100px; height: 50px"></div>`},
		Options{Reported: "div"})
	cs := componentsIn(out.Ops)
	if len(cs) != 1 {
		t.Fatalf("%d components, want 1", len(cs))
	}
	if cs[0].Name != "gauge" {
		t.Errorf("the component is named %q, want the element's name attribute", cs[0].Name)
	}
}

func TestTwoReportedElementsAreTwoComponentsInPaintOrder(t *testing.T) {
	out := Compose(Input{HTML: `<div id="a" style="display:block; width: 100px; height: 50px"></div>
		<div id="b" style="display:block; width: 100px; height: 50px"></div>`},
		Options{Reported: "div"})
	cs := componentsIn(out.Ops)
	if len(cs) != 2 || cs[0].Name != "a" || cs[1].Name != "b" {
		t.Errorf("got %+v, want a then b", cs)
	}
}

func TestAReportedSelectorThatDoesNotParseIsReportedAndReportsNothing(t *testing.T) {
	out := Compose(Input{HTML: `<div id="clock" style="display:block; width: 100px; height: 50px"></div>`},
		Options{Reported: "div >> {"})
	if cs := componentsIn(out.Ops); len(cs) != 0 {
		t.Errorf("%d components with a malformed selector, want 0", len(cs))
	}
	found := false
	for _, f := range out.Findings {
		if f.Rule == RuleInvalidCSS {
			found = true
		}
	}
	if !found {
		t.Error("no finding about the malformed Reported selector")
	}
}
