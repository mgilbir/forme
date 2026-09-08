package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// Four shorthands whose every longhand this engine already had.
//
// A shorthand the cascade does not know is not a shorthand that does nothing: it
// is an *unregistered property*, so the declaration is reported as unimplemented
// and dropped. "place-items: center" and "columns: 3 12em" were both of those,
// while the two longhands each stands for were registered, read and acted on —
// so the same page written one way worked and written the other did not.
//
// The rule is worth stating rather than the four cases: a shorthand every
// longhand of which is implemented has to be expanded, because a document that
// writes it is asking for something this engine can do.

// computed returns one element's computed styles under a declaration.
func computedUnder(t *testing.T, decl string) ComputedStyle {
	t.Helper()
	rules, errs := css.ParseStylesheet("#e { " + decl + " }")
	if len(errs) > 0 {
		t.Fatalf("%q does not parse: %v", decl, errs)
	}
	doc, _, _ := html.Parse(`<div id="e">x</div>`)
	out := Apply(doc, []Sheet{{Rules: rules, Name: "a.css"}})
	var got ComputedStyle
	doc.Walk(func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Name == "div" {
			got = out.Styles[n]
		}
		return true
	})
	return got
}

// reportsUnimplemented reports whether a declaration was refused as a property
// this engine does not have.
func reportsUnimplemented(t *testing.T, decl string) bool {
	t.Helper()
	rules, _ := css.ParseStylesheet("#e { " + decl + " }")
	doc, _, _ := html.Parse(`<div id="e">x</div>`)
	for _, f := range Apply(doc, []Sheet{{Rules: rules}}).Findings {
		if strings.Contains(f.Message, "is not implemented") {
			return true
		}
	}
	return false
}

// TestAShorthandSetsWhatItsLonghandsWouldHave.
func TestAShorthandSetsWhatItsLonghandsWouldHave(t *testing.T) {
	for _, tc := range []struct{ short, long string }{
		// Box Alignment's three: the block axis first, then the inline one,
		// which is "gap"'s order and "margin"'s.
		{"place-items: center start", "align-items: center; justify-items: start"},
		{"place-items: center", "align-items: center; justify-items: center"},
		{"place-content: space-between center",
			"align-content: space-between; justify-content: center"},
		{"place-self: center stretch", "align-self: center; justify-self: stretch"},

		// Multi-column's, told apart by type rather than by position — so the
		// two orders are the same declaration.
		{"columns: 3 12em", "column-count: 3; column-width: 12em"},
		{"columns: 12em 3", "column-count: 3; column-width: 12em"},
		{"columns: 3", "column-count: 3; column-width: auto"},
		{"columns: 12em", "column-count: auto; column-width: 12em"},
		{"columns: auto", "column-count: auto; column-width: auto"},
		{"columns: auto 12em", "column-count: auto; column-width: 12em"},
		{"columns: 3 auto", "column-count: 3; column-width: auto"},
	} {
		short, long := computedUnder(t, tc.short), computedUnder(t, tc.long)
		for _, name := range []string{
			"align-items", "justify-items", "align-content", "justify-content",
			"align-self", "justify-self", "column-count", "column-width",
		} {
			if short[name] != long[name] {
				t.Errorf("%q gave %s=%q and %q gave %q",
					tc.short, name, short[name], tc.long, long[name])
			}
		}
		if reportsUnimplemented(t, tc.short) {
			t.Errorf("%q was reported as a property this engine does not have", tc.short)
		}
	}
}

// TestAShorthandThisCannotReadWholeIsRefused. Half a shorthand is not what was
// asked for: a value the expander cannot take apart is dropped and reported,
// rather than half-applied.
func TestAShorthandThisCannotReadWholeIsRefused(t *testing.T) {
	for _, decl := range []string{
		"columns: 3 4",        // two counts, and no width
		"columns: 12em 3em",   // two widths
		"columns: auto auto",  // one slot named twice and neither of them
		"columns: 3 12em 4em", // three values
		"columns: solid",      // not a value of either half
		"columns: 1.5",        // a number that is not an integer
		"place-items: a b c",  // three values in a two-slot shorthand
	} {
		got := computedUnder(t, decl)
		if got["column-count"] != "auto" && got["column-count"] != "" {
			t.Errorf("%q set column-count to %q", decl, got["column-count"])
		}
		if got["column-width"] != "auto" && got["column-width"] != "" {
			t.Errorf("%q set column-width to %q", decl, got["column-width"])
		}
	}
	// The control: the same shorthand written correctly is not refused.
	if got := computedUnder(t, "columns: 3 12em"); got["column-count"] != "3" {
		t.Errorf("the control set column-count to %q, so the cases above prove "+
			"nothing", got["column-count"])
	}
}
