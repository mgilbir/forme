package layout

import (
	"strings"
	"testing"
)

// A formatting context this engine recognises and does not lay out.
//
// Every test here asks *which boxes are reported*, not whether the finding
// exists. That is the whole difficulty: reporting every flex, grid and ruby box
// would be crying wolf on six of the suite's documents, and reporting none of
// them is the silence the finding is for. The line between the two is the
// content, and each case below is one side of it.

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

// TestAFlexContainerWithItemsSaysItIsNotOne. Until this was reported, a flex
// container laid its children out as a column of full-width blocks and the
// document said nothing — the page was plausible, wrong, and had a clean bill of
// health.
func TestAFlexContainerWithItemsSaysItIsNotOne(t *testing.T) {
	for _, value := range []string{"flex", "inline-flex", "grid", "inline-grid"} {
		got := unlaidFindings(t, `<div id="f"><div>a</div><div>b</div></div>`,
			`#f { display: `+value+` }`)
		if len(got) != 1 {
			t.Errorf("display: %s reported %d findings, want 1: %v", value, len(got), got)
			continue
		}
		if !strings.Contains(got[0], value) {
			t.Errorf("the finding for display: %s is %q and does not name the value",
				value, got[0])
		}
		if !strings.Contains(got[0], "stacked") {
			t.Errorf("the finding %q does not say what the page came out as", got[0])
		}
	}
}

// TestOneItemIsEnough. A flex item is sized from its own content where a block
// child fills its container, so a container with a single item is already laid
// out at the wrong width — there is no "small enough not to matter" here, and a
// rule that waited for two would be quiet about exactly the page an author
// notices first.
func TestOneItemIsEnough(t *testing.T) {
	for _, doc := range []string{
		`<div id="f"><div>a</div></div>`,
		`<div id="f">bare text</div>`,
		`<div id="f"><span>a</span></div>`,
	} {
		if got := unlaidFindings(t, doc, `#f { display: flex }`); len(got) != 1 {
			t.Errorf("%s reported %d findings, want 1: %v", doc, len(got), got)
		}
	}
}

// TestAnEmptyFlexContainerIsTheBoxThatWasAsked. letter-spacing-204 writes
// "A<span class=flex></span><span class=block></span>D" and spaces the atomic
// inlines: the flex container is empty, an empty box is empty however it is laid
// out, and a finding there would say the page was wrong when it was right.
func TestAnEmptyFlexContainerIsTheBoxThatWasAsked(t *testing.T) {
	for _, doc := range []string{
		`<div id="f"></div>`,
		`<div id="f">   </div>`,
		`<div id="f"><div style="display: none">a</div></div>`,
		`<div id="f"><div style="position: absolute">a</div></div>`,
	} {
		if got := unlaidFindings(t, doc, `#f { display: flex }`); len(got) != 0 {
			t.Errorf("%s reported %v; the box holds no flex item and is the box "+
				"the specification asks for", doc, got)
		}
	}
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
	} {
		if got := unlaidFindings(t, doc, `#f { display: `+value+` }`+ann); len(got) != 0 {
			t.Errorf("display: %s reported %v, and it is laid out", value, got)
		}
	}
}
