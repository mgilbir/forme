package style

import "testing"

// TestTheTransformGrammarIsCSSTransforms checks CSS Transforms 1 §6 and §12's
// grammar, with CSS Transforms 2's 3D functions and percentages in a scale:
// every function is valid CSS whether or not layout applies it, and what is
// not CSS is dropped so that the declaration before it stands.
func TestTheTransformGrammarIsCSSTransforms(t *testing.T) {
	for _, v := range []string{
		"none", "translate(10px)", "translate(10px, 50%)", "translate(0)", "translateX(-1em)",
		"translateY(calc(10% + 2px))", "scale(2)", "scale(2, 0.5)", "scale(50%)", "scaleX(-1)",
		"scaleY(1.5)", "rotate(90deg)", "rotate(0)", "rotate(0.25turn)", "rotate(-100grad)",
		"rotate(1.5rad)", "skew(10deg)", "skew(10deg, 0)", "skewX(5deg)", "skewY(0)",
		"matrix(1, 0, 0, 1, 10, 20)", "ROTATE(90DEG)",
		"translate(10px) rotate(90deg) scale(2)",
		"matrix3d(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1)",
		"translate3d(1px, 2%, 3px)", "translateZ(1px)", "scale3d(1, 2, 3)", "scaleZ(2)",
		"rotate3d(0, 0, 1, 90deg)", "rotateX(10deg)", "rotateY(0)", "rotateZ(90deg)",
		"perspective(100px)", "perspective(none)",
	} {
		if got := expandOf(t, "transform: scale(9); transform: "+v).Get("transform"); got == "scale(9)" {
			t.Errorf("%q was dropped", v)
		}
	}
	for _, v := range []string{
		"translate()", "translate(1px, 2px, 3px)", "translate(1px 2px)", "translate(10deg)",
		"scale(1px)", "scale()", "rotate(90)", "rotate(10px)", "skew(1deg, 2deg, 3deg)",
		"matrix(1, 0, 0, 1, 10)", "matrix(1, 0, 0, 1, 10px, 0)", "none rotate(90deg)",
		"rotate(90deg),scale(2)", "translate3d(1px, 2px, 3%)", "rotate3d(0, 0, 1)",
		"perspective(-1px)", "shake(2px)", "rotate", "translate(1px,)",
	} {
		if got := expandOf(t, "transform: scale(9); transform: "+v).Get("transform"); got != "scale(9)" {
			t.Errorf("%q was kept as %q", v, got)
		}
	}
}

// TestTheTransformOriginGrammarIsCSSTransforms checks §7's three forms: one
// position, a horizontal and a vertical one with an optional depth, and two
// keywords in either order with an optional depth.
func TestTheTransformOriginGrammarIsCSSTransforms(t *testing.T) {
	for _, v := range []string{
		"center", "left", "bottom", "10px", "50%", "10px 20%", "left top", "top left",
		"right 10px", "10px bottom", "center center 5px", "bottom right 0", "0 0",
	} {
		if got := expandOf(t, "transform-origin: 1px 2px; transform-origin: "+v).Get("transform-origin"); got == "1px 2px" {
			t.Errorf("%q was dropped", v)
		}
	}
	for _, v := range []string{
		"top 10px", "left right", "top bottom", "10px 20px 30%", "10px 20px 30px 40px",
		"middle", "10deg", "left 10px top 5px",
	} {
		if got := expandOf(t, "transform-origin: 1px 2px; transform-origin: "+v).Get("transform-origin"); got != "1px 2px" {
			t.Errorf("%q was kept as %q", v, got)
		}
	}
}
