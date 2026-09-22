package style

import "testing"

// A two-value background-repeat inside the "background" shorthand.
//
// css-backgrounds-3 §3.4 gives <repeat-style> as
//
//	repeat-x | repeat-y | [repeat | space | round | no-repeat]{1,2}
//
// so the pair form takes its values from the four in the brackets, and
// "repeat-x" and "repeat-y" stand outside it: each is already a pair —
// shorthand for "repeat no-repeat" and "no-repeat repeat" — and so cannot be
// half of one.
//
// The shorthand's parser is where this has teeth, because there the parts are
// identified by type rather than by position: on seeing a repeat keyword it
// looks at the next part, and if that is also a repeat keyword it takes the two
// together. Without the axis check "background: url(p.png) repeat-x no-repeat"
// would be read as a pair and quietly accepted.
//
// Nothing reached that check. It was at 0% coverage across every unit test and
// all 6253 reftest documents: the corpus writes background-repeat pairs in the
// longhand, and writes the shorthand with one repeat keyword, so the branch that
// consumes two of them had never run.
func TestATwoValueRepeatInTheBackgroundShorthand(t *testing.T) {
	for _, c := range []struct{ decl, want string }{
		{"background: url(p.png) repeat no-repeat", "repeat no-repeat"},
		{"background: url(p.png) no-repeat repeat", "no-repeat repeat"},
		{"background: url(p.png) space round", "space round"},
		{"background: url(p.png) round space", "round space"},
		{"background: url(p.png) no-repeat no-repeat", "no-repeat no-repeat"},

		// One keyword is one keyword, including an axis one, which is the
		// whole value it is allowed to be.
		{"background: url(p.png) repeat-x", "repeat-x"},
		{"background: url(p.png) repeat-y", "repeat-y"},
		{"background: url(p.png) no-repeat", "no-repeat"},

		// The second keyword is taken from the part after, not from the end,
		// so a pair still reads correctly with other parts around it.
		{"background: red url(p.png) space round fixed", "space round"},
	} {
		t.Run(c.decl, func(t *testing.T) {
			if got := expandOf(t, c.decl).Get("background-repeat"); got != c.want {
				t.Errorf("%q gave background-repeat %q, want %q", c.decl, got, c.want)
			}
		})
	}
}

// An axis keyword as half of a pair is not a repeat style — it is an invalid
// declaration, and the whole shorthand goes with it.
//
// That last part is what makes this worth pinning rather than shrugging at. An
// invalid shorthand is dropped entire, so every one of the eight longhands keeps
// whatever an earlier rule gave it; a parser that accepted the pair would set
// eight properties from a declaration the author wrote by mistake, and the one
// visible symptom would be a background image appearing where the cascade says
// there is none.
func TestAnAxisRepeatIsNotHalfAPair(t *testing.T) {
	for _, decl := range []string{
		"background: url(p.png) repeat-x no-repeat",
		"background: url(p.png) no-repeat repeat-x",
		"background: url(p.png) repeat-y repeat",
		"background: url(p.png) round repeat-y",
		"background: url(p.png) repeat-x repeat-y",
		"background: url(p.png) repeat-x repeat-x",
	} {
		t.Run(decl, func(t *testing.T) {
			// An earlier rule the shorthand must not disturb, so that "left
			// alone" and "set to the initial value" are told apart.
			cs := expandOf(t, "background-repeat: space; background-image: url(q.png); "+decl)
			if got := cs.Get("background-repeat"); got != "space" {
				t.Errorf("%q set background-repeat to %q; an axis keyword "+
					"cannot be half of a pair, so the declaration is invalid "+
					"and the earlier \"space\" stands", decl, got)
			}
			if got := cs.Get("background-image"); got != "url(q.png)" {
				t.Errorf("%q left background-image %q; the shorthand is "+
					"invalid and is dropped entire, so it resets nothing and "+
					"the earlier url(q.png) stands", decl, got)
			}
		})
	}
}
