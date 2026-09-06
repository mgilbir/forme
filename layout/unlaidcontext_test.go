package layout

import (
	"strings"
	"testing"
)

// A formatting context this engine recognises and does not lay out.
//
// Ruby is the last of them. Flex and grid were here too, and each left when it
// was arranged: layout/flex.go and layout/grid.go report the containers they
// cannot arrange at the box, with the reason, which is a fact about the
// container rather than about the keyword. See layout/flex_test.go and
// layout/grid_test.go.
//
// Every test here asks *which boxes are reported*, not whether the finding
// exists. That is the whole difficulty: reporting every ruby box would be
// crying wolf on five of the suite's documents, and reporting none of them is
// the silence the finding is for.

// unlaidFindings returns the messages reported about display, in order.
func unlaidFindings(t *testing.T, htmlSrc, cssSrc string) []string {
	t.Helper()
	got := Compose(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}}, Options{})
	var out []string
	for _, f := range got.Findings {
		if f.Property == "display" {
			out = append(out, f.Message)
		}
	}
	return out
}

// TestARubyBoxIsReportedOnlyWhereThereIsAnAnnotation.
//
// Ruby lays a "ruby-text" above its base. With no annotation there is nothing to
// lift, and the base laid out inline is what ruby comes to — which is what five
// of the suite's text-autospace documents rely on: they write
// "display: ruby" on a span of plain text purely to make an element boundary.
func TestARubyBoxIsReportedOnlyWhereThereIsAnAnnotation(t *testing.T) {
	if got := unlaidFindings(t, `<div id="r">abc永</div>`, `#r { display: ruby }`); len(got) != 0 {
		t.Errorf("a ruby box with no annotation reported %v", got)
	}
	got := unlaidFindings(t,
		`<div id="r">漢<span id="a">かん</span></div>`,
		`#r { display: ruby } #a { display: ruby-text }`)
	if len(got) != 1 {
		t.Fatalf("a ruby box with an annotation reported %d findings, want 1: %v",
			len(got), got)
	}
	if !strings.Contains(got[0], "above") {
		t.Errorf("the finding %q does not say where the annotation went", got[0])
	}
	// Nested, not a child: the annotation may be wrapped, and HTML's own <ruby>
	// puts it beside the base rather than inside it.
	if n := len(unlaidFindings(t,
		`<div id="r">漢<span><span id="a">かん</span></span></div>`,
		`#r { display: ruby } #a { display: ruby-text }`)); n != 1 {
		t.Errorf("an annotation one level down reported %d findings, want 1", n)
	}
}

// TestOnlyTheContextsThisEngineDoesNotLayOutAreReported is the containment
// argument. The report must not widen: every one of these is laid out, and a
// finding on any of them would be a page called wrong that is right.
func TestOnlyTheContextsThisEngineDoesNotLayOutAreReported(t *testing.T) {
	// Two children *and* an annotation inside, so that a value wrongly routed
	// through either branch of unlaidBoxIsNotTheBoxAsked would be reported.
	// Without the annotation a table sent through the ruby branch is spared by
	// that branch rather than by the switch, and the check passes for a table
	// this engine had stopped laying out.
	const doc = `<div id="f"><div>a</div><div id="an">b</div></div>`
	const ann = ` #an { display: ruby-text }`
	for _, value := range []string{
		"block", "inline", "inline-block", "flow-root", "list-item",
		"table", "inline-table", "table-row", "table-cell", "none",
		"flex", "inline-flex", "grid", "inline-grid",
	} {
		if got := unlaidFindings(t, doc, `#f { display: `+value+` }`+ann); len(got) != 0 {
			t.Errorf("display: %s reported %v, and it is laid out", value, got)
		}
	}
}
