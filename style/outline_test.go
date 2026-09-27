package style

import "testing"

// TestALoneAutoOutlineIsAutoStyleAndColour is css-ui-4 §3.1's ambiguity rule:
// "In the ambiguous case where a lone auto value is specified, or if auto is
// specified together with an <'outline-width'> value, but without an explicit
// <'outline-style'> or <'outline-color'> value, both outline-style and
// outline-color are set to auto." Beside an explicit style, "auto" is the
// colour; beside an explicit colour, it is the style.
func TestALoneAutoOutlineIsAutoStyleAndColour(t *testing.T) {
	for _, tc := range []struct{ value, style, colour, width string }{
		{"auto", "auto", "auto", "medium"},
		{"3px auto", "auto", "auto", "3px"},
		{"auto auto", "auto", "auto", "medium"},
		{"auto red", "auto", "red", "medium"},
		{"red auto", "auto", "red", "medium"},
		{"auto dashed", "dashed", "auto", "medium"},
		{"dashed auto 2px", "dashed", "auto", "2px"},
		{"2px solid", "solid", "invert", "2px"},
	} {
		css := `#p { outline: ` + tc.value + ` }`
		for property, want := range map[string]string{
			"outline-style": tc.style, "outline-color": tc.colour, "outline-width": tc.width,
		} {
			if got, findings := winner(t, css, property); got != want {
				t.Errorf("outline: %s gave %s %q, want %q (%v)", tc.value, property, got, want, findings)
			}
		}
	}
	// And an "auto" with nowhere to go is not an outline.
	for _, bad := range []string{"auto dashed red", "auto auto red", "auto auto auto", "dashed red auto"} {
		css := `#p { outline: 1px dotted blue } #p { outline: ` + bad + ` }`
		if got, _ := winner(t, css, "outline-style"); got != "dotted" {
			t.Errorf("outline: %s was taken: the style is %q", bad, got)
		}
	}
}

// TestOutlineOffsetIsALength: css-ui-4 §3.5's <length>, of either sign, whose
// computed value is an absolute length; a percentage is not one.
func TestOutlineOffsetIsALength(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"4px", "4px"}, {"-3px", "-3px"}, {"0", "0"},
	} {
		got, findings := winner(t, `#p { outline-offset: `+tc.value+` }`, "outline-offset")
		if got != tc.want {
			t.Errorf("outline-offset: %s computed %q, want %q", tc.value, got, tc.want)
		}
		for _, f := range findings {
			if f.Unsupported {
				t.Errorf("outline-offset: %s was reported: %v", tc.value, f)
			}
		}
	}
	if got, _ := winner(t, `#p { outline-offset: 2px } #p { outline-offset: 10% }`, "outline-offset"); got != "2px" {
		t.Errorf("a percentage outline-offset was taken: %q", got)
	}
	if got, _ := winner(t, `#p { outline-offset: 2px }`, "outline-offset"); got == "" {
		t.Error("outline-offset has no value")
	}
	if got, _ := winner(t, `#p { color: red }`, "outline-offset"); got != "0" {
		t.Errorf("outline-offset's initial value is %q, want 0", got)
	}
}
