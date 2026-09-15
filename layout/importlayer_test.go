package layout

import "testing"

// TestAnImportCarryingALayerNameIsNotFetched is the guard the note in
// style/layer.go rests on, and it is here rather than in style because this is
// where the claim is decidable: expansion happens in the pipeline, and a test
// that applies a sheet directly cannot see whether a sheet arrived.
//
// The first version of it was in style and asserted that the cascade reports
// @import — which it does whether or not the import was expanded, so a plant
// that let importReference accept a layer() prelude did not move it. The test
// was true and guarded nothing.
//
// What matters is narrower: import expansion takes a bare reference only, so an
// import carrying a layer name is left in the sheet rather than fetched. Were
// it fetched and the layer name dropped, its rules would arrive *unlayered* —
// and an unlayered rule beats every layered one, so a sheet the author put at
// the bottom of the layer order would win against all of them.
func TestAnImportCarryingALayerNameIsNotFetched(t *testing.T) {
	const imported = `#d { color: rgb(1, 2, 3) }`

	// The control: a bare @import is fetched, so the resolver is asked and the
	// rules arrive. Without this the assertion below passes on a resolver that
	// was never going to be asked for anything.
	plain := &fileResolver{files: map[string][]byte{"other.css": []byte(imported)}}
	Build(Input{
		HTML:      `<style>@import url(other.css);</style><div id="d">x</div>`,
		Resources: plain,
	})
	if len(plain.asked) == 0 {
		t.Fatal("a bare @import asked the resolver for nothing; the fixture does " +
			"not reach the expansion this is about")
	}

	// And the one under test: carrying a layer name, it is not fetched at all.
	layered := &fileResolver{files: map[string][]byte{"other.css": []byte(imported)}}
	built := Build(Input{
		HTML:      `<style>@layer base, other; @import url(other.css) layer(base);</style><div id="d">x</div>`,
		Resources: layered,
	})
	for _, ref := range layered.asked {
		if ref == "other.css" {
			t.Error("an @import carrying a layer name was fetched; its rules would " +
				"arrive unlayered, and an unlayered rule beats every layered one")
		}
	}
	// It is reported rather than passed over, because a sheet that does not
	// arrive is worth saying.
	found := false
	for _, f := range built.Findings {
		if f.Rule == RuleUnsupportedAtRule {
			found = true
		}
	}
	if !found {
		t.Errorf("nothing said the import was not applied: %v", built.Findings)
	}
}

// TestALayerStatementDoesNotStopTheImports is a defect the test above found,
// and it predates the cascade taking @layer at all.
//
// CSS Cascade §3.1 says @import must precede every other rule "ignoring @charset
// and @layer statement rules" — the comment in expandImports says so in as many
// words — and the scan stopped at the @layer anyway. So
//
//	@layer reset, base;
//	@import url(reset.css) ;
//
// which is how a stylesheet written in layers begins, never fetched anything at
// all. It mattered less while layers were dropped whole; it matters now.
//
// The fix is what the sheet keeps rather than what it loses: the imports are cut
// out of it one at a time, so a @layer statement written among them stays where
// it was and the order it fixes survives.
func TestALayerStatementDoesNotStopTheImports(t *testing.T) {
	const imported = `#d { color: rgb(1, 2, 3) }`
	for _, css := range []string{
		`@import url(other.css);`,
		`@charset "utf-8"; @import url(other.css);`,
		`@layer a, b; @import url(other.css);`,
		`@charset "utf-8"; @layer a, b; @import url(other.css);`,
		`@import url(other.css); @layer a, b;`,
	} {
		res := &fileResolver{files: map[string][]byte{"other.css": []byte(imported)}}
		Build(Input{
			HTML:      `<style>` + css + `</style><div id="d">x</div>`,
			Resources: res,
		})
		asked := false
		for _, ref := range res.asked {
			if ref == "other.css" {
				asked = true
			}
		}
		if !asked {
			t.Errorf("%q fetched nothing; a @layer statement is allowed among the "+
				"imports and must not end them", css)
		}
	}
}

// TestTheLayerOrderSurvivesTheImports is the other half: the statement has to
// stay in the sheet, because the order it fixes is the whole of its effect.
//
// Cutting the whole leading run would fetch the import and lose the order,
// which is a subtler wrong than not fetching at all — the rules would arrive
// and land in layers numbered by where their blocks happen to sit.
//
// A statement is allowed on either side of the imports and the two are saved by
// different halves of the change, so both are written out here: one before is
// handed to the cascade ahead of the imported sheets, one after is left in the
// sheet by the imports being cut out of it a span at a time.
func TestTheLayerOrderSurvivesTheImports(t *testing.T) {
	// "theme" is named first, so it loses to "base" however the blocks are
	// written. If the statement were cut away, the blocks would fix the order
	// themselves and theme — written later — would win.
	const blocks = `
		@layer base { #d { color: rgb(0, 0, 255) } }
		@layer theme { #d { color: rgb(0, 255, 0) } }`
	for _, sheet := range []string{
		`@layer theme, base; @import url(other.css);` + blocks,
		`@import url(other.css); @layer theme, base;` + blocks,
	} {
		built := Build(Input{
			HTML:      `<style>` + sheet + `</style><div id="d">x</div>`,
			Resources: &fileResolver{files: map[string][]byte{"other.css": []byte(`#d { font-style: italic }`)}},
		})
		found := boxWithID(t, built.Root, "d")
		if got := found.Style["color"]; got != "rgb(0, 0, 255)" {
			t.Errorf("%q: the colour is %q, want the blue of the layer named last "+
				"in the statement; the statement fixes the order and has to survive "+
				"the imports being lifted out", sheet, got)
		}
		// And the imported sheet arrived, or the test above is what failed.
		if got := found.Style["font-style"]; got != "italic" {
			t.Errorf("%q: the imported sheet did not arrive: font-style is %q", sheet, got)
		}
	}
}

// TestTheLayerOrderIsFixedBeforeTheImportedSheet is the half of the statement's
// effect that survived the fix above and was still wrong.
//
// An imported sheet is lifted out and applied *before* what is left of the
// sheet that imported it — that is what "@import comes first" means once the
// sheets are flattened. A @layer statement left where it was written would then
// run after the imported sheet had already declared those same layers itself,
// in the order its blocks happen to sit in, and the statement would find them
// all named and change nothing.
//
// So this document
//
//	@layer theme, base;
//	@import url(other.css);        /* @layer base {...} @layer theme {...} */
//
// gave the win to theme: the layer the author named first precisely so that it
// would lose. The statement is handed to the cascade ahead of the sheets it
// orders instead, which is where it was written.
func TestTheLayerOrderIsFixedBeforeTheImportedSheet(t *testing.T) {
	built := Build(Input{
		HTML: `<style>@layer theme, base;
			@import url(other.css);</style><div id="d">x</div>`,
		Resources: &fileResolver{files: map[string][]byte{"other.css": []byte(
			`@layer base { #d { color: rgb(0, 0, 255) } }
			 @layer theme { #d { color: rgb(0, 255, 0) } }`)}},
	})
	found := boxWithID(t, built.Root, "d")
	if got := found.Style["color"]; got != "rgb(0, 0, 255)" {
		t.Errorf("the colour is %q, want the blue of the layer the statement "+
			"named last; the statement was written above the @import and has to "+
			"reach the cascade before the sheet it orders", got)
	}
}
