package layout

import (
	"strings"
	"testing"
)

// Where a word ends, for "text-transform: capitalize".
//
// CSS Text §2.1 titlecases the first letter of each word, and a word boundary is
// a question about the text rather than about one text node: in "<b>e</b>xample"
// the "x" does not begin a word, so the boundary has to be carried across the
// elements between them. A block-level box resets it, because a block begins its
// text afresh.
//
// A <br> does the same thing for the same reason and did not: it is a line break
// the author wrote, and a word does not continue across a line the author ended.
// "i ask<br/>questions" came out "I Ask" and "questions".
//
// The suite writes its expectation out in full — text-transform-cap-003 is six
// paragraphs and a reference that spells each one — which is why the cases below
// are that document rather than a shape of my own choosing.

// transformed is the text a document put on the page, runs joined.
func transformed(t *testing.T, markup string) string {
	t.Helper()
	var runs []string
	for _, op := range paintOf(t, `<p id="p">`+markup+`</p>`, `#p { font-size: 16px }`) {
		if v, ok := op.(DrawText); ok {
			runs = append(runs, v.Text)
		}
	}
	return strings.Join(runs, "")
}

// TestCapitalizeFindsTheWordBoundariesTheReaderSees is the suite's own six cases.
func TestCapitalizeFindsTheWordBoundariesTheReaderSees(t *testing.T) {
	for _, tc := range []struct{ what, markup, want string }{
		{"a quotation mark starts a word",
			`<span style="text-transform:capitalize">i ask "questions"</span>`,
			`I Ask "Questions"`},
		{"and it starts one across an element boundary",
			`i ask "<span style="text-transform:capitalize">questions"</span>`,
			`i ask "Questions"`},
		{"an untransformed letter still begins the word it is in",
			`<span style="text-transform:capitalize">i ask ` +
				`<span style="text-transform:none">q</span>uestions</span>`,
			`I Ask questions`},
		{"a line break the author wrote ends a word",
			`<span style="text-transform:capitalize">i ask<br />questions</span>`,
			`I AskQuestions`},
		{"a no-break space is still a boundary",
			`<span style="text-transform:capitalize">i ask&#160;questions</span>`,
			"I Ask\u00a0Questions"},
		{"and nowrap changes nothing",
			`<span style="text-transform:capitalize">i ask questions</span>`,
			`I Ask Questions`},
	} {
		if got := transformed(t, tc.markup); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.what, got, tc.want)
		}
	}
}

// TestAWordBreakOpportunityIsNotAWordBoundary is the containment case, and it is
// the element most easily confused with the one that was fixed.
//
// <wbr> offers a place a line may break and marks no boundary in the text, so
// "sur<wbr/>name" is one word and takes one capital. Treating every inline
// element that affects line breaking as a boundary would capitalise the middle of
// a word an author only wanted to be breakable.
func TestAWordBreakOpportunityIsNotAWordBoundary(t *testing.T) {
	got := transformed(t, `<span style="text-transform:capitalize">sur<wbr />name</span>`)
	if got != "Surname" {
		t.Errorf("got %q, want %q — a <wbr> is an opportunity, not a boundary",
			got, "Surname")
	}
}

// TestABlockStillEndsAWord, which is what the reset was there for before, and
// must keep working: two paragraphs are two lines of text whatever the last
// letter of the first one was.
func TestABlockStillEndsAWord(t *testing.T) {
	var runs []string
	for _, op := range paintOf(t,
		`<div id="d"><p>hi</p><p>there</p></div>`,
		`#d { text-transform: capitalize; font-size: 16px }`) {
		if v, ok := op.(DrawText); ok {
			runs = append(runs, v.Text)
		}
	}
	got := strings.Join(runs, "|")
	if got != "Hi|There" {
		t.Errorf("got %q, want %q", got, "Hi|There")
	}
}

// TestOnlyABreakEndsTheWordAmongTheInlines. The rule is about one element, and a
// span or an emphasis in the middle of a word must not end it — which is the
// case the boundary is carried across elements for in the first place.
func TestOnlyABreakEndsTheWordAmongTheInlines(t *testing.T) {
	for _, tag := range []string{"span", "em", "b", "i"} {
		got := transformed(t, `<span style="text-transform:capitalize">e<`+tag+
			`>x</`+tag+`>ample</span>`)
		if got != "Example" {
			t.Errorf("<%s> in the middle of a word gave %q, want %q", tag, got, "Example")
		}
	}
}
