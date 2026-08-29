package layout

import (
	"strings"
	"testing"
)

// A <span> between two preserved spaces, under white-space: break-spaces.
//
// UAX #14's LB7 is "× SP": a line may not end in front of a space, so an
// opportunity offered to one is not offered at all. That is why an opportunity
// carried across a box boundary is withheld from a space — and break-spaces is
// the one value that overrules it. CSS Text §3 says so in the words that name
// this case:
//
//	there is a soft wrap opportunity after every preserved white space
//	character, including between white space characters
//
// SplitAtBreaks applies that inside a run, so a run of preserved spaces breaks
// wherever it has to. It could not apply it *between* two runs, because it is
// given one box's text at a time — so the answer depended on whether a <span>
// happened to fall between two of the spaces, and an empty one was enough.
//
// The fill then had nowhere to break the overflowing line and rewound to the
// last opportunity it had, which was several characters earlier. The suite
// writes it as trailing-ideographic-space-break-spaces-005 and -006: "ああ␣␣␣␣ああ"
// in three ideographs of room, which is three, three and two — and came out as
// one, three, three and one.

// spacedLines returns what each line of a box reads.
//
// Courier at 20px is 12px a character, so a box 36px wide holds three and the
// content of each line is the whole assertion.
func spacedLines(t *testing.T, body, ws string) []string {
	t.Helper()
	root := layoutOf(t, 600, `<div id="d">`+body+`</div>`, noDefaults+
		`#d { font-family: Courier; font-size: 20px; width: 36px; white-space: `+ws+` }`)
	var out []string
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "d" {
				for _, ln := range f.Lines {
					var b strings.Builder
					for _, r := range ln.Runs {
						b.WriteString(r.Text)
					}
					out = append(out, b.String())
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// TestBreakSpacesBreaksBetweenTwoSpacesInDifferentBoxes is the bug.
//
// "AA    AA" is eight characters in three of room, and §3 puts an opportunity
// after every one of the spaces: "AA ", "   ", "AA". Putting a span anywhere
// among the spaces must not change that — and an *empty* span is the sharpest
// version of it, because it contributes nothing to the line at all and still
// split the text into two boxes.
func TestBreakSpacesBreaksBetweenTwoSpacesInDifferentBoxes(t *testing.T) {
	want := []string{"AA ", "   ", "AA"}
	for _, tc := range []struct{ what, body string }{
		{"one box", `AA    AA`},
		{"a span round the spaces", `AA <span>   </span>AA`},
		{"an empty span among them", `AA <span></span>   AA`},
		{"a span round one of them", `AA <span> </span>  AA`},
		{"two spans", `AA <span> </span><span> </span> AA`},
	} {
		got := spacedLines(t, tc.body, "break-spaces")
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s: %q, want %q — §3 puts an opportunity between two "+
				"preserved spaces, and a box boundary is not one of them",
				tc.what, got, want)
		}
	}
}

// TestTheOtherWhiteSpaceValuesAreUnchangedByASpan is the containment case, and
// it is what says this rule belongs to break-spaces and nowhere else.
//
// LB7 stands for every other value: a run of preserved spaces under pre-wrap
// hangs off the end of the line as one thing and is not broken in the middle,
// and a span among them must not make it breakable. Getting this wrong is
// invisible in a document with no span in it, which is most of them.
func TestTheOtherWhiteSpaceValuesAreUnchangedByASpan(t *testing.T) {
	for _, ws := range []string{"pre-wrap", "pre", "normal", "nowrap", "pre-line"} {
		plain := spacedLines(t, `AA    AA`, ws)
		for _, tc := range []struct{ what, body string }{
			{"a span round the spaces", `AA <span>   </span>AA`},
			{"an empty span among them", `AA <span></span>   AA`},
		} {
			got := spacedLines(t, tc.body, ws)
			if strings.Join(got, "|") != strings.Join(plain, "|") {
				t.Errorf("white-space: %s with %s set %q and without it %q",
					ws, tc.what, got, plain)
			}
		}
	}
	// And the fixture really does say something: under pre-wrap the four spaces
	// stay together and hang, which is a different answer from break-spaces'.
	if got, want := spacedLines(t, `AA    AA`, "pre-wrap"), []string{"AA    ", "AA"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("pre-wrap set %q, want %q; if it agreed with break-spaces the "+
			"test above would hold whatever the rule did", got, want)
	}
}
