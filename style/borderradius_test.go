package style

import "testing"

// border-radius, CSS Backgrounds 3 §4.1, and css-logical §4.6's corners.

// TestTheBorderRadiusShorthandExpands is the specification's own two examples,
// and the completions its prose gives.
func TestTheBorderRadiusShorthandExpands(t *testing.T) {
	for _, tc := range []struct {
		decl           string
		tl, tr, br, bl string
	}{
		// "border-radius: 4em" is each corner "4em" both ways — computed, so
		// in pixels at the initial 16px font size.
		{"border-radius: 4em", "64px 64px", "64px 64px", "64px 64px", "64px 64px"},
		// "border-radius: 2em 1em 4em / 0.5em 3em; is equivalent to
		// top-left 2em 0.5em, top-right 1em 3em, bottom-right 4em 0.5em,
		// bottom-left 1em 3em".
		{"border-radius: 2em 1em 4em / 0.5em 3em", "32px 8px", "16px 48px", "64px 8px", "16px 48px"},
		{"border-radius: 1px 2px", "1px 1px", "2px 2px", "1px 1px", "2px 2px"},
		{"border-radius: 1px 2px 3px 4px/5% 6%", "1px 5%", "2px 6%", "3px 5%", "4px 6%"},
	} {
		cs := expandOf(t, tc.decl)
		for name, want := range map[string]string{
			"border-top-left-radius": tc.tl, "border-top-right-radius": tc.tr,
			"border-bottom-right-radius": tc.br, "border-bottom-left-radius": tc.bl,
		} {
			if got := cs.Get(name); got != want {
				t.Errorf("%s: %s is %q, want %q", tc.decl, name, got, want)
			}
		}
	}
}

// TestABorderRadiusThatIsNotOneIsDropped: §4.2's rule, that an invalid
// declaration does not exist, so the one before it stands.
func TestABorderRadiusThatIsNotOneIsDropped(t *testing.T) {
	for _, bad := range []string{
		"border-radius: -1px",
		"border-radius: 1px / 2px / 3px",
		"border-radius: 1px 2px 3px 4px 5px",
		"border-radius: / 1px",
		"border-radius: 1px /",
		"border-radius: red",
		"border-top-left-radius: 1px 2px 3px",
		"border-top-left-radius: -2px",
		"border-top-left-radius: auto",
	} {
		cs := expandOf(t, "border-radius: 9px; "+bad)
		if got := cs.Get("border-top-left-radius"); got != "9px 9px" {
			t.Errorf("after %q the top-left radius is %q, want the earlier 9px 9px", bad, got)
		}
	}
	if got := expandOf(t, "border-top-left-radius: 3px").Get("border-top-left-radius"); got != "3px" {
		t.Errorf("a one-value longhand computed to %q", got)
	}
	if got := expandOf(t, "").Get("border-bottom-left-radius"); got != "0" {
		t.Errorf("the initial radius is %q, want 0", got)
	}
}

// TestALogicalCornerIsAPhysicalOne: css-logical §4.6's corners, named block
// side first, land on the physical corner the two sides meet at.
func TestALogicalCornerIsAPhysicalOne(t *testing.T) {
	for _, tc := range []struct {
		decl, want string
	}{
		{"border-start-start-radius: 7px", "border-top-left-radius"},
		{"border-start-end-radius: 7px", "border-top-right-radius"},
		{"border-end-start-radius: 7px", "border-bottom-left-radius"},
		{"border-end-end-radius: 7px", "border-bottom-right-radius"},
		{"direction: rtl; border-start-start-radius: 7px", "border-top-right-radius"},
		{"writing-mode: vertical-rl; border-start-start-radius: 7px", "border-top-right-radius"},
		{"writing-mode: vertical-lr; border-end-end-radius: 7px", "border-bottom-right-radius"},
	} {
		cs, _ := logicalStyle(t, tc.decl)
		if got := cs.Get(tc.want); got != "7px" {
			t.Errorf("%s: %s is %q, want 7px", tc.decl, tc.want, got)
		}
	}
}
