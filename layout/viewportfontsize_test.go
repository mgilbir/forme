package layout

import (
	"strconv"
	"testing"
)

// TestAFontSizeInViewportUnitsIsAPercentageOfThePageArea is audit C157 end to
// end. The page area is what every other viewport-relative length is resolved
// against (lengthContext), and a font-size in vw is resolved in the cascade, so
// the cascade has to be told the same area: "font-size: 5vw" sets the text as
// the same number of pixels written out does, and nothing reports it as
// unresolvable. It used to be kept at the inherited size, with a finding saying
// so, although the page was decided before anything was styled.
func TestAFontSizeInViewportUnitsIsAPercentageOfThePageArea(t *testing.T) {
	area := A4.Content()
	for unit, px := range map[string]float64{
		"vw":   area.W.Px() / 20,
		"vh":   area.H.Px() / 20,
		"vmin": min(area.W.Px(), area.H.Px()) / 20,
		"vmax": max(area.W.Px(), area.H.Px()) / 20,
	} {
		place := func(size string) (float64, []Finding) {
			out := Compose(Input{HTML: `<p id="a" style="margin: 0; font-size: ` + size +
				`"><span style="font-size: 0.5em">x</span>x</p>`}, Options{Page: A4})
			f := fragmentFor(out.Root, "a")
			if f == nil {
				t.Fatalf("%s: the paragraph was not laid out", size)
			}
			return f.BorderRect.H.Px(), out.Findings
		}
		got, findings := place("5" + unit)
		want, _ := place(strconv.FormatFloat(px, 'f', -1, 64) + "px")
		if got != want {
			t.Errorf("font-size: 5%s set a line %vpx tall, and the %vpx it is %vpx", unit, got, px, want)
		}
		for _, f := range findings {
			if f.Property == "font-size" {
				t.Errorf("font-size: 5%s was reported: %s", unit, f.Message)
			}
		}
	}
}
