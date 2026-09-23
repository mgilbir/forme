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

// TestALegacyColourValueIsReadAsHTMLReadsIt is HTML §2.3.6's "rules for
// parsing a legacy colour value", which take almost any string: what is not a
// hexadecimal digit becomes a zero, and the rest is padded, split in three and
// read as red, green and blue. Every browser does this, so "<font
// color=ff0000>" — the most common legacy spelling, with no "#" — is red.
//
// The expected values are worked by hand from the algorithm's steps, and the
// well-known ones are the ones every browser shows ("chucknorris" is #c00000).
// This used to be a test that each of them was refused and left transparent,
// which is a page no browser draws (audit C116).
func TestALegacyColourValueIsReadAsHTMLReadsIt(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"ff0000", "#ff0000"},
		{"ffffff", "#ffffff"},
		{"chucknorris", "#c00000"},
		{"#ffff", "#ffff00"},
		{"#gggggg", "#000000"},
		{"rgb(1,2,3)", "#001030"},
		{"1", "#010000"},
		{"#12345678901", "#125690"},
		// Parts longer than eight keep their last eight (step 12), and zeros
		// every part begins with go while a part is longer than two (step 13).
		{"0123456789abcdef0123456789ab", "#23cd67"},
		{"000100020003", "#010203"},
		{"  lime  ", "lime"},
	} {
		doc := parseDoc(t, `<body bgcolor="`+tc.value+`">x</body>`)
		if got := styleOf(t, doc, nil, "body", "background-color"); got != tc.want {
			t.Errorf("bgcolor=%q: background-color came out %q, want %q",
				tc.value, got, tc.want)
		}
	}
	// And the two it refuses: nothing at all, and "transparent", which step 3
	// names.
	for _, value := range []string{"", "   ", "transparent", "TRANSPARENT"} {
		doc := parseDoc(t, `<body bgcolor="`+value+`">x</body>`)
		if got := styleOf(t, doc, nil, "body", "background-color"); got != "transparent" {
			t.Errorf("bgcolor=%q gave %q; it is not a colour", value, got)
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
