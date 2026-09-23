package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Intrinsic sizes are answered by the formatting context that lays the box out.
//
// CSS Sizing 3 §5 asks a box how wide it would like to be, and the box's
// formatting context answers: a block stacks its children and needs the widest,
// Flexbox §9.9 sums the items of a row, and Grid §12 sums its tracks. Every box
// here was measured as a block, so a float or an absolutely positioned box
// holding a flex row or a grid shrank to the widest item and hung the rest
// outside itself.
//
// The fixture is flex_test.go's: Courier at 20px is 12px a character, so every
// width below is a whole number of characters and every sum is exact.

const intrinsicFCCSS = `body { margin: 0 }
	* { font-family: Courier; font-size: 20px; line-height: 20px }`

// fcLayout lays a document out at 1000px and returns its root and findings.
func fcLayout(t *testing.T, htmlSrc, cssSrc string) (*Fragment, []Finding) {
	t.Helper()
	got := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: intrinsicFCCSS + cssSrc}}})
	if got.Root == nil {
		t.Fatalf("the document produced no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(1000)
	h, _ := style.FromPx(10000)
	frag := Layout(got.Root, Size{W: w, H: h}, nil, rec)
	if frag == nil {
		t.Fatal("layout produced no fragment")
	}
	return frag, rec.Findings()
}

// fcWidth is the border-box width of #id.
func fcWidth(t *testing.T, htmlSrc, cssSrc, id string) float64 {
	t.Helper()
	root, _ := fcLayout(t, htmlSrc, cssSrc)
	return find(t, root, id).BorderRect.W.Px()
}

func wantWidth(t *testing.T, what string, got, want float64) {
	t.Helper()
	if got != want {
		t.Errorf("%s: %gpx wide, want %gpx", what, got, want)
	}
}

// TestAShrinkToFitBoxHoldsAFlexRow is the audit's C26 case and its relatives:
// the max-content width of a row is the sum of its items' max-content
// contributions (§9.9.1's Web-compatible algorithm), and every shrink-to-fit
// box — a float, an absolutely positioned box, an inline-flex — is that wide.
// Measured as a block, each was one item wide.
func TestAShrinkToFitBoxHoldsAFlexRow(t *testing.T) {
	row := `<div id="f" style="display: flex"><div>AAAA</div><div id="b">BBBB</div></div>`
	wantWidth(t, "a float holding a row",
		fcWidth(t, `<div id="x" style="float: left">`+row+`</div>`, ``, "x"), 96)
	wantWidth(t, "a floated flex container",
		fcWidth(t, `<div id="x" style="float: left; display: flex"><div>AAAA</div><div>BBBB</div></div>`,
			``, "x"), 96)
	wantWidth(t, "an absolutely positioned flex container",
		fcWidth(t, `<div id="x" style="position: absolute; display: flex"><div>AAAA</div><div>BBBB</div></div>`,
			``, "x"), 96)
	wantWidth(t, "an inline-flex holding blocks",
		fcWidth(t, `<div><span id="x" style="display: inline-flex"><div>AAAA</div><div>BBBB</div></span></div>`,
			``, "x"), 96)

	// The second item is inside the float and not beside it.
	root, _ := fcLayout(t, `<div id="x" style="float: left">`+row+`</div>`, ``)
	if b := find(t, root, "b"); b.BorderRect.X.Px() != 48 || b.BorderRect.W.Px() != 48 {
		t.Errorf("the second item is at x=%g width=%g, want x=48 width=48",
			b.BorderRect.X.Px(), b.BorderRect.W.Px())
	}

	// A nested row used as an item asks its parent row for its own sum, so
	// the item after it starts where it ends.
	root, _ = fcLayout(t, `<div style="display: flex"><div id="i" style="display: flex">`+
		`<div>AAAA</div><div>BBBB</div></div><div id="c">C</div></div>`, ``)
	if c := find(t, root, "c"); c.BorderRect.X.Px() != 96 {
		t.Errorf("the item after a nested row is at x=%g, want 96: the nested row "+
			"was measured as one of its items", c.BorderRect.X.Px())
	}
}

// TestARowsGapsAreRoomItNeeds. Box Alignment §8: a gap is space between two
// items, and a row measured without its gaps is too narrow by them.
func TestARowsGapsAreRoomItNeeds(t *testing.T) {
	wantWidth(t, "a floated row with a 10px gap",
		fcWidth(t, `<div id="x" style="float: left; display: flex; column-gap: 10px">`+
			`<div>AAAA</div><div>BBBB</div><div>CC</div></div>`, ``, "x"), 48+48+24+20)
	// A percentage gap is of the width being worked out, and resolves against
	// nothing while it is: the float is its items and no more.
	wantWidth(t, "a floated row with a percentage gap",
		fcWidth(t, `<div id="x" style="float: left; display: flex; column-gap: 10%">`+
			`<div>AAAA</div><div>BBBB</div></div>`, ``, "x"), 96)
}

// TestARowsMinimumDependsOnWhetherItWraps. §9.9.1: a single-line container's
// min-content width is the sum of its items' min-content contributions, because
// none may go onto another line; a multi-line one's is the largest, because
// each may. A float in a 30px column is sized to its minimum.
func TestARowsMinimumDependsOnWhetherItWraps(t *testing.T) {
	items := `<div>AAAA BB</div><div>CCC</div>`
	wantWidth(t, "a single-line row squeezed",
		fcWidth(t, `<div style="width: 30px"><div id="x" style="float: left; display: flex">`+
			items+`</div></div>`, ``, "x"), 48+36)
	wantWidth(t, "a single-line row squeezed, with a gap",
		fcWidth(t, `<div style="width: 30px"><div id="x" style="float: left; display: flex; column-gap: 5px">`+
			items+`</div></div>`, ``, "x"), 48+36+5)
	// An item that may not shrink is floored by its flex base size, and that
	// base is found under the constraint the row is measured under (§9.2.3):
	// at min-content, its content's min-content size and not its whole line.
	wantWidth(t, "a single-line row squeezed, with an item that may not shrink",
		fcWidth(t, `<div style="width: 30px"><div id="x" style="float: left; display: flex">`+
			`<div style="flex: none">AAAA BB</div><div>CCC</div></div></div>`, ``, "x"), 48+36)
	wantWidth(t, "a wrapping row squeezed",
		fcWidth(t, `<div style="width: 30px"><div id="x" style="float: left; display: flex; `+
			`flex-wrap: wrap; column-gap: 5px">`+items+`</div></div>`, ``, "x"), 48)
}

// TestAColumnIsAsWideAsItsWidestItem. §9.9.2: down a column the width is the
// cross size, and a single line's is its largest contribution.
func TestAColumnIsAsWideAsItsWidestItem(t *testing.T) {
	wantWidth(t, "a floated column",
		fcWidth(t, `<div id="x" style="float: left; display: flex; flex-direction: column; row-gap: 50px">`+
			`<div>AAAA</div><div>BBBBBB</div></div>`, ``, "x"), 72)
}

// TestAWrappingColumnWithAHeightIsReported. Its lines depend on its items'
// heights, which the intrinsic walk does not measure; it is sized as one line
// and says so, rather than quietly.
func TestAWrappingColumnWithAHeightIsReported(t *testing.T) {
	said := func(css string) bool {
		_, findings := fcLayout(t, `<div style="float: left; display: flex; flex-direction: column; `+
			css+`"><div>AAAA</div><div>BBBB</div></div>`, ``)
		for _, f := range findings {
			if f.Property == "flex-wrap" && strings.Contains(f.Message, "measured as though") {
				return true
			}
		}
		return false
	}
	if !said("flex-wrap: wrap; height: 20px") {
		t.Error("a wrapping column with a height was measured as one line and not reported")
	}
	if said("flex-wrap: wrap") {
		t.Error("a wrapping column with no height to wrap against was reported")
	}
	if said("height: 20px") {
		t.Error("a column that does not wrap was reported")
	}
}

// TestAFlexItemContributesWhatItWillTake is §9.9.3. An item's contribution is
// the larger of its content and its stated width, capped by its flex base size
// if it may not grow, floored by it if it may not shrink, and held between its
// own minimum and maximum.
func TestAFlexItemContributesWhatItWillTake(t *testing.T) {
	float := func(item string) string {
		return `<div id="x" style="float: left; display: flex">` + item + `<div>B</div></div>`
	}
	for _, c := range []struct {
		what, item string
		want       float64
	}{
		{"an inflexible basis wider than the content",
			`<div style="flex: 0 0 100px">A</div>`, 100 + 12},
		{"an inflexible basis narrower than the content",
			`<div style="flex: none; width: 30px">AAAA</div>`, 30 + 12},
		{"a declared width with the initial factors",
			`<div style="width: 30px">AAAA</div>`, 30 + 12},
		{"a width that may grow asks for its content",
			`<div style="width: 30px; flex-grow: 1">AAAA</div>`, 48 + 12},
		{"a width wider than the content that may grow asks for the width",
			`<div style="width: 100px; flex-grow: 1">A</div>`, 100 + 12},
		{"a maximum below the content",
			`<div style="flex: 1; max-width: 24px">AAAA</div>`, 24 + 12},
		{"a basis of zero that may grow asks for its content",
			`<div style="flex: 1">AAAA</div>`, 48 + 12},
		{"a percentage width asks for its content",
			`<div style="width: 10%">AAAA</div>`, 48 + 12},
		{"margins and padding are part of the contribution",
			`<div style="margin: 0 3px; padding: 0 2px">AAAA</div>`, 48 + 10 + 12},
	} {
		wantWidth(t, c.what, fcWidth(t, float(c.item), ``, "x"), c.want)
	}
}

// TestAShrinkToFitBoxHoldsAGrid is Grid §12's answer: the sum of the tracks
// sized under a min-content or a max-content constraint, gaps included.
func TestAShrinkToFitBoxHoldsAGrid(t *testing.T) {
	grid := func(style, items string) string {
		return `<div id="x" style="float: left; display: grid; ` + style + `">` + items + `</div>`
	}
	two := `<div>AAAA</div><div>BBBBBBBB</div>`
	wantWidth(t, "auto auto", fcWidth(t, grid("grid-template-columns: auto auto", two), ``, "x"), 48+96)
	wantWidth(t, "auto auto with a gap",
		fcWidth(t, grid("grid-template-columns: auto auto; column-gap: 10px", two), ``, "x"), 48+96+10)
	// §12.7.1 with an indefinite free space: one fr is shared, so two equal
	// columns are each as wide as the wider item.
	wantWidth(t, "1fr 1fr", fcWidth(t, grid("grid-template-columns: 1fr 1fr", two), ``, "x"), 96+96)
	wantWidth(t, "1fr 2fr",
		fcWidth(t, grid("grid-template-columns: 1fr 2fr", `<div>AAAAAAAA</div><div>B</div>`), ``, "x"), 96+192)
	// An item spanning flexible tracks asks for the fr that lets them hold its
	// whole line, and the tracks share it; their bases hold only its longest
	// word.
	wantWidth(t, "an item spanning 1fr 1fr",
		fcWidth(t, grid("grid-template-columns: 1fr 1fr",
			`<div style="grid-column: 1 / 3">AAAA AAAA</div>`), ``, "x"), 108)
	// §12.5 under a max-content constraint: an "auto" minimum is the items'
	// max-content contribution, so a track whose factor is below one does not
	// shrink its line.
	wantWidth(t, "0.5fr", fcWidth(t, grid("grid-template-columns: 0.5fr",
		`<div>AAAA AAAA</div>`), ``, "x"), 108)
	wantWidth(t, "a fixed track and an auto one",
		fcWidth(t, grid("grid-template-columns: 50px auto", two), ``, "x"), 50+96)
	// A percentage track depends on the width being asked for and is "auto"
	// while it is (§7.2.1).
	wantWidth(t, "a percentage track", fcWidth(t, grid("grid-template-columns: 50% auto", two), ``, "x"), 48+96)
	// An item spanning both columns asks them together for what it needs.
	wantWidth(t, "a spanning item",
		fcWidth(t, grid("grid-template-columns: auto auto",
			`<div>A</div><div>B</div><div style="grid-column: 1 / 3">CCCCCCCCCC</div>`), ``, "x"), 120)
	// Squeezed to its minimum: the tracks at their base sizes, and a flexible
	// track at its base too, the flex fraction being zero.
	wantWidth(t, "squeezed auto auto",
		fcWidth(t, `<div style="width: 10px">`+grid("grid-template-columns: auto 1fr",
			`<div>AA AAAA</div><div>BBB BB</div>`)+`</div>`, ``, "x"), 48+36)
}

// TestAGridItemsOwnLimitsAreInItsContribution. The item's min-width and
// max-width hold its contribution as they hold its width.
func TestAGridItemsOwnLimitsAreInItsContribution(t *testing.T) {
	wantWidth(t, "an item with a maximum",
		fcWidth(t, `<div id="x" style="float: left; display: grid">`+
			`<div style="max-width: 50px">AAAAAAAA</div></div>`, ``, "x"), 50)
	wantWidth(t, "an item with a minimum",
		fcWidth(t, `<div id="x" style="float: left; display: grid">`+
			`<div style="min-width: 100px">A</div></div>`, ``, "x"), 100)
}

// TestAKeywordLimitIsInAContribution. A box whose min-width is max-content is
// laid out at its whole line, and a parent that shrinks round it has to be
// that wide too.
func TestAKeywordLimitIsInAContribution(t *testing.T) {
	wantWidth(t, "a child at min-width: max-content",
		fcWidth(t, `<div id="x" style="float: left"><div style="width: 10px; min-width: max-content">`+
			`AAAA BBBB</div></div>`, ``, "x"), 108)
	wantWidth(t, "a child at max-width: min-content",
		fcWidth(t, `<div id="x" style="float: left"><div style="max-width: min-content">`+
			`AAAA BB</div></div>`, ``, "x"), 48)
}

// TestAnAbsolutelyPositionedChildDoesNotWidenItsParent is C35: out of flow, it
// takes no room in the box it is written in, so a float around it is as wide
// as the content that is in flow.
func TestAnAbsolutelyPositionedChildDoesNotWidenItsParent(t *testing.T) {
	menu := `<div style="position: absolute; width: 500px">menu</div>`
	wantWidth(t, "a float with an absolutely positioned child",
		fcWidth(t, `<div id="x" style="float: left"><div>hi</div>`+menu+`</div>`, ``, "x"), 24)
	wantWidth(t, "an inline-block with an absolutely positioned child",
		fcWidth(t, `<div><div id="x" style="display: inline-block"><div>hi</div>`+menu+`</div></div>`, ``, "x"), 24)
	wantWidth(t, "a fixed child",
		fcWidth(t, `<div id="x" style="float: left"><div>hi</div>`+
			`<div style="position: fixed; width: 500px">menu</div></div>`, ``, "x"), 24)
	// A float among them is still measured: it is inside the box.
	wantWidth(t, "a float child",
		fcWidth(t, `<div id="x" style="float: left"><div>hi</div>`+
			`<div style="float: left; width: 50px">f</div></div>`, ``, "x"), 50)
	// And an absolutely positioned flex item is not an item of the row.
	wantWidth(t, "a row with an absolutely positioned child",
		fcWidth(t, `<div id="x" style="float: left; display: flex"><div>hi</div>`+menu+`</div>`, ``, "x"), 24)
}

// TestIntrinsicKeywordsOnAFlexItemAreApplied is C152. A basis or a width
// written as an intrinsic keyword names a size of the item's content, and each
// fell to "auto" without a word.
func TestIntrinsicKeywordsOnAFlexItemAreApplied(t *testing.T) {
	row := func(item string) string {
		return `<div style="display: flex; width: 600px">` + item + `</div>`
	}
	for _, c := range []struct {
		what, item string
		want       float64
	}{
		{"flex-basis: max-content beside a width",
			`<div id="i" style="flex-basis: max-content; width: 50px">AAAAAAAA</div>`, 96},
		{"flex-basis: min-content",
			`<div id="i" style="flex-basis: min-content">AA AA AA</div>`, 24},
		{"flex-basis: fit-content in a wide row is its line",
			`<div id="i" style="flex-basis: fit-content">AA AA AA</div>`, 96},
		{"width: min-content",
			`<div id="i" style="width: min-content">AA AA AA</div>`, 24},
		{"flex-basis: content sets a keyword width aside",
			`<div id="i" style="width: min-content; flex-basis: content">AA AA AA</div>`, 96},
		{"max-width: min-content holds a growing item",
			`<div id="i" style="flex-grow: 1; max-width: min-content">AA AA AA</div>`, 24},
	} {
		wantWidth(t, c.what, fcWidth(t, row(c.item), ``, "i"), c.want)
	}
	wantWidth(t, "min-width: max-content stops a shrinking item",
		fcWidth(t, `<div style="display: flex; width: 50px">`+
			`<div id="i" style="flex: 1 1 10px; min-width: max-content">AA AA AA</div></div>`, ``, "i"), 96)
	// fit-content is of the room the line leaves the item.
	wantWidth(t, "width: fit-content in a narrow row",
		fcWidth(t, `<div style="display: flex; width: 50px">`+
			`<div id="i" style="width: fit-content; flex-shrink: 0">AA AA AA</div></div>`, ``, "i"), 50)
	// Down a column the width is the cross size, and a keyword there is a size
	// of the item's own: it is not stretched to the column.
	wantWidth(t, "width: min-content down a column",
		fcWidth(t, `<div style="display: flex; flex-direction: column; width: 600px">`+
			`<div id="i" style="width: min-content">AA AA AA</div></div>`, ``, "i"), 24)
	wantWidth(t, "width: max-content down a column",
		fcWidth(t, `<div style="display: flex; flex-direction: column; width: 600px">`+
			`<div id="i" style="width: max-content">AA AA AA</div></div>`, ``, "i"), 96)
}

// TestAnIntrinsicKeywordOnAFlexContainerIsApplied. A flex or grid container's
// width goes through the same resolution a block's does; it refused the
// keywords only because what it would have read was the widest item.
func TestAnIntrinsicKeywordOnAFlexContainerIsApplied(t *testing.T) {
	wantWidth(t, "a row at width: max-content",
		fcWidth(t, `<div id="x" style="display: flex; width: max-content">`+
			`<div>AAAA</div><div>BBBB</div></div>`, ``, "x"), 96)
	wantWidth(t, "a grid at width: max-content",
		fcWidth(t, `<div id="x" style="display: grid; grid-template-columns: auto auto; width: max-content">`+
			`<div>AAAA</div><div>BBBB</div></div>`, ``, "x"), 96)
	_, findings := fcLayout(t, `<div style="display: flex; width: min-content"><div>A</div></div>`, ``)
	for _, f := range findings {
		if f.Property == "width" && f.Rule == RuleUnsupportedValue {
			t.Errorf("an applied keyword was reported: %s", f.Message)
		}
	}
}

// TestAFlexBasisKeywordNotReadIsReported. "stretch" and "fit-content()" are
// valid flex bases that this engine does not read; they fall to "auto", and
// the document is told.
func TestAFlexBasisKeywordNotReadIsReported(t *testing.T) {
	reported := func(basis string) bool {
		_, findings := fcLayout(t, `<div style="display: flex"><div style="flex-basis: `+
			basis+`">A</div></div>`, ``)
		for _, f := range findings {
			if f.Property == "flex-basis" && f.Rule == RuleUnsupportedValue {
				return true
			}
		}
		return false
	}
	for _, basis := range []string{"stretch", "fit-content(20px)"} {
		if !reported(basis) {
			t.Errorf("flex-basis: %s was dropped without a finding", basis)
		}
	}
	for _, basis := range []string{"min-content", "max-content", "fit-content", "auto", "content", "10px"} {
		if reported(basis) {
			t.Errorf("flex-basis: %s is applied and was reported", basis)
		}
	}
	// On a box that is not a flex item it does nothing, which is not a drop.
	_, findings := fcLayout(t, `<div style="flex-basis: stretch">A</div>`, ``)
	for _, f := range findings {
		if f.Property == "flex-basis" {
			t.Errorf("flex-basis on a block was reported: %s", f.Message)
		}
	}
}

// TestPreservedWhiteSpaceMakesNoItem is C130. Flexbox §4 and Grid §6: a run of
// text that is only document white space is not rendered, whatever white-space
// says — so indentation in a "white-space: pre" container is not three extra
// items.
func TestPreservedWhiteSpaceMakesNoItem(t *testing.T) {
	src := "<div id=\"f\" style=\"display: %s; white-space: pre\">\n  <div>a</div>\n  <div>b</div>\n</div>"
	for _, display := range []string{"flex", "grid"} {
		root, _ := fcLayout(t, strings.Replace(src, "%s", display, 1), ``)
		if n := len(find(t, root, "f").Children); n != 2 {
			t.Errorf("display: %s with preserved white space has %d items, want 2", display, n)
		}
	}
	// A no-break space is not document white space: it is text an author wrote,
	// and it makes an item.
	root, _ := fcLayout(t, "<div id=\"f\" style=\"display: flex\"> <div>a</div></div>", ``)
	if n := len(find(t, root, "f").Children); n != 2 {
		t.Errorf("a no-break space beside an item made %d items, want 2", n)
	}
}
