package layout

import (
	"reflect"
	"testing"
)

// Options.Reported: elements the backend asked to be told about. Each match
// is laid out and painted like any box, and also reported as a Component —
// its border box, its id. See Component and Options.Reported.

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

func TestAReportedElementIsStillPainted(t *testing.T) {
	html := `<div id="clock" style="display:block; width: 100px; height: 50px; background: red; border: 2px solid blue"><p>words</p></div>`
	plain := Compose(Input{HTML: html}, Options{}).Ops
	reported := Compose(Input{HTML: html}, Options{Reported: "#clock"}).Ops
	if cs := componentsIn(reported); len(cs) != 1 {
		t.Fatalf("%d components, want 1: %+v", len(cs), cs)
	}
	var rest []Op
	for _, op := range reported {
		if _, ok := op.(Component); !ok {
			rest = append(rest, op)
		}
	}
	if len(rest) != len(plain) {
		t.Fatalf("%d ops besides the component, want the %d painted without a selector", len(rest), len(plain))
	}
	for i := range plain {
		if !reflect.DeepEqual(rest[i], plain[i]) {
			t.Errorf("op %d is %+v, want %+v as painted without a selector", i, rest[i], plain[i])
		}
	}
}

func TestAGeneratedBoxIsNotReportedForItsElement(t *testing.T) {
	out := Compose(Input{HTML: `<style>#clock::before { content: "x"; display: block }
		#clock::after { content: "y"; display: block }</style>
		<div id="clock" style="display:block; width: 100px; height: 50px"></div>`},
		Options{Reported: "#clock"})
	cs := componentsIn(out.Ops)
	if len(cs) != 1 {
		t.Fatalf("%d components, want only the element's own box: %+v", len(cs), cs)
	}
	if h := cs[0].Rect.H.Px(); h != 50 {
		t.Errorf("the component is %vpx tall, want the element's 50px", h)
	}
}
