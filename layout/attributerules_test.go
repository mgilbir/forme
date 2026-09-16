package layout

import "testing"

// HTML's rendering section gives a user agent stylesheet, and part of it is
// written in attribute selectors: the ways a document said "hide this",
// "number this in roman numerals" and "centre this" before there was CSS to say
// it with. Those rules were missing, so every one of those documents said
// nothing at all.
//
// They are in the user agent sheet rather than in the presentational-hint table
// because that is where HTML puts them, and because the difference is not
// observable: a hint carries zero specificity in the author origin and so loses
// to every author declaration, and a user agent rule loses to them too. What a
// hint would beat is a user agent rule, and there is none of those to beat here.

// styleOfID lays a document out and answers one computed property of #d, or
// says there was no box.
func styleOfID(t *testing.T, markup, property string) (string, bool) {
	t.Helper()
	built := Build(Input{HTML: markup})
	var found *Box
	var walk func(*Box)
	walk = func(b *Box) {
		if b == nil || found != nil {
			return
		}
		if b.Element != nil {
			if id, _ := b.Element.Attr("id"); id == "d" {
				found = b
				return
			}
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	if found == nil {
		return "", false
	}
	return found.Style[property], true
}

// TestTheHiddenAttributeHides, §15.3.1.
//
// It was not read at all, so "<div hidden>" was a visible div — which is the
// ordinary way a document hides something and so the one this engine was most
// likely to meet.
func TestTheHiddenAttributeHides(t *testing.T) {
	for _, markup := range []string{
		`<div id="d" hidden>x</div>`,
		`<div id="d" hidden="">x</div>`,
		`<div id="d" hidden="hidden">x</div>`,
		`<span id="d" hidden>x</span>`,
	} {
		if _, ok := styleOfID(t, markup, "display"); ok {
			t.Errorf("%s produced a box", markup)
		}
	}
	// "until-found" means hidden until the browser's own search finds it, and
	// there is nothing to search a printed page with — so the element is laid
	// out, which is what a browser shows once the search has found it. The
	// value is compared case-insensitively, which is HTML's own flag.
	for _, markup := range []string{
		`<div id="d" hidden="until-found">x</div>`,
		`<div id="d" hidden="UNTIL-FOUND">x</div>`,
	} {
		if _, ok := styleOfID(t, markup, "display"); !ok {
			t.Errorf("%s produced no box; until-found is not hidden on paper", markup)
		}
	}
	// And it is a user agent rule, so an author sheet beats it. That is the
	// half that says it is a default and not a law.
	if got, ok := styleOfID(t, `<style>#d { display: block }</style><div id="d" hidden>x</div>`,
		"display"); !ok || got != "block" {
		t.Errorf("an author's display:block lost to [hidden]: %q, %v", got, ok)
	}
}

// TestTheListTypeAttributeChoosesTheCounter, §15.3.7.
//
// Nine rules, and the interesting half is the flags: the ordered ones carry "s"
// because "a" and "A" are two different numberings, and "type" is one of the
// attributes HTML otherwise compares case-insensitively. Without the flag the
// two are one selector.
func TestTheListTypeAttributeChoosesTheCounter(t *testing.T) {
	for _, c := range []struct{ markup, want string }{
		{`<ol id="d" type="1"><li>x</li></ol>`, "decimal"},
		{`<ol id="d" type="a"><li>x</li></ol>`, "lower-alpha"},
		{`<ol id="d" type="A"><li>x</li></ol>`, "upper-alpha"},
		{`<ol id="d" type="i"><li>x</li></ol>`, "lower-roman"},
		{`<ol id="d" type="I"><li>x</li></ol>`, "upper-roman"},
		{`<ol><li id="d" type="A">x</li></ol>`, "upper-alpha"},
		// The unordered ones are keywords rather than a case distinction, so
		// they carry "i" instead.
		{`<ul id="d" type="none"><li>x</li></ul>`, "none"},
		{`<ul id="d" type="disc"><li>x</li></ul>`, "disc"},
		{`<ul id="d" type="circle"><li>x</li></ul>`, "circle"},
		{`<ul id="d" type="square"><li>x</li></ul>`, "square"},
		{`<ul id="d" type="SQUARE"><li>x</li></ul>`, "square"},
		{`<ul><li id="d" type="Circle">x</li></ul>`, "circle"},
		// And the defaults are untouched.
		{`<ol id="d"><li>x</li></ol>`, "decimal"},
		{`<ul id="d"><li>x</li></ul>`, "disc"},
		// A value that is not one of them changes nothing.
		{`<ol id="d" type="florb"><li>x</li></ol>`, "decimal"},
	} {
		got, ok := styleOfID(t, c.markup, "list-style-type")
		if !ok || got != c.want {
			t.Errorf("%s gave %q, want %q", c.markup, got, c.want)
		}
	}
}

// TestTheAlignAttributeAligns, §15.3.2.
//
// The element lists are HTML's own and are not a family that can be shortened:
// <legend> and <caption> are deliberately absent from all four, and a <table
// align> is not alignment at all but a float — which this does not do, and
// which is why <table> is not here either.
func TestTheAlignAttributeAligns(t *testing.T) {
	for _, c := range []struct{ markup, want string }{
		{`<p id="d" align="right">x</p>`, "right"},
		{`<p id="d" align="left">x</p>`, "left"},
		{`<p id="d" align="justify">x</p>`, "justify"},
		{`<p id="d" align="center">x</p>`, "center"},
		{`<div id="d" align="center">x</div>`, "center"},
		// "middle" is centre, and only on a <div>: it is the one value the
		// list for "center" carries that the other three lists do not.
		{`<div id="d" align="middle">x</div>`, "center"},
		{`<h3 id="d" align="right">x</h3>`, "right"},
		{`<table><tr><td id="d" align="right">x</td></tr></table>`, "right"},
		{`<table><tr id="d" align="right"><td>x</td></tr></table>`, "right"},
		// The value is an enumerated one, so its case does not matter.
		{`<p id="d" align="RIGHT">x</p>`, "right"},
		{`<p id="d" align="Center">x</p>`, "center"},
		// And a value that is not one of them changes nothing.
		{`<p id="d" align="florb">x</p>`, "start"},
		{`<p id="d">x</p>`, "start"},
	} {
		got, ok := styleOfID(t, c.markup, "text-align-all")
		if !ok || got != c.want {
			t.Errorf("%s gave %q, want %q", c.markup, got, c.want)
		}
	}
	// An author sheet beats it, as with every rule in here.
	if got, _ := styleOfID(t, `<style>#d { text-align: left }</style><p id="d" align="right">x</p>`,
		"text-align-all"); got != "left" {
		t.Errorf("an author's text-align lost to align=right: %q", got)
	}
}

// TestTheAlignAttributeOnARule, §15.3.6, where align is not about the text but
// about which way the rule is pushed.
//
// The <hr> itself gains the auto margins HTML gives it, which is what the three
// align rules override and what makes a narrowed rule sit in the middle.
func TestTheAlignAttributeOnARule(t *testing.T) {
	for _, c := range []struct{ markup, left, right string }{
		{`<hr id="d">`, "auto", "auto"},
		{`<hr id="d" align="left">`, "0", "auto"},
		{`<hr id="d" align="right">`, "auto", "0"},
		{`<hr id="d" align="center">`, "auto", "auto"},
		{`<hr id="d" align="RIGHT">`, "auto", "0"},
	} {
		left, ok := styleOfID(t, c.markup, "margin-left")
		right, _ := styleOfID(t, c.markup, "margin-right")
		if !ok || left != c.left || right != c.right {
			t.Errorf("%s gave margins %q/%q, want %q/%q", c.markup, left, right, c.left, c.right)
		}
	}
}
