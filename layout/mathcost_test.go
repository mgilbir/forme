package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// mathRowDoc is a formula of n operators between operands, in one row.
func mathRowDoc(t *testing.T, n int) (Built, FontSet) {
	set := namedFaceSet{family: "math", face: mathTestFace(t), standard: StandardFonts()}
	var sb strings.Builder
	sb.WriteString("<math>")
	for i := 0; i < n; i++ {
		sb.WriteString("<mi>x</mi><mo>+</mo>")
	}
	sb.WriteString("<mn>1</mn></math>")
	return Build(Input{HTML: sb.String(), Fonts: set}), set
}

// TestAFormulaIsLaidOutInLinearTime: a row of n operators and operands, and
// one of four times as many, laid out whole.
func TestAFormulaIsLaidOutInLinearTime(t *testing.T) {
	small, set := mathRowDoc(t, 40)
	large, _ := mathRowDoc(t, 160)
	w, _ := style.FromPx(1e6)
	run := func(b Built) func() {
		return func() { Layout(b.Root, Size{W: w, H: w}, set, nil) }
	}
	if r := costtest.Time(t, "a row of operators", run(small), run(large)); r.Ratio > 8 {
		t.Errorf("four times the row cost %.1f times as much; linear is about four\n%v", r.Ratio, r)
	}
	if ratio := costtest.Allocated(t, "a row of operators", run(small), run(large)); ratio > 8 {
		t.Errorf("four times the row allocated %.1f times as much", ratio)
	}
}

// TestAnOperatorsPropertiesAreFoundInLinearTime: every operator's form asks
// where it stands among its row's children that are not space-like, and the
// row answers once. Asked of each operator by walking the row, it is the
// square of the row — which the layout of a whole formula hides behind the
// cost of shaping each token, so it is timed here on its own: each operator of
// a row asked for its properties, by a layout run that has asked nothing yet.
func TestAnOperatorsPropertiesAreFoundInLinearTime(t *testing.T) {
	ask := func(n int) func() {
		b, set := mathRowDoc(t, n)
		var math *Box
		var walk func(*Box)
		walk = func(x *Box) {
			if isMathMLRoot(x.Element) {
				math = x
			}
			for _, c := range x.Children {
				walk(c)
			}
		}
		walk(b.Root)
		return func() {
			l := newLayouter(b.Root, A4.Content(), set, nil)
			for _, k := range math.Children {
				l.mathOperator(k)
			}
		}
	}
	if r := costtest.Time(t, "the operators of a row", ask(500), ask(2000)); r.Ratio > 8 {
		t.Errorf("four times the operators cost %.1f times as much; linear is about four\n%v", r.Ratio, r)
	}
}
