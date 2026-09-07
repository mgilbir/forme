package font

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestTheSecondCodesForSpaceAndHyphenAreThere.
//
// PDF 32000-1 Annex D.2 gives three codes a second name: WinAnsiEncoding's
// 0xA0 is SPACE and 0xAD is HYPHEN, and MacRomanEncoding's 0xCA is SPACE. They
// are the no-break space and the soft hyphen of the source encodings, and each
// was missing — so a document holding one had nothing to encode it as, in a
// table whose whole job is to say what a code stands for.
func TestTheSecondCodesForSpaceAndHyphenAreThere(t *testing.T) {
	for _, tc := range []struct {
		table map[byte]string
		code  byte
		want  string
		what  string
	}{
		{WinAnsiEncodingNames, 0xA0, "space", "WinAnsi's no-break space"},
		{WinAnsiEncodingNames, 0xAD, "hyphen", "WinAnsi's soft hyphen"},
		{MacRomanEncodingNames, 0xCA, "space", "MacRoman's no-break space"},

		// The first codes, which were never in doubt.
		{WinAnsiEncodingNames, 0x20, "space", "WinAnsi's ordinary space"},
		{WinAnsiEncodingNames, 0x2D, "hyphen", "WinAnsi's ordinary hyphen"},
		{MacRomanEncodingNames, 0x20, "space", "MacRoman's ordinary space"},
	} {
		if got := tc.table[tc.code]; got != tc.want {
			t.Errorf("%s: code 0x%02X is %q, want %q", tc.what, tc.code, got, tc.want)
		}
	}
}

// TestABudgetThatStoppedShortSaysSo.
//
// A cmap walk that gives up on its work bound hands back what it has, nil where
// that is nothing, and the partial flag set. The flag used to be "did I read
// anything", so a budget that ran out before the first mapping reported false —
// and an empty map is nil to the caller, which is how "this font has no cmap"
// is spelt. The two were the same answer, so a reader that gave up looked like
// a font with nothing in it.
//
// The map stays nil, which is the other half of the rule: a non-nil cmap is
// authoritative, so an empty one answers .notdef for every code.
func TestABudgetThatStoppedShortSaysSo(t *testing.T) {
	// A format 4 table mapping a hundred characters, read with no budget at
	// all, with one unit, and with enough.
	segs := [][3]int{{0x41, 0x41 + 99, 1}, {0xFFFF, 0xFFFF, 1}}
	b := fonttest.CmapFormat4(segs)

	for _, tc := range []struct {
		work    int
		nonNil  bool
		partial bool
		what    string
	}{
		{0, false, true, "no budget at all"},
		{1, true, true, "one unit"},
		{1000, true, false, "enough"},
	} {
		m, partial := ParseCmapSubtable(b, tc.work)
		if (m != nil) != tc.nonNil {
			t.Errorf("%s: the map is non-nil = %v, want %v", tc.what, m != nil, tc.nonNil)
		}
		if partial != tc.partial {
			t.Errorf("%s: partial = %v, want %v", tc.what, partial, tc.partial)
		}
	}
}
