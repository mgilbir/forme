package style

import "testing"

// CSS Text Decoration 3 §3's four properties, as the cascade reads them. What
// is drawn is layout/emphasis.go's, and tested there.

// TestTheEmphasisGrammarsAreTextDecoration3 holds the three longhands to §3.1,
// §3.2 and §3.4.
func TestTheEmphasisGrammarsAreTextDecoration3(t *testing.T) {
	for _, c := range []struct {
		property, kept string
		valid, bad     []string
	}{
		{"text-emphasis-style", "sesame",
			[]string{"none", "filled", "open", "dot", "circle", "double-circle", "triangle",
				"sesame", "filled dot", "dot filled", "open sesame", "'x'", `"*"`, "''"},
			[]string{"none dot", "filled open", "dot circle", "open 'x'", "'x' 'y'", "red",
				"filled filled", "auto", "1px"}},
		{"text-emphasis-color", "green",
			[]string{"red", "currentcolor", "rgb(0, 0, 255)", "transparent"},
			[]string{"none", "dot", "red blue"}},
		{"text-emphasis-position", "under left",
			[]string{"over", "under", "over right", "right over", "under left", "left under"},
			[]string{"right", "left", "over under", "right left", "auto", "over right left"}},
	} {
		for _, v := range c.valid {
			if got := expandOf(t, c.property+": "+v).Get(c.property); got == "" || got != v && !sameWords(got, v) {
				t.Errorf("%s: %q was dropped or changed to %q", c.property, v, got)
			}
		}
		// An invalid declaration is dropped, and the one before it stands.
		for _, v := range c.bad {
			decl := c.property + ": " + c.kept + "; " + c.property + ": " + v
			if got := expandOf(t, decl).Get(c.property); !sameWords(got, c.kept) {
				t.Errorf("%s: %q was kept as %q", c.property, v, got)
			}
		}
	}
}

// sameWords reports whether two serialised values are the same once their
// spacing and quoting are set aside, which is all the check above needs.
func sameWords(a, b string) bool {
	norm := func(s string) string {
		out := make([]byte, 0, len(s))
		for i := 0; i < len(s); i++ {
			switch s[i] {
			case ' ', '"', '\'':
				continue
			}
			out = append(out, s[i])
		}
		return string(out)
	}
	return norm(a) == norm(b)
}

// TestTheEmphasisShorthandSetsStyleAndColour is §3.3: <'text-emphasis-style'>
// || <'text-emphasis-color'>, in either order, what it leaves out reset — and
// the position, which it does not name, left alone.
func TestTheEmphasisShorthandSetsStyleAndColour(t *testing.T) {
	for _, c := range []struct{ decl, style, colour string }{
		{"text-emphasis: dot", "dot", "currentcolor"},
		{"text-emphasis: filled dot red", "filled dot", "red"},
		{"text-emphasis: red filled dot", "filled dot", "red"},
		{"text-emphasis: 'x' blue", "'x'", "blue"},
		{"text-emphasis: none red", "none", "red"},
		// A colour alone resets the style, which turns the marks off.
		{"text-emphasis-style: dot; text-emphasis: red", "none", "red"},
		// And a style alone resets the colour.
		{"text-emphasis-color: red; text-emphasis: open", "open", "currentcolor"},
	} {
		got := expandOf(t, "text-emphasis-position: under left; "+c.decl)
		if s := got.Get("text-emphasis-style"); !sameWords(s, c.style) {
			t.Errorf("%q: the style is %q, want %q", c.decl, s, c.style)
		}
		if s := got.Get("text-emphasis-color"); !sameWords(s, c.colour) {
			t.Errorf("%q: the colour is %q, want %q", c.decl, s, c.colour)
		}
		if s := got.Get("text-emphasis-position"); !sameWords(s, "under left") {
			t.Errorf("%q: the position became %q; the shorthand does not set it", c.decl, s)
		}
	}
	// "||" takes each operand whole, so the colour cannot stand inside the
	// style's two words; and each operand once.
	for _, decl := range []string{
		"text-emphasis: filled red dot", "text-emphasis: red blue",
		"text-emphasis: dot circle", "text-emphasis: 'x' dot", "text-emphasis: dot red dot",
	} {
		got := expandOf(t, "text-emphasis: sesame green; "+decl)
		if s := got.Get("text-emphasis-style"); !sameWords(s, "sesame") {
			t.Errorf("%q was taken: the style is %q", decl, s)
		}
	}
}

// TestEmphasisInherits: all three longhands inherit, unlike a decoration.
func TestEmphasisInherits(t *testing.T) {
	doc := parseDoc(t, `<p id="t"><span id="s">x</span></p>`)
	got := Apply(doc, []Sheet{author(t,
		"#t { text-emphasis: open triangle red; text-emphasis-position: under left }")})
	s := got.Styles[elementFor(t, doc, "#s")]
	for property, want := range map[string]string{
		"text-emphasis-style":    "open triangle",
		"text-emphasis-color":    "red",
		"text-emphasis-position": "under left",
	} {
		if v := s.Get(property); !sameWords(v, want) {
			t.Errorf("a child's %s is %q, want the parent's %q", property, v, want)
		}
	}
}
