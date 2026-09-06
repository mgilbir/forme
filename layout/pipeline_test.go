package layout

import "testing"

// TestAStylesheetForPaperIsAppliedToPaper is the end of the @media story: the
// commonest line in any stylesheet meant for print — "@media print { .no-print
// { display: none } }" — used to leave the box on the page, because every
// at-rule was reported and skipped.
//
// It is here rather than in style/ because what an author checks is the page:
// the box is gone, and the one the same stylesheet hides on a screen is not.
func TestAStylesheetForPaperIsAppliedToPaper(t *testing.T) {
	const doc = `<p id="a">on paper</p><p id="b">everywhere</p>`

	got := Compose(Input{HTML: doc, CSS: []Stylesheet{{
		Source: `@media print { #a { display: none } }`}}}, Options{})
	if fragmentFor(got.Root, "a") != nil {
		t.Errorf("a box hidden by @media print is still on the page")
	}
	if fragmentFor(got.Root, "b") == nil {
		t.Errorf("the box outside the query went with it")
	}

	got = Compose(Input{HTML: doc, CSS: []Stylesheet{{
		Source: `@media screen { #a { display: none } }`}}}, Options{})
	if fragmentFor(got.Root, "a") == nil {
		t.Errorf("a box hidden only on a screen is missing from the paper")
	}
}

// TestAQueryIsAnsweredAboutTheSheetItWasGiven. The page Compose is handed is
// what the query asks about, and a caller printing on something else gets
// different rules — which is the whole reason the size is threaded through the
// styling rather than left to a default.
func TestAQueryIsAnsweredAboutTheSheetItWasGiven(t *testing.T) {
	const doc = `<p id="a">a</p>`
	const css = `@media (min-width: 400px) { #a { display: none } }`

	wide := Compose(Input{HTML: doc, CSS: []Stylesheet{{Source: css}}},
		Options{Page: PageSizePt(600, 800)})
	if fragmentFor(wide.Root, "a") != nil {
		t.Errorf("a 600pt sheet did not match (min-width: 400px)")
	}

	narrow := Compose(Input{HTML: doc, CSS: []Stylesheet{{Source: css}}},
		Options{Page: PageSizePt(200, 800)})
	if fragmentFor(narrow.Root, "a") == nil {
		t.Errorf("a 200pt sheet matched (min-width: 400px)")
	}
}
