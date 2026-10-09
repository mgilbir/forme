package layout

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// A font whose morx runs away: HarfBuzz's fixture TestMORXThirtysix, an
// insertion whose entries never advance, which HarfBuzz gives up on for a
// single "A" (shape.refuseAAT). What a document set in it is told.

// runawayDoc is "A" set in TestMORXThirtysix, loaded by the document's own
// @font-face, or, with control, in the caller's face.
func runawayDoc(t *testing.T, control bool) Input {
	t.Helper()
	data, err := os.ReadFile("../testdata/harfbuzz/aat/fonts/TestMORXThirtysix.ttf")
	if err != nil {
		t.Fatal(err)
	}
	family := "Runaway"
	if control {
		family = "serif"
	}
	return Input{
		HTML: `<style>@font-face { font-family: Runaway; src: url(runaway.ttf) }</style>` +
			`<p style="font-family: ` + family + `">A</p>`,
		Resources: &fileResolver{files: map[string][]byte{"runaway.ttf": data}},
	}
}

// TestARunawayMorxIsReportedAndNotSetSilently: composed, the run is set as
// far as the font's morx got before its allowance ran out, as HarfBuzz leaves
// it, and the document is told so, naming the font, the table, the allowance
// and the text; composed under limits, the document is refused with an error
// saying the same, as for any run a limit refuses. A document set in another
// face is told nothing of the kind.
func TestARunawayMorxIsReportedAndNotSetSilently(t *testing.T) {
	const font = "TestMORXThirtysix-Regular"
	found := func(c Composed) []Finding {
		var out []Finding
		for _, f := range c.Findings {
			if f.Rule == RuleLimit && strings.Contains(f.Message, "ran out of") {
				out = append(out, f)
			}
		}
		return out
	}
	got := found(Compose(runawayDoc(t, false), Options{}))
	if len(got) != 1 {
		t.Fatalf("composed, the document was told %v", got)
	}
	t.Log(got[0].Error())
	for _, want := range []string{font, "its morx table ran out of the 65536 operations", `shaping "A"`, "HarfBuzz gives up on the run"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("the finding %q does not say %q", got[0].Message, want)
		}
	}
	if got := found(Compose(runawayDoc(t, true), Options{})); len(got) != 0 {
		t.Errorf("a document set in another face was told %v", got)
	}

	c, err := ComposeContext(context.Background(), runawayDoc(t, false), Options{}, shape.RunLimits{})
	if !errors.Is(err, shape.ErrRunLimit) || !strings.Contains(err.Error(), `the morx table of "`+font+`" ran out of`) {
		t.Errorf("composed under limits, the error was %v", err)
	}
	t.Log(err)
	if len(c.Ops) != 0 || len(c.Findings) != 0 {
		t.Errorf("a refused composition has %d ops and %d findings", len(c.Ops), len(c.Findings))
	}
	if _, err := ComposeContext(context.Background(), runawayDoc(t, true), Options{}, shape.RunLimits{}); err != nil {
		t.Errorf("a document set in another face was refused: %v", err)
	}
}
