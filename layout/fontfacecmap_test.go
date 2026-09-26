package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// TestAFontFaceWhoseCmapNamesNoGlyphSaysSo is css-text/zwnj-renders-invisible:
// an @font-face whose font, WPT's fonts/ahem-visible-zwnj.otf, maps A, B and
// U+200C through format-4 deltas onto glyphs 35 and 36 of a font whose maxp
// declares three.
//
// Refusing it is right, and it is what the browsers do: OTS, which Chrome and
// Firefox pass every web font through, rejects the same cmap for "Range glyph
// reference too high". What was wrong was the reason given for it: "the font
// has no Unicode character map", which is false of a font carrying two.
func TestAFontFaceWhoseCmapNamesNoGlyphSaysSo(t *testing.T) {
	ghosts := fonttest.CmapFormat4([][3]int{
		{0x41, 0x42, -30}, {0x200C, 0x200C, -8169}, {0xFFFF, 0xFFFF, 1},
	})
	cmap := font.SFNTTables(fonttest.SFNTWithCmapSubtables([]fonttest.CmapSub{
		{Plat: 0, Enc: 3, Data: ghosts}, {Plat: 3, Enc: 1, Data: ghosts},
	}))["cmap"]
	otf := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: 1000, HasShape: true},
			{Rune: 'B', Advance: 1000, HasShape: true},
		},
		Extra: map[string][]byte{"cmap": cmap},
	})

	built := Build(Input{
		HTML: `<style>
@font-face { font-family: ahem-zwnj; src: url('../../fonts/ahem-visible-zwnj.otf'); }
.ahem-zwnj { font-family: ahem-zwnj; }
body { font-size: 60px; }
</style><body><div class="ahem-zwnj">&#x034F;&#x200C;</div></body>`,
		Resources: &fileResolver{files: map[string][]byte{"../../fonts/ahem-visible-zwnj.otf": otf}},
	})

	// The refusal is unchanged: the family did not load.
	requireFinding(t, built.Findings, RuleResourceBlocked, `the @font-face for "ahem-zwnj" loaded no font`)
	// And the reason is the font's.
	requireFinding(t, built.Findings, RuleResourceBlocked,
		"the font's Unicode character map names no glyph the font has")
	for _, f := range built.Findings {
		if strings.Contains(f.Message, "has no Unicode character map") {
			t.Errorf("a font carrying a Unicode character map was said to have none: %s", f.Error())
		}
	}
}
