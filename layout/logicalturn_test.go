package layout

import (
	"fmt"
	"testing"
)

// TestALogicalMarginInAVerticalBoxIsWhereItsLinesPutIt is audit C40 end to end:
// inside "writing-mode: vertical-rl" the inline start is the top and the block
// start is the right, so a logical margin draws exactly what the physical one
// it names draws. It drew the horizontal-tb one — the left margin for
// inline-start — with nothing reported.
func TestALogicalMarginInAVerticalBoxIsWhereItsLinesPutIt(t *testing.T) {
	const box = `<div style="writing-mode: vertical-rl; height: 200px">` +
		`<p id="a" style="margin: 0; %s">x</p></div>`
	for logical, physical := range map[string]string{
		"margin-inline-start: 50px": "margin-top: 50px",
		"margin-block-start: 30px":  "margin-right: 30px",
		"padding-inline-end: 20px":  "padding-bottom: 20px",
	} {
		place := func(decl string) [4]float64 {
			got := Compose(Input{HTML: fmt.Sprintf(box, decl)}, Options{})
			f := fragmentFor(got.Root, "a")
			if f == nil {
				t.Fatalf("%s: the paragraph was not laid out", decl)
			}
			r := f.BorderRect
			return [4]float64{r.X.Px(), r.Y.Px(), r.W.Px(), r.H.Px()}
		}
		if got, want := place(logical), place(physical); got != want {
			t.Errorf("%s drew the box at %v, and %s at %v", logical, got, physical, want)
		}
	}
}
