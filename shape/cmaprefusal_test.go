package shape

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// withCmap is a two-glyph TrueType font — .notdef, then glyphs 1 and 2 for A
// and B — whose cmap is replaced by the given subtables.
func withCmap(subs ...fonttest.CmapSub) []byte {
	cmap := font.SFNTTables(fonttest.SFNTWithCmapSubtables(subs))["cmap"]
	return fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: 1000, HasShape: true},
			{Rune: 'B', Advance: 1000, HasShape: true},
		},
		Extra: map[string][]byte{"cmap": cmap},
	})
}

// TestARefusedCmapSaysWhatIsWrongWithIt is the refusal of a font that maps no
// character to a glyph, told three ways because there are three such fonts.
//
// The one that matters is WPT's fonts/ahem-visible-zwnj.otf, loaded by
// css/css-text/zwnj-renders-invisible.html: its cmap is there, in two format-4
// subtables, and every glyph it names is past the three its maxp declares. The
// refusal is right — OTS, which Chrome and Firefox pass every web font through,
// rejects the same cmap for "Range glyph reference too high" — but it was
// reported as "the font has no Unicode character map", which is false of it.
func TestARefusedCmapSaysWhatIsWrongWithIt(t *testing.T) {
	ghosts := fonttest.CmapFormat4([][3]int{
		{0x41, 0x42, -30}, {0x200C, 0x200C, -8169}, {0xFFFF, 0xFFFF, 1},
	})
	format2 := fonttest.CmapFormat4([][3]int{{0x41, 0x42, -64}})
	format2[1] = 2
	symbol := fonttest.CmapFormat4([][3]int{{0xF041, 0xF042, 0x10000 - 0xF040}, {0xFFFF, 0xFFFF, 1}})

	for _, tc := range []struct {
		what    string
		subs    []fonttest.CmapSub
		want    string
		notWant string
	}{
		{
			what:    "a map naming only glyphs past maxp",
			subs:    []fonttest.CmapSub{{Plat: 0, Enc: 3, Data: ghosts}, {Plat: 3, Enc: 1, Data: ghosts}},
			want:    "names no glyph the font has: every character in it is mapped to a glyph index past the 3 glyphs",
			notWant: "has no Unicode character map",
		},
		{
			what:    "a Unicode subtable in a format this does not read",
			subs:    []fonttest.CmapSub{{Plat: 3, Enc: 1, Data: format2}},
			want:    "Unicode character map could not be read",
			notWant: "has no Unicode character map",
		},
		{
			what: "no Unicode subtable at all",
			subs: []fonttest.CmapSub{{Plat: 3, Enc: 0, Data: symbol}},
			want: "the font has no Unicode character map",
		},
	} {
		_, err := Load(withCmap(tc.subs...))
		if err == nil {
			t.Errorf("%s: the font loaded; it maps no character and must be refused", tc.what)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: refused with %q, want it to say %q", tc.what, err, tc.want)
		}
		if tc.notWant != "" && strings.Contains(err.Error(), tc.notWant) {
			t.Errorf("%s: refused with %q, which says %q of a font that has one",
				tc.what, err, tc.notWant)
		}
	}

	// The control: the same font with deltas that land on its glyphs loads, so
	// the refusals above are the cmap's and not the fixture's.
	good := fonttest.CmapFormat4([][3]int{{0x41, 0x42, -64}, {0xFFFF, 0xFFFF, 1}})
	if _, err := Load(withCmap(fonttest.CmapSub{Plat: 3, Enc: 1, Data: good})); err != nil {
		t.Fatalf("the control font was refused: %v", err)
	}
}
