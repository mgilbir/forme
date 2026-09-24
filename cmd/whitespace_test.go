package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// White space in syntax is the specification's, not Unicode's.
//
// CSS Syntax 3 §4.2 makes white space a space, a tab or a newline, and Infra's
// ASCII whitespace, which HTML splits and trims on, is those with the form
// feed and carriage return. Neither holds anything outside ASCII.
//
// Go's strings.TrimSpace and strings.Fields, and the same two in package bytes,
// trim and split on unicode.IsSpace, which is Unicode's White_Space by the
// toolchain's release: U+00A0 NO-BREAK SPACE, U+2003 EM SPACE, U+3000
// IDEOGRAPHIC SPACE, U+0085 and a score more. The engine asked them some two
// hundred questions about CSS values and HTML attributes. The cascade's grammar
// hid most of them — "border-style: solid\u2003" is one identifier, and not a
// keyword, so it is dropped before layout reads it — and the rest were live:
// a quoted font family 'Courier\u2003' was set in Courier; "@media
// (orientation: portrait\u00a0)" matched a portrait page; colspan="\u00a02"
// spanned two columns where HTML's integer rules read no number; <input
// type="checkbox\u00a0"> was a checkbox; valign="top\u3000" aligned to the
// top; and a no-break space alone between two blocks, or alone in an alt, was
// taken for the newline between two tags and made no line.
//
// They go through internal/ascii now — TrimSpace and Fields for HTML's set,
// TrimCSSSpace and CSSFields for CSS's — and a text node's white space goes
// through package paragraph, whose question it is. This keeps it so: nothing
// in the engine may call the four unless it is in whitespaceExempt, which says
// why. The generators and tools under cmd/ are not held to it. They read the
// Unicode Character Database and the font and hyphenation sources, whose
// fields are not CSS or HTML, and no document reaches them.

// unicodeWhiteSpaceSplits are the functions of packages strings and bytes that
// trim or split on Unicode's white space.
var unicodeWhiteSpaceSplits = map[string]bool{"TrimSpace": true, "Fields": true}

// whitespaceExempt are the functions that trim or split on Unicode's white
// space on purpose, keyed by file and function, and why. It is empty, and an
// entry must still make such a call: an entry that has stopped is an exemption
// waiting for a use nobody meant.
var whitespaceExempt = map[string]string{}

// TestSyntaxWhiteSpaceIsTheSpecifications.
func TestSyntaxWhiteSpaceIsTheSpecifications(t *testing.T) {
	found, checked := stringsCallsIn(t, root, unicodeWhiteSpaceSplits)
	used := map[string]bool{}
	var refused []string
	for _, f := range found {
		if strings.HasPrefix(f.where, "cmd/") {
			continue
		}
		if _, ok := whitespaceExempt[f.where]; ok {
			used[f.where] = true
			continue
		}
		refused = append(refused, f.String())
	}
	sort.Strings(refused)
	for _, r := range refused {
		t.Errorf("%s: Unicode's white space, where CSS and HTML name their own; "+
			"use internal/ascii (TrimSpace and Fields for HTML, TrimCSSSpace and "+
			"CSSFields for CSS), package paragraph for a text node, or add the "+
			"function to whitespaceExempt with the reason it is neither", r)
	}
	for where, why := range whitespaceExempt {
		if !used[where] {
			t.Errorf("%s is excused from this check because it %s, and no longer "+
				"trims or splits on Unicode's white space; take it off the list", where, why)
		}
	}
	if checked < 250 {
		t.Fatalf("only %d Go files were read", checked)
	}
}

// TestTheWhiteSpaceCheckSeesACall holds the check to a tree it must refuse,
// for the reason TestTheASCIICaseCheckSeesACall does.
func TestTheWhiteSpaceCheckSeesACall(t *testing.T) {
	dir := t.TempDir()
	src := `package p

import (
	"bytes"
	str "strings"
)

func keyword(v string) string { return str.TrimSpace(v) }

type r struct{}

func (r) words(b []byte) [][]byte { return bytes.Fields(b) }

var split = str.Fields

func fine(v string) string { return str.TrimLeft(v, " ") }
`
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p_test.go"),
		[]byte("package p\n\nimport \"strings\"\n\nvar _ = strings.TrimSpace(\" \")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, checked := stringsCallsIn(t, dir, unicodeWhiteSpaceSplits)
	var got []string
	for _, f := range found {
		got = append(got, f.String())
	}
	sort.Strings(got)
	want := []string{
		"p.go:(r).words calls bytes.Fields",
		"p.go:keyword calls strings.TrimSpace",
		"p.go:package level calls strings.Fields",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || checked != 1 {
		t.Errorf("the check found, in %d files,\n  %s\nwant, in 1,\n  %s",
			checked, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
