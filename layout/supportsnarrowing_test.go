package layout

import (
	"strings"
	"testing"
)

// TestASupportsBlockLetsThroughADeclarationThatStillReports is the other half
// of the narrowing style/supports.go keeps, and it is checked here because this
// is where the finding lands.
//
// An @supports condition is answered by the value grammar — whether the
// declaration is CSS the cascade applies — and not by whether layout draws
// every keyword it accepts. "position: sticky" is valid, so "(position:
// sticky)" is answered yes and the block is applied, although sticky
// positioning is not laid out here.
//
// What makes that sound rather than merely convenient is that the author is
// still told. The declaration the block let through is reported by the stage
// that could not act on it, so nothing is silently dropped; what changes is
// that the rest of the block, the part that was understood, is applied instead
// of being thrown away with it.
func TestASupportsBlockLetsThroughADeclarationThatStillReports(t *testing.T) {
	built := Build(Input{
		HTML: `<style>@supports (position: sticky) { p { position: sticky; color: red } }</style><p id="d">x</p>`,
	})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w := bgpx(600)
	h := bgpx(10000)
	rec := NewRecorder(nil)
	Layout(built.Root, Size{W: w, H: h}, nil, rec)

	var found bool
	for _, f := range append(append([]Finding(nil), built.Findings...), rec.Findings()...) {
		if strings.Contains(f.Message, "sticky") {
			found = true
		}
	}
	if !found {
		t.Error("the sticky declaration the @supports block let through was not " +
			"reported anywhere; the condition is answered about the property " +
			"precisely because the value is answered here")
	}
}
