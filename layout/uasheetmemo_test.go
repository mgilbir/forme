package layout

import (
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/style"
)

// The default stylesheet is fourteen kilobytes of the same CSS for every
// document, and it was read again for every one.
//
// Two things have to hold for the memo to be a memo of a pure function rather
// than a change of behaviour: the reading happens once, and everything the
// reading *produced* — a finding, an @font-face, an @page — still reaches each
// document. They are separate tests because they fail separately.

// TestTheUserAgentSheetIsReadOnce.
//
// Pointer identity of the rule slice, because that is the fact: a second parse
// would produce an equal slice and a different one, and "equal" is what a
// weaker assertion here would have accepted.
func TestTheUserAgentSheetIsReadOnce(t *testing.T) {
	a, b := parsedUserAgentCSS(), parsedUserAgentCSS()
	if len(a.rules) == 0 {
		t.Fatal("the default stylesheet parsed to no rules at all")
	}
	if &a.rules[0] != &b.rules[0] {
		t.Error("the default stylesheet was parsed twice; the memo is not one")
	}
	// And a Build does not read it again either, which is the call that used to.
	Build(Input{HTML: `<p>x</p>`})
	if c := parsedUserAgentCSS(); &a.rules[0] != &c.rules[0] {
		t.Error("a Build re-read the default stylesheet")
	}
}

// TestBuildingDoesNotChangeTheSheetItShares is the hazard a memo brings with
// it: the rules are one slice now, handed to every document, and a stage that
// wrote through it would style the next document with the last one's changes.
//
// The signature is taken across four documents chosen to reach the parts that
// take rules apart — a nested at-rule, a pseudo-element, a table and a form —
// and any difference at all fails.
func TestBuildingDoesNotChangeTheSheetItShares(t *testing.T) {
	before := sheetSignature(parsedUserAgentCSS().rules)
	for _, src := range []string{
		`<p>x</p>`,
		`<style>@media print { p::before { content: "a" } }</style><p>x</p>`,
		`<table><tr><td>x</td></tr></table>`,
		`<form><input type="text"><select><option>a</option></select></form>`,
	} {
		Build(Input{HTML: src})
		if got := sheetSignature(parsedUserAgentCSS().rules); got != before {
			t.Fatalf("building %q changed the shared default stylesheet", src)
		}
	}
}

// sheetSignature is a rule list written out far enough to notice a change.
func sheetSignature(rules []css.Rule) string {
	out := make([]byte, 0, 4096)
	for _, r := range rules {
		out = append(out, r.Name...)
		out = append(out, byte('a'+len(r.Prelude)%26), byte('a'+len(r.Block)%26))
		if r.At {
			out = append(out, '@')
		}
		if r.HasBlock {
			out = append(out, '{')
		}
		out = append(out, byte(r.Offset), byte(r.Offset>>8), '|')
	}
	return string(out)
}

// TestReadingOnceStillReportsEveryTime.
//
// The memo is of the reading; the reporting is per document. A sheet read once
// that also *reported* once would put a finding about the default stylesheet on
// whichever document happened to be built first in the process, and on no
// other — which is worse than not reporting it at all, because it would look
// like a fault in that document.
//
// The default sheet raises none of these today, so it is asked of handOver
// directly with a reading that does: a vacuous version of this test would pass
// on a memo that swallowed all three.
func TestReadingOnceStillReportsEveryTime(t *testing.T) {
	read := readSheet(style.OriginUserAgent, "invented",
		`@font-face { font-family: X; src: url(x.ttf) } `+
			`@page { margin-top: 1px } `+
			`p { color: red } )`)
	if len(read.errs) == 0 || len(read.faces) == 0 || len(read.pages) == 0 {
		t.Fatalf("the fixture produced %d errors, %d faces and %d pages; it does "+
			"not reach the three things handOver replays",
			len(read.errs), len(read.faces), len(read.pages))
	}
	for i := 0; i < 2; i++ {
		rec := NewRecorder(nil)
		var faces []pendingFontFace
		var pages []pendingPage
		read.handOver(rec, style.OriginUserAgent, "invented", &faces, &pages)
		if n := len(rec.Findings()); n != len(read.errs) {
			t.Errorf("document %d got %d findings, want %d", i, n, len(read.errs))
		}
		if len(faces) != len(read.faces) || len(pages) != len(read.pages) {
			t.Errorf("document %d got %d faces and %d pages, want %d and %d",
				i, len(faces), len(pages), len(read.faces), len(read.pages))
		}
	}
}
