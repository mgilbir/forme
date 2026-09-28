package style

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/html"
)

// MathML Core's cascade: math-style, math-shift, math-depth, "font-size:
// math", and the attributes that mean declarations.

// mathStyles styles a document and answers a property of the element whose id
// is given.
func mathStyles(t *testing.T, doc string, m Metrics, sheets ...Sheet) (func(id, property string) string, Styled) {
	t.Helper()
	d, _, _ := html.Parse(doc)
	got := ApplyWith(d, sheets, m)
	byID := map[string]ComputedStyle{}
	d.Walk(func(n *html.Node) bool {
		if id, ok := n.Attr("id"); ok {
			byID[id] = got.Styles[n]
		}
		return true
	})
	return func(id, property string) string {
		cs, ok := byID[id]
		if !ok {
			t.Fatalf("no element %q", id)
		}
		return cs.Get(property)
	}, got
}

// TestMathDepthComputesToAnInteger is §4.5's four cases, and what the
// inherited math-style decides about auto-add.
func TestMathDepthComputesToAnInteger(t *testing.T) {
	get, styled := mathStyles(t, `<div id="root">`+
		`<div id="compact" style="math-style: compact"><p id="auto" style="math-depth: auto-add"></p></div>`+
		`<div id="normal" style="math-style: normal"><p id="auto0" style="math-depth: auto-add"></p></div>`+
		`<div id="three" style="math-depth: 3"><p id="add" style="math-depth: add(-5)">`+
		`<span id="inherits"></span><span id="abs" style="math-depth: 7"></span></p></div>`+
		`<div id="huge" style="math-depth: 99999999999999999999"><p id="huger" style="math-depth: add(5)"></p></div>`+
		`<div id="calc" style="math-depth: 2"><p id="c" style="math-depth: calc(1 + 1)"></p></div>`+
		`</div>`, nil)
	for id, want := range map[string]string{
		"root": "0", "auto": "1", "auto0": "0", "three": "3", "add": "-2",
		"inherits": "-2", "abs": "7",
		// Bounded, and add() does not carry it past the bound.
		"huge": "1048576", "huger": "1048576",
		// A calc() this engine does not evaluate is reported and dropped,
		// and the depth is inherited.
		"c": "2",
	} {
		if got := get(id, "math-depth"); got != want {
			t.Errorf("#%s math-depth = %s, want %s", id, got, want)
		}
	}
	reported := false
	for _, f := range styled.Findings {
		reported = reported || f.Unsupported && strings.Contains(f.Message, "calc()")
	}
	if !reported {
		t.Errorf("the math-depth calc() was not reported: %v", styled.Findings)
	}
}

// TestMathFontScaleIsSection45 holds the scale factor to the algorithm's
// arithmetic: with a MATH table whose scale-downs are 0.8 and 0.6, a script
// is 0.8, a script's script 0.6 — not 0.8 × 0.71 — one level on from a script
// 0.6/0.8, and every level past the second 0.71; going back up is the
// inverse; and a font with no MATH table is 0.71 a level throughout.
func TestMathFontScaleIsSection45(t *testing.T) {
	for _, tc := range []struct {
		a, b    int
		hasMath bool
		want    float64
	}{
		{0, 0, true, 1},
		{0, 1, true, 0.8},
		{0, 2, true, 0.6},
		{0, 3, true, 0.6 * 0.71},
		{1, 2, true, 0.6 / 0.8},
		{1, 3, true, 0.6 / 0.8 * 0.71},
		{2, 3, true, 0.71},
		{-1, 1, true, 0.8 * 0.71}, // a ≤ 0 and b ≥ 2 does not hold; b == 1 does
		{-3, 1, true, 0.8 * 0.71 * 0.71 * 0.71},
		{2, 0, true, 1 / 0.6},
		{3, 1, true, 1 / (0.6 / 0.8 * 0.71)},
		{0, 1, false, 0.71},
		{0, 2, false, 0.71 * 0.71},
		{2, 0, false, 1 / (0.71 * 0.71)},
	} {
		got := mathFontScale(tc.a, tc.b, 0.8, 0.6, tc.hasMath)
		if math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("scale(%d→%d, math %v) = %v, want %v", tc.a, tc.b, tc.hasMath, got, tc.want)
		}
	}
}

// fakeMathMetrics states scale-downs of 0.8 and 0.6 for a style whose family
// is "withmath", and no MATH table for any other.
type fakeMathMetrics struct{ asked int }

func (fakeMathMetrics) XHeight(ComputedStyle, Unit) (float64, bool) { return 0, false }
func (f *fakeMathMetrics) MathScaleDowns(cs ComputedStyle, _ Unit) (float64, float64, bool) {
	f.asked++
	if cs.Get("font-family") == "withmath" {
		return 0.8, 0.6, true
	}
	return 0.71, 0.5041, false
}

// TestFontSizeMathScalesByTheInheritedFont: the parent's size times the scale
// for how far math-depth moved, from the parent's first available font —
// which is why the child's own family, set beside it, does not decide it.
func TestFontSizeMathScalesByTheInheritedFont(t *testing.T) {
	m := &fakeMathMetrics{}
	get, _ := mathStyles(t, `<div id="with" style="font: 20px withmath">`+
		`<p id="s" style="font-size: math; math-depth: 1; font-family: other">`+
		`<span id="ss" style="font-size: math; math-depth: add(1)"></span></p>`+
		`<p id="same" style="font-size: math"></p></div>`+
		`<div id="without" style="font: 20px nomath">`+
		`<p id="w1" style="font-size: math; math-depth: 2"></p></div>`, m)
	for id, want := range map[string]float64{
		"s":    20 * 0.8,        // 0 → 1, from withmath
		"ss":   20 * 0.8 * 0.71, // 1 → 2, from other, which has no MATH table
		"same": 20,
		"w1":   20 * 0.71 * 0.71,
	} {
		got := get(id, "font-size")
		px := strings.TrimSuffix(got, "px")
		if !strings.HasSuffix(got, "px") || math.Abs(parsePx(t, px)-want) > 1.0/64 {
			t.Errorf("#%s font-size = %s, want %vpx", id, got, want)
		}
	}
	if m.asked == 0 {
		t.Error("the font was never asked for its scale-downs")
	}
}

func parsePx(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("%q is not a number", s)
	}
	return v
}

// TestMathMLAttributesAreHints: the global attributes of §2.1.4–§2.1.6 and the
// element ones, each as a declaration of zero specificity that any author
// rule beats, and each ignored where its value is not what it takes.
func TestMathMLAttributesAreHints(t *testing.T) {
	get, _ := mathStyles(t, `<math>`+
		`<mrow id="c" mathcolor="red" mathbackground="#00ff00" dir="RTL"></mrow>`+
		`<mrow id="bad" mathcolor="not a colour" mathbackground="12px" dir="up"></mrow>`+
		`<mrow id="size" mathsize="20px"></mrow><mrow id="pct" mathsize="150%"></mrow>`+
		`<mrow id="kw" mathsize="large"></mrow>`+
		`<mrow id="ds" displaystyle="TRUE"></mrow><mrow id="dsf" displaystyle="false"></mrow>`+
		`<mrow id="dsbad" displaystyle="yes"></mrow>`+
		`<mrow id="sl" scriptlevel="+2"><mrow id="slm" scriptlevel="-1"></mrow></mrow>`+
		`<mrow id="sla" scriptlevel="3"></mrow><mrow id="slbad" scriptlevel="+-1"></mrow>`+
		`<mrow scriptlevel="2"><mrow id="slf" scriptlevel="1.5"></mrow></mrow>`+
		`<mi id="mv" mathvariant="NORMAL">x</mi><mi id="mvb" mathvariant="bold">x</mi>`+
		`<mn id="mvn" mathvariant="normal">1</mn>`+
		`<mspace id="sp" width="3px" height="4px" depth="5px"></mspace>`+
		`<mspace id="sph" height="4px"></mspace><mspace id="spd" depth="5px"></mspace>`+
		`<mspace id="sppct" width="50%" height="10%"></mspace>`+
		`<mpadded id="pad" width="7px"></mpadded><mpadded id="padpct" width="50%"></mpadded>`+
		`<mrow id="beaten" mathcolor="red"></mrow>`+
		`</math>`, nil,
		author(t, `#beaten { color: blue }`),
		// The user agent's rule, which a hint beats and an author rule would not.
		sheet(t, OriginUserAgent, `mi { text-transform: math-auto } mn { text-transform: uppercase }`))
	for _, tc := range []struct{ id, property, want string }{
		{"c", "color", "red"},
		{"c", "background-color", "#00ff00"},
		{"c", "direction", "rtl"},
		{"bad", "color", "black"},
		{"bad", "background-color", "transparent"},
		{"bad", "direction", "ltr"},
		{"size", "font-size", "20px"},
		{"pct", "font-size", "24px"},
		{"kw", "font-size", "16px"},
		{"ds", "math-style", "normal"},
		{"dsf", "math-style", "compact"},
		{"dsbad", "math-style", "normal"},
		{"sl", "math-depth", "2"},
		{"slm", "math-depth", "1"},
		{"sla", "math-depth", "3"},
		{"slbad", "math-depth", "0"},
		{"slf", "math-depth", "2"},
		{"mv", "text-transform", "none"},
		{"mvb", "text-transform", "math-auto"},
		{"mvn", "text-transform", "uppercase"},
		{"sp", "width", "3px"},
		{"sp", "height", "calc(4px + 5px)"},
		{"sph", "height", "4px"},
		{"spd", "height", "5px"},
		{"sppct", "width", "auto"},
		{"sppct", "height", "auto"},
		{"pad", "width", "7px"},
		{"padpct", "width", "auto"},
		{"beaten", "color", "blue"},
	} {
		if got := get(tc.id, tc.property); got != tc.want {
			t.Errorf("#%s %s = %q, want %q", tc.id, tc.property, got, tc.want)
		}
	}
}

// TestDisplayTakesMath: <display-outside> || [ <display-inside> | math ].
func TestDisplayTakesMath(t *testing.T) {
	for value, ok := range map[string]bool{
		"math": true, "block math": true, "math inline": true, "run-in math": true,
		"list-item math": false, "math flow": false, "math math": false,
	} {
		get, _ := mathStyles(t, `<div id="d" style="display: `+value+`"></div>`, nil)
		got := get("d", "display") != "inline"
		if got != ok {
			t.Errorf("display: %s accepted = %v, want %v", value, got, ok)
		}
	}
}

// TestAHintThisEngineCannotEvaluateIsReported: an attribute whose value is
// valid and names what this engine does not evaluate — a width of 13lh, a
// mathsize of 2rlh — is dropped as its declaration would be, and says so; the
// property is then what the cascade gives without it. A hint it does
// evaluate says nothing.
func TestAHintThisEngineCannotEvaluateIsReported(t *testing.T) {
	get, styled := mathStyles(t, `<math><mspace id="w" width="13lh" height="10px"></mspace>`+
		`<mrow id="s" mathsize="2rlh"></mrow><mspace id="ok" width="3px"></mspace></math>`, nil)
	for _, want := range []string{`"width: 13lh" uses the unit lh`, `"font-size: 2rlh" uses the unit rlh`} {
		found := false
		for _, f := range styled.Findings {
			// Unsupported and naming its property, as a dropped
			// declaration's finding does: that is what layout reports as
			// unsupported-property rather than as a selector.
			if strings.Contains(f.Message, want) && strings.Contains(f.Message, "attribute") &&
				f.Unsupported && strings.HasPrefix(want, `"`+f.Property+`:`) {
				found = true
			}
		}
		if !found {
			t.Errorf("no finding says %s: %v", want, styled.Findings)
		}
	}
	if len(styled.Findings) != 2 {
		t.Errorf("%d findings, want the two: %v", len(styled.Findings), styled.Findings)
	}
	if got := get("w", "width"); got != "auto" {
		t.Errorf("the 13lh space's width is %q, want auto: the hint dropped", got)
	}
	if got := get("s", "font-size"); got != "16px" {
		t.Errorf("the 2rlh row's font-size is %q, want the inherited 16px", got)
	}
	if got := get("ok", "width"); got != "3px" {
		t.Errorf("the 3px space's width is %q", got)
	}
}
