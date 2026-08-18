package style

import "testing"

// The presentational colour attributes of HTML's rendering section.
//
// <body bgcolor="#ffff00"> maps to background-color, and <body text> to color.
// They are the oldest thing in the hint table and the only entries that are not
// a length, which is why they are read by a reader of their own.

// TestAColourAttributeSetsItsProperty.
func TestAColourAttributeSetsItsProperty(t *testing.T) {
	for _, tc := range []struct{ markup, property, want string }{
		{`<body bgcolor="#ffff00">x</body>`, "background-color", "#ffff00"},
		{`<body bgcolor="#ff0">x</body>`, "background-color", "#ff0"},
		{`<body bgcolor="yellow">x</body>`, "background-color", "yellow"},
		{`<body bgcolor="YELLOW">x</body>`, "background-color", "YELLOW"},
		{`<body text="green">x</body>`, "color", "green"},
	} {
		doc := parseDoc(t, tc.markup)
		got := styleOf(t, doc, nil, "body", tc.property)
		if got != tc.want {
			t.Errorf("%s: %s came out %q, want %q",
				tc.markup, tc.property, got, tc.want)
		}
	}
}

// TestAColourAttributeThisCannotReadIsRefused.
//
// HTML's own rule is far wider: its "legacy colour value" takes any string at
// all, strips what it cannot use and pads what is left, so "chucknorris" is a
// colour and comes out #C00000. That algorithm is deliberately not here, and
// what matters is which way the gap falls — a hint that refuses leaves the
// property at the value a document not writing the attribute would have got,
// and a hint that guesses paints the page a colour nobody asked for with nothing
// to report it.
func TestAColourAttributeThisCannotReadIsRefused(t *testing.T) {
	for _, markup := range []string{
		`<body bgcolor="chucknorris">x</body>`,
		`<body bgcolor="#ffff">x</body>`,
		`<body bgcolor="#gggggg">x</body>`,
		`<body bgcolor="">x</body>`,
		`<body bgcolor="rgb(1,2,3)">x</body>`,
	} {
		doc := parseDoc(t, markup)
		if got := styleOf(t, doc, nil, "body", "background-color"); got != "transparent" {
			t.Errorf("%s: background-color came out %q; a value this cannot read "+
				"leaves the property at its initial transparent", markup, got)
		}
	}
}

// TestAStylesheetBeatsAColourAttribute. A hint is not an inline style: it sits
// below every stylesheet declaration, which is what lets an author's rule take
// the markup's colour off.
func TestAStylesheetBeatsAColourAttribute(t *testing.T) {
	doc := parseDoc(t, `<body bgcolor="#ffff00">x</body>`)
	got := styleOf(t, doc, []Sheet{author(t, `body { background-color: lime }`)},
		"body", "background-color")
	if got != "lime" {
		t.Errorf("background-color came out %q, want the stylesheet's lime", got)
	}
}
