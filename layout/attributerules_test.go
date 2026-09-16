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

// TestTheFrameAttributeChoosesTheTablesEdges, §15.3.8.
//
// Eight values, each naming which edges of the table are drawn. They are the
// one place in HTML where a four-value border-style is the point rather than a
// shorthand's convenience: "hsides" is "outset hidden outset hidden", and it
// cannot be said with fewer.
func TestTheFrameAttributeChoosesTheTablesEdges(t *testing.T) {
	for _, c := range []struct{ frame, top, right, bottom, left string }{
		{"void", "hidden", "hidden", "hidden", "hidden"},
		{"above", "outset", "hidden", "hidden", "hidden"},
		{"below", "hidden", "hidden", "outset", "hidden"},
		{"hsides", "outset", "hidden", "outset", "hidden"},
		{"lhs", "hidden", "hidden", "hidden", "outset"},
		{"rhs", "hidden", "outset", "hidden", "hidden"},
		{"vsides", "hidden", "outset", "hidden", "outset"},
		{"box", "outset", "outset", "outset", "outset"},
		{"border", "outset", "outset", "outset", "outset"},
		// The value is compared case-insensitively, which is HTML's flag and
		// also what "frame" being an enumerated attribute already asks for.
		{"HSIDES", "outset", "hidden", "outset", "hidden"},
		// And a value that is none of them draws nothing.
		{"florb", "none", "none", "none", "none"},
	} {
		markup := `<table id="d" frame="` + c.frame + `"><tr><td>x</td></tr></table>`
		for _, side := range []struct{ name, want string }{
			{"top", c.top}, {"right", c.right}, {"bottom", c.bottom}, {"left", c.left},
		} {
			got, ok := styleOfID(t, markup, "border-"+side.name+"-style")
			if !ok || got != side.want {
				t.Errorf("frame=%q gave border-%s-style %q, want %q",
					c.frame, side.name, got, side.want)
			}
		}
	}
}

// TestTheRulesAttributeChoosesTheInnerEdges, §15.3.8.
//
// It is the other half of frame: which lines are drawn *between* the cells. The
// part worth stating is that every value of it also puts the table in the
// collapsed border model, and that is not decoration — CSS 2.1 §17.6.1 ignores
// a row's and a section's border entirely in the separated model, so
// "rules=groups" without the collapse would compute borders that nothing draws.
func TestTheRulesAttributeChoosesTheInnerEdges(t *testing.T) {
	onTable := func(value string) string {
		return `<table id="d" rules="` + value + `"><tr><td>x</td></tr></table>`
	}
	onCell := func(value string) string {
		return `<table rules="` + value + `"><tr><td id="d">x</td></tr></table>`
	}
	for _, value := range []string{"none", "groups", "rows", "cols", "all"} {
		if got, _ := styleOfID(t, onTable(value), "border-collapse"); got != "collapse" {
			t.Errorf("rules=%q left border-collapse %q; a row's border is not "+
				"drawn at all in the separated model", value, got)
		}
		if got, _ := styleOfID(t, onTable(value), "border-top-style"); got != "hidden" {
			t.Errorf("rules=%q gave the table border-top-style %q, want hidden",
				value, got)
		}
	}
	// The cells, which is where the five values differ from each other.
	for _, c := range []struct{ value, block, inline string }{
		{"none", "none", "none"},
		{"groups", "none", "none"},
		{"rows", "none", "none"},
		{"cols", "none", "solid"},
		{"all", "solid", "solid"},
	} {
		if got, _ := styleOfID(t, onCell(c.value), "border-top-style"); got != c.block {
			t.Errorf("rules=%q gave the cell border-top-style %q, want %q",
				c.value, got, c.block)
		}
		if got, _ := styleOfID(t, onCell(c.value), "border-left-style"); got != c.inline {
			t.Errorf("rules=%q gave the cell border-left-style %q, want %q",
				c.value, got, c.inline)
		}
	}
	// And a value that is none of the five says nothing at all.
	if got, _ := styleOfID(t, onTable("florb"), "border-collapse"); got != "separate" {
		t.Errorf("rules=\"florb\" left border-collapse %q, want separate", got)
	}
}

// TestRulesReachesTheRowsAndTheGroups is the half of "rules" that is not on the
// cells at all: "rows" draws a line under each row, and "groups" draws one
// around each section and between the column groups.
//
// The logical properties are the specification's own — "border-block-width" on
// a row and "border-inline-width" on a colgroup — and they are what says the
// line is drawn across the writing direction rather than on a named edge.
func TestRulesReachesTheRowsAndTheGroups(t *testing.T) {
	const markup = `<table rules="%"><colgroup id="cg"><col></colgroup>` +
		`<tbody id="tb"><tr id="r"><td>x</td></tr></tbody></table>`
	with := func(value string) string {
		out := ""
		for i := 0; i < len(markup); i++ {
			if markup[i] == '%' {
				out += value
				continue
			}
			out += string(markup[i])
		}
		return out
	}
	for _, c := range []struct{ value, id, property, want string }{
		{"rows", "r", "border-top-style", "solid"},
		{"rows", "r", "border-bottom-style", "solid"},
		{"rows", "r", "border-left-style", "none"},
		{"groups", "tb", "border-top-style", "solid"},
		{"groups", "tb", "border-left-style", "none"},
		{"groups", "cg", "border-left-style", "solid"},
		{"groups", "cg", "border-top-style", "none"},
		// And the values that say nothing about them say nothing.
		{"all", "r", "border-top-style", "none"},
		{"cols", "tb", "border-top-style", "none"},
	} {
		built := Build(Input{HTML: with(c.value)})
		var found *Box
		var walk func(*Box)
		walk = func(b *Box) {
			if b == nil || found != nil {
				return
			}
			if b.Element != nil {
				if id, _ := b.Element.Attr("id"); id == c.id {
					found = b
					return
				}
			}
			for _, k := range b.Children {
				walk(k)
			}
		}
		walk(built.Root)
		if found == nil {
			t.Fatalf("rules=%q: no box for #%s", c.value, c.id)
		}
		if got := found.Style[c.property]; got != c.want {
			t.Errorf("rules=%q gave #%s %s %q, want %q",
				c.value, c.id, c.property, got, c.want)
		}
	}
}

// TestTheBodyLinkAttributeBeatsTheDefaultBlue is the half of the link colour
// that only a document with the default stylesheet on it can show.
//
// The style package's own tests assert that the hint applied; what they cannot
// assert is what it beat, because the blue an unstyled link gets is a rule in
// layout's user agent sheet. A hint that lost to it would be a document whose
// "<body link>" did nothing and looked exactly like one that had not written it.
func TestTheBodyLinkAttributeBeatsTheDefaultBlue(t *testing.T) {
	if got, ok := styleOfID(t, `<body><a id="d" href="x">x</a></body>`, "color"); !ok || got != "#0000ee" {
		t.Fatalf("an unstyled link is %q; the fixture does not reach the rule "+
			"the attribute has to beat", got)
	}
	if got, _ := styleOfID(t, `<body link="red"><a id="d" href="x">x</a></body>`, "color"); got != "red" {
		t.Errorf("the link is %q, want red: a presentational hint beats the "+
			"user agent sheet, which is the whole of where it sits", got)
	}
	// And the underline stays, which is the other half of the default rule and
	// is not what the attribute is about.
	if got, _ := styleOfID(t, `<body link="red"><a id="d" href="x">x</a></body>`,
		"text-decoration-line"); got != "underline" {
		t.Errorf("the underline became %q; the attribute names a colour", got)
	}
}
