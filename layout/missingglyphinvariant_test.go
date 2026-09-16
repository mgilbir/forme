package layout

import (
	"fmt"
	"strings"
	"testing"
)

// A character the engine reports as missing is one no face it has can set.
//
// That is what the glyph-missing finding means — "the face ... has no glyph for
// ..., which is set as a space, so the character is missing from the page and
// from the text extracted out of it" — and it is a claim about the whole font
// set rather than about the face the run happened to land in. A report that is
// true of the face and false of the set is not a report about the document: it
// is the engine describing a choice it made, as though it were a limit it ran
// into.
//
// Nothing checked it, and the case it did not check was live. A grapheme cluster
// whose halves live in different faces — "⛹🏿", the person in Noto Sans Symbols
// and the skin tone in Unifont Upper — had no one face that could set the whole
// of it, so it stayed with the primary, which could set neither half, and both
// characters were reported missing while the set held a face for each. The cut
// in faceRunsFor is what fixed that; this is the statement of what it fixed,
// over more clusters than the fix was written against.
//
// It is not a tautology. With the cut removed it names six characters across
// three of the texts below, which is how it was checked before being believed.
//
// The list is chosen rather than sampled, and that is deliberate: the shapes
// here are the ones where a cluster can span two faces — an emoji with a
// modifier, a joined sequence, a flag, a keycap, a mark on a base from a script
// the primary face does not cover. A corpus that cannot reach a branch says
// nothing about it however long it runs.
func TestACharacterReportedMissingHasNoFace(t *testing.T) {
	set := fallbackFontSet(t)
	fall, ok := set.(FallbackFontSet)
	if !ok {
		t.Fatal("the suite's font set offers no fallback, so there is no set to " +
			"ask whether a reported character is really missing from")
	}

	for _, text := range []string{
		// Emoji clusters whose parts live in different faces.
		"⛹\U0001F3FF",
		"⛹\U0001F3FF‍♀️",
		"\U0001F939\U0001F3FF‍♀️",
		"\U0001F468‍\U0001F469‍\U0001F466",
		"\U0001F1E6\U0001F1E7",
		"\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F",
		"1️⃣",
		"©️",
		"❤️‍\U0001F525",
		// Marks on bases, where the base and the mark need not be in one face.
		"กุ้", "क्ष", "بَّ",
		"각", "é̂̃",
		"א́", "ا́", "ა́", "ա́",
		"Ꭰ́", "ሀ́", "ཀི",
	} {
		_, findings := layoutWith(t, set,
			`<div id="d">`+text+`</div>`, `#d{font-size:20px}`)
		for _, found := range findings {
			if found.Rule != RuleGlyphMissing {
				continue
			}
			for _, r := range text {
				if !strings.Contains(found.Message, fmt.Sprintf("U+%04X", r)) {
					continue
				}
				if _, has := fall.FaceFor(string(r), false, false); has {
					t.Errorf("%q: U+%04X was reported missing and the set has a face "+
						"that can set it; the report describes where the character "+
						"was put rather than what could be done with it",
						text, r)
				}
			}
		}
	}
}
