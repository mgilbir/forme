package shape

import (
	"testing"
)

// The default model held to HarfBuzz's, over the scripts that have one of
// their own.
//
// A language tag can name the script tag a run is looked up under:
// "und-x-hbscdflt" names 'DFLT', and a font that states its rules there has a
// Javanese, Balinese or Tibetan run set by the default model — the font's
// features over the text in the order it is written, with no syllables and no
// reordering — which is HarfBuzz's hb-ot-shaper-default. That is what a font
// written without the script's own model gets too, since it states its rules
// only under 'DFLT' or 'latn' (see categorize). The corpora are the ones the
// script models are held to, shaped by HarfBuzz under that tag; the three
// fonts all state rules under 'DFLT', so the model is the default one on both
// sides.
//
// Under this tag the answers differ from the script models' in 630 Javanese,
// 404 Balinese and 1,324 Tibetan strings, so these are not the same assertion
// made twice. Khmer's Noto answers the same either way and is not repeated.
var defaultModelCases = []struct {
	name, font, corpus, expected string
}{
	{name: "javanese", font: "fonts/NotoSansJavanese.ttf",
		corpus: "javanese.txt", expected: "javanese.dflt.expected.txt"},
	{name: "balinese", font: "fonts/NotoSansBalinese.ttf",
		corpus: "balinese.txt", expected: "balinese.dflt.expected.txt"},
	{name: "tibetan", font: "fonts/NotoSerifTibetan.ttf",
		corpus: "tibetan.txt", expected: "tibetan.dflt.expected.txt"},
}

// TestTheDefaultModelAgreesWithHarfBuzz compares every case, each shaped in the
// language its expectations record.
func TestTheDefaultModelAgreesWithHarfBuzz(t *testing.T) {
	for _, tc := range defaultModelCases {
		t.Run(tc.name, func(t *testing.T) {
			corpus, expected, header := readHarfBuzzGolden(t, tc.corpus, tc.expected)
			lang := header["language"]
			if lang != "und-x-hbscdflt" {
				t.Fatalf("%s was shaped in %q; this compares the default model, named by und-x-hbscdflt",
					tc.expected, lang)
			}
			f := harfbuzzFace(t, tc.font, header)
			if len(corpus) != len(expected) || len(corpus) == 0 {
				t.Fatalf("%d strings in %s and %d lines of expectations", len(corpus), tc.corpus, len(expected))
			}
			differing := 0
			for i, s := range corpus {
				glyphs, _ := f.ShapeGlyphsInContext(s, "", "", Features{Language: lang})
				if same, why := sameAsHarfBuzz(f, glyphs, expected[i]); !same {
					differing++
					if differing <= 40 {
						t.Errorf("%s\n  %s\n  forme     %s\n  harfbuzz %s",
							describeRunes(s), why, describeGlyphs(glyphs), describeExpected(f, expected[i]))
					}
				}
			}
			if differing > 40 {
				t.Errorf("... and %d more", differing-40)
			}
			t.Logf("%d of %d agree (harfbuzz %s, %s)", len(corpus)-differing, len(corpus),
				header["harfbuzz"], lang)
		})
	}
}
