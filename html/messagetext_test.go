package html

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/diag"
)

// A message quotes the document, and the document is bytes.
//
// The scheduled fuzz run (run 36294627182, on dd4394f) found "<A\x93": a tag
// name ending in a byte that begins no UTF-8 character, which the tag name
// state keeps as it keeps anything that is not white space, "/" or ">". The
// message about it quoted the name, and so was not text. Every place in this
// package that quotes a name now passes it through shown, and add makes every
// message text whatever it quotes. Each half has its own test below, because
// each hides the other from the fuzz target: with either in place the crasher
// passes.

// TestAMessageQuotingTheDocumentIsText is the crasher and the shapes beside it:
// a byte that is not UTF-8, a C0 control and a C1 control, in a tag name, an
// end tag name, and an attribute name, in each of the messages that quote them.
func TestAMessageQuotingTheDocumentIsText(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string // one message the document must produce, exactly
	}{
		{"<A\x93", "the tag <a� is never closed"},
		{"<a\x01b>x", "<a?b> is never closed"},
		{"</x\u0085>", "</x?> closes nothing: no <x?> is open here"},
		{"</p\x93", "the end tag </p� is not closed with \">\""},
		{"<p a\x93=1 a\x93=2>", "the attribute \"a�\" appears twice on <p>"},
		{"<p a\x1b=>", "the attribute \"a?\" has no value"},
		{"<p a\x93='x", "the value of \"a�\" is never closed"},
		{"<p a\x93\"=1>", "'\"' in an attribute name of <p> is part of the name, " +
			"which is how HTML reads it; a space or a quote is missing"},
		{"<p\x93 a=b\"c>", "'\"' in the unquoted value of \"a\" is part of the value, " +
			"which is how HTML reads it; quote the value"},
		{"<table><x\x93>", "<x�> is not table content and was written inside a table; " +
			"it belongs before the table and is read there"},
		{"<b\x93><div></b\x93>", "</b�> cannot close the <b�> outside the <div> " +
			"it is written in, and is ignored"},
	} {
		_, errs, _ := Parse(tc.src)
		found := false
		for _, e := range errs {
			if !diag.IsText(e.Message) {
				t.Errorf("%q: a message that is not text: %q", tc.src, e.Message)
			}
			found = found || e.Message == tc.want
		}
		if !found {
			t.Errorf("%q: no message %q among %q", tc.src, tc.want, errs)
		}
	}
}

// TestANameIsCutInAMessage holds shown to its other half: a name is the
// document's to choose, and a message quoting it quotes the first
// maxShownBytes of it.
func TestANameIsCutInAMessage(t *testing.T) {
	long := strings.Repeat("x", 5000)
	for _, src := range []string{
		"<" + long,
		"<" + long + ">",
		"</" + long + ">",
		"<p " + long + "=1 " + long + "=2>",
		"<table><" + long + ">",
	} {
		_, errs, _ := Parse(src)
		if len(errs) == 0 {
			t.Fatalf("%.20q...: no message at all", src)
		}
		cut := false
		for _, e := range errs {
			if len(e.Message) > 400 {
				t.Errorf("%.20q...: a message of %d bytes: %.120q...", src, len(e.Message), e.Message)
			}
			cut = cut || strings.Contains(e.Message, strings.Repeat("x", maxShownBytes)+"...")
		}
		if !cut {
			t.Errorf("%.20q...: no message quotes the name cut: %q", src, errs)
		}
	}
}

// TestEveryMessageIsMadeText drives the tokenizer's sink directly, with
// messages written as a careless new call site would write them.
func TestEveryMessageIsMadeText(t *testing.T) {
	tok := newTokenizer("", false)
	tok.fail(0, "<a\x93> by fail")
	tok.unsupported(0, "<a\x01> by unsupported")
	tok.limit(0, "<a\u009b> by limit")
	tok.stopped(0, "<a\x93\x7f> by stopped")
	want := []string{
		"<a�> by fail",
		"<a?> by unsupported",
		"<a?> by limit",
		"<a�?> by stopped",
	}
	if len(tok.errs) != len(want) {
		t.Fatalf("recorded %d problems, want %d", len(tok.errs), len(want))
	}
	for i, e := range tok.errs {
		if e.Message != want[i] {
			t.Errorf("problem %d is %q, want %q", i, e.Message, want[i])
		}
	}
}
