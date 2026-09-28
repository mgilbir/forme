package layout

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// scaleDownFace is a face with a MATH table whose scale-downs are 80 and 60
// per cent, and nothing else of a math font's.
func scaleDownFace(t *testing.T) *shape.Face {
	t.Helper()
	return scaleDownFaceOf(t, 80, 60)
}

func scaleDownFaceOf(t *testing.T, script, scriptScript int) *shape.Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name:   "ScaleDown",
		Glyphs: []fonttest.Glyph{{Rune: 'x', Advance: 500, HasShape: true}},
		Extra: map[string][]byte{"MATH": fonttest.MATH(fonttest.MathOptions{
			Constants: map[string]int{"ScriptPercentScaleDown": script, "ScriptScriptPercentScaleDown": scriptScript},
		})},
	})
	face, err := shape.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// TestTheMathMLUserAgentSheet is appendix A as the cascade computes it: the
// <math> root set in the "math" family and reset, its display and math-style
// by its display attribute, the script levels of scripts, radicals and
// fractions — scaled by the MATH table of the font the parent is set in — and
// horizontal writing whatever an author says.
func TestTheMathMLUserAgentSheet(t *testing.T) {
	set := namedFaceSet{family: "math", face: scaleDownFace(t), standard: StandardFonts()}
	b := Build(Input{
		HTML: `<p style="font: italic bold 20px serif; letter-spacing: 3px">` +
			`<math id="m"><msup><mi id="base">x</mi><mn id="sup">2</mn></msup>` +
			`<mroot><mi>y</mi><mn id="index">3</mn></mroot>` +
			`<mfrac><mi id="num">a</mi><mn id="den">b</mn></mfrac>` +
			`<mover accent="true"><mi>z</mi><mo id="accent">^</mo></mover>` +
			`<mi id="vertical" style="writing-mode: vertical-rl">v</mi>` +
			`<semantics><mi id="sem1">a</mi><annotation id="sem2">b</annotation></semantics>` +
			`<maction><mi id="act1">a</mi><mi id="act2">b</mi></maction>` +
			`<merror id="err"><mi>e</mi></merror><mphantom id="ph"><mi>p</mi></mphantom>` +
			`<mtable id="tab"><mtr id="tr"><mtd id="td"><mi>c</mi></mtd></mtr></mtable>` +
			`<mfrac id="frac"><mi>a</mi><mi>b</mi></mfrac>` +
			`<mmultiscripts><mi id="mb">b</mi><mi id="post1">1</mi><mi id="post2">2</mi>` +
			`<mprescripts id="pre"/><mi id="pre1">3</mi><mi id="pre2">4</mi></mmultiscripts>` +
			`<munderover accent="true"><mo id="uob">x</mo><mi id="uou">u</mi><mi id="uoo">o</mi></munderover>` +
			`<msub><mi>s</mi><mi id="sub">i</mi></msub></math>` +
			`<math id="d" display="BLOCK"><mfrac><mi id="dnum">a</mi><mn>b</mn></mfrac></math></p>` +
			`<div hidden id="hidden"></div><math><mrow id="mhidden" hidden></mrow></math>`,
		Fonts: set,
	})
	if len(b.Findings) != 0 {
		for _, f := range b.Findings {
			if f.Rule != RuleUnsupportedElement {
				t.Errorf("finding: %s %s", f.Rule, f.Message)
			}
		}
	}
	byID := map[string]style.ComputedStyle{}
	for n, cs := range b.Styles {
		if id, ok := n.Attr("id"); ok {
			byID[id] = cs
		}
	}
	for _, tc := range []struct{ id, property, want string }{
		{"m", "display", "inline math"},
		{"m", "font-family", "math"},
		{"m", "font-style", "normal"},
		{"m", "font-weight", "normal"},
		{"m", "letter-spacing", "normal"},
		{"m", "font-size", "20px"},
		{"m", "math-style", "compact"},
		{"base", "display", "block math"},
		{"base", "text-transform", "math-auto"},
		{"base", "math-depth", "0"},
		{"sup", "math-depth", "1"},
		{"sup", "font-size", "16px"}, // 20 × 0.8
		{"index", "math-depth", "2"},
		{"index", "font-size", "12px"}, // 20 × 0.6
		{"num", "math-depth", "1"},     // auto-add under a compact <math>
		{"num", "math-style", "compact"},
		{"den", "math-shift", "compact"},
		{"accent", "math-depth", "1"},
		{"accent", "font-size", "20px"}, // an accent keeps its base's size
		{"vertical", "writing-mode", "horizontal-tb"},
		{"d", "display", "block math"},
		{"d", "math-style", "normal"},
		{"dnum", "math-depth", "0"}, // auto-add under a normal <math>
		{"dnum", "font-size", "20px"},
		{"sem1", "display", "block math"},
		{"sem2", "display", "none"},
		{"act1", "display", "block math"},
		{"act2", "display", "none"},
		{"err", "border-top-style", "solid"},
		{"err", "border-top-color", "red"},
		{"err", "background-color", "lightYellow"},
		{"ph", "visibility", "hidden"},
		{"tab", "display", "inline-table"},
		{"tab", "math-style", "compact"},
		{"tr", "display", "table-row"},
		{"td", "display", "table-cell"},
		{"td", "text-align-all", "center"},
		{"td", "padding-left", "8px"}, // 0.4em of 20px
		{"frac", "padding-left", "1px"},
		{"frac", "padding-right", "1px"},
		{"mb", "math-shift", "normal"},
		{"post1", "math-shift", "compact"},
		{"post2", "math-shift", "normal"},
		{"pre", "math-shift", "compact"},
		// After <mprescripts> the odd positions are the subscripts.
		{"pre1", "math-shift", "compact"},
		{"pre2", "math-shift", "normal"},
		{"uob", "math-shift", "compact"},
		{"uou", "font-size", "16px"},
		{"uoo", "font-size", "20px"},
		{"sub", "math-shift", "compact"},
		{"hidden", "display", "none"},
		// HTML's [hidden] rule is HTML's: MathML Core gives MathML no hidden.
		{"mhidden", "display", "block math"},
	} {
		if got := byID[tc.id].Get(tc.property); got != tc.want {
			t.Errorf("#%s %s = %q, want %q", tc.id, tc.property, got, tc.want)
		}
	}
}

// TestAFontStatingNoughtScalesByTheFallback: §5.1's scriptPercentScaleDown is
// 0.71 where the font states nought, and the script's script is still the
// font's own — which is what tells the fallback from a font with no table.
func TestAFontStatingNoughtScalesByTheFallback(t *testing.T) {
	set := namedFaceSet{family: "math", face: scaleDownFaceOf(t, 0, 60), standard: StandardFonts()}
	b := Build(Input{HTML: `<math style="font-size: 100px"><msup><mi>x</mi><mn id="s">2</mn></msup>` +
		`<mroot><mi>y</mi><mn id="ss">3</mn></mroot></math>`, Fonts: set})
	for n, cs := range b.Styles {
		switch id, _ := n.Attr("id"); id {
		case "s":
			if got := cs.Get("font-size"); got != "71px" {
				t.Errorf("the script is %s, want 71px", got)
			}
		case "ss":
			if got := cs.Get("font-size"); got != "60px" {
				t.Errorf("the script's script is %s, want 60px", got)
			}
		}
	}
}
