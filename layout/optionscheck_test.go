package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// unitPx is a length in CSS pixels, for the page geometry below.
func unitPx(t *testing.T, v float64) style.Unit {
	t.Helper()
	u, ok := style.FromPx(v)
	if !ok {
		t.Fatalf("%v px is not a layout unit", v)
	}
	return u
}

// TestNonsenseOptionsAreRefusedAndSaidSo is the half of Options that was never
// checked.
//
// A minimum scale of 2 is a floor above every scale there is and refused every
// document; a negative one turned the guard off, which is the same shape of
// mistake as a cap of zero; a page one point wide left no content box. They are
// the caller's numbers rather than the document's, and Compose returns no error
// — so a caller has no way to be told unless the findings say it.
func TestNonsenseOptionsAreRefusedAndSaidSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
		says string
	}{
		{"a minimum scale no document can reach", Options{MinScale: 2}, "no scale can reach"},
		{"a minimum scale nothing is below", Options{MinScale: -1}, "no scale can be below"},
		{"a negative minimum font size", Options{MinFontSizePt: -1}, "no size is below"},
		{"a page with no height",
			Options{Page: PageSize{Width: unitPx(t, 600)}}, "not a sheet"},
		{"a page with a negative width",
			Options{Page: PageSize{Width: unitPx(t, -1), Height: unitPx(t, 800)}}, "not a sheet"},
		{"a negative margin", Options{Page: PageSize{
			Width: unitPx(t, 600), Height: unitPx(t, 800),
			Margin: Edges{Top: unitPx(t, -100), Right: unitPx(t, -100),
				Bottom: unitPx(t, -100), Left: unitPx(t, -100)}}}, "outside the paper"},
		{"margins wider than the sheet", Options{Page: PageSize{
			Width: unitPx(t, 200), Height: unitPx(t, 800),
			Margin: Edges{Right: unitPx(t, 300), Left: unitPx(t, 300)}}}, "leaves nothing to print in"},
	} {
		got := Compose(Input{HTML: `<p>hello</p>`}, tc.opts)
		var said bool
		for _, f := range got.Findings {
			if f.Rule == RuleInvalidCSS && strings.Contains(f.Message, tc.says) {
				said = true
			}
		}
		if !said {
			t.Errorf("%s: raised %v, none of them saying %q", tc.name, got.Findings, tc.says)
		}
		// And whatever was refused, what comes out is a page that can be drawn.
		if got.Scale <= 0 {
			t.Errorf("%s: the scale is %v", tc.name, got.Scale)
		}
	}
}

// TestOrdinaryOptionsAreNotRefused is the control. Every default and every
// value a caller sensibly passes must go through untouched.
func TestOrdinaryOptionsAreNotRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"the zero value", Options{}},
		{"A4", Options{Page: A4}},
		{"Letter with a floor", Options{Page: Letter, MinScale: 0.25, MinFontSizePt: 4}},
		{"a scale of exactly one", Options{MinScale: 1}},
		{"a page with no margin at all",
			Options{Page: PageSize{Width: unitPx(t, 600), Height: unitPx(t, 800)}}},
	} {
		got := Compose(Input{HTML: `<p>hello</p>`}, tc.opts)
		for _, f := range got.Findings {
			if f.Rule == RuleInvalidCSS {
				t.Errorf("%s: %s", tc.name, f.Message)
			}
		}
	}
}

// TestAPageRuleCannotMakeAPageWithNoRoomOnIt is the same geometry arriving from
// the document instead of from the caller.
//
// "@page { margin: 100mm }" on A5 leaves a content box of negative width, which
// came out of the scale-to-fit arithmetic as a scale of minus one and a half
// and a finding saying the text would be set at minus nineteen points.
func TestAPageRuleCannotMakeAPageWithNoRoomOnIt(t *testing.T) {
	for _, tc := range []struct{ name, css string }{
		{"margins wider than the sheet", `@page { margin: 100mm }`},
		{"a negative margin", `@page { margin: -20mm }`},
	} {
		got := Compose(Input{
			HTML: `<p>hello</p>`, CSS: []Stylesheet{{Source: tc.css}},
		}, Options{Page: A5})
		if got.Scale <= 0 {
			t.Errorf("%s: the scale is %v", tc.name, got.Scale)
		}
		for _, f := range got.Findings {
			if strings.Contains(f.Message, "-") && f.Rule == RuleMinFontSize {
				t.Errorf("%s: %s", tc.name, f.Message)
			}
		}
	}
}
