package layout

import "testing"

// Which element decides whether a line may end at a boundary between two boxes.
//
// §5.1 gives the boundary to the innermost element containing both characters,
// and that is the answer for an opportunity the boundary itself creates — one
// "word-break: break-all" makes at the edge of a span, say. It is the wrong
// answer for an opportunity a *space* left behind. §3 gives that one to the
// space — "there is a soft wrap opportunity after every white space character"
// — so what decides whether it may be taken is the element the space is in.
//
// The two agree everywhere white-space is inherited rather than declared, which
// is why this went unnoticed. The suite's white-space-wrap-after-nowrap-001 is
// the document where they part: a nowrap block holding a wrapping span whose
// last character is a space, and then more of the block's own text.

const boundaryCSS = `body { margin: 0 }
	#d { width: 10ch; font-family: Courier; font-size: 20px; line-height: 1 }
	.normal { white-space: normal }
	.nowrap { white-space: nowrap }
	.pre { white-space: pre }`

// boundaryLines is how many lines a paragraph came out as.
func boundaryLines(t *testing.T, markup string) int {
	t.Helper()
	root := layoutOf(t, 400, `<div id="d">`+markup+`</div>`, boundaryCSS)
	ys := map[float64]bool{}
	for _, op := range Paint(root) {
		if o, ok := op.(DrawText); ok {
			ys[o.At.Y.Px()] = true
		}
	}
	return len(ys)
}

// TestASpaceInAWrappingSpanBreaksTheNowrapAroundIt is the suite's fifth and
// sixth rows: ten characters of room, five characters, a space, five more.
func TestASpaceInAWrappingSpanBreaksTheNowrapAroundIt(t *testing.T) {
	for _, markup := range []string{
		`<div class="nowrap"><span class="normal"><span class="nowrap">12345` +
			`</span> </span>67890</div>`,
		`<div class="nowrap"><span class="normal"><span class="nowrap">12345 ` +
			`</span> </span>67890</div>`,
	} {
		if got := boundaryLines(t, markup); got != 2 {
			t.Errorf("%s came out as %d lines, want 2 — the space is in a "+
				"wrapping span and the opportunity after it is the space's",
				markup, got)
		}
	}
}

// TestASpaceInANowrapSpanBreaksNothing, which is the same shape with the one
// element that decides it turned around. Without this row the rule above is
// satisfied by an engine that ignores nowrap at a boundary altogether.
func TestASpaceInANowrapSpanBreaksNothing(t *testing.T) {
	const markup = `<div class="normal"><span class="nowrap">12345 </span>` +
		`<span class="nowrap">67890</span></div>`
	if got := boundaryLines(t, markup); got != 1 {
		t.Errorf("came out as %d lines, want 1 — the space is in a nowrap span "+
			"and leaves no opportunity", got)
	}
}

// TestTheBoundaryStillDecidesWhereNoSpaceLeftTheOpportunity. An opportunity
// "word-break: break-all" makes at the edge of a span is the boundary's own,
// and §5.1 gives it to the innermost element containing both characters — which
// is what break-boundary-2-chars-001 turns on.
func TestTheBoundaryStillDecidesWhereNoSpaceLeftTheOpportunity(t *testing.T) {
	root := layoutOf(t, 400,
		`<div id="d" style="width:3ch;word-break:break-all">abc`+
			`<span class="pre">xyz</span>def</div>`, boundaryCSS)
	var lines []string
	at := map[float64]int{}
	for _, op := range Paint(root) {
		o, ok := op.(DrawText)
		if !ok {
			continue
		}
		y := o.At.Y.Px()
		if _, seen := at[y]; !seen {
			at[y] = len(lines)
			lines = append(lines, "")
		}
		lines[at[y]] += o.Text
	}
	// Three characters a line, and the span is one of them: the line ends at
	// each of its edges even though the span itself may not be broken inside.
	want := []string{"abc", "xyz", "def"}
	if len(lines) != len(want) {
		t.Fatalf("the paragraph came out as %d lines (%q), want 3", len(lines), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d reads %q, want %q", i+1, lines[i], want[i])
		}
	}
}
