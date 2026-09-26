package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/style"
)

// Syntax white space is the specification's and not Unicode's: see
// internal/ascii/space.go and cmd/whitespace_test.go.
//
// Four characters stand in for the ones Unicode calls white space and CSS and
// HTML do not — the no-break space, the em space, the ideographic space — and
// the byte order mark, which neither calls white space and which the tests
// hold to the same answer so that nobody "fixes" it into one.
var notSyntaxSpace = []struct{ ch, name string }{
	{"\u00a0", "a no-break space"},
	{"\u2003", "an em space"},
	{"\u3000", "an ideographic space"},
	{"\ufeff", "a byte order mark"},
}

// TestAKeywordReaderTakesCSSWhiteSpaceOnly asks the readers layout reads its
// keywords with. The cascade's grammar drops "border-style: solid\u2003" before
// it gets here — the value is one identifier, and not a keyword — so these are
// the readers' own answer, which is what a value that reaches them by another
// path gets: a presentational hint, the user agent's sheet, a caller's style.
func TestAKeywordReaderTakesCSSWhiteSpaceOnly(t *testing.T) {
	readers := []struct {
		name string
		read func(string) bool // whether the value was read as the keyword
		word string
	}{
		{"parseDisplay", func(v string) bool { d := parseDisplay(v); return d.outer == OuterInline && d.inner == InnerFlowRoot }, "inline-block"},
		{"parseBorderStyle", func(v string) bool { return parseBorderStyle(v) == parseBorderStyle("solid") }, "solid"},
		{"isBold", isBold, "bold"},
		{"isItalic", isItalic, "italic"},
		{"objectFitOf", func(v string) bool { f, ok := objectFitOf(v); return ok && f == objectContain }, "contain"},
		{"noBorder", noBorder, "hidden"},
		{"markerText", func(v string) bool { return markerText(v, 1) == markerText("square", 1) }, "square"},
		{"paragraph.WhiteSpaceOf", func(v string) bool { return paragraph.WhiteSpaceOf(v) == paragraph.WhiteSpaceOf("preserve") }, "preserve"},
		{"paragraph.TransformOf", func(v string) bool { return paragraph.TransformOf(v) == paragraph.TransformOf("uppercase") }, "uppercase"},
	}
	for _, r := range readers {
		// CSS's own white space round the keyword is still only white space.
		for _, v := range []string{r.word, " " + r.word + " ", "\t" + r.word + "\n", strings.ToUpper(r.word)} {
			if !r.read(v) {
				t.Errorf("%s(%q) did not read %s", r.name, v, r.word)
			}
		}
		for _, s := range notSyntaxSpace {
			for _, v := range []string{r.word + s.ch, s.ch + r.word} {
				if r.read(v) {
					t.Errorf("%s(%q) read %s: %s is part of the identifier, and "+
						"the identifier is not the keyword", r.name, v, r.word, s.name)
				}
			}
		}
	}
}

// TestAQuotedFamilyNameKeepsItsSpaces. A family name in quotes is the string,
// and a string's characters are the name's: 'Courier\u2003' is a family nobody
// has, and the text falls back. It was trimmed by Unicode's white space and
// set in Courier.
func TestAQuotedFamilyNameKeepsItsSpaces(t *testing.T) {
	faceOf := func(family string) string {
		root := layoutOf(t, 600, `<p style="font-family: `+family+`">x</p>`)
		var runs []TextRun
		var walk func(*Fragment)
		walk = func(f *Fragment) {
			for _, l := range f.Lines {
				runs = append(runs, l.Runs...)
			}
			for _, c := range f.Children {
				walk(c)
			}
		}
		walk(root)
		if len(runs) == 0 || runs[0].Face == nil {
			t.Fatalf("font-family %q set no text", family)
		}
		return runs[0].Face.Name()
	}
	courier := faceOf("Courier")
	if got := faceOf(" Courier\t"); got != courier {
		t.Errorf("Courier with CSS white space round it was set in %s, want %s", got, courier)
	}
	for _, s := range notSyntaxSpace {
		if got := faceOf("'Courier" + s.ch + "'"); got == courier {
			t.Errorf("'Courier' with %s after it was set in %s, which is the family without it", s.name, got)
		}
	}
}

// TestANoBreakSpaceBetweenBlocksIsALine. White space between two blocks makes
// no box when it collapses away, and the white space that does is CSS Text's
// document white space: spaces, tabs and segment breaks. A no-break space is
// text that happens to be blank — the author wrote it to hold a line open — and
// it was dropped as if it were the newline between two tags.
func TestANoBreakSpaceBetweenBlocksIsALine(t *testing.T) {
	anon := func(between string) bool {
		return strings.Contains(bodyBoxes(t, `<div><p>a</p>`+between+`<p>b</p></div>`), "anonymous block")
	}
	for _, between := range []string{" ", "\n\t \r\n", ""} {
		if anon(between) {
			t.Errorf("%q between two blocks made a line", between)
		}
	}
	for _, s := range notSyntaxSpace {
		if !anon(s.ch) {
			t.Errorf("%s between two blocks made no line", s.name)
		}
	}
}

// TestAnAltOfANoBreakSpaceIsText. An empty alt says the image is decoration;
// one holding a no-break space says something, and a broken image shows it.
func TestAnAltOfANoBreakSpaceIsText(t *testing.T) {
	for _, alt := range []string{"", " ", "\n"} {
		if got := bodyBoxes(t, `<img alt="`+alt+`" src="missing.png">`); strings.Contains(got, "text") {
			t.Errorf("alt=%q gave the image text:\n%s", alt, got)
		}
	}
	for _, s := range notSyntaxSpace {
		if got := bodyBoxes(t, `<img alt="`+s.ch+`" src="missing.png">`); !strings.Contains(got, "text") {
			t.Errorf("an alt of %s gave the image no text:\n%s", s.name, got)
		}
	}
}

// TestASpanIsReadByHTMLsIntegerRules. §2.3.4.1 skips ASCII white space before
// the digits and nothing else; "\u00a02" is no number, and a colspan that is no
// number is one.
func TestASpanIsReadByHTMLsIntegerRules(t *testing.T) {
	span := func(v string) int {
		b := &Box{Element: &html.Node{Type: html.ElementNode, Name: "td",
			Attrs: []html.Attribute{{Name: "colspan", Value: v}}}}
		return spanValue(b, "colspan", 1000)
	}
	for _, v := range []string{"2", " 2", "\t\n\f\r2", "2 ", "2px"} {
		if got := span(v); got != 2 {
			t.Errorf("colspan=%q spans %d, want 2", v, got)
		}
	}
	for _, s := range notSyntaxSpace {
		if got := span(s.ch + "2"); got != 1 {
			t.Errorf("colspan of %s then 2 spans %d, want 1", s.name, got)
		}
	}
}

// TestAnInputTypeIsAKeywordAsWritten. The type attribute is enumerated: HTML
// matches its keywords ASCII case-insensitively and does not trim, so a type
// with white space round it is no keyword and the input is the text field its
// invalid value default makes it, which is what a browser draws.
func TestAnInputTypeIsAKeywordAsWritten(t *testing.T) {
	typeOf := func(v string) string {
		return inputTypeOf(&html.Node{Type: html.ElementNode, Name: "input",
			Attrs: []html.Attribute{{Name: "type", Value: v}}})
	}
	if got := typeOf("CheckBox"); got != "checkbox" {
		t.Errorf("type=CheckBox is %q, want checkbox", got)
	}
	for _, v := range []string{" checkbox", "checkbox\n", "checkbox\u00a0", "\u3000checkbox"} {
		if got := typeOf(v); got != "text" {
			t.Errorf("type=%q is %q, want text", v, got)
		}
	}
}

// TestAGridAreaNameHoldsWhatIsNotCSSWhiteSpace. CSS Grid §7.3 tokenizes a
// grid-template-areas string into names made of ident code points, separated
// by white space, and a no-break space or an em space is an ident code point:
// "a\u2003b c" is two cells, one named "a\u2003b", and not three. Split by
// Unicode's white space the row had a third column, and the item named c was
// placed in it with nothing said.
//
// What the engine does with the two cells is not this test's question. It
// reads area names in ASCII only (isAreaName), so it refuses the template and
// reports it, which is a gap of its own; the assertion is only that c is not
// in the third column a split would have made.
func TestAGridAreaNameHoldsWhatIsNotCSSWhiteSpace(t *testing.T) {
	xOfC := func(row string) style.Unit {
		root := layoutOf(t, 600, `<div style="display: grid; grid-template-columns: 50px 50px; grid-template-areas: '`+row+`'">`+
			`<p style="grid-area: c; margin: 0">x</p></div>`)
		f := findFragment(t, root, func(f *Fragment) bool {
			return f.Box != nil && f.Box.Element != nil && f.Box.Element.Name == "p"
		})
		return f.BorderRect.X
	}
	split := xOfC("a\tb c")
	if second := xOfC("ab c"); second == split {
		t.Fatalf("c is at x=%v in the second column and in the third: this test can tell nothing apart", split)
	}
	for _, s := range notSyntaxSpace {
		if got := xOfC("a" + s.ch + "b c"); got == split {
			t.Errorf("with %s between a and b, c is at x=%v, the third column: the row was split there", s.name, got)
		}
	}
}

// TestARelIsSplitOnASCIIWhiteSpace. rel is a set of space-separated tokens,
// split on HTML's white space: "stylesheet\u00a0" is one token, and not the
// keyword, and "alternate\u3000stylesheet" is one token that is neither.
func TestARelIsSplitOnASCIIWhiteSpace(t *testing.T) {
	for _, rel := range []string{"stylesheet", "\tStyleSheet\n", "icon\fstylesheet", "preload\r\nstylesheet"} {
		if !relIsStylesheet(rel) {
			t.Errorf("rel=%q is not a stylesheet", rel)
		}
	}
	for _, s := range notSyntaxSpace {
		if relIsStylesheet("stylesheet" + s.ch) {
			t.Errorf("stylesheet with %s after it was read as the keyword", s.name)
		}
		if !relIsStylesheet("stylesheet alternate" + s.ch) {
			t.Errorf("alternate with %s after it was read as the keyword", s.name)
		}
	}
}
