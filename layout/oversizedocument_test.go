package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/html"
)

// TestADocumentOverTheHTMLCapIsReportedNotFatal is this side of html.Parse's
// contract.
//
// The parser refuses a document over its byte cap and says so, and this reports
// what it says. It used to hand back no tree at all for that one input, and the
// walk below the report dereferenced it: the largest documents an engine can be
// handed were the ones that took the process down instead of being reported.
//
// The check is on the whole path — parse, style, box, compose — because a nil
// tree does not fail where it is made.
func TestADocumentOverTheHTMLCapIsReportedNotFatal(t *testing.T) {
	src := strings.Repeat("<p>text that will never be read</p>", (64<<20)/35+1)

	built := Build(Input{HTML: src})
	if built.Document == nil {
		t.Fatal("Build produced no document for an oversize input")
	}
	if !hasRule(built.Findings, RuleLimit) {
		t.Errorf("an oversize document raised %v, want the limit finding naming the cap",
			ruleNames(built.Findings))
	}
	if hasRule(built.Findings, RuleInvalidMarkup) {
		t.Error("a document over the byte cap was reported as invalid markup; " +
			"nothing is wrong with it, the engine stopped short")
	}

	composed := Compose(Input{HTML: src}, Options{})
	for _, op := range composed.Ops {
		switch op.(type) {
		case DrawText, DrawImage:
			t.Errorf("an unread document drew %T", op)
		}
	}
}

// TestABoundReachedIsALimitNotInvalidMarkup covers the rest of the class. Every
// bound the parser has says "the engine stopped short"; none of them says the
// author wrote something wrong, and an author sent to the wrong one of those
// looks for a mistake that is not there.
//
// The node cap belongs to this class too and is checked in the html package
// instead: reaching it needs a million nodes, and laying out a million nodes to
// learn how one finding is spelled is thirty seconds for nothing.
func TestABoundReachedIsALimitNotInvalidMarkup(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"the byte cap", strings.Repeat("<p>x</p>", (64<<20)/8+1)},
		{"the depth cap", strings.Repeat("<div>", 300)},
	} {
		built := Build(Input{HTML: tc.src})
		if !hasRule(built.Findings, RuleLimit) {
			t.Errorf("%s: raised %v, want a limit finding", tc.name, ruleNames(built.Findings))
		}
		for _, f := range built.Findings {
			if f.Rule == RuleInvalidMarkup && strings.Contains(f.Message, "than this engine will") {
				t.Errorf("%s: %q was reported as invalid markup", tc.name, f.Message)
			}
		}
	}
}

// TestTheEmptyFrameLaysOutAsAnEmptyPage pins what an unread document becomes,
// so that "the tree is returned" does not quietly mean "a tree that draws
// something".
func TestTheEmptyFrameLaysOutAsAnEmptyPage(t *testing.T) {
	frame, _, _ := html.Parse("")
	if frame == nil {
		t.Fatal("an empty document produced no tree")
	}
	composed := Compose(Input{HTML: ""}, Options{})
	for _, op := range composed.Ops {
		switch op.(type) {
		case DrawText, DrawImage:
			t.Errorf("an empty document drew %T", op)
		}
	}
}

func ruleNames(fs []Finding) []Rule {
	out := make([]Rule, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return out
}
