package html

import (
	"strings"
	"testing"
)

// The names in a tag, read as the tokenizer's states read them.
//
// §13.2.5.8, the tag name state, has three terminators — white space, "/" and
// ">" — and three things it does with every other character: an ASCII capital
// is appended lowercased, a NUL is appended as U+FFFD with a parse error, and
// anything else is appended as it stands. The attribute name state (§13.2.5.33)
// is the same with "=" added to the terminators. Neither folds anything outside
// ASCII, and neither stops at a character just because it is not a letter.
//
// The tag name used to stop at the first byte that was not an ASCII letter,
// digit, "-", "_" or ":", so the rest of the name became an attribute; and both
// names were lowercased by strings.ToLower, which is Unicode's case mapping and
// turns a KELVIN SIGN into the letter k and a dotted capital I into two code
// points.

// firstTag is the first tag token of src and the findings reading it produced.
func firstTag(t *testing.T, src string) (token, []Error) {
	t.Helper()
	tz := newTokenizer(src, false)
	for {
		tk := tz.next()
		switch tk.kind {
		case tokStartTag, tokEndTag:
			return tk, tz.errs
		case tokEOF:
			return tk, tz.errs
		}
	}
}

func TestATagNameRunsToWhiteSpaceSlashOrGreaterThan(t *testing.T) {
	for _, tc := range []struct {
		src, name string
		attrs     []string
		what      string
	}{
		{"<ABC>", "abc", nil, "an ASCII capital is lowercased"},
		{"<aſb>", "aſb", nil, "a letter outside ASCII is part of the name"},
		{"<math-α>", "math-α", nil, "a custom element name may hold one"},
		{"<emotion-😍>", "emotion-😍", nil, "or an emoji, as the standard's own example does"},
		{"<SPAN\u212A>", "span\u212A", nil, "a KELVIN SIGN is not the letter k"},
		{"<X\u212ABD>", "x\u212Abd", nil, "so it does not make an <xkbd>"},
		{"<DİV>", "dİv", nil, "a dotted capital I is not folded, and not split in two"},
		{"<a.b>", "a.b", nil, "a full stop is part of the name"},
		{`<a"b>`, `a"b`, nil, "a quote is part of the name"},
		{"<a<b>", "a<b", nil, "a less-than sign is part of the name"},
		{"<a=b>", "a=b", nil, "an equals sign is part of a tag name"},
		{"<o:P>", "o:p", nil, "a colon is part of the name"},
		{"<a\tb>", "a", []string{"b"}, "a tab ends it"},
		{"<a\nb>", "a", []string{"b"}, "a line feed ends it"},
		{"<a\fb>", "a", []string{"b"}, "a form feed ends it"},
		{"<a b>", "a", []string{"b"}, "a space ends it"},
		{"<a/b>", "a", []string{"b"}, "a solidus ends it"},
	} {
		tk, errs := firstTag(t, tc.src)
		if tk.kind != tokStartTag || tk.name != tc.name {
			t.Errorf("%s: %q read as token %d named %q, want a start tag named %q",
				tc.what, tc.src, tk.kind, tk.name, tc.name)
			continue
		}
		var names []string
		for _, a := range tk.attrs {
			names = append(names, a.Name)
		}
		if strings.Join(names, " ") != strings.Join(tc.attrs, " ") {
			t.Errorf("%s: %q has the attributes %q, want %q", tc.what, tc.src, names, tc.attrs)
		}
		if tc.src != "<a/b>" && len(errs) != 0 {
			t.Errorf("%s: %q reported %v, which the tag name state does not", tc.what, tc.src, errs)
		}
	}
	// A NUL is U+FFFD and a parse error.
	tk, errs := firstTag(t, "<a\x00b>")
	if tk.name != "a\uFFFDb" {
		t.Errorf("a NUL in a tag name read as %q, want %q", tk.name, "a\uFFFDb")
	}
	if !hasMessage(errs, "NUL") {
		t.Errorf("a NUL in a tag name was not reported: %v", errs)
	}
}

func TestAnEndTagNameIsReadTheSameWay(t *testing.T) {
	for _, tc := range []struct{ src, name string }{
		{"</ABC>", "abc"},
		{"</aſb>", "aſb"},
		{"</X\u212ABD>", "x\u212Abd"},
		{"</a.b >", "a.b"},
	} {
		if tk, _ := firstTag(t, tc.src+"<i>"); tk.kind != tokEndTag || tk.name != tc.name {
			t.Errorf("%q read as token %d named %q, want an end tag named %q",
				tc.src, tk.kind, tk.name, tc.name)
		}
	}
	// The end tag open state: only an ASCII letter begins a name. Anything else
	// is a bogus comment, which is no tag at all, and "1x" is not a name.
	for _, src := range []string{"</1x>", "</ſpan>", "</>", "</.a>"} {
		tk, errs := firstTag(t, src+"<i>")
		if tk.kind != tokStartTag || tk.name != "i" {
			t.Errorf("%q was read as a tag named %q, want no tag before the <i>", src, tk.name)
		}
		if !hasMessage(errs, "an end tag with no name") {
			t.Errorf("%q was not reported: %v", src, errs)
		}
	}
}

func TestAnAttributeNameFoldsOnlyASCII(t *testing.T) {
	for _, tc := range []struct{ src, name, what string }{
		{"<p LANG=tr>", "lang", "an ASCII capital is lowercased"},
		{"<track \u212Aind=subtitles>", "\u212Aind", "a KELVIN SIGN is not the letter k"},
		{"<p TİTLE=x>", "tİtle", "a dotted capital I is not folded"},
		{"<p ſtyle=x>", "ſtyle", "a long s is kept"},
		{"<p a\x00b=x>", "a\uFFFDb", "a NUL is U+FFFD"},
	} {
		tk, _ := firstTag(t, tc.src)
		if len(tk.attrs) != 1 || tk.attrs[0].Name != tc.name {
			t.Errorf("%s: %q has the attributes %v, want one named %q", tc.what, tc.src, tk.attrs, tc.name)
		}
	}
}

// TestANameOutsideASCIIIsOneElement is the tree the old reading got wrong: the
// start tag "<aſb>" was an element "a" with an attribute, and its end tag an
// "</a" that closed nothing.
func TestANameOutsideASCIIIsOneElement(t *testing.T) {
	doc, errs, _ := Parse("<p>a<aſb>x</aſb>y</p>")
	el := findElement(doc, "aſb")
	if el == nil {
		t.Fatalf("no <aſb> in the tree:\n%s", tree(doc))
	}
	if len(el.Attrs) != 0 || textOf(el) != "x" {
		t.Errorf("<aſb> has the attributes %v and the text %q, want none and \"x\"", el.Attrs, textOf(el))
	}
	if findElement(doc, "a") != nil {
		t.Errorf("the name was cut at the long s:\n%s", tree(doc))
	}
	if len(errs) != 0 {
		t.Errorf("a well-formed document reported %v", errs)
	}
	// And "<X\u212ABD>" is not an <xkbd>.
	doc, _, _ = Parse("<p><X\u212ABD>x</X\u212ABD></p>")
	if findElement(doc, "xkbd") != nil || findElement(doc, "x\u212Abd") == nil {
		t.Errorf("a KELVIN SIGN in a tag name was folded:\n%s", tree(doc))
	}
}
