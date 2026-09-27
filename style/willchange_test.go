package style

import "testing"

// TestWillChangeIsItsGrammar: css-will-change 1 §3's "auto |
// <animateable-feature>#", a feature being scroll-position, contents or a
// <custom-ident> other than will-change, none, all, auto, scroll-position and
// contents. layout reads it (see layout/willchange.go).
func TestWillChangeIsItsGrammar(t *testing.T) {
	for _, v := range []string{"auto", "filter", "transform, filter", "scroll-position",
		"contents", "FILTER", "opacity , contents , color", "--custom"} {
		if ok, unsupported := JudgeValue("will-change", angleValues(t, v)); !ok || unsupported != "" {
			t.Errorf("will-change: %s is %v, %q; want valid", v, ok, unsupported)
		}
	}
	for _, v := range []string{"none", "all", "will-change", "auto, filter", "filter auto",
		"filter,", ", filter", "10px", "filter transform", `"filter"`} {
		if ok, _ := JudgeValue("will-change", angleValues(t, v)); ok {
			t.Errorf("will-change: %s was taken as valid", v)
		}
	}
}
