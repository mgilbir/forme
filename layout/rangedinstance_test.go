package layout

import (
	"testing"

	"github.com/mgilbir/forme/shape"
)

// TestEachRuleOfOneVariableFileIsSetAtItsOwnInstance is the case the per-box
// memory of instances (boxFamilies) has to tell apart.
//
// Two @font-face rules load the same variable file — so the same face — for
// two ranges, and set it with different font-variation-settings descriptors:
// the first at a weight of 300, the second at 700. The face is the same and the
// instance is not, so remembering an instance by its face alone sets the whole
// paragraph at whichever weight its first character found. The letters are
// interleaved within a word so that one box's walk meets both rules.
func TestEachRuleOfOneVariableFileIsSetAtItsOwnInstance(t *testing.T) {
	resolver, err := NewDirResolver("../fonts/notosans")
	if err != nil {
		t.Fatal(err)
	}
	const doc = `<style>
@font-face { font-family: Split; src: url(NotoSans-Variable.ttf); font-variation-settings: "wght" 300; unicode-range: U+0-7F }
@font-face { font-family: Split; src: url(NotoSans-Variable.ttf); font-variation-settings: "wght" 700; unicode-range: U+80-24F }
p { font-family: Split, serif }
</style><p>oÒoÒ ÒoÒo</p>`
	out := Compose(Input{HTML: doc, Resources: resolver}, Options{Page: A4})
	faces := map[string]*shape.Face{}
	for _, op := range out.Ops {
		if o, ok := op.(DrawText); ok {
			for _, r := range o.Text {
				switch r {
				case 'o', 'Ò':
					if f, seen := faces[string(r)]; seen && f != o.Face {
						t.Fatalf("%q is set in two faces", r)
					}
					faces[string(r)] = o.Face
				}
			}
		}
	}
	light, heavy := faces["o"], faces["Ò"]
	if light == nil || heavy == nil {
		t.Fatalf("the paragraph did not draw both letters: %v", faces)
	}
	if light == heavy {
		t.Fatalf("o and Ò are set in the same face, %s; the two rules clamp the "+
			"weight at 300 and at 700, which are two instances", light.Name())
	}
	lw, _ := light.Advance('o')
	hw, _ := heavy.Advance('o')
	if lw >= hw {
		t.Errorf("the o of the rule at 300 is %v wide and that of the rule at 700 %v; "+
			"the lighter instance has to be the narrower", lw, hw)
	}
}
