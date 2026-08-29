package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
)

// The size a document is set at when it has not said, and why "monospace" makes
// it a different number.
//
// CSS 2.1 §15.7 makes the absolute size keywords a user agent's own scale and
// then says the scale need not be one scale — "the table of scaling factors"
// may differ between fonts, because "the x-height of a monospace font is usually
// smaller". Every browser keeps a second preference for it at thirteen pixels
// against sixteen, and the suite's references are drawn by browsers.
//
// The eight table-anonymous-objects tests that share
// no_red_3x3_monospace_multi_table-ref turn on it and on nothing else: their
// reference has 790 pixels for a row of forty-two monospaced characters, which
// is 806 at 32px and 655 at 26. The reference wrapped a line the test — which
// says "white-space: nowrap" — could not.

// sizeOfIn computes an element's font-size through the cascade in a document of
// the caller's own, which is what these need: the rule is about an element's
// family and its ancestors' declarations, so the tree has to carry both.
func sizeOfIn(t *testing.T, markup, sheet, selector string) float64 {
	t.Helper()
	rules, errs := css.ParseStylesheet(sheet)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	doc := parseDoc(t, markup)
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	n := elementFor(t, doc, selector)
	vals, _ := css.ParseComponentValues(got.Styles[n]["font-size"])
	l, _, ok := ParseLength(vals, LengthContext{})
	if !ok || l.Kind != LengthAbsolute {
		t.Fatalf("the computed font-size of %s is %q, which is not an absolute "+
			"length", selector, got.Styles[n]["font-size"])
	}
	return l.Value.Px()
}

const monoMarkup = `<div id="outer"><div id="inner">x</div></div>`

func TestAMonospaceDocumentWithNoSizeIsSetSmaller(t *testing.T) {
	got := sizeOfIn(t, monoMarkup, `#outer { font-family: monospace }`, "#outer")
	if got != DefaultMonospaceFontSize {
		t.Errorf("an element asking for a monospaced face and no size computed "+
			"%gpx, want %gpx: §15.7's scale is the user agent's and a user agent "+
			"keeps a second one for monospace", got, float64(DefaultMonospaceFontSize))
	}
}

func TestASizeUnderAMonospaceDefaultIsRelativeToIt(t *testing.T) {
	got := sizeOfIn(t, monoMarkup,
		`#outer { font-family: monospace } #inner { font-size: 2em }`, "#inner")
	if got != 26 {
		t.Errorf("\"font-size: 2em\" inside a monospaced document computed %gpx, "+
			"want 26px: an em is twice the 13px its parent was set at, not twice "+
			"the 16px a proportional document would have been", got)
	}
}

func TestTheDefaultScaleStopsAtAStatedSize(t *testing.T) {
	// The outer element states a size, so the inner one is measured from what
	// was stated. Its family changes the face and not the number.
	got := sizeOfIn(t, monoMarkup,
		`#outer { font-size: 20px } #inner { font-family: monospace }`, "#inner")
	if got != 20 {
		t.Errorf("a monospaced element under a stated 20px computed %gpx, want "+
			"20px: a default applies where nothing has been stated, and 20px was "+
			"stated", got)
	}
}

func TestTheStatedSizeNeedNotBeAnAncestorsOwn(t *testing.T) {
	// Stated on the element itself, which is the same rule read at zero depth.
	got := sizeOfIn(t, monoMarkup,
		`#outer { font-family: monospace; font-size: medium }`, "#outer")
	if got != DefaultFontSize {
		t.Errorf("a monospaced element that states \"medium\" computed %gpx, want "+
			"%gpx", got, float64(DefaultFontSize))
	}
}

func TestComingOffTheMonospaceScale(t *testing.T) {
	// Which scale an element is on is its own family's answer, so a serif inside
	// a monospaced document is back on the proportional one. Nothing has stated
	// a size anywhere, so both elements are still at a default — different
	// defaults.
	sheet := `#outer { font-family: monospace } #inner { font-family: serif }`
	if got := sizeOfIn(t, monoMarkup, sheet, "#outer"); got != DefaultMonospaceFontSize {
		t.Errorf("the monospaced outer element computed %gpx, want %gpx", got,
			float64(DefaultMonospaceFontSize))
	}
	if got := sizeOfIn(t, monoMarkup, sheet, "#inner"); got != DefaultFontSize {
		t.Errorf("a serif element inside a monospaced document computed %gpx, "+
			"want %gpx: the scale follows the element's own family", got,
			float64(DefaultFontSize))
	}
}

func TestOnlyTheFirstFamilyChoosesTheScale(t *testing.T) {
	// A list is a preference in order, and only the first entry says what the
	// element is meant to look like.
	for _, tc := range []struct {
		family string
		want   float64
	}{
		{"monospace", DefaultMonospaceFontSize},
		{"ui-monospace", DefaultMonospaceFontSize},
		{"monospace, serif", DefaultMonospaceFontSize},
		{"MONOSPACE", DefaultMonospaceFontSize},
		{`"monospace"`, DefaultMonospaceFontSize},
		{"Georgia, monospace", DefaultFontSize},
		{"Courier", DefaultFontSize},
		{"serif", DefaultFontSize},
	} {
		got := sizeOfIn(t, monoMarkup, `#outer { font-family: `+tc.family+` }`, "#outer")
		if got != tc.want {
			t.Errorf("\"font-family: %s\" with no size computed %gpx, want %gpx",
				tc.family, got, tc.want)
		}
	}
}

func TestAnElementThatNamesNoFamilyIsProportional(t *testing.T) {
	if got := sizeOfIn(t, monoMarkup, ``, "#outer"); got != DefaultFontSize {
		t.Errorf("an element naming no family computed %gpx, want %gpx", got,
			float64(DefaultFontSize))
	}
}
