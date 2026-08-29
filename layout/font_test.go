package layout

import "testing"

// Which face a box is set in when the families it named are not there.

// TestAFamilyNobodyHasFallsBackToTheOneNobodyNamed.
//
// A document naming a family that has not loaded is set in the same type as the
// document beside it that named none. Both are font-family's initial value,
// which is what "the default font" means here.
//
// It used to be sans-serif, which made those two documents disagree: the initial
// value is "serif", so a plain <div> came out in the serif face and a <div>
// asking for a family that had not loaded came out in the sans one. content-076
// is that difference exactly — "<font face='PASS PASS'>", a family nobody has,
// against a reference that names none.
func TestAFamilyNobodyHasFallsBackToTheOneNobodyNamed(t *testing.T) {
	faceOf := func(css string) string {
		t.Helper()
		for _, op := range paintOf(t, `<div id="d">x</div>`, css) {
			if d, ok := op.(DrawText); ok && d.Text == "x" && d.Face != nil {
				return d.Face.Name()
			}
		}
		t.Fatalf("no run for %q", css)
		return ""
	}
	none := faceOf(`#d { font-size: 16px }`)
	missing := faceOf(`#d { font-size: 16px; font-family: "no-such-family" }`)
	if none != missing {
		t.Errorf("a box naming no family is set in %q and one naming a family that "+
			"has not loaded in %q; both are the initial value", none, missing)
	}
	// And a family that *is* available is still used, so this is not passing by
	// ignoring the property.
	if got := faceOf(`#d { font-size: 16px; font-family: Courier }`); got == none {
		t.Errorf("a family that is available was ignored: %q", got)
	}
	// A generic in the list is still honoured, and is what an author writes to
	// ask for the other default on purpose.
	if got := faceOf(`#d { font-size: 16px; font-family: "no-such-family", sans-serif }`); got == none {
		t.Errorf("the sans-serif after the missing family was ignored: %q", got)
	}
}
